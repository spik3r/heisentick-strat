# pm-missing-candle

Semantic under test: a missing candle inside a session. Candle-count phrases
count the bars that exist; no synthetic bar is inserted and no lookback is
converted into wall-clock time.

## Spec text

`strat/docs/dsl-spec-families/priceMomentum.md`, "Purpose":

> At the close of bar `i`, it compares that close with the close exactly `N`
> bars earlier

and "Phrases": "`lookback N candles` — Positive integer close-to-close ROC
lookback."

`strat/docs/dsl-spec.md` §2: "**candle-count units** — author-facing count
phrases use `candle` or `candles`".

## Strategy and bars

Strategy identical to `pm-signal-close-entry`. Bars 0-29 are the same flat
warm-up (T0 + 0h … T0 + 29h). The candle at T0 + 30h is absent; the series
continues at T0 + 31h:

| index | time | open | high | low | close | ROC vs close[index−2] |
| --- | --- | --- | --- | --- | --- | --- |
| 29 | T0+29h | 2000 | 2005 | 1995 | 2000 | 0 |
| (none) | T0+30h | | | | | |
| 30 | T0+31h | 2000 | 2010 | 2000 | 2010 | +0.50% (vs index 28) |
| 31 | T0+32h | 2010 | 2020 | 2010 | 2020 | **+1.00% (vs index 29)** |
| 32 | T0+33h | 2020 | 2030 | 2020 | 2030 | |
| 33 | T0+34h | 2030 | 2040 | 2030 | 2040 | |
| 34 | T0+35h | 2040 | 2050 | 2040 | 2050 | high reaches 2047.5 |
| 35 | T0+36h | 2050 | 2055 | 2045 | 2050 | |
| 36 | T0+37h | 2050 | 2055 | 2045 | 2050 | |

## Derivation

1. "Exactly N bars earlier" is an index offset. Index 31 (T0 + 32h) closes
   2020; two bars earlier is index 29 (T0 + 29h, three hours earlier in
   wall-clock time), close 2000: +1.0% ≥ 0.6% → long signal at index 31.
2. Index 30 is +0.5% (2010 vs index 28's 2000): no signal.
3. Entry at the signal close: index 31, 2020, entryT = T0 + 32h.
4. Stop: lows of indices 29-31 (1995, 2000, 2010) or 28-30 (1995, 1995,
   2000) → 1995; stop 1992.5; distance 27.5; target 2047.5. True ranges are
   still 10 everywhere (index 30 opens at the index 29 close), so ATR = 10.
5. Index 34 (T0 + 35h) high 2050 reaches the target → exit `tp` at 2047.5.

Expected: one long trade, entryIndex 31 / entryT T0+32h, exitIndex 34 /
exitT T0+35h, entry 2020, exit 2047.5, reason `tp`.

Trap: a time-based reading ("the close two hours earlier") finds no candle
at T0 + 30h for index 31 and would skip it; its first signal would be index
32 (2030 vs index 30's 2010 = +0.995%), entry 2030.

## Spec gaps and assumptions

- SPEC GAP: the spec never says what a missing candle means for a
  candle-count phrase or for ATR. The reading here (count existing bars, no
  synthetic fill) follows from "exactly N bars earlier".
