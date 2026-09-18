# Setup family: `range break fake` (rangeBreakFake)

Status: specified (2026-07-03)

Part of `strat/docs/dsl-spec.md` §9; authoring rules and the claim tracker live in
`plans/COMPLETED.md` (spec-families closeout).

## Type phrase

```
setup {
  type: range break fake
}
```

## Purpose

Trades a false break of the current detected range: price pierces the active
range high or low, then reclaims back toward the range. Shorts fade failed
breaks above `range.high`; longs fade failed breaks below `range.low`.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `pierce at least <N> ATR` | Require the failed break to trade at least `N * ATR` beyond the range edge during the reclaim window. `minimum` and `min` are accepted in place of `at least`. | `0.1` | `rangeBreakFake.pierceAtr` |
| `reclaim within <N> candles` | Look back over the last `N` candles for the pierce and require the signal close to finish back inside the range. | `3`; close-back-inside required | `rangeBreakFake.reclaimCandles`, `rangeBreakFake.requireCloseBackInside = 1` |
| `reclaim wick within <N> candles` | Look back over the last `N` candles for the pierce, but allow a wick-only reclaim without requiring the signal close back inside the range. `wick` may appear anywhere in the phrase. | `3`; close-back-inside disabled | `rangeBreakFake.reclaimCandles`, `rangeBreakFake.requireCloseBackInside = 0` |
| `retest within <N> candles` | Require a retest of the failed edge within the last `N` candles: highs at or above `range.high` that close back below for shorts, lows at or below `range.low` that close back above for longs. | Retest off; `6` candles when enabled | `rangeBreakFake.retestFailedEdge = 1`, `rangeBreakFake.retestCandles` |
| `retest off` / `retest none` | Disable the failed-edge retest requirement. | Off | `rangeBreakFake.retestFailedEdge = 0`, `rangeBreakFake.retestCandles` |
| `target range midpoint` / `target range mid` | Target the midpoint of the active range. | `range midpoint` | `rangeBreakFake.targetMode = "rangeMid"` |
| `target opposite edge` | Target the opposite side of the active range. | `range midpoint` | `rangeBreakFake.targetMode = "oppositeEdge"` |
| `target vwap` | Target VWAP when `ctx.vwap` is finite; otherwise current runtime behavior falls back to the range midpoint. | `range midpoint` | `rangeBreakFake.targetMode = "vwap"` |
| `target <N>R` | Use a fixed R-multiple target. The `R` suffix is required by the family parser. | `1R` | `rangeBreakFake.targetMode = "fixedR"`, `rangeBreakFake.targetR` |

## Defaults and interactions

Selecting `type: range break fake` re-bases shared defaults to a 3-candle
recent-extreme stop with `0.25 ATR` padding, `0` minimum stop, `1.5 ATR`
maximum stop, breakeven after `0.75R` plus `0.02 ATR`, a 3-candle cooldown,
and `target 1R` as the family R fallback.

The runtime only trades when `ctx.lastRange` exists and its `sinceActive` is
within `range.activeWithinCandles` (`range active within N candles`, default
`8`). It then applies shared session gates, side gates, candle-quality gates
(`tail rejection at least`, `close location at least`), entry-distance gating
(`entry distance max X ATR`, default runtime value `0.6`), stop-size bounds,
and `minimum reward` (`target.minR`, default `0.6R`).

The default target mode is the range midpoint. `target vwap` is opportunistic:
if no finite VWAP is present in context, the implementation currently targets
the range midpoint instead of rejecting the setup.

## Example

```dsl
dsl v7
strategy "Spec Range Break Fake" {
  description "Fade failed range breaks back toward the active range."
}

market conditions {
  symbols(XAUUSD)
  timeframes(5m, 15m)
  sessions(london, ny)
  day type in (ranging, choppy)
}

levels {
  priority(AH, AL, LH, LL, PDH, PDL)
  range edge must be within 1 ATR of level
}

setup {
  type: range break fake
  pierce at least 0.1 ATR
  reclaim within 2 candles
  target range midpoint
}

filters {
  range method: zone
  range active within 8 candles
  tail rejection at least 0.3
  entry distance max 0.5 ATR from level
}

risk {
  stop beyond last 3 candle extreme by 0.25 ATR
  stop size <= 1.4 ATR
}

target {
  fallback: 1R
  minimum reward: 0.6R
}

management {
  move stop to breakeven after 0.75R plus 0.02 ATR
  wait 3 candles after trade
}

execution {
  risk: 200 USD
}
```
