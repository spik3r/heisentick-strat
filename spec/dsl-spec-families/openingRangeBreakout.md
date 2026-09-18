# Setup family: `opening range breakout` (openingRangeBreakout)

Status: specified (2026-07-03)

Part of `strat/docs/dsl-spec.md` §9; authoring rules and the claim tracker live in
`plans/COMPLETED.md` (spec-families closeout).

## Type phrase

```
setup {
  type: opening range breakout
}
```

## Purpose

Trades the first held continuation break after a session opening range is
frozen. The range is built from already-closed candles or minutes at the start
of an enabled trade window, then the setup enters the first close that holds
beyond the high or low.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `opening range first <N> candles [in (<sessions>)]` | Build the opening range from the first `N` closed candles of the current allowed window. Numeric `bars` unit aliases are deprecated but accepted. If `in (...)` is present, those sessions also replace the shared enabled session set. | `4` candles; type defaults sessions to London+NY only when sessions are still unset by the author | `openingRangeBreakout.firstCandles`, `openingRangeBreakout.firstMinutes = 0`, `openingRangeBreakout.openingSessions`, `sessions` |
| `opening range first <N> minutes [in (<sessions>)]` | Build the opening range from closed bars whose minutes-from-window-start are less than `N`; the setup waits until the current bar is at or after that minute mark. If `in (...)` is present, those sessions also replace the shared enabled session set. | Off; candle count mode is used | `openingRangeBreakout.firstMinutes`, `openingRangeBreakout.firstCandles = 0`, `openingRangeBreakout.openingSessions`, `sessions` |
| `opening range every <N> hours UTC` | Use fixed, contiguous UTC slots of integer `N` hours (1–24) instead of named local trade windows. The first 15 minutes of each slot form the range, one causal breakout may enter, and an open position exits when the next slot begins. Slots are epoch-aligned and remain contiguous even when `N` does not divide 24. | Off | `openingRangeBreakout.utcSlotMinutes` |
| `break ... [by <N> ATR]` | Set the breakout buffer beyond the frozen range edge. The parser accepts any phrase headed by `break`; `by` or `buffer` supplies the number. | `0` | `openingRangeBreakout.breakBufferAtr` |
| `hold <N> candles` | Require the last `N` closes to hold beyond the relevant range edge after range formation. | `1` | `openingRangeBreakout.holdCandles` |
| `retest within <N> candles [tolerance <X> ATR]` | Require a retest of the broken edge before entry. Long retests need a low at/through the high edge plus a close back above it; shorts mirror that at the low edge. `by <X> ATR` is also accepted for tolerance. | Retest off; `6` candles and `0.2 ATR` tolerance when enabled | `openingRangeBreakout.requireRetest = 1`, `openingRangeBreakout.retestCandles`, `openingRangeBreakout.retestToleranceAtr` |
| `retest off` / `retest none` | Disable the retest requirement. | Off | `openingRangeBreakout.requireRetest = 0`, `openingRangeBreakout.retestCandles`, `openingRangeBreakout.retestToleranceAtr` |
| `stop <N> pips` | Use a fixed absolute stop distance converted with the route instrument's reviewed pip size. When the order opens, the engine anchors this distance to the actual fill. Supported by this family for operational canaries. | Family stop | `stop.type = pips`, `stop.pips` |
| `target <N> pips` | Use a fixed absolute target distance converted with the route instrument's reviewed pip size. When the order opens, the engine anchors this distance to the actual fill. Supported by this family for operational canaries. | Family target | `target.pips` |

## Defaults and interactions

Selecting `type: opening range breakout` enables London+NY sessions only when
the author has not already set sessions, and explicit `sessions(...)`,
`windows(...)`, or `opening range ... in (...)` values win regardless of
directive order. The type also sets a structure stop beyond the opposite range
edge with `0.25 ATR` padding, `0.4 ATR` minimum stop and `3 ATR` maximum stop,
breakeven after `0.5R` plus `0.05 ATR`, a 12-candle cooldown, and a `1R`
family target fallback.

The runtime freezes each session/day range causally from prior bars only. It
enters at the signal close, marks each session/day as seen after the first
entry, and manages existing positions with shared breakeven and max-hold
rules. Shared `target <N>R` sets `target.orbR`; `target <N> range` sets a
range-multiple target (`target.orbRange`) that overrides the R target when
positive. `stop <N> ATR` switches to a fixed ATR stop; otherwise the stop is
the opposite opening-range edge plus shared stop padding. In UTC-slot mode,
the engine observes a slot change on the next closed bar and closes the open
trade at that bar's close. For XAUUSD, the reviewed pip size is `0.1`, so 10
pips is 1.0 price point and 20 pips is 2.0 price points.

## Example

```dsl
dsl v7
strategy "Spec Opening Range Breakout" {
  description "Join the first held break of the London or NY opening range."
}

market conditions {
  slices(XAUUSD 5m, XAUUSD 15m)
  sessions(london, ny)
  day type in (trending, ranging, choppy)
}

setup {
  type: opening range breakout
  opening range first 4 candles in (london, ny)
  break beyond range edge by 0.05 ATR
  hold 1 candle
}

filters {
  higher timeframe must agree
}

risk {
  stop opposite range edge 0.25 ATR
  stop size <= 3 ATR
}

target {
  target 1 range
}

management {
  move stop to breakeven after 0.75R plus 0.05 ATR
  maxHoldCandles 18
  wait 12 candles after trade
}

execution {
  risk: 200 USD
}
```
