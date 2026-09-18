# pm-session-no-entry-outside-window

Semantic under test: a signal on a bar outside every enabled session is not
traded; the next signal inside a window is.

## Spec text

`strat/docs/dsl-spec.md` §4 "`market conditions` — where and when the strategy
trades":

> `sessions(asia, london, ny)` — enable trading sessions (default
> asia+london+ny; `mid` exists and defaults off).

> `trade window unrestricted` — research-only override that removes the
> engine's fixed UTC+10 trade-window gate.

`strat/docs/dsl-spec.md` §10: "market gates (sessions, windows, …)" are the
first admission gate; "the first failing gate rejects the signal".

`strat/docs/dsl-spec-families/priceMomentum.md`: shared "session" phrases
apply to this family.

## Session windows (assumption)

The spec names the sessions but never states their hours. The only written
hint is "fixed UTC+10". The fixture assumes the engine table
(`engine/dsl/sessions.js`, `TRADE_WINDOWS`, local = UTC+10):

| session | local | UTC |
| --- | --- | --- |
| asia | 09:00–12:00 | 23:00–02:00 |
| mid | 12:00–16:00 | 02:00–06:00 |
| london | 16:00–19:00 | 06:00–09:00 |
| ny | 21:00–24:00 | 11:00–14:00 |

Windows are half-open (`start <= hour < end`). This case enables `asia`
only.

## Strategy

As `pm-signal-close-entry` but with `sessions(asia)` and **without**
`trade window unrestricted`.

## Bars (XAUUSD 1h, T0 = 2026-01-05T00:00Z)

Bars 0-43 flat at 2000 (open 2000, high 2005, low 1995). Bar 44 is flat with
its low raised to 2000 (open 2000, high 2010, low 2000, close 2000) so the
3-candle low is the same with or without the signal bar.

| k | UTC | local | window | open | high | low | close | ROC vs k−2 | signal |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 44 | Jan 6 20:00 | 06:00 | none | 2000 | 2010 | 2000 | 2000 | 0 | - |
| 45 | 21:00 | 07:00 | none | 2000 | 2010 | 2000 | 2010 | +0.50% | none |
| 46 | 22:00 | 08:00 | **none** | 2010 | 2020 | 2010 | 2020 | +1.00% | long, **rejected** |
| 47 | 23:00 | 09:00 | asia | 2020 | 2027 | 2017 | 2025 | +0.746% | **long, admitted** |
| 48 | Jan 7 00:00 | 10:00 | asia | 2025 | 2035 | 2025 | 2035 | +0.74% | position open |
| 49 | 01:00 | 11:00 | asia | 2035 | 2045 | 2035 | 2045 | | |
| 50 | 02:00 | 12:00 | none | 2045 | 2055 | 2045 | 2055 | | high reaches 2052.5 |
| 51 | 03:00 | | | 2055 | 2060 | 2050 | 2055 | | |
| 52 | 04:00 | | | 2055 | 2060 | 2050 | 2055 | | |

## Derivation

1. Bar 46: 2020 / 2000 = +1.0% → long signal. 22:00 UTC is 08:00 local,
   outside asia (09–12). The session gate rejects it; no order exists.
2. Bar 47: 2025 / 2010 = +0.746% ≥ 0.6% → long signal. 23:00 UTC is 09:00
   local, the first asia hour → admitted. Entry at the close: 2025.
3. Stop: lows of bars 45-47 (2000, 2010, 2017) or 44-46 (2000, 2000, 2010)
   → 2000. Stop = 2000 − 2.5 = 1997.5; distance 27.5 (2.75 ATR, in bounds);
   target 2052.5.
4. Bar 50 high 2055 ≥ 2052.5 → exit `tp` at 2052.5 (exits are not gated by
   sessions; see `pm-position-crosses-session-end`).

Expected: one long trade, 47 → 50, entry 2025, initialSl 1997.5, initialTp
2052.5, exit 2052.5, reason `tp`.

Trap: an engine without the session gate (or with a different window table)
enters at bar 46 at 2020 (stop 1997.5 from the bar 44-46 lows, target
2047.5) and reports entryIndex 46.

## Spec gaps and assumptions

- SPEC GAP: session hours and the UTC+10 anchor are not in the spec. The
  fixture depends on the table above; if the spec later defines different
  hours the bar timestamps must be re-derived.
- SPEC GAP: whether `sessions(...)` gates only entries or also forces exits.
  This case does not depend on it (the target is hit two bars after the
  window ends; see the companion case).
