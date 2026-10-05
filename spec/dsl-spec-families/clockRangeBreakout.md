# Setup family: `clock range breakout` (`clockRangeBreakout`)

Status: documentation-only v1 contract (HT-167). Parser, engine and
conformance fixtures are not implemented by this spec PR. The requirements
below are acceptance criteria for a separately reviewed implementation.

Part of `spec/dsl-spec.md` §9. Backlog task:
[HT-167](https://github.com/spik3r/heisentick-backlog/blob/claude/ht133-clock-range-breakout/tasks/HT-167-dsl-clock-range-breakout.md).

Task identity note: originally proposed as HT-133, now HT-167 to resolve a
backlog ID collision. Deferred extras are HT-168, originally proposed as
HT-134. Earlier commits and branch names retain those historical IDs. The
identifier-only correction was separate from this reviewed v1 contract.

## Type phrase and purpose

```
setup {
  type: clock range breakout
}
```

After a fixed-clock range closes, place pending buy/sell stop orders above
its high and below its low for the permitted sides. The first modeled fill
wins and cancels the other order (OCO). Protect the trade with a percent stop
and require a clock close. At most one entry is allowed per range day.

This is separate from `opening range breakout`, which enters after a bar
holds beyond its range. Existing families, their defaults and their
conformance goldens retain their existing semantics.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `clock UTC<±H[:MM]>` (in `market conditions`) | Integer UTC offset minutes in `[-720, 840]` (`[-12:00, +14:00]`), no DST | Required | `clock.utcOffsetMinutes` |
| `range <HH:MM> to <HH:MM>` | Same declared-clock date, half-open `[start, end)`, `start < end` | Required | `clockRangeBreakout.rangeStartMinute`, `clockRangeBreakout.rangeEndMinute` |
| `orders expire <HH:MM>` | Expire unfilled orders at the resolved time | Required close time | `clockRangeBreakout.expireMinute` |
| `buffer <N> pips` | Finite, nonnegative distance in the instrument's reviewed pips | `0` | `clockRangeBreakout.bufferPips` |
| `stop <N> percent` (in `risk`) | Finite percent of the actual post-slippage fill, `0 < N <= 100` | Required | `stop.type = percent`, `stop.percent` |
| `close positions at <HH:MM>` (in `management`) | Required clock liquidation of an open position | Required | `clockRangeBreakout.closeMinute` |
| `side long only`, `side short only`, `side both` (in `filters`) | Static side allowance; only allowed sides receive an order | Both | `allowLong`, `allowShort` |

## Family defaults and supported directives

Selecting this family neutralizes inherited named-session/trade-window,
context/day-type/movement/candle/grade gates, entry-distance/setup-age gates
and cooldown defaults. It disables target, breakeven, trailing, partial exit,
max-hold and ATR stop padding/clamping. In particular, the shared default
1R target and 0.75R breakeven must not leak into this family.

The v1 directive surface is the family phrases above, strategy metadata,
route selection (`slices`, or shared `symbols`/`timeframes` selection), static
side allowance, and shared fixed-USD execution risk. Those route restrictions,
the family clock, range coverage, pending-stop lifecycle, one-position broker
constraint and percent-risk rules are the only admission/management inputs.
No additional market/context/candle/level/grade filters or entry modes are
supported. Explicit unsupported directives are compile errors in any section
order, including `sessions`/`windows`, every `trade window`, `local hour`,
weekday/New York/session-phase gates, target, non-percent stops, ATR stop
bounds, breakeven, trail, partial, cooldown and `maxHoldCandles`. They must
never be ignored or downgraded to warnings, even if they request an
apparently neutral value.

Static side and route admission are evaluated at placement, independently
for each permitted side, using only information available then. The completed
fill bar's candle quality, context, high/low or close cannot retroactively
admit an earlier intrabar order. V1 has no bar-derived admission filters;
a future extension must define their placement-time causal inputs explicitly.

## Clock, day key and alignment

Offset hours are integer tokens, with an optional two-digit minute component
`00`–`59`; the signed total must be within `[-720, 840]`. Thus `UTC+5:30` is
valid, `UTC+14:01` and `UTC+2:60` are invalid. Wall-clock times use strict
`HH:MM`, `00:00`–`23:59`; `24:00` and malformed/fractional tokens are invalid.
Clock conversion is offset arithmetic on UTC timestamps, independent of the
host timezone and daylight saving time.

The range start and end are on the same declared-clock date and require
`start < end`. Its day key is that date formatted `YYYY-MM-DD`. A range may
cross UTC midnight even though it does not cross midnight on its own clock.
For each range day, resolve expiry and close on that clock date. If a time
is earlier than range end, advance it by one clock calendar day. Equality
with range end is invalid. After resolution require:

```
rangeEnd < expiry <= close
```

Expiry defaults to the required close. For `clock UTC+10` with range
`11:05 to 14:05`, close `03:00` resolves to the next calendar day. This
represents the same instants as UTC+2 range `03:05 to 06:05`, close `19:00`.
Equivalent offsets/times must produce identical price/time trades.

Range start, range end, expiry and close must each align to every effective
route timeframe's UTC bar grid after offset/date conversion: UTC timestamp
in milliseconds modulo timeframe duration in milliseconds equals zero. This includes
explicit `slices(...)` and implicit symbol/timeframe routes, including any
shared route defaults. For example the equivalent range above is valid on
1m/5m but not 15m; expiry or close at `:02` is invalid on 5m. No v1 bar may
span an expiry/close boundary and use later prices before that boundary.

## Range formation and placement

Use fully closed, on-grid bars with open timestamps in `[rangeStart,
rangeEnd)`. Compute `expectedBarCount = (rangeEnd - rangeStart) / timeframe`
and require at least `ceil(0.9 * expectedBarCount)` distinct slots, including
the final expected slot immediately before range end. The end-time bar is
excluded from the range and may be the first entry bar.

Apply existing shared series validation where it provides a check (including
column shape and finiteness). Additionally, this family requires strictly
increasing, distinct execution-bar open timestamps aligned to its declared
UTC timeframe grid and valid OHLC bounds (`low <= min(open, close)` and
`max(open, close) <= high`). Supply family-scoped checks for guarantees the
shared validator does not provide; do not tighten existing families' input
contracts. Reject malformed/duplicate/off-grid data rather than count it as
coverage or silently repair it. Empty, insufficient-coverage,
missing-final-slot and zero-width (`rangeHigh <= rangeLow`) ranges produce
no setup. Every rejected day has a stable diagnostic reason. Do not inspect
later bars or require a future closing quote to decide whether a setup exists.

Place once at `rangeEnd`, after the final range bar closes. Buffer conversion
uses the shared reviewed registry in `engine/instrument_pips.go`; an unknown
instrument pip size fails closed, even when the buffer is zero. For XAUUSD,
`pipSize = 0.1`, so `10 pips = 1` price unit. The unrounded triggers are:

```
buyTrigger  = rangeHigh + bufferPips * pipSize
sellTrigger = rangeLow  - bufferPips * pipSize
```

Prices retain the shared float64 modeling and output-serialization policy.
This family introduces no broker tick/lot quantizer. Triggers and effective
fills must be finite and positive; an invalid allowed-side placement rejects
the day rather than silently changing the requested pair of orders.

Only statically allowed sides receive an order. If placement is rejected or
broker-blocked, including by a prior position, mark the day done and do not
queue a later placement. A fill-time broker rejection or invalid effective
fill likewise cancels the pending pair and consumes the day; no retries.
The family never opens a second position alongside an existing position.

## Pending orders, fills and stops

1. Orders are eligible only on bars with open timestamps in
   `[rangeEnd, expiry)`. At every available bar open, expire pending orders
   and execute an already-open position's due clock close before new entry
   or intrabar price tests. A bar that closes a position cannot open another.
2. A long trigger is touched when `barHigh >= buyTrigger`; a short trigger
   when `barLow <= sellTrigger`. Among allowed, pending orders, a long fills
   at `max(buyTrigger, barOpen)` and a short at
   `min(sellTrigger, barOpen)`, then shared adverse entry slippage applies
   exactly once. These stop orders do not use market-order `costs.fillOn`.
3. If both pending triggers are touched in the same bar, compare absolute
   distances from `barOpen` to the trigger prices. Choose the nearer trigger;
   exact distance ties choose long. Cancel the other order immediately on
   entry. This is a deterministic OHLC modeling convention, not evidence of
   the actual intrabar order.
4. Let `p` be the actual post-slippage entry fill and `q` the stop percent.
   Require finite positive `p` and finite `0 < q <= 100`. Stop distance is
   `p * q / 100`; SL is `p - distance` for long and `p + distance` for short.
   Distance must be finite and positive and the resulting SL finite. Size is
   `riskUsd / distance` under the existing shared risk-presence and fee
   contract, with no ATR clamp or grade scaling. An explicit zero risk stays
   zero and is not replaced by default risk. Shared invalid-size admission
   still applies.
5. If the entry bar reaches SL, exit at SL with shared exit costs. This
   pessimistic whole-entry-bar rule includes a possible pre-entry extreme.
   On later bars an open beyond SL exits at that open; otherwise a touch
   exits at SL, with shared exit costs. Entry slippage is not reapplied.
6. Entry, expiry or rejected placement consumes the range day. A stop or
   clock close cannot re-arm it. At most one entry per day key, with no retry
   after a failed fill and no entry in an exit bar.

V1 has no magnifier input or finer-series fallback. Independent 1m and 5m
runs use these same rules on their respective execution bars. A 1m run does
not reveal intraminute ordering. Any future magnifier needs a separately
reviewed, versioned input/coverage/chronology contract shared by Go, WASM and
JS; existing source-timeframe mechanisms do not implicitly enable one here.

## Clock close, finalization and result schema

Close an open position at the first available bar open at or after its
resolved clock close, applying shared exit costs before intrabar stop tests.
A missing clock-close quote can therefore delay liquidation beyond the
requested time. If the dataset ends before such a quote, actual run
finalization liquidates at the last available bar's close with shared exit
costs. Never drop or omit the open trade. Prefix/incremental/resume evaluation
retains open state and does not terminally liquidate until actual finalization.

Retain shared top-level trade reasons: a stop uses the existing stop category,
a clock exit uses `reason: "rule"` and `rule: "clock-range-close"`, and terminal
liquidation uses `reason: "end-of-test"`. As in the shared contract, `rule` is
omitted for non-rule exits. Absent `tp` and `initialTp` serialize as `null`.

Use one nested `trade.meta.clockRangeBreakout` object identically in Go,
WASM and JS, containing `rangeHigh`, `rangeLow`, `rangeBars` (distinct admitted
range bars), `side` (`long` or `short`), `dayKey`, `orderPlacedAt` (UTC milliseconds, `rangeEnd`)
and `exitReason` (`stop`, `clock` or `end-of-data`). This family `exitReason`
is separate from the existing top-level reason/rule contract. Trade indices
and entry/exit timestamps retain shared execution-bar conventions.

## Diagnostics

Compilation fails with stable diagnostics for missing clock/range/percent
stop/required close; malformed or out-of-bounds clock/time tokens; local
start >= end; equality or invalid resolved expiry/close ordering; any of the
four boundaries off an effective route grid (name the pair/timeframe);
negative/nonfinite buffer; unsupported pip route; nonfinite or out-of-range
percent; and any unsupported explicit directive, regardless of section order.
Unknown runtime routes must also fail closed rather than invent a pip size.
Runtime data/placement diagnostics distinguish malformed series, empty range,
insufficient coverage, missing final slot, nonpositive range width, invalid
price/distance/size and broker-blocked placement. Rejected days and expired
orders must be observable without inventing trades.

## Example (proposed syntax, not yet implemented)

UTC+10 range 11:05–14:05 / next-day 03:00 close represents UTC+2 range
03:05–06:05 / 19:00 close. The 5m example is a separate resolution from a 1m
run, even when the wall-clock schedule is identical.

```dsl
dsl v7
strategy "Clock Range Breakout - XAUUSD" {
  description "UTC+10 range 11:05-14:05; close at first quote from next-day 03:00 (UTC+2 03:05-06:05, close 19:00)."
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

## Required conformance fixtures and acceptance

These are fixture requirements, not implemented tests in this documentation
PR. Reviewed hand-derived synthetic semantics are authoritative; Go-generated
parse/run goldens must satisfy them before native/WASM/JS parity is accepted.
All existing-family goldens must remain unchanged. The implementation must
provide these twelve groups, with explicit resolution/costs and exact expected
config, diagnostics, fills, size, stops, reasons, timestamps and metadata:

1. UTC+10/UTC+2 equivalence, year/month boundaries, negative/fractional
   offsets, local ranges crossing UTC midnight, and rejection of local
   start >= end.
2. Missing required phrases, malformed/out-of-bounds times/offsets,
   negative/nonfinite buffer, unsupported pip routes, invalid percent and
   forbidden target/management/gates in every relevant directive order.
3. All four boundaries aligned for explicit slices and implicit routes;
   valid 1m/5m baseline, invalid 15m baseline and rejected 5m `:02` expiry/close.
4. Of 36 expected 5m slots: 33 including the final slot accepted, 32 rejected,
   35 without the final slot rejected; no bars/flat range rejected; end-time
   bar excluded from range but eligible for first fill. Invalid series cannot
   inflate coverage; duplicate/unordered/off-grid timestamps and inconsistent
   OHLC fail this family's added checks, without changing other families'
   validation.
5. Ordinary and gap-through long/short fills, both-edge equal-distance tie,
   first-side OCO, side-only allowance, expiry boundary exclusion, rejected
   placement/prior-position blocking and no retry after fill, stop or expiry.
6. Entry-bar pessimistic stop, later gap-stop at open, clock close before that
   bar's high/low and no entry in an exit bar.
7. Missing clock-close quote exits at next actual open; no later quote exits
   at terminal last close with one recorded trade; prefix/resume retains
   state until finalization, with no silent omission or early liquidation.
8. With zero buffer/costs, range 99–101, entry-bar open 100/high 102/low 98:
   long wins the tie, entry 101, 1% SL 99.99, exactly one stopped trade.
9. Nonzero pip conversion; slippage changes actual fill, percent stop and
   size; explicit zero risk follows the shared presence contract; no ATR
   clamp, default target, breakeven or grade sizing leaks into results.
10. Bare example fills in UTC+10 mid-session and outside inherited named
    windows; unrelated fill-bar candle/context changes cannot retroactively
    admit an order. Stop fills are independent of market `costs.fillOn`.
11. Exact Go/native-WASM/JS parsed config and trade-envelope parity under the
    shared numeric serialization policy, including null targets, reason/rule,
    nested metadata, indices/timestamps and tie behavior; old goldens unchanged.
12. Historical comparison lists every changed trade/day, uses matching
    resolution and raw-fill cost assumptions, separates intended spec
    differences from baseline defects, and leaves zero unexplained differences.

Historical simulators are archival comparison baselines, not semantic oracles.
Keep a pinned reproducible archive and synthetic defect regressions before
retiring a standalone script. A percentage-of-days agreement threshold must
not hide dropped or extra trades. Compare raw fills first: price-unit PnL and
fixed-dollar risk sizing can yield different profit factors even for identical
fills. Cost/resolution differences must also be explicit. Language support
and parity alone provide no edge, promotion, basket, forward-test or live
execution qualification.
