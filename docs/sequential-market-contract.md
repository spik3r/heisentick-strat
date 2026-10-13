# Sequential E1/E2 market companion v2

Producer contract for running caller-authored Sequential Full E1/E2 sources on
supported symbols and timeframes, with trades, marked equity and statistics from
the one Go execution. It is additive. The v1 export
`engineRunSequentialFixture`, its schema `strat-sequential-backtest-result-v1`,
its four pinned sources and `docs/sequential-metrics-contract.md` are unchanged.

This is a producer qualification surface. It is not a release, a historical
study, a consumer activation or an owner-approved real-market calendar. Claim:
[HT-228, 2026-10-13 continuation](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-228-full-sequential-go-dsl-engine.md).

## What was limiting v1, and what v2 changes

| Limit in v1 | Kind | v2 |
| --- | --- | --- |
| Four exact source SHA-256 values | Fixture pin | Any authored Full E1/E2 source. The source's own route and policy are used. |
| Full E1/E2 only on `SYNTH` / `5m` (`validateSequentialAccountingFixture`) | Fixture pin | `symbol` and `timeframe` come from the source. The fixture must match. Seven timeframes below. |
| Start equity omitted or 0 becomes 10000 | Generic normalization | Not applied. `costs.startEquity` must be given and positive. |
| Timestamp step other than one timeframe is refused (`unsupported-sequential-time-gap`) | Fail-closed refusal bound by the corpus | Unchanged. See the decision sheet. |
| A held position on the last bar refuses the whole run (`unsupported-incomplete-terminal-run`) | Fail-closed refusal bound by the corpus | Unchanged. See the decision sheet. |
| Core resets on `data_gap` | Engine capability | Unreachable through admitted input, because admission refuses first. Unchanged. |

"Fail-closed refusal bound by the corpus" means the implementation coordinator
recorded the restriction in the contract (section 7, "D4 E1/E2 synthetic
execution acceptance", which says such restrictions "defer undefined inputs;
they do not settle new trading policies") and the independent execution corpus
(`engine/testdata/sequential_execution_e1_e2_v1`, convention 8 and the
`refusals` cases) binds the refusal. It is not an owner-approved semantic. The corpus even binds
`e1.refuse_missing_interval_after_exit`: a hole after the trade has closed is
still refused. These two refusals are not removed here.

## Entry points

| Surface | Call |
| --- | --- |
| WASM | `engineRunSequentialMarket(fixtureJSON, source)` and `engineAdmitSequentialMarket(fixtureJSON, source)`, two strings in, one JSON string out |
| Native | `enginewasm --sequential-market run fixture.json source.strat` and `... admit ...` |

Both return exactly one envelope and never throw. Every failure, including argument,
size and file-read errors, uses the envelope of the operation that was selected, so an
`admit` reply always fits the admission schema. Native exits 0 for a success
envelope (`run`) or a runnable admission (`admit`), 1 for a refusal, 2 for bad
arguments. Wrong argument types in WASM, including a BigInt, return a typed
`arguments` refusal and leave the instance usable.

`admit` answers "would this exact request run" by executing it and discarding the
result, so `runnable` is true exactly when `run` succeeds and a refusal carries
the same error. A held terminal position is known only after execution, so
admission cannot be cheaper than a run. It also reports bar-spacing gaps
(`timeline`) so the app can show where a series is not contiguous. Each listed gap has
`index`, `previousOpenMs`, `openMs`, `deltaMs` and `onGrid`. `missingBars` is present
only when `deltaMs` is a whole number of steps; an off-grid interval gets no
missing-bar count, because none is defined and none is invented by rounding.
At most 32 gaps are listed; `gapCount` is complete.

Schemas: `spec/schemas/sequential-market-result-v2.schema.json` and
`spec/schemas/sequential-market-admission-v2.schema.json`. Metrics, trade
envelope, costs and capabilities reuse the v1 definitions by reference.

## Request

The fixture is the existing original six-column run fixture
(`dsl-conformance-run-fixture-v1`), decoded by the same strict transport as v1:
duplicate keys, unknown fields, fractional or non-integral timestamps, nulls,
trailing values, source/HTF projections and calendars are refused. Limits: 64 MiB
fixture, 8 MiB UTF-8 source, 1 to 500,000 bars.

Required:

- `symbol`, `timeframe` equal to the source's `sequential symbol` and `sequential timeframe`.
- `strategyId`: any non-empty label. It is echoed, not used to select behavior.
- `rangeMethod: "zone"`, `case`, `schema`.
- `costs.feePerUnit`, `costs.fillOn` (`open` or `nextOpen`), `costs.slippage`,
  `costs.startEquity` (> 0). `costs.slippageBps` is optional and non-negative.
- `bars`: `[openMs, open, high, low, close, volume]`, finite, `low <= open, close <= high`,
  volume >= 0, `openMs` an integer below 2^53 and strictly increasing.

The source must parse to `setup { type: sequential full ... }` with
`sequential policy E1` or `E2`, an uppercase `sequential symbol` matching
`^[A-Z][A-Z0-9_.-]{0,31}$`, one `sequential timeframe`, and explicit
`sequential riskUsd` and `sequential maxNotionalUsd`. Nothing is defaulted.

| Timeframe | Step (ms) |
| --- | --- |
| 1m | 60000 |
| 5m | 300000 |
| 15m | 900000 |
| 30m | 1800000 |
| 1h | 3600000 |
| 4h | 14400000 |
| 1d | 86400000 |

The route is not tied to an instrument list. Fees and slippage are in price
units per unit; a caller must supply values that suit the instrument. The
engine knows no pip size, contract multiplier or currency conversion: quantity
is `riskUsd / |fill - stop|` in raw price units, as in v1.

Not supported, always refused: Legacy Setup-9 sources (B0/B0S keep their v1
route), P1, any other family, `timedCalendar`, source/higher-timeframe series,
`forceRoute`, execution windows, close fills, unknown cost or top-level fields.

## Execution semantics (unchanged from v1 and the accepted corpus)

Decision at the close of bar `d`; fill at the open of `d + 1` plus adverse
slippage. Stop is the Setup (E2: Setup to Countdown-13) extreme less
`0.10 * ATR14(d)`; target is `2R` from the slipped fill; `R = |fill - stop|`.
A fill on or beyond the stop is rejected (`stop_breached_at_entry`). Quantity is
`min(riskUsd / R, maxNotionalUsd / |fill|)`, fractional, with a strict
`capBinds` flag. Stop first if one bar touches both. Time exit at the open of
bar `e + 4` (E1) or `e + 12` (E2), before that bar's stop and target checks. One
position at a time; later same-side or opposite decisions are rejected as
`blocked_in_position` or `simultaneous_signal`. Costs: one adverse slippage
offset on every fill, entry commission debited at entry, exit commission on exit.
ATR14 needs 14 bars. These are the accepted conventions and nothing here
reinterprets them. "Bars" always means observed bars in the supplied array.

Marked equity has one point per input bar: start equity plus realized P&L plus
open P&L at that bar's close from the actual slipped entry. No hypothetical exit
fee or slippage is charged to an open mark. `maxDD` and `maxDDpct` use the
close-bar mark basis `sequential-broker-close-mtm-v1`, defined in the v1 contract.

## Success envelope

`strat-sequential-market-result-v2`, `contractVersion` 2:

- `identity`: request and source SHA-256, `strategyId`, `route`
  (`symbol`, `timeframe`, `timeframeMs`, `profile`, `policy`, `riskUsd`,
  `maxNotionalUsd`), `barCount`, first and last bar open, effective `costs`.
- `run`: the unchanged trade envelope with the `sequentialFull` audit of every
  decision (filled, rejected with reason, or `no_next_bar`).
- `metrics`: the v1 projection (headline, per-trade fees and net, equity).
  Unavailable statistics are `null` with a reason field; never silently zero.
- `terminal`: `position` is always `"flat"`; `pendingOrderRejected` is true if a
  decision on the last bar had no next open.
- `capabilities`: headline, trades and equity true; groupings, monthly,
  R-distribution and portfolio false. Consumers must not recompute them in JS.
- `limits`: `contiguousBarsOnly: true`, `closureCalendar: false`,
  `terminalOpenPosition: false`, `maxBars: 500000`.

A short window is not flagged. A series shorter than the ATR14 and Setup warm-up (a decision is possible only from the 14th bar for E1 and the 26th for E2) succeeds with zero trades and a `no-trades` reason; the app should show the bar count beside it.

A zero-trade success is a real result: statistics are null with `no-trades`,
equity is constant. It is never used to mean "unsupported".

## Refusals

A refusal envelope has only `schema`, `contractVersion` and `error`
(`code`, `message`, optional `field`, `barIndex`, `opportunityId`). It contains no
identity, run, metrics, terminal, capabilities or limits. `admit` classifies:

- `disposition: "unsupported"`: the engine deliberately does not support this
  input yet. The app should say so and not offer a retry.
- `disposition: "invalid"`: the input is malformed or contradicts itself.

| `error.code` | `field` | disposition | Meaning |
| --- | --- | --- | --- |
| `unsupported-sequential-time-gap` | `series.timestamp` | unsupported | A bar opens more than one step after the previous bar (spacing below one step is `invalid`, field `bars.timestamp`). Closure, holiday, outage and missing data are not told apart. `barIndex` is the first bar after the hole. |
| `unsupported-incomplete-terminal-run` | `held position` | unsupported | The last bar ends with an open position. Nothing is closed or marked on the app's behalf. |
| `unsupported_config` | `source`, `costs.*`, or a top-level key | unsupported | Unsupported policy, symbol form, timeframe, fill mode, negative cost, or unknown field. |
| `unsupported-sequential-route` | `timedCalendar`, ... | unsupported | A calendar, source series or route override was supplied. |
| `invalid-sequential-accounting` | `source.family` | unsupported | Source is not a Full E1/E2 source (Legacy, generic). |
| `invalid-sequential-accounting` | `route`, `costs.startEquity`, `bars`, `bars.timestamp`, ... | invalid | Contradiction or malformed value. |
| `invalid-sequential-input` | `$...`, `bars.*` | invalid | Transport defects: duplicate key, fractional timestamp, null cell, bad JSON. |
| `invalid-sequential-derived-value` | `fill`, `size`, ... | invalid | A derived price or quantity is not finite. |
| `SEQUENTIAL_DSL_INVALID` | `source` | invalid | Empty, oversize or non-UTF-8 source. |
| `SEQUENTIAL_FIXTURE_INVALID` | `fixture`, `schema`, `costs` | invalid | Missing required key, wrong schema, oversize. |

## Terminal open positions and pending orders

What v2 does today, and what each choice would do to the outputs:

| Item | v2 today | Recommended extension (not implemented, needs a decision) |
| --- | --- | --- |
| Position open after the last bar | Whole run refused. No trades, no equity. | Return the run with `terminal.position: "open"` and the open trade's side, entry index, fill, quantity, stop, target, bars held and unrealized P&L. Do not close it. |
| Marked equity | n/a (refused) | Last equity point includes unrealized P&L at the last close, with no hypothetical exit cost. `endEquity` is then marked equity, not `startEquity + net`, and is labelled so. |
| Completed-trade statistics | n/a | Win rate, profit factor, expectancy, averages, streaks, worst loss, average hold: completed trades only, labelled so. The open trade is in none of them. |
| Entry fee of the open trade | n/a | Already debited in the broker's cash, so it is inside marked equity. Report it on the open trade. `headline.net` stays the sum of completed trades. |
| Pending order (decision on the last bar) | Valid run. Audit row `no_next_bar`, no trade, `terminal.pendingOrderRejected: true`. | Unchanged. The order is never filled at a price the series does not contain. |
| Drawdown | Close-bar mark basis, all bars. | Same, including the open mark. |

## Decision sheet

Settled and implemented: route and policy from the authored source; explicit
costs and start equity; contiguous observed bars; accepted E1/E2 conventions;
v1 untouched. Open, with the written status of each. A chat summary was not
treated as approval anywhere below.

### D-A. Closures and unexpected gaps

- Question: when a series has holes, which holes may the engine continue over,
  and what happens at an unexpected one?
- Written status: `plans/sequential-semantics-contract.md` 4.1 supports only
  series with no declared closure until a calendar exists. Section 7, "D4 E1/E2
  synthetic execution acceptance", records the fail-closed restriction: missing
  intervals and calendar declarations return typed unsupported-input errors, and
  the core's unexpected-gap reset "does not by itself decide an open execution
  position's treatment or ATR seed across missing intervals". D6 says the full
  profile "gets a calendar-aware proposal developed with HT-231" and that it
  "replaces the 4.1 refuse-all rule once the proposal is reviewed". The
  proposal (`plans/sequential-cross-market-v1/protocol.md` section 8) is a draft;
  its decision-sheet Q4 is unanswered. The execution corpus binds refusal of
  every hole (`e1.refuse_missing_interval_*`, `e2.refuse_missing_interval_in_countdown`).
- Recommendation: the caller supplies an explicit, versioned, hashed list of
  declared closure intervals (UTC ms start and end) in the request. The engine
  accepts a hole only if one declared interval covers it exactly, and keeps all
  state across it. Any other hole stays a typed refusal (today's behavior). The
  engine never infers a calendar from missing candles. Resetting at an
  unexpected gap (section 8 of the draft) would be a separate opt-in, because it
  changes what a "run" means for statistics.
- Affected: the four corpus refusal cases for holes (they would split into
  declared-closure successes and undeclared refusals); every FX, metals and
  index series with a weekend or daily break; ATR and Setup continuity below.
- Source: contract 4.1, section 7 D6, HT-231 protocol section 8, decision sheet Q4.

### D-B. Observed-bar counting across a declared closure

- Question: are the Setup comparison (`close[t-4]`), ATR14, the E1 four-bar
  response window, the hold limit (4 or 12) and warm-up counted in observed
  bars, with a closure contributing no bars?
- Written status: the contract is index-based ("compare by array index"), and
  D6 says state survives a declared closure with comparisons on either side.
  No text says the hold limit counts observed bars across a closure.
- Recommendation: yes, observed bars everywhere. A weekend does not age a
  position or an order. Equity gets one mark per observed bar, none for the
  closure.
- Affected: any declared-closure case with a position held across a closure.
- Source: contract 4.1 and D6; HT-231 protocol section 8.

### D-C. Fill after a closure ("reopened next open")

- Question: may a decision on the last bar before a closure fill at the first
  observed open after it?
- Written status: unwritten. The accepted rule is "next open", and the corpus
  tests only 5-minute price gaps with no timestamp hole (`e1.gap_up_entry`).
- Recommendation: yes, at the first observed open plus the usual adverse
  slippage, with the existing wrong-side rejection against the actual fill. An
  undeclared gap anywhere, including one that would strand a pending decision,
  refuses the whole run as in D-A; nothing is dropped or filled across it. Gap risk on an open position (stop at the reopened
  open) already follows the accepted `min(open, stop)` rule.
- Affected: cases with a decision on the last bar before a closure; stop exits
  at reopening.
- Source: corpus convention 4 and 6; D6.

### D-D. Terminal open position reporting

- Question: when a window ends with a position open, refuse, or report it open
  without closing it?
- Written status: refuse (corpus convention 8, `e1.refuse_open_at_end_*`; contract
  section 7, "D4 E1/E2 synthetic execution acceptance": "no implicit final-close
  liquidation and no successful zero-trade substitute"). The same section says a
  "separate bounded decision must specify missing-interval/open-exposure
  handling, ATR continuity, and endpoint/open-position reporting" before real
  backtester adoption. HT-228 lists open-exposure endpoint integration as gated.
- Recommendation: report open, never close. Exact behavior is the right-hand
  column of the terminal table above. Never force an end-of-test exit: the broker
  has no such exit for E1/E2 and a forced exit would add a trade the strategy
  did not take.
- Affected: the five `refuse_open_at_end_*` corpus cases; any Backtester window
  that ends mid-trade, which is most of them at a user-chosen end date.
- Source: contract section 7 (acceptance record); HT-228 status block;
  `docs/sequential-metrics-contract.md` ("A held Full position at the terminal bar
  refuses the whole result").

### D-E. Study defaults

Not approved and not applied: `$50` risk, `$200,000` notional cap, `$10,000`
equity. The v2 route takes risk and cap from the authored source and start equity
from the request. The examples use invented numbers (risk 90, cap 1,000,000,
equity 10,000) only so the corpus arithmetic stays checkable.

## Consumer handoff

Supported source identifiers: none are pinned. `strategyId` is a free label and does not select behavior, so a v2 run may echo any string; do not read meaning into it. A source is supported if it is a
Full E1 or E2 source as described under Request. The v1 strategy ids
(`dslSequentialFullE1`, `dslSequentialFullE2`) and their four pinned hashes still
work only on the v1 export. Identify a v2 run by `identity.dslSha256`,
`identity.route` and `identity.fixtureSha256`.

Supported symbols and timeframes: any symbol matching the pattern above with
1m, 5m, 15m, 30m, 1h, 4h or 1d, on contiguous bars only. The engine does not
know an instrument list. The Backtester must supply bars that are contiguous in
the chosen timeframe. Real FX, metals and index data usually are not: expect an
`unsupported-sequential-time-gap` refusal until D-A is decided. Also expect
`unsupported-incomplete-terminal-run` whenever a window ends in a trade until D-D
is decided.

Runnable invented examples with expected outcomes are in
`engine/testdata/sequential_market_v1/` (`EXAMPLES.json` indexes six: target
exit with fees, stop exit with drawdown, an E2 trade, a pending order on the last
bar, a held position at the end, and a 48-bar weekend hole). Their fixture bytes
go to the export unchanged. They are not market data.

Refusal behavior the app must implement: call `admit` first or handle the `run`
refusal; show `error.code`, `error.field` and `error.barIndex`; use
`disposition` to choose the message; never render a refusal as zero trades; do
not fall back to JavaScript execution or statistics. The inspection export
(`engineInspectSequentialSource`) predates v2 and still reports
`sourceAccounting: unsupported` and `marketDataReadiness: unknown`. Do not use it
to decide runnability. Use `admit`.

Tests the consumer should run when adopting:

1. `GOOS=js GOARCH=wasm go test -exec=... -run '^TestSequentialMarket' ./engine ./cmd/enginewasm`
   against the pinned producer source, or the equivalent in the producer's CI.
2. In the app's own tests: for each example in `EXAMPLES.json`, call both exports
   with the published WASM and compare to the expectation in the index. Check that
   a refusal renders its code and bar index, that no refusal shows trades, and
   that a zero-trade success renders the `no-trades` reason.
3. Validate every envelope against the two v2 schemas.
4. Check `identity.fixtureSha256` and `identity.dslSha256` against the bytes sent.
5. Confirm the v1 export still behaves as before for the existing synthetic route.

## Evidence in this slice

- `engine/sequential_market_corpus_test.go`: every binding case of the accepted
  execution corpus through the v2 entry point (both sides), then 18 cases on seven
  route/timeframe/price-scale combinations with and without fees. Expected
  marked equity, drawdown, fees and statistics are recomputed from the corpus's
  expected trades, not read from the engine. Relative tolerance 1e-9, no
  absolute floor; indices, counts, strings and null reasons exact.
- `engine/sequential_market_causality_test.go`: prefix causality on seeded
  random walks, bit-exact.
- `cmd/enginewasm/sequential_market_bridge_test.go`: hand-derived numbers
  (fees, equity points, drawdown), refusals, admission/run agreement, v1 unchanged.
- `scripts/checks/sequential-market-parity.mjs`: native CLI against Node-hosted
  WASM exports, schema validation, relative tolerance 1e-12 on every non-index number (prices, quantities, money); indices, timestamps, counts and strings exact.

Remaining limits: no closure calendar, no gap handling, no terminal-open
reporting, no E1/E2 prefix or checkpoint API, no P1, no Legacy route, no
historical or holdout run, no browser check, no release or tag. Cross-target
arithmetic uses the pinned schedules in
`scripts/checks/sequential-arithmetic-schedules.json`; this slice changes no
pinned arithmetic function, only the source inventory.
