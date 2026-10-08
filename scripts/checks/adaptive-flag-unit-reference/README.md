# Frozen reference field mapping and lossless comparison (B0)

This directory contains test-only declarations and a generic JSON comparator.
It contains no economic calculation, projector, strategy invocation, transport,
registry integration, or oracle runner. The retained Python reference and its
tests were read as source only. Neither was imported or executed for this work.

## Authority and immutable reference identities

The frozen HT195 Stage B contract-v2, its ORACLE_COMPARISON.md section 6.1, and
the explicit root B0-only release govern these files. Producer base:
`4575c7952b8f986beec584af14d215b66a54b9f3`.

The independently retained HT184 checkpoint4 files have these SHA-256 identities:

| Reference file | SHA-256 |
| --- | --- |
| unit_projection.py | 76c58c46f80217baf68fbe4071077b61134a78bde1b12b9ee3439a0de51eadce |
| research_common.py | 0bbfb21c5ab6a121215ba5ec250b3f2fcbabed3a0cf357387a93fb538877bba8 |
| test_unit_projection.py | 3bbd3d43331367535752866d4078a432f8e8a18b0ce8495154c40ff37b90f7fa |

The original retained source-tree identity is
`dc5ab170e90e604c18417dad91dd94a302cc1bab9c5b27bd4d9cd307b33306b9`.
These are retained reference identities, not an invented repository commit.
The three individual file hashes were verified again after the comparator tests.
No reference bytes were modified or copied by this mapping proposal.

## Declarative mapping

`field-mapping-v1.json` freezes closed reference and proposed Go projection
schemas, plus 348 explicit comparison rules. These cover all 326 declared
reference branches/leaves and all 326 declared Go branches/leaves. The separate
`manifestMapping` view lists the 113 manifest rules for provenance review. It is
an exact view of the manifest entries in `fields`, not a second policy.

All non-manifest economic names and structures match directly. Every order,
cost event, closed trade, summary, year map, mark, daily row, drawdown value,
terminal state, exposure, event-time bound, copied gap, and list item is covered.
Each array element and dynamic year-map member uses one explicit `*` member
placeholder. That means all members must match; it never excludes a subtree.
Arrays preserve order. Map keys must match. Ordinary JSON object property order
is immaterial to economic equality; separate transport byte parity remains
mandatory.

The four fill-only order fields form an all-or-none presence group:
`raw_entry`, `fill_risk_raw_U`, `display_proxy_adjusted_entry`, and
`fill_risk_effective_U`. They are absent before fill and present after fill.
Closed trades and a nonnull terminal open position require all four. A nullable
field must remain present, even when null. All other declared fields are
mandatory. Empty arrays/maps remain arrays/maps. Candidate registry output is
forbidden by the closed root schema.

The comparison profile intentionally narrows the baseline's otherwise open
metadata dictionaries to the B0-reviewed fixture shape:

- producer_identity contains exactly kind and compiler, retained as reference
  evidence rather than substituted for the actual Go build identity
- data_source contains exactly id and source_sha256; id admits the contract's
  ASCII 1..128 domain, including both the hand fixture and 288-case corpus IDs
- raw_envelope_file_identities contains exactly dslSha256, configSha256, and
  btb1Sha256 on the common Stage B runtime-envelope domain
- include_registry is fixed false

Although the Python baseline conditionally emits outer file identities for
standalone raw-run inputs, later Stage B comparison must provide the owned raw
runtime envelope with all three exact identities. Standalone raw-run comparison
is outside this profile. Extra metadata leaves fail closed until separately
reviewed and frozen; no arbitrary dictionary is silently ignored.

### Invented hand-ledger scope

The six frozen hand-ledger bodies are adapter-only internal full-projection
mapping inputs, including one explicitly expected invalid-risk refusal. They
are not public runtime-envelope inputs, native-fill evidence, or proof that the
strategy could generate their events. The two closed-side fixtures and the
invalid-risk-filled fixture deliberately retain the order reason
`synthetic-exit` and event basis `invented adapter fixture`. Those honest labels
must not be replaced with plausible native reasons to make admission pass.

Each hand body now carries the complete canonical INITIAL/M30 effectiveConfig:
policy, numericalPolicy, timeframe, bundle, and all 18 rule leaves. Static tests
validate every one of the six configurations through the unchanged token-aware
mapping, compare input values with source-read INITIAL literals, and reject the
previous incomplete object and deletion of each required config field/leaf.
This validates input metadata completeness without calculating any economics.

The separately frozen wrapper declares exactly three invented 5/6/7 hash
sentinels. Its byte composition wraps the unchanged raw body under `run`; the
wrapper is only an internal adapter comparison input. It cannot authenticate
DSL/config/BTB1 files, establish native raw provenance, or add a public raw-ledger
upload route. Successful later adapter comparison must be reported separately
from qualification of freshly generated native runtime envelopes.

The Go manifest keys bind the actual B0 `report/adaptiveflagunit/types.go`
declarations. Economic values within the manifest and equivalent provenance
leaves are individually compared across renamed keys. Changed schema names,
Python canonical-hash domains, retained CPython runtime fields, and new Go
build/policy/hash fields are explicitly enumerated per leaf as `evidence_only`.
Both values are retained in the receipt and each side is schema-checked. The
mapping never declares a Python runtime or canonical hash equal to a different
Go identity. Fixed reference explanations are constant-checked.

`schema_only` is restricted to closed, required, nonnullable object containers
whose descendants are all separately enumerated. It accommodates flattening
the three file-identity leaves and new provenance containers; it cannot hide
an array, dynamic map, nullable object, or any descendant. `validate_mapping`
rejects unmapped/duplicate paths, type drift, and subtree exclusions before
comparing any data.

Every native/WASM build-identity difference has an explicit per-leaf allowlist
entry. Those values still require observation against each actual executed
artifact/host. The comparator does not authenticate them. All other native/WASM
bytes must match. The complete original Stage A raw byte segment is compared
separately and exactly, including whitespace/newline; this mapping does not
parse/remarshal it or certify byte preservation.

## Generic comparator

`compare.py` uses both JSON numeric callbacks to keep each original number
token. It rejects duplicate decoded keys, malformed JSON, trailing documents,
non-JSON numeric constants, invalid UTF-8, and unpaired Unicode surrogates.

- float64 fields convert the original token directly to finite binary64 and
  compare all 64 bits, preserving signed zero, subnormals, and finite underflow
- integer fields require an integer token without a fraction or exponent,
  compare exact integer values, and enforce the declared range
- null, missing, false, zero, empty array, and empty object remain distinct
- unknown fields fail even if both sides contain the same unknown field
- mismatch receipts retain original tokens and binary64 bit hex where finite
- no rounding, tolerance, reassociation, economic calculation, or normalization
  is performed

`compare_values` also accepts independently computed Python expected values;
FLOAT leaves must be Python float and INTEGER leaves Python int. Actual numbers
must still be token-preserved JSON. Ordinary json.loads followed by float
conversion is deliberately unsupported because the sign of integer-looking -0
would already be lost.

Generic schema comparison:

```sh
python3 -I -S -B compare.py --schema schema.json --expected expected.json --actual actual.json
```

Mapped comparison, only after a separate explicit oracle-execution release
names the frozen input corpus and the outputs have been retained:

```sh
python3 -I -S -B compare.py --mapping field-mapping-v1.json --expected reference-projection.json --actual go-projection.json
```

The CLI compares files already supplied; it cannot invoke the oracle. A passing
receipt establishes declared-field comparison only. Provenance authentication,
raw byte preservation, actual host execution, cross-host payload byte parity,
aggregate E/G budgets, and lifecycle/business invariants are separate gates.
No comparison receipt alone claims a full Stage B or accounting PASS.

## B0 verification receipt

Checkpoint1 passed 16 generic tests. Checkpoint2 adds three static fixture tests
for all-six-config coverage, required-field rejection, and honest synthetic-exit
preservation. Run the complete 19-test suite with:

```sh
python3 -I -S -B scripts/checks/adaptive-flag-unit-reference/test_compare.py -v
```

The test suite covers signed-zero PASS/FAIL pairs, integer versus float token
typing, exact integers/ranges, subnormal/underflow/adjacent-value probes,
independent Python float bit fixtures, positive-zero cancellation-result bit
probes, missing/null/false/zero/empty distinctions, duplicate and unknown fields,
array order and length, map keys, fill-field presence groups, Unicode/numeric
syntax refusals, renamed leaf comparison, retained evidence, full mapping
coverage, duplicate mapping rejection, and subtree-exclusion rejection.

All 19 checkpoint2 tests passed on 2026-10-07 UTC. The focused receipt is retained separately as
`evidence/checkpoint2-comparator-tests.log`. The checkpoint1 artifacts and
receipts remain unchanged. The mapping and comparator implementation are also
unchanged by this correction.

Only these new generic tests ran. The immutable accounting oracle, its tests,
raw strategy runs, historical data, and transports were not executed. B0
independent review and a subsequent explicit root release remain prerequisites
for any accounting/lifecycle port or authorized oracle evaluation.
