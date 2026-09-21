# pm-fill-on-open-entry

Semantic under test: with `costs.fillOn: "open"`, a market order raised at a
signal close fills at the next bar's open, so the entry index and timestamp
move one bar later than in `pm-signal-close-entry`.

## Spec text

`spec/dsl-spec.md` §6: "`enter at market` — enter on signal close (default)."
The decision is made at the signal close.

`spec/dsl-spec-families/priceMomentum.md`, "Purpose" and "Shared risk,
management, and guard phrases" (quoted in `pm-signal-close-entry`).

`spec/dsl-spec.md` §7, "Execution" defines `costs.fillOn: open` as a
next-bar-open market fill and preserves `nextOpen` as the explicit spelling.

## Strategy and bars

Identical to `pm-signal-close-entry`; only `costs.fillOn` differs (`open`).

## Derivation

1. Signal at the close of bar 31 (+1.0%), as in the base case.
2. `fillOn: open`: the order fills at the open of the next bar, bar 32. Bar
   32 opens at 2020 (the bar 31 close), so the price is unchanged; the
   discriminating fields are `entryIndex` 32 and `entryT` = bars[32].t.
3. Stop and target are read as fixed when the order is raised (the signal
   close): stop 1992.5, target 2047.5, distance 27.5 = 1R. Measured from the
   fill they are the same numbers because fill = signal close.
4. Bar 34 high 2050 reaches 2047.5 → exit `tp` at 2047.5.

Expected: one long trade, entryIndex 32 / entryT bars[32].t / entry 2020,
initialSl 1992.5, initialTp 2047.5, exit 34 @ 2047.5, reason `tp`.

## Bracket timing

The order keeps its signal-time stop and target. The engine also supports
explicit fill-relative distances for callers that need brackets anchored to
the eventual fill; this fixture uses the signal-time levels so the expected
prices remain hand-derived.
