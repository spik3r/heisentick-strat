# DSL Phrase Reference

Quick lookup for common Strat v7 directive phrases. The normative reference is
[`dsl-spec.md`](dsl-spec.md); setup-family-specific phrases live in
[`dsl-spec-families/`](dsl-spec-families/). Every fenced `dsl` block in this
file is compiled by the doc-fence tests.

## Categories

- [Strategy & Routing](#strategy--routing)
- [Market Conditions](#market-conditions)
- [Levels](#levels)
- [Filters, Trigger, Entry](#filters-trigger-entry)
- [Setups](#setups)
- [Risk, Target, Management](#risk-target-management)
- [Execution & Grade](#execution--grade)
- [Compiling Example](#compiling-example)

## Strategy & Routing

| Phrase | Parameters | Description | Example |
| --- | --- | --- | --- |
| `dsl v7` | - | Declares the current language version. | `dsl v7` |
| `strategy "Name"` | name | Sets the display name. | `strategy "London sweep" { ... }` |
| `description "..."` | text | Sets strategy notes/intent. | `description "Fade failed breakouts."` |
| `slices(...)` | symbol/timeframe pairs | Preferred route declaration. | `slices(XAUUSD 1h, EURUSD 15m)` |
| `symbols(...)` | symbols | Symbol allow-list when not using `slices`. | `symbols(XAUUSD, EURUSD)` |
| `timeframes(...)` | timeframes | Timeframe allow-list when not using `slices`. | `timeframes(15m, 1h)` |

## Market Conditions

| Phrase | Parameters | Description | Example |
| --- | --- | --- | --- |
| `sessions(...)` | `asia`, `mid`, `london`, `ny` | Enables trading sessions. | `sessions(london, ny)` |
| `trade window in (...)` | `<session>.<part>` | Restricts fixed UTC+10 windows to half-open 60-minute parts; the fourth `mid` hour matches only `mid.all`. | `trade window in (london.open, ny.open)` |
| `trade window minutes A to B` | minutes | Restricts minutes from the active window open to the half-open interval `[A, B)`. | `trade window minutes 0 to 90` |
| `local weekday in (...)` / `not in (...)` | weekdays | Allows or blocks local weekdays. | `local weekday not in (Fri)` |
| `local hour in (...)` / `not in (...)` | hours | Allows or blocks local hours. | `local hour in (7, 8, 9)` |
| `session phase in (...)` / `not in (...)` | phases | Allows or blocks named session phases. | `session phase not in (lunch)` |
| `day type in (...)` | day types | Allows day-shape classes, optionally with movement escape. | `day type in (trending) or movement below 0.6` |
| `prior day type in (...)` / `not in (...)` | day types | Allows or blocks prior-day classes. | `prior day type in (ranging)` |
| `day theme in (...)` | themes | Requires a day-plan theme. | `day theme in (buy_lows, join_momentum)` |
| `open location in (...)` | locations | Filters where the day opened versus key levels. | `open location in (nearPDH, nearDO)` |
| `movement below X` | ratio | Caps movement efficiency. | `movement below 0.8` |
| `room session at least X ATR` | scope, ATR | Requires unused room in a range scope. | `room session at least 1.2 ATR` |
| `<scope> range used below N%` | scope, percent | Filters range usage. | `session range used below 70%` |
| `<scope> range exhausted up/down` | scope, side | Requires range exhaustion. | `session range exhausted up at least 70%` |
| `session bias ...` | `up`, `down`, `mixed`, `pause` | Filters session/window bias. | `session bias up` |
| `seasonality ...` | dimension/classes | Filters by prior-data seasonal class. | `seasonality intraday all supports entry` |
| `ema length N` | length | Enables EMA context and the `EMA` level. | `ema length 21` |

## Levels

| Phrase | Parameters | Description | Example |
| --- | --- | --- | --- |
| `priority(...)` | level names | Ordered level universe. | `priority(PDH, PDL, VWAP, EMA)` |
| `near within X` | ATR distance | Requires proximity to priority levels. | `near within 1.5` |
| `near <levels> within X` | levels, ATR | Sets priority and proximity together. | `near PDH PDL within 1.2` |
| `support <price> [to <price>]` | price/zone | Defines a custom support level or zone. | `support 2285.0 to 2287.5` |
| `resistance <price> [to <price>]` | price/zone | Defines a custom resistance level or zone. | `resistance 2350.0` |
| `trendline from ... to ...` | dates/prices | Defines a custom diagonal level. | `trendline from 2026-01-02 2300 to 2026-01-10 2340` |
| `fib from ... to ... at ...` | dates/prices/ratios | Defines Fibonacci levels from a leg. | `fib from 2026-01-02 2300 to 2026-01-10 2400 at 0.5 0.618` |
| `level must align ...` | ATR distance | Requires level confluence. | `level must align with another level within 0.4 ATR` |

## Filters, Trigger, Entry

| Phrase | Parameters | Description | Example |
| --- | --- | --- | --- |
| `range method ...` | `pivot`, `zone` | Selects range detection mode. | `range method pivot` |
| `range active within N candles` | candle count | Limits range age. | `range active within 8 candles` |
| `channel active within N candles` | candle count | Enables and limits channel age. | `channel active within 12 candles` |
| `channel width ... ATR` | ATR bounds | Filters channel width. | `channel width between 0.6 and 2.4 ATR` |
| `channel direction in (...)` | directions | Filters channel slope. | `channel direction in (ascending, descending)` |
| `higher timeframe ...` | mode/timeframe | Enables or disables HTF alignment. | `higher timeframe must agree` |
| `side ...` | side mode | Restricts trade direction. | `side long only` |
| `approach at least N candles` | candle count | Requires sustained approach. | `approach at least 3 candles` |
| `tail rejection at least X` | ratio | Requires rejection-tail quality. | `tail rejection at least 0.35` |
| `close location at least X` | ratio | Requires close location quality. | `close location at least 0.65` |
| `entry distance max X ATR` | ATR distance | Caps entry distance from the level. | `entry distance max 1.2 ATR` |
| `candle in (...)` | candle names | Restricts signal candle shapes. | `candle in (pin, engulf, outside)` |
| `setup expires after N candles` | candle count | Limits setup age. | `setup expires after 6 candles` |
| `when price sweeps ... then signal ...` | edge/side | Adds an explicit sweep-and-reclaim entry rule. | `when price sweeps range.high by 0.3 within 3 then signal short` |
| `enter at market` | - | Enters on the signal close. | `enter at market` |
| `enter with limit ...` | edge/expiry | Places a limit order at the swept edge. | `enter with limit at swept edge within 5 candles` |

## Setups

`type: <family>` selects the setup family and must appear before that family's
own phrases.

| Phrase | Description | Example |
| --- | --- | --- |
| `type: failed breakout` | Default sweep/failure setup. | `type: failed breakout` |
| `type: flag continuation` | Break then shallow flag continuation. | `type: flag continuation` |
| `type: break retest` | Break, retest, continuation rejection. | `type: break retest` |
| `type: opening range breakout` | Breakout from an opening range. | `type: opening range breakout` |
| `type: session break hold` | Session high/low break and hold. | `type: session break hold` |
| `type: channel break hold` | Channel edge break and hold. | `type: channel break hold` |
| `type: inside day expansion` | Expansion from an inside-day range. | `type: inside day expansion` |
| `type: day open reclaim` | Sweep and reclaim of day open. | `type: day open reclaim` |
| `type: supply demand` | Retest of detected supply/demand zones. | `type: supply demand` |
| `type: double top bottom` | Two-swing reversal. | `type: double top bottom` |
| `type: range break fake` | Fade a failed confirmed-range breakout. | `type: range break fake` |
| `type: trend pullback` | Pullback into an established trend. | `type: trend pullback` |
| `type: fib continuation` | Fibonacci continuation setup. | `type: fib continuation` |
| `type: triple push exhaustion` | Three-push exhaustion reversal. | `type: triple push exhaustion` |
| `type: vwap extension fade` | Mean reversion from VWAP extension. | `type: vwap extension fade` |
| `type: volume anomaly exhaustion` | Exhaustion after anomalous volume. | `type: volume anomaly exhaustion` |
| `type: elder triple screen` | Elder triple-screen alignment. | `type: elder triple screen` |
| `type: price momentum` | Close-to-close time-series momentum. | `type: price momentum` |
| `lookback N candles` | Sets the price-momentum ROC lookback. | `lookback 20 candles` |
| `neutral zone X percent` | Sets the symmetric no-signal threshold. | `neutral zone 1 percent` |

## Risk, Target, Management

| Phrase | Parameters | Description | Example |
| --- | --- | --- | --- |
| `stop beyond last N candle extreme by X ATR` | candles, ATR | Structure stop beyond a recent extreme. | `stop beyond last 3 candle extreme by 0.25 ATR` |
| `stop beyond ... edge by X ATR` | structure, ATR | Setup-structure stop. | `stop beyond pullback edge by 0.25 ATR min 0.5 ATR` |
| `stop beyond opposite range edge` | ATR/min/max optional | Stop behind the opposite range edge. | `stop beyond opposite range edge by 0.25 min 0.4 max 3` |
| `stop M ATR` | ATR multiple | Fixed ATR stop. | `stop 1.2 ATR` |
| `stop beyond <ratio> retrace` | fib ratio | Fib-retrace stop. | `stop beyond 0.618 retrace by 0.2` |
| `stop size min A max B` | ATR bounds | Clamps stop size. | `stop size min 0.2 max 1.5` |
| `target NR` | R multiple | Fixed R-multiple target. | `target 2R` |
| `take profit opposite ... edge` | range/channel | Targets an opposite edge. | `take profit at the opposite range edge` |
| `minimum reward X` | R multiple | Minimum R for edge target. | `minimum reward 0.8` |
| `fallback X` | R multiple | Fallback R when no edge target exists. | `fallback 1` |
| `target fib extension <ratio>` | ratio | Measured-move target. | `target fib extension 1.272` |
| `target N range` | range multiple | Opening-range target. | `target 1 range` |
| `move stop to breakeven after X R` | R, optional ATR | Moves stop to breakeven. | `move stop to breakeven after 0.75R plus 0.05 ATR` |
| `partial P% at XR` | percent/R | Takes a partial exit. | `partial 50% at 1R move stop to breakeven` |
| `trail N ATR after XR` | ATR/R | Enables an ATR trailing stop. | `trail 1.5 ATR after 1R` |
| `wait N candles after trade` | candle count | Re-entry cooldown. | `wait 12 candles after trade` |
| `maxHoldCandles N` | candle count | Time stop. | `maxHoldCandles 24` |

## Execution & Grade

| Phrase | Parameters | Description | Example |
| --- | --- | --- | --- |
| `risk N USD` | amount | Fixed account-currency risk per trade. | `risk 200 USD` |
| `context N` | score | Grade context weight. | `context 2` |
| `location N` | score | Grade location weight. | `location 2` |
| `trigger N` | score | Grade trigger weight. | `trigger 1` |
| `risk reward N` | score | Grade reward weight. | `risk reward 1` |
| `require >= N` | score | Rejects signals below total grade. | `require >= 4` |
| `size by score ...` | threshold/multiplier pairs | Scales risk by grade. | `size by score 5 1.25 7 1.5` |

## Compiling Example

```dsl
dsl v7
strategy "Phrase Reference Sample" {
  description "Compilable Strat v7 sample for phrase-reference coverage."
}

market conditions {
  slices(XAUUSD 1h)
  sessions(london, ny)
  trade window in (london.open, ny.open)
  trade window minutes 0 to 90
  session phase not in (lunch)
  day type in (trending) or movement below 0.6
  ema length 21
}

levels {
  priority(PDH, PDL, VWAP, EMA)
  near within 1.5
}

filters {
  higher timeframe must agree
  side both
  tail rejection at least 0.35
  close location at least 0.65
  entry distance max 1.2 ATR
}

trigger {
  candle in (pin, engulf, outside)
  setup expires after 6 candles
}

setup {
  type: failed breakout
}

risk {
  stop beyond last 3 candle extreme by 0.25 ATR
  stop size min 0.2 max 1.5
}

target {
  target 2R
}

management {
  partial 50% at 1R move stop to breakeven
  move stop to breakeven after 0.75R plus 0.05 ATR
  wait 12 candles after trade
  maxHoldCandles 24
}

execution {
  risk 200 USD
}
```
