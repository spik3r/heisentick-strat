# HT-177: Master Structural v10 native offline reference

Canonical task: [HT-177](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-177-master-structural-dsl-historical-test.md).

This separately named candidate preserves the qualified Regime Engine v9
profiles. It is a fixed Go research interpretation of the received v10 logic,
not compiled Pine, broker parity, or production readiness. No release, app
adoption, registration, deployment, data purchase or orders are included.

## Runnable boundary

Only `engine/master.Run` and `heisentick master-report` execute this family.
The generic broker, report/grid, prepared/shared/prefix/checkpoint and engine
WASM paths fail closed with `master-structural-native-dedicated-runner-required`.
Parsing does not authorize browser execution. Exactly two mode bundles exist:
`SOURCE_HISTORICAL_REFERENCE` and `PROTECTED_STABLE_REFERENCE`. Signal periods,
H4 phase, precision, costs, sizing and management cannot be combined or tuned.

```text
heisentick master-report --dsl-file=source.strat --m5-file=owned-input.bin \
  --warmup-from=2025-12-01T00:00:00Z \
  --trade-from=2026-01-02T00:00:00Z \
  --trade-to=2026-10-01T00:00:00Z --spread=0
```

All flags are required once. Input is six-column BTB1 OHLCV with exact byte
length; volume cannot be silently defaulted. Output includes full DSL, config
and data hashes, the closed config, indicator/H4 mappings, signal opportunities,
orders, trades, entry-month summaries and explicit terminal exposure. No output
is written until validation and JSON encoding succeed. Data ownership and source
hash lineage must be established separately before any historical replay.

## Fixed signals and explicit clocks

M5 inputs are strictly increasing exact UTC opens with finite positive coherent
OHLC and explicit nonnegative finite volume. M30 buckets use first/max/min/last/
sum. No bars are interpolated. Partial observed bars remain in indicators; the
latest 40 observed M30 bars must each contain all six M5 slots for a signal.
Wholly missing bars do not reset this inherited readiness condition. This is
not a complete-session or native-broker-calendar certificate.

HMA9 crosses HMA21, using floor half lengths and round-half-up square-root
lengths. HMA25 is diagnostic only. Wilder ATR14 and Supertrend(3,10) share the
qualified v9 arithmetic. Require chart Supertrend alignment, volume strictly
above current-inclusive SMA20, ATR14/close percent at least 0.08 and the mapped
H4 direction. H4 uses the same Supertrend recurrence and UTC phase0 buckets
00/04/08/...; availability is nominal start+4h. Short H4 blocks are retained;
no additional complete-session gate is invented. Before H4 ATR initialization,
the Python reference's default bearish direction is preserved even while its
line is null. This differs from the supplied Pine control's explicit readiness
check and is one reason no Go/Pine equivalence is claimed.

SOURCE queries latest H4 close <= M30 decision close. Same-time H4 completion
precedes that source decision deterministically. PROTECTED queries latest H4
close <= M30 bar open, exposing the new boundary snapshot one decision later.
Both mappings and availability timestamps remain visible in every indicator row.
There is no developing-H4 or phase-selection mode.

## Management bundles

SOURCE anchors at signal close, seeds directional extremes with the signal
bar and leaves its first position bar unprotected until that bar completes.
Current ATR controls the 4ATR target, 2ATR stop component and 2ATR activation.
Activation is recomputed every surviving close, not latched. Before activation,
long stop=max(anchor-2ATR, ST) and short stop=min(anchor+2ATR, ST); after it,
use bare ST. This can widen risk. ATR growth can deactivate the flag and a
signal extreme can activate it before the position experiences that price.
Wrong-side stop activation schedules a next-observed-open market exit. No
explicit regime-flip exit exists. A newly marketable target remains a regular,
non-latched limit on the following open/path under the received simulator.

PROTECTED is the already-declared AUDIT_SAFE policy. Freeze signal ATR;
initial risk is min(2ATR, positive directional signal-close-to-ST distance),
otherwise 2ATR. Attach the bracket to entry and translate its stop plus fixed
4ATR target from actual fill. Track post-fill extrema only and latch activation
at fill +/-2 signal ATR. Until then the initial stop is unchanged. Afterward,
ratchet only to same-direction ST strictly on the appropriate side of reference
close; never loosen. An adverse completed chart-regime transition requests a
next-observed-open market close while keeping protection represented as live.
Its reason is `regime_flip_market_exit`, not Python's `wrong_side_stop` label.
This is a different policy from SOURCE and does not ensure a positive-net stop.

## Execution, price precision and sizing

Every native path is an aggregated M30 OHLC path, never an M5 traversal. M5
rows establish aggregation and the actual first-observed open timestamp only.
Chronology: pending entry, pending forced exit, live opening bracket gaps,
then OHLC path, completed-bar signal and surviving-position management. Pending
forced exits take priority at the open. High comes first only if strictly nearer
the open than low; ties go low first. Gap fills use actual open and targets
can improve. Path barrier fills are interval-censored to the native close.

Prices remain continuous float64 reference values. The uncompiled Pine control
uses outward mintick rounding, which is intentionally not implemented here.
Long entry is reference open+spread; short liquidation OHLC is reference+spread.
Short entry and long liquidation use reference prices. These are bid-style
scenario assumptions, not verified feed quote-side provenance.

Equity starts at USD10000, notional is 10% of current equity, fee is USD0.50 per
unit per side, and spread is exactly 0 or 1 USD/unit. Quantity is exactly
`floor((equity*0.1/actualEntry + 1e-12)/0.1)*0.1`.
One unit=one ounce is illustrative, not verified broker metadata. Ten percent
notional is not ten percent risk. A nonpositive rounded quantity terminates the
run with `*QuantityBelowStepError` before fee debit or position creation,
rather than following Python into a zero-quantity fake position. No financing,
slippage, margin, latency, liquidity or minimum-stop certificate is provided.

The qualified v9 causality correction is retained: future bar incompleteness
cannot cancel a pending entry. Fill at the next real observed open and flag
partial traversal. This differs from the Python cancellation rule and must be
reconciled explicitly in any authorized historical comparison. Warmup starts
at an observed M5 open. The explicit end is a completed-history watermark with
an observed M5 close exactly there. Start flat; no warmup entry carries across
the start. End-close management is allowed but end-close new entries are not.
Keep final exposure and pending state; never fabricate terminal liquidation.

## Diagnostics and qualification

Executable comparisons are exact. The received 1e-9 diagnostic threshold is
used only for actual stop widening, target change and hypothetical lock-switch
relaxation. Historical-comparator tolerances are a third, separate concept.
SOURCE hypothetical relaxation compares bare ST against the unlocked candidate
at identical current inputs; actual widening compares against prior live stop.
PROTECTED rejected worse eligible ST candidates are separately reported and
never labelled executed widening or SOURCE lock relaxation. Its pre-entry-only
activation is always false. Compare normalized meanings, not misleading Python
labels. Open-trade edits are retained separately from closed-trade counters.

Closed net, rounded-quantity PF, equal-unit PF, closed-equity drawdown, terminal
cash, open entry fee, mark and hypothetical closing fee are separate. Monthly
summaries use entry month. Closed-equity drawdown is not intratrade drawdown;
scenarios cannot be summed or treated as paired causal effects.

Tests use invented data only and include mode-specific H4 boundaries, indicator
initialization, lifecycle/lock failures, gap and both-touch order chronology,
continuous-price preservation, quantity steps, watermark/future independence,
strict configuration admission and generic/WASM refusal. Two new parse fixtures
are separate from the unchanged 116 prior parse and 100 run goldens. Historical
execution, if separately released after independent synthetic review, is limited
to the four fixed mode/spread cases on already inspected owned history. Stop on
unexplained divergence. This source change claims no fresh validation result.
