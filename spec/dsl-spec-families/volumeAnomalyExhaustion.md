# Setup family: `volume anomaly exhaustion` (volumeAnomalyExhaustion)

Status: specified (2026-07-03)

Specified against `engine/dsl/setups/volumeAnomalyExhaustion.js`,
`strat/implementations/browser-runtime/compiler/parseSetups.js`, `engine/dsl/spec/setupParams.js`,
`test/engine.test.mjs`, and
`strat/conformance/parse/setup-volume-anomaly-exhaustion.strat` plus
`strat/conformance/parse/setup-volume-anomaly-exhaustion.cfg.json`.

Part of `strat/docs/dsl-spec.md` §9; authoring rules and the claim tracker live in
`plans/COMPLETED.md` (spec-families closeout).

## Type phrase

```
setup {
  type: volume anomaly exhaustion
}
```

## Purpose

Volume anomaly exhaustion looks for stopping or topping activity: unusually
high tick volume with either a narrow spread or a rejection wick near a
priority level, followed by a reclaim of that level. It then targets VWAP when
VWAP offers enough reward, otherwise it falls back to a fixed R target.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `type: volume anomaly exhaustion` | Selects the volume-anomaly exhaustion family and applies its family defaults. | Not selected unless this type is declared. Accepted aliases include `vpa exhaustion` and `volumeAnomalyExhaustion`. | `setupType = "volumeAnomalyExhaustion"`, default symbols/timeframes/sessions when still unset, resets `stop`, `breakeven`, `cooldownCandles`, `maxHoldCandles`, `target.vaeR`, `target.minR`, level defaults, initializes `volumeAnomalyExhaustion` |
| `volume anomaly ratio at least X` | Minimum `volumeRatio20` for an anomaly candle. | 1.8. | `volumeAnomalyExhaustion.volumeRatioMin` |
| `high effort narrow spread max X ATR` | Maximum spread ATR that qualifies the anomaly as high-effort/narrow-spread. | 0.65 ATR. | `volumeAnomalyExhaustion.maxSpreadAtr` |
| `or rejection wick at least X` or `rejection wick at least X` | Minimum directional rejection wick that can qualify the anomaly. | 0.45. | `volumeAnomalyExhaustion.rejectionWickMin` |
| `anomaly lookback N candles` | Number of recent candles, including the current candle, searched for the qualifying anomaly. | 3 candles. | `volumeAnomalyExhaustion.anomalyLookbackCandles` |
| `target vwap else NR` | Uses VWAP when it is on the profitable side and pays at least the minimum reward; otherwise uses the fixed R value. | `target vwap else 1R`. | `volumeAnomalyExhaustion.targetMode = "vwapElseFixedR"`, `target.vaeR` when an R token is present |
| `target NR` | Uses a fixed R target only. | Parser fallback is 1R. | `volumeAnomalyExhaustion.targetMode = "fixedR"`, `target.vaeR` |

## Defaults and interactions

The type phrase defaults symbols to `XAUUSD` if none were already set, replaces
the default timeframe list with `5m, 15m` if it was still the base default, and
sets sessions to London and NY only when the author has not already set
sessions. Explicit `sessions(...)` or deprecated `windows(...)` values win
regardless of whether they appear before or after the type phrase. It also
sets stop defaults to the last 5-candle extreme plus 0.25 ATR, min stop 0.4
ATR, max stop 2 ATR, breakeven after 0.5R plus 0.05 ATR, cooldown 8 candles,
max hold 24 candles, fixed fallback target 1R, and minimum VWAP target reward
0.5R.

Unless `priority(...)` was already explicit, runtime level priority is
`CAM_R4 CAM_R3 CAM_S3 CAM_S4 PDH PDL VWAP`, with level distance 0.6 ATR when
the shared distance was still at its default. Short setups use upper levels,
long setups use lower levels, and VWAP is side-aware. The entry candle must
reclaim the selected level, optional shared `close location` must pass, and an
anomaly whose only qualifier was a rejection wick also requires the signal bar
to meet the rejection threshold. Each side/level/session-phase can fire once
per day.

## Example

```dsl
dsl v7
strategy "Volume Anomaly Exhaustion Spec Example" {
  description "Fade a high-volume level reclaim back toward VWAP."
}

market conditions {
  symbols(XAUUSD)
  timeframes(5m, 15m)
  sessions(london, ny)
  day type in (ranging, choppy)
}

levels {
  priority(VWAP)
  nearKeyLevel withinAtr 0.6
}

setup {
  type: volume anomaly exhaustion
  volume anomaly ratio at least 1.8
  high effort narrow spread max 0.65 ATR
  or rejection wick at least 0.45
  anomaly lookback 3 candles
  target vwap else 1R
}

filters {
  close location at least 0.5
}

risk {
  stop beyond last 5 candle extreme by 0.25 ATR
  stop size min 0.4 max 2
}

target {
  minimum reward 0.5R
}

management {
  move stop to breakeven after 0.5R plus 0.05 ATR
  maxHoldCandles 24
  wait 8 candles after trade
}

execution {
  risk 200 USD
}
```
