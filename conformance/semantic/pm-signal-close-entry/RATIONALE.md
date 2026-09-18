# pm-signal-close-entry

Semantic under test: `enter at market` (the default) fills at the signal
bar's close with `fillOn: close`. This case is also the base for the other
`pm-*` cases, which change one bar or one option each.

## Spec text

`strat/docs/dsl-spec-families/priceMomentum.md`, "Purpose":

> At the close of bar `i`, it compares that close with the close exactly `N`
> bars earlier: `rocPct = 100 * (close[i] / close[i - N] - 1)`. It signals
> long at or above the positive threshold, short at or below the negative
> threshold, and stays neutral between them.

Same file, "Shared risk, management, and guard phrases":

> The type starts with a recent-extreme stop over 3 candles plus 0.25 ATR,
> 0.5–3 ATR stop bounds, a 1R target, breakeven after 0.5R plus 0.05 ATR, and
> a 12-candle entry-relative cooldown. Shared stop, target, breakeven, partial,
> trail, max-hold, fixed-risk, session, side, and cooldown phrases override
> those values

`strat/docs/dsl-spec.md` §6 "`filters`, `trigger`, `entry`":

> `enter at market` — enter on signal close (default).

`strat/docs/dsl-spec.md` §7 "`risk`, `target`, `management`, `execution`":

> `stop beyond last N candle extreme by X ATR` — stop beyond the recent
> extreme (defaults: 3 candles, 0.25 ATR padding).
> `stop size min A max B` — clamp the stop distance in ATR
> `target <N>R` — R-multiple target
> `move stop to breakeven after X R [plus Y ATR]` — breakeven move … `breakeven off` disables

`strat/docs/dsl-spec.md` §4: `trade window unrestricted` "removes the engine's
fixed UTC+10 trade-window gate"; `movement below X` caps movement efficiency;
`day type in (...)` with `trending, ranging, choppy` being the only day types
(`docs/strategy-dsl-context.md` grammar: `daytype = "ranging" | "choppy" | "trending"`).

## Strategy

- `lookback 2 candles`, `neutral zone 0.6 percent`, `side long only`.
- `stop beyond last 3 candle extreme by 0.25 ATR`, `stop size min 0.5 max 3`,
  `target 1R`, `breakeven off`, `wait 12 candles after trade`.
- Gates neutralised so only the family rule decides: `trade window
  unrestricted`, all three day types allowed, `movement below 1.1`
  (efficiency is at most 1), `candle in (any)`.
- Slippage 0, so fills are exactly the quoted prices.

## Bars (XAUUSD 1h, T0 = 2026-01-05T00:00Z)

Every bar has a 10-point true range and opens at the previous close, so any
ATR (any length, SMA or Wilder) equals 10 once it has data. Bars 0-29 are
flat: open 2000, high 2005, low 1995, close 2000.

| k | open | high | low | close | ROC vs close[k-2] | signal |
| --- | --- | --- | --- | --- | --- | --- |
| 28 | 2000 | 2005 | 1995 | 2000 | 0 | - |
| 29 | 2000 | 2005 | 1995 | 2000 | 0 | - |
| 30 | 2000 | 2010 | 2000 | 2010 | +0.50% | below 0.6: none |
| 31 | 2010 | 2020 | 2010 | 2020 | +1.00% | **long** |
| 32 | 2020 | 2030 | 2020 | 2030 | +0.995% | (position open) |
| 33 | 2030 | 2040 | 2030 | 2040 | +0.99% | (position open) |
| 34 | 2040 | 2050 | 2040 | 2050 | +0.98% | high reaches 2047.5 |
| 35 | 2050 | 2055 | 2045 | 2050 | | |
| 36 | 2050 | 2055 | 2045 | 2050 | | |

## Derivation

1. Signal: bar 31 close 2020 against bar 29 close 2000 = +1.0% ≥ 0.6%.
   Bar 30 (+0.5%) is below the threshold. No earlier bar moves.
2. Entry: "enter on signal close" → entryIndex 31, entry 2020.
3. Stop: lowest low of the last 3 candles. Including the signal bar
   (bars 29-31) the lows are 1995, 2000, 2010; excluding it (bars 28-30) they
   are 1995, 1995, 2000. Both give 1995. Stop = 1995 − 0.25 × 10 = 1992.5.
4. Stop distance = 2020 − 1992.5 = 27.5 = 2.75 ATR, inside [0.5, 3] ATR.
5. Target = entry + 1R = 2020 + 27.5 = 2047.5.
6. Bars 32-33 stay between stop and target. Bar 34 high 2050 ≥ 2047.5 and
   low 2040 > 1992.5 → target filled at the level: exitIndex 34, exit 2047.5,
   reason `tp`.
7. Breakeven is off, so nothing moves the stop before bar 34.

Expected: one long trade, 31 → 34, entry 2020, initialSl 1992.5, initialTp
2047.5, exit 2047.5, reason `tp`.

## Spec gaps and assumptions

- SPEC GAP: the ATR length and smoothing used by `by X ATR` are not
  specified. The fixture makes every true range 10 so the value is 10
  regardless.
- SPEC GAP: whether "last 3 candles" includes the signal candle. The fixture
  is built so both readings give the same low.
- SPEC GAP: an intrabar touch of the target (high ≥ target) fills at the
  target level. This is the usual bar-model reading and is assumed here.
- SPEC GAP: `fillOn` is not documented. `close` is taken to mean that the
  signal-close market order fills on the signal bar.
