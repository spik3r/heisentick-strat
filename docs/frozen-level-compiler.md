# Compile-only frozen-level controls (HT-171)

The `frozenLevelBreakout` family compiles an explicitly selected, unverified research control. It does not reconstruct original trader instructions or execute a strategy. Every successful parse reports `frozen-level-quote-execution-unimplemented`.

The sole profile is `stored-m30-lock-isolation-v1`. Its complete fixed expansion lives in [`dsl/frozen_level_profile.json`](../dsl/frozen_level_profile.json). It binds the reviewed causal primitives and lifecycle at source commit `0ca22bf6493cd02e1f2adf13094981dfdcac148f`. The evidence supports `/3800` as a selected lock-location convention; entry, original risk and activation remain unresolved. Every variable setting and immutable reference must be authored. No defaults, sweep or historical execution are supplied.

## Exact phrase table

Keywords follow existing case-insensitive DSL matching; quoted identities preserve case, and the normalized control profile is its canonical lowercase ID. Every directive is required exactly once except description. Sections and fields may be ordered freely, but a directive in the wrong section is an error. Type may occur anywhere within setup. Numeric tokens are unquoted unsigned base-10 literals without exponent notation. No unknown/trailing tokens.

| Section | Exact phrase | Result and bounds |
|---|---|---|
| header | dsl v7 | v7 only |
| strategy | strategy "NAME" { ... } | nonempty name; JSON double-quoted strings only |
| strategy | description "TEXT" | optional metadata; never scanned as directives |
| setup | type: frozen level breakout | canonical family |
| setup | frozen control stored-m30-lock-isolation-v1 | sole initial profile |
| management | frozen lock none / frozen lock scale / frozen lock pivot | exactly one |
| management | frozen activation N R | finite N > 0, explicit for all arms; inert for none |
| management | frozen timeout N minutes | integer 1..150119987579; milliseconds <=9007199254740991 |
| risk | frozen grid N price units | finite decimal N > 0; unverified research grid |
| execution | frozen freshness predecessor N ms receipt M ms | integers N,M in 1..9007199254740991, inclusive bounds |
| execution | frozen latency entry N ms amendment M ms | integers N,M in 0..9007199254740991 |
| market conditions | frozen source "ID" version "VERSION" sha256 "DIGEST" | immutable paired-quote/source manifest |
| market conditions | frozen calendar "ID" version "VERSION" sha256 "DIGEST" | finite qualified broker-date/opening calendar |
| market conditions | frozen ordering "ID" version "VERSION" sha256 "DIGEST" | source/context total-order proof |
| market conditions | frozen schedule "ID" version "VERSION" sha256 "DIGEST" | operating/break/flat schedule and calendar binding |
| execution | frozen costs "ID" version "VERSION" sha256 "DIGEST" | spread, fee, penalty and financing contract |
| execution | frozen assumptions "ID" version "VERSION" sha256 "DIGEST" | exact candidate assumptions/registration manifest |

ID: 1..64 ASCII characters, pattern [A-Za-z0-9][A-Za-z0-9._-]*.
VERSION: 1..32 characters, same pattern.
DIGEST: exactly 64 lowercase hex characters.
These are opaque registry identities, not URLs, file paths, credentials or permission to fetch anything.

Numeric maxima are representation limits, not historical-job authorization or resource budgets.
The named reference example authors activation 1R, timeout 60 minutes, grid .01, predecessor age 5000ms and zero extra latency. These are not missing-field defaults. Receipt age is also explicitly authored; availability still follows the declared event-time reference model.
Every changed field changes config identity and must match separately registered assumptions before any later job. No sweep is implemented.

Ordinary ATR stops/targets, sessions, local clocks, legacy slices/bar execution, cooldown, partial exits, tranches, additions, recovery sizing and general expressions are forbidden even when apparently neutral. Audit all authored content, including inline leading/middle/trailing fragments. Preserve old-family behavior and PR85 quote-aware metadata tokenization.


## Normalized root and identity

[frozen-level-config-v1.schema.json](../spec/schemas/frozen-level-config-v1.schema.json) is normative for normalized JSON values.
The root has EXACTLY five required keys:
- dslVersion: integer 7
- setupType: string "frozenLevelBreakout"
- name: nonempty well-formed Unicode string, preserved exactly without trimming
- description: well-formed Unicode string, always present; omitted DSL description normalizes to ""; explicitly empty description produces the same config
- frozenLevelBreakout: the exact typed spec object in the schema

The inner object has exactly contractVersion, controlProfile, profile, lockMode, activationR, timeoutMinutes, priceGrid, maxPredecessorAgeMS, maxReceiptAgeMS, entryLatencyMS, amendmentLatencyMS and inputRefs.
Every field is mandatory. The full profile equals [`dsl/frozen_level_profile.json`](../dsl/frozen_level_profile.json) as normalized JSON values; altered/missing/extra fields, types or array membership/order fail, even with unchanged controlProfile.
No generic ATR/session/execution defaults, digest fields, warning fields or arbitrary metadata are admitted inside the root.

A failed new-family parse returns cfg={} and nonempty errors/diagnostics; it cannot return usable canonical bytes or digests.
The ordinary ParseResult/native/WASM envelope shape is unchanged. A separate Go identity helper can return canonical config/policy bytes and digests for tests and later callers; these derived fields live OUTSIDE the hashed config. Native/WASM tests exercise this helper without adding shared envelope fields.

### Exact policy projection

Start with a validated normalized root C. Build P with exactly these root keys:
1. P.dslVersion = C.dslVersion
2. P.setupType = C.setupType
3. P.frozenLevelBreakout = a value copy of the entire inner spec, except that inputRefs contains exactly source, calendar, ordering, schedule and costs

P has no name or description. The entire assumptions reference is absent, not null or blank. All other spec/profile/semantic reference fields are retained.
policyDigest = SHA256(canonical Go JSON bytes of P).
The assumptions payload binds policyDigest. Its exact payload bytes determine the assumptions reference.
configDigest = SHA256(canonical Go JSON bytes of complete C).
A separate run registration binds configDigest after the assumptions reference exists. The assumptions payload never contains configDigest; no self-referential hash cycle is permitted.

Canonical bytes are encoding/json.Marshal of the fully normalized Go map: object keys lexicographically ordered, no whitespace or final LF, standard Go escaping including HTML-sensitive characters and U+2028/U+2029, shortest finite float64 JSON for activationR/priceGrid, and integer JSON for version/duration/age/count fields. Fixed profile values are copied from the validated constant, not retained from caller maps. No trade-output 15-digit rounding.
Name/description and only the assumptions ref change configDigest but not policyDigest. Changes to any other normalized semantic value/ref change both (if they remain valid). Fixed-profile mutations are invalid and get no digest.
Whitespace, object order, supported escape aliases and numeric lexical aliases that normalize to the SAME values preserve canonical bytes/digests. This is distinct from actual normalized value changes.

### External payload hashes

InputRef.sha256 identifies the EXACT persisted payload bytes, not a reserialized object, pretty-print, newline-normalized text or separately JS-canonicalized value. Each future payload is an immutable UTF-8 JSON document; hash its supplied bytes before interpreting it, then apply its own versioned strict schema.
A trailing LF, property ordering or whitespace change changes that external payload hash. IDs/versions must agree with the payload's declared identity. Payloads must not contain their own resulting SHA as a self-reference.
The synthetic payloads in ../dsl/testdata/frozen_level/payloads/ demonstrate this byte contract; they explicitly have qualified=false and a fixture-only schema. They are NOT qualified market/calendar/cost data and must never pass future execution admission.

### Strict raw JSON boundary

Before any ordinary map/float64 conversion:
- Require one well-formed UTF-8 JSON document and EOF after optional JSON whitespace. Reject extra values, comments, BOM, malformed UTF-8 and invalid raw control characters.
- Reject duplicate object keys at EVERY depth using decoded names; "id" and "\u0069d" are duplicates.
- Validate quoted strings/escapes without repair. Valid surrogate pairs decode normally. Lone high/low surrogates, malformed escapes and unescaped controls fail. Legitimate U+FFFD is allowed and is distinct from repairing invalid bytes.
- Preserve every numeric token as exact text. For integer-valued fields, parse the mathematical decimal/exponent value exactly before integral/range checks; JSON 1, 1.0 and 1e0 are accepted integer aliases and normalize to integer 1. 9007199254740991.1 is fractional and MUST fail before float rounding. Exponent magnitude is capped at 10000 as a lexical resource bound; larger exponents fail even for zero. Out-of-range magnitudes fail safely.
- JSON grammar signs/exponents are accepted where the resulting field value is valid, except any negative zero spelling is rejected. Decimal fields normalize to finite positive float64; overflow, underflow-to-zero and nonfinite values fail. No string-to-number/null-to-zero conversion.
- The Go-map setupType may be a string or the known dsl.FamilyID type and normalizes to the canonical string; other string fields use valid strings. Go-map entry to the strict decoder accepts lossless integer types or json.Number for integer fields, not float32/float64 values whose fractional source could already have rounded away. Decimal fields accept finite float64, safe integer types or json.Number. Unknown Go types fail. Existing old-family decoders are unaffected.
- This strict decoder plus normalized schema is the contract; JSON Schema alone cannot observe duplicate keys, original Unicode escapes or numeric tokens after an unsafe decode.
- Bounded implementation limits: at most 1 MiB of new-family source/raw JSON, nesting depth 64 and numeric token length 1024 bytes. These are parser resource limits, not research parameter changes.

## Complete new-family source grammar

Header dsl v7 occurs exactly once before all blocks, apart from whitespace/comments.
Exactly one of each block follows in any order: strategy "NAME" {...}, market conditions {...}, setup {...}, risk {...}, management {...}, execution {...}.
Blocks cannot nest. Description is optional only within strategy. All directives in the phrase table remain required once and only in their designated sections. Type may be last in setup. Identical duplicates, repeated blocks/header/type/description, unknown sections and any outside/trailing fragments fail.

Keywords are case-insensitive; decoded name/description/ref ID/version retain case. The one recognized profile spelling normalizes to its canonical lowercase ID.
Only JSON-style double-quoted strings are supported for this family: backslash/quote/slash, b/f/n/r/t and valid uXXXX escapes. Literal unescaped newlines/control characters and single/backtick quote styles fail. Escaped metadata can contain braces, #, quotes, backslashes or directive-looking text without becoming syntax.
Outside strings, # comments end at LF. There are no // or block comments. Comments and insignificant whitespace do not affect config.

DSL integer tokens match 0 or [1-9][0-9]*. DSL decimal tokens match an unsigned integer with optional dot+digits, or dot+digits (so .01 and 0.01 normalize alike).
Reject 01, 1., signs, quoted numbers, exponents, Unicode digits, nan/inf, overflow and underflow-to-zero. Duration/age/latency tokens are converted as exact integers before bounds checks, not via float64.
A family-local full-source scanner must protect quoted content and account for every non-comment token. Do not reuse the old quote-unsafe # stripping for new-family metadata. Its selection/probe must not reinterpret valid old-family quoted metadata; if a generic parse would yield the new family, it must go through full strict validation before returning a config.

../dsl/testdata/frozen_level/example.strat is one complete invented source with type last, .01, escaped name/metadata, six real fixture-payload hashes and explicit settings.
../dsl/testdata/frozen_level/config.normalized.json is its hand-specified expected normalized object, NOT output from a proposed parser.
../dsl/testdata/frozen_level/policy.projection.json is the exact P above.
*.canonical.json contain literal canonical bytes without LF; DIGESTS.json records exact sizes and SHA-256s.

## Fixed semantic binding

The profile normatively binds frozen-level-primitives-v1 and frozen-level-lifecycle-v1 at the reviewed merged source commit 0ca22bf6493cd02e1f2adf13094981dfdcac148f. Added fixed fields clarify, not change, these meanings:
- Clock M30/M15 and M5 slots align to the UTC epoch millisecond grid, not broker civil midnight.
- Momentum and structural risk use rolling six completed M5 slots at the signal, independently of stored entry M30 slots. Close > first open permits long, < permits short, equality permits neither.
- Completed half-open source bars satisfy endMS <= knownAtMS <= decisionMS and full contiguous slot/source identity checks.
- Event timestamps and source sequences are integers 0..9007199254740991; order is lexicographically strict. For this profile availability equals event milliseconds and cannot regress.
- Activation is inclusive favorable movement on liquidation bid (long) or ask (short) from actual effective entry, divided by fixed initial R0. Commission is excluded from the risk-distance denominator. No-lock activation is inert.
- Camarilla uses the reviewed float64 order D=1.1*(H-L), C and ±D/12,/6,/4,/2. The fixed ladder order is S4,S3,S2,S1,C,R1,R2,R3,R4.
- Pivot raw eligibility is tested before snapping. Snap toward entry; then require strictly profitable/tightening and at least one grid of liquidation-side clearance. Equal snapped stops retain the first fixed-ladder item. Threshold comparisons use reviewed exact shortest-decimal rational values without epsilon.
- Ordering payloads must bind the same source, calendar and context-window identities, not merely contain a nonempty arbitrary proof string.

Retain the strict earlier-millisecond lock context cutoff. Before future quote execution, separately choose/register one common opportunity rule for equal-millisecond context/entries across ALL three managers. Do not repair timestamps, defer only selected arms, or silently relax locking. A valid compiled profile is still not an execution-complete candidate.


## Go interfaces and runtime refusal

`dsl.Parse` routes the reserved family through its full-source scanner. `NormalizeFrozenLevelConfig` validates a Go map; `DecodeFrozenLevelConfigJSON` is the lossless raw JSON boundary. Ordinary `json.Unmarshal` into `map[string]any` cannot preserve the numeric/duplicate-key contract and is intentionally rejected for integer fields. `FrozenLevelIdentity` returns canonical config/policy bytes and digests outside the config. `FrozenLevelProfile` returns an independent copy of the fixed profile.

Family selection audits all authored clauses using the legacy parser's logical-line expansion and normalization, with quoted data made opaque only for that audit. It also audits explicit selectors in setup bodies, including anonymous groups that legacy expansion might discard; balanced named list arguments remain values. Bare list and free metadata clauses use the actual inline clause boundaries, so they cannot hide a later selector. Quoted, compact, truncated or otherwise malformed frozen selectors reserve the family and fail the strict grammar instead of becoming a legacy default. Unterminated metadata cannot hide a later physical-line selector. Valid legacy metadata and symbols, including an instrument literally named `FROZEN`, retain their previous interpretation. The permissive legacy canonicalizer never adopts the new family; only the strict front-end can produce its config.

The production WASM exports validate original JavaScript UTF-16 code units before converting source to Go UTF-8. Unpaired surrogates return an input error without a config or trades; valid surrogate pairs and literal U+FFFD are preserved exactly. This validation applies to all source arguments, including comments, and does not interpret strategy syntax. Native commands read raw source bytes without JSON string decoding, so the strict family scanner can reject malformed UTF-8 directly. The fixture JSON `source` field is provenance, not executable DSL source.

Direct, shared, prepared, variant and prefix admission reject a frozen family discriminator or reserved root object before routing, empty data or timed-family dispatch. A valid root receives the stable unimplemented error; ambiguous roots receive that error with validation detail. The family is absent from `implementedFamily`. Fixture and column bridges also refuse execution. `ForceRoute` does not override admission.

A source-entry prepared handle snapshots the admitted top-level config map. Later changes to the caller's root cannot insert a reserved object or relabel that handle. Nested legacy values keep their existing semantics. Ordinary prepared paths derive state without retaining the caller root. New direct/variant requests still validate the current root.

## Qualification still required

Compilation validates reference structure, not external payload truth, access, freshness or authorization. The fixture payloads explicitly say `qualified=false`; they cannot authorize a job. Later source qualification must verify paired quotes, coverage, sequence/availability, finite broker calendar, operating schedule, costs and registered assumptions by exact payload bytes and identity. It must preserve unresolved gaps and reconcile the strict pre-entry context cutoff with one common opportunity rule across managers.

The later authoritative Go quote adapter must implement side-aware fills/exits, gaps, old-order precedence, timeout, all costs and consistent report units before native/WASM execution becomes available. Current release/app adoption and the known app corpus-materialization gaps remain separate gates. No JavaScript fallback or bar approximation is admitted by this checkpoint. Tranches, partial exits, additions and recovery sizing remain excluded.

## Verification

Synthetic fixtures cover hand-derived canonical bytes/hashes, strict raw decoding, complete syntax accounting, runtime refusal and retained-map behavior. `scripts/checks/frozen-level-compiler-parity.sh` executes the actual Go parser/decoder/admission tests under native and WASM and compares the canonical trace; the bridge parity script checks committed parse goldens and fixture/column refusal. Existing parse/run goldens must remain byte-identical.

Independent regression tests retain the exact-number, duplicate-key and all-block-order oracles and the original selector failures. The production bridge regression confirms that a malformed frozen selector cannot execute the existing five-trade legacy fixture. `scripts/checks/source-transport-parity.sh` exercises actual JavaScript calls, including malformed original UTF-16, valid Unicode controls and native malformed-byte input.
