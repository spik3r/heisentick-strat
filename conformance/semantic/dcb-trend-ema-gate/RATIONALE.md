# dcb-trend-ema-gate

Semantic under test: with `trend ema N`, a long breakout also needs the close
above EMA(N) of closes, seeded with the first close of the series.

## Spec text

`spec/dsl-spec-families/donchianBreakout.md`, "Phrases": `trend ema N` —
"Long needs `close > EMA`". "The EMA uses `alpha = 2 / (N + 1)`, is seeded
with the first close of the run's bar series and updates on every bar".

## Derivation

- Bars 0-24: o 2100, h 2105, l 2095, c 2100. EMA(10) = 2100.
- Bar 25: o 2100, h 2100, l 1995, c 2000. EMA = 2100 - (2/11) x 100.
- Bars 26-29: o 2000, h 2005, l 1995, c 2000. After the five closes at 2000
  (bars 25-29) the EMA is 2000 + 100 x (9/11)^5 = 2036.66.
- Bar 30: o 2000, h 2010, l 2000, c 2010. Channel high (bars 27-29) 2005, so
  the close breaks out. EMA = 2036.66 + (2/11) x (2010 - 2036.66) = 2031.82.
  2010 > 2031.82 is false: the gate blocks the entry. Warm-up is
  max(3, 10) = 10 bars, long passed.
- Bars 31-34: o 2010, h 2015, l 2005, c 2010. Channel highs 2010 and then
  2015; no close is above them.
- Expected trades: none.

Status: derived.
