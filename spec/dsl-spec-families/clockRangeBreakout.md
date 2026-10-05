# Setup family: `clock range breakout` (`clockRangeBreakout`)

Status: proposed (HT-133). Parser, engine and conformance fixtures follow in
the next heisentick-strat PR; this file is the text they are built to.

Part of `spec/dsl-spec.md` §9. Backlog task:
[HT-133](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-133-dsl-clock-range-breakout.md).

## Type phrase

```
setup {
  type: clock range breakout
}
```

## Purpose

Trades a timed range the way a pending-order expert advisor does. Once a
fixed clock range has closed, the setup rests a buy stop above its high and a
sell stop below its low. The first order to trigger opens the trade and the
other is cancelled. The trade is protected by a percent stop and closed at a
fixed clock time.

It is a separate family from `opening range breakout`. That family enters at
the close of a bar that held beyond the range; this one fills at the range
edge itself with a stop order, and holds to a clock time instead of a target.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `clock UTC<±H[:MM]>` (in `market conditions`) | Fixed offset from UTC used to read every clock time of this strategy (`range`, `orders expire`, `close positions at`). No DST. Offset must lie in `[-12:00, +14:00]`. | None: required, an absent clock is a compile error | `clock.utcOffsetMinutes` |
| `range <HH:MM> to <HH:MM>` | The range window on the declared clock, half-open `[start, end)`. `start < end` and both on the same clock day. | None: required | `clockRangeBreakout.rangeStartMinute`, `clockRangeBreakout.rangeEndMinute` |
| `orders expire <HH:MM>` | Unfilled stop orders are deleted at this clock time. | The `close positions at` time; if that is also absent, the end of the range's clock day | `clockRangeBreakout.expireMinute` |
| `buffer <N> pips` | Distance added above the high and below the low for the two stop prices. Pip size is the route instrument's reviewed pip size (XAUUSD `0.1`). | `0` | `clockRangeBreakout.bufferPips` |
| `stop <N> percent` (in `risk`) | Stop distance as a percent of the actual fill price. `0 < N <= 100`. | None: required, any other stop phrase is a compile error for this family | `stop.type = percent`, `stop.percent` |
| `close positions at <HH:MM>` (in `management`) | Close an open position at this clock time. | Off: the position is held until its stop or the end of data | `clockRangeBreakout.closeMinute` |

The family has no target. An absent `target` section means no target; any
`target` phrase is a compile error for this family. Breakeven, trail, partial
and `maxHoldCandles` are not part of v1 and are compile errors when present
(follow-up: HT-134).

### Times after the range end

`orders expire` and `close positions at` name a wall-clock time. Each resolves
to the first occurrence of that time at or after the range end. So with
`range 11:05 to 14:05` on `clock UTC+10`, `close positions at 03:00` means
03:00 the next calendar day. Both must resolve to a time after the range end,
and `orders expire` must not be later than `close positions at`.

### Alignment

The route timeframe must divide the range edges. After converting `range`
start and end to UTC minutes, each must be a multiple of the timeframe's
minutes. `range 03:05 to 06:05` is valid on 1m and 5m and an error on 15m.
The check runs for every `slices(...)` pair. Closes and expiry may fall on any
bar boundary.

## Evaluation

All state is built from closed bars.

1. **Day key.** The clock day of the range start, on the declared clock. One
   setup per day key. A day with no range bars has no setup.
2. **Range.** High and low of the closed bars whose open time lies in the
   range window. If fewer than 90% of the expected bars exist (`(end - start)`
   divided by the timeframe), the day has no setup. Weekends and holidays fall
   out of this rule.
3. **Placement.** Orders are placed at the close of the last range bar, which
   is exactly the range end. If that bar is missing, the day has no setup.
   Buy stop price = high + buffer; sell stop price = low − buffer.
4. **Active window.** An order can fill on bars that open at or after the
   range end and before the expiry time.
5. **Fill.** A buy stop fills when a bar's high reaches its price; a sell stop
   when a bar's low does. The fill is at the stop price, or at the bar open
   when the bar opens beyond the price (a gap). Slippage and fees apply on top
   as for any other fill. If one bar reaches both prices before either has
   filled, the order nearer the bar open fills first; when a magnifier series
   is supplied the finer bars decide.
6. **One trade.** The first fill wins; the other order is cancelled. After a
   fill, an expiry, or a stop-out, the day key is done: no second entry.
7. **Stop.** `stop <N> percent` sets the protective stop at
   `fill × (1 ∓ N/100)`, anchored to the actual fill price. If the entry bar's
   range also reaches the stop, the trade stops out on that bar at the stop
   level. A bar after the entry that opens beyond the stop fills at its open
   (the shared stop-fill rule in §7).
8. **Close.** At the first bar whose open time is at or after the close time,
   an open position is closed at that bar's open. Orders already cancelled or
   expired do not return.
9. **No re-entry in an exit bar.** A bar that closes a position cannot also
   open one.

Position size follows `execution { risk: N USD }`: risk divided by the stop
distance from the actual fill.

## trade.meta

`clockRangeBreakout.rangeHigh`, `rangeLow`, `rangeBars`, `side`, `dayKey`,
`orderPlacedAt` (UTC ms), `exitReason` (`stop`, `clock`, `end-of-data`).

## Compile diagnostics

- `clock` missing, or outside `[-12:00, +14:00]`.
- `range` missing, `start >= end`, or an edge not aligned to a route
  timeframe (message names the pair and timeframe).
- `orders expire` or `close positions at` resolving before the range end, or
  `orders expire` later than `close positions at`.
- `stop` missing or not of the form `<N> percent`; `N <= 0` or `N > 100`.
- A `target`, breakeven, trail, partial or `maxHoldCandles` phrase.
- `sessions(...)`, `trade window ...`, or `local hour ...` present: this
  family reads only its own clock, so these gates would silently disagree
  with it (warning, not error, for `local hour`).

## Example

Brisbane (UTC+10, no DST) time, written so the same instants read as the
BMTrading EA's 03:05–06:05 server time on UTC+2:

```dsl
dsl v7
strategy "Clock Range Breakout - XAUUSD" {
  description "Stop orders at the 11:05-14:05 UTC+10 range edges (03:05-06:05 UTC+2), flat by 03:00."
}

market conditions {
  slices(XAUUSD 5m)
  clock UTC+10
}

setup {
  type: clock range breakout
  range 11:05 to 14:05
  orders expire 03:00
  buffer 0 pips
}

risk {
  stop 1 percent
}

management {
  close positions at 03:00
}

execution {
  risk: 200 USD
}
```
