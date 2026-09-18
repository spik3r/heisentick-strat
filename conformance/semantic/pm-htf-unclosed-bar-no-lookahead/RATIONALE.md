# pm-htf-unclosed-bar-no-lookahead

Semantic under test: the higher-timeframe guard reads only completed HTF
candles. A signal inside a forming 4h candle is judged by the last closed 4h
candle, even when the forming one already points the other way.

## Spec text

`strat/docs/dsl-spec.md` §10 "Evaluation order and causality":

> Higher-timeframe values must come from *completed* HTF candles (no
> forward-fill of a forming candle).

`strat/docs/dsl-spec.md` §6: "`higher timeframe must agree` | `higher
timeframe must not oppose entry` | `higher timeframe off` — HTF gate (mode
notAgainst/off; default off). Accepted shorthands are … `higher timeframe
<timeframe> must agree` … Supported timeframe tokens include `4h`".

`strat/docs/dsl-spec-families/priceMomentum.md`, "Shared risk, management, and
guard phrases":

> `higher timeframe must not oppose entry` is the family-specific HTF guard.
> Missing HTF context fails closed, an opposing completed direction rejects
> the entry, and a genuinely flat completed HTF state permits either side.

## Strategy

As `pm-signal-close-entry` (with `trade window unrestricted`) plus
`higher timeframe 4h must not oppose entry` in `filters`.

## Bars (XAUUSD 1h, T0 = 2026-01-05T00:00Z; 4h candles start at T0 + 4j h)

Bars 0-31 flat at 2000 → 4h candles #0-#7 are flat (open 2000, high 2005,
low 1995, close 2000).

| k | 4h # | open | high | low | close | ROC vs k−2 | signal |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 32 | 8 | 2000 | 2000 | 1990 | 1995 | −0.25% | - |
| 33 | 8 | 1995 | 1997 | 1987 | 1990 | −0.50% | - |
| 34 | 8 | 1990 | 1992 | 1982 | 1985 | −0.50% | - |
| 35 | 8 | 1985 | 1987 | 1977 | 1980 | −0.50% | - |
| 36 | 9 | 1980 | 1990 | 1980 | 1990 | +0.25% | - |
| 37 | 9 | 1990 | 2000 | 1990 | 2000 | **+1.01%** | long, **rejected** (last closed 4h = #8, bearish) |
| 38 | 9 | 2000 | 2005 | 1995 | 2000 | +0.50% | - |
| 39 | 9 | 2000 | 2005 | 1995 | 2000 | 0 | - |
| 40 | 10 | 2000 | 2002 | 1992 | 1995 | −0.25% | - |
| 41 | 10 | 1995 | 2005 | 1995 | 2005 | +0.25% | - |
| 42 | 10 | 2005 | 2015 | 2005 | 2015 | **+1.0025%** | long, **admitted** (last closed 4h = #9, bullish) |
| 43 | 10 | 2015 | 2025 | 2015 | 2025 | | position open |
| 44 | 11 | 2025 | 2035 | 2025 | 2035 | | |
| 45 | 11 | 2035 | 2045 | 2035 | 2045 | | high reaches 2040.5 |
| 46 | 11 | 2045 | 2050 | 2040 | 2045 | | |
| 47 | 11 | 2045 | 2050 | 2040 | 2045 | | |

Completed 4h candles (`htfBars`, aggregated from the rows above):

| # | opens | open | high | low | close | direction (any reading) |
| --- | --- | --- | --- | --- | --- | --- |
| 7 | T0+28h | 2000 | 2005 | 1995 | 2000 | flat |
| 8 | T0+32h | 2000 | 2000 | 1977 | 1980 | **down**: close < open, close < prior close, lower high, lower low |
| 9 | T0+36h | 1980 | 2005 | 1980 | 2000 | **up**: close > open, close > prior close, higher high, higher low |
| 10 | T0+40h | 2000 | 2025 | 1992 | 2025 | up |
| 11 | T0+44h | 2025 | 2050 | 2025 | 2045 | up |

Candle #9 closes at T0 + 40h. Bars 36-39 lie inside it; bars 40-43 lie
inside #10.

## Derivation

1. Bar 37 (T0 + 37h): 2000 / 1980 = +1.01% → long signal. The last completed
   4h candle at that decision is #8 (closed at T0 + 36h), which is bearish
   under every direction reading → "an opposing completed direction rejects
   the entry". No trade. The forming candle #9 (open 1980, currently 2000)
   would permit the long; consulting it is the look-ahead this case forbids.
2. Bars 38-41 produce no signal (ROC ≤ 0.5%).
3. Bar 42 (T0 + 42h): 2015 / 1995 = +1.0025% → long signal. The last
   completed 4h candle is #9 (closed at T0 + 40h, two hours before bar 42
   opens, so availability is unambiguous), bullish under every reading →
   does not oppose → admitted. Entry at the close, 2015.
4. Stop: lows of bars 40-42 (1992, 1995, 2005) or 39-41 (1995, 1992, 1995)
   → 1992. Stop = 1992 − 2.5 = 1989.5; distance 25.5 (2.55 ATR, in bounds);
   target 2040.5. True ranges are 10 on every bar, so ATR = 10.
5. Bar 45 high 2045 ≥ 2040.5 → exit `tp` at 2040.5.

Expected: one long trade, 42 → 45, entry 2015, initialSl 1989.5, initialTp
2040.5, exit 2040.5, reason `tp`.

Traps: a look-ahead engine enters at bar 37 (2000). An engine that reads #9
only from a decision strictly later than T0 + 41h still admits bar 42.

## Spec gaps and assumptions

- SPEC GAP: "direction" of a completed HTF candle is not defined (single
  candle close vs open? close vs a prior close? a multi-candle bias with a
  threshold?). The fixture makes #8 bearish and #9 bullish under all
  single-candle and two-candle readings. A reading that needs a long HTF
  history (for example a 24-candle bias) fails closed on this 12-candle
  series and reports no trade; that reading must be written into the spec
  before this case can be marked derived.
- SPEC GAP: the exact availability instant of a completed HTF candle
  (decision close ≥ HTF close vs strictly after). The admitted signal is two
  hours clear of the boundary so both readings agree.
