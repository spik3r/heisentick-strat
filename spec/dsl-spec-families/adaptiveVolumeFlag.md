# Adaptive volume flag

`adaptiveVolumeFlag` implements one source-faithful, configurable native raw
OHLC reference. It is not the earlier fixed `goldFlagReference` recipe.

## Closed source grammar

```strat
dsl v7
strategy "Adaptive input identity" {
  description "Explicit raw reference settings"
}
market {
  adaptiveflag timeframe M30
}
setup {
  type: adaptive volume flag
  adaptiveflag policy DELAYED_OHLC_REFERENCE_V1
  adaptiveflag bundle INITIAL
  adaptiveflag pivotSensitivity 3
  adaptiveflag minPoleATR 1.8
  adaptiveflag minFlagBars 3
  adaptiveflag maxFlagBars 16
  adaptiveflag maxFlagRetrace 0.50
  adaptiveflag useVolumeFilter true
  adaptiveflag useEMATrend true
  adaptiveflag fastEMALen 50
  adaptiveflag slowEMALen 200
  adaptiveflag targetR 2.5
  adaptiveflag atrStopMult 1.2
  adaptiveflag validBars 12
  adaptiveflag maxHold 60
  adaptiveflag atrLen 14
  adaptiveflag volumeSMALen 20
  adaptiveflag volumeSMAMult 0.9
  adaptiveflag flagWidthPoleMult 0.55
  adaptiveflag entryBufferATR 0.05
}
```

Exactly one strategy, market and setup block is required. All 18 settings,
policy, bundle, timeframe and type are mandatory, once each. Description is
optional and becomes an empty string; the name must contain non-whitespace.
There are no generic risk/management blocks, grids, implicit defaults, overrides,
lists, source-feed aggregation, semicolons or trailing ignored directives.

Keywords are case-insensitive; policy and bundle values are case-sensitive.
Timeframe is M30 or H1. Integers are unsigned decimal digits without leading
zeros; decimals are unsigned, finite base-ten decimals (including `.05`), without
exponent notation in source. Booleans are `true` or `false`. JSON-string names
and descriptions, whitespace and hash comments use the existing strict scanner.

`INITIAL`, `TWEAKED`, `SNAPSHOT_C` and `CUSTOM` are input identities described in
[the offline guide](../../docs/adaptive-volume-flag-offline.md). Named identity
matching happens before float64 conversion, so a decimal differing beyond
float64 precision cannot silently claim the same preset. CUSTOM decimals use
nearest float64 values; overflow, underflow-to-zero and negative zero fail.
Lengths are integers from 1 through 1,000,000, with minFlagBars <= maxFlagBars.
MinPoleATR, targetR and atrStopMult are positive; other multipliers are
nonnegative. These resource/domain bounds are not empirical recommendations.

The root config has exactly dslVersion=7, name, description,
setupType=adaptiveVolumeFlag and adaptiveVolumeFlag. The family object has
exactly contractVersion=adaptive-volume-flag-config-v1, policy,
numericalPolicy=BINARY64_ORDERED_V1, timeframe, bundle and rules. Every rule is mandatory; unknown or null fields fail.

The [closed schema](../schemas/adaptive-volume-flag-config-v1.schema.json)
expresses types, numeric bounds and exact preset branches. Runtime decoding
also enforces the min/max relationship, Unicode/raw-number semantics and
float64 admission. Raw JSON callers must use DecodeAdaptiveVolumeFlagConfigJSON
before ordinary map decoding to preserve duplicate-key and number evidence.
Shared strict limits are 1 MiB JSON/source, JSON depth 64, number/token length
1024 bytes and exponent magnitude 10000. Escaped duplicate keys, invalid UTF-8,
unpaired surrogates, trailing documents and malformed numbers are rejected.

Reserved adaptive family/directive intent selects the strict parser even with
damaged punctuation or a later legacy selector. It never authorizes a generic
fallback. Genuine quoted metadata and balanced list values remain opaque.
Successful parsing emits `adaptive-volume-flag-native-dedicated-runner-required`.

## Fixed numerical contract

`BINARY64_ORDERED_V1` is fixed provenance in the compiled configuration and
result, not a tunable strategy knob. ATR seed true ranges accumulate in input
order starting from binary64 zero, rounding each addition; division by length
is a separate rounded operation. Wilder recurrence evaluates multiplication,
addition and division in that written order. EMA and volume SMA likewise
retain explicit operation order. Every new-family arithmetic boundary uses
explicit float64 rounding; no epsilon, decimal quantization or automatic
comparison rounding is applied.

The reviewed Python probe's default on CPython 3.12.14 uses compensated float
summation for its ATR seed. That can differ from this policy, even by one ULP
at an admission threshold. Independent Go comparison uses a separately named
sequential-contract adapter while preserving the frozen Python default and
its archived outcomes. Existing invented-fixture agreement is fixture-scoped.

The frozen file starts with `//@version=6`; “V5” is the strategy title.
Current [Pine type documentation](https://www.tradingview.com/pine-script-docs/language/type-system/)
describes comparison-operand rounding to nine fractional digits, which is not
this direct-comparison model. The exact built-in ATR accumulation/FMA order
is not established by mathematical SMA/RMA definitions. No Pine bitwise or
universal original-Python parity is claimed. Tests retain an independent
nearest-even seed oracle, compensated counterfactual and adjacent-threshold
state divergence; they do not select a numeric policy from performance.

## Source indicators and setup

For ATR length N, initial TR is high-low, later TR is the maximum of high-low
and the two previous-close gaps. The first N true ranges seed Wilder ATR at
index N-1; later values are `(prior*(N-1)+TR)/N`. Recursive EMA starts at the
first close with alpha `2/(length+1)`. Volume SMA includes current volume and
requires a full window. No generic engine indicator or session defaults apply.

A pivot at i-sensitivity is confirmed only after all symmetric right-side rows
exist, retaining its original occurrence index. Plateaus are rejected strictly
as an explicit reference convention. The bullish pole ends at the moving
high[i-sensitivity] and starts at latest pivot low; bearish uses latest pivot
high minus moving low[i-sensitivity]. The source's pivot ordering and inclusive
age <= maxFlagBars+sensitivity+6 are preserved.

Both flag windows are anchored to bars since latest pivot HIGH, clamped to
minFlagBars..maxFlagBars. The window includes the current row. These lengths
are lookback clamps, not textbook minimum/maximum pattern duration. Source
retrace and width thresholds use <=; negative retraces are not clamped away.
Trend tests preserve `(fast > slow OR close > fast)` and their bearish mirror.
The volume threshold is strict `currentVolume > SMA*multiplier` at creation,
not at eventual breakout. Disabled filters truly bypass their predicates.

The flat, no-pending close selects long before short, freezes trigger at the
flag edge plus directional entryBufferATR, SL at the opposite edge plus
atrStopMult, and TP at trigger plus directional planned distance*targetR.
The target is not recalculated from an opening-gap fill. Snapshot candidates
may exist while occupied, but cannot create additional orders.

## Pending and delayed execution

Creation close cannot fill its own entry. First opportunity is the next
observed row. Pending prices and filters are not updated. A stop entry gaps
to open; otherwise a touched trigger fills at the trigger. The flat close
increments age after the row's fill opportunity and cancels only when
age > validBars, so INITIAL has 13 opportunities and TWEAKED/C have 21.
Expiry can rearm on that same close. An exit can likewise allow new creation
at its close, including the final supplied bar; no next bar is fabricated.

The filled position has no active bracket during its entry bar. At entry-bar
close the first bracket is created. If that close is at/beyond SL (inclusive),
a market exit latches for the next observed open even if price recovers.
Earlier intrabar touches cannot activate a bracket that did not yet exist.
A TP limit does not acquire this market-stop latch. The retained TP convention
is price-constrained and is not newly established TradingView parity.

An established bracket follows open, nearer extreme, other extreme, close;
equal distances select high first. Strict gap-through SL or TP fills at open,
including favorable target improvement; an opening equality is a barrier touch.
The first crossed barrier wins. Queued market exits run before those brackets.
There is no tick reconstruction or guarantee about the actual intrabar path.

At the entry close the hold counter is zero. Hold H queues a market close at
E+H close after that row's bracket checks, and fills at E+H+1 observed open.
Gaps/weekends do not become extra rows. Terminal pending, open and queued
exits remain represented with no automatic liquidation or closed-trade metrics.

## Optional evaluation window

`AdaptiveFlagRequest.Window` is an optional typed Go request with
`TradeFromMS` and `TradeToMS`; native CLI uses paired `--trade-from` and
`--trade-to` exact UTC RFC3339 flags. It does not enter strategy config/rules.
Its closed result projection is `{tradeFromMs, tradeToMs}` or null for full
input. Limits are nonnegative exact-safe millisecond grid points with
start < end. The native interface does not accept a raw JSON request document.

Use the frozen row-open `[start,end)` convention. Indicators/pivots process
all retained prefix rows continuously from the first supplied bar. Rows before
start cannot create orders and the broker remains flat, so no warmup order or
position carries into evaluation. Source indexes never reset. Creation on the
first observed eligible row uses that row's close; a gap across start creates
no synthetic decision. Occupancy is not reconstructed by post-filtering trades.

The first supplied open at/after end cuts the series before any indicator or
broker processing. Only retained row values/timestamps are validated; column
shape and the complete native BTB1 byte envelope remain mandatory. A malformed
ignored tail has no bearing on this consumed-prefix result and is not certified.
An invalid timestamp encountered before the cutoff fails; no empty retained
prefix is admitted. A nonempty prefix may be all warmup and produce no orders.
A final included close (possibly exactly at end) may create an order retained
as terminal pending. The excluded end-open cannot fill it, close exposure or
advance an expiry/hold clock. There is no window-end liquidation.

Results report the requested window, provided/consumed/ignored row counts,
pre-trade and eligible row counts, first/last retained opens, last nominal
close and consumed-prefix input hash. The outer CLI hash always identifies the
entire supplied BTB1 file, including any excluded suffix. Earlier observations
outside the supplied file are never fetched or implied as warmup.

## Evidence and execution boundary

Raw outputs keep causal snapshots, order and state IDs, prices, distances,
fill opportunities and events. Open events have point model time; nonopening
crossings have an open interval between supplied open and nominal close, not a
fabricated tick time. Decisions at close are nominal model points. Missing
intervals neither prove no price events nor provide a qualified finer witness.

The canonical Go broker owns position creation and closure only. The adapter
uses zero costs/zero-size internal bookkeeping and exposes no economic fields.
No cooldown, stop clamp, session/grade filter, break-even, trail, resizing or
terminal liquidation is imported from a generic family.

Only RunAdaptiveVolumeFlag and the native adaptive-flag-report command execute
this policy. Generic execution, report/grid, prepared/shared contexts,
prefix/checkpoint and engine-WASM calls fail before generic defaults or empty
input shortcuts. WASM parsing support is not WASM execution support. All old
families and corpus records retain their semantics. No app adoption or release
is implied by this producer implementation.
