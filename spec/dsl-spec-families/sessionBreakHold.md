# Setup family: `session break hold` (sessionBreakHold)

Status: specified (2026-07-03)

Part of `strat/docs/dsl-spec.md` §9; authoring rules and the claim tracker live in
`plans/COMPLETED.md` (spec-families closeout).

## Type phrase

```
setup {
  type: session break hold
}
```

## Purpose

`session break hold` joins continuation after price breaks a prior-session high
or low and proves the break by closing beyond the level for several candles.
Asia and London highs can produce long setups; Asia and London lows can
produce short setups.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `hold N candles` | The last `N` closes must all remain beyond the broken session level, and the close before that hold window must still be inside the range. | `3` | `sessionBreakHold.holdCandles` |
| `stop inside range X ATR` | Place the stop `X` ATR back inside the broken range; also sets the minimum stop distance to `X` ATR. The parser also accepts `stop inside range by X ATR`. | `0.5` | `stop.paddingAtr`, `stop.minAtr` |

## Defaults and interactions

The type phrase sets `setupType: "sessionBreakHold"`, initializes
`sessionBreakHold`, and re-bases shared defaults to the session-break profile:
stop padding `0.5` ATR, minimum stop `0.5` ATR, maximum stop `3` ATR,
breakeven after `0.5R` plus `0.05` ATR, cooldown `12` candles, and target
`1.2R`.

Runtime levels are prior-session `AH`, `AL`, `LH`, and `LL`. When
`priority(...)` names any of those keys, only the named keys are allowed;
otherwise all four are eligible. `AH` and `LH` are long levels, while `AL` and
`LL` are short levels. The setup records one entry per side/level key per day.

Higher-timeframe filtering is controlled by the shared `higher timeframe must
agree` / `must not oppose entry` directive; current DSL behavior leaves it off
unless that directive is present, even though the setup module's raw default is
on. Session windows come from `sessions(...)`; if omitted, the core default of
Asia, London, and NY applies.

Shared `side`, `target NR`, breakeven, partial, trail, cooldown, hold, and risk
directives map into this family's runtime params. `target NR` sets
`target.sbhR`. The family-specific `stop inside range` phrase maps to
`stopInsideAtr`; shared stop-size bounds can set the maximum stop distance, but
`stop inside range` itself sets the padding and minimum together.

Current trigger behavior is fixed in the setup module: entries require a
bullish/bearish pin, engulfing candle, or outside bar in the continuation
direction. The shared trigger-candle list is parsed but does not choose a
narrower trigger set or disable the trigger for this family.

## Example

```dsl
dsl v7
strategy "Spec Session Break Hold" {
  description "Join a prior-session level break after several closes hold."
}

market conditions {
  timeframes(15m)
  sessions(london, ny)
  day type in (trending, choppy)
}

levels {
  priority(AH, AL, LH, LL)
}

setup {
  type: session break hold
  hold 3 candles
  stop inside range 0.5 ATR
}

filters {
  higher timeframe must agree
  side both
}

target {
  target 1.2R
}

management {
  move stop to breakeven after 0.5R plus 0.05 ATR
  wait 12 candles after trade
}

execution {
  risk 200 USD
}
```
