# Ordinary backtest v1 contract

This is a bounded producer contract, not a release receipt or execution
activation. It defines future inspect/report behavior and the admission surface;
it does not establish observer coverage, runtime qualification, browser readiness,
or complete Backtester statistics coverage. `engineRunFixture` and all existing
runner, financial, dedicated, paper and checkpoint contracts remain unchanged.
There is no JavaScript execution or financial-reducer fallback.

The initial Go helpers provide strict immutable transport capture and preliminary
parse/profile/original-bar checks. `PrepareOrdinaryBacktestAdmission` always
returns `Preflight() == "not-evaluated"`; it does not call every existing run
validator or prove observer coverage. Its data-validation fact is not a route or
execution admission. Only the later assembled inspect/report surface may report
`admitted`, after sharing all run validators and qualifying raw accounting.

## Schema entry points

The four self-contained JSON Schema 2020-12 documents use stable versioned URNs:

| File under `spec/schemas/` | `$id` | Root |
|---|---|---|
| `ordinary-backtest-request-v1.schema.json` | `urn:heisentick:strat:ordinary-backtest-request:v1` | `request` |
| `ordinary-backtest-inspection-v1.schema.json` | `urn:heisentick:strat:ordinary-backtest-inspection:v1` | `inspectResponse`: inspection or inspect error |
| `ordinary-backtest-report-v1.schema.json` | `urn:heisentick:strat:ordinary-backtest-report:v1` | `runResponse`: report or run error |
| `ordinary-backtest-error-v1.schema.json` | `urn:heisentick:strat:ordinary-backtest-error:v1` | Error inventory: inspect error or run error |

Select the response schema from the operation being called **before** examining
its result. Never use an inventory union to choose an operation. Schemas enforce
shape and fixed metric labels; strict decoding, byte identity, admission,
financial formulas and lifecycle checks below are additional requirements.

## Transport and immutable identity

The proposed operations are `engineInspectBacktestV1(requestJSON, source)` and
`engineRunBacktestReportV1(requestJSON, source)`. Each accepts exactly two
primitive strings, in that order, and returns one complete JSON success or typed
producer error. The chosen operation fixes `operation` to `inspect` or `run`;
the request must agree. Inspection never executes a strategy.

Validation order is: argument arity/types; raw source/request Unicode and byte
bounds; duplicate-detecting JSON decoding; complete structural validation;
immutable source/request/data/producer capture; parsing and Go config identity;
shared deterministic admission; execution; projection/serialization. Never trust
a request ID extracted from a partially invalid request. Specific transport
budget/data errors precede the generic schema error. A panic becomes a
stage-correct error and the one-shot worker is discarded.

Reject invalid UTF-8, raw unpaired UTF-16 and escaped lone surrogates in every
JSON value and key. Valid escaped surrogate pairs are accepted. Detect duplicate
keys after unescaping, including escaped spellings of the same key. Reject
trailing documents, unknown fields, nonfinite/exponent-overflow numbers and all
coercion, replacement decoding or normalization. Before browser capture, reject
accessors/prototypes. Decode native output with the same strict bounded decoder.

Both `requestId` and `strategyId` are exactly 1..128 ASCII characters: first
`[A-Za-z0-9]`, remainder `[A-Za-z0-9._:-]`. They are case-sensitive; whitespace,
newlines, normalization and truncation are forbidden. A request ID is unique in
its active controller generation. Strategy ID is the caller-run namespace,
separate from a saved-storage ID; the host may deliberately keep it stable for
corresponding comparisons. Neither ID grants execution or release authority.

Each `identity` is SHA-256 plus its exact UTF-8 byte count:

- `binding.source`: exact second argument, with no source rewriting.
- `binding.request`: exact first argument, including all route/options/data.
- `binding.data`: exact raw JSON subdocument embedded once as `data`; Go retains
  and hashes those same raw bytes. If request data is null, this binding is null.

Do not hash reconstructed or canonicalized JSON. The browser independently
hashes immutable captured bytes and compares every receipt. Each bound result's
request ID, strategy ID, route and effective execution settings must equal the
current capture. In a report, `nativeRun.case` is exactly the request ID and
`nativeRun.strategyId` is the captured strategy ID; internal agreement between
result fields is insufficient. Inspect/run request hashes differ because their
operation/request ID differ; bind each to its own capture and compare their
source, data and effective settings separately.

`configSha256` is a Go-owned identity of the successfully parsed configuration,
not a browser-invented or client-authenticated authority. Preserve its actual
digest once computed. A parse-rejected inspection has null config identity and
null source facts. The producer tuple must equal the build-owned receipt:
repository, immutable tag, source commit/tree, `js/wasm` target, Go version and
schema-set digest. Activation additionally requires independently verified WASM
and official shim digests/sizes and API versions. A fetched manifest or embedded
stamp cannot authenticate itself. No caller value can replace the release
receipt; absent/mismatched release identity fails before data acquisition.

## Shared admission and readiness

All request fields are explicit: `rangeMethod` is `zone|pivot`; `params` is
exactly `{}`; `forceRoute` is false; `chartType` is `candles`; reporting basis is
`ordinary-logical-completed-close-mtm-v1`. Costs require positive finite
`startEquity`, nonnegative finite `feePerUnit`, `slippage`, `slippageBps`, and
`fillOn` exactly `close|open|nextOpen`. Preserve supplied fill spelling and native
restrictions. There is no implicit default or ignored override.

The bounded input profile is `original-chart-bars-v1`, with one original OHLCV
series and source timeframe equal to chart timeframe. Source/HTF companion
series, transformed bars, callbacks, precomputed signals, runtime/broker/paper
state and checkpoints are outside this profile. Route support and declared
source route are Go admission checks, not inferred from display labels.

`data` may be null for inspection; run requires 1..10,000 rows of
`strat-original-bars-v1`. Each row is exactly `[t, open, high, low, close, volume]`
with finite values, nonnegative volume, coherent OHLC (`high >= max(open, close,
low)` and `low <= min(open, close, high)`), and strictly increasing nonnegative
safe-integer millisecond timestamps. `firstT` and `lastT` equal the actual
endpoints. Go validates complete-bar closure against its supported timeframe
and `closedThroughT`. Origin identity/generation and closure provenance are
caller-declared, not exchange/calendar authentication. Execute the entire
captured finite dataset: no sorting, deduplication, dropped forming/malformed
bars, hidden warm-up, truncation or tail fallback. Indicators warm up on that
supplied beginning.

Reserved dedicated intent is checked before zero-data/off-route shortcuts:
Sequential, Clock/R11/timed, Master, adaptive, gold, regime, frozen inputs and
dedicated range-reversion, including aliases and hidden nested object keys.
Preserve existing parser/config and `engine/runner.go` range-reversion guards;
nested-map detection does not claim arbitrary array traversal. A compatible
dedicated adapter keeps its exact contract/guards. There is no ordinary
source-hash allowlist and no fallback through a less strict export. Missing raw
accounting coverage is `ACCOUNTING_UNAVAILABLE`, not execution permission.

Inspection separates parse, data and admission: absent data means
`data=absent`, `preflight=not-evaluated`, `execution=not-run`. `preflight=admitted`
requires accepted parse, validated data, ordinary classification, supported
accounting and every shared run validator passing. Dedicated classification
names its known producer contract and cannot be ordinary-admitted. Run reparses
and revalidates independently; an inspection receipt does not authorize it.
Preflight cannot guarantee runtime completion or device capacity.

## Producer errors

Every error contains `operation`, `requestId`, `binding`, `error` and
`diagnostics`; null never means an omitted key. The operation always comes from
the called export, even for invalid arguments. Codes below carry the
`BACKTEST_` prefix.

| Stage | Allowed code suffixes | Request ID / binding | Config identity |
|---|---|---|---|
| transport | ARGUMENTS, SOURCE_ENCODING, JSON_ENCODING, DUPLICATE_KEY, UNKNOWN_FIELD, SCHEMA, INPUT_LIMIT, DATA_REQUIRED, DATA_LIMIT, INTERNAL | both null | no binding |
| parse | PARSE_REJECTED, INTERNAL | exact captured ID / complete binding | null |
| admission | ROUTE_UNSUPPORTED, OPTIONS_UNSUPPORTED, DATA_INVALID, FORMING_BAR, DEDICATED_REQUIRED, ACCOUNTING_UNAVAILABLE, INTERNAL | exact captured ID / complete binding | actual digest |
| execution, run only | INTERNAL | exact captured ID / complete binding | actual digest |
| report, run | OUTPUT_LIMIT, NONFINITE_OUTPUT, INTERNAL | exact captured ID / complete binding | actual digest |
| report, inspect | OUTPUT_LIMIT, NONFINITE_OUTPUT, INTERNAL | exact captured ID / complete binding | null only if parsing/config hashing never succeeded; otherwise actual digest |

`DATA_REQUIRED` is missing/null run data; `DATA_LIMIT` is an over-limit row
array; other byte/depth/node ceilings use `INPUT_LIMIT`. Invalid syntax/Unicode
uses `JSON_ENCODING`. Inspect cannot have an execution-stage error. If a
parse-rejected inspection fails serialization, its report-stage error retains
null config identity while preserving every other captured binding. Never
fabricate a digest or discard a computed digest to fit an error branch. Parse
stage ends when successful parsing/config hashing completes. Unknown code/stage
pairs, incomplete bindings, cross-operation responses and wrong current
identities are invalid results. Allowed unbound transport refusals remain valid
even for a browser-captured request; later stages cannot be unbound.

Host release/cancellation/timeout/worker/invalid-result failures use the separate
`GO_BACKTEST_*` namespace, never fabricated producer errors or empty success.
Safe diagnostics must not log raw source, bars, private headers or arbitrary
stacks.

## Raw capture, finalization and arithmetic

Only producer Go computes report financial values. A partial-aware observer
must copy actual broker before/after mutations, entry fees, exit credits,
position/order state and original-close marks on every admitted runner path.
Existing dedicated collectors do not prove ordinary coverage. A missing capture
or nonfinite raw/projected value refuses the whole report. Never reconstruct
accounting from rounded trade rows or ending-minus-starting equity.

`f64(expr)` below denotes an explicit binary64 observer/projection rounding
boundary. It is not an instruction to change existing broker expressions or
force cross-target operation schedules. Existing financial operations preserve
their accepted target-dependent schedule. Same-target capture-off/on execution
must preserve decisions, fills, costs and native serialization exactly.
Cross-target accounting differences require independent replay of each pinned
target's actual arithmetic with exact predicted result bits; there is no new
tolerance or blanket byte-equality promise. Unknown schedules and unexplained
differences fail. The existing [target arithmetic qualification contract](sequential-metrics-contract.md#owner-approved-exact-target-arithmetic-relation--2026-10-10)
describes the scoped precedent; ordinary coverage requires its own evidence.

Observer projections use finite shortest-round-trip JSON, normalizing signed
zero only at output. Native trades preserve existing 15-significant-digit
serialization. Raw signed-zero evidence must not be lost during qualification.
Native `trade.pnl` is an exit credit excluding the original entry debit. Raw
`equity[]` financial fields are plain native-account-unit numbers under the
fixed original-close running-peak basis; they are not tagged summary metrics.

There is exactly one retained mark per original input row. Preserve existing
ordinary final liquidation; replace its last mark after liquidation, then
compute drawdown from the finalized curve. Discarded provisional marks cannot
leave maxima behind. Do not import paper/other dedicated retained-open policies
or invent fills for pending last-bar orders. Negative finite equity and
percentage drawdowns above 100 remain representable.

Let `S` be exact required start equity, `R` actual terminal broker realized,
`Rc` broker realized observed immediately after the most recent full flatten
(or zero before any completion), and `U` terminal original-close unrealized.
Partial exits do not update `Rc`; final flatten copies the actual realized-after
snapshot. When ordinary finalization is flat, `R == Rc` is capture identity;
neither must bitwise equal a separately regrouped logical-position net.

## Fixed summary metrics

The schema fixes each field's unit, basis, status and allowed unavailable reason.
All account fields, completion counts/net and drawdown scalars are available on
every successful report, including no-trade reports. Zero is a value, not
absence. Unless stated otherwise below, money uses `native-account-unit`.

| Field | Exact formula / reduction | Unit when different | Fixed basis |
|---|---|---|---|
| `account.startEquity` | `S` | | `explicit-start-equity-v1` |
| `account.completedNetPnl` | `Rc`; never summed logical nets or equity subtraction | | `broker-last-flat-completed-v1` |
| `account.completedEndEquity` | `f64(S + Rc)` | | `broker-last-flat-completed-v1` |
| `account.completedReturnPct` | `f64(f64(Rc / S) * 100)` | percent | `broker-last-flat-completed-v1` |
| `account.realizedCashPnl` | `R`, including actual entry debits and partial/final exit credits | | `broker-realized-terminal-v1` |
| `account.markedEndEquity` | `f64(f64(S + R) + U)` | | `broker-close-mtm-terminal-v1` |
| `account.markedNetPnl` | `f64(R + U)` | | `broker-close-mtm-terminal-v1` |
| `account.markedReturnPct` | `f64(f64(markedNetPnl / S) * 100)` | percent | `broker-close-mtm-terminal-v1` |
| `account.openUnrealizedPnl` | Flat: 0; otherwise `f64(f64(f64(close - entry) * side) * actualRemainingSize)`, side +/-1, no hypothetical exit costs | | `original-close-open-position-v1` |
| `account.openEntryFeePaid` | Original observed nominal entry fee of the current open logical position; otherwise 0; no partial allocation | | `actual-open-entry-fee-v1` |
| `account.entryFeesPaid` | Start 0; in entry event order, `fee=f64(feePerUnit * actualEntrySize)`, `total=f64(total + fee)` | | `nominal-entry-fees-event-order-v1` |
| `account.exitFeesPaid` | Start 0; in partial/final exit event order, `fee=f64(feePerUnit * actualClosedSize)`, `total=f64(total + fee)` | | `nominal-exit-fees-event-order-v1` |
| `completedPositions.count` | Number of fully flattened logical positions | count | `logical-position-completed-v1` |
| `completedPositions.wins` | Number of completed logical nets strictly > 0 | count | `logical-position-completed-v1` |
| `completedPositions.losses` | Number of completed logical nets strictly < 0 | count | `logical-position-completed-v1` |
| `completedPositions.breakeven` | Number of completed logical nets exactly 0; signed zero is zero | count | `logical-position-completed-v1` |
| `completedPositions.netPnl` | Per position, `X=0`; `X=f64(X + actualRawExitCredit)` in exit order; on flatten `n=f64(X - originalEntryFee)`. Aggregate starts 0; `Net=f64(Net + n)` in completion order | | `logical-position-net-reduction-v1` |
| `completedPositions.winRate` | If count > 0: `f64(wins / count)`; else unavailable / `no-trades` | ratio | `logical-position-completed-v1` |
| `completedPositions.profitFactor` | In completion order, `W=0`, `G=0`; for `n>0`, `W=f64(W+n)`; for `n<0`, `G=f64(G-n)`. No completions: unavailable / `no-trades`; else `G=0`: unavailable / `no-losses`; otherwise `f64(W/G)` (0 for loser-only) | ratio | `logical-position-completed-v1` |
| `completedPositions.expectancy` | Always unavailable / `not-in-contract`; no value | | `ordinary-statistic-not-in-contract-v1` |
| `completedPositions.averageWin` | Always unavailable / `not-in-contract`; no value | | `ordinary-statistic-not-in-contract-v1` |
| `completedPositions.averageLoss` | Always unavailable / `not-in-contract`; no value | | `ordinary-statistic-not-in-contract-v1` |
| `completedPositions.worstLoss` | Always unavailable / `not-in-contract`; no value | | `ordinary-statistic-not-in-contract-v1` |
| `drawdown.amount` | Peak starts `S`; scan finalized marks in index order: `peak=max(previousPeak,E)`, `dd=f64(peak-E)`; maximum starts 0, updates only on strict greater-than | | `post-execution-original-close-running-peak-v1` |
| `drawdown.percent` | At the same mark/peak: `pct=f64(f64(dd/peak)*100)`; independent maximum starts 0, updates only on strict greater-than | percent | `post-execution-original-close-running-peak-v1` |

`amountAt` and `percentAt` retain the earliest index/time of their own strictly
positive maximum; ties keep the earlier point and a zero maximum has exactly
null location. Their maxima may occur at different marks. Locator and native
trade index/time references must match captured rows. The browser checks shape,
identity, fixed labels and availability consistency, never peak/metric math.

The four unavailable statistics require a separately defined Go contract and
independent acceptance before complete statistics migration can be claimed.
Seven analyses are also always unavailable / `not-in-contract`: `groupings`,
`rDistribution`, `monthlyReturns`, `costSweep`, `portfolioFit`, `replayMetrics`
and `persistedSummary`. No legacy reducer may fill these gaps.

A report's bar count/endpoints and equity length match input. Native trade count
equals its array length. Native case, strategy ID, route, range method and costs
match captures; no dedicated extensions are admitted. Preserve native
`slippageBps,omitempty`: explicit request/binding zero means native omission;
positive request Bps means the same positive native value. Present native zero
is noncanonical. Other native costs equal captures exactly. Native metadata is
bounded producer JSON, never executable configuration.

## Provisional bounds and host obligations

These are qualification ceilings, not measured safe-device or hard-real-time
claims. Use a lower producer/device bound when applicable; raising or affirming
capacity requires independent measurement.

- Source: 262,144 UTF-8 bytes; request plus source: 2,097,152 bytes.
- Text-only inspection request: 65,536 bytes plus separately bounded source.
- Data: 1..10,000 rows; result: 8,388,608 UTF-8 bytes.
- Request and result each: container depth at most 32, JSON nodes at most
  500,000. Root is one node; each value and object member name adds one; array
  indices/punctuation do not. Root container depth is 1; each nested container
  adds one; primitives do not. Reject depth 33 or node 500,001 during scanning.
  Conservative byte/code-unit prechecks may refuse earlier.
- Run overall: 45,000 ms from immutable capture start. Capture/acquisition,
  inspect and run/report each: 15,000 ms including preparation and validation.
  Text-only inspect: 15,000 ms from capture start. Use the smaller remaining
  phase/overall budget; it never resets at worker creation. Artifact fetch,
  verification/startup, Go, decoding and result validation belong in the phase.

Acquisition accepts AbortSignal, absolute monotonic deadline and explicit
byte/row caps; checks header count before allocating rows; limits Fetch-exposed
body bytes and materialized decoded data independently to 2,097,152 each; and
supports stream cancellation. The final request/source cap still applies.
Unsupported application-level compression/container formats refuse before
decoding. No unbounded full-history materialization followed by a late size
check. Fetch compression is not an app-measured wire-byte or RSS bound. Refuse
before using a transport that cannot guarantee bounded cancellable acquisition.

One controller owns capture, acquisition, one-shot worker and settlement. Check
current generation/identities, AbortSignal and supervisor monotonic time after
every await and before subsequent work. Source/route/settings/data/release
changes, navigation, disposal, supersession or cancellation invalidate the
generation immediately; abort fetch, cancel/release readers safely, ignore late
bounded hash results, terminate workers and remove listeners exactly once.
Do not wait for stalled reads/hashes before cancellation.

A separate supervisor timer requests termination while Go is synchronous. Also
check `performance.now()` before expensive result decoding and immediately
before publication after validation: `now >= min(phaseDeadline, overallDeadline)`
means timeout even when a success message was queued first. Stale generation
cannot publish success. Throttling or a blocked supervisor can delay physical
termination; no observed late result may be accepted. No retries through a
broader contract and no success inferred from inspect, schema checks or
synthetic examples. Release, native/WASM arithmetic, observer coverage and
browser lifecycle qualification remain separate gates.
