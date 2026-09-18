# Setup family: `intra hour run exhaustion` (intraHourRunExhaustion)

Status: specified (2026-08-30)

Specified against `engine/dsl/setups/intraHourRunExhaustion.js`,
`strat/implementations/browser-runtime/compiler/parseSetups.js`, `engine/dsl/spec/setupParams/intraHourRunExhaustion.js`,
the Go parser/runtime equivalents (`strat/implementations/server-runtime/parser_intra_hour_run_exhaustion.go`,
`go/native/family_intra_hour_run_exhaustion.go`), and the
`setup-intra-hour-run-exhaustion` / `family-intra-hour-run-exhaustion`
conformance cases.

Part of `strat/docs/dsl-spec.md` §9.

## Purpose

The family reads a clock hour on a timeframe that divides it. The first
`run N candles` candles of the hour must close in one direction; the hour's
final candle must then close back inside its own range against that run. The
entry is taken at the close of that final candle — which is also the hour
boundary — and fades the run.

Nothing after the signal candle is visible: the run, its extreme, and the ATR
are all read from candles that have already closed.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `type: intra hour run exhaustion` | Selects the family. `intraHourRunExhaustion` is also accepted. | Not selected unless declared. | `setupType = "intraHourRunExhaustion"`, initializes `intraHourRunExhaustion`, sets `target.ihreR = 1.5` |
| `run hour candles N` | Candles per clock hour. `4` is a 15m route, `2` a 30m route. | 4. | `intraHourRunExhaustion.hourCandles` |
| `run N candles` | Same-direction candles required before the hour's final candle. Must be below `hour candles`. | 3. | `intraHourRunExhaustion.runCandles` |
| `run size min X ATR` | Minimum run size, first run open to last run close, in ATR. | 0.8. | `intraHourRunExhaustion.minRunAtr` |
| `run atr length N` | ATR lookback used for the run-size gate, the stop pad, and diagnostics. | 14. | `intraHourRunExhaustion.atrLen` |
| `exhaustion close location X percent` | How far back inside its own range the final candle must close, read from the run's side. Must be above 0 and below 50. | 35 percent. | `intraHourRunExhaustion.exhaustLocationPct` |
| `exhaustion require poke` | Also require the final candle to trade beyond the run extreme before closing back. | Off. | `intraHourRunExhaustion.requirePoke` |
| `exhaustion stop pad X ATR` | Stop distance beyond the hour extreme. | 0.25. | `intraHourRunExhaustion.stopPadAtr`, `stop.paddingAtr` |

A run is "steady" only when every candle closes beyond its own open *and*
beyond the previous candle's close. A hour whose candles are not contiguous at
the timeframe's spacing is skipped rather than patched.

## Shared risk, management, and guard phrases

The type phrase sets a structure stop with 0.25 ATR padding, no breakeven, a
4-candle cooldown, a 8-candle max hold, and `target.ihreR = 1.5`. `target N R`
writes `ihreR`; `maxHoldCandles N` and `wait N candles after trade` apply
normally. The stop is placed beyond the whole hour's extreme (the run extreme
or the final candle's own extreme, whichever is further), so the stop-size
bounds do not apply.

## Example

```dsl
dsl v7
strategy "Intra Hour Run Exhaustion Spec Example" {
  description "Fade a steady three-candle intra-hour run rejected on the hour's final candle."
}

market conditions {
  slices(XAUUSD 15m)
  sessions(asia, london, ny)
  day type in (trending, ranging, choppy)
}

setup {
  type: intra hour run exhaustion
  run hour candles 4
  run 3 candles
  run size min 0.8 ATR
  run atr length 14
  exhaustion close location 35 percent
  exhaustion stop pad 0.25 ATR
}

target {
  target 1.5R
}

management {
  maxHoldCandles 8
  wait 4 candles after trade
}

execution {
  risk 200 USD
}
```

## Measured result

The shape was measured before the family was written and does not pay: see
`strategies/evidence/intra-hour-run-exhaustion-2026-08-30.md`. The family
exists so the hypothesis is expressible and re-testable, not because a route
using it is deployable.
