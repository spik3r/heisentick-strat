# Setup family: `triple push exhaustion` (triplePushExhaustion)

Status: specified (2026-07-03)

Specified against `engine/dsl/setups/triplePushExhaustion.js`,
`strat/implementations/browser-runtime/compiler/parseSetups.js`, `engine/dsl/spec/setupParams.js`,
`test/engine.test.mjs`, and
`strat/conformance/parse/setup-triple-push-exhaustion.strat` plus
`strat/conformance/parse/setup-triple-push-exhaustion.cfg.json`.

Part of `strat/docs/dsl-spec.md` §9; authoring rules and the claim tracker live in
`plans/COMPLETED.md` (spec-families closeout).

## Type phrase

```
setup {
  type: triple push exhaustion
}
```

## Purpose

Triple push exhaustion fades a late third push to a fresh extreme after the
preceding pushes decelerate. Confirmed swing pivots are used causally, so the
setup waits for the pivot lag before evaluating the three-push structure.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `type: triple push exhaustion` | Selects the triple-push reversal family and applies its family defaults. | Not selected unless this type is declared. Accepted aliases include `three push exhaustion`, `wedge exhaustion`, and `triplePushExhaustion`. | `setupType = "triplePushExhaustion"`, resets `stop`, `breakeven`, `cooldownCandles`, `target.tpeR`, initializes `triplePush` |
| `lookback N candles` | Maximum bars back for the three confirmed pushes. | 60 bars. | `triplePush.lookbackBars` |
| `min separation N` or `separation N` | Minimum bars between consecutive pushes. | 5 bars. | `triplePush.minSeparation` |
| `push decay X` | Requires the third leg to be no larger than the second leg times X. | 1.0, meaning the third push must not exceed the second. | `triplePush.pushDecay` |
| `decel size` \| `decel slope` | Chooses whether deceleration compares leg size or price-per-bar slope. | Runtime default is `size`; examples may choose `slope`. Invalid modes are compile errors. | `triplePush.decelMode` |
| `measured move` \| `structure target` | Uses the pre-sequence base as the target when it pays at least 1R; otherwise falls back to the R target. | Off; target is an R multiple. | `triplePush.targetStructure = 1` |
| `near X ATR` or `near within X ATR` | Requires current price to remain within X ATR of the third-push extreme. | 1.5 ATR. | `triplePush.nearAtr` |
| `pivot N` | Bars on each side required to confirm a swing pivot. | 3 bars. | `triplePush.pivotK` |
| `confirm with close location X` | Minimum directional close-location score for the rejection candle. | 0.55. | `triplePush.confirmCloseLocation` |

## Defaults and interactions

The type phrase sets stop defaults to structure/pivot extremes with 0.25 ATR
padding, min stop 0.4 ATR, max stop 3.5 ATR, breakeven after 0.6R plus 0.05
ATR, cooldown 12 candles, and `target.tpeR = 2`. Runtime defaults include
`pivotK = 3`, `lookbackBars = 60`, `minSeparation = 5`, `pushDecay = 1`,
`decelMode = "size"`, `targetStructure = 0`, `nearAtr = 1.5`, and
`confirmCloseLocation = 0.55`.

The setup tries short first, then long, unless side filters disable a side. It
requires ATR, movement efficiency between `minEr` and `maxEr` when those are
set, three progressing confirmed pivots with opposite pullbacks between them,
price still near the third push, and a rejection candle unless trigger candles
are explicitly set to `any`. Higher-timeframe bias is off by default; when
enabled, this exhaustion family fades the HTF direction, so shorts want an up
bias and longs want a down bias. Shared management, `target NR`, `maxHoldCandles`,
`wait N candles`, and fixed-risk execution apply normally.

## Example

```dsl
dsl v7
strategy "Triple Push Exhaustion Spec Example" {
  description "Fade a decelerating third push after confirmed swing pivots."
}

market conditions {
  slices(XAUUSD 1h, XAUUSD 4h)
  sessions(asia, london, ny)
  day type in (trending, ranging, choppy)
}

setup {
  type: triple push exhaustion
  lookback 60 candles
  min separation 5
  push decay 0.9
  decel slope
  near 1.5 ATR
  confirm with close location 0.55
}

risk {
  stop size max 3.5
}

target {
  target 2R
}

management {
  move stop to breakeven after 0.6R plus 0.05 ATR
  maxHoldCandles 24
  wait 12 candles after trade
}

execution {
  risk 200 USD
}
```
