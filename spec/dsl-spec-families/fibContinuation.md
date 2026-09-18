# Setup family: `fib continuation` (fibContinuation)

Status: specified (2026-07-03)

Part of `strat/docs/dsl-spec.md` §9; authoring rules and the claim tracker live in
`plans/COMPLETED.md` (spec-families closeout).

## Type phrase

```
setup {
  type: fib continuation
}
```

## Purpose

Fib continuation trades a pullback after a directional impulse. The setup
finds the most recent impulse leg, waits for price to retrace into the
configured fib-depth zone, and enters with directional confirmation for a
continuation target.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `impulse at least X ATR` | Require the impulse leg to travel at least `X` ATR. | `2` | `fibContinuation.impulseAtr` |
| `impulse at least X ATR within N candles` | Also limit the impulse search lookback to `N` candles. `over` and `lookback` are accepted instead of `within`. | `2 ATR within 24 candles` | `fibContinuation.impulseAtr`, `fibContinuation.impulseLookbackBars` |
| `retrace into A to B` | Require the pullback depth to be between fib ratios `A` and `B`. | `0.5 to 0.618` | `fibContinuation.retraceMin`, `fibContinuation.retraceMax` |
| `pullback into A to B` | Alias for `retrace into A to B`. | `0.5 to 0.618` | `fibContinuation.retraceMin`, `fibContinuation.retraceMax` |
| `confirm with close location X` | Require the signal candle's close-location score to be at least `X` in the trade direction. | `0.6` | `fibContinuation.confirmCloseLocation` |

## Defaults and interactions

`type: fib continuation` enables EMA context length `21` and rebases shared
defaults to a fib-retrace stop at `0.786` plus `0.25 ATR`, min stop `0.4 ATR`,
max stop `3 ATR`, target `2R` with fib extension `1.618`, breakeven after
`0.5R` plus `0.05 ATR`, and a 12-candle cooldown. Runtime params default to
Asia and London enabled, NY disabled, both sides, and HTF bias enabled unless
the shared higher-timeframe directive turns it off.

The long side finds the lowest low, then the highest high after it, inside the
lookback; the short side does the mirror image. The pullback depth is computed
from the impulse end to the pullback extreme. Trigger candles are on by default
for this family: a directional pin, engulf, outside bar, or same-direction
candle can confirm when close location passes. Shared `trigger candle in (any)`
sets `useTrigger` off. `stop beyond <ratio> retrace [by X]` changes the fib
stop ratio and padding; `target fib extension <ratio>` changes the extension.
The runtime uses the farther continuation objective between the R target and
the fib-extension target.

## Example

```dsl
dsl v7
strategy "Fib Continuation Spec Example" {
  description "Documented impulse, retrace, and continuation example."
}

market conditions {
  slices(XAUUSD 15m, XAUUSD 1h)
  sessions(asia, london)
  day type in (trending, ranging, choppy)
}

setup {
  type: fib continuation
  impulse at least 2 ATR within 24 candles
  retrace into 0.5 to 0.618
  confirm with close location 0.6
}

filters {
  higher timeframe must agree
  side both
}

risk {
  stop beyond 0.786 retrace by 0.25 ATR
  stop size min 0.4 max 3
}

target {
  target fib extension 1.618
}

management {
  move stop to breakeven after 0.5 R plus 0.05 ATR
  maxHoldCandles 24
  wait 12 candles after trade
}

execution {
  risk 200 USD
}
```
