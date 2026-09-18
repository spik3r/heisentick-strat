# Setup family: `flag continuation` (flagContinuation)

Status: specified (2026-07-03)

Part of `strat/docs/dsl-spec.md` §9; authoring rules and the claim tracker live in
`plans/COMPLETED.md` (spec-families closeout).

## Type phrase

```
setup {
  type: flag continuation
}
```

## Purpose

`flag continuation` joins a trending move after a fresh prior-day high or low
break, a strong pole, and a shallow flag near the broken level. The current
trend direction chooses the side: uptrends look for long flags above `PDH`,
downtrends look for short flags below `PDL`.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `first pullback` | Require the flag to be the first pullback after the level break. | `0` (off) | `flag.firstPullbackOnly` |
| `avoid lunch` | Block entries during the `lunch` session phase. | `0` (off) | `flag.avoidLunchBreakouts` |
| `pullback max depth X` | Maximum flag depth as a fraction of the pole range. | `0.34` | `flag.maxFlagDepth` |
| `pullback volatility contract` | Require the flag's average bar range to be smaller than the pole's average bar range. Any `pullback` phrase containing `contract` enables it. | `0` (off) | `flag.pullbackVolatilityContract` |
| `impulse close location at least X` | Require the pole-ending candle to close in the directional part of its range. `least`, `minimum`, and `min` are accepted. | `0` (off); parser fallback `0.75` | `flag.impulseCloseLocationMin` |
| `impulse at least X ATR over N candles` | Set minimum pole range and pole lookback. `least`, `minimum`, and `min` are accepted. | `1.5` ATR over `12` candles | `flag.poleAtr`, `flag.poleBars` |
| `pole net at least X of range` | Require net directional progress across the pole to be at least `X` of the pole range. `least` and `min` are accepted. | `0.25` | `flag.minPoleNetFrac` |
| `pole at least X ATR over N candles` | Alias shape for the pole range/lookback settings. `least` and `min` are accepted. | `1.5` ATR over `12` candles | `flag.poleAtr`, `flag.poleBars` |
| `flag depth max X of pole` | Maximum flag depth as a fraction of the pole range. `max`, `under`, and `below` are accepted. | `0.34` | `flag.maxFlagDepth` |
| `flag close position at least X` | Require the entry candle close to be at least `X` through the flag range in the continuation direction. `least` and `min` are accepted. | `0.45` | `flag.flagCloseFrac` |
| `flag N to M candles` | Minimum and maximum flag length. | `3` to `10` candles | `flag.minFlagBars`, `flag.maxFlagBars` |
| `break within N candles` | Prior-day level break must have occurred within the last `N` candles. | `24` | `flag.freshBreakBars` |
| `level tolerance X ATR` | Flag may extend this far beyond the broken prior-day level. | `0.5` | `flag.levelToleranceAtr` |
| `movement between X and Y` | Requires the current movement-efficiency ratio to be inside `[X, Y]`. | `0.5` to `0.75` | `flag.minEr`, `flag.maxEr` |

## Defaults and interactions

The type phrase sets `setupType: "flagContinuation"` and re-bases shared
defaults to the flag profile: stop padding `0.1` ATR, minimum stop `0.5` ATR,
unbounded maximum stop, breakeven after `0.5R` plus `0.05` ATR, cooldown
`12` candles, and target `0.8R`.

The runtime requires `ctx.regime === "trending"` and only evaluates the side
matching `ctx.trendDir`. Long flags are anchored to `PDH`; short flags are
anchored to `PDL`. The target is capped at the pole range in runtime
regardless of whether the target phrase includes the words `capped at pole`.

Higher-timeframe filtering is controlled by the shared `higher timeframe must
agree` / `must not oppose entry` directive; current DSL behavior leaves it off
unless that directive is present, even though the setup module's raw default is
on. Session windows come from `sessions(...)`; if omitted, the core default of
Asia, London, and NY applies. The setup module also has a slow-bias option, but
there is currently no family phrase that enables it.

Shared `side`, `target NR`, stop, breakeven, partial, trail, cooldown, hold,
and risk directives map into this family's runtime params. `target NR` sets
`target.flagR`. Stop phrases such as `stop beyond flag edge by X ATR min Y ATR`
set the flag stop padding and minimum distance.

Current trigger behavior is fixed in the setup module: entries require a
bullish/bearish pin, engulfing candle, or outside bar in the continuation
direction. The shared trigger-candle list is parsed but does not choose a
narrower trigger set for this family.

## Example

```dsl
dsl v7
strategy "Spec Flag Continuation" {
  description "Join a shallow flag after a fresh prior-day level break."
}

market conditions {
  slices(XAUUSD 1h)
  sessions(asia, london)
  day type in (trending)
}

setup {
  type: flag continuation
  pole at least 1.5 ATR over 12 candles
  pole net at least 0.25 of range
  flag 3 to 10 candles
  flag depth max 0.34 of pole
  flag close position at least 0.45
  break within 24 candles
  level tolerance 0.5 ATR
  movement between 0.5 and 0.75
}

filters {
  higher timeframe must agree
  side both
}

risk {
  stop beyond flag edge by 0.1 ATR min 0.5 ATR
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
