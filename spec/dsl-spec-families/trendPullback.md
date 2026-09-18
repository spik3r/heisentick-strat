# Setup family: `trend pullback` (trendPullback)

Status: specified (2026-07-03)

Part of `strat/docs/dsl-spec.md` §9; authoring rules and the claim tracker live in
`plans/COMPLETED.md` (spec-families closeout).

## Type phrase

```
setup {
  type: trend pullback
}
```

## Purpose

`trend pullback` joins continuation in a trending market after price tags an
EMA, stays within a controlled pullback depth, and resumes on a directional
trigger candle. The default promoted shape is long-only XAUUSD continuation,
but the family runtime can evaluate either side when shared side filters allow
it.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `ema length N` | EMA length used for the pullback tag and trend-side close test; also enables EMA context. | `21` | `trendPullback.emaLen`, `emaLen` |
| `ema slope window N` | EMA slope lookback window. `ema slope N` and `ema window N` are also accepted by the parser shape. | `5` | `trendPullback.emaSlopeLen` |
| `pullback within N candles` | The EMA tag must have happened within the last `N` candles. | `6` | `trendPullback.pullbackWithin` |
| `pullback tag X ATR` | Tag tolerance around the EMA, in ATR. Longs require a low at or below `EMA + X ATR`; shorts require a high at or above `EMA - X ATR`. | `0.25` | `trendPullback.pullbackAtr` |
| `pullback max depth X` | Maximum pullback depth as a fraction of the preceding impulse range. | `0.5` | `trendPullback.maxPullbackDepth` |
| `pullback attempt count N` | Wait for the `N`th distinct consecutive signal attempt before entering. | `1` | `trendPullback.attemptCount` |
| `pullback session VWAP wick touch` | Replaces the EMA-tag location gate with a touch whose full candle range contains the current causal VWAP. | Off | `trendPullback.sessionVwapTouch = "wick"` |
| `pullback session VWAP close touch` | Replaces the EMA-tag location gate with a touch whose real body (open-to-close) contains the current causal VWAP. | Off | `trendPullback.sessionVwapTouch = "close"` |
| `pullback first session VWAP touch only` | Admits only session touch count one. The count is incremented on every closed configured-session candle before trend/trigger gates, and resets for each local trade-window session. | Required with a session-VWAP touch | `trendPullback.sessionVwapFirstTouchOnly = 1` |
| `reclaim session VWAP on trend side` | Requires the decision close to finish strictly above current VWAP for a long or strictly below it for a short. | Required with a session-VWAP touch | `trendPullback.sessionVwapTrendSideReclaim = 1` |
| `movement between X and Y` | Requires the current movement-efficiency ratio to be inside `[X, Y]`. | `0.35` to `0.95` | `trendPullback.minEr`, `trendPullback.maxEr` |

## Defaults and interactions

The type phrase sets `setupType: "trendPullback"`, initializes
`trendPullback` with `emaLen: 21` and `emaSlopeLen: 5`, enables core EMA
context at length `21`, and re-bases shared defaults to the pullback profile:
stop padding `0.25` ATR, minimum stop `0.5` ATR, unbounded maximum stop,
breakeven after `0.5R` plus `0.05` ATR, cooldown `12` candles, and target
`2R`.

The runtime requires a trending regime, movement efficiency inside the
configured range, a close on the trend side of the EMA, and a positive EMA
slope for longs or negative slope for shorts. Pullback depth is measured from
the prior `12` bars before the tag (`impulseLookbackBars`); that lookback has
no DSL phrase. Stops use the pullback extreme plus the configured padding and
minimum distance. Explicit pullback-style max-stop clamps are enforced by the
runtime and setup-candidate diagnostics.

Higher-timeframe filtering is controlled by the shared `higher timeframe must
agree` / `must not oppose entry` directive; current DSL behavior leaves it off
unless that directive is present, even though the setup module's raw default is
on. Session windows come from `sessions(...)`; if omitted, the core default of
Asia, London, and NY applies.

Shared `side`, `target NR`, stop, breakeven, partial, trail, cooldown, hold,
and risk directives map into this family's runtime params. `target NR` sets
`target.tpbR`. The shared `trigger any` phrase disables the family trigger
requirement. Other trigger-candle lists are parsed, but the runtime still uses
its built-in pin, engulfing, or outside-bar trigger set when triggers are on.

The session-VWAP phrases are an all-or-nothing opt-in: the compiler rejects a
touch phrase without both the first-touch and trend-side-reclaim phrases (and
rejects either companion phrase without a wick/close touch). They use the
existing causal day-reset VWAP column while counting touches separately within
each enabled local trade window (Asia, Mid, London, or NY). The count is
advanced before regime, HTF, trigger, cooldown, or stop admission, so an
earlier failed touch cannot be reclassified later as the first eligible touch.
With this gate, the touch index replaces the EMA tag as the pullback anchor;
EMA slope still supplies the trend direction. No strategy is registered merely
by authoring these phrases.

## Example

```dsl
dsl v7
strategy "Spec Trend Pullback" {
  description "Join a controlled EMA pullback in a trending market."
}

market conditions {
  slices(XAUUSD 1h)
  sessions(asia, london)
  day type in (trending)
}

setup {
  type: trend pullback
  ema length 21
  ema slope window 5
  pullback tag 0.25 ATR
  pullback within 6 candles
  pullback max depth 0.5
  pullback attempt count 1
  movement between 0.35 and 0.95
  pullback session VWAP wick touch
  pullback first session VWAP touch only
  reclaim session VWAP on trend side
}

filters {
  higher timeframe must agree
  side long only
}

risk {
  stop beyond pullback edge by 0.25 ATR min 0.5 ATR
}

target {
  target 2R
}

management {
  move stop to breakeven after 0.5R plus 0.05 ATR
  wait 12 candles after trade
}

execution {
  risk 200 USD
}
```
