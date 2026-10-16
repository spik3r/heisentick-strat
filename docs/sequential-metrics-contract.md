# Sequential companion accounting and metrics v1

This is a versioned synthetic qualification surface. It implements the bounded
HT-229 claim in backlog PR #467 and does not activate a strategy, release,
historical study or paper-forward route. The same Go execution owns trades and
metrics. The existing generic run and trade envelopes retain their meanings.

## Surface and source admission

`engineRunSequentialFixture(fixtureJSON, source)` is a separate WASM export.
The native `enginewasm --sequential-backtest fixture.json source.strat` dispatch
exists to qualify that same adapter. It is not the `heisentick report` command.
The four source identities and hashes are pinned in
`sequential-source-freeze.json`. These are fixed XAUUSD/1h legacy controls and
SYNTH/5m Full E1/E2 controls. Risk/cap constants are invented fixture inputs.

The input is the original six-column OHLCV run fixture, bounded by the adapter,
without source/HTF projection, transformed bars, calendar declarations,
execution-window overrides or policy checkpoints. Missing intervals on Full
retain the native typed refusal. Price gaps at the next ordinary open retain
their accepted broker behavior. Duplicate JSON keys, fractional timestamp
tokens, nonfinite values, malformed rows and mismatched source/route identity
are refused, not dropped or coerced into empty success.

Legacy entries remain signal-close fills with existing end-of-test liquidation.
Full entries remain next-open fills with natural stop/target/time exits. A held
Full position at the terminal bar refuses the whole result. Neither policy is
changed to make a finite chart interval appear complete.

## Envelope

The success envelope has schema `strat-sequential-backtest-result-v1` and
contractVersion 1, a request/source-hash-bound identity, unchanged `run`,
`metrics`, and explicit capabilities. A failure contains only schema/version
and its typed `error`; it contains no successful or partial run/metrics.
Full input/configuration/gap/terminal refusal identity remains visible.

`metrics.basis` is `sequential-broker-close-mtm-v1`. V1 qualifies headline
statistics, completed trades and close-bar equity. Group/session/monthly,
R-distribution, comparison and portfolio widgets remain unavailable until
their own Go projections qualify. Consumers must not fall back to JS formulas.

## Raw accounting and serialization

The private collector observes the same broker's original arithmetic and event
order. Generic callers pass no collector; reset clears the prior run's pointer.
The exact source gate precedes allocation, preventing adaptive-family callers
from enabling this collector through JSON.

- The broker debits the entry commission immediately. Retained `run.trades[].pnl`
  is the existing exit-credit P&L, including the exit fee but excluding the
  earlier entry debit. Its meaning and 15-significant-digit serialization stay
  unchanged. The existing per-unit fee applies on entry and exit for both
  Sequential families. Existing adverse fill slippage applies to every entry
  and exit, including Legacy terminal liquidation; this projection does not
  introduce a new cost model or choose historical-study costs.
- Companion `tradeAccounting`, aligned by closed-trade index, supplies nominal
  raw `entryFee`, `exitFee` and all-fee `netPnl = exitCredit - entryFee`.
- Account `headline.net` is terminal raw broker realized P&L. It is not
  `sum(serialized run.trades[].pnl)` and is not calculated by subtracting large
  ending and starting equity values.
- New companion values use standard finite binary64 shortest-round-trip JSON.
  Signed zero is normalized at the output boundary. Nonfinite raw/derived
  values cause a typed refusal before serialization; the generic serializer's
  nonfinite-to-zero fallback is never a metrics policy.

Actual realized-before/after snapshots must form an exact state chain, ending
at the captured broker state. Entry arithmetic can fuse on ARM64, so replaying
an independently rounded `before - entryFee` is not an exact invariant. A
separate, finite operation/operand-sensitive error budget covers alternative
per-trade reductions. There is no blanket monetary epsilon, relative-to-one
floor or infinite MaxFloat64 ULP allowance. Separately serialized per-trade nets
need not sum bit-for-bit to separately serialized account net.

## Equity and close-bar MTM drawdown

Effective starting equity uses the existing omitted/zero-to-10000 normalization,
then must be finite and positive on this dedicated surface.

After each original input bar has completed its existing execution sequence:

1. Cash is effective starting equity plus captured realized P&L.
2. Open unrealized P&L uses that original bar's close, actual slipped entry,
   side and raw quantity. No hypothetical exit fee or exit slippage is charged.
3. Equity is cash plus open unrealized P&L.

Each exported `equity[].t` is that original input bar's timestamp (the bar open),
not its close time or an inferred calendar timestamp.

There is exactly one mark per original bar. Same-bar exits/reentries include all
actual debits and credits before the mark. A queued opportunity alone changes
no equity. Legacy terminal liquidation replaces the final bar's mark, never
adds another one. The last successful flat mark and endEquity use the same
calculation. No partial curve escapes a Full terminal refusal.

The drawdown peak starts at initial equity, before the first mark. Each mark
updates its contemporaneous peak before reading drawdown. `maxDD` is the maximum currency drawdown;
`maxDDpct` is the independently maximum percentage drawdown. Their maxima may
occur at different bars: start 100, marks 80, 200, 170 gives 30 currency and
20 percent. Negative finite equity and drawdown above 100 percent remain valid.

The display label is **Close-bar MTM drawdown**. It is neither intrabar worst
case risk nor the existing report's closed-trade/final-peak drawdown nor the old
JS percentage sampled only at the maximum absolute drawdown. Other families'
statistics are not rewritten or relabeled by this projection.

## Headline definitions

- `trades` is the closed-trade count. Wins have raw all-fee net strictly above
  zero. Zero is non-winning; no epsilon changes classification.
- `winRate` is a percentage. Profit factor is positive gross winnings divided
  by absolute non-winning losses. Positive winnings with zero loss amount,
  including winners mixed with flats, yield null with `no-losses`. All-flat
  completed trades yield numeric zero; loser-only runs also yield numeric zero.
- `expectancy` is account net per completed trade. `avgWin` is the mean positive
  trade net; `avgLoss` is the signed mean non-winning trade net, including flats.
  `worstLoss` is the minimum trade net with a zero floor when no trade loses.
- Win and non-winning streaks follow native closed-trade order; zero extends
  the non-winning streak. `avgHoldBars` averages exitIndex minus entryIndex,
  including zero duration for a same-bar exit.
- `endEquity` is starting equity plus raw account net. `returnPct` uses account
  net and positive effective starting equity, without funding assumptions.

On a valid nonempty-bar run with no trades, net/return/DD/streaks are genuine
zero, end equity equals start and the curve is constant. Win rate, PF,
expectancy and average hold are null with `no-trades`. No winning trades gives
null avgWin with `no-winning-trades`; no non-winning trades gives null avgLoss
with `no-nonwinning-trades`, including the no-trade case. A non-winning class consisting only of flats has
numeric avgLoss zero. Reason fields are omitted for defined numeric values.
A Full `no_next_bar` opportunity without a held position is a valid no-trade
run, not an incomplete-terminal refusal. Retained trade `pnl`, companion
`netPnl` and account `headline.net` are distinct quantities; consumers use the
Go-provided quantity appropriate to the label and never recompute statistics
from the 15-digit exit-credit field.

## Qualification and the HT-245 boundary

Minimal broker hooks are required to observe Legacy's single execution; prefix
replay, alternate loops or per-bar reruns would change the qualified surface.
No original trading arithmetic expression is intentionally split or replaced.
Nevertheless, source hooks can alter compiler/FMA behavior. A nil collector is
not evidence of bit neutrality.

Before acceptance, compare prechange artifacts, capture-off and capture-on on
native amd64, native ARM64 and actual Node-hosted Go/WASM. Preserve existing
goldens and generic outputs. Identities, counts, signs, classifications, streaks
and null reasons must match exactly. Near-cancellation categorical mismatch
halts qualification; inherited floating-output bounds cannot waive it.

Changing broker.go invalidates the adaptive constructor fingerprint. Independent
resource/domain review must account for fixed broker overhead and prove no
adaptive caller can allocate capture before renewing that fingerprint. This
bounded joint review does not close HT-245's broader arithmetic investigation.

No architecture, browser, performance, historical or release pass is implied by
this document. Actual commands, candidate hashes, results and limitations belong
in the separate qualification receipt.

## Owner-approved exact target arithmetic relation — 2026-10-10

The owner approved this qualification refinement at 12:59:03 UTC: preserve
engine outputs and require the exact result of each verified platform's
arithmetic, with every unexplained difference failing. This replaces universal
raw cross-target byte equality only for the independently replayed accounting
and metric operations below. It does not change any runtime operation, permit
a numerical tolerance, or establish a universal cross-compiler guarantee.

The verifier uses CPython 3.12.x standard-library integer/rational arithmetic
with binary64 round-to-nearest, ties-to-even and explicit signed-zero rules.
Missing or unsupported Python fails clearly. The qualified Go compiler is
exactly Go 1.22.12 with ordinary `CGO_ENABLED=0`, `-buildvcs=false`, `-trimpath`
builds and no compiler/FMA overrides or experiments. Native execution must
really be Linux amd64 or Linux ARM64 with the pinned architecture feature
settings; the paired WASM runs in Node. Source identities and selected generated
instruction schedules are pinned in
`../scripts/checks/sequential-arithmetic-schedules.json`. Unknown versions,
architectures, options, source hashes or instruction schedules fail closed.

The prechange source baseline is fixed at
`f8aba9ef13e22ace21ce31f2ed06cd86ad53464d`, tree
`8e4ef994a636f2125bab2bb909c28fca5db2351d`. Qualification verifies the baseline
source files against that authenticated tree before building. Arbitrary
standalone binaries or a caller-provided label cannot establish prechange
provenance. No verifier network fetch or dependency installation is performed.

- Same-target prechange/current capture-off/capture-on, repeated runs, generic
  and column exports, raw helper and real companion remain exact comparisons.
- Cross-target input/cost/fill/size/points operand bits, source/request and
  trade identities/order, decisions, counts, signs, classes, streaks, null
  reasons and refusals remain exact. An upstream or categorical mismatch
  cannot be explained away by the arithmetic relation.
- The verifier independently replays each target's entry debit, exit credit,
  realized-state mutation, all-fee trade net, every original-bar close MTM
  mark, all reductions/ratios and both independent drawdown maxima. Each
  actual result must match its predicted bits exactly. Every differing leaf
  must be explained by those pinned operations; missing, extra or unknown
  evidence and nonfinite reference results fail.
- Raw negative zero is retained in the evidence. Only the existing companion
  projection normalizes zero; normalization cannot hide a raw sign/category
  change in the comparison.

The inspected ARM64 entry debit fuses the fee multiplication with subtraction;
its exit credit rounds gross P&L before the fused fee-product subtraction.
The inspected amd64/WASM schedules round those products separately. Account
addition remains separately rounded. No alternative fusion orientation or
algebraic reassociation is admitted merely because the language permits it.

For projected percentages, division is rounded before the final multiplication
by 100: `returnPct = (net / startEquity) * 100`,
`winRate = (wins / trades) * 100`, and each
`drawdownPct = (drawdown / contemporaneousPeak) * 100`. These are not specified
as one rounding of the final exact rational. Ordered trade reductions precede
PF and mean divisions. The selected report schedule must match its pin; the
accounting reconciliation budget elsewhere in this document is not a ratio
tolerance. The independent corpus may retain exact economic rationals beside
these operation-ordered binary64 checks without changing its expected values.

Qualification receipts separately state whether raw cross-target outputs are
byte-identical and whether all observed differences are exactly reproduced.
A qualified relation must never be reported as raw-bit identity. A failed or
incomplete check remains a failure. Actual native ARM64/amd64 and Node-WASM
evidence plus fresh independent verifier review are required after the final
source union; cross-compilation alone is insufficient.

## Gold paper source-pin review (2026-10-16)

Gold paper source additions changed the complete compiler-input inventory, so
Sequential qualification refused the old source pins before running a case.
Review renewed only `sources` and each target's `buildInputs` in
`sequential-arithmetic-schedules.json`. The global inventory adds
`engine/paper_bidask_dual_ema.go` and `engine/paper_bidask_experiment.go` and
changes five existing gold CLI, bridge and engine files. Each dependency closure
adds those two files and changes four hashes; its package set is unchanged.

Two independent compile-only observations with Go 1.22.12 compared all seven
pinned instruction streams on Linux AMD64, Linux ARM64 and WASM. All 21 streams
matched their previous hashes and instruction counts. Instruction pins,
accounting schedules, source parents, compiler flags, toolchain requirements and
the verifier remain unchanged.

The unchanged full qualification then passed on actual Linux ARM64 with
Go 1.22.12, CPython 3.12.14 and Node 24.21.0, using the authenticated prechange
source tree and offline execution: 88 cases, 64 exact arithmetic pairs and all
19 verifier self-tests. No evidence was missing and no mismatch was unexplained.
Seven cross-target comparisons have the existing exact pinned-target arithmetic
explanation; raw cross-target values are not all bit-identical. An initial local
attempt stopped because the isolated image lacked Git; adding Git allowed the
same provenance checks to run without changing them.

This source-pin review establishes neither market profitability nor release,
consumer adoption, deployment or forward activation.

## Price momentum fixed ATR stop: layout review (2026-10-16, pending)

PR142 adds `StopATR float64` to `priceMomentumParams`, a struct embedded by
value in `flagParams`, which `broker` embeds. The struct grows from 16 to 24
bytes and now ends at broker offset 6064 instead of 6056; the broker grows
from 9,704 to 9,712 bytes (Go 1.22.12, 64-bit targets). No protected function
body was edited, but every broker field after the new member moves by 8 bytes.

Sequential qualification therefore refuses the committed manifest before any
case runs (CI run 38833834951: source inventory). Seven sources differ:
`dsl/parser_price_momentum.go`, `engine/config.go`, `engine/config_setups.go`,
`engine/conformance.go`, `engine/family_price_momentum.go`, `engine/run.go` and
`engine/runner.go`. Each target's dependency closure changes the same seven
hashes; package sets are unchanged.

Two independent Go 1.22.12 observations compared all seven pinned instruction
streams on Linux AMD64, Linux ARM64 and WASM: a cross-compile on Darwin ARM64,
and a build inside a Linux ARM64 container (native ARM64 and WASM, AMD64
cross-compiled). The observation harness uses the proof script's own
`source_hashes`, `dependency_inputs` and `instruction_lists`, and first
reproduced every pin on main 557e4d1 exactly. Both observations agree byte for
byte. `finish` and `ProjectSequentialMetrics` are unchanged on all targets.
`openPosition`, `closePosition` and the collector's `entry`, `exit` and `mark`
change on all three targets, 15 streams in total. Every changed instruction
is one of:

- the same opcode with only a broker-field displacement at or above offset
  8,000 moved by exactly +8 (all AMD64 and WASM changes);
- on ARM64, `MOVD $8184, R3; ADD R3, R0, R21` becoming `ADD $8192, R0, R21`,
  because 8,192 fits the shifted 12-bit immediate form, one instruction shorter
  in `openPosition`, `closePosition` and `entry` (counts 180 to 179, 289 to
  288, 237 to 236);
- on ARM64, a branch or `ADR` PC-relative operand moved by exactly -4 after that
  collapse (`B*`, `CB*`, `TBNZ`, `JMP`, and `ADR` into `runtime.duffcopy` /
  `runtime.duffzero`).

No floating-point, integer arithmetic, comparison or call instruction differs
in opcode or in any operand other than those listed. The review repair on top
of PR142 (bounds and invalid-stop checks) produces instruction streams
identical to the PR142 head; only the new field changes them.

A proposed manifest that renews only `sources`, each target's `buildInputs`
and the 15 changed instruction pins (schema, versions, build settings,
`sourceParents` and accounting schedules byte-identical) is attached to the PR
for proof-owner review. It is not committed. The committed instruction pins,
the verifier and compiler flags are unchanged.

Local qualification ran in a Linux ARM64 container (Go 1.22.12, CPython
3.12.15, Node 24.21.0, all checksum-verified downloads), executing the CI
job's own steps on the final tree:

- With the committed manifest, every step but one passed. Sequential companion
  parity failed exactly as in CI (0/88 cases, hard failure on the source
  inventory).
- With the proposed manifest, committed locally only, all 31 steps passed.
  Sequential companion parity ran 88/88 cases, 64 exact arithmetic pairs and
  19/19 verifier self-tests, with 0 unexplained and 7 explained cross-target
  comparisons (87 explained raw leaves). All nine same-architecture checks
  passed, including pre-change baseline f8aba9e to current on native and
  Node-WASM, so no trade arithmetic changed.

These are local results. Exact-head CI on GitHub and the proof owners' review
of the pin renewal are still required before merge. This review establishes
neither release, consumer adoption, deployment nor forward activation.
