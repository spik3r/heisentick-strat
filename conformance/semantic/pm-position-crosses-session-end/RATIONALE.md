# pm-position-crosses-session-end

Semantic under test: a position opened inside an enabled session is not
closed when the session window ends; sessions gate entries only. The trade
runs to its target two bars after the window closes.

## Spec text

`strat/docs/dsl-spec.md` §4: "`sessions(asia, london, ny)` — enable trading
sessions"; §10 lists "market gates (sessions, windows, …)" under "Signal
admission". Nothing in the spec describes a session-end exit, and
`maxHoldCandles` (§7) is the only time-based exit defined ("time stop (0 =
off)"), left at its default 0 here.

`strat/docs/dsl-spec-families/priceMomentum.md`: shared session phrases
apply; the family lists no session-end exit.

## Session windows (assumption)

Same table as `pm-session-no-entry-outside-window`: asia = 09:00–12:00 local
(UTC+10) = 23:00–02:00 UTC, half-open. `sessions(asia)` only.

## Strategy

As `pm-signal-close-entry` but with `sessions(asia)` and **without**
`trade window unrestricted`.

## Bars (XAUUSD 1h, T0 = 2026-01-05T00:00Z)

Bars 0-46 flat at 2000 (open 2000, high 2005, low 1995).

| k | UTC | local | window | open | high | low | close | ROC vs k−2 | signal |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 46 | Jan 6 22:00 | 08:00 | none | 2000 | 2005 | 1995 | 2000 | 0 | - |
| 47 | 23:00 | 09:00 | asia | 2000 | 2010 | 2000 | 2010 | +0.50% | none |
| 48 | Jan 7 00:00 | 10:00 | asia | 2010 | 2020 | 2010 | 2020 | +1.00% | **long, admitted** |
| 49 | 01:00 | 11:00 | asia | 2020 | 2030 | 2020 | 2030 | | position open |
| 50 | 02:00 | 12:00 | **none** | 2030 | 2040 | 2030 | 2040 | | window ended; position stays |
| 51 | 03:00 | 13:00 | none | 2040 | 2050 | 2040 | 2050 | | high reaches 2047.5 |
| 52 | 04:00 | | | 2050 | 2055 | 2045 | 2050 | | |
| 53 | 05:00 | | | 2050 | 2055 | 2045 | 2050 | | |

## Derivation

1. Bar 48: 2020 / 2000 = +1.0% → long signal; 00:00 UTC is 10:00 local,
   inside asia → admitted. Entry at the close, 2020.
2. Stop: lows of bars 46-48 (1995, 2000, 2010) or 45-47 (1995, 1995, 2000)
   → 1995; stop 1992.5; distance 27.5; target 2047.5.
3. Bar 50 opens at 02:00 UTC = 12:00 local: the asia window has ended. No
   rule closes the position; bar 50 (high 2040) does not reach the target.
4. Bar 51 high 2050 ≥ 2047.5 → exit `tp` at 2047.5, one hour after the
   window closed.

Expected: one long trade, 48 → 51, entry 2020, initialSl 1992.5, initialTp
2047.5, exit 2047.5, reason `tp`.

Trap: an engine that flattens at the window end reports exitIndex 49 or 50
with a different reason and a price of 2030 or 2040.

## Spec gaps and assumptions

- SPEC GAP: session hours (see the companion case).
- SPEC GAP: the spec does not say explicitly that sessions never force an
  exit. The reading follows from §10, which lists sessions under signal
  admission only, and from the absence of any session-exit phrase.
