# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/). Versions follow
semver; a semantic change to the parser, engine or corpus bumps the minor
version and is named here.

## [Unreleased]

- Minor bump (next `0.11.0`): add whole-strategy Go run goldens for `dslWeekendExtremeFadeBtcusdtFourHour` on BTCUSDT 4h, `dslBreakRetestAsiaLondonOneFourHourReview` on XAUUSD 1h, and `dslSupplyDemandFxRejectionOnePointThree` on EURUSD 15m. Their 3,803-, 8,552-, and 36,036-bar fixtures match all 31, 16, and 16 JS trade rows exactly. Go weekend-fade diagnostic serialization now matches JS precision and metadata. These are conformance checks, not performance promotions.

## [0.10.0] — 2026-09-26

- Minor bump: new DSL setup family `named level sweep` (HT-054 Phase A,
  #46). Sweep and reclaim of a named level (`PDH/PDL`, `WH/WL`, `AH/AL`,
  `LH/LL`, …) rather than a detected range or channel:
  `when price sweeps <LEVEL> [by at least X ATR] and closes back above|below it`
  and `when price tests <LEVEL> within X ATR and closes above|below it`, each
  with optional `by at least Y ATR` and `within N candles`. New shared
  phrases: `stop below|above the signal candle and the level by X ATR`,
  `trigger { no confirmation candle }` (a clearer synonym for
  `candle in (any)`), and `prior day range at least X ATR`. Six new
  diagnostics with conformance fixtures. Existing `.cfg.json` goldens gain
  the new shared default `"priorDay": {"minRangeAtr": null}`; no other
  output changes. The runtime is minimal (sweep/test geometry, stop, one
  signal per level per day); full runtime and strategy ports follow.
- `report`: every trade carries a stable `signalId`, and
  `--include-trades=1 --trade-export=1` writes a `trade-export.v1` document
  (heisentick-contracts `v0.7.0`) with strategy, engine and data-file
  digests (#45).

## [0.9.0] — 2026-09-24

- Added opt-in transfer route runs to the report and grid CLI (#44).
  Declared-route gating stays the default. Transfer runs bypass only the
  strategy's declared symbol/timeframe route allowlist, mark evidence with
  `routeMode: transfer` and `forceRoute: true`, and warn that off-route
  results are exploratory. The registered strategy source is not rewritten.

## [0.8.0] — 2026-09-23

- Fixed Go parsing and entry admission for `trade window in (...)` and
  `trade window minutes A to B` so they preserve the JavaScript compatibility
  rule (#41). Windows remain fixed at UTC+10; named parts are half-open
  60-minute thirds, and the fourth `mid` hour matches only `mid.all`.
- Also in this release: defensive VWAP (#40) and No NY Close (#42) strategy
  goldens, admission of verified gapful execution ends (#43), and isolated
  full-data browser reliability repeats (#39).

## [0.7.0] — 2026-09-23

- Added the exact `dslSmaGoldenCrossXauusdOneMinuteCanary` source and an
  `XAUUSD 1m` Go run golden with 13 trades. The fixture reuses the reviewed
  OHLCV values from `family-sma-golden-cross-protected` with deterministic
  one-minute closed-bar timestamps. It proves execution and serialization for
  a test-only report route; it is not economic, promotion, Forward, or live-use
  evidence.

## [0.6.1] — 2026-09-23

Patch release: opening-range-breakout UTC-slot scans stop at the first older
slot boundary. This preserves selected bars while preventing full-history
rescans at each boundary that exhausted browser WASM memory on long inputs.

## [0.6.0] — 2026-09-23

Minor bump: the engine and CLI add the first opt-in native Forward prefix
contract. Existing report, grid and no-checkpoint prefix behavior is unchanged.

- `engine.RunPrefix` preserves an open position at the end of supplied data,
  returns stable lifecycle IDs for open and closed rows, and reports a digest
  binding the complete replay input. C5 and special-family paths fail typed
  instead of falling back to report liquidation.
- `engine.RunPrefixResumable` adds an opaque, versioned checkpoint for
  opening-range-breakout routes. It restores open and pending state, emits
  closed-trade deltas, rejects changed input/config/state and leaves every
  unsupported family or route closed. Callers still supply the complete
  extended bar prefix.
- `heisentick forward-prefix` emits one JSON result without a synthetic
  end-of-test close. Optional checkpoint input/output flags publish a synced
  `0600` successor candidate without replacing an existing file. The caller
  advances its durable pointer only after exit 0 and complete stdout capture.
- Existing `report` and `grid` JSON and liquidation semantics are unchanged.

## [0.5.0] — 2026-09-23

Minor bump: disabled Mid sessions no longer consume setup cooldown state in
families using the shared session gate.

- Setup discovery now checks `UseMidWindow` before recording
  an attempt. A later setup in an enabled session may therefore enter where
  the previous disabled-session attempt had suppressed it.
- Reviewed run goldens change from 13 to 14 trades for break-retest and from
  11 to 13 for double-top-bottom. Trend-pullback retains its documented legacy
  Mid discovery path when VWAP-touch is off.
- Consumers must carry the explicit Mid session flag into both family setup
  gates to match the Go corpus. Historical reports keep their release identity.

## [0.4.1] — 2026-09-23

Patch release: report JSON now preserves an absent stop as `null`.

- `report.Trade` serializes `initialSl` and `sl` as `null` when the engine
  marks the trade `NoStop`, matching the run-conformance representation.
- A genuine numeric zero stop remains numeric when `NoStop` is false.
- Parser, engine, corpus and trade economics are unchanged.

## [0.4.0] — 2026-09-21

Minor bump: D-18, D-19 and D-20 adopt reviewed engine and execution semantics.

- **D-18:** `costs.fillOn: open` treats a market order raised at a signal
  close as a next-bar-open fill. The explicit `nextOpen` spelling remains
  supported; signal-time stops handle worse-open gaps, fill-relative brackets
  stay anchored to the eventual fill, and a final-bar pending order never
  becomes a phantom trade.
- **D-19:** A completed higher-timeframe candle is classified from its own
  open and close (`up`, `down` or `flat`). The last completed candle becomes
  available from its close; forming and stale projections remain unavailable.
- **D-20:** Exit reasons use the canonical `tp`, `sl`, `partial`, `time`,
  `end-of-test` and `rule` categories. Rule exits carry their strategy-rule
  identity in the separate `rule` field.

## [0.3.0] — 2026-09-20

Minor bump: the engine changed a fill rule (gap through a carried stop).

- `diagnostics/`: `Evaluate(Input) Result` derives the overfit metrics and
  warnings the app's `scripts/strategy/overfitReport.mjs` computes from
  three passes (full-history realistic, recent window, full-history
  harsh). `PassFromDocument` reads a pass from a `report.Document`
  (primary cost row, per-slice primary net, year groupings, last bar
  time). Warning codes: `best-slice-concentration`,
  `recent-year-negative`, `harsh-cost-negative`, `thin-sample`,
  `recent-window-pf-drop`, `blow-up-year`, in that order, with the JS
  wording. A nil or non-finite profit factor reads as 0, as it does in the
  JS after its JSON round trip. The fixtures under `diagnostics/testdata/`
  and their `.expected.json` (regenerated by `testdata/gen/generate.mjs`
  from the JS functions) are the contract.

- Engine: a carried stop that the bar opens beyond fills at the open
  (`min(stop, open)` for longs, `max(stop, open)` for shorts), not at the
  stop level. Entry-bar handling is unchanged; slippage still applies on top.
  Matches `conformance/semantic/pm-gap-through-stop`. No `conformance/run`
  golden changes. Spec §7 now states the stop and target gap-fill rules.
- `montecarlo/`: new package porting heisentick
  `scripts/strategy/monteCarlo.mjs`. `montecarlo.Run(Input) (Result, error)`
  takes the realised per-trade P&L stream, a start equity, a `Method`
  (`Permutation`, `Bootstrap`, `Both`), an iteration count and a uint32
  seed, and returns the script's block: `realized` (net, maxDD),
  `orderShuffleMaxDD` and `bootstrapMaxDD` (p50, p95, p99, worst) and
  `bootstrapNet` (p5, p50, p95, probNetLeZero). Same mulberry32 stream,
  shuffle, bootstrap index and nearest-rank percentile as the script, so
  the numbers match bit for bit; `montecarlo/testdata/` fixtures written by
  the JS oracle in `testdata/gen/oracle.mjs` are the contract. An empty
  stream returns `trades: 0, warning: "no trades"`; zero iterations, an
  unknown method, a non-positive start equity or a non-finite P&L value
  are errors. CI now runs on `montecarlo/**` changes.

## [0.2.1] — 2026-09-19

Report package (one-engine migration M2, ticket T-B4). CLI output for the
existing flags is byte-identical; the new fields are additive.

- `report/`: the strategy report moved out of `package main` into an
  importable package. `report.Build(ctx, Request) (Document, error)` takes
  a parsed config, a loaded `Route` (entry, source, higher-timeframe
  series), strategy id and name, optional slippage and basis-point
  overrides, `IncludeTrades`, `HoldoutFromT` and `GeneratedAt`, and returns
  the CLI's payload. Also exported: `CostModes`, `ParseSlippageBps`,
  `RunCostRowsFromShared` (grid), `RouteWarnings`, `ResolveSourceTimeframe`,
  `ResolveHigherTimeframe`, `StrategyID`, `StrategyDisplayName`,
  `WriteJSON`, `TradesSchema` and the document types (`Document`, `Slice`,
  `CostRow`, `Trade`, `PrimaryCost`).
- `Document` gains `headline` (the primary row as the app's
  `headlineFromReport` maps it: `winRate` a fraction, `profitFactor` null
  with `profitFactorReason` `no-trades` or `no-losses`, `maxDrawdown` the
  row's `dd` with `maxDrawdownUnit: percent-of-peak-equity`, `firstTradeT`
  the earliest entry, `lastTradeT` the latest exit), `groupings` of the
  primary trades by `session` (the engine's session phase at the entry bar),
  `side`, `exitReason` and UTC entry `year`, each list ordered by key;
  `dateBounds` (first and last bar time and bar count of the entry series
  as run); and `holdout` when a boundary is given: trades with
  `entryT >= HoldoutFromT` are holdout, the rest in-sample, each side a
  headline over its own equity curve from 10000.
- `cmd/heisentick report` is a thin adapter: flags, file loading,
  `report.Build`, JSON. New flags `--evidence=1` (emit the four new fields)
  and `--holdout-from=<epoch-ms>`. Without `--evidence=1` the payload is
  unchanged, so the CLI goldens did not change.
- The engine still has no execution window, warm-up or spread, commission
  and financing costs; the package documents that trimming the series is
  the caller's job and runs fills on the close from a start equity of 10000.

No parser, engine or corpus semantics changed.

## [0.2.0] — 2026-09-19

Direction change: the conformance corpus is generated by the Go engine and
JavaScript conforms to it (one-engine migration M2, ticket T-B3). Before this
the app's JS engine wrote the goldens and Go had to match them.

- `cmd/conformance`: `regen` writes `conformance/parse/*.cfg.json`,
  `conformance/run/*.trades.json` and the scoreboard in
  `conformance/metadata.json` from `dsl.Parse` and `engine.RunFixtureCase`;
  `check` fails on any byte that differs; `list` prints each run case with
  its status. `regen` refuses to write a golden for a TODO case.
- `conformance/metadata.json` gains `goRun: { implemented, todo }`
  (32 implemented, 0 TODO). No parse or run golden changed content: the Go
  regen reproduces all 43 parse and 32 run files byte for byte.
- `engine.ImplementedRunCases`, `engine.RunConformanceTODO` and
  `engine.ConformanceProjection` are exported so the generator and the Go
  conformance test share one projection.
- CI runs `go run ./cmd/conformance check`. The release archive
  `conformance.tar.gz` now also carries `examples/`, and `wasm_exec.js` from
  the build toolchain ships next to the WASM assets (issue #3).
- `conformance/README.md` rewritten for this repository: Go generates, JS and
  WASM conform, regeneration is a reviewed PR, how to add a case, the four
  coverage classes.

No parser or engine semantics changed.

## [0.1.0] — 2026-09-19

Mechanical extraction of `strat/` and `go/` from `spik3r/heisentick` at
`4db7c3aa`. No parser, engine or corpus semantics changed. Import paths are
`github.com/spik3r/heisentick-strat/...`; the `go/dsl` and `go/engine`
alias facades are gone. Fixture-path changes in six test files and two
frozen `.strat` copies under `engine/testdata/strategies/` make
`go test ./...` pass in the new tree. The conformance corpus is
byte-identical to the app's at that commit (43 parse, 32 run, 15 semantic
cases).
