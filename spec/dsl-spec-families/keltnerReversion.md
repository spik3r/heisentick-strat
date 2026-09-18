# Setup family: `keltner reversion` (keltnerReversion)

Status: specified (2026-08-14)

Specified against `engine/dsl/setups/keltnerReversion.js`,
`strat/implementations/browser-runtime/compiler/parseSetups/keltnerPhrases.js`,
`engine/dsl/spec/setupParams.js`, the Go parser equivalents, and the
keltner-reversion parse conformance case.

Part of `strat/docs/dsl-spec.md` §9.

## Purpose

A Keltner channel is an EMA of the close with bands set a fixed number of ATR
either side:

```text
midline = EMA(close, emaLen)
upper   = midline + bandAtr * ATR
lower   = midline - bandAtr * ATR
```

Keltner reversion trades price back toward the midline after price has pushed
through a band edge and then closed back inside it. The reclaim is the point:
entering while price is still outside the band is a naked fade, and this
repository's fade results are consistently negative. The target is the midline
when it pays enough, otherwise a fixed R.

Both the EMA and the channel are folded forward one completed bar at a time.
Each bar in the reclaim window is judged against the band that existed on that
bar, never against the current one, so no future bar is read.

**Measured result: this family does not produce an edge on the routes it has
been tried on.** See
`strategies/evidence/keltner-band-reversion-intraday-2026-08-14.md`. The
family is specified and available; it is not a candidate.

## Type phrase

```
setup {
  type: keltner reversion
}
```

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `type: keltner reversion` | Selects the family and applies its defaults. Accepts the `keltnerReversion` spelling. | Not selected unless declared. | `setupType = "keltnerReversion"`, timeframes default to `15m` when still at the base default, stop/breakeven/cooldown/max-hold/target defaults, initializes `keltnerReversion` |
| `ema length N` | Midline EMA length. Integer greater than 1. | 20. | `keltnerReversion.emaLen` |
| `distance X ATR` | Band half-width in ATR. Finite and positive. | 2. | `keltnerReversion.bandAtr` |
| `pierce at least X ATR` | How far beyond the band edge a bar must trade to qualify as a stretch. Finite and non-negative. | 0.25. | `keltnerReversion.minPierceAtr` |
| `reclaim within N candles` | Length of the window searched for the qualifying stretch, including the current candle. Positive integer. | 3. | `keltnerReversion.reclaimCandles` |
| `target midline else NR` | Targets the midline when it pays at least the minimum reward, otherwise the fixed R value. | `target midline else 1R`. | `keltnerReversion.targetMode = "midlineElseFixedR"`, `keltnerReversion.targetR` |
| `target NR` | Fixed R target only. | — | `keltnerReversion.targetMode = "fixedR"`, `keltnerReversion.targetR` |

## Entry, stop, and target

A long fires when all of the following hold on a completed bar:

- some bar in the reclaim window traded at or below `lower - minPierceAtr * ATR`
  as measured on that bar;
- the current bar closes back above the current `lower`;
- the current close is still below the midline, so the reversion has room to pay;
- the side is permitted, and the HTF guard permits it when enabled.

Shorts are the mirror. The stop goes beyond the stretch extreme by the shared
`paddingAtr`, clamped by the shared `minAtr` / `maxAtr` stop bounds. The target
is the midline when `midline - entry >= minR * risk` (mirrored for shorts),
otherwise `entry + targetR * risk`.

## Defaults and interactions

The type phrase sets the timeframe list to `15m` when it was still the base
default, a stop of the stretch extreme plus 0.25 ATR with 0.4–2 ATR bounds,
breakeven after 0.5R plus 0.05 ATR, a 12-candle cooldown, a 24-candle max hold,
a 1R fallback target, and a 0.5R minimum midline reward.

Shared stop, target, breakeven, partial, trail, max-hold, fixed-risk, session,
side, and cooldown phrases override those values. The family-specific HTF guard
is the shared `higher timeframe must not oppose entry`.

## Example

```dsl
dsl v7
strategy "Keltner Band Reclaim Reversion" {
  description "Reversion to the Keltner midline after a band pierce is reclaimed."
}

market conditions {
  slices(XAUUSD 15m)
}

setup {
  type: keltner reversion
  ema length 20
  distance 2 ATR
  pierce at least 0.25 ATR
  reclaim within 3 candles
  target midline else 1R
}

execution {
  risk: 200 USD
}
```
