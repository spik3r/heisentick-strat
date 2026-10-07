# Adaptive volume flag: native raw reference

Tracking: [HT-185](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-185-adaptive-volume-flag-dsl.md).
This is a separate family from the fixed PR388 `goldFlagReference` strategy.

## Run the implemented path

```sh
heisentick adaptive-flag-report --dsl-file=explicit.strat --bars-file=supplied.btb1
# Optional evaluation window, preserving earlier supplied warmup:
heisentick adaptive-flag-report --dsl-file=explicit.strat --bars-file=supplied.btb1 \
  --trade-from=2026-01-01T00:00:00Z --trade-to=2026-02-01T00:00:00Z
```

The source must declare every setting. Complete invented parser examples are
in `conformance/parse/family-adaptive-volume-flag-{initial,tweaked,snapshot_c}.strat`.
The command executes the Go implementation, not merely the parser. Supply
already-formed M30 or H1 bars in exact six-column BTB1 format, including volume.
Without a window the command executes the entire supplied input in order.
It performs no download, aggregation, implicit warmup import or interpolation.
The optional paired window is explicit evaluation metadata, not a strategy
parameter or an instruction to reset source indicators.

Both limits must be exact UTC RFC3339 and on the declared grid, with nonnegative
start < end. Eligibility follows bar-open `[start,end)`. Rows before start
remain in the retained prefix for indicator/pivot warmup, with a flat broker
and no pending/order creation. The first observed eligible row may create at
its close; a gap across start does not invent a row. The first supplied open
at/after end and all subsequent rows are excluded before calculations or
execution. A final included close may retain a pending order or queued exit;
there is no end liquidation or fill on a row at/after end.

Only the retained prefix is value/clock validated; equal column lengths and
the complete BTB1 envelope remain required. A malformed future suffix cannot
change the consumed prefix, and is not certified by a successful prefix run.
An empty retained prefix fails; a nonempty all-warmup prefix can return flat
with zero eligible trade rows. Never substitute slicing at start or filtering
full-history trades for this request boundary.

Each timestamp is an increasing, nonnegative, exact-safe Unix millisecond UTC
bar open on the declared grid. Source OHLC must be finite, consistent and
strictly positive; volume must be finite and nonnegative. Missing grid slots
remain gaps. Nominal close-phase availability is `open + timeframe`, never the
next observation across a gap. Provider latency, quote side and tick ordering
remain unverified. Input is assumed to consist of completed supplied bars;
there is no current-bar completeness oracle.

## Settings identity

- `INITIAL`: frozen source values, including flags 3–16, EMA 50/200, validity 12
  and 1.2 ATR stop buffer.
- `TWEAKED`: flags 5–21, EMA 55/144, validity 20 and 2.2 ATR stop buffer.
  Exactly these six inputs differ from INITIAL.
- `SNAPSHOT_C`: TWEAKED except a 2.0 ATR stop buffer. TWEAKED is not overwritten.
- `CUSTOM`: all 18 settings must still be supplied and validated. No setting
  is silently inferred or clamped.

Named presets are complete immutable identities, not defaults plus overrides.
Changing a named setting requires another matching named identity or explicit
CUSTOM. Identity does not establish TradingView parity, profitability or
qualification of any account property, quantity, fee or execution rule.

## What the output proves

The envelope includes SHA256 of exact DSL bytes, canonical compiled config
JSON and original BTB1 bytes. The run separately hashes its effective rule
specification and canonical ordered `[t,o,h,l,c,v]` rows; these two hashes are
explicitly not original-file identities. Optional executionWindow metadata
records the requested limits; providedSourceRows, usedSourceRows,
ignoredSuffixRows, preTradeRows and eligibleTradeRows distinguish all supplied
rows from the consumed prefix and evaluation rows. First/last retained opens
and last retained nominal close are explicit. InputSHA256 covers only consumed
rows, while the outer BTB1 hash covers the entire supplied file.

The output contains causal indicator/pivot/predicate snapshots, selected order
states, fill opportunities, raw model entry/exit prices, planned trigger-to-stop
and actual-fill-to-stop distances, event ordering/time intervals, observed gaps
and terminal pending/open/queued state. Snapshot candidates are predicates,
not claims that an order was created while another order occupied the strategy.

The engine uses canonical Go broker position creation/closure with an internal
zero-size placeholder, zero costs and no generic management defaults. Economic
records are not exposed. There are no quantity, point-value, cash P&L, return,
equity, risk budget, margin, commission, funding or trading-account claims.
Costs and research sizing need a separate reviewed projection; the CLI rejects
those flags rather than borrowing a generic report's defaults.

## Model boundary

Policy `DELAYED_OHLC_REFERENCE_V1` pins the source quirks and corrected first
protective-stop activation interpretation. See the [family specification](../spec/dsl-spec-families/adaptiveVolumeFlag.md).
Strict plateau rejection, high-first equal-distance paths, delayed stop
activation and price-constrained target handling are explicit reference
conventions. Synthetic agreement with the corrected probe is not an observation
of TradingView, a broker, actual ticks or an executable edge.

The native dedicated runner is implemented. Generic report/grid, shared and
prepared contexts, prefixes/checkpoints, and engine-WASM execution refuse this
family, including empty-data calls and malformed reserved intent. The Go WASM
parser can compile the source but returns the native-only warning. No app UI
adoption, strategy registration, tag, release, pin or deployment is included.

Fixed numerical policy `BINARY64_ORDERED_V1` is separate provenance, not a
strategy parameter. It accumulates the ATR seed left-to-right in binary64,
rounds each operation explicitly, preserves the written Wilder recurrence and
uses direct binary64 comparisons. CPython 3.12 compensated `sum()` can produce
a different seed at boundaries. The independent comparator must explicitly
select this ordered policy; archived/default Python behavior is unchanged.
The supplied source declares Pine v6; its documented comparison rounding is
not silently imported. Neither original-Python nor Pine bitwise parity is
claimed.

Explicit float64 conversions prevent fused multiply-add from discarding an
intermediate rounding, as defined by the [Go specification](https://go.dev/ref/spec#Floating_point_operators).
Native amd64 tests alone do not establish cross-architecture qualification;
that remains a separately recorded gate.

## Validation

```sh
gofmt -l .
go vet ./...
go test ./...
go run ./cmd/conformance check
go build -trimpath -o /tmp/heisentick-adaptive ./cmd/heisentick
GOOS=js GOARCH=wasm go build -trimpath -o /tmp/adaptive-dsl.wasm ./cmd/dslwasm
GOOS=js GOARCH=wasm go build -trimpath -o /tmp/adaptive-engine.wasm ./cmd/enginewasm
git diff --check
```

Tests use invented source patterns and direct synthetic lifecycle controls.
Only the three new parse goldens and their count metadata are added; all prior
parse/run files retain their bytes. Standard Go tests exercise adapter refusal;
actual Node-hosted WASM refusal evidence is distinct from a browser/application
integration check. Independent review and root approval precede publication.
