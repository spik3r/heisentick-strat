# pm-same-bar-stop-and-target

Semantic under test: when one bar touches both the stop and the target, the
engine must apply one deterministic rule. The conservative rule is that the
stop wins.

## Spec text

`strat/docs/dsl-spec-families/priceMomentum.md`, "Shared risk, management, and
guard phrases" (recent-extreme stop, 1R target; quoted in
`pm-signal-close-entry`).

`strat/docs/dsl-spec.md` §7 defines the stop and target phrases but says
nothing about the order in which they are tested inside one bar.

`docs/candle-probability-research.md` "Ambiguous barriers" describes the
same problem for a research tool and is not part of the language spec.

## Strategy and bars

Strategy identical to `pm-signal-close-entry`. Bars 0-31 identical (signal
at bar 31, entry 2020, stop 1992.5, target 2047.5). Then:

| k | open | high | low | close | note |
| --- | --- | --- | --- | --- | --- |
| 32 | 2020 | 2050 | 1990 | 2020 | high ≥ 2047.5 **and** low ≤ 1992.5 |
| 33 | 2020 | 2025 | 2015 | 2020 | |
| 34 | 2020 | 2025 | 2015 | 2020 | |

## Derivation

1. Entry as in the base case: bar 31 at 2020, stop 1992.5, target 2047.5.
2. Bar 32 opens inside the bracket (2020), then trades to 2050 and 1990: both
   levels are touched and the bar gives no information about which came
   first.
3. Conservative rule: assume the adverse level was hit first. Exit at the
   stop, 1992.5, on bar 32, reason `sl`.
4. Bars 33-34 are flat and produce no new signal (ROC +0.5% and 0%; the
   12-candle cooldown also applies).

Expected: one long trade, 31 → 32, exit 1992.5, reason `sl`.

## Spec gaps and assumptions

- SPEC GAP: no same-bar ambiguity rule is written. Candidate rules: stop
  first (used here), target first, open-proximity (whichever level is
  nearer the open is hit first: here the stop is 27.5 away and the target
  27.5 away, a tie), or a `close`-based tie-break. The spec must choose one;
  until it does the case is `spec-gap`.
