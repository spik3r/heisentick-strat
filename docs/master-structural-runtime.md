# Dedicated Master Structural runtime report

Canonical task: [HT-183](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-183-master-structural-runtime-bridge.md).

This producer route exposes the fixed Master v10 reference through a separately
versioned, explicit [portable binary64 arithmetic contract](master-structural-portable-arithmetic.md).
The existing native `master-report` remains the legacy arithmetic default. Both
routes share one Go lifecycle, aggregation/calendar rules and fixed policies.
Portable arithmetic is a distinct numerical interpretation; its values and even
boundary decisions are not promised identical to legacy native output. The
[offline reference limitations](master-structural-offline.md) still apply.

## API and compatibility

After starting `enginewasm.wasm` with its matching Go `wasm_exec.js`:

```js
const metadata = JSON.stringify({
  schema: 'master-structural-portable-runtime-request-v1',
  arithmeticContract: 'master-binary64-separated-v1',
  warmupFromT: 0,
  tradeFromT: 72000000,
  tradeToT: 90000000,
  spread: 0,
});
const json = engineRunMasterPortableReport(metadata, source, btb1Uint8Array);
const document = JSON.parse(json);
```

The endpoint numbers above are illustrative synthetic UTC millisecond boundaries,
not a requested historical run. The exact three arguments are metadata JSON text,
DSL source text and an ordinary `Uint8Array` containing six-column BTB1 bytes.
Subarrays with nonzero byte offsets are supported; unrelated backing-buffer
prefix/suffix bytes are never read or hashed. Shared buffers, detached buffers,
`DataView`, other typed arrays, boxed strings, subclasses and prototype-only
imitations are refused. The adapter uses intrinsic typed-array accessors, so
shadowed `byteLength`, `byteOffset` or `buffer` properties do not change the view.
This is input validation, not a security boundary against code that can replace
JavaScript built-ins or otherwise control the calling process.

Success returns the full `strat-master-structural-portable-cli-v1` JSON envelope,
including whitespace and trailing newline. Its `arithmetic` object identifies
`master-binary64-separated-v1`, per-operation ties-to-even binary64 rounding,
no implicit contraction, and no tick quantization. The run carries its own
`strat-master-structural-portable-report-v1` schema and `arithmeticContract` ID.
Source/config/data hashes bind the exact source bytes, closed config and entire
supplied BTB1 view. All indicators, H4 snapshots, signals, trades, edits, summaries
and terminal states remain present. Future unused rows may change the data hash
without changing the run result. The withdrawn, never-released
`engineRunMasterReport` export and nonportable request schema have no silent alias.

Failure returns a JSON string containing only `error`; it never returns a partial
success report. Invalid calls do not disable later valid calls. Run this
synchronous CPU-bound export in a Worker in any eventual consumer; this task
does not provide an asynchronous, cancelable, live or streaming engine.

`report/masterstructural.Build` supplies the existing CLI's unchanged legacy
document. `BuildPortableV1` applies bounded shared admission and calls
`master.RunPortableV1`. Native `master-portable-report` requires the explicit
`--arithmetic-contract=master-binary64-separated-v1` flag. It validates the raw
spread token with the same exact-decimal logic as WASM, then reads files with
maximum-plus-one bounded streams before allocation can grow without limit.
Legacy native successful bytes and native limits remain unchanged.

## Closed input and resource admission

- Metadata is at most 4,096 UTF-8 bytes. Every schema field is mandatory exactly
  once; unknown/case-alias/duplicate decoded keys, nulls and trailing documents
  fail closed. Endpoints are safe nonnegative integer UTC millisecond M30
  boundaries with warmup < trade-from < trade-to. Endpoint JSON tokens use
  integer decimal syntax, not fraction/exponent notation. Spread is exactly 0
  or 1 as a plain decimal token of at most 64 bytes, without exponent notation
  or float-rounding admission; fees and initial equity cannot be supplied or changed.
- Source is at most 65,536 UTF-8 bytes. Original UTF-16 is validated before
  conversion, including lone surrogates in comments and quoted text. Valid
  astral characters retain their exact UTF-8 bytes and hashes. The existing
  strict Master parser and closed config decoder remain authoritative.
- BTB1 is version 1, exactly six columns and 1–100,000 rows, with exact total
  length `16 + rows * 6 * 8` (at most 4,800,016 bytes). The 16-byte header and
  actual view size are checked before copying the complete payload or decoding
  six float columns. No absent volume, ignored extra columns or suffix bytes.
- All supplied values must be finite, including unused future rows; volume must
  be nonnegative. Timestamps must be strictly increasing, exact safe-integer M5
  opens. Existing engine OHLC coherence and watermark/readiness checks still
  apply. No interpolation, quote-side inference or new calendar rule is added.
- Unexpected nonfinite arithmetic intermediates fail immediately, including
  before readiness or later min/max/band carry-forward could mask them. Only
  intentionally uncomputed warm-up sentinel slots are exempt. No success bytes
  are returned for an arithmetic failure.
- A conservative upper bound on encoded report size must be at most 128 MiB
  before the engine is called. A request can pass the row cap yet fail this
  output bound, especially with sparse rows. This is an encoded-output budget,
  not a claim that total process/Worker memory is limited to 128 MiB.

### Output-bound proof

Preflight scans the already size-bounded bytes without allocating decoded series.
It counts actual relevant M30 and completed H4 buckets; it does not divide source
row count by six or infer missing buckets. The fixed constructors guarantee:

- One indicator per observed M30 bucket and one H4 row per completed observed H4
  bucket; partial observed buckets remain represented.
- At most one signal, closed trade and order edit per trading M30 row.
- At most one monthly group per closed trade, one terminal position and one
  terminal pending signal. There are exactly 12 fixed assumption strings.
- All report strings are fixed literals no longer than 256 UTF-8 bytes, bounded
  fixed exit reasons, or entry-month labels no longer than 16 bytes for admitted
  timestamps. User-derived name/description occur only in the closed config.

The bound recursively counts the actual Go report field names and types, all
punctuation and indentation. It allows 32 bytes for each finite JSON number,
6× UTF-8 byte expansion plus quotes for strings and keys, and the complete
cardinality allowance for every array. Embedded structs are deliberately counted
as extra nested objects, overcounting keys and indentation. Unknown dynamic types
or new unaccounted slices fail closed. Exact bounded pretty config bytes and a
4,096-byte fixed portable envelope/provenance allowance are added. This bounds both the engine's
compact JSON validity allocation and the final pretty document before either
runs; the final size assertion is defense in depth only.

Tests bind the frozen constructors/primitive sources underlying these
cardinality/string guarantees and check worst-case escaping, extreme finite
numbers, full nested shapes, config expansion and sparse inputs. Changes to those
constructors require an explicit review/update of the bound proof. The standard
JSON serializer is retained; no bespoke report serializer changes CLI bytes.

## Unsupported routes and adoption gates

`engineRunFixture`, `engineRunColumns`, generic report/grid and
prepared/shared/prefix/checkpoint execution still refuse Master Structural with
`master-structural-native-dedicated-runner-required`. That existing diagnostic
identifier is retained for compatibility and refers to the dedicated-route
requirement; it is not an instruction to fall back to generic execution.
Non-Master or malformed reserved-family source is refused by the new route too.

This is producer implementation only. It does not change an app release pin,
register a strategy, expose a UI, place orders, publish a tag or deploy anything.
A separately reviewed tagged release plus digest-pinned consumer adoption remain
required. Reports still state `pineParityVerified: false` and
`costComplete: false`; this is neither Pine/broker parity nor new historical
performance evidence.

## Reproducible synthetic validation

Build the CLI and both WASM targets with Go 1.22, then run:

```sh
node scripts/checks/master-runtime-parity.mjs \
  /path/to/heisentick /path/to/enginewasm.wasm \
  "$(go env GOROOT)/misc/wasm/wasm_exec.js"

node scripts/checks/master-portable-legacy-parity.mjs \
  /path/to/heisentick /path/to/pinned-38613eb8-heisentick

GOOS=js GOARCH=wasm go test \
  -exec="$(go env GOROOT)/misc/wasm/go_js_wasm_exec" -run '^TestPortable' \
  ./internal/float64contract ./engine/regime ./engine/master
```

The script instantiates actual Go WASM under Node and compares full native/WASM
portable report bytes using the frozen invented corpus and fixed case windows.
Every target verifies the same corpus/source/data hashes; no host independently
generates transcendental inputs. The separate legacy script compares full Master
and v9 reports with a pinned baseline built by the same native compiler. Coverage includes both fixed modes/spreads,
long and short trades, H4 horizons, partial/gapped input, partial-entry causality,
future-watermark invariance, open terminal exposure, invalid input and transport
limits, valid/invalid Unicode, generic refusals and legacy positive controls.
This is Node-hosted WASM evidence, not an app UI or browser-engine qualification.
No market data, saved historical report or private trade/account records belong
in the source fixtures or publication artifacts.
