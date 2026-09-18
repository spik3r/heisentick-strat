# Setup family: `failed breakout` (failedBreakout)

Status: specified (2026-07-03)

Specified against `engine/dsl/setups/levelSweep.js`,
`strat/implementations/browser-runtime/compiler/parseSetups.js`, `engine/dsl/spec/runtime.js`,
`test/engine.test.mjs`, and
`strat/conformance/parse/setup-level-sweep.strat` plus
`strat/conformance/parse/setup-level-sweep.cfg.json`.

Part of `strat/docs/dsl-spec.md` §9; authoring rules and the claim tracker live in
`plans/COMPLETED.md` (spec-families closeout).

## Type phrase

```
setup {
  type: failed breakout
}
```

## Purpose

Failed breakout fades a sweep through a recent range or channel edge after
price closes back inside the structure. It is the default DSL setup family and
is intended for ranging or choppy conditions near prioritized levels.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `type: failed breakout` | Selects the range/channel sweep-reclaim family. | The whole DSL already defaults to `failedBreakout`; this phrase makes it explicit. | `setupType = "failedBreakout"` |
| `when price sweeps range.high\|range.low\|channel.high\|channel.low by X within N then signal long\|short [grade G]` | Defines the sweep edge, sweep distance, reclaim window, entry side, and optional signal grade. Channel edges also enable channel detection. | High edges default short, low edges default long; sweep distance inherits the configured 0.1 ATR default unless overridden; reclaim defaults to 3 candles. | `sweepRules.high/low.source`, `sweepRules.high/low.sweepDistanceAtr`, `sweepRules.high/low.reclaimCandles`, `sweepRules.high/low.side`, `sweepRules.high/low.grade`, `channel.enabled` for channel edges |
| `sweep high\|low sweepDistanceAtr X reclaimCandles N enter long\|short` | Deprecated key/value spelling for a sweep rule. `sweepAtr` and `reclaimBars` are accepted aliases. | Source inherits the existing rule source or `range`; side defaults short at highs and long at lows; sweep distance inherits the configured 0.1 ATR default unless overridden; reclaim defaults to 3 candles. | Same `sweepRules.high/low.*` keys as the `when price sweeps ...` phrase |

## Defaults and interactions

Selecting this family does not rebase defaults; the base `DEFAULT_CFG` is the
failed-breakout profile. Defaults include range/choppy day types, 5m and 15m
timeframes, Asia/London/NY sessions, priority levels `PDH PDL WH WL`, range
active within 8 candles, approach for at least 3 candles, pin/engulf/outside
trigger candles, stop beyond the last 3-candle extreme by 0.25 ATR with max
size 1.5 ATR, opposite range-edge target with minimum reward 0.6R and fallback
1R, breakeven after 0.75R plus 0.02 ATR, and a 3-candle cooldown.

Runtime entry requires a recent range or channel, a nearest prioritized level
when `levelPriority` is non-empty, optional confluence, a sweep and close back
inside within the reclaim window, sustained approach, trigger candle and candle
quality gates, optional entry-distance/setup-age gates, and one daily entry per
side/edge price. `take profit [at] [the] opposite channel edge` switches the
target lookup to the active channel; limit entry rests at the swept edge instead
of entering on the signal close.

## Example

```dsl
dsl v7
strategy "Failed Breakout Spec Example" {
  description "Fade range-edge sweeps after price reclaims the box."
}

market conditions {
  timeframes(5m, 15m)
  sessions(asia, london, ny)
  day type in (ranging, choppy)
}

levels {
  priority(WH, WL, PDH, PDL, AH, AL, LH, LL)
  range edge must be within 1.5 ATR of level
}

filters {
  range method zone
  range active within 8 candles
  approach at least 3 candles toward level
}

setup {
  type: failed breakout
}

trigger {
  candle in (pin, engulf, outside)
}

entry {
  when price sweeps range.high by 0.1 within 3 then signal short grade A
  when price sweeps range.low by 0.1 within 3 then signal long grade A
}

risk {
  stop beyond last 3 candle extreme by 0.25 ATR
  stop size max 1.5
}

target {
  take profit at opposite range edge
  minimum reward 0.6R
  fallback 1R
}

management {
  move stop to breakeven after 0.75R plus 0.02 ATR
  wait 3 candles after trade
}

execution {
  risk 200 USD
}
```
