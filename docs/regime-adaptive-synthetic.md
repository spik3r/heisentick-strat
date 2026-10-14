# HT287 regime adaptive synthetic inspector

Current development base: `33e2f61f6359b696c3caaf0f263c41ee6b3f9054`. Canonical claim: https://github.com/spik3r/heisentick-backlog/pull/563 . This is a Go-only synthetic indicator/signal candidate, not an executable Pine-to-DSL strategy or an execution-parity certificate. No existing family, generic DSL, broker, browser or consumer pin is changed.

## Closed boundary

`go run ./cmd/regime-adaptive-inspect engine/regimeadaptive/testdata/ramp-v1.json` accepts one strict JSON request. It rejects duplicate/unknown/missing/null fields, noninteger timestamps, invalid OHLC, profile/source mismatch, changed Inputs, historical data, execution mode and unsupported timeframe kinds. Only fixed default Inputs are currently supported. The fixture is artificial: rise, turn, oscillation; no private or market dataset is used. Caller-provided native bars have explicit opening and closing timestamps; no session/MTF aggregation is inferred. Synthetic seconds/minutes/days use fixed durations, which does not certify actual daily sessions.

Profiles are separately identified by original v4.2 and flat-risk SHA256. Signal inspection preserves source seconds-chart sensitivity, repeated wick and simultaneous both-band flags, RSI50 scale triggers and initial/scale overlap. Flat-risk uses minute-only sensitivity and reports indicator/signal eligibility after both-band skip; `flatEligible` does not evaluate position, equity, quantity, currency, margin or actual admission. No position policy is implemented. Regime classification preserves the source's residual range branch when indicator comparisons are unavailable.

Rows expose TR, Wilder ATR14, RSI14, SMA14 of RSI, PSAR0.018/0.018/0.20, BB20 population variance, CHOP14 and raw signals. Missing values are null. Arithmetic uses existing separated binary64 primitives to prevent native FMA in recurrences. Log/sqrt numerical equivalence still needs the named native/WASM toolchain checks and Pine witness tolerances. Existing shared SMA ATR and unrelated regimeEngine/rangeReversion arithmetic are untouched.

The RSI both-zero seed is explicitly unavailable pending a Pine flat-series witness. PSAR initialization/reversal and near-zero variance behavior have hand-derived tests, but are not matched against TradingView exports. This candidate makes no built-in indicator parity claim.

`executionStatus=unsupported-not-run`; `orderLifecycle`, `trades`, `pendingOrders` and `terminalPositions` are null. They must never be interpreted as empty equivalent execution. Pending required execution work includes next-open fills, source pyramiding2, reversals and exit allocation, refreshing signal-close brackets, collisions/gaps/marketable targets, quantity/cost/currency/margin/rounding and terminal accounting. Source wrong-side range targets remain visible in signal fixtures; no validity filter is silently introduced. These require independent execution fixtures and a matched Pine witness.

## Verification commands

Pinned verification: task-local Go1.22.12, Node24.10.0, Git2.54.0. The global Go1.27.0/Node26 installations are not changed. Use task-specific GOCACHE; never clear global caches.

```sh
gofmt -l .
go vet ./...
go test ./...
go run ./cmd/conformance check
go build -trimpath -o /tmp/ht287-native ./cmd/regime-adaptive-inspect
GOOS=js GOARCH=wasm go build -trimpath -o /tmp/ht287-inspect.wasm ./cmd/regime-adaptive-inspect
GOOS=js GOARCH=wasm go build -trimpath -o /tmp/ht287-dslwasm.wasm ./cmd/dslwasm
GOOS=js GOARCH=wasm go build -trimpath -o /tmp/ht287-enginewasm.wasm ./cmd/enginewasm
```

Run inspector native and its WASM using the same fixture and the matching Go `misc/wasm/go_js_wasm_exec` with pinned Node. Compare complete JSON diagnostics. Building the two existing WASM targets does not mean this family is integrated into them. This family has no generic parse/run conformance admission and no backtester/forward-tester compatibility claim. No Linux execution or instruction proof is implied by macOS builds. Independent review must precede release/adoption.

Historical validation is blocked pending provenance/license, data and session receipts, actual Pine source/Inputs/Properties/account/margin witness, indicator and order/trade/pending/terminal exports, frozen chronological development/validation/holdout boundaries and approved staged research. No historical outputs, sealed holdouts, broker orders or deployments are part of this slice. Monte Carlo/walk-forward evidence would be premature before an executable qualified baseline and protocol.

## Observed first-slice verification

Go1.22.12 macOS arm64: gofmt empty, vet and full tests passed; conformance check matched 135 parse and 109 run goldens, 0 TODO run cases. Inspector native and WASM plus existing dslwasm/enginewasm builds passed. Complete inspector JSON for the committed 90-bar synthetic fixture matched byte-for-byte, SHA256 `600e6aa9abdeec4e44e2af66f61c8e4d22e7f722797c9d88ece44e0f8dbc4ce5`. Native inspector SHA256 `6357f5ba90f794c70c0b80af62d1512977f6dba50bbfad54c88bf1a4b4a857df`; inspector WASM `59c0bfc32c5e23a08c79aac93afd4cf08579ddf0847a819b46c1a9cb280d7e80`; existing dslwasm `d3a2a19b0d0087d129ce4f69ac99156ab881fb67b2f0e1dee63408f39e49d444`; existing enginewasm `4980791e7fc7222a1c8e60179cf673dce9e337ced69bfb2b4465d9c99f539bf1`.

Independent first-slice review found and verified repair of signed timestamp subtraction overflow; it requested complete default PSAR and reversal-prefix fixtures and fuller consumer receipts. Final re-review found no blocking bugs within the documented slice. Reviewer focused tests used separate global Go1.27/Node26 and an isolated cache; qualification evidence above used task-local Go1.22.12/Node24.10.0. No Linux instruction/runtime proof, Pine witness, execution accounting, consumer route parity or historical outcomes were obtained.
