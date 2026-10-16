# Setup family: `price momentum` (priceMomentum)

Status: specified (2026-07-31)

Specified against `engine/dsl/setups/priceMomentum.js`,
`strat/implementations/browser-runtime/compiler/parseSetups/momentumPhrases.js`,
`engine/dsl/spec/setupParams.js`, the Go parser/runtime equivalents, and the
price-momentum parse/run conformance cases.

Part of `strat/docs/dsl-spec.md` §9.

## Purpose

Price momentum is causal, single-symbol time-series momentum. At the close of
bar `i`, it compares that close with the close exactly `N` bars earlier:

```text
rocPct = 100 * (close[i] / close[i - N] - 1)
```

It signals long at or above the positive threshold, short at or below the
negative threshold, and stays neutral between them. It does not rank symbols.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `type: price momentum` | Selects the family. This exact spelling is the only type phrase. | Not selected unless declared. | `setupType = "priceMomentum"`, initializes `priceMomentum` |
| `lookback N candles` | Positive integer close-to-close ROC lookback. | Required; no default. | `priceMomentum.lookbackBars` |
| `neutral zone X percent` | Finite, strictly positive symmetric threshold. | Required; no default. | `priceMomentum.thresholdPct` |

The signal is unavailable before `N + 1` closes. A missing, non-finite, or
non-positive denominator fails closed.

## Shared risk, management, and guard phrases

The type starts with a recent-extreme stop over 3 candles plus 0.25 ATR,
0.5–3 ATR stop bounds, a 1R target, breakeven after 0.5R plus 0.05 ATR, and a
12-candle entry-relative cooldown. Shared stop, target, breakeven, partial,
trail, max-hold, fixed-risk, session, side, and cooldown phrases override those
values without adding family-specific money rules.

`stop N ATR` replaces the recent-extreme stop with a fixed stop `N × ATR` from
the signal close (below it for longs, above it for shorts). The stop padding
and extreme lookback are ignored. The stop-size bounds still apply, so an `N`
outside them rejects every entry. They are compared with `N` itself and are
inclusive: `stop 2 ATR` with `stop size min 2` trades. The bound is not
re-derived from the rounded stop price. The computed stop must also be finite
and on the losing side of the signal close. For stops parsed as `fixedAtr`,
`N` must be finite and positive. `stop 0 ATR`, `stop -1 ATR`, `stop NaN ATR`,
`stop Inf ATR` and `stop -Inf ATR` are parse errors; direct price-momentum
`fixedAtr` configurations also reject nonpositive, non-finite, missing or
nonnumeric multiples. A pre-existing lexer limitation remains: `stop +Inf ATR`
and out-of-range numeric literals can enter the generic stop branch instead,
so this change does not guarantee rejection of those spellings.
The stop stays anchored to the signal close with either fill model; with a
next-open fill, size uses the distance from the slipped fill to that stop.

`wait N candles after trade` measures from the accepted entry bar. Re-entry is
blocked while `currentBar - priorEntryBar < N` and is permitted at equality. A
trade held through that boundary has no residual wait after exit.

`higher timeframe must not oppose entry` is the family-specific HTF guard.
Missing HTF context fails closed, an opposing completed direction rejects the
entry, and a genuinely flat completed HTF state permits either side. A
completed candle is up when close > open, down when close < open, and flat when
close == open. Direction is available from the first completed HTF candle; it
does not use a multi-candle direction warm-up. Indicator warm-up is separate.
Forming candles and stale intraday projections fail closed.

## Example

```dsl
dsl v7
strategy "Price Momentum Spec Example" {
  description "Causal close-to-close momentum with a non-opposing HTF guard."
}

market conditions {
  slices(USDJPY 1h)
  sessions(asia, london, ny)
  day type in (trending, ranging, choppy)
}

setup {
  type: price momentum
  lookback 20 candles
  neutral zone 1 percent
}

filters {
  higher timeframe must not oppose entry
}

risk {
  stop beyond last 3 candle extreme by 0.25 ATR
  stop size min 0.5 max 3
}

target {
  target 1R
}

management {
  move stop to breakeven after 0.5R plus 0.05 ATR
  wait 12 candles after trade
}

execution {
  risk 200 USD
}
```
