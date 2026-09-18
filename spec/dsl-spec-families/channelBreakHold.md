# Setup family: `channel break hold` (channelBreakHold)

Status: specified (2026-07-03)

Part of `strat/docs/dsl-spec.md` §9; authoring rules and the claim tracker live in
`plans/COMPLETED.md` (spec-families closeout).

## Type phrase

```
setup {
  type: channel break hold
}
```

## Purpose

Channel break-hold trades continuation after price closes outside a causal
channel rail and holds there for the configured number of candles. A hold above
the upper rail enters long; a hold below the lower rail enters short.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `hold N candles` | Require the last `N` closes to hold outside the relevant channel rail, with the prior candle still inside or at the rail. | `3` | `channelBreakHold.holdCandles` |
| `stop inside channel X ATR` | Place the stop `X` ATR back inside the broken channel rail and set the minimum stop distance to the same value. | `0.5` from setup type defaults | `stop.paddingAtr`, `stop.minAtr` |
| `stop inside range X ATR` | Accepted by the parser for the same stop-inside behavior. | `0.5` from setup type defaults | `stop.paddingAtr`, `stop.minAtr` |

## Defaults and interactions

`type: channel break hold` enables channel detection and rebases shared
defaults to stop padding `0.5 ATR`, min stop `0.5 ATR`, max stop `3 ATR`,
target `1R`, breakeven after `0.5R` plus `0.05 ATR`, and a 12-candle
cooldown. Runtime params use the shared channel filters:
`channel active within N candles`, `channel width ...`, `channel direction in
(...)`, `channel source ...`, and `channel min width ...`.

The runtime rejects inactive or too-old channels, channels outside the
configured width bounds, and channel directions outside the allowed list. It
requires the pre-hold candle to be inside or at the rail, then all hold candles
to close beyond that rail. If trigger candles are not `any`, the current bar
must pass the shared pin/engulf/outside trigger check. Shared higher-timeframe,
session, side, target, breakeven, partial, max-hold, cooldown, and fixed-risk
directives feed runtime params. Entries are deduped per day by side, channel
start, and rounded rail price.

## Example

```dsl
dsl v7
strategy "Channel Break Hold Spec Example" {
  description "Documented channel rail break-and-hold continuation example."
}

market conditions {
  slices(XAUUSD 15m)
  sessions(london, ny)
  day type in (trending, choppy)
  movement below 1
}

setup {
  type: channel break hold
  hold 2 candles
  stop inside channel 0.5 ATR
}

filters {
  channel active within 12 candles
  channel width between 1 and 4 ATR
  channel direction in (directional)
  side both
}

triggers {
  candle in (any)
}

target {
  target 1R
}

management {
  move stop to breakeven after 0.5 R plus 0.05 ATR
  wait 12 candles after trade
}

execution {
  risk 200 USD
}
```
