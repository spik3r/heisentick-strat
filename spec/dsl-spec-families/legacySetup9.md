# Setup family: `legacy setup 9` (`legacySetup9`)

Status: specified. Go only (HT-227). Not compiled by the JavaScript runtime.

This family ports two archived strategies as frozen, named profiles so they
can be run again as historical controls. It is not evidence of an edge and it
is not the full DeMark Sequential method. Each profile ID names a fixed rule
set. A corrected variant is a new profile ID; these profiles never change.

```dsl
dsl v7
strategy "Legacy Setup 9 control" {
  description "Raw TD Setup 9 reversal, archived behaviour."
}
market conditions { slices(XAUUSD 1h) }
setup {
  type: legacy setup 9
  sequential profile seq.legacy.setup9.v1
}
```

## Profiles

| Profile ID | Archived source | Signal |
| --- | --- | --- |
| `seq.legacy.setup9.v1` | `dslTDSequential` | Stored Setup count equals 9 |
| `seq.legacy.setup9_perf_seasonal.v1` | `dslTDSeasonalReversal` | Count 9, immediate inclusive perfection, and the causal next-hour seasonal bias agrees with the side |

The source is pinned to heisentick commit
`48a1761867342494c69c77e7ecce325e10d5d935`. `sequential profile <id>` is
required, may appear once, and must follow `type:`. Unknown IDs are errors.

## Closed rule set

The profile fixes every strategy value. These are the only accepted lines:
`dsl`, `strategy`/`description`, `slices`, `type`, `sequential profile`, and
`riskUsd` (finite, positive; default 200). Any other directive, including
`side`, `stop`, `target`, `trade window`, `wait`, `maxHoldCandles`,
`breakeven` and `seasonality`, is a parse error. Nothing is ignored.

- Setup counter: at index `i >= 4`, `close[i] > close[i-4]` adds to a sell
  count (a negative count becomes +1), `close[i] < close[i-4]` adds to a buy
  count (a positive count becomes -1), equal closes reset it to 0. The
  comparison is by array index, so it spans weekends and missing bars. There
  is no price-flip precondition: a run starts on the first bar after any down
  or equal bar, and sides can flip directly.
- Stored count: the strategy reads an 8-bit signed value. The internal count
  never wraps; only the stored value does (modulo 256, range -128 to 127).
  This is a preserved defect. On 300 strictly rising bars the stored value is
  9 at index 12, -9 at index 250 and 9 at index 268, so the profile signals a
  short, a long and a short there. A stored 127 at index 130 becomes -128 at
  131. A run so long is rare on real bars, but the profile does not hide it.
- Buy 9 enters long; sell 9 enters short. The long rule is tried first. A
  signal on a bar with an open position is dropped, not queued.
- Perfection (seasonal profile only): at the bar where the stored count is 9,
  buy needs `min(low[i-1], low[i]) <= min(low[i-3], low[i-2])` and sell needs
  `max(high[i-1], high[i]) >= max(high[i-3], high[i-2])`. Equality passes.
  Bars before index 3 never pass. There is no delayed perfection.
- Seasonality (seasonal profile only): an intraday table over UTC hours, built
  only from earlier bars, with a 90-day window (an observation exactly 90 days
  old is kept). The strategy reads the bucket of the next UTC hour. The gate
  fails when that bucket does not exist, has no directional bars, or has fewer
  than 10 directional bars. A short needs `bullishPercent <= 40`; a long needs
  `bullishPercent >= 60`. Both are inclusive. Dojis are excluded from the
  percentage. The table is UTC only: no DST or session calendar. On timeframes
  of two hours or longer with hour-aligned opens the next-hour bucket is never
  created and the profile never signals.
- Stop: long `low - 0.5 * ATR14`, short `high + 0.5 * ATR14`, from the signal
  bar. ATR is the simple moving average of true range, seeded with the partial
  average for the first bars. Target: `entry + side * |entry - stop| * 2` with
  `entry` the signal-bar close before slippage. Both are fixed at signal time.
- Size is `riskUsd / |filled entry - stop|`, using the slipped fill. A zero
  distance gives size 0. There is no breakeven, trail, cooldown or time exit.
  An open position at the end of the data is closed at the last close.

## Execution

The only supported fill is the archived convention: `costs.fillOn = close`,
meaning the signal-bar close plus slippage. A carried stop that the next bar
opens through fills at the open; if one bar touches both stop and target, the
stop wins. `fillOn = open` or `nextOpen` is refused with an error: the shared
next-open comparison policy needs its own reviewed rules for the stop and
target (HT-231). The profile ID, not the cost settings, names the rules, so
the two execution policies can never share a config digest.

## Engine support

The family runs through `Run`, prepared runs and `RunPrefix`. The counter is a
function of the closes up to each bar, so prefixes, batches and resumed
checkpoints agree. `RunPrefixResumable` accepts the family. Source-entry (C5)
routes are not supported.

## Known platform difference (D1)

The shared broker computes a closed trade's P&L as
`points*size - feePerUnit*size` in `closePosition` (`engine/broker.go`). On
arm64 the Go compiler fuses that into one fused multiply-add, so `pnl` can
differ in the last bits from the same expression on amd64 and from the archived
JavaScript, which never fuses. The effect needs a non-zero `feePerUnit`, applies
to every family that uses the broker, and does not affect amd64, CI or the WASM
builds. This family's own stop and target arithmetic is protected from fusion.
The oracle test compares P&L exactly off arm64 and within the rounding of its
three terms on arm64. Fixing the broker is a separate task: it would change
arm64 output for every family.
