# Setup family: `elder triple screen` (elderTripleScreen)

Status: specified (2026-07-03)

Specified against `engine/dsl/setups/elderTripleScreen.js`,
`strat/implementations/browser-runtime/compiler/parseSetups.js`, `engine/dsl/spec/setupParams.js`,
`test/engine.test.mjs`, and
`strat/conformance/parse/setup-elder-triple-screen.strat` plus
`strat/conformance/parse/setup-elder-triple-screen.cfg.json`.

Part of `strat/docs/dsl-spec.md` §9; authoring rules and the claim tracker live in
`plans/COMPLETED.md` (spec-families closeout).

## Type phrase

```
setup {
  type: elder triple screen
}
```

## Purpose

Elder Triple Screen is a multi-timeframe trend-following setup. It aligns with
higher-timeframe direction, waits for an entry-timeframe pullback to EMA, then
enters on a momentum trigger or EMA reclaim while controlling pullback depth.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `type: elder triple screen` | Selects the Elder Triple Screen family and applies its family defaults. | Not selected unless this type is declared. Accepted aliases include `triple screen` and `elderTripleScreen`. | `setupType = "elderTripleScreen"`, default symbol/timeframe when still unset, seeds `htf`, resets `stop`, `breakeven`, `cooldownCandles`, `maxHoldCandles`, initializes `elderTripleScreen` |
| `ema length N` | EMA length used for the pullback/reclaim and enabled EMA context. | 21. | `elderTripleScreen.emaLen`, `emaLen` |
| `pullback tag X` | Maximum ATR distance from EMA that counts as tagging the pullback. | 0.3 ATR. | `elderTripleScreen.pullbackAtr` |
| `pullback within N` | Number of recent bars searched for the EMA tag. | 8 bars. | `elderTripleScreen.pullbackWithin` |
| `pullback max depth X` | Maximum pullback depth as a fraction of the prior impulse range. | 0.6. | `elderTripleScreen.maxPullbackDepth` |
| `trigger off` \| `trigger none` \| `trigger disabled` | Disables the trigger-candle/EMA-reclaim requirement. Any other `trigger ...` phrase enables it. | On. | `elderTripleScreen.useTrigger` |
| `htf bias off` \| `htf trend off` | Disables the setup-local higher-timeframe bias requirement. Any other `htf bias ...` or `htf trend ...` phrase enables it. | On in runtime defaults. | `elderTripleScreen.useHtfBias` |

## Defaults and interactions

The type phrase defaults symbols to `XAUUSD` if none were already set and
replaces the default timeframe list with `1h` if it was still the base default.
It sets stops to the pullback extreme plus 0.3 ATR, min stop 0.5 ATR, max stop
3 ATR, breakeven after 1R plus 0.05 ATR, cooldown 8 candles, and max hold
20 candles. Runtime defaults include EMA length 21, EMA slope window 5, pullback
tag 0.3 ATR, pullback within 8 bars, max pullback depth 0.6, impulse lookback
12 bars, target 2R, trail after 1.5R by 1.5 ATR, higher-timeframe bias on, and
trigger on.

Shared `higher timeframe must agree` sets the core `htf.mode` to `notAgainst`,
but this family also has its own `elderTripleScreen.useHtfBias` default, so HTF
alignment remains on even when no shared HTF phrase is present. Shared
`target NR`, `move stop to breakeven ...`, `trail ...`, `wait N candles`, max
hold, side filters, and fixed-risk execution map through. Omitted trail values
keep the Elder defaults of 1.5 ATR after 1.5R.

## Example

```dsl
dsl v7
strategy "Elder Triple Screen Spec Example" {
  description "Follow the higher-timeframe trend after a pullback to EMA."
}

market conditions {
  slices(XAUUSD 1h)
  sessions(london, ny)
  day type in (trending, ranging)
}

setup {
  type: elder triple screen
  ema length 21
  pullback tag 0.3
  pullback within 8
  pullback max depth 0.6
}

filters {
  higher timeframe must agree
}

risk {
  stop beyond pullback extreme by 0.3 ATR min 0.5 ATR
}

target {
  target 2R
}

management {
  move stop to breakeven after 1R plus 0.05 ATR
  trail 1.5 ATR after 1.5R
  wait 20 candles after trade
}

execution {
  risk 200 USD
}
```
