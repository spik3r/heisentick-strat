# Gold flag reference

`goldFlagReference` is a Go-only, native-offline reference family tracked by
HT182. Its sole policy, `PR388_CAUSAL_STRESS_V1`, is one indivisible causal OHLC
stress scenario. It is not a tunable strategy, a profitability claim, or evidence
of actual broker fills. The source reference and finer-feed investigations are
not execution providers for this family.

## Complete source

```strat
dsl v7
strategy "Gold flag reference synthetic control" {
  description "Synthetic parser fixture; dedicated native runner only"
}
market {
  goldflag timeframe M30 from M15
}
setup {
  type: gold flag reference
  goldflag policy PR388_CAUSAL_STRESS_V1
}
```

Exactly these three blocks are required, once each. Block and keyword case and
whitespace are insignificant; blocks and setup directives may be reordered.
The strategy name is a nonempty double-quoted JSON string. The optional
strategy description is also double-quoted and projects as `""` when omitted.
The policy token is unquoted and case-sensitive. Hash comments are supported.
There are no literal semicolons, implicit defaults, extra blocks, risk/cost
phrases, parameter lists, overrides, or grids.

Parsing emits exactly one native-only warning:
`gold-flag-reference-native-dedicated-runner-required`. Malformed or mixed
reserved intent is routed to the strict scanner and rejected with an empty
configuration. It never falls back to a generic family. Legacy metadata and
balanced named-list values remain opaque to family selection.

## Closed projection and input boundaries

The configuration root has exactly `dslVersion`, `name`, `description`,
`setupType`, and `goldFlagReference`. The family object has exactly
`contractVersion`, `policy`, and `profile`. Contract version is
`gold-flag-reference-config-v1`; `setupType` is exactly `goldFlagReference`.
Every field in the complete profile is mandatory and immutable, including nested
objects and the ordered diagnostic-cost array. Every parse/profile call returns
independent maps and arrays.

The contract schema is
[`gold-flag-reference-config-v1.schema.json`](../schemas/gold-flag-reference-config-v1.schema.json).
Call `DecodeGoldFlagReferenceConfigJSON` before ordinary map decoding when raw
JSON is the input. It rejects duplicate decoded keys (including escaped-key
collisions), unknown/missing/null fields, trailing documents, invalid UTF-8,
unpaired Unicode surrogates, incorrect scalar types and numeric mutations
before floating-point rounding can hide them. Positive equivalent decimal
spellings are allowed, for example `14.0` and `1.4e1`. Negative zero and alternate
fixed-zero spellings are rejected by the shared strict-number contract. JSON
is limited to 1 MiB, nesting depth 64, numeric tokens 1024 bytes, and exponent
magnitude 10000. JSON Schema does not replace these raw-input checks.

`DecodeGoldFlagReference(Config)` admits only the full closed projection and
returns `GoldFlagReferenceSpec{Policy: "PR388_CAUSAL_STRESS_V1"}`.
`IsGoldFlagReferenceReserved` recognizes canonical and damaged case/space/hyphen/
underscore family identities before generic defaults or empty-data shortcuts;
it does not make those identities valid configuration spellings.

## Bars, readiness and causal context

- Selected input is finite, positive-price, ordered, unique M15 OHLCV at exact UTC M15
  opens. Selection is an explicit half-open range at UTC M30 boundaries, before
  aggregation. No observations before its start are used.
- M30 is UTC phase zero, aggregated first-open / maximum-high / minimum-low /
  last-close / summed-volume. Partial buckets remain. Whole missing buckets
  are not fabricated and do not reset observed-row history. Missing intervals
  do not prove absence of price events and do not cancel pending orders.
- Availability and signal decisions use nominal M30 close. An opening event
  uses the first actually observed constituent open, which can be later than
  the nominal bucket open. Provider publication latency remains unverified.
- Wilder ATR14 first becomes valid at observed index 14, seeded by the mean of
  TR indices 1 through 14. Index zero is excluded from the seed. Subsequent
  values use `(priorATR * 13 + currentTR) / 14`.
- H4 and UTC-day bars are independently aggregated from observed M30 rows.
  Context uses the latest nominal close no later than the M30 decision close,
  then the extrema of the last 12 H4 bars or last five daily bars. A partial
  context bucket becomes available at its nominal close; a future incomplete
  bucket is never consumed. The horizon is clock-based even across gaps.

## Fixed pattern and signal

The six observed pole rows immediately precede six observed flag rows ending
at the signal. Pole and flag sizes are their high-minus-low ranges. The first
occurrence wins an extrema tie. A long requires the first pole low before the
first pole high; a short requires the reverse. Equal extrema indices do not
establish a direction.

All thresholds are inclusive:

- Pole range is at least 2 signal ATR
- Flag range is at most 0.6 pole range and at most 1.5 signal ATR
- Long flag low is at least pole high minus half the pole range
- Short flag high is at most pole low plus half the pole range

At least one available H4/daily context must qualify. Proximity is directional:
long flag high must be at least context high minus signal ATR; short flag low
must be at most context low plus signal ATR. This is a one-sided threshold,
not an absolute-distance cap beyond the level. There is no DE/ATR or volume
filter.

The stop entry is the directional flag edge plus an outward 0.1 signal ATR
buffer. The opposite flag edge plus its outward 0.1 ATR buffer is the fixed
stop/opposite boundary. Planned risk below 0.4 signal ATR is rejected.
Every selected signal consumes cooldown, including invalid-risk signals: the
next six observed rows cannot select another signal, so the earliest next
selection is prior signal index plus seven. Only one position is allowed.
The last observed row manages existing exposure but cannot create a new order.

## Pending order and reference execution

A pending order activates on the next observed row. The four observed rows
at signal indices +1 through +4 are eligible, inclusively. Calendar time and
missing buckets do not shorten that window. If fewer eligible rows remain at
the end, the order remains pending rather than expiring early.

The opening is known before unordered later extremes. An opposite opening
cancels first. An entry-side opening gap fills at its actual observed opening
price. An intrabar entry touch fills at the entry level; an opposite-only touch
cancels. The signal stop stays fixed. Risk and the 2R target are calculated
from the actual fill, so an entry gap changes both.

On each live row, a known opening stop or target takes precedence over later
extremes. An opening target exit is capped at the target: no opening-price
improvement is credited. The named ambiguous scenario is entry-first,
stop-first. Same-row entry versus cancellation and unordered stop versus target
retain their local OHLC-feasible alternatives. Those alternatives are not
propagated portfolio bounds, fill observations, or a reconstructed tick path.

The time exit is fill index plus 24 observed rows, after bracket checks on that
row. The exit row cannot select a new signal. Open positions remain open at
terminal data, with no terminal liquidation. Reference prices are continuous
float64 values without tick rounding. Position quantity is one reference unit,
with no equity feedback. The bracket attaches at fill and the stop never trails. Existing native broker primitives own
position/bracket/time-exit accounting; this family is not a second runtime.

## Diagnostic costs and timing evidence

Diagnostic cost is an explicit external report request chosen from exactly
0, 0.06, 0.15, 0.25, or 0.50 price points per fill. These values are deliberately
absent as tunable DSL/config fields. A closed trade deducts two fill costs from
its reference P&L; cost cannot change signals, fills, brackets, risk, or sizing.
These are fixed sensitivity values, not measured spread/fees/financing.

Event-time evidence distinguishes known observed openings `[t,t]`, proven
nonopening crossings `(observedOpen,lastObservedClose)`, coarsely saved
zero-gap fills `[observedOpen,lastObservedClose)`, and nominal-close decisions
`[close,close]`. The zero-gap coarse interval does not assert that execution
occurred at the open. None establishes actual provider latency or broker time.

## Supported route and regression contract

Only the dedicated native offline runner admits this family. Generic execution,
grid expansion, prefix execution and WASM/browser execution refuse reserved
family intent before generic fallback. The manifest's family, both Go setup
aliases and both directive heads are `go-only`; adding passive grammar entries
does not advertise consumer execution support.

The single synthetic parse fixture is
`family-gold-flag-reference-pr388-causal-stress-v1`. It is generated by the
repository's Go conformance workflow. Existing parse/run goldens must remain
byte-identical. Native parser tests cover strict selection, complete projection,
raw JSON and exact numbers, immutable nested values, schema mutations and
legacy-source preservation. Engine fixtures/tests separately establish the
execution policy; a parse golden alone is not executable strategy coverage.
