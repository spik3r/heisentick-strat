# pm-trade-window-london-close

Semantic under test: segmented trade-window admission uses the intentional fixed
UTC+10 compatibility clock and half-open 60-minute parts.

## Spec text

`spec/dsl-spec.md` section 4 defines the fixed UTC+10 entry windows and
`trade window in (<window>.<part>)`. It defines `.open`, `.middle`, and
`.close` as the first, second, and third half-open 60-minute portions. Segment
gates respect `sessions(...)` and intersect `trade window minutes A to B`.
The four-hour `mid` window has a separate boundary regression: 15:00-16:00
local matches only `mid.all`.

## Fixture

The bars and momentum setup are the hand-derived
`pm-session-no-entry-outside-window` case shifted nine hours later. This makes
bar 46 land at 17:00 fixed UTC+10 (`london.middle`) and bar 47 at 18:00
(`london.close`) without changing prices, ATR, signal strength, stop, or target.

- Bar 46: +1.0% momentum signal at 17:00 local. `london.close` rejects it.
- Bar 47: +0.746% signal at 18:00 local, the inclusive start of
  `london.close`. Entry is 2025 at the signal close.
- Stop is 1997.5; target is 2052.5. Bar 50 reaches the target.

Expected: one long trade, entry bar 47 and target exit bar 50. Focused engine
boundary tests pin every segment edge and the fourth `mid` hour.
