# Setup family: range reversion

`range reversion` is a research-only native family for a close-back-inside
sweep of either a prior chart-timeframe channel or completed source-timeframe
extremes. The Go runner is invoked explicitly with
`heisentick range-reversion-report`. Ordinary reports, grids, generic engine
execution and generic browser routes reject the family. The separate
[fixed-rule R11 fixture/WASM bridge](../../docs/r11-wasm-fixture-v1.md) admits
only its two hash-pinned research sources and explicit two-stream input contract;
it does not make arbitrary range-reversion sources executable in generic routes.

The source is strict and complete. A canonical strategy has `strategy`,
`market` and `setup` blocks. It specifies the chart and source timeframe,
source availability policy, range basis and lookback, policy, every optional
gate's enable flag and parameters, ATR stop and target, cooldown, breakeven
trigger and offset in ticks, and the assumed tick size. Unknown directives,
duplicate directives, missing fields, invalid numbers and duplicate blocks
are errors. All effective fields appear in the config emitted by the parser.

Reserved family intent selects strict parsing before legacy defaults or later
`type:` declarations can replace it. Quoted, punctuation-separated or Unicode-
spaced spellings of the complete family name are refused, not accepted aliases.
Quoted metadata and named-list data remain opaque, and ordinary legacy uses of
`range` retain their existing meaning. Canonical grammar and fixed-rule source
hashes are unchanged.

```strat
dsl v7
strategy "Prior completed 4h range reversion" {
  description "Fade a sweep and close-back-inside of the previous 20 completed 4h bars"
}
market {
  rangereversion timeframe M30
  rangereversion source H4 availability next-native-row
}
setup {
  type: range reversion
  rangereversion policy IMMEDIATE_STOP_FIRST_V1
  rangereversion bounds source 20
  rangereversion candle-color false
  rangereversion htf-ema false 200
  rangereversion vector-gates false 14 30 48
  rangereversion range-expansion false 1.1
  rangereversion daily-chop 14 38.2 61.8
  rangereversion atr 14
  rangereversion stop-atr 0.6
  rangereversion target-r 2
  rangereversion cooldown 2
  rangereversion breakeven true 1 10
  rangereversion tick-size 0.01
}
```

## Rule contract

Chart bounds use the previous N lower-timeframe candles and exclude the signal
candle. Source bounds use the most recent N completed H4 candles, including the
latest one available at the lower-timeframe candle open. Source row `i` is
available only at the timestamp of source row `i+1`; a trailing source row with
no successor cannot be used. This `NEXT_NATIVE_ROW` rule handles source gaps
without inventing nominal closes.

A short signal needs a high above the upper bound and a close below it. A long
needs a low below the lower bound and a close above it. Candle-color, completed
H4 EMA(200), custom vector gates, range expansion and daily CHOP are independent
optional gates. The vector gate matches this strategy's custom raw-DM sums, DX
and Wilder RMA ADX calculation; it is not the platform's built-in ADX. CHOP
waits for a complete set of non-NA true ranges. The optional
`rangereversion daily-chop <period> <min> <max>` gate requires finite ordered
bounds (`min < max`) and admits values inside the inclusive range. Its daily
series is native D1 OHLCV on UTC-midnight timestamps. A row becomes available
at the next native daily row timestamp, and gate lookup uses availability no
later than the LTF signal close. Daily gaps, Sunday rows and flat weekend bars
remain in the sequence. The final daily row without a successor and CHOP
warmup rows reject only that new signal. Daily CHOP uses the native range
reversion convention where the first row's TR is missing without a prior
close; period 14 first warms on row 15. The earlier descriptive Daily
attribution used high-low TR for its first row and therefore warms one row
earlier. The gate is never applied to open positions or to a completed ledger.
Exact Pine preset values are in
`examples/range-reversion-pine-chart20.strat`; they use chart bounds over 20
bars, require candle color, enable the completed H4 EMA and custom vector/range
gates.

Signals arm a market order for the next observed lower-timeframe open. Stops
anchor to the signal extreme plus/minus ATR times `stop-atr`; target distance
is `target-r` times the signal-close-to-stop distance. A completed close that
reaches the configured R threshold queues a breakeven stop for the next bar;
the offset is `breakEvenOffsetTicks * tickSize` from the actual fill. There is
no time expiry. The cooldown counts observed lower-timeframe rows after an exit.

`DELAYED_PINE_OHLC_V1` models the supplied Pine cadence: no bracket on the
entry candle, a close-triggered stop latch when the first bracket is submitted
already through the stop, then a nearer-extreme OHLC path with an explicit
high-first tie. The exact Pine preset also requires chart bounds and candle
color. `IMMEDIATE_STOP_FIRST_V1` activates the bracket on the entry candle, uses
stop-first resolution when both barriers are touched, and discards a bar that
signals both directions before applying candle-color or trend gates. Actual
entry risk is the absolute fill-to-stop distance. It cancels only zero or
near-zero distances (at most `1e-12`) and reports that count under the stable
`canceledAtEntryNonpositiveRisk` field. A positive-distance fill already
through its stop or target is flattened at the raw open with both entry and
exit execution costs, and is labeled `stop-gap` or `target-gap`. This is the
source-series research policy used by B1/B20; it is not Pine parity.

## Inputs and output

`range-reversion-report` requires separate six-column BTB1 files for the entry
and H4 source series, and requires `--daily-bars-file` only when the daily CHOP
gate is enabled. The CLI rejects a daily file when the gate is absent. Daily
input is clipped strictly before `tradeTo` before indicators or decision joins.
The command also requires exact UTC evaluation bounds, adverse slippage per fill,
commission per unit per side, and a positive fixed unit size. Slippage affects
entry/exit prices and fill-relative breakeven. The result includes DSL and raw
BTB1 hashes, canonical consumed-series hashes, effective config, signal/trade
ledger, gross/net price-unit and planned-risk profit factors, nonpositive-risk
entry cancellation count, and censored/pending state. It does not infer point
value, account equity sizing, financing or broker fills.

The family is dedicated-runner only. Its parser warning is not a qualification
claim. Standard generic reports, grids and browser execution remain unsupported;
the separately versioned, source-pinned R11 fixture/WASM bridge above is the
bounded research exception.
