# Setup family: `legacy setup 9` (`legacySetup9`)

Status: specified. Go only (HT-227). Not compiled by the JavaScript runtime.
The family is not available in the deployed app; adopting it is separate work
(HT-230).

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

## Preserved historical behaviour

The `.v1` profiles keep the archived behaviour, including its defects. A change
to any of these is a new profile ID, never an edit of `.v1`.

- The stored Setup count wraps like an 8-bit signed value while the internal
  count does not, so long runs emit phantom or repeated nines (see above).
- The seasonal gate is UTC only and reads the next UTC hour's bucket. On bars
  of two hours or longer with hour-aligned opens that bucket is never created,
  so the seasonal profile never signals there. This is the historical
  behaviour and this change adds no runtime warning; a warning for consumers
  belongs to the adoption task (HT-230).

## What "matches the archived JavaScript" means

Three levels, checked separately:

1. Exact semantic and trade matching. Setup counts, signals, sides, entry and
   exit bars, reasons, tags, and every price and size (`entry`, `exit`, `sl`,
   `tp`, `size`, `points`) are compared exactly, treating positive and negative
   zero as equal, with the pinned JavaScript
   on invented bars (`engine/testdata/legacy_setup9/oracle.json`, generated by
   `oracle/gen.mjs` from heisentick commit
   `48a1761867342494c69c77e7ecce325e10d5d935`). The engine test compares the
   broker's raw trades.
2. Tolerance-based numerical matching. Trade `pnl` is the only value compared
   with a tolerance, and only on arm64 (D1 below): the tolerance branch runs
   for every arm64 run, including zero `feePerUnit`. The bound is the sum of
   operand/result ULP sizes and need not be zero when the fee is zero. These
   checks therefore do not establish raw-bit P&L equality on arm64. Off arm64
   P&L is compared exactly.
3. Serialized matching. The conformance bridge and WASM output round numbers
   to 15 significant digits. A comparison of serialized values proves parity of
   the rounded values, not of the raw bits. `scripts/checks/legacy-setup9-parity.mjs`
   checks this level.

## Known numerical difference (D1)

The shared broker computes a closed trade's P&L as
`points*size - feePerUnit*size` in `closePosition` (`engine/broker.go`). On
arm64 the Go compiler may fuse that into one fused multiply-add, so `pnl` can
differ in the last bits from the same expression compiled without fusion and
from the archived JavaScript, which never fuses. This shared arithmetic applies
to every family that uses the broker. The oracle's arm64 tolerance applies
regardless of whether `feePerUnit` is zero; it is a numerical bound, not a claim
that a raw-bit difference was observed in every run. The family's own stop and
target arithmetic is written to prevent fusion. Fixing the broker is a
separate task because it changes arm64 output for every family.

The oracle test requires the JavaScript P&L to equal the unfused formula
exactly. On arm64 it accepts a Go P&L within the sum of the unit-in-the-last-place
sizes of the three terms (`points*size`, `feePerUnit*size`, and the result). That
bound comes from the operands; it is not a fixed number of result ulps. Off
arm64 the comparison is exact. This note makes no claim about which platforms
CI uses or about cross-architecture equality of other outputs.

## Parser note: repeated `type:`

Elsewhere in the parser a later `type:` line with a phrase that names no family
is ignored and the earlier family stays. That is repo-wide behaviour that
predates this family and is out of scope here. For this family a later `type:`
that names another family after `sequential profile` is an error, and the
compact one-line form is split at `sequential` like the other directive words.
