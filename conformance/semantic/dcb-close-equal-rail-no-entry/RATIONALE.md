# dcb-close-equal-rail-no-entry

Semantic under test: the breakout comparison is strict. A close equal to the
channel high is not a long signal.

## Spec text

`spec/dsl-spec-families/donchianBreakout.md`, "Purpose": "Both comparisons
are strict: a close equal to a rail is not a breakout."

## Derivation

- Bars 0-19: o 2000, h 2005, l 1995, c 2000.
- Bar 20: o 2000, h 2005, l 1995, c 2005. Channel high (bars 17-19) = 2005.
  2005 > 2005 is false: no signal.
- Bar 21: o 2005, h 2005, l 1995, c 2000. Bars 22-25: o 2000, h 2005,
  l 1995, c 2000. Every channel high from bar 21 on is 2005 and every close
  is at most 2000: no signal.
- Expected trades: none.

Status: derived.
