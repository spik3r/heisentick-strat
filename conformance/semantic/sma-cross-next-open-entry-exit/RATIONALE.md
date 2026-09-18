# sma-cross-next-open-entry-exit

Semantic under test: a family whose rule says "next bar open" enters and
exits on the bar after the signal bar, at that bar's open, not at the signal
bar's close.

## Spec text

`strat/docs/dsl-spec-families/smaGoldenCross.md`, opening paragraph:

> It computes simple moving averages of completed bar closes, enters long at
> the next bar open when the fast SMA crosses above the slow SMA, and queues a
> long close at the next bar open when the fast SMA crosses below the slow SMA.

Same file: "Backtest sizing is a fixed one-unit entry" and "The exact source
translation has no stop, target, trailing, session, or other risk-management
semantics."

## Strategy

`sma fast 2`, `sma slow 3`, `side long only`. Slippage is 0 so the fill is
exactly the bar open.

## Bars (XAUUSD 1h, T0 = 2026-01-05T00:00Z, index k opens at T0 + k h)

| k | open | high | low | close | SMA2 | SMA3 | fast vs slow |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | 2040 | 2040 | 2030 | 2030 | - | - | - |
| 1 | 2030 | 2030 | 2020 | 2020 | 2025 | - | - |
| 2 | 2020 | 2020 | 2010 | 2010 | 2015 | 2020 | below |
| 3 | 2010 | 2010 | 2000 | 2000 | 2005 | 2010 | below |
| 4 | 2000 | 2010 | 2000 | 2010 | 2005 | 2006.67 | below |
| 5 | 2010 | 2020 | 2010 | 2020 | 2015 | 2010 | **above: golden cross** |
| 6 | 2020 | 2030 | 2020 | 2030 | 2025 | 2020 | above |
| 7 | 2030 | 2040 | 2030 | 2040 | 2035 | 2030 | above |
| 8 | 2040 | 2040 | 2030 | 2030 | 2035 | 2033.33 | above |
| 9 | 2030 | 2030 | 2020 | 2020 | 2025 | 2030 | **below: bearish cross** |
| 10 | 2020 | 2020 | 2010 | 2010 | 2015 | 2020 | below |

SMA2(5) = (2010 + 2020) / 2 = 2015; SMA3(5) = (2000 + 2010 + 2020) / 3 = 2010.
SMA2(9) = (2030 + 2020) / 2 = 2025; SMA3(9) = (2040 + 2030 + 2020) / 3 = 2030.

## Derivation

1. Bars 2-4: SMA2 is below SMA3. At the close of bar 5 SMA2 (2015) is above
   SMA3 (2010) for the first time: fast crosses above slow on bar 5.
2. "enters long at the next bar open": the entry is bar 6 at its open, 2020.
   An engine that fills on the signal close would report bar 5 at 2020 as
   well (same price here, different index and timestamp); the index and
   timestamp are the discriminating fields.
3. At the close of bar 9 SMA2 (2025) is below SMA3 (2030) after being above
   at bar 8: fast crosses below slow. "queues a long close at the next bar
   open": exit bar 10 at its open, 2020.
4. No stop, target or session applies to this family, so no other exit can
   pre-empt the rule exit.

Expected: one long trade, entryIndex 6 / entry 2020, exitIndex 10 / exit 2020,
reason `rule`.

## Spec gaps and assumptions

- SPEC GAP: the family file does not define "crosses above" at equality.
  The fixture avoids equality (strict inequality on both sides of the cross).
- SPEC GAP: no exit-reason vocabulary exists. `rule` is this set's name for a
  family close rule; see the README.
- The fixture's `fillOn: close` is assumed not to override a family rule that
  names the next bar open explicitly (see `pm-fill-on-open-entry` for the
  fillOn gap).
