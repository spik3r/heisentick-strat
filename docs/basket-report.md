# Basket reporting v1

This statistics-only Go API consumes closed exit records with caller-verified
**net** P&L. It does not execute a strategy, calculate fills or costs, convert
currencies, inspect files named in metadata, or authenticate caller identities.

## Entry points

- Native: `enginewasm --basket-report request.json`
- WASM: `engineBuildBasketReport(requestJSONString)` returns a JSON string
- Go: `report.DecodeBasketReport(bytes)` or `report.BuildBasketReport(request)`

The shared JSON decoder returns `basket-report-result-v1`, or that schema with
an `error` object containing `code: "invalid-request"` and a diagnostic message.
Success includes the SHA-256 of the exact request bytes as `requestSha256`.
The typed Go builder has no request bytes, so its `requestSha256` is empty.

## Request

The exact top-level fields are `schema` (`basket-report-request-v1`), `nowMs`,
`identity`, `legs`, optional `bar`, and optional `sourceIsDsl`.
`nowMs` is explicit epoch milliseconds; use one captured value for every stage
of a multi-leg report. No wall clock is consulted.

Each leg has `symbol`, `tf`, `bars`, `identity`, `trades`, and optionally
`dataSpan`, `provenance`, `harshTrades`, and `harshIdentity`.
`dataSpan` contains nullable epoch-millisecond `firstBarT` and `lastBarT`.
A missing/null harsh array means the pass was not supplied; `[]` means an empty
pass was supplied. Identity and provenance are opaque JSON objects, echoed in
full. They are claims, not authentication or independent verification.

Each trade requires finite numeric `entry`, `size`, `pnl`, and integer epoch-
millisecond `entryT`. Optional fields are `exitT`, `entryIndex`, `exitIndex`,
`side`, `tag`, nullable `initialSl` and `sl`, `partial`, and `status`.
`status` is `closed` (also the default), `open`, or `prefix`. Open and prefix
records are excluded from cash counts, risk metrics, and year buckets, and
counted separately in `excludedTrades`. The caller must label such records;
the report API cannot infer whether an otherwise closed-looking record came
from an unfinished strategy run.

Calculation objects reject unknown fields, duplicate keys, incorrect casing,
wrong types, and unexpected null values. Only the explicitly opaque metadata
objects allow arbitrary nested field names. Bounds are 32 MiB per request,
128 legs, 100,000 total primary-plus-harsh input rows, 64 KiB per metadata
object, JSON depth 32, and timestamps from the epoch through UTC year 9999.

## Statistics and limits

- R is net `pnl / (size * abs(entry - initialSl))`. Only a missing/null
  `initialSl` falls back to `sl`. Missing stops, zero risk distance, or
  non-positive size are unmeasurable. These rows still count in cash and total
  trades. A numerical stop price of zero is valid when its risk distance is
  nonzero. Nonfinite request values are rejected.
- Partial exits use the entry index, entry time, side, tag, and entry price as
  their grouping key. P&L and size are summed; the initial stop is preserved.
  Only identities containing partial rows are merged. These groups must include
  exactly one final non-partial exit and
  have invariant initial-stop metadata. When the initial stop is absent, the
  fallback stop must remain invariant; otherwise reconstruction fails closed.
  Unrelated non-partial rows are never merged, including when another entry
  contains partial exits.
- Per-leg R drawdown follows reporting-row order, starting at zero R. Merged
  entries retain the order of their first occurrence. No artificial cross-leg
  drawdown is reported.
- Profit factors recombine gross profit and loss. Positive profit without loss
  is represented by `pfR: null, pfRReason: "no-losses"` (or `pf`/`pfReason` for
  cash). Empty and all-flat samples have PF zero.
- Per-leg R expectancy divides by measurable trades. Aggregate and per-symbol
  R expectancy divide by all closed reporting trades, including unmeasurable
  ones. Harsh expectancy divides by measurable harsh trades.
- Years use the **UTC entry year**, regardless of exit time. `complete` means
  only that the calendar year is before the UTC year of `nowMs`; it does not
  certify full-year data coverage. Current/future years remain in totals but
  do not count toward completed-year stability.
- `recentNegativeYearRun` examines at most the last **two observed** completed-
  year buckets. It is not an arbitrary-length streak, and missing calendar
  years are not filled with invented zeroes.
- Frequency uses offered bar coverage, not the span between trades, with a
  365.25-day year. Per-symbol frequency uses the longest leg coverage after
  its two-decimal rounding. Decimal rounding preserves the exact binary input
  value's nearest two-decimal result, with midpoint ties away from zero.
- Empty measurable harsh evidence for a primary-trading leg, unmeasurable
  harsh rows, and excluded open/prefix harsh rows mark `missingHarsh` true.
  `harshUnmeasurableTrades` and `harshExcludedTrades` expose those gaps on legs
  and the aggregate. Measured aggregate harsh statistics remain descriptive;
  incomplete evidence fails both harsh verdict components, and the affected
  symbol's `harsh` is null.
- All cash values retain their supplied units. There is no currency conversion.
  A nonfinite risk calculation is unmeasurable and is counted as such.
  Overflow in any reported aggregate metric fails the entire report rather
  than emitting misleading null or infinite metrics.

## Optional configured verdict

There are no policy defaults. When `bar` is present, all fields must be supplied:
`basket`, `minAggregateTrades`, `minAggregatePfR`, `minPerSymbolPfR`,
`minTradesPerYearPerSymbol`, `minAggregateHarshExpectancyR`,
`minPerSymbolHarshPfR`, `requireAggregateYearStability`,
`maxTrailingNegativeYearsPerSymbol`, and `requireDslSource`.

The result then includes `verdict.pass`, named `components`, and failure
`reasons`. Harsh aggregate expectancy uses a strict greater-than threshold;
other numeric floors use greater-than-or-equal. Coverage rejects missing or
unexpected symbols and duplicate routes. Source availability is the caller's
`sourceIsDsl` assertion. Configuring a trailing-year limit does not expand the
fixed two-observed-year measurement window described above.

## Reproducible native/WASM check

After building native and WASM binaries with a matching Go toolchain:

```sh
node scripts/checks/basket-report-parity.mjs /path/to/enginewasm /path/to/enginewasm.wasm /path/to/wasm_exec.js
```

The parity check uses invented fixtures only, including refused partial
reconstruction and incomplete harsh-evidence counterexamples.
