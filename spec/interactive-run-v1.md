# Interactive run v1 (local producer contract)

The request uses the existing `dsl-conformance-run-fixture-v1` JSON plus the
DSL source as a separate string. The new `dsl-interactive-run-v1` response is
produced by `engine.RunInteractiveFixture`, native `enginewasm --interactive`,
and WASM `engineRunInteractiveFixture` from the **same Go source build**. It
does not change the `dsl-conformance-trades-v1` result or its goldens.

## Admitted route

Only a chart-timeframe `smaGoldenCross` DSL strategy on its declared route,
with no source or HTF bar inputs, is admitted. The existing fixture costs
(`fillOn`, slippage, basis-point slippage, fee per unit, positive start equity)
are supported. Route mismatch, source/HTF input, and every other setup family
are explicit `unsupported-route` errors. The ordinary broker loop supplies
the marks. Scheduled source-entry, special-family and windowed loops are not
represented by this version. SMA Golden Cross does not emit skip reasons, so
`skips` is exactly an empty object for this admitted route.

## Success envelope

The response contains `schema`, SHA-256 hashes of the exact raw fixture JSON
and source, `run` (the unchanged conformance trade envelope), one
`equityCurve` and `closedEquityCurve` value per input bar, post-liquidation
`cashEndEquity`, `skips`, and `stats`. The host must attach the verified
native/WASM artifact digest to the presented result; an input hash does not
identify an executable. A failure is `{schema,error:{code,message}}`; callers
must not turn it into a successful empty run.

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
