# Setup family: `inside day expansion` (insideDayExpansion)

Status: specified (2026-07-03)

Part of `strat/docs/dsl-spec.md` §9; authoring rules and the claim tracker live in
`plans/COMPLETED.md` (spec-families closeout).

## Type phrase

```
setup {
  type: inside day expansion
}
```

## Purpose

Trades expansion away from a prior inside day. The setup first confirms that
the previous day stayed inside the day before it, then enters a held close
above the inside-day high for longs or below the inside-day low for shorts.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `inside tolerance <N> ATR` | Allow the previous day to exceed the prior day's high/low by up to `N * ATR` and still count as inside. | `0.05` | `insideDay.insideToleranceAtr` |
| `hold <N> candles` | Require the last `N` closes to hold beyond the inside-day high or low, with the prior close on the other side of the level. | `1` | `insideDay.holdCandles` |
| `stop inside range <N> ATR [min <M>]` | Place the stop inside the broken inside-day range by setting shared stop padding. Optional `min` overrides the shared minimum stop. `by` is accepted before the padding value. | `0.35 ATR` padding; `0.5 ATR` minimum | `stop.paddingAtr`, `stop.minAtr` |

## Defaults and interactions

Selecting `type: inside day expansion` sets structure-stop defaults to `0.35
ATR` padding inside the range, `0.5 ATR` minimum stop, `3 ATR` maximum stop,
breakeven after `0.5R` plus `0.05 ATR`, a 12-candle cooldown, and a `1R`
family target fallback.

The runtime computes prior-day ranges causally from completed bars only. It
requires the current trade window, shared side gates, optional shared HTF
agreement when `higher timeframe must agree` or `must not oppose entry` is
declared, stop-size bounds, and the shared target R (`target.ideR`). Explicit
shared trigger directives such as `candle in (pin, engulf, outside)` enable
the setup's trigger check; `trigger any` disables it. It records one long per
inside-day high and one short per inside-day low using a daily seen set.

## Example

```dsl
dsl v7
strategy "Spec Inside Day Expansion" {
  description "Trade expansion after a prior inside day."
}

market conditions {
  timeframes(15m)
  sessions(london)
  day type in (trending, choppy)
}

levels {
  priority(PDH, PDL)
}

setup {
  type: inside day expansion
  inside tolerance 0.05 ATR
  hold 2 candles
  stop inside range 0.35 ATR
}

filters {
  higher timeframe must agree
}

target {
  target 0.8R
}

management {
  move stop to breakeven after 0.5R plus 0.05 ATR
  wait 12 candles after trade
}

execution {
  risk: 200 USD
}
```
