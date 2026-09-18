# Setup family: `vwap extension fade` (vwapExtensionFade)

Status: specified (2026-07-03)

Specified against `engine/dsl/setups/vwapExtensionFade.js`,
`strat/implementations/browser-runtime/compiler/parseSetups.js`, `engine/dsl/spec/setupParams.js`,
`test/engine.test.mjs`, and
`strat/conformance/parse/setup-vwap-extension-fade.strat` plus
`strat/conformance/parse/setup-vwap-extension-fade.cfg.json`.

Part of `strat/docs/dsl-spec.md` §9; authoring rules and the claim tracker live in
`plans/COMPLETED.md` (spec-families closeout).

## Type phrase

```
setup {
  type: vwap extension fade
}
```

## Purpose

VWAP extension fade trades mean reversion after price stretches far enough away
from VWAP, usually late in a range or choppy session. The setup enters back
toward VWAP when candle quality, session, day-shape, stop, and reward checks
all pass.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `type: vwap extension fade` | Selects the VWAP extension fade family and applies its family defaults. | Not selected unless this type is declared. `vwapExtensionFade` is also accepted. | `setupType = "vwapExtensionFade"`, resets `stop`, `breakeven`, `cooldownCandles`, `maxHoldCandles`, `target.vefR`, `target.minR`, initializes `vwapExtensionFade` |
| `distance from vwap at least X ATR` | Minimum signed VWAP distance: shorts require price at least X ATR above VWAP, longs require price at least X ATR below VWAP. | 1.2 ATR. | `vwapExtensionFade.minDistanceAtr` |
| `target vwap` | Prefer VWAP as the target. If VWAP is invalid, on the wrong side of entry, or below minimum reward, runtime falls back to the fixed R target. | Runtime default is VWAP. | `vwapExtensionFade.targetMode = "vwap"` |
| `target NR` | Use a fixed R target instead of VWAP. | Shared type default sets `target.vefR = 1`; parser fallback is 1R. | `vwapExtensionFade.targetMode = "fixedR"`, `vwapExtensionFade.targetR` |
| `require range day` | Requires current regime to be ranging or choppy. | On. | `vwapExtensionFade.requireRangeDay = 1` |
| `require range day off` or `range day off` | Disables the ranging/choppy regime requirement. | Off only when explicitly disabled. | `vwapExtensionFade.requireRangeDay = 0` |

## Defaults and interactions

The type phrase sets stop defaults to the last 5-candle extreme plus 0.25 ATR,
min stop 0.4 ATR, max stop 2 ATR, breakeven after 0.5R plus 0.05 ATR, cooldown
12 candles, max hold 24 candles, fixed fallback target 1R, and minimum reward
0.4R. Runtime defaults also include session phases `lunch` and `close`, no
Asia session, London and NY enabled, tail rejection 0.35, target mode `vwap`,
and both sides allowed.

Shared `session phase in (...)` overrides the runtime phase list. Shared
`tail rejection at least X` and `close location at least X` feed the candle
quality check. Shared `target { target NR }` sets `target.vefR`, which is used
as the fallback R multiple; the setup-local `target NR` phrase instead changes
the VWAP-fade target mode to fixed R. Each side/session/VWAP zone can only fire
once per day.

## Example

```dsl
dsl v7
strategy "VWAP Extension Fade Spec Example" {
  description "Fade a late-session extension back toward VWAP."
}

market conditions {
  symbols(XAUUSD)
  timeframes(15m)
  sessions(london, ny)
  session phase in (close)
  day type in (ranging, choppy) strictly
}

setup {
  type: vwap extension fade
  distance from vwap at least 1.2 ATR
  require range day
  target vwap
}

filters {
  tail rejection at least 0.35
  close location at least 0.55
}

risk {
  stop beyond last 5 candle extreme by 0.25 ATR
  stop size max 2
}

target {
  target 1R
  minimum reward 0.4R
}

management {
  move stop to breakeven after 0.5R plus 0.05 ATR
  maxHoldCandles 24
  wait 12 candles after trade
}

execution {
  risk 200 USD
}
```
