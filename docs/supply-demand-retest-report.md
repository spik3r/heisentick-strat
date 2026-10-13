# Supply-demand entry-timeframe retest report v1

This additive, versioned companion runs the native Go S&D retest engine and
returns its original trades, native headline statistics and original-entry-bar
close-mark equity. It is mechanically qualified with invented inputs, not a
profitability, market-calendar, quote, liquidity, or broker-fill claim. See
[HT280](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-280-supply-demand-retest-reporting.md).
A release, consumer pin/adapter and UI qualification are separate gates.

## Entrypoints and closed request

- CLI: `heisentick supply-demand-retest-report --request-file=request.json --dsl-file=strategy.strat`.
  Exactly these two `--name=value` arguments are accepted. It reads bounded
  regular files and writes one complete success/refusal JSON line. A refusal is
  a JSON result; callers must check `error`, not just process exit status.
- WASM: `engineRunSupplyDemandRetestReport(requestJSON, dslSource)` returns one
  JSON string. Exactly two primitive strings are required; there is no object
  coercion. Invalid literal UTF-16, including lone surrogate arguments, refuses.
- Both use `report.RunSupplyDemandRetestReport`, backed by
  `engine.RunSupplyDemandRetestWithAccounting`; there is no JS stats adapter.

The request is a JSON object with **all and only** these fields:

| Field | Required value |
|---|---|
| `schema` | `strat-supply-demand-retest-request-v1` |
| `case`, `strategyId`, `symbol` | Nonempty UTF-8 labels, respectively at most 256, 128, 32 bytes; no control characters |
| `timeframe` | `15m`, `30m`, or `1h`; equals DSL `entryTf` |
| `sourceTimeframe` | `4h`; equals the DSL source route |
| `rangeMethod` | `zone` |
| `costs` | Closed object with all five fields below |
| `bars`, `sourceBars`, `sourceHtfBars` | Explicit arrays of six-number `[t,o,h,l,c,v]` rows |
| `higherTimeframe` | Empty string when HTF is off; otherwise `1d` |

There is no request `contractVersion`, context override, chart-HTF input,
calendar, precomputed signal, execution window, ForceRoute or terminal option.
Unknown/duplicate/missing keys, JSON null in required strings/arrays/numbers,
trailing JSON, invalid UTF-8 and escaped unpaired surrogates refuse.

`costs` requires `feePerUnit`, `slippage`, `slippageBps`, `startEquity`, `fillOn`.
The first three are finite/nonnegative, equity is finite/positive, and fill is
`close`, `open`, or `nextOpen`. These are supplied illustrative broker inputs;
no spread, financing, contract multiplier, account currency or realistic default
is inferred. The existing fixed-risk sizing and broker cost units are retained.

## Admitted DSL and data

The source must parse without errors, select supply demand, explicitly enable
`retest on entry timeframe`, and admit the exact requested symbol/entry route.
Off-route requests refuse rather than succeeding with misleading zero trades.
Enabled partial exits refuse. Other admitted management, including breakeven,
trailing and maximum hold, retains native meaning.

HTF modes are only `off` or `notAgainst` (DSL `higher timeframe must agree`).
Off requires an empty daily array. `notAgainst` requires at least two daily rows
and DSL HTF timeframe `auto`, `daily`, or `1d`. Strict modes and weekly context
refuse. Completed daily close versus open gives up/down/flat: flat permits both
sides, up long, down short; unavailable prevents a new signal.

Every nonempty series has at least two rows, strictly increasing open timestamps
on its declared UTC grid, positive-multiple deltas and **minimum delta equal to
the declared duration**. This makes the legacy duration inference fixed even for
short accepted prefixes. Timestamps are exact nonnegative decimal integers,
validated before conversion to binary64, and at most `2^53-1-durationMs`.
OHLC must be finite/coherent and have magnitude at most `1e8`; volume is finite
in `[0,1e12]`. The price cap protects the existing signed32-bit WASM zone key.
Gaps are supplied gaps: no padding, resampling or fabricated rows.

A missing completed 4h projection blocks new signals for that entry bar. Missing
or unavailable daily bias similarly blocks new HTF-gated signals. Existing
positions and pending orders still execute on supplied chart bars.

### Causal daily policy and retained limitation

The new policy identity is
`completed-daily-direction-decision-time-weekend-v1` (or `off`). It projects
completed daily direction to completed 4h decisions, then entry bars. When a
daily close is at least one day old, stale eligibility uses the existing
Saturday-overlap predicate only through the **current 4h decision**, never an
unclosed future daily timestamp. Appending or mutating future timestamps/OHLC
therefore cannot change an earlier admitted decision. Fixed duration/grid
admission is essential to that claim; prefixes without a known daily interval
refuse instead of guessing a duration.

This corrects a demonstrated legacy defect: adding an unclosed following-Monday
daily row could enable an earlier Thursday S&D trade. Long and short invented
signal fixtures preserve the legacy expectation and separately assert the new
corrected result. Other causal cases and same-policy capture-on/off trades match.
The ordinary generic APIs retain their prior HTF behavior and existing goldens;
this is a new semantic surface, not universal legacy trade parity.

The inherited reuse predicate accepts any interval overlapping Saturday UTC, or
at least seven days. It has **no maximum stale age** and is not a verified
provider holiday calendar. Separate maximum-age/calendar policy and generic
legacy migration remain open follow-ups.

## Bounded profile

- Request at most 64 MiB UTF-8; source at most 1 MiB; all supplied rows combined
  at most 500,000; output at most 64 MiB, with a conservative pre-serialization
  type/value bound and a second actual-length check.
- Explicit parsed numbers are finite with magnitude at most `1e6`; `riskUsd`
  is in `[0,1e12]`. Explicit nonfinite/overflow spellings refuse before legacy
  fallback parsing. Quoted labels and comments are not numeric parameters.
- Active count parameters must be exact integers before architecture-sized
  conversion: min/max base and impulse lengths `1..256`, min base <= max base;
  source length `0..256` (zero retains the existing flat-direction shortcut;
  a positive minimum-source ATR threshold still rejects that flat direction); CHOCH lookback
  `1..256` when enabled, otherwise `0..256`. Zone age, wait, reaction, retests,
  CHOCH lifetime, cooldown, max hold and entry expiry are `0..1e6`.
- Active distance, body, width, candle-quality, prior-day-range, invalidation,
  risk/target and management
  coefficients are nonnegative; stop minimum <= maximum. Absent internal
  unlimited thresholds stay absent; an explicit infinity is not admitted.
- Parsed config depth <=16, nodes <=2048, each map <=128 entries, each array
  <=64 elements and aggregate array elements <=128. Keys/strings <=256 UTF-8
  bytes except name/description <=1024. Non-JSON value types refuse.
- Active channel computation, channel targets, nonempty seasonality and any RMV
  declaration refuse. Dormant channel default metadata does not activate work.
  These constraints bound all effective list-driven/context loops.

The checked outcome-blind admission score is at most 500,000,000:

`S*(K*(B+2*I+512)+16384) + C*(16384+80*(L+64)) + D*512`

C/S/D are supplied entry/source/daily counts; B is maximum base length;
K=maxBase-minBase+1; I is impulse length; L is active CHOCH lookback or zero.
Checked unsigned arithmetic refuses overflow. The score normally binds before
the row cap. It is not an instruction count, wall-time guarantee or browser/RSS
promise. Capacity qualification must cover chart-heavy, source-heavy,
daily-heavy and maximal-loop settings, with full outputs and measured resources.

## Result and accounting

Success has `schema: strat-supply-demand-retest-result-v1`, `contractVersion: 1`,
`identity`, `run`, `metrics`, `terminal`, `capabilities`. Identity binds exact
request/source SHA256, case/strategy/symbol, every route/duration/count/first/last
time, HTF mode and policy, actual costs, profile
`supply-demand-retest-bounded-v1`, basis, terminal policy, limits and work score.
Empty daily series has count0, duration0, and null first/last timestamps.

`run` is the unchanged native RunResult, including its existing trade rounding.
`metrics.basis` is `supply-demand-retest-broker-close-mtm-v1`. The raw broker
entry debit, exit credit and close marks are reduced by the existing Go
accounting validator/reducer, not a sum of rounded `trade.pnl`. Account net is
raw `finalRealized`; trade net includes entry and exit fees; equity is starting
equity + observed realized + close-mark unrealized. Future exit fees/slippage
are not reserved in an open mark. Drawdown is anchored at starting equity and
uses contemporaneous peaks; it excludes intrabar adverse extremes.

Headline fields are start/end equity, net, returnPct, trades, winRate,
profitFactor, expectancy, avgWin, avgLoss, worstLoss, maxDD, maxDDpct,
maxWinStreak, maxLossStreak, avgHoldBars. Unavailable denominator-based statistics
remain null with the reducer's exact reasons: `no-trades`,
`no-winning-trades`, `no-nonwinning-trades`, or `no-losses`. With trades but no
positive or negative net, profit factor is zero; wins with zero gross loss give
null/`no-losses`. Flat trades count as non-wins in loss-streak and average-loss
denominators.
Trade accounting supplies index, entryFee, exitFee, netPnl. Equity has one point
per original entry bar, `{index,t,equity}`; there is no synthetic starting row.
Marks and legacy trade T fields use the original **bar-open timestamp** even
though execution/marking happens after that bar closes. Duration is explicit.

The unchanged terminal policy is `last-close-liquidation`. A held position
closes on the last supplied close using existing costs; the final mark is
replaced, leaving terminal position flat. `terminal` records policy, position,
liquidated, endOfTestTrades and actual pendingMarketOrders, pendingLimitOrders,
pendingDeferredExits observed **before** liquidation. An unfilled last-bar
next-open order is distinguishable from no signal. No cancellation or fill
cashflow is invented. `open` and `nextOpen` mean the next supplied row, including
across gaps, with unchanged stop/target gap handling.

Capabilities headline/trades/equity are true. Groupings, monthly, R distribution,
portfolio, live and paper are false. Consumers must use native values directly
and preserve null/reason conventions; ClockRange/JS statistic reconstruction is
not this contract.

Failure contains only schema, contractVersion and a bounded `error` with code,
optional field and barIndex, never identity/run/metrics/terminal/capabilities.
Codes include invalid-input, unsupported-profile, unsupported-route, work-limit,
invalid-derived-value, output-limit, invalid-arguments, input-unavailable and
internal-error. No partial success is serialized. Multiple simultaneous defects have no specified
diagnostic precedence; consumers must treat any refusal as failure rather than
require identical diagnostic field selection. Accounting corruption,
unknown metadata, nonfinite derived values, unsafe output shape and over-limit
encoded upper bounds fail closed.

## Qualification and candidate identity

Tests cover both sides, all three entry routes/fill aliases, raw cash and fees,
marks/final liquidation, pending orders, management, formation/flip whole-entry
intervals, source gaps, completed daily alignment, future source/daily invariance,
strict input/config/output refusal, and actual native/Node-WASM complete-byte
parity. Every previous conformance golden is preserved. The strict arithmetic
source/dependency inventory requires fresh compiler observation on linux-amd64,
linux-arm64 and js-wasm, with unchanged pinned instruction schedules; the
verifier and tolerances are not weakened.

The PR436 prototype defaults (daily HTF, breakeven disabled, risk100) are distinct
from the archived dispatch source (active0.75R breakeven, risk200). This report
admits a candidate-aligned runtime; it does not turn the synthetic fixture into
an authored production candidate, claim a faithful JS port, or qualify history.
A separately versioned research source, explicit instruction mapping, parsed
configuration assertions, release identity and consumer adoption are required.

### Cost encoding in the result

Both `identity.costs` and retained `run.costs` use the existing `Costs` encoding:
`feePerUnit`, `fillOn`, `slippage` and `startEquity` are always present;
`slippageBps` is omitted **only when its supplied value is zero**. A consumer
must interpret that specific omission as zero and require nonzero values to be
present. This is an output-encoding rule, not an input default: all five request
cost fields remain mandatory, and the exact original request hash binds their
explicit values. No other omitted cost field or inferred economic default is
admitted.
