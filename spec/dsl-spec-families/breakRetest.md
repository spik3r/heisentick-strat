# Setup family: `break retest` (breakRetest)

Status: specified (2026-07-03)

Part of `strat/docs/dsl-spec.md` §9; authoring rules and the claim tracker live in
`plans/COMPLETED.md` (spec-families closeout).

## Type phrase

```
setup {
  type: break retest
}
```

## Purpose

`break retest` joins continuation after price closes through a selected key
level, pulls back to retest that level from the continuation side, then holds
beyond it on a continuation trigger candle. Long setups use highs as broken
resistance that becomes support; short setups mirror against lows.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `break within N candles` | The level break must have happened within the last `N` candles. | `12` | `breakRetest.freshBreakBars` |
| `break distance at least X ATR` | The break window must have run at least `X` ATR beyond the level before the retest. `least`, `minimum`, and `min` are accepted. | `1` | `breakRetest.minBreakAtr` |
| `retest within N candles` | A candle in the last `N` candles must tag the broken level from the continuation side. | `6` | `breakRetest.retestBars` |
| `retest tolerance X ATR` | Maximum ATR-scaled distance used to decide whether the pullback tagged the level. | `0.35` | `breakRetest.levelTolerance` |
| `displacement at least X ATR` | Optional conviction filter: the hold bar range must be at least `X` ATR. `least`, `minimum`, and `min` are accepted. | `0` (off) | `breakRetest.displacementAtr` |
| `movement between X and Y` | Requires the current movement-efficiency ratio to be inside `[X, Y]`. | `0.4` to `0.95` | `breakRetest.minEr`, `breakRetest.maxEr` |
| `swing levels lookback N candles pivot K count M` | Replace the default prior-day/session level map with confirmed causal swing highs for longs and swing lows for shorts. A pivot is only available after `K` bars confirm it. | key levels | `breakRetest.levelSource`, `swingLookbackBars`, `swingPivotK`, `swingLevelCount` |
| `ema length N` | Enable an EMA context column for this family. | off | `emaLen` |
| `ema slope window N` | Use this many bars to measure the EMA slope when EMA confluence is enabled. | `5` | `breakRetest.emaSlopeLen` |
| `ema confluence within X ATR` | Require the entry close to be on the EMA/slope trend side and the flipped level to be within `X * ATR` of the EMA. | off | `breakRetest.emaConfluenceAtr` |

## Defaults and interactions

The type phrase sets `setupType: "breakRetest"`, initializes
`breakRetest`, and re-bases shared defaults to a retest-style stop and
management profile: stop padding `0.25` ATR, minimum stop `0.4` ATR,
unbounded maximum stop, breakeven after `0.5R` plus `0.05` ATR, cooldown
`12` candles, and target `0.8R`.

When `priority(...)` is omitted, runtime level selection uses only `PDH` for
longs and `PDL` for shorts. Explicit priority may select prior-day, prior-week,
or prior-session levels: long side recognizes `PDH`, `WH`, `AH`, `LH`; short
side recognizes `PDL`, `WL`, `AL`, `LL`.

The runtime always requires a trending regime and movement efficiency inside
the configured range. Higher-timeframe filtering is controlled by the shared
`higher timeframe must agree` / `must not oppose entry` directive; current DSL
behavior leaves it off unless that directive is present, even though the setup
module's raw default is on. Session windows also come from the shared
`sessions(...)` directive; if omitted, the core default of Asia, London, and NY
applies.

Shared `side`, `target NR`, `target fib extension R`, breakeven, partial,
trail, cooldown, hold, and risk directives map into this family's runtime
params. The regular `target NR` sets `target.brR`; `target fib extension R`
sets `target.brFibExt` and projects the measured break impulse instead of
using an R multiple. Stop directives such as `stop beyond pullback edge by X
ATR min Y ATR max Z ATR` set the padding/minimum/maximum fields; runtime stops
are based on the retest-window extreme and enforce the explicit max-stop clamp
when present.

Current trigger behavior is fixed in the setup module: entries require a
bullish/bearish pin, engulfing candle, or outside bar in the entry direction.
The shared trigger-candle list is parsed but does not choose a narrower trigger
set for this family.

## Example

```dsl
dsl v7
strategy "Spec Break Retest" {
  description "Join after a key level break retests and holds."
}

market conditions {
  timeframes(15m, 1h)
  sessions(london, ny)
  day type in (trending) strictly
}

levels {
  priority(PDH, PDL)
}

setup {
  type: break retest
  swing levels lookback 60 candles pivot 2 count 4
  break within 12 candles
  break distance at least 1 ATR
  retest within 6 candles
  retest tolerance 0.35 ATR
  displacement at least 0.5 ATR
  movement between 0.4 and 0.95
  ema length 50
  ema slope window 5
  ema confluence within 0.25 ATR
}

filters {
  higher timeframe must agree
  side both
}

risk {
  stop beyond pullback edge by 0.25 ATR min 0.4 ATR
}

target {
  target 0.8R
}

management {
  move stop to breakeven after 0.5R plus 0.05 ATR
  wait 12 candles after trade
}

execution {
  risk 200 USD
}
```
