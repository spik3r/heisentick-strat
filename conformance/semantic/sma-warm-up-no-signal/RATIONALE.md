# sma-warm-up-no-signal

Semantic under test: an indicator that is not yet defined produces no signal.
A partial-window average must not stand in for the missing history.

## Spec text

`strat/docs/dsl-spec-families/smaGoldenCross.md`:

> It computes simple moving averages of completed bar closes, enters long at
> the next bar open when the fast SMA crosses above the slow SMA

A simple moving average of N closes needs N completed closes; a cross needs
the averages on the signal bar and on the bar before it.

`strat/docs/dsl-spec.md` §10 "Evaluation order and causality":

> No strategy-visible value may depend on the current bar's future or on
> later bars

## Strategy

`sma fast 2`, `sma slow 4`, `side long only`.

## Bars (XAUUSD 1h, T0 = 2026-01-05T00:00Z)

| k | open | high | low | close | SMA2 | SMA4 (full) | SMA4 (partial mean, wrong) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | 2040 | 2040 | 2030 | 2030 | - | - | 2030 |
| 1 | 2030 | 2030 | 2000 | 2000 | 2015 | - | 2015 |
| 2 | 2000 | 2020 | 2000 | 2020 | 2010 | - | 2016.67 |
| 3 | 2020 | 2040 | 2020 | 2040 | 2030 | 2022.5 | 2022.5 |
| 4 | 2040 | 2050 | 2040 | 2050 | 2045 | 2027.5 | - |
| 5 | 2050 | 2060 | 2050 | 2060 | 2055 | 2042.5 | - |
| 6 | 2060 | 2070 | 2060 | 2070 | 2065 | 2055 | - |

## Derivation

1. SMA4 first exists at bar 3 (closes 2030, 2000, 2020, 2040 → 2022.5).
2. A crossover on bar 3 would need SMA4 on bar 2, which does not exist. The
   most conservative reading of an undefined comparison is "no signal".
3. From bar 3 on, SMA2 is above SMA4 on every bar (2030 > 2022.5,
   2045 > 2027.5, 2055 > 2042.5, 2065 > 2055) so there is never a
   below-then-above transition.
4. Therefore no signal and no trade in the whole series.

Trap: an implementation that averages whatever closes exist would compute
"SMA4" = 2016.67 on bar 2, see SMA2 2010 below it, then 2030 > 2022.5 on bar 3,
and fire a golden cross on bar 3 with an entry at bar 4 open 2040.

Expected: `trades: []`.

## Spec gaps and assumptions

- SPEC GAP: the spec does not say in words that an SMA of N closes is
  undefined before N closes. The derivation takes the mathematical meaning.
