# pm-gap-through-stop

Semantic under test: a bar that opens beyond the stop fills the stop at the
(worse) open, not at the stop level.

## Spec text

`strat/docs/dsl-spec-families/dailyFlushFailure.md`, "Phrases":

> A stop is active on the entry bar, and a gap through the stop exits at the
> worse open.

This is the only sentence in `strat/docs/` that states a gap-fill rule. It
is written for one family; the shared stop phrases in `strat/docs/dsl-spec.md`
§7 do not repeat it. The fixture applies the same rule to the shared stop,
because a fill at a price the market never traded is not a defensible
alternative.

`strat/docs/dsl-spec-families/priceMomentum.md`, "Shared risk, management, and
guard phrases" (stop and target; quoted in `pm-signal-close-entry`).

## Strategy and bars

Strategy identical to `pm-signal-close-entry`. Bars 0-31 identical (signal
at bar 31, entry 2020, stop 1992.5, target 2047.5). Then:

| k | open | high | low | close | note |
| --- | --- | --- | --- | --- | --- |
| 32 | 1980 | 1985 | 1975 | 1980 | opens 12.5 below the stop |
| 33 | 1980 | 1985 | 1975 | 1980 | |
| 34 | 1980 | 1985 | 1975 | 1980 | |

## Derivation

1. Entry as in the base case: bar 31 at 2020, stop 1992.5.
2. Bar 32 opens at 1980, below the stop. The first price available after
   the gap is the open; the stop fills there: exit 1980 on bar 32, reason
   `sl`. The loss is 40 points, not the planned 27.5.
3. Bars 33-34 are flat (no signal; cooldown also applies).

Expected: one long trade, 31 → 32, exit 1980, reason `sl`.

## Spec gaps and assumptions

- SPEC GAP: the "worse open" rule is stated only in the daily-flush-failure
  family file. The shared execution model should state it once for all
  stops. An engine that fills at the stop level reports exit 1992.5.
