# Setup family: `clock fade` (clockFade)

Status: specified (2026-11-23)

Specified against `dsl/parser_clock_fade.go`, `engine/family_clock_fade.go`,
the `*clock-fade*` parse and run conformance cases and the `cf-*` semantic
case.

Classification: **go-only**. The Go engine is the only execution authority.
Browser and app consumers run the family through the Go engine (WASM or
native); the legacy JavaScript runtime refuses it.

Part of `spec/dsl-spec.md` §9.

## Purpose

Clock fade trades against the trailing move into one New York wall-clock
minute. At the close of the bar that opens at that minute, it compares the
close with the close exactly `N` bars earlier:

```text
move = close[i] - close[i - N]
```

It enters against the move (short after a rise, long after a fall) and leaves
after a fixed number of bars or at the stop. A zero move, a missing price, or
a window that is not `N` equally spaced bars (a session gap inside it) gives no
signal. Only bars up to `i` are read.

The research behind it is `strategies/evidence/gold-ny-close-giveback-2026-11-22.md`
in the heisentick app: gold gives back part of the move into 16:00 New York in
both directions. That note finds the net edge about zero after a one-minute
fill delay at today's spread, so the family is a research tool, not a
tradable claim.

## Phrases

All family phrases are written in the `setup` section; the same phrase shape
in another section is an error.

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `type: clock fade` | Selects the family. This exact spelling is the only type phrase. | Not selected unless declared. | `setupType = "clockFade"`, initializes `clockFade` |
| `lookback N candles` | Positive integer trailing-move length in bars, at most 100000. | Required; no default. | `clockFade.lookbackBars` |
| `signal at new york HH:MM` | Wall-clock open time of the signal bar in America/New_York (24-hour, strict `HH:MM`, DST-aware). The decision is taken at that bar's close. | Required; no default. | `clockFade.newYorkMinute` (minutes after midnight) |

Other phrases:

- `stop N ATR` is the only stop shape. Default 3. The stop is `N x ATR` from
  the signal close, on the losing side.
- `maxHoldCandles N` is the time exit, positive integer, default 1. With the
  default close fill, the position opens at the signal bar's close and closes
  at the close of the bar `N` bars later. `signal at new york 15:55` on 5m
  bars therefore enters at 16:00:00 and leaves at 16:05:00 with `N = 1`.
- `side long only` / `side short only` and `risk N USD` keep their shared
  meaning. `costs.fillOn` selects the fill as for other families.

There is no target. Shared session, day-type, movement, clock, higher-timeframe
and `new york hour` gates, `target`, `trail`, `breakeven`, `partial`, `wait`,
`enter` phrases and source timeframe execution are refused: the family owns
its time gate and its exit.

## Example

```dsl
dsl v7
strategy "Gold NY Close Fade" {
  description "Fade the 15 minute move into 16:00 New York."
}

market conditions {
  slices(XAUUSD 5m)
}

setup {
  type: clock fade
  lookback 3 candles
  signal at new york 15:55
}

risk {
  stop 3 ATR
}

management {
  maxHoldCandles 1
}

execution {
  risk 200 USD
}
```
