# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/). Versions follow
semver; a semantic change to the parser, engine or corpus bumps the minor
version and is named here.

## [Unreleased]

### Sequential E1/E2 market companion v2 (additive, not released)

- Add `engineRunSequentialMarket`, `engineAdmitSequentialMarket` and
  `enginewasm --sequential-market run|admit`: caller-authored Full E1/E2 sources
  on any uppercase symbol and 1m to 1d timeframe, with trades, marked equity and
  statistics from the existing Go execution, accounting collector and metrics
  projection. Start equity must be explicit. Bars must be contiguous and the book
  flat at the end; timestamp gaps and a held terminal position stay typed
  refusals, and `admit` reports which. Result and admission schemas are
  `sequential-market-result-v2` and `sequential-market-admission-v2`. The v1
  export, schema, pinned sources and goldens are unchanged. Gap, closure,
  observed-bar and terminal-open handling are open decisions; see
  [the contract](docs/sequential-market-contract.md). No broker, seqcore,
  Legacy, P1 or golden change.

### 0.32.0 candidate

This is an undated candidate, not a published release. See the current
[release checklist](docs/paper-prefix-release-checklist.md#current-v0320-release-gates).

- Add bounded VWAP-extension-fade execution to the native/public-WASM
  paper-prefix API: close fills, 15-minute zone context, USD and abstract linear
  multiplier-one accounting. Go-owned authored-source coverage and complete
  option checks refuse unsupported or overwritten intent. Quote-side execution,
  first-observed-quote fills, enabled partial/trailing management and historical
  profitability are not qualified by this addition. See [the option class](spec/forward-paper-prefix.md#vwap-extension-fade-paper-option-class).

- Harden ordinary ORB paper admission using Go-owned authored coverage and
  effective-config checks. Previously ignored directives, including an
  opposite-range-edge target, now refuse rather than silently execute different
  intent. Preserve the ordinary parser, family/broker arithmetic and admitted
  close/next-open management semantics. This does not implement the unwired
  retest language or qualify the archived seasonal legacy paper profile. See
  [the ORB option class](spec/forward-paper-prefix.md#opening-range-breakout-paper-option-class).

- Add the transport-only `engineRunPaperBidAsk` public Go WASM export over the
  unchanged sampled bid/ask API. Preserve exact producer documents, fixed native
  refusal envelopes, strict original UTF-16/JSON validation and all native
  limits. Actual public-export parity and target-local checkpoint proofs leave
  existing exports unchanged; consumer limits, adoption and activation remain
  separate. See [the transport contract](spec/forward-paper-bidask.md#public-wasm-transport).

- Add the bounded native sampled bid/ask ORB paper contract and strict local-only
  `paper-bidask` command, separately versioned from paper-prefix. It models
  original committed entry quotes, sampled straddling exposure, then exclusive
  completed-side-bar protection; preserves UTC-slot opening priority and ordinary
  Go breakeven. Exact quantity/cost accounting, causal replay and source admission
  have their own qualification. Partial/generic-trailing management is the next
  separate tranche; no consumer activation, broker orders or historical outcome
  claim. See [the contract](spec/forward-paper-bidask.md).

The sampled bid/ask contract is ORB-only; its partial-exit and generic-trailing
management remain unsupported even though admitted ORB paper-prefix options
retain those existing semantics. Full-prefix replay integrity does not provide
authenticated feed/provenance, durable exactly-once effects, fencing or session
isolation. Consumer pins, qualification and default-off activation gates remain
separate. The existing 135 parse and 109 run goldens and all 21 approved
instruction pins are unchanged.

## [0.31.0] - 2026-10-11

Published from `e59dff4fe170a250111fb965fe775af919b3bd26` at 12:53:23 UTC.
[Release and verified assets](https://github.com/spik3r/heisentick-strat/releases/tag/v0.31.0)
and [verification receipt](https://github.com/spik3r/heisentick-backlog/blob/7dec2bd365c0014bf017222dc4d1270e40943321/tasks/HT-243-forward-go-producer-parity-gaps.md#published-v0310-producer-package-verified--2026-10-11t1259z).
This dates the prior candidate notes after publication; the frozen v0.31.0 tag and
assets are unchanged. Qualified evidence covers the named ordinary ORB/raw legacy
fixtures/options only. Seasonal-specific paper qualification and the known
ignored-ORB-authored-directive admission gap remain separate gates; consumers
stay default-off. See the [release checklist](docs/paper-prefix-release-checklist.md).

- Add the transport-only `engineRunPaperPrefix` public Go WASM export over the
  same Go paper execution/accounting API. Require one valid UTF-16 request JSON
  string within an explicit 16 MiB UTF-8 input budget, return unchanged producer
  results or fixed safe errors, and prove actual native/public-WASM parity and
  same-target recovery. No new family, endpoint liquidation or portable
  checkpoint support; tagged release and browser adoption remain separate.

- Add a Go-owned paper-prefix API/native command with explicit costs/equity,
  causal activation, open account MTM, partial/final trade accounting and
  replay-verified recovery. Paper qualification covers ordinary ORB and raw
  `seq.legacy.setup9.v1`; the producer also admits the seasonal legacy profile,
  but seasonal-specific paper qualification and consumer admission remain gated.
  Existing prefix/Sequential contracts remain unchanged. HTF/source-entry,
  full E1/E2, release adoption and operational activation remain separate gates.

- Add read-only Go Sequential source inspection through the native library/CLI
  and engine WASM. Preserve the parser result, bind source and Go config bytes,
  and distinguish generic execution from fixture-only accounting and unsupported
  market/endpoint/prefix capabilities. No execution or accounting support is
  broadened; release assets and consumer integration remain separate gates.

- Include the corrected HT-245 broker arithmetic investigation and actual
  target qualification ([PR #118](https://github.com/spik3r/heisentick-strat/pull/118)).
  This is diagnostic evidence, not a production arithmetic correction or a
  claim of universal cross-target equality.

The existing 135 parse and 109 run goldens are unchanged from v0.30.0.
Documentation preparation preserves all 21 instruction pins approved with the
Go paper producer; those pins are not all identical to the pre-paper release.

## [0.30.0] — 2026-10-11

Published from `4ab0fde8dfcfd3f2b33e4ea0af6186474c2b6ffa`. These five
entries describe the shipped source changes; post-tag inspection and paper
execution additions are listed separately above. Consumer qualification and
activation remain separate from producer publication.

- Refuse malformed reserved `range reversion` selectors before legacy parsing
  can silently execute a different family. Preserve canonical R11 grammar,
  fixed-rule source hashes and quoted metadata/list values. Add invented
  parser/native/WASM admission regressions and one diagnostic parse fixture;
  every existing golden is unchanged. Clarify the existing dedicated fixed-rule
  WASM bridge exception.

- Add a source-frozen Sequential companion backtest export with one-pass raw
  broker accounting, all-fee trade net, headline statistics and explicitly
  versioned close-bar MTM equity/drawdown. Keep existing run/trade envelopes,
  execution policies and generic callers unchanged. Typed refusals exclude
  incomplete Full terminal runs and invalid/unrepresentable metrics. Initial
  capabilities cover headline/trades/equity only; grouped/monthly/portfolio
  projections and consumer activation remain separate gates.

- Add the distinct `sequential full` DSL family for the versioned synthetic
  `seq.full.public_approx.v1` E1/E2 policies. Bind explicit symbol, timeframe,
  risk and exposure inputs; reuse the accepted core and existing broker for
  next-open execution, fixed signal ATR14, structural stops, 2R targets and
  natural time exits. Preserve legacy Setup-9 and existing regression goldens.
  Ordinary native/fixture and existing WASM bridge routes return a typed
  opportunity audit. Missing intervals/calendar declarations, incomplete
  terminal exposure, P1, source/HTF projection and shared/prefix/checkpoint
  execution are explicitly refused. Add two parse and four invented producer
  run regressions plus native/Node-WASM tests; the independent HT249 acceptance
  corpus, browser/consumer qualification and historical approval remain gates.

- Add the `legacy setup 9` family (go-only) with the frozen profiles
  `seq.legacy.setup9.v1` and `seq.legacy.setup9_perf_seasonal.v1`, selected by
  a required `sequential profile <id>` line. They port two archived
  strategies faithfully, including the stored Int8 count wrap. Entries fill at
  the signal-bar close only; other fills are refused. Resumable prefix
  checkpoints now accept this family. Add parse and run conformance cases; no
  existing golden changes. A `type:` that switches to another family after
  `sequential profile` is a parse error, and the engine refuses a config that
  carries a legacy profile under another family. In setup bodies written on one
  line, `sequential` now starts a new directive.

- Add `retest on entry timeframe` to the `supply demand` family. With
  `source timeframe 4h` and `entryTf 15m`, `30m` or `1h`, zones are made,
  flipped and expired on completed 4h candles while the touch, rejection and
  entry are judged on the entry candles' own closes. The existing
  `source timeframe` + `entryTf` behaviour (the whole setup runs on the source
  series and the finished signal is dispatched on the first entry close) is
  unchanged without the phrase. `entryTf 1h` is admitted only with the phrase.
  Retest candles begin at or after the completed source formation/flip close;
  no preceding intrabar movement is counted as a retest. Preserve mode selection
  through JSON config transport and reject contradictory family/routes or shared
  variants instead of silently falling back to another execution path.
  New parse and run fixtures; no existing golden changes. Browser runtime and
  WASM adoption follow the release.

## [0.29.0] — 2026-10-08

- Add the bounded `UNIT_POINT_VALUE_1` Go research-accounting projection and
  dedicated native/WASM routes for exact adaptive `INITIAL`, `TWEAKED` and
  `SNAPSHOT_C` baselines (HT-195,
  [PR #105](https://github.com/spik3r/heisentick-strat/pull/105)).
  Preserve creation-time quantity, marked/open/closed accounting and explicit
  RAW/BASIC/HARSH cost policies; report hypothetical liquidation separately
  and keep funding unknown. The initial unit profile admits at most 1,024
  retained rows, including all warmup, and 366 UTC calendar dates. CUSTOM/F/G
  overlays, generic engine routes and account-dollar/percentage claims remain
  refused. Native and Node-hosted Go/WASM qualification uses the retained
  invented corpus; browser capacity, Backtester adoption and deployment remain
  separate gates.
- Add an optional native daily CHOP band to the dedicated `rangeReversion`
  research runner (HT-190). The separate D1 input preserves native UTC-midnight
  rows and uses next-native-row completion; the gate applies only before new
  signal queueing. Gate-off configuration and output fields remain unchanged.
  `range-reversion-report` requires `--daily-bars-file` only when enabled.
  The Daily-gated definition remains unsupported by generic, WASM and browser
  execution.

## [0.28.0] — 2026-10-08

- Add bounded native research overlays for the exact adaptive `SNAPSHOT_C`
  configuration (HT-191, [PR #102](https://github.com/spik3r/heisentick-strat/pull/102)).
  The overlay bypasses only retrace and width numeric caps; EMA, volume, source
  features and portfolio lifecycle remain enabled. It does not add historical
  G execution, parameter search, or generic DSL, parser, app or default-route
  support.
- Add bounded Stage A native/WASM raw reports for exact adaptive `INITIAL`,
  `TWEAKED` and `SNAPSHOT_C` configurations on M30 and H1 (HT-195,
  [PR #103](https://github.com/spik3r/heisentick-strat/pull/103)). The route
  shares the Go state machine and report construction. It refuses CUSTOM/F/G
  overlays and economics or account-stat requests; this stage makes no P&L,
  equity, cost or trading-performance claim.
- Add a dedicated `engineRunR11Fixture` Go/WASM export for the two
  source-pinned, one-unit R11 research rules ([PR #104](https://github.com/spik3r/heisentick-strat/pull/104)).
  It checks exact DSL bytes, route, original M30/H1 and H4 OHLCV rows, UTC
  window and explicit per-fill costs before calling the existing native
  range-reversion runner. Its versioned response binds fixture and DSL byte
  hashes and returns the native ledger and accounting summary. Generic
  fixture and JavaScript execution still refuse the family. The known
  synthetic Python/native entry-bar BE-close-latch difference is unchanged.

## [0.27.0] — 2026-10-07

- Fix `IMMEDIATE_STOP_FIRST_V1` entry-gap handling for the Go-only
  `rangeReversion` family (HT-190). A next-open fill through its stop now uses
  absolute fill-to-stop risk and flattens at that open, charging both execution
  sides and commission with `stop-gap` or `target-gap` reason labels; only zero
  or near-zero risk (at most `1e-12`) is canceled. The report keeps the existing
  cancellation-counter field name. `DELAYED_PINE_OHLC_V1` behavior is unchanged.

## [0.26.0] — 2026-10-07

- Add the Go-only `rangeReversion` v7 research family (HT-190) and dedicated
  `range-reversion-report` command for completed-source-range sweeps and the
  supplied Pine chart-channel contract. The two order policies explicitly
  model delayed Pine brackets or immediate stop-first brackets, with strict
  configuration, next-native-row source availability, input hashes, fixed
  units and request-level costs. Generic, grid, prefix, WASM and browser
  execution reject the family. The release adds no browser, UI or broker
  execution support.

## [0.25.0]

This minor version includes clock-range fixture diagnostics, the down-shock
native-report source fix, the opt-in Master portable arithmetic contract and
the dedicated research families below. The semantic corrections can change
results for affected authored DSL programs even though the 128 v0.24.1
parse/run goldens remain unchanged. Consumer adoption, browser qualification,
deployment and strategy promotion are separate decisions.

- Add `engineRunClockRangeFixture`, a dedicated Go/WASM fixture export with
  the unchanged ordinary trade envelope and per-day entered, rejected, expired
  and terminal-pending diagnostics (HT-167, PR97). The Go engine remains the
  execution authority. Clock-range checkpoint resume is unsupported; the
  column route is unqualified. Consumer browser adoption is separate and must
  retain strict original OHLCV/UTC-millisecond input admission, cancellation,
  source/route binding and errors without JavaScript execution fallback.

- Fix `downShockRebound` direct, prepared and native-report execution to pass
  the already-admitted source series into its family runner (HT-187, PR98).
  The previous report adapter recorded source provenance but omitted source
  rows from the fixture object consumed by the runner, silently suppressing
  trades. The fixture/native/WASM paths now agree on invented source-close,
  prefix, entry-mode and cost controls. Detector arithmetic, source-close
  timing, execution-window boundaries and all existing fixture goldens are
  unchanged. This repairs input plumbing; it is not new strategy evidence.

- Add the explicitly selected `master-binary64-separated-v1` arithmetic
  contract through native `master-portable-report` and the dedicated
  `engineRunMasterPortableReport` WASM export (HT-183, PR95). Its versioned
  request/report schemas and numerical provenance distinguish it from legacy
  `master-report`, `master.Run`, v9 and existing reference primitives, which
  remain unchanged defaults. The never-released nonportable export is withdrawn
  rather than aliased or silently reinterpreted. Portable source rounding
  barriers, bounded transport/report encoding and immediate nonfinite refusals
  are limited to the named opt-in path. AMD64/ARM64 native/WASM byte equality
  and same-architecture legacy preservation are release gates; cross-builds
  alone are insufficient. No universal portable-versus-legacy decision equality,
  Pine parity, cost completeness or consumer browser qualification is claimed.

- Include reviewed frozen-level primitives/lifecycle, strict source transport,
  quote-input admission and synthetic lifecycle/fee traces (PR86–91).
  `frozenLevelBreakout` remains compile-only and refuses generic execution.
  Regime, gold-flag and adaptive references retain their dedicated native
  routes; timed-return retains its separate calendar profile. These families
  do not become selectable browser strategies through this producer release.

### Release and consumer-adoption gates

- Freeze the reviewed commit/tree after the diagnostics and source-plumbing
  changes land. Rebuild from a clean standalone checkout using the resolved
  release toolchain, `CGO_ENABLED=0` and `-trimpath`; record CLI VCS revision,
  `vcs.modified=false`, toolchain identity, matching `wasm_exec.js`, manifest,
  archive contents and asset checksums. A changed source commit requires new
  stamped assets even if the source tree is identical. Ordinary tar archive
  hashes can vary; also verify every extracted file against the source.
- Preserve all 128 v0.24.1 goldens (68 parse and 60 run) and verify the complete
  current corpus (122 parse and 100 run) with explicit known classifications.
  These are different counts. Retain full source checks, clock fixture parity,
  down-shock native-report/actual-WASM regressions and Master portable/legacy
  gates. Do not regenerate goldens, normalize away failures or infer consumer
  readiness from producer CI alone.
- After publication, consumers must replace provisional identities with the
  verified tag/assets and update the pin, parser/engine WASM, matching shim,
  grammar projection, inventory and runtime identity together. Require final
  exact-head app CI and desktop/mobile browser acceptance against those bytes.
  App merge, preview and deployment remain separate owner decisions; prevent
  automatically queued previews from bypassing that deployment gate.
- Roll back app adoption as one unit to the previous v0.24.1 pin/assets and
  rebuild the frontend. Do not move or delete a published tag, weaken refusal
  guards or substitute JavaScript clock-range execution. No database/data
  migration, trading-default change, observer, order or historical-performance
  qualification is included.

### Included source changes

- Add the separate `adaptiveVolumeFlag` native raw reference family (HT-185),
  with strict explicit INITIAL/TWEAKED/SNAPSHOT_C/CUSTOM settings, causal
  source snapshots, frozen pending stops, delayed brackets, close-activated
  stop exits and retained terminal states. Reuse canonical position/close
  machinery without altering existing broker semantics. Dedicated
  `adaptive-flag-report` only; generic and engine-WASM routes fail closed.
  No sizing, costs, account economics, TradingView parity, performance
  or app adoption is claimed. New arithmetic uses explicit float64 barriers;
  fixed `BINARY64_ORDERED_V1` records sequential seed/direct comparisons.
  CPython-default/Pine bitwise parity is not claimed; cross-architecture
  qualification remains a separate gate. Optional request-only evaluation
  windows preserve continuous warmup with a flat pre-start broker and exclude
  rows opening at or after the explicit end. Full-input mode remains.

- Add the strict Go-only `goldFlagReference` family (HT-182), preserving the
  fixed PR388 detector and observed-row clocks in the named causal coarse-OHLC
  stress scenario. Reuse the canonical broker with a dedicated opening-target
  precedence opt-in, leaving legacy defaults unchanged. Native `gold-flag-report`
  only, with signal snapshots, order/event/coverage traces, local alternatives,
  explicit terminal exposure and five fixed diagnostic costs. Generic, grid,
  prefix and engine-WASM paths refuse execution. Invented fixtures preserve
  prior goldens. No observed-fill, profitability, app adoption or live
  readiness claim is included.

- Add the fixed native-offline Master Structural v10 research family (HT-177),
  with separate historical-source and stable-protected policy bundles, explicit
  H4 clock horizons, continuous reference prices, rounded illustrative quantity,
  and full order/activation/terminal audits. Reuse qualified v9 primitives
  without changing either v9 policy. The legacy native path uses
  `master-report`; generic fixture/column/report/grid/prefix/checkpoint routes
  remain refused. The separately selected portable CLI/WASM contract is
  described above. New parsing and invented synthetic
  cases preserve all prior goldens. No Pine/broker parity, consumer adoption,
  deployment or historical performance claim is included.

- Add the fixed, Go-only Regime Engine DSL interpretation (HT-175), with
  source-like-v1 and audit-baseline-v1 policies, a dedicated engine/regime API
  and regime-report CLI, indicator/order/terminal-exposure audits, and strict
  refusal from generic report/grid/prefix/WASM execution. Preserve existing
  conformance goldens; new parser cases and invented native synthetic tests
  qualify the bounded implementation. HMA floor-half arithmetic, native OHLC
  path, notional sizing and conditional costs are explicit; Pine parity and
  broker contract mapping remain unverified. Consumer adoption requires
  separate qualification.

- Add the clock range breakout setup family (`type: clock range breakout`,
  id `clockRangeBreakout`) specified by HT-167: a fixed-UTC-offset clock range,
  buy/sell stop orders with OCO and one entry per range day, a percent stop
  sized from the post-slippage fill, and a required clock close. The family
  runs its own broker loop with no inherited session, gate, target,
  breakeven, trailing, partial, max-hold or ATR-stop behavior, and rejects any
  other authored directive as a compile error. New phrases: `clock UTC<±H[:MM]>`,
  `orders expire`, `buffer N pips`, `close positions at`, and `stop N percent`
  and `range HH:MM to HH:MM` in this family. The reviewed pip registry moves to
  `dsl.ReviewedInstrumentPip` (engine behavior unchanged). Add 31 parse and
  38 run conformance cases; all earlier goldens are unchanged. Browser
  adoption, a consumer pin, strategy registration and historical comparison
  require separate qualification. JavaScript clock-range execution remains
  unsupported.

- Correct numeric Failed Breakout `target N R` lowering to `fallbackR`, the
  field the execution engine consumes. Previously the accepted phrase wrote
  an unused `r` field and silently retained the default 1R fallback. Opposite
  edge priority and the minimum-reward filter remain unchanged. Add invented
  long/short trade-outcome and native/WASM regressions. This prospective
  semantic correction requires explicit consumer adoption and acceptance.

- Parse opening-range `first N minutes` as minutes and disable candle-count
  mode, as specified. Preserve candle/bar units and integer-hour UTC slots;
  reject malformed or ambiguous duration prefixes and non-positive,
  fractional or overflowing durations rather than silently coercing them.
  Add two synthetic parse goldens; no existing run golden changes. This is
  a prospective parser-semantic correction requiring explicit consumer
  adoption and acceptance.

- Reject a supplied timed-return calendar on another strategy family before
  generic fixture execution or shared/native preparation. This closes mixed
  profile admission without changing valid existing family behavior.

- Add a native-only opt-in timed-return family and `timed-report` CLI with
  typed opening/completed-close anchors, pinned timezone/calendar inputs,
  one-bar-delayed scheduled fills, one fixed unit and dated admission audit.
  Two invented run fixtures and one parse fixture exercise the new contract;
  existing goldens retain their semantics. Consumer adoption of this bounded
  calendar profile requires separate qualification.

## [0.24.1] — 2026-10-01

- Add exact-source Go run fixtures for four XAUUSD research strategies using
  frozen 5m/15m bars, and emit the causal seasonality diagnostics used by
  JavaScript in trade metadata. These fixtures add implementation coverage;
  they do not promote the strategies.

## [0.24.0] — 2026-10-01

- Fix named-level flag arming to honor long-only and short-only filters.
  Disabled sides do not consume a level attempt. Add a short-only execution
  fixture using a long breakout; existing goldens remain unchanged.

## [0.23.0] — 2026-10-01

- Add the named-level flag setup with causal completed-level snapshots,
  one-attempt-per-level UTC-day arming, signal-ATR stops and body filters,
  optional known-level admission and minimum signal-risk filters, and 24-bar
  hold defaults. Add parse and run conformance fixtures; prior run goldens are
  unchanged.

## [0.22.0] — 2026-10-01

- Add the v7 RMV filter and reusable ATR-range position series. RMV uses the
  current ATR within an inclusive trailing ATR window, requires a full seed
  and lookback, and is unavailable when the ATR range is flat.

## [0.21.0] — 2026-09-30

- Add the research-only down-shock rebound family with completed 15m context,
  immediate or reversal 1m entry, fixed source ATR stops, optional ATR or bp
  targets, and elapsed-time/gap exits. The owner approved merge and release. The historical study failed;
  this does not promote a strategy.
- Extend run fixture v1 with optional source timeframe and source bar columns
  so the producer corpus can exercise native source/entry execution. Add seven
  synthetic run goldens and seven parse goldens; existing goldens are unchanged.


## [0.20.0] — 2026-09-29

- Add an exact-source Go run golden for the XAUUSD daily Dual EMA research
  strategy. Its 2,244-bar fixture verifies 52 trade rows. The full 8,244-bar
  local route matches all 200 JavaScript trades within 1e-8 for trade identity
  and financial fields. This adds conformance coverage; it does not promote
  the strategy or approve Forward execution.

## [0.19.0] — 2026-09-28

- Keep the base failed-breakout management defaults when its setup type is
  stated explicitly. Other setup families still apply their family defaults
  (#71).

## [0.18.0] — 2026-09-28

- Add Go parsing and execution for causal seasonality filters. The filters use
  prior bars for UTC hour, weekday, week and month buckets, with explicit
  sample thresholds and rolling lookbacks (#59).
- Visit every Supply-Demand zone created on the same bar before trying older
  zones. This restores the EURUSD 1h trade omitted by Go and matches the
  AUDUSD 1h zone, stop and target selected by JavaScript (#69).

## [0.17.0] — 2026-09-28

- Parse `stop size min ... max ...` as one directive inside a `risk` block so
  explicit stop bounds override setup defaults (#67).
- Add an exact-source Go run golden for the Daily S&D research strategy on
  XAUUSD 4h. Its 4,504-bar fixture verifies 43 trade rows; this is a
  conformance check, not performance or deployment evidence (#66).

## [0.16.0] — 2026-09-28

- Minor bump: fair-value-gap formations and lifecycle updates continue while a
  position is open. A later retest can now use a gap formed during that trade
  (#63).
- Reports and grids warn when a weekend extreme fade route has no Monday
  00:00 UTC bar after twelve contiguous 4h bars. On those routes the setup
  cannot enter, so zero trades are not a strategy result. Trades and existing
  goldens are unchanged (#62).
- Add exact-source Go run goldens for the VSA Weekly-Extreme Wide Payout
  research strategy on XAUUSD 1h and 4h. These fixtures verify Go execution;
  they do not establish economic or deployment evidence (#64).

## [0.15.0] — 2026-09-28

- Minor bump: named-level sweeps deduplicate daily by resolved level price and
  enforce the shared minimum and maximum stop-distance bounds (#57).
- The parser preserves strategy names, descriptions and inline setup blocks
  in standalone and braced declarations (#58).
- Source-entry routes keep pre-coverage bars for context warmup but discard
  orders formed before chart coverage begins. Scheduled chart orders are
  matched to captured source orders by source-bar index (#60).

## [0.14.0] — 2026-09-28

- Minor bump: Go now parses and applies `local weekday`/`weekday`,
  `local hour`/`hour`, and `open location` market filters. Weekday and hour
  filters use the browser runtime's fixed UTC+10 clock; allowlists and
  exclusions run during entry admission, and open-location allowlists use
  causal session context. The parser rejects invalid values and unsupported
  negated open-location filters. A parse conformance fixture covers the new
  phrases.

## [0.13.0] — 2026-09-28

- Minor bump: Go fair-value-gap entries now apply the shared market gates
  after a valid retest is identified, matching the browser runtime. A rejected
  market gate leaves the gap available for a later retest.

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
