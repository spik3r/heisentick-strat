# Setup family: `ema ribbon` (emaRibbon)

Status: specified (2026-10-17)

Specified against `dsl/parser_ema_ribbon.go`, `engine/family_ema_ribbon.go`,
the JavaScript reference in the heisentick app
(`engine/dsl/setups/emaRibbon.js`,
`strat/implementations/browser-runtime/compiler/parseSetups/emaRibbonPhrases.js`,
`strat/implementations/browser-runtime/compiler/finalizeEmaRibbon.js`), the
`*ema-ribbon*` parse and run conformance cases and the `er-*` semantic cases.

Part of `spec/dsl-spec.md` §9.

## Purpose

EMA ribbon is a causal, single-symbol trend follower. It holds a position
while a ribbon of EMAs of closes stays in order, and leaves when the order
breaks. Optional filters build a higher-timeframe trend and a daily close
from the route's own bars, and an optional UTC window limits the hours at
which the strategy acts. The research behind it is
`strategies/evidence/ema-ribbon-trend-2026-10-17.md` in the heisentick app
(gold 1h, 2003-2026).

## Phrases

All family phrases are written in the `setup` section; the same phrase shape
in another section is an error.

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `type: ema ribbon` | Selects the family. This exact spelling is the only type phrase. | Not selected unless declared. | `setupType = "emaRibbon"`, initializes `emaRibbon` |
| `ribbon ema N1 N2 ...` | 2 to 8 strictly increasing positive integer EMA lengths, fastest first. | Required; no default. | `emaRibbon.emas` |
| `pullback ema N` | Enables pullback re-entry against EMA(N). | Off. | `emaRibbon.pullbackEma` |
| `trend filter TF ribbon` | Higher-timeframe filter: the ribbon's lengths on TF buckets must be aligned the trade's way. TF is `2h`, `4h`, `8h` or `1d`. | Off. | `emaRibbon.trendFilter = {timeframe, mode: "ribbon"}` |
| `trend filter TF ema F above S` | Higher-timeframe filter: EMA(F) above EMA(S) of TF bucket closes for longs, below for shorts. Requires `F < S`. | Off. | `emaRibbon.trendFilter = {timeframe, mode: "pair", fast, slow}` |
| `shorts below daily ema N` | Shorts need the last closed UTC day's close below EMA(N) of daily closes. Longs are not gated. | Off. | `emaRibbon.shortDailyEma` |
| `act hours A to B utc` | Entries and signal exits only when the decision bar's close time falls in the UTC hours `[A, B)`, wrapping past midnight when `A > B`. Whole hours 0-23, `A != B`. | All hours. | `emaRibbon.actFromUtc`, `emaRibbon.actToUtc` |

Counts are exact positive integers, at most 100000. A malformed family phrase
is an error naming its exact shape.

## Definitions

- **EMA(N)** of a value series uses `alpha = 2 / (N + 1)`, is seeded with the
  first value and is valid from index `3 x N`. Before that it is undefined.
- **Alignment** of an ordered list of EMA values (fastest first) is long when
  every value is strictly above the next, short when every value is strictly
  below the next, and none otherwise, including when any value is undefined.
  Equal values are not aligned.
- **ATR(14)** is Wilder's: the first bar's high minus low, then
  `ATR += (TR - ATR) / 14`. It is valid from index 14.
- **Bar duration** is the smallest positive gap between consecutive bar times
  in the series. The **decision time** of bar `i` is its open time plus that
  duration, which is its close.
- **Buckets** for the trend filter and the daily gate are UTC-aligned
  (`key = t - t mod width`; widths 2h, 4h, 8h, 24h). A bucket's close is the
  close of its last bar. At bar `i`, the filter reads the last bucket whose
  end (`key + width`) is at or before bar `i`'s decision time. EMAs over
  bucket closes follow the EMA definition above, indexed by bucket.

## Entry

At the close of bar `i`, with no open position, no pending order, and no
position closed on bar `i`:

1. The ribbon is aligned at `i` (direction `d`).
2. Either the ribbon was not aligned `d` at `i - 1`, or `pullback ema N` is
   set, EMA(N) is defined at `i - 1` and `close[i - 1]` was on the wrong side
   of it (below for a long, above for a short).
3. `close[i]` is strictly beyond the fastest EMA (above for a long).
4. When a trend filter is set, its alignment at bar `i` is `d`.
5. When `d` is short and the daily gate is set, the last closed day's close
   is strictly below its daily EMA.
6. Bar `i`'s decision time is inside the act window, when one is set.
7. ATR(14) is defined at `i` and `stop distance = k x ATR(14)[i]` is finite
   and positive.

The order fills at bar `i + 1`'s open (`DSL-ER`). `side: long only` and
`side: short only` drop the other side.

## Stop and target

- `stop N ATR` sets `k`; the default is 3. The stop is `k x ATR(14)[i]` from
  the **fill price**, active from the fill bar.
- `target NR` places a target `R x stop distance` from the fill. With no
  target phrase, or `target 0R`, there is no target.

## Exit

At the close of bar `j` with an open position, when the ribbon is no longer
aligned in the position's direction and bar `j`'s decision time is inside the
act window, the position closes at bar `j + 1`'s open with reason `rule` and
rule `ribbon-break`. Outside the window the exit waits: the condition is
checked again at each later bar's close, and the stop stays active. A stop
or target touched intrabar closes the position first.

## Market gates and management

The family sets an unrestricted trade window, all three day types and no
movement-efficiency cap. It refuses, wherever written:

- session, trade-window, day-type, regime, movement, trendiness,
  higher-timeframe (`higher timeframe ...`, `htf ...`), New York hour, local
  hour/weekday, prior-day and open-location gates;
- breakeven (`move stop to breakeven ...`), trail, partial, `wait`, and
  `maxHoldCandles` phrases;
- stop shapes other than `stop N ATR` and targets other than `target NR`;
- `enter` phrases and source-timeframe execution.

The act window and trend filter replace the shared time and trend gates; the
ribbon-break exit replaces shared management.

## Forward use

Every decision reads only bars up to the decision bar, and the act window is
judged at the decision bar's close rather than at the next bar's open. A
forward runner that sets a target (forward paper requires one) should use a
far target such as `target 10R`; the ribbon-break exit is a strategy exit.

## Example

```strat
dsl v7
strategy "Gold EMA Ribbon 1h" {
  description "Gold 1h EMA ribbon with a 4h EMA pair filter, pullback re-entry, a daily short gate and an act window."
}

market conditions {
  slices(XAUUSD 1h)
}

setup {
  type: ema ribbon
  ribbon ema 8 13 21 34 55
  pullback ema 13
  trend filter 4h ema 8 above 21
  shorts below daily ema 50
  act hours 22 to 12 utc
}

risk {
  stop 3 ATR
}

execution {
  risk 200 USD
}
```
