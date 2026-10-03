# Interactive run v1 (local producer contract)

The request uses the existing `dsl-conformance-run-fixture-v1` JSON plus the
DSL source as a separate string. The new `dsl-interactive-run-v1` response is
produced by `engine.RunInteractiveFixture`, native `enginewasm --interactive`,
and WASM `engineRunInteractiveFixture` from the **same Go source build**. It
does not change the `dsl-conformance-trades-v1` result or its goldens.

## Admitted route

Chart-timeframe `smaGoldenCross`, `dualEmaResumption`, and `namedLevelSweep`
with HTF mode off and no source or HTF bars are admitted. The ordinary
`sessionBreakHold` family is also admitted either with HTF mode off and no
HTF bars, or with `notAgainst`, a declared higher timeframe and nonempty HTF
bars. Its HTF decisions use the latest completed higher-timeframe bar; missing
intraday periods fail closed, while the existing scheduled weekend-closure
rule may reuse the last completed bar. The interactive boundary binds both timestamp series to
their declared fixed timeframes: ordered, grid-aligned rows, whole-period
gaps, and at least one adjacent period in each series. Ambiguous fully sparse
inputs and mislabeled HTF bars are rejected because the shared projection
currently infers duration from observed spacing. Every admitted strategy must
also pass its declared route.
No explicit source-timeframe or source-entry route is admitted yet.
Each input series has a 200,000-row local ceiling; the browser admission gate
may set a smaller per-route ceiling until its worker resource budget is measured.
Interactive execution records
evaluated Go entry-gate rejections with `skips: {}` (or positive counts),
`skipDiagnostics: "measured"`, and `skipReasonSchema: "dsl-skip-reasons-v1"`.
An empty map means no instrumented gate rejected a bar; it does not mean
every bar formed a setup. The existing fixture costs
(`fillOn`, slippage, basis-point slippage, fee per unit, positive start equity)
are supported. Route mismatch, unqualified HTF mode/input combinations,
source-timeframe input, and every other setup family are explicit
`unsupported-route` errors. The ordinary broker loop supplies
the marks. Scheduled source-entry, special-family and windowed loops are not
represented by this version. The recorded reasons cover instrumented market
gates, not every evaluated entry filter, missing setup, cooldown, or failed
geometry. Broader reason coverage remains a separate qualification gate.
Same-build native/WASM parity requires identical structure, route and input
metadata, skip schema and counts, trade count/order, trade decisions and fill fields,
categorical states, counts, final cash and errors. For Dual EMA and named-level
sweep, explicit derived-number paths may have cross-target floating-point
rounding: per-bar
marked and closed equity, trade `pnl`, and statistic `tradeNet`, `maxDD`, and
`maxClosedDD` have `1e-9` absolute budgets; trade `points` and the two drawdown
percentages have `1e-10`; trade `meta.signalAtr` has `1e-12`. All other values
must match exactly. The admitted 5000-bar Dual EMA fixture has only 13 marked
equity differences, at most `1.82e-12`; local browser 4h samples found bounded
drift in the listed fields for Dual EMA (22,180 differences, maximum `7.28e-12`)
and named-level sweep (28 differences, maximum `3.64e-12`). Compare decoded values under these explicit
rules and retain separate native/WASM response hashes, rather than claiming
byte identity across targets.

## Success envelope

The response contains `schema`, SHA-256 hashes of the exact raw fixture JSON
and source, `run` (the unchanged conformance trade envelope), one
`equityCurve` and `closedEquityCurve` value per input bar, post-liquidation
`cashEndEquity`, `skips`, `skipDiagnostics`, `skipReasonSchema`, and `stats`. The host must attach the verified
native/WASM artifact digest to the presented result; an input hash does not
identify an executable. A failure is `{schema,error:{code,message}}`; callers
must not turn it into a successful empty run.

## Skip reason counting

One count is the **first failed instrumented entry gate evaluated for one chart bar while
the strategy is flat**, before any possible order from that bar. The Go broker
short-circuits at that gate. Later gates on the same bar are not counted, and
the same bar contributes at most one count. Position-management bars, warm-up,
cooldown, unresolved levels, family-specific geometry, and failures of
uninstrumented seasonality, candle-quality, day-theme or distance checks are
outside this metric. A missing setup alone adds no count, though a market gate
may reject a bar before its setup is checked. `namedLevelSweep` evaluates the market gates
before its setup check on each flat bar; `smaGoldenCross` evaluates RMV only
after a long cross; `dualEmaResumption` currently has no evaluated market gate.
Thus maps from different families must not be read as equal-denominator
rejection rates. The code vocabulary and priority below are owned by Go, not
by legacy JavaScript display strings.

| Priority | Reason codes | Meaning |
| --- | --- | --- |
| 1 | `gate.utc_window`, `gate.session_window` | Bar outside the configured UTC or local session/segment trade window. |
| 2 | `gate.rmv_unavailable`, `gate.rmv_threshold` | Required relative measured volatility is missing or fails its comparison. |
| 3 | `gate.local_weekday_allow`, `gate.local_weekday_block`, `gate.local_hour_allow`, `gate.local_hour_block` | Local calendar allow/block checks, in that order. |
| 4 | `gate.open_location`, `gate.session_phase` | Market context allow checks. |
| 5 | `gate.prior_day_type_allow`, `gate.prior_day_type_block` | Completed prior-day type allow/block checks. An unavailable prior day fails an allow check. |
| 6 | `gate.day_regime`, `gate.movement_er` | Day regime and movement efficiency checks. |
| 7 | `gate.prior_day_range_missing`, `gate.prior_day_range_too_small` | Required prior-day range/ATR is unavailable or below the configured threshold. |

For the 4,504-bar Daily SND fixture, this priority produces 1,605 UTC-window,
1,649 prior-day-type and 13 movement-ER counts, totaling 3,267. The historical
JavaScript total is also 3,267 but its reason split differs because its gate
priority differs. Neither engine's counts are copied from the other.

At each bar, after fills, intrabar exits and strategy decisions,
`closedEquityCurve[i] = startEquity + realized`, including entry and exit fees
already paid. `equityCurve[i]` adds open-position P&L marked at that bar's
close. Both are recorded **before** optional final-data liquidation, as in
the legacy JS chart loop. `cashEndEquity` is the separately reported balance
after that liquidation, including its exit slippage and fee. Thus the last
chart mark can differ from final cash.

`stats.net = cashEndEquity - startEquity`; wins, losses, gross results,
streaks, averages and profit factor use each closed trade's P&L **minus its
allocated entry fee**. `tradeNet` preserves the sum of legacy trade P&L for
comparison. When gross loss is zero and gross win positive, profit factor is
`null` with `profitFactorState: "unbounded"`; zero activity uses numeric zero
and `"zero"`. `endEquity` equals final cash, not the final chart mark.

`tradeNetPnl[i]` is the Go-owned fee-inclusive outcome for `run.trades[i]`:
trade P&L after its allocated entry commission. It is an empty array when
there are no trades. Consumers may group these outcomes for display without
reimplementing fee allocation or changing the conformance trade record.
This field is required in the local v1 contract; producer and consumer must
be rebuilt together. Its cross-target derived-float tolerance is `1e-9`
absolute per outcome, matching the underlying trade P&L tolerance.

`maxDD` and `maxDDpct` are the independent maximum absolute and percentage
falls against the *running peak*, initialized at start equity, over every
marked bar **and final cash**. `maxClosedDD` and `maxClosedDDpct` apply the
same rule to the closed-equity curve and final cash. Percentage units are
0–100 percent, not fractions. These definitions intentionally differ from
the legacy JS `computeStats` when entry fees or changing peak denominators
matter. Historical results must keep their original engine and schema
identity.

The validation/report queue result in `heisentick-contracts` is a compact
asynchronous artifact reference, not this interactive chart response.
Producer-owned v1 lives here until a cross-repo consumer needs a published
generated contract; then add a distinct schema and follow that repository's
generation, fixtures, and release rules.
