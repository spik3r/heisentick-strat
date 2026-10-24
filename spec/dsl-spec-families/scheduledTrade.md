# Scheduled trade

`scheduled trade` is a native Go-only execution family for one fixed-clock
entry and exit per settlement date. It supports one explicit FX slice at 1m or
5m, UTC clocks, a fixed one-unit order, and a pip stop. The reviewed FX pip map
is USD/JPY, EUR/JPY, GBP/JPY, AUD/JPY, NZD/JPY, CAD/JPY, CHF/JPY at 0.01, and
EUR/USD, GBP/USD, AUD/USD, NZD/USD, USD/CAD, USD/CHF at 0.0001.

```dsl
dsl v7
strategy "Tokyo settlement short"
market conditions {
  slices(USDJPY 5m)
}
setup {
  type: scheduled trade
  scheduled timezone UTC
  scheduled calendar japan-settlement
  scheduled entry 00:55
  scheduled exit 01:30
  scheduled side short
  scheduled stop 20 pips
  scheduled quantity 1 unit
}
```

Settlement candidates fall on the 5th, 10th, 15th, 20th, 25th and last
calendar day of each month. Weekend dates roll back to Friday; a Japanese
national holiday or the Jan 1–3 / Dec 31 closures then exclude the date without
another roll. The embedded official Cabinet Office holiday list covers
2021-01-01 through 2027-12-31. Runs whose bars fall outside that span fail;
the engine does not infer holiday dates for unpublished years. The source CSV
is [published by the Cabinet Office](https://www8.cao.go.jp/chosei/shukujitsu/syukujitsu.csv);
`scripts/update-japan-settlement-holidays.mjs` checks or regenerates the
embedded dates from a downloaded copy while preserving earlier years. Before
extending coverage, compare the target year's CSV rows with the Cabinet Office
holiday page, then add the reviewed full-file SHA-256 to
`REVIEWED_CSV_SHA256` in the tool. The tool accepts only a pinned source hash;
the hash records which bytes were reviewed but does not itself establish that
the official list is complete. It rejects unreviewed coverage years and never
estimates unpublished future holidays. Cabinet Office says 2028 dates will be
published in February 2027.

For this renewal, save the CSV and run
`node scripts/update-japan-settlement-holidays.mjs --csv FILE --through-year 2027 --check`.
After reviewing a newly published year against the Cabinet Office page, add
the full CSV's SHA-256 under that coverage year in `REVIEWED_CSV_SHA256` before
extending coverage.

Entry requires an observed bar at the exact scheduled open and a contiguous
preceding bar. There is no catch-up entry. The stop is anchored to the actual
slipped entry fill. A stop crossed by a later opening gap fills at that worse
open; otherwise an intrabar stop fills at its stop price. At the first observed
open at or after the exit clock, the time exit takes precedence over that
bar's later high/low, after checking an opening gap through the stop. A run
ending before a stop or time exit records an `open-terminal` audit row and
open-position detail; it does not fabricate a liquidation trade.

The fixed fee is debited on both entry and exit for this family. `Trade.Points`
is gross price movement and `Trade.PnL` is that movement less two
`feePerUnit` debits. Slippage uses the shared broker model. PnL is in quote
currency for one fixed unit, not a USD account-equity return.

The family is not supported by prepared, prefix, report, shared-grid, transfer,
source/HTF or execution-window paths. Use the checked native `engine.Run`,
`RunFixtureCase`, `engineRunFixture` bridge, or the released CLI command:

```sh
heisentick scheduled-trade-fixture \
  --fixture-path=conformance/run/family-scheduled-trade.fixture.json \
  --strategy-path=conformance/run/family-scheduled-trade.strat
```
