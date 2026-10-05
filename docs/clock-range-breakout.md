# Clock range breakout: Go producer evidence and reconciliation plan

Backlog task: HT-167. Contract: [clockRangeBreakout.md](../spec/dsl-spec-families/clockRangeBreakout.md).

This note maps the twelve acceptance groups to the committed fixtures and tests,
and records the ledger a later historical comparison must fill. It describes
the unreleased Go producer only. It does not report a historical result, an
edge, a basket decision or permission for forward or live use.

## Status

| Item | State |
| --- | --- |
| Go parser, compile diagnostics, engine, serialization | Implemented, unreleased |
| Native Go and Go/WASM execution of every new case | Compared byte for byte by `scripts/checks/clock-range-breakout-parity.mjs` |
| Independent semantic review of the engine | Pending (author self-review only) |
| Checkpoint resume (`RunPrefixResumable`) | Unsupported for this family; replay prefixes only |
| App JavaScript runtime | Not implemented. Go/WASM/JS parity is pending |
| Release tag, consumer pin, strategy registration | Not done; separate reviewed steps |
| Historical reconciliation against the archival script | Not run; see "What is still needed" |

## Acceptance groups and their fixtures

Parse cases are `conformance/parse/<name>.strat`; run cases are
`conformance/run/family-clock-range-breakout-<name>.*`. Go tests carry the
hand-derived arithmetic in comments. `crbCorpusCases` in
`engine/clock_range_breakout_corpus_test.go` holds the expectation for every
run case; the committed fixtures are checked against that table, and
`CRB_WRITE_CORPUS=1 go test ./engine ./dsl` rewrites the inputs from it.

| Group | Requirement | Conformance cases | Go tests |
| --- | --- | --- | --- |
| 1 | Clock equivalence, dates, offsets | run: `ordinary-long`, `ordinary-long-utc2`, `midnight-range`, `year-boundary-negative-offset`, `year-end`, `month-end`, `leap-day`, `fractional-offset-minus-3-30`, `fractional-offset-plus-5-30`; parse: `setup-clock-range-breakout-utc2`, `setup-clock-range-breakout-long-only-fractional`, `setup-clock-range-breakout-quoted-metadata`, `diagnostic-…-range-start-after-end` | `TestClockRangeBreakoutClockDatesAndOffsets`, `TestClockRangeBreakoutEquivalentOffsetsGiveIdenticalTrades` |
| 2 | Compile rejection | parse: `diagnostic-clock-range-breakout-*` (inline-forbidden-leading/middle/trailing, missing-required, offset-out-of-bounds, offset-minutes, malformed-offset, malformed-time, expiry-equals-range-end, expiry-after-close, unsupported-pip-route, negative-buffer, nan-buffer, percent-zero, percent-over-100, forbidden-sessions-before-sections, forbidden-target-reordered, forbidden-local-hour, forbidden-atr-stop, forbidden-neutral-hold-limit) | `TestClockRangeBreakoutCompileRejections`, `TestClockRangeBreakoutRejectsForbiddenDirectivesInEveryOrder` (every forbidden directive x every section x all 120 section orders), `TestClockRangeBreakoutParseIsOrderIndependent`, `…RejectsForbiddenDirectivesInsideInlineBlocks`, `TestQuotedMetadataKeepsDirectiveWordsInOtherFamilies` |
| 3 | Route alignment | parse: `setup-clock-range-breakout` (5m), `…-long-only-fractional` (1m and 5m), `diagnostic-…-route-15m`, `…-range-start-off-grid`, `…-range-end-off-grid`, `…-expiry-off-grid`, `…-close-off-grid`, `…-implicit-default-route`; run: `one-minute` | `TestClockRangeBreakoutRouteAlignment`, `TestClockRangeBreakoutFailsClosedOnUnknownRuntimeRoute` |
| 4 | Coverage and input validation | run: `coverage-33-accepted`, `coverage-32-rejected`, `coverage-35-missing-final-slot`, `flat-range-rejected`, `empty-range-rejected`, `range-end-bar-excluded` | `TestClockRangeBreakoutCoverage`, `…EmptyAndFlatRangesAreRejected`, `…RangeEndBarIsExcludedFromRangeButCanFill`, `…RejectsMalformedSeries`, `…RejectsMalformedFixtureRows`, `TestOtherFamiliesKeepTheirSeriesContract`; WASM and native malformed-series check in the parity script |
| 5 | Orders and lifecycle | run: `ordinary-long`, `ordinary-short`, `gap-through-long`, `gap-through-short`, `numeric-tie`, `nearer-trigger-short`, `later-touch-stop`, `long-only-sell-touch`, `expiry-bar-excluded`, `expiry-last-bar-fills`, `coverage-32-rejected`, `multiple-days` | `TestClockRangeBreakoutFills`, `…BothTouchedNearestTriggerWins`, `…FirstSideCancelsOtherAndDayIsUsed`, `…SideAllowance`, `…ExpiryBoundaryExcludesExpiryBar`, `…MultipleDays`, `…NonpositiveTriggerRejectsTheDay`, `…PlacementIsBlockedByAnOpenPosition` |
| 6 | Exit ordering | run: `numeric-tie`, `nearer-trigger-short`, `later-gap-stop`, `later-touch-stop`, `clock-close-before-extremes` | `TestClockRangeBreakoutLaterBarStopsAndClockPrecedence`, `…CloseInstantBarCannotEnter` |
| 7 | Missing close, finalization, resume | run: `missing-close-quote`, `terminal-last-close` | `TestClockRangeBreakoutMissingCloseQuoteAndFinalization`, `…PrefixRetainsOpenStateUntilFinalization` (replay), `…PlacesWhenTheFinalRangeBarCloses`, `…FirstPostRangeQuoteAfterExpiry`, `…CheckpointResumeIsUnsupported`, `…ExecutionWindow` |
| 8 | Numeric tie | run: `numeric-tie` | `TestClockRangeBreakoutNumericTieFixture` |
| 9 | Money and default isolation | run: `pip-buffer-xauusd`, `pip-buffer-eurusd-short`, `slippage-fee-sizing`, `explicit-zero-risk`, `no-inherited-management` | `…PipBufferConversion`, `…SlippageMovesFillStopAndSize`, `…RiskPresence`, `…InheritedManagementDoesNotLeak`, `…DirectConfigRiskBoundary` |
| 10 | Causality | run: `ordinary-long` (fills at 14:05 UTC+10, outside the inherited windows), `fill-bar-shape`, `fill-on-open` | `…BareFamilyIgnoresInheritedWindows`, `…FillBarShapeAndFillOnDoNotChangeTheTrade` |
| 11 | Envelope and config parity | every golden (null `tp`/`initialTp`, `reason`/`rule`, nested `meta.clockRangeBreakout`, indices, timestamps) and every parse golden | `scripts/checks/clock-range-breakout-parity.mjs` (native vs WASM vs golden); JS comparison pending adoption |
| 12 | Historical reconciliation | synthetic defect regressions below; historical comparison pending | this document |

Prior-position blocking (group 5) cannot occur with valid data: the final slot
of the next day's range always sits at or after the resolved close, so the due
close has already closed the earlier position. The placement guard is covered
by a direct unit test only.

## Reconciliation ledger (group 12)

The archival comparison script is a baseline, not the oracle. It is pinned,
with its invocation and cost identity, in the private backlog task. Nothing
from the private research or market data is copied here.

A historical comparison fills one ledger. For each range day in the compared
period:

| Column | Meaning |
| --- | --- |
| `dayKey` | Declared-clock date, `YYYY-MM-DD` |
| `baseline` / `v1` | Whether each side traded that day |
| `side`, `entryT`, `entryPrice`, `exitT`, `exitPrice` | Raw fill comparison at matching resolution, buffer and slippage, before sizing |
| `class` | One class from the table below, or `unexplained` |
| `size`, `pnl` | Compared only after raw fills match, with fixed-dollar risk and fees stated |

Acceptance needs zero `unexplained` rows. A percentage of matching calendar
days is not an acceptance measure, and no baseline defect is reproduced to
improve agreement.

| Class | Difference | v1 behavior | Synthetic regression |
| --- | --- | --- | --- |
| `missing-final-slot` | Baseline can trade a day whose final range slot is missing | Day rejected (`missing-final-slot`) even at 35 of 36 bars | `coverage-35-missing-final-slot` |
| `terminal-omission` | Baseline can drop a trade with no qualifying close quote | Exits at the next real open, or at the last close with `end-of-test` | `missing-close-quote`, `terminal-last-close` |
| `cross-utc-day-range` | Baseline does not support a range that crosses UTC midnight | Day keyed by the declared-clock date | `midnight-range`, `year-boundary-negative-offset` |
| `entry-bar-target` | Baseline target branch ignores entry-bar target hits | No target exists in v1; the baseline target rows cannot be used as evidence | `no-inherited-management` |
| `modeled-ordering` | Both-edge ties and whole-entry-bar stops | Nearer trigger wins, tie goes long; entry-bar stop exits at the stop. These are modeling conventions, not a defect fix | `numeric-tie`, `nearer-trigger-short` |
| `sizing-and-cost` | Percent stop, post-slippage fill, fixed-dollar risk, fees | Reported separately after raw fills match | `slippage-fee-sizing`, `pip-buffer-xauusd` |

Affected-day counts and impact are not measured. They are unknown until the
comparison runs.

## What is still needed for the historical comparison

1. Authorization to run it.
2. The pinned baseline script checkout and its recorded invocation and cost mode.
3. The 1-minute bar data the baseline used (identity and checksum), and the
   same bars for the Go run; this repository holds no market data.
4. A run of the Go producer at matching resolution, buffer and slippage, with
   per-day output (`PreparedRun.ClockRangeDays` and the trade list) to build
   the ledger.

Until then group 12 is open. This producer change covers the synthetic defect
regressions only.
