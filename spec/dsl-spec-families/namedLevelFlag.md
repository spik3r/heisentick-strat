# Setup family: `named level flag` (`namedLevelFlag`)

This research family models a directional impulse through a completed named
level, followed by a short flag and a close beyond the completed flag range.
It is separate from `flag continuation` and `named level sweep`: it uses the
named level only to arm the setup, then trades the continuation after the flag.

## Defaults

`type: named level flag` sets an impulse body minimum of 0.8 ATR, a close
clearance of 0.1 ATR beyond a stable named level, a maximum of ten observed
post-impulse bars, and requires at least two completed flag bars and one
opposite-colour candle. The impulse candle is excluded from flag extrema.

The flag fails if price wicks through its impulse midpoint in the adverse
direction, closes back through the named level, or the level changes from its
cached completed value. A level can arm once per UTC day. Entry is a market
order at the next open after a close beyond the previous flag extreme. The
signal close anchors a fixed 2R target. The stop sits 0.2 signal ATR beyond the
flag extreme, and signal-close risk must be 0.4–2 ATR. The setup requires a
stable known named level ahead when `target mode known level 2R` is selected;
that level admits the setup but does not replace the 2R target.

The research strategy uses a 24-observed-bar hold and next-open fills, set by
its execution configuration. At the DSL family level, exit behavior follows
the strategy's configured hold and the engine closes any open position at the
end of the dataset. The family does not move its stop to breakeven. It uses
normal engine risk sizing and fill cost rules.

## Breakout body filter

In `filters`, `breakout body at least X ATR` requires the absolute body of the
breakout candle to meet the threshold using the breakout signal ATR. It does
not require the breakout candle to have the impulse colour.

`signal risk at least X points` sets the minimum distance from the breakout
signal close to the padded flag stop, in instrument price units. It is applied
after the stop ATR bounds and uses the signal ATR for the stop pad and bounds.
The default is zero, which disables the floor. A rejected armed attempt is
consumed; it does not wait for a later breakout.

```dsl
dsl v7
strategy "Named Level Flag"
levels { priority(PDH, PDL, AH, AL) }
setup { type: named level flag }
filters {
  breakout body at least 0.5 ATR
  signal risk at least 1.2 points
}
```

## Causal level snapshots

Only values supplied through `levels { priority(...) }` are considered.
Eligibility compares the current level values with the prior cached flat-bar
snapshot. Snapshots are not refreshed while a position is open, matching the
app research adapter. Strategies should list completed references such as
prior-day and prior-session levels; developing current-day or current-session
extremes are not completed levels.
