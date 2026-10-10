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
  unchanged.
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

There is exactly one mark per original bar. Same-bar exits/reentries include all
actual debits and credits before the mark. A queued opportunity alone changes
no equity. Legacy terminal liquidation replaces the final bar's mark, never
adds another one. The last successful flat mark and endEquity use the same
calculation. No partial curve escapes a Full terminal refusal.

The drawdown peak starts at initial equity, before the first mark. Each mark
updates its contemporaneous peak. `maxDD` is the maximum currency drawdown;
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
  completed trades yield numeric zero.
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
with `no-nonwinning-trades`. A non-winning class consisting only of flats has
numeric avgLoss zero. Reason fields are omitted for defined numeric values.

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
