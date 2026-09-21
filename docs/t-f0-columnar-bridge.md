# T-F0 columnar WASM bridge measurement

This is an experimental producer-side interface. It does not authorise a
browser default, a rollout, a release, or a decision on D-11/G6.

Source baseline: `8e143604373ca7ae8f327a5bd7513590fe580d60` (v0.3.0).
The final local WASM artifact SHA-256 is
`96db5b84f019aeec3b08f5b53f477ac5dd09f4c6f5976f29e78bd998ee853dce`.

## Interface and scope

`engineRunColumns(metaJSON, source, t, o, h, l, c, v)` accepts six equal-size
`Float64Array` columns. Each input is copied once into a Go-owned
`[]float64`; the native byte view is used only for the WASM copy. Output has a
fixed 13-float record per trade plus JSON carrying side, reason, optional rule
identity, tag, metadata and nullable stop/target state, so it can reconstruct
the complete native trade record.

The interface rejects source-timeframe, higher-timeframe, and precomputed
context inputs. It is therefore a single chart-series measurement, not an
all-strategy bridge claim.

Timing reports copy-in, Go adapter/parser, `engine.Run`, result packing and
copy-out separately. The JS comparator builds context columns and runs the
current app `runBacktest` from the same typed columns; its context and run
timings are reported separately.

## Inputs

The real read-only input is `XAUUSD/5m.bin` from the local backtester data
root. It has 397,727 bars from 2021-01-03 through 2026-08-11 and SHA-256
`47da5c9f63c91a02a60747c3197f2ef9972eb4bd33f3e2d29fa6ac8063c6e3b6`.

50,000 and 200,000 are real prefix windows. Exact 400,000 is unavailable;
the full available 397,727-bar window is reported separately when measured.
No synthetic padding is used.

## Results

| Bars | Input | WASM wall ms | copy-in | adapter | engine.Run | pack | copy-out | JS context | JS run | Trade equality |
|---:|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| 50,000 | real | 10,148.651 | 0.864 | 15.514 | 10,095.652 | 29.330 | 0.160 | 1,773.611 | 4,835.327 | 1,297 reconstructed trades matched all native fields; app-only `positionId` omitted; derived `pnl`/`points` rounded to 1e-9 |
| 200,000 | real | 147,656.974 | 6.551 | 16.140 | 147,527.811 | 94.316 | 0.175 | 4,432.959 | 84,920.809 | 5,244 trades: count matched on the pre-review artifact |
| 397,727 | real full available | pending | pending | pending | pending | pending | pending | pending | pending | pending |
| 400,000 | unavailable | — | — | — | — | — | — | — | — | only 397,727 real bars available |

The 200k result predates the alignment and case-identity review fixes. Those
fixes do not alter engine semantics, but this report does not present it as a
final-artifact full-field comparison. The 50k result is one local Node/WASM
sample. It is not a browser worker measurement and cannot settle D-11's
browser budget.

At both measured sizes, Go engine computation dominates bridge time and is
slower than the current JS context-plus-run path. These samples do not support
a D-11 performance pass.

## Validation limits

One IAB Chrome 153/macOS browser smoke served the final WASM artifact and the
existing `pm-signal-close-entry` semantic fixture. It reconstructed all 13
numeric fields plus side, reason, rule identity, tag, and metadata and matched the JSON
fixture bridge exactly for its one nonempty trade; a mismatched input column
was rejected. [The harness](../scripts/spikes/enginewasm/columnar-browser-smoke.html)
expects `engine.wasm`, `wasm_exec.js`, `fixture.json`, and `strategy.strat` in
the same served directory. This is one correctness smoke, not performance,
coverage, or G6 evidence. The Go fixture bridge regression
still checks all four deployed cases against native output, including
nonempty cases; the local real-data columnar harness checks complete trade
reconstruction at 50k.
