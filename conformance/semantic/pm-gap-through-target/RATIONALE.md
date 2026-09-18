# pm-gap-through-target

Semantic under test: a bar that opens beyond the target. The conservative
reading credits the exit at the target level, never better.

## Spec text

`strat/docs/dsl-spec-families/priceMomentum.md`, "Shared risk, management, and
guard phrases": "a 1R target" (quoted in `pm-signal-close-entry`).

`strat/docs/dsl-spec.md` §7: "`target <N>R` — R-multiple target".

`strat/docs/dsl-spec-families/dailyFlushFailure.md` states a worse-open rule
for stops only ("a gap through the stop exits at the worse open"); nothing
is written for a favourable gap through a target.

## Strategy and bars

Strategy identical to `pm-signal-close-entry`. Bars 0-31 identical (signal
at bar 31, entry 2020, stop 1992.5, target 2047.5). Then:

| k | open | high | low | close | note |
| --- | --- | --- | --- | --- | --- |
| 32 | 2060 | 2065 | 2055 | 2060 | opens 12.5 above the target |
| 33 | 2060 | 2065 | 2055 | 2060 | |
| 34 | 2060 | 2065 | 2055 | 2060 | |

## Derivation

1. Entry as in the base case: bar 31 at 2020, target 2047.5.
2. Bar 32 opens at 2060, above the target. The target is reached on bar 32.
3. Conservative reading: the strategy is credited with the planned target
   price 2047.5 (exactly 1R), never a better price the plan did not ask
   for. Exit 2047.5 on bar 32, reason `tp`.
4. Bars 33-34 are flat (no signal; cooldown also applies).

Expected: one long trade, 31 → 32, exit 2047.5, reason `tp`.

## Spec gaps and assumptions

- SPEC GAP: no rule for a favourable gap through a target. The alternative
  reading, symmetric with the stop rule, is "fill at the open" (2060,
  1.45R). The spec must pick one; the stop rule's wording ("worse open")
  suggests the asymmetric, conservative reading used here.
