# dcb-signal-close-entry

Semantic under test: a donchian breakout signals when the close is strictly
above the highest high of the N bars before it, the signal bar is not part of
its own channel, and entry fills at the signal close with a fixed ATR stop and
an R target measured from that close.

## Spec text

`spec/dsl-spec-families/donchianBreakout.md`, "Purpose": the channel is
`max(high[i - N] .. high[i - 1])`; "Bar `i` is not part of its own
channel"; "Both comparisons are strict".

Same file, "Entry, stop and target": entry at the signal bar's close; long
stop `close[i] - k * ATR[i]`; `target NR` at `close[i] + R * k * ATR[i]`.

## Derivation

- Bars 0-19: o 2000, h 2005, l 1995, c 2000. True range 10 on every bar, so
  ATR = 10 for any length or smoothing.
- Bar 20: o 2000, h 2010, l 2000, c 2010. True range max(10, 10, 0) = 10.
  Channel = bars 17-19, high 2005. 2010 > 2005: long signal.
  Including bar 20 would give a rail of 2010 and 2010 > 2010 is false, so the
  trade exists only if the signal bar is excluded.
- Entry 2010 (fillOn close, slippage 0). Stop 2010 - 2 x 10 = 1990.
  Risk 20, target 2010 + 20 = 2030.
- Bar 21: o 2010, h 2020, l 2010, c 2020. No exit.
- Bar 22: o 2020, h 2030, l 2015, c 2018. High 2030 >= 2030: exit at the
  target, reason `tp`. Bar 22 closes 2018 against a channel high of 2020
  (bars 19-21): no re-entry on the exit bar.
- Bars 23-26: o 2018, h 2023, l 2013, c 2018; channel highs 2030 and then
  2023, so no close breaks out.

Status: derived.
