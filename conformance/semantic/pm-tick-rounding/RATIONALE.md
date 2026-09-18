# pm-tick-rounding

Semantic under test: derived prices (stop, target) that fall between ticks.
The spec defines no tick size and no rounding rule, so the expected values
are the unrounded arithmetic and the case is `spec-gap` until the spec
decides.

## Spec text

`strat/docs/dsl-spec.md` §7: "`stop beyond last N candle extreme by X ATR`";
§2 "Common token shapes": "**number** — JavaScript-style decimal". Nothing
in `strat/docs/` mentions a tick size or price rounding. The migration plan
(`backlog/plans/2026-09-18-one-engine-migration.md`, "Correctness") calls
for "banker's-rounding-free tick rounding defined in the spec" and exact
comparison "on prices after tick rounding" — the rule does not exist yet.

`strat/docs/dsl-spec-families/openingRangeBreakout.md` mentions that "For
XAUUSD, the reviewed pip size is `0.1`" for `stop <N> pips`, which is a pip,
not a tick, and applies to that phrase only.

## Strategy and bars

Bars identical to `pm-signal-close-entry`. The strategy differs in one
number: `stop beyond last 3 candle extreme by 0.0125 ATR`.

## Derivation

1. Signal and entry as in the base case: bar 31 at 2020; ATR = 10.
2. Padding = 0.0125 × 10 = 0.125. Lowest low of the last 3 candles = 1995.
   Stop = 1995 − 0.125 = 1994.875.
3. Distance = 2020 − 1994.875 = 25.125 = 2.5125 ATR, inside [0.5, 3].
4. Target = 2020 + 25.125 = 2045.125.
5. Bar 34 high 2050 ≥ 2045.125 → exit at the target level, reason `tp`.

Expected (unrounded): initialSl 1994.875, initialTp 2045.125, exit 2045.125.

Candidate rounded values, for the reviewer who writes the rule:

| rule | tick | stop | target |
| --- | --- | --- | --- |
| none (this fixture) | - | 1994.875 | 2045.125 |
| half away from zero | 0.01 | 1994.88 | 2045.13 |
| half to even (banker's) | 0.01 | 1994.88 | 2045.12 |
| away from the position (stop down, target up) | 0.01 | 1994.87 | 2045.13 |
| any of the above | 0.1 | 1994.9 | 2045.1 |

Half-to-even and half-away differ on the target (…12 vs …13), which is
exactly the distinction the plan warns about.

## Spec gaps and assumptions

- SPEC GAP: tick size per instrument and the rounding rule. Until written,
  Go should be compared against this case on `entryIndex`, `exitIndex` and
  `reason` only, and the prices should be re-derived once the rule exists.
