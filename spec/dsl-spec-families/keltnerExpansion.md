# Setup family: `keltner expansion` (keltnerExpansion)

Status: specified (2026-08-14)

Specified against `engine/dsl/setups/keltnerReversion.js`,
`strat/implementations/browser-runtime/compiler/parseSetups/keltnerPhrases.js`,
`engine/dsl/spec/setupParams.js`, the Go parser equivalents, and the
keltner-expansion parse conformance case.

Part of `strat/docs/dsl-spec.md` §9.

## Purpose

Keltner expansion is the continuation reading of the channel described in
`keltnerReversion.md`. Instead of fading a push through a band edge, it trades
with the push — but only after the move *holds* outside the band for a second
consecutive close. Breakout entries without a hold are a documented negative
result in this repository, so the hold is a required part of the family rather
than an option.

**Measured result: this family does not produce an edge on the routes it has
been tried on.** See
`strategies/evidence/keltner-band-reversion-intraday-2026-08-14.md`. The
family is specified and available; it is not a candidate.

## Type phrase

```
setup {
  type: keltner expansion
}
```

## Phrases

Identical to `keltner reversion` — the two families share one phrase parser and
one config key (`keltnerReversion`). See
[`keltnerReversion.md`](keltnerReversion.md) for the phrase table. `reclaim
within N candles` is accepted but does not affect expansion entries, which
always read the two most recent bars.

## Entry, stop, and target

A long fires when all of the following hold on a completed bar:

- the previous bar closed above its own upper band, and its high reached at
  least `upper + minPierceAtr * ATR` as measured on that bar;
- the current bar also closes above the current upper band — the hold;
- the side is permitted, and the HTF guard permits it when enabled.

Shorts are the mirror. The stop goes back inside the channel, at the current
band edge offset by the shared `paddingAtr` and clamped by the shared stop
bounds; the band edge is the level whose loss invalidates the expansion. The
target is a fixed `targetR` multiple of that risk — an expansion has no midline
magnet to aim at.

## Defaults and interactions

Same defaults as `keltner reversion`, and the same shared-phrase overrides.

## Example

```dsl
dsl v7
strategy "Keltner Band Expansion Hold" {
  description "Continuation after a Keltner band pierce holds outside the channel."
}

market conditions {
  slices(XAUUSD 15m)
}

setup {
  type: keltner expansion
  ema length 20
  distance 2 ATR
  pierce at least 0.25 ATR
  target 1R
}

execution {
  risk: 200 USD
}
```
