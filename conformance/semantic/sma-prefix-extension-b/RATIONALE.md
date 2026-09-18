# sma-prefix-extension-b

Semantic under test: appending future bars never changes an earlier decision.
`fixture.json` carries `"extends": "sma-prefix-extension-a"`; bars 0-10 are
byte-identical to that case and bars 11-16 are appended. The first trade must
be identical to the prefix case's trade. This is the invariant that makes a
re-run-on-each-closed-bar forward runner valid.

## Spec text

`strat/docs/dsl-spec.md` §10 "Evaluation order and causality":

> No strategy-visible value may depend on the current bar's future or on
> later bars: levels, ranges, channels, day types, biases, seasonality and
> VP context are computed from completed data only.

`strat/docs/dsl-spec-families/smaGoldenCross.md`: SMAs of completed closes;
entry and exit at the next bar open.

## Bars (XAUUSD 1h, T0 = 2026-01-05T00:00Z)

Bars 0-10: see `sma-cross-next-open-entry-exit`. Appended bars:

| k | open | high | low | close | SMA2 | SMA3 | fast vs slow |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 10 | 2020 | 2020 | 2010 | 2010 | 2015 | 2020 | below |
| 11 | 2010 | 2010 | 2000 | 2000 | 2005 | 2010 | below |
| 12 | 2000 | 2010 | 2000 | 2010 | 2005 | 2006.67 | below |
| 13 | 2010 | 2020 | 2010 | 2020 | 2015 | 2010 | **above: golden cross** |
| 14 | 2020 | 2030 | 2020 | 2030 | 2025 | 2020 | above |
| 15 | 2030 | 2040 | 2030 | 2040 | 2035 | 2030 | above |
| 16 | 2040 | 2050 | 2040 | 2050 | 2045 | 2040 | above |

## Derivation

1. Trade 1: exactly the trade of `sma-prefix-extension-a` (entry bar 6 open
   2020, exit bar 10 open 2020, reason `rule`). Every value it depends on
   (SMAs up to bar 9, the bar 10 open) is unchanged by the appended bars.
2. Trade 2: SMA2 is below SMA3 on bars 10-12 and above on bar 13
   (2015 > 2010): golden cross on bar 13 → entry bar 14 open 2020.
3. SMA2 stays above SMA3 through bar 16; the data ends with the position
   open → reason `end-of-test` on bar 16, exit at the bar 16 close 2050
   (see `sma-end-of-test-open-position` for the liquidation-price gap).

Expected: two long trades, [6→10 @ 2020/2020 `rule`], [14→16 @ 2020/2050
`end-of-test`].

## Spec gaps and assumptions

- Same as `sma-cross-next-open-entry-exit` and
  `sma-end-of-test-open-position`.
