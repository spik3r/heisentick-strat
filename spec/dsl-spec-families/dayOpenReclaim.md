# Setup family: `day open reclaim` (dayOpenReclaim)

Status: specified (2026-07-03)

Part of `strat/docs/dsl-spec.md` §9; authoring rules and the claim tracker live in
`plans/COMPLETED.md` (spec-families closeout).

## Type phrase

```
setup {
  type: day open reclaim
}
```

## Purpose

Trades a reclaim of the current day open after price first stretches away
from it into a nearby prior-day or prior-week level. Longs reclaim upward
through the day open after a downside stretch; shorts reclaim downward after
an upside stretch.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `stretch from day open at least <N> ATR` | Require price to stretch at least `N * ATR` away from `ctx.dayOpen` within the reclaim window before the reclaim can qualify. `minimum` and `min` are accepted in place of `at least`. | `1` | `dayOpen.stretchAtr` |
| `reclaim day open within <N> candles` | Set the lookback window used for the stretch and require the previous close to be on the far side of day open while the signal close reclaims through it. | `3` | `dayOpen.reclaimCandles` |

## Defaults and interactions

Selecting `type: day open reclaim` sets a recent-extreme stop over the last
`5` candles with `0.25 ATR` padding, `0.5 ATR` minimum stop, `2 ATR` maximum
stop, breakeven after `0.5R` plus `0.05 ATR`, a 12-candle cooldown, and a
`1R` family target fallback.

The runtime requires a finite `ctx.dayOpen`, current trade-window membership,
shared side gates, optional shared HTF agreement when declared, optional
trigger-candle filtering when `candle in (...)` is declared, stop-size bounds,
and a nearby level. The nearby level search uses the shared `priority(...)`
order and `levelDistanceAtr` (`near... within`, `recent extreme must be
within`, or equivalent level-distance phrases), defaulting to `PDH`, `PDL`,
`WH`, `WL` within `1.5 ATR`.

The stretched extreme is measured over the same `reclaimCandles` window used
for the day-open reclaim. Targets use shared `target <N>R` as `target.dorR`;
the setup does not have a family-specific target phrase.

## Example

```dsl
dsl v7
strategy "Spec Day Open Reclaim" {
  description "Trade NY day-open reclaims after a stretch into prior levels."
}

market conditions {
  timeframes(15m)
  sessions(ny)
  day type in (ranging, choppy)
}

levels {
  priority(PDH, PDL, WH, WL)
  recent extreme must be within 1.5 ATR of level
}

setup {
  type: day open reclaim
  stretch from day open at least 0.7 ATR
  reclaim day open within 3 candles
}

triggers {
  candle in (pin, engulf, outside)
}

filters {
  higher timeframe must agree
  side: short only
}

risk {
  stop beyond last 5 candle extreme by 0.25 ATR
  stop size <= 2 ATR
}

target {
  target 1.2R
}

management {
  move stop to breakeven after 0.5R plus 0.05 ATR
  wait 12 candles after trade
}

execution {
  risk: 200 USD
}
```
