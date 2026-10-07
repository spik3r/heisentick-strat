# Master portable binary64 contract v1

Task: [HT-183](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-183-master-structural-runtime-bridge.md).

`master-binary64-separated-v1` is an explicit numerical interpretation of the
fixed Master v10 policy. Select it with `master.RunPortableV1`, native
`master-portable-report --arithmetic-contract=master-binary64-separated-v1`, or
`engineRunMasterPortableReport` with the matching closed metadata schema.
There is one shared Go sequencing/state machine and no independent JS engine.
The [runtime contract](master-structural-runtime.md) defines transport, provenance,
resource limits, complete report fields and refusal routes.

## Why it is separately named

Go permits combining floating-point operations into a fused operation. The
[Go specification](https://go.dev/ref/spec#Floating_point_operators) states that
an explicit floating-point conversion rounds to the target precision, preventing
fusion that would discard that rounding. The portable helpers explicitly convert
the result of each add/subtract/multiply/divide to float64. Intermediate results
use binary64 round-to-nearest, ties-to-even; no implicit multiply-add contraction,
decimal truncation, price normalization, tick quantization or tolerance is added.
Signs of zero and subnormals are retained according to these operations.

The prior ordinary ARM64 investigation at
[PR95 run37582054148](https://github.com/spik3r/heisentick-strat/actions/runs/37582054148)
found numeric native/WASM differences in the fixed invented reports. A temporary
internal compiler experiment isolated contraction as the cause for that corpus.
The ordinary check remained failed. That experiment is diagnostic evidence only;
no compiler-debug setting is used in this implementation, CI qualification or
release builds. No universal or historical equivalence follows from those cases.

Existing `master-report`, `master.Run`, `regime.Run`, `ReferenceIndicators`,
`ReferenceBarIndicators` and `ReferenceBarrier` keep their legacy defaults.
Portable output has distinct envelope/run schemas and explicit arithmetic
provenance. Portable and legacy reports may differ numerically and at exact
comparison boundaries; callers must never relabel one as the other.

## Operation inventory

Operand order and grouping remain those of the reference expressions. Explicit
barriers cover aggregation volumes; WMA weight division/product/accumulation;
HMA `2*half-whole` and smoothing; Wilder initialization and recurrence; true-range
differences; Supertrend midpoint/ATR factor/bands; volume mean/ratio; HMA crossing;
`100*ATR14/close`; spread additions; equity-based sizing, epsilon, floor and step;
entry fees and cash; source anchor/current-ATR stops, targets and activation;
protected structural distance, frozen ATR brackets, activation and ratchets;
directional/marketability/diagnostic comparisons; barrier gap and nearer-extreme
distances; gross/net/quantity cashflow; summary/monthly accumulations, PF, drawdown,
open marks and hypothetical closing fees. Integer timestamps and counters do not
become floating-point arithmetic. HMA lengths remain floor-half and round-half-up
square-root, with the same fixed lengths and warm-up boundaries.

Existing `1e-12` sizing epsilon and `1e-9` diagnostic thresholds are preserved.
No new tolerance is used for exact gates, trade decisions or byte parity.

Every evaluated portable arithmetic operation checks finite operands and its
rounded result. An unexpected NaN/infinity stops the report immediately, even
if a later min/max, band carry-forward, readiness check, skipped RMA value,
nullable conversion or JSON encoding would hide it. Intentional uncomputed
warm-up slots are exempt until their defined readiness boundary; initialization
on finite inputs is checked before readiness. Underflow to a finite subnormal
or signed zero is permitted. Overflow avoided only by a hypothetical fused
operation is still an error under this separated contract.

The checked barrier kernel preserves stop-gap then target-gap chronology, and
only then intrabar touch/nearer-extreme logic. Strictly nearer high is high first;
equal distances remain low first. It does not pre-evaluate a future path after
an earlier return. This is a numerical kernel, not a separate executor.

## Bounds and reproducibility

The bounded adapter validates input size/layout and finite transmitted values,
then counts observed native/higher/trading buckets before decoding or running.
The shape proof bounds every result array/string/number, exact escaped config,
and a 4,096-byte fixed portable envelope allowance before either engine compact
JSON or final pretty JSON allocation. The post-serialization length check is
only defense in depth. Added arithmetic/provenance fields do not create new
unbounded collections. Frozen constructor/helper hashes require re-review when
those dependencies change; they are not a substitute for the cardinality proof.

`testsupport/testdata/master-portable-corpus-v1.json` stores 38 fixed invented
cases and 11 deduplicated gzip/base64 BTB1 assets. Decompression is size-bounded;
source/data/corpus hashes are checked. All targets consume identical bits and
fixed windows rather than generating a waveform with host math or selecting a
terminal window from the implementation under test. Reports are compared byte
for byte, including whitespace, provenance and final newline. A mismatch remains
a failure even when bounded diagnostics classify it as numeric only.

Qualification includes runtime-bit arithmetic vectors (with independently
specified expected bits and an explicit FMA contrast), warm-up/recurrence and
boundary tests, actual Go/WASM kernel tests, ordinary native/WASM reports, and
per-architecture legacy Master/v9 before/after bytes against producer commit
`38613eb8f4e8dd8afa5e68660d2f7b83937c64fb`, built with the same Go compiler.
Cross-builds alone are not execution evidence. AMD64 and ARM64 receipts must
identify the same frozen input identities; neither may normalize results.

This producer stage adds no Pine/broker parity, observed execution, financing
model, historical requalification, app pin, tag, deployment or orders. A future
release/adoption must separately disclose and pin this opt-in contract.
