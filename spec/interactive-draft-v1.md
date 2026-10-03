# Local mutable DSL interactive v1

This unreleased opt-in contract admits mutable SMA Golden Cross and Dual EMA
Resumption source without a catalog source-hash whitelist. Other families and
constructs require further qualification. It does not change historical
`dsl.Parse`, the frozen `dslEditorStrategy` source or conformance goldens.

`InspectInteractiveSource` / native `--interactive-source-profile` / WASM
`engineInspectInteractiveSource` consume exactly `{schema,symbol,timeframe}`
with `schema: dsl-interactive-source-profile-v1`, and the source as a separate
string. Nonempty symbol/timeframe are required. The result contains `parse`
(Go config and diagnostics), `profile` (family, symbol, timeframe, rangeMethod,
maxBars 30000, calculationSource raw) and SHA256 of exact request/source bytes.
The host verifies the executable identity and binds it across preflight and run.
Preflight validates executor config support using a dummy bar; it performs no
strategy execution or market-data inference.

Source uses draft-specific `dsl.ParseStrict`: exactly one `dsl v7`, setup type
and nonempty complete `slices` declaration; exact long-only side syntax; finite
decimal period/risk tokens without ignored suffixes; no unresolved parser
errors or warnings. Existing SMA/Dual family audits own the admitted phrases.
Unknown setup types and multiple types fail instead of retaining a default.
HTF, source/entry timeframe, microstructure/morphology, approach, grade and
other unaudited directives are refused. This narrower source profile does not
claim support for all ordinary-family directives or rewrite older syntax.

`RunInteractiveDraftFixture` / native `--interactive-draft` / WASM
`engineRunInteractiveDraftFixture` consume only `schema`, `case`, `strategyId`,
`symbol`, `timeframe`, `rangeMethod`, `costs`, `bars`, plus separate source.
Schema is `dsl-conformance-run-fixture-v1`; strategyId is `dslDraftStrategy`.
All strings and all five cost fields are explicit. Source is reparsed and route
admission rechecked at execution. Range method must match the Go profile.
Unknown/case aliases, duplicate JSON keys and trailing JSON fail. No parameters,
context, auxiliary rows, transformations or execution windows are accepted.

Input has 1–30000 six-cell finite raw OHLCV rows with safe nonnegative integer
UTC timestamps, strictly increasing and on the declared fixed timeframe grid;
OHLC bounds and nonnegative volume must be valid. Gaps of whole periods are
allowed. For multiple rows, at least one adjacent declared-period pair is
required to verify cadence; an entirely gapped series is refused. Supported
durations are 1m/5m/15m/30m/1h/4h/1d. The caller supplies closed bars; this
contract does not infer closure from a provider clock.

Success uses `dsl-interactive-draft-v1` and the existing interactive accounting,
curves, trade outcomes, stats, skip schema, final liquidation and exact-byte
provenance from `interactive-run-v1.md`. Both targets reuse the same Go engine
and return structured `{schema,error:{code,message}}` on failure. The reserved
draft identity is rejected by ordinary interactive/conformance/direct run
entrypoints. Draft preflight is not an authorization token for another source.

Consumers must preserve Go accounting, label legacy idle JS diagnostics as
advisory in Go mode, reject unsupported requests visibly without fallback,
and prevent stale source/route/build results from being published.
