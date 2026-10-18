# er-next-open-entry-ribbon-break

Semantic under test: an ema ribbon signal at bar i's close fills at bar
i+1's open, with an ATR stop measured from that fill; the position closes at
the next open after the ribbon stops being aligned with it; no entry is
placed while a position is open.

## Spec text

`spec/dsl-spec-families/emaRibbon.md`:

- "Alignment": every EMA strictly above the next slower one is long
  alignment; equal values are not aligned. EMA(N) is seeded with the first
  close and valid from bar 3N.
- "Entry": a fresh alignment (not aligned the same way on bar i-1) with the
  close beyond the fastest EMA; the order fills at the next bar's open.
- "Stop and target": the stop is `k x ATR(14)` (Wilder, valid from bar 14)
  at the signal bar, measured from the fill.
- "Exit": at the close of a bar where the ribbon is no longer aligned with
  the position, the position closes at the next bar's open, reason `rule`,
  rule `ribbon-break`.

## Derivation

- Bars 0-19: o 2000, h 2005, l 1995, c 2000. True range 10, so ATR = 10.
  EMA(2) = EMA(3) = 2000: not strictly ordered, no alignment.
- Bar 20: o 2000, h 2010, l 2000, c 2010. TR = max(10, 10, 0) = 10, ATR 10.
  EMA(2) = 2000 + 2/3 x 10 = 2006.667; EMA(3) = 2000 + 1/2 x 10 = 2005.
  2006.667 > 2005: long-aligned, and bar 19 was not aligned, so this is a
  fresh alignment. Close 2010 > EMA(2) 2006.667: long signal.
- Bar 21 opens 2010: entry 2010 (slippage 0). Stop 2010 - 3 x 10 = 1980.
  Close 2020: EMA(2) = 2006.667 + 2/3 x 13.333 = 2015.556; EMA(3) =
  2005 + 1/2 x 15 = 2012.5. Aligned long: hold.
- Bar 22: close 2010: EMA(2) = 2015.556 - 2/3 x 5.556 = 2011.852; EMA(3) =
  2012.5 - 1/2 x 2.5 = 2011.25. Aligned long: hold.
- Bar 23: close 2000: EMA(2) = 2011.852 - 2/3 x 11.852 = 2003.951; EMA(3) =
  2011.25 - 1/2 x 11.25 = 2005.625. EMA(2) < EMA(3): not long-aligned, so
  the exit is queued for the next open. The bar is a fresh short alignment,
  but the long is still open, so no order is placed.
- Bar 24 opens 2000: exit 2000, reason `rule` (`ribbon-break`). Lows never
  reach 1980. Bar 24 stays short-aligned (2001.317 < 2002.813), which is not
  fresh, and no pullback EMA is set: no entry. Bars 25-29 close 2000 and keep
  EMA(2) below EMA(3): no fresh alignment, no further trades.
