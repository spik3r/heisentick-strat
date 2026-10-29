# Ordinary execution observations v1 — proposal only

This is a **contract design and synthetic reference-consumer test proposal**.
No parser, engine, CLI, WASM export or consumer currently negotiates, produces
or accepts this envelope. Passing these probes is not observed runtime evidence,
engine conformance, arithmetic preservation, release qualification or permission
to execute historical data. Existing ordinary v1 contracts stay unchanged.

## Inventory and ownership boundary

At producer `cde04bd8001424902f369e61d95f9a3fd16577bd`:

- `engine/ordinary_backtest_inspection.go` performs nonexecuting parse, config,
  route, options and data admission. Even dataful Inspect attaches no observer.
- `engine/ordinary_backtest_state.go` captures actual queue/position/financial
  boundaries. An unchanged before/after state does not identify a rejected gate.
- `engine/sequential_inspection.go` describes source capabilities, not execution.
- `diagnostics/diagnostics.go` derives report-level overfit warnings.
- `engine/frozencontrol/trace.go` is a separate test-only synthetic witness.
- The legacy app `scripts/strategy/strategyFunnel.mjs` performs a separate bar
  walk with an inert counting API after its JS backtest. Its coarse candidates
  are not observations from an authoritative Go execution. They cannot populate
  this envelope. Current `dslTry.mjs` explicitly refuses an unavailable Go funnel.

HT-200 retains ordinary admission, financial/state capture, bridges, transport
and adoption ownership. HT-328 retains source-coordinate/edit production work.
This proposal adds neither runtime hooks nor a second parser or financial reducer.

## Negotiation and identity

A future separately reviewed wrapper may explicitly request profile
`price-momentum-original-v1` and `exampleLimit` (0..16). It must negotiate that
version with an authenticated, qualified producer before the explicit run.
Do **not** add an observer option to strict ordinary-backtest-request-v1, add
fields to report-v1, infer support from a successful parse, or retry through a
broader/legacy endpoint. The exact wrapper/export/negotiation transport remains
unimplemented and needs a separate contract/ownership review.

The proposal is a sidecar to one successfully validated ordinary report, not a
new report or execution grant. The host captures a unique `invocationId`, exact
source/request/data bytes, expected Go config digest, trusted producer receipt,
requested profile/example limit and generation before asynchronous work. The
report digest is SHA-256 of the exact successful ordinary report UTF-8 bytes,
not reconstructed JSON. Validate that report using its existing operation,
identity, accounting and release rules first. Never accept self-described
producer fields as authority or adopt a release from the sidecar.

`binding` contains the invocation, source/request/data identities (SHA-256 plus
UTF-8 byte length), config digest, report digest, and producer identity. Compare
all against independently trusted capture/report/receipt values. Request identity
binds route, settings and costs; data identity binds the exact original-bar input.
The opaque invocation ID also distinguishes identical repeated requests. Profile,
family, bar count and requested example limit must match trusted context. The
sidecar's additional schema must be explicitly qualified; the existing producer
`schemaSetSha256` is not silently redefined to include it.

An absent sidecar is **unavailable**, not a complete zero. Existing admission,
parse, data and runtime failures retain their existing error envelopes; this
sidecar does not invent a successful report for a failed run.

## Proposed bounded coverage

Only the existing ordinary, original-chart-bar **Price Momentum** family is the
first proposed coverage profile. Preserve all existing admission refusals,
including dedicated, source/HTF-series, scheduled/windowed, unsupported accounting
and nonliquidating routes. A producer that cannot fully observe this profile
returns `unsupported` / `coverage-unavailable`, never zero-filled counters.
New families or a larger profile require their own lifecycle and preservation
qualification. Family names in unavailable envelopes identify the admitted
source family; they do not grant tracing support.

Observe the one authoritative invocation without replaying predicates or running
a second strategy. Do not relax filters, infer which unevaluated gates would
also fail, change decision order, predict profits on rejected opportunities or
offer automatic weakening. Attribute the first actually evaluated blocking
stage only. `rejected-entry-gates` is deliberately coarse: the compound
`enterSetup` condition short-circuits, and inner gate reasons remain unknown
until separately qualified hooks observe them. This is not a claim that every
inner gate was evaluated.

### Counters and conservation

All counters are exact nonnegative integers at most 10,000. Complete coverage
means all supplied original bars were observed, once per actual family-handler
visit, through the ordinary terminal lifecycle. Do not use bar-open position
state: pending fills and intrabar exits precede the family handler.

| Counter group | Meaning |
|---|---|
| `bars.visited` | Actual Price Momentum handler visits; equals `barCount`. |
| `bars.managingPosition` | Position present at handler entry; management path taken. |
| `bars.cooldown` | No position and existing cooldown rejected the visit. |
| `bars.signalUnavailable` | Direction input unavailable/invalid, including warmup. |
| `bars.noSignal` | Valid direction calculation with no directional threshold signal. |
| `bars.candidates` | Valid nonzero directional signal, one candidate per such visit. |
| `candidates.created` | Equals `bars.candidates`. |
| `candidates.rejectedSide` | Direction disallowed at the actual side check. |
| `candidates.rejectedATR` | ATR prerequisite unavailable after candidate creation. |
| `candidates.rejectedStop` | Actual stop construction/bounds check rejected. |
| `candidates.rejectedEntryGates` | Actual `enterSetup` refused; inner reason unobserved. |
| `candidates.acceptedOrders` | Actual `enterSetup` accepted an order. Not a fill/trade. |
| `orders.accepted` | Equals `candidates.acceptedOrders`. |
| `orders.filled` | Actual opening event associated with an accepted candidate. |
| `orders.pendingAtEnd` | Accepted order still pending at the terminal boundary. |

Required equalities for `complete`:

```
bars.visited = managingPosition + cooldown + signalUnavailable + noSignal + candidates
candidates.created = rejectedSide + rejectedATR + rejectedStop + rejectedEntryGates + acceptedOrders
orders.accepted = orders.filled + orders.pendingAtEnd
orders.pendingAtEnd <= 1
```

The proposed profile has no other terminal accepted-order disposition. If actual
producer paths admit another disposition, refuse coverage and revise/qualify the
profile; never force it into one of these buckets. Fill/order counts need actual
queue/open observations. Completed trades, PnL and a boolean entry attempt cannot
reconstruct them. A last-bar next-open order may be accepted and remain unfilled.

### Candidate identity and consumption

A PM candidate is `(signalIndex, side)` within one invocation, encoded exactly as
`pm:<decimal signalIndex>:<long|short>`. It is not a trade identity. Each rejection
in this profile has `consumed:false`: it does not advance PM entry cooldown.
An accepted order has `consumed:true`: PM's accepted-entry state advances, even
when the order is queued rather than filled. Subsequent fill/terminal-order
examples preserve that candidate identity. Managing-position/cooldown/signal-unavailable/no-signal
examples have null signal/candidate/side/consumption fields.

This rule is **not universal**. Classic flag calls void `enter` and then updates
cooldown even if broker gates refused. Extending PM semantics to that family
would produce false explanations.

## Status and failure isolation

| Status | Reason | Counters/examples |
|---|---|---|
| `complete` | null | Full measured counters and bounded examples. |
| `not-requested` | `not-requested` | Counters null; examples empty; omitted count 0. |
| `unsupported` | `coverage-unavailable` | Same unavailable representation. |
| `incomplete` | `observer-limit` or `observer-failed` | Same unavailable representation. |

Only an opted-in, matching PM profile can be complete. A not-requested response
must agree with the host's actual request. Unavailable statuses never render
zero counters or a ranked blocking gate. They may accompany a successful report
from the same unmodified run, but do not explain its outcome.

The proposed passive observer may stop recording on its own bounded capacity
error while normal execution continues within existing run budgets; publish
only an incomplete sidecar with null counters. Do not truncate execution or
alter financial capture to keep a diagnostic successful. Do not recover an
engine panic as an observer-only failure. Cancellation, supersession, ordinary
run failure or expiry means **no successful report/sidecar publication**; no
partial counters or late queued result can become current. A future observer
implementation must separately prove this failure-isolation policy.

## Examples, source references and bounds

There are eleven example kinds, listed in the schema's enum order. For each
positive terminal counter kind, retain its earliest actual event (bar index,
then event occurrence within that bar). Sort those representatives by bar index,
then enum rank; publish the first `exampleLimit` representatives. A repeated
kind is not a second example. `omittedExampleKinds` is the number of positive
kinds omitted by that limit, **not** omitted bars or truncated counter coverage.
A complete result with zero example budget may still have complete counters.
The consumer can check membership, order, counts and bounds, but aggregates
cannot prove earliest-event selection; future producer witness tests must do so.

Each example's bar index/time must match captured original bars. A rejected or
accepted-order example has `signalIndex == barIndex`; a filled-order example
has `signalIndex <= barIndex`; pending-at-end uses the last bar and that same signal index: an earlier
next-open order would have encountered its next supplied bar. A candidate ID
must agree exactly with signal and side. One signal index has one side and one
accepted/rejected disposition; examples cannot contradict one another. At most
one handler-outcome example may refer to a bar, while an order fill can occur
on a bar that also has its own handler outcome. A sampled candidate/order must
not use a signal bar whose sampled handler outcome says no candidate existed.
One sampled order cannot be both filled and pending. Distinct sampled candidate
and accepted-order identities cannot exceed their corresponding aggregate
counts. No accepted order sample may predate the sampled earliest accepted-order
representative, when that representative is present.

`sourceRange` is null unless the producer proves the exact authored source
mapping. Non-null ranges are half-open UTF-8 byte offsets into the captured
source, with `0 <= startByte <= endByte <= source byte length`; boundaries must
not split a UTF-8 code point or a CRLF pair. These are HT-328 conventions, not
new span-production or edit capability. Internal defaults/unmapped shared gates
remain null. Never derive ranges from prose, guessed source lines or a second
parser. Source/data contents, arbitrary messages and prices are absent.

The proposed sidecar cap is 65,536 UTF-8 bytes, depth 16, 8,192 JSON nodes and
16 serialized examples; the fixed profile retains at most eleven representatives.
Node counting matches ordinary transport: values plus object names, not array
indices. Counters never saturate/wrap; exhaustion makes observations incomplete.
No unbounded per-bar event log or allocations/goroutines per callback are allowed.
These are contract ceilings, not measured safe-device/RSS or real-time claims.
Existing ordinary input caps and 15-second phase / 45-second overall deadlines
remain unchanged and include observer work. Cancellation and source/settings/
data/release changes invalidate the generation immediately. Recheck monotonic
time and current invocation before decode and immediately before publication;
`now >= deadline` refuses even a success already queued. Physical worker
termination remains the existing supervisor obligation.

Strict decoding rejects duplicate decoded keys, unknown fields, invalid UTF-8,
unpaired surrogates, nonfinite numbers, unsafe/fractional integer lexemes, extra
JSON documents and resource overflows before semantic use. No coerced numbers,
case-folded keys or replacement Unicode. Schema shape is necessary, not sufficient:
trusted identity, status, counter conservation, source/bar/example relationships
and host lifecycle checks are mandatory.

## Qualification sequence — still future work

The accompanying fixtures are invented JSON reference-consumer probes. Their
source/config/request/report bytes and producer identities are explicitly fake;
no engine is invoked to produce observations. They cannot establish causality,
producer earliest-example fidelity, interruption latency or arithmetic parity.

Before instrumentation: coordinate exact engine/shared-path ownership, choose
passive hook locations without recomputation/reordering, and obtain independent
contract review. Then require invented-bar producer cases for every observed
stage, no-candidate and missing-prerequisite distinctions, immediate/next-open
acceptance, fill timing consistent with same-bar versus next-supplied-bar mode,
terminal pending orders, cancellation, bounded output and failure.
Prove off/on trades, accounting float bits, order/position lifecycle, results and
serialization on each actually qualified target. Preserve existing arithmetic
schedules, compiler-source inventories, fingerprints and independent proof gates;
new input closures need their separately reviewed amendments. Run full Go tests,
vet, conformance and actual native/public-WASM qualification. Cross-compiling
ARM64 is not native ARM64 execution.

Only a separately qualified producer release may feed a validation artifact
contract and pinned consumer rendering. Consumers distinguish source/admission
facts from actual execution observations, preserve staleness/explicit-only Run,
and must pass protocol, renderer and real-browser lifecycle tests. The current
null ordinary app receipt remains null. No release, pin, market run, broker
operation, deployment or HT-200 completion is granted here.
