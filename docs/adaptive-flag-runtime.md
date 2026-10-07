# Adaptive baseline raw native/WASM runtime

Canonical task: [HT-195](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-195-adaptive-flag-backtester-adoption.md).
This producer-only Stage A adds a dedicated raw report transport. It does not
add Backtester UI adoption, research accounting, a release/pin or deployment.
It does not qualify TradingView parity, observed fills or profitability.

## Interfaces

```
heisentick adaptive-flag-runtime-report --request-file=request.json \
  --dsl-file=baseline.strat --bars-file=supplied.btb1
```

Each flag is required exactly once. No other option is silently ignored.
WASM exposes `engineRunAdaptiveFlagReport(metadataJSON, sourceText, btb1View)`:
two primitive strings and an ordinary `Uint8Array` of complete six-column BTB1.
Both transports use the same Go `report/adaptiveflag` builder and unchanged
`engine.RunAdaptiveVolumeFlag` state machine. JavaScript is transport only.

The request must contain exactly these fields:

```
{
  "schema": "adaptive-flag-runtime-request-v1",
  "numericalPolicy": "BINARY64_ORDERED_V1",
  "symbol": "XAUUSD",
  "timeframe": "M30",
  "executionWindow": null
}
```

Timeframe is `M30` or `H1` and must equal the DSL. The XAUUSD label is declared
route identity, not proof that anonymous BTB1 rows came from that provider or
market. `executionWindow` is required: null for full supplied input, or exactly
`{"tradeFromMs":0,"tradeToMs":3600000}`. Window tokens are unsigned plain JSON
integers on the declared UTC grid, safe integers, with start < end. Fraction,
exponent, negative-zero, duplicate/unknown/missing keys and trailing input fail.

Only exact named INITIAL, TWEAKED and SNAPSHOT_C bundles execute. All settings
remain explicit in the existing DSL. CUSTOM/lookalikes, changed named inputs,
filter/cap ablations, G research metadata, mixed families, auxiliary contexts,
checkpoint/resume and parameter overrides are refused by this runtime.
The strict runtime parser directly calls the existing closed adaptive parser;
it never enters generic selectors or another family's route expansion. Parser
errors/error diagnostics and invalid config fail before full bar copying.

## Raw-only result and identity

The versioned `strat-adaptive-volume-flag-runtime-v1` envelope carries canonical
request, hashes of exact request/source/full BTB1 bytes, the existing root config
hash/config, `rawOnly: true`, `economics: "unavailable-stage-a"`, and the unchanged
raw run. Original field order, config encoding and raw-reference semantics remain.
The inner effective-config and consumed-prefix row hashes are different
identities from original source/BTB1 file bytes. None replaces the others.

No quantity, starting equity, P&L, fees, spread/slippage, statistics, percentage
return or account settings are accepted, even when zero. The broker's internal
zero-size placeholder is never an economic result. Consumers must not create
zero-cost trades, $10,000 capital defaults or JS-derived performance from this
report. A future Go-owned research projection and Backtester adoption require
the separate HT-195 Stage B/C gates.

The native legacy `adaptive-flag-report` remains available with its original
source/input admission, schemas, report bytes, CUSTOM and native G support.
New baseline runtime support does not admit G through WASM. Generic report,
fixture/column WASM, prepared/shared/prefix/checkpoint routes still refuse the
adaptive family. No existing family changes its execution policy.

## Time, source and model boundaries

The [native reference contract](adaptive-volume-flag-offline.md) remains the
source of indicator, pivot, signal and order-state semantics. In particular,
confirmation is causal, both flag windows retain the source's latest-pivot-HIGH
anchoring, levels remain frozen, creation cannot fill on its own bar, the first
bracket is delayed until entry close, and an activated protective stop latches
a next-observed-open exit. Targets, gaps, high-first ties, expiry/hold counters
and terminal exposure retain their original reference conventions.

The request window uses bar-open `[start,end)`. Supplied earlier prefix rows
warm indicators with a flat broker and no orders. Input at/after end is excluded
before calculation; final included-close pending/open/queued state is not
liquidated. Close availability is open plus timeframe, not next observed quote.
Nonopening events remain time intervals, not invented tick timestamps.

Only retained row values are validated. The cutoff comparison precedes excluded
row validation exactly as in the native reference, so an ignored +Inf boundary
timestamp or arbitrary later values do not invalidate an otherwise valid prefix.
That suffix is not certified. Full BTB1 shape and original-byte identity remain
mandatory. Never silently slice off warmup, aggregate source bars, invent missing
volume, interpolate gaps or substitute a different data revision.

## Bounded runtime admission

- Metadata and source: at most 4,096 UTF-8 bytes each
- Supplied BTB1: 1..16,384 rows, exactly six columns, at most 786,448 bytes
- Retained rows, including supplied warmup: 1..4,096; reject rather than trim
- Retained OHLCV: original finite/positive-price/consistent-OHLC/nonnegative-volume
  conditions, with absolute magnitude at most the binary64 constant `1e100`
- Exact-safe, increasing nonnegative timestamps and nominal closes on M30/H1 grid
- Complete output hard ceiling: 128 MiB

The magnitude cap belongs only to this runtime profile. It bounds intermediate
arithmetic under the fixed named rules without changing native arithmetic or
admission. Subnormals, finite underflow, signed zero and direct comparisons are
not quantized or given an epsilon. No dynamic price/ATR/stop-distance divisor is
introduced; canonical risk sizing is bypassed by the explicit raw zero-size path.

For N retained rows, the reviewed constructors have N snapshots/states, <=N
orders/gaps, <=4N events and <=26 event links per order. Fixed strings, escaped
source/config, scalar lengths, punctuation and pretty whitespace are included.
The conservative maximum at 4,096 retained rows is 93,195,064 bytes, below the
128 MiB ceiling. This admission precedes column/engine/internal JSON allocations;
after-serialization checks provide defense in depth. Constructor/helper source
fingerprints require review before the proof can be renewed after source changes.

These are individual data/output/work bounds, not a 128 MiB process-memory
promise. Engine/serializer/Go heap, transport and JavaScript string/parse copies
can coexist. Browser peak memory/latency and larger-history capacity need their
own measurements. The initial profile does not claim full-history capacity.

The WASM bridge bounds UTF-16 before conversion, rejects unpaired surrogates,
uses intrinsic view accessors, admits the 16-byte header before full copy and
copies only the selected subarray. It rejects shared/detached/proxy/spoofed
transports and catches copy traps. Captured guarded DataView word reads avoid
the Go shim's unguarded bulk-copy exception path. This does not claim security
against arbitrary replacement of an entire executing worker environment.

## Failure and verification

Validation and complete serialization precede success output. Native failure
returns a nonzero error and no partial success document. WASM failure returns
only a closed error object with code `ADAPTIVE_RUNTIME_REJECTED`, phase
request/source/input/resource/execution and a UTF-8 message capped at 1,024 bytes.
A rejected call must leave the runtime usable.

Run formatting, vet, full Go tests and conformance; build native and both WASM
modules. Use `scripts/checks/adaptive-flag-runtime-parity.mjs` with the frozen
invented corpus, and `scripts/checks/adaptive-flag-runtime-transport.mjs` for
actual JS boundary/refusal/recovery checks. Execute adaptive numerical and
lifecycle kernels under Go/WASM as well. Preserve all existing goldens.

Complete-report byte comparisons must use identical source/metadata/BTB1 bytes
on actual normal-toolchain AMD64, ARM64 and Go/WASM targets, with corpus/artifact
identities recorded. Cross-compilation and parser parity do not qualify execution.
Native legacy A/B/C/CUSTOM comparisons are literal bytes; native G preserves all
ordered non-build-provenance bytes while separately verifying truthful compiler,
VCS revision/modified values. Original reports and hashes are retained intact.

This source document specifies gates rather than claiming all targets, browser,
release or application acceptance have passed. Exact qualification receipts and
remaining gates belong to the canonical task. Historical reports are never
relabelled as portable results merely because invented controls agree.
