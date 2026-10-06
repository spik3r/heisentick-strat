# Frozen quote-input admission

`engine/frozeninput.Decode(config, documents, artifacts)` admits bounded,
internally consistent input declarations for the compile-only frozen control.
It returns structural evidence. The family still refuses execution through all
existing native, WASM, prepared and prefix runtime routes.

The exact three schemas live in `spec/schemas/frozen-quote-{snapshot,source,ordering}-v1.schema.json`.
The hand-derived four-row vector is in `engine/frozeninput/testdata/`; all vector
files are invented. Its frozen design manifest deliberately retains the phrase
"design vector only": actual implementation tests now compare those independent
bytes, identities and projection with the constructor's output.

## Boundary and identities

The constructor accepts the original snapshot, source and ordering JSON bytes,
one declared raw quote artifact and any declared supporting evidence leaves.
It calls `NormalizeFrozenLevelConfig` on the received mutable map, then
`FrozenLevelIdentity` on that independent normalized copy. References, identities
and the budget use that copy. Previously validated but subsequently modified
configs reject. There is no constructor from caller-created DTOs or cached hashes.

Actual canonical-config, document and artifact byte lengths share an inclusive
1,048,576-byte budget. Each document also keeps the existing 1 MiB, depth-64,
1024-byte numeric token and absolute-exponent-10000 limits. Declared record or
byte counts never size an allocation. Artifact bytes may be binary. No provider
decoder, download, streaming adapter, registry or job is invoked.

Raw JSON rejects missing, unknown and duplicate decoded keys, null substitutes,
invalid UTF-8, unpaired surrogate escapes and coercion. Every integer is checked
exactly before conversion, in 0..9007199254740991; mathematical integer aliases
normalize to int64. Negative zero is forbidden. Original positive decimal Bid
must not exceed Ask, even if float64 would round the crossed pair to equality.
Only then do finite positive float64 values form the normalized snapshot. No
grid snapping occurs. A non-crossed sub-precision spread can normalize equal;
original bytes remain available, with precision and costs unqualified.

The local acyclic graph is raw leaves and raw snapshot → normalized snapshot →
source → ordering → compiled references. External hashes include all whitespace
and final newlines. The normalized snapshot is a fresh exact-key Go map, with
int64 integers, float64 prices, original strings and unchanged row order, encoded
by `encoding/json.Marshal` without indentation or final LF. No file embeds its
own resulting digest. Opaque artifact aliases are source declarations; actual
JSON document IDs and versions must agree with their full references.

## Order, intervals and evidence

Rows preserve strict `(eventMS, sequence)` order. Equal milliseconds with rising
sequences remain distinct paired quotes. Availability must equal event time for
this explicit reference assumption. Ordinals and locator indices both equal the
zero-based row index and identify the same raw artifact. Provider values have
explicit global-strict or per-event-millisecond counter policies. The latter
permits a reset only at a later event millisecond. No sort, repair or row drop is
available. Raw artifact hash/count checks establish consistency with the snapshot,
not the fidelity of the declared provider decoder.

Warm-up and evaluation are contiguous half-open intervals, with positive
evaluation width and optional empty warm-up. Every quote lies in their union.
Ordered coverage segments partition that union exactly. Complete-claimed needs
raw/coverage evidence; gaps need coverage evidence; closures need closure evidence.
Evidence must resolve and its declared scope must contain the cited interval.
Quotes inside gaps or closures reject. Unknown segments are explicit and may
contain quotes or none. Missing evidence rejects rather than silently downgrading.

Ordering binds the same source, snapshot, calendar, decoder, raw artifact, row
count and endpoint keys. Asserted ordering needs raw or ordering evidence covering
the full scope. Unqualified ordering can have no evidence. Context bindings can be
empty; nonempty bindings have unique window IDs, one of the four fixed kinds,
Bid side, matching source/calendar, a positive contained interval and explicitly
unqualified evidence state. Independent bar sources and inferred completion or
known-at ordering are outside this contract.

The primitive Source is exactly source provider, dataset, compiled source-ref
SHA-256 and Bid side. Every Quote retains Bid and Ask. OrderingProof is
`frozen-ordering:` plus the compiled ordering digest; it is an identity, not a
qualification certificate.

Successful Evidence has status `structurally-valid-input`. RunReady,
ContinuityVerified and SourceOrderVerified stay false. Coverage assertions and
declared sequence evidence state are preserved. Raw decoder fidelity, calendar,
prior opening, completed context, schedule and costs remain unqualified. No
lifecycle call, zero-trade run result or executable family registration is added.

## Ownership and verification

Input fields are private. Config, original/canonical bytes, artifact bytes, rows,
locators, evidence lists and nested context views are copied at retention or
access. Observations and Source return values. Failed construction returns nil;
nil/zero Input values cannot report successful admission, and invalid observation
indices return false. Concurrent mutation during a call remains an ordinary Go
caller data race; mutation after return cannot affect retained evidence.

Run `go test ./dsl ./engine/frozeninput` and
`bash scripts/checks/frozen-level-input-parity.sh`. The latter executes the same
Go tests natively and in Go/WASM, comparing literal projections, targeted refusal
messages, the independent rational price oracle and nested mutation counts.
Existing compiler/refusal, primitive/lifecycle, clock-range and 114/100 conformance
checks remain required. This checkpoint grants no release, app adoption, source
qualification or historical-run readiness.

The following checkpoint is one end-to-end invented common opportunity through
compiled config, admitted input, context/lifecycle, side-aware fills, all three
managers and a reconciled fee ledger, including an actual accepted pivot lock,
short mirror and gap/unresolved cases. Before fills, review the common policy when
the first admission-ready quote shares the frozen context millisecond. Selective
waiting or relaxing the strict pre-entry freeze rule is not implied here.
