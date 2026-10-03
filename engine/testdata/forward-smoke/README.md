# Frozen authored-JS Forward smoke oracles

These six fixtures were generated with `runBacktest` from app commit
`a7bc97689838d7cc5e2d7fdf462b9ca2f5dbfd28` using the two authored-JS
canary modules. The source SHA-256 values are pinned in each fixture and in
`engine/forward_smoke.go`. Each fixture records complete JS trades and
`closeAtEnd: false` results over closed-bar prefixes of length 2, 3, 7, 10,
and 12. The Go test compares every fill, bracket, size, index, time, PnL,
reason, tag, and preserved open position. The only intentional vocabulary
difference is JS terminal `eod` versus Go `end-of-test` (engine D-20).

The BTC rule is named “ThreeCandle” for historical reasons; its source checks
only the current and immediately prior candle. Both modules carry a visible
“BAD TEST ONLY” label and are operational checks, never research strategies.

`cmd/enginewasm` exposes the strict `forward-smoke-request-v1` adapter in
native (`--forward-smoke` / `--forward-smoke-prefix`) and WASM
(`engineRunForwardSmoke`). The request must specify `mode` (`whole` or
`prefix`), the exact strategy ID and SHA-256 version, symbol, `1m` timeframe,
costs, and an array of completed `{t,o,h,l,c,v}` candles. Native CLI mode
overrides `mode` in the JSON. Prefix execution replays the supplied series;
its digest binds that input and is not a resumable engine-state checkpoint.
