# Sequential E1/E2 execution acceptance corpus (`seq-exec-e1e2.v1`)

Synthetic, valid, finite OHLC fixtures with expected decision, order, fill, stop,
target, sizing and exit records for the owner-approved E1/E2 execution rules.
Claimed in HT-249. Test data only: no Go, no engine, no historical data, no outcome.

All prices, risk amounts and notional caps are invented. They relate to no study
percentage, leverage cap or market. Symbol `SYN`, 5m bars, bar `i` opens at
`1699999800000 + i * 300000` ms unless `series.open_ms` is given.

## Authority and independence

Expectations come from the written contract, not from any implementation:

- `plans/sequential-semantics-contract.md` section 7 (D4) and the 2026-10-10
  acceptance and capability boundary in backlog PR 464 (head `12c7d80`, checked 2026-10-10; the D4 text is unchanged from `4316ef9`).
- `plans/sequential-core-freeze-v1.md` (`seq-core-freeze.v1`) for Setup,
  perfection, Countdown and window rules.
- The owner's conventions in the HT-249 request, listed below.

The author did not run or read Astra's Go code or ML PR 121. Three lifecycle
series (deferral, overlap, Setup-22 collision) reuse bars 0 to N of accepted core
fixtures (`plans/sequential-full-fixtures/`, blobs recorded in each case's
`source_fixture`); the lifecycle events there are already reviewed, and the
execution records are new. Author values came from hand-designed bars, hand-derived
checks (each case has a `derivation` and the check is asserted at build time)
and an author calculator kept outside the repository. `tools/harness.mjs` is a
second implementation written from this README and the freeze, not from the
author's calculator. The JSON files are the authority.

## Conventions under test

1. **ATR.** Simple average of the last 14 true ranges (`TR[0] = high - low`,
   later `max(h-l, |h-prevClose|, |l-prevClose|)`), as `contextcols.ComputeATR`.
   Frozen at the decision bar `d`. Needs 14 bars, so `d >= 13`.
2. **Decision.** E1: first `perfection` or `delayed_perfection` per Setup
   episode. A delayed one is eligible at age `d - s9` of 1 to 4; age 5 or more is
   `signal_expired` (no order, and it does not recur). E2: `countdown_complete`
   (age 0).
3. **Stop.** Long: lowest low over the anchor minus `0.10 * ATR14(d)`. Short:
   highest high plus the buffer. E1 anchor: Setup bars 1 to 9 (`s9-8 .. s9`).
   E2 anchor: the admitting Setup's bar 1 through the Countdown-13 bar
   (`adm_s9-8 .. d`), even if a later same-side Setup replaced the Setup
   reference. Bars before the anchor and bars after it (including the entry bar)
   do not count.
4. **Entry.** Next open, bar `e = d + 1`. Fill `F = open[e] + slippage` (long) or
   `- slippage` (short). Reject `stop_breached_at_entry` when the stop is on or beyond
   the fill (long `S >= F`, short `S <= F`). A last-eligible decision still fills
   at `d + 1`. A decision on the final bar gets no order (`no_next_bar`).
5. **Target and size.** `R = |F - S|`, target `F +/- 2R` from the slipped fill.
   `size = risk_amount / R`, then the notional cap: if `size * F > max_notional`,
   `size = max_notional / F` and `cap_binds` is true (OQ-1).
6. **Held bars and exits.** The entry bar is held bar 1. Bars `e .. e+N-1` are
   checked (`N` = 4 for E1, 12 for E2): stop touched when `low <= S` (long) /
   `high >= S` (short); target touched when `high >= TP` / `low <= TP`; touching
   counts. If both on one bar, stop first. A stop exit level is `S` on the entry bar
   and at `min(open, S)` (long) / `max(open, S)` (short) on later bars (gap rule).
   A target exit level is `TP`. At the open of bar `e+N` the position time-exits at
   that open, before that bar's stop and target checks.
7. **Costs.** One explicit slippage offset `slippage` (price units, `config.slippage`) is
   applied adversely to every fill: entry, stop exit, target exit and time exit, as the
   retained broker does (contract section 3.5: exits also slip; PR 464 keeps the broker
   and its explicit costs). Triggers use the raw levels: a target is reached when
   `high >= TP`, and it fills at `TP - slippage` (long) or `TP + slippage` (short); a stop
   fills at `S` or the gap open, then slips. No fees. `gross_pnl = dir * (exit - F) *
   size`; `r_multiple = dir * (exit - F) / R`, so a target exit is below +2R by
   `slippage / R`.
8. **Refusals (PR 464 capability boundary).** A missing timestamp interval refuses
   the run. A dataset that ends with a position still open refuses the run with
   `unsupported-incomplete-terminal-run`; there is no end-of-data liquidation.
   Price gaps between consecutive bars are normal and are tested (gap fills,
   gap stops, gap entries). Only the refusal itself is binding. `kind_binding` and
   `index_binding` are false: the kind string and `at_bar_index` (the first bar after
   the hole, or the last bar of a terminal run) are the author's suggestion. See OQ-5.
9. **One position, one book; exits before signals.** Exits at a bar's open come before
   a decision taken at that bar's close, so the book is flat for it
   (`e1.exit_precedes_same_bar_decision`). `blocked_in_position` and
   `simultaneous_signal` are unreachable: see OQ-6.
10. **Assumptions inherited from the existing Go broker, not written in the contract.**
    A touch counts as a hit (`low <= S`, `high >= TP`); a stop on the entry bar has level
    `S`; a later bar that opens through the stop has the open as its level. The contract keeps
    the broker as the only fill implementation, so the fixtures follow it.
    Cases that rely on these: `e1.stop_touch_equal`, `e1.entry_bar_target_touch`,
    `e1.tiny_distance_entry_bar_target`, `*.entry_bar_*`. The gap-to-the-open stop
    rule is written in the contract (Panel C), and stop-first on the entry bar is in PR 464.
    **Target exits slip too.** Contract section 3.5 says both "exits also slip" and "TP fills
    at exactly tp" (the legacy JS broker). The corpus follows the Go broker's reported
    behaviour (Astra's review of head `2b106e1`: adverse slippage on every exit, target
    included). Every binding case with a target exit and non-zero slippage depends on this
    (`e1.target_basic`, `e1.slip_large`, `e2.target_basic`, and others); the owner should
    confirm it. If the owner prefers unslipped target fills, those expected exits and P&L
    change by `slippage` and `slippage * size`.
11. **Mirror.** Each case has a long variant and a short variant. For `mirror.exact`
    cases the short bars are `price -> 200 - price` with high and low swapped, and
    every price in the expected records mirrors; ATR, size, P&L and R are equal.

## Files

| File | Content |
| --- | --- |
| `cases/atr.json` | ATR14 availability (index 12 unavailable, 13 first) and gap true range |
| `cases/e1.json` | E1 stops, entry, held-bar and exit boundaries, delayed ages 1/4/5, no-decision cases |
| `cases/e2.json` | E2 anchors (Setup, Countdown, pre-Setup, entry bar, overlap), deferral, collision, boundaries |
| `cases/refusals.json` | Missing intervals and open positions at the end of data, both policies |
| `cases/provisional.json` | Cases that depend on an open question; not binding |
| `DERIVATIONS.md` | The derivation of every case in words and numbers |
| `MANIFEST.json` | File hashes and the corpus hash |
| `tools/validate.mjs` | Schema and internal-consistency validator |
| `tools/harness.mjs` | Independent recalculation harness |

Case record: `id, group, policy, status (binding|provisional), open_question,
title, derivation, prefix_group, series, config, mirror, source_fixture, variants
{long, short: {bars[{o,h,l,c}], expected}}`. `expected` is either `{outcome: "ok",
decisions, orders, trades}` or `{outcome: "refused", refusal}`. Compare numbers with a
tolerance of 1e-9 (relative); the buffer `0.10 * ATR` is not exact in binary.

`prefix_group` marks cases that share bars up to `shared_through_index`; their
decisions and fills must be identical. This is the future-bar check: bars after the
decision (and after the fill) must not change an earlier decision. An engine
that exposes its decisions even when it refuses a truncated run (an open position or
no next bar) can also be run on the bars cut at `d` and at `d+1`; the decision record
must not change. An engine that returns nothing for a refused run cannot be checked
this way and relies on the `prefix_group` cases.

## Coverage map

Long and short: every case. Insufficient ATR warm-up: `atr.warmup_boundary` (see
OQ-6). First eligible decision: `e1.target_basic` (bar 13, the earliest bar any
full-profile Setup 9 can complete) and `e2.first_eligible_completion` (bar 25).
Last eligible: `e1.delayed_age4_fill_outside_window`, `e1.delayed_age5_expired`,
`*.no_next_bar`. Next-open fills, gaps, slippage: `e1.gap_up_entry`,
`e1.stop_gap_open_through`, `e1.slip_*`, `e2.slip_target_from_fill`. Wrong-side and
zero distance: `e1.wrong_side_*`, `e2.wrong_side_gap_through`. Target after
slippage: the slip cases. Exposure caps: `pv.cap_*` (provisional). Held-bar
boundaries: `*.time_exit_*`, `*.stop_last_checked_bar`, `*.target_last_checked_bar`,
`*.entry_bar_*`. Anchor edges: `e1.anchor_excludes_pre_setup_bar`, `e1.anchor_includes_setup_bar1`, `e1.anchor_includes_setup9_bar`, `e2.anchor_*`. Stop/target ambiguity: `*.stop_target_same_bar`, `e1.entry_bar_both`.
End of data: `refusals.json`. Future-bar changes: `prefix_group` cases.

## Open questions (reported, not frozen)

Cases that depend on one are `provisional` and carry its ID. Their expected values
follow the recommendation.

- **OQ-1 Exposure cap.** The contract says "the notional cap applies" and nothing more.
  Unresolved: reduce or reject an oversize trade, how notional is measured, and lot
  rounding. If the owner accepts the recommendation, the exact rule is:
  `size_unconstrained = risk_amount / |F - S|`; `notional_unconstrained = size_unconstrained * F`
  with `F` the slipped entry fill; if `notional_unconstrained > max_notional` then
  `size = max_notional / F` and `cap_binds = true`, else `size = size_unconstrained`;
  equality does not bind; size is not rounded; the stop and 2R target are not changed by the
  cap; realised risk then falls below `risk_amount`. The alternative is to reject the entry
  (no trade) when the cap would bind. Only `pv.cap_binds_reduces_size` and
  `pv.cap_equal_not_binding` use the rule; they stay provisional until the owner chooses.
- **OQ-2 Target when a bar opens beyond it.** The existing broker fills at the target
  level (no gap credit); the contract only specifies gaps for stops.
  Recommendation: keep the broker behaviour. `pv.target_gap_open_beyond`.
- **OQ-3 Exit slippage. Closed.** Version 1 of this corpus gave entry-only slippage; Astra's
  review of head `2b106e1` showed the retained broker slips every fill, and contract
  section 3.5 already says so. All binding cases now use one adverse offset on every fill
  (convention 7). Still open inside it: whether a limit-style target exit should slip.
  The retained broker does, so the corpus follows it; a separate target-exit rule would need
  an owner-approved cost API.
- **OQ-4 Open below the stop but slipped fill above it.** The rule tests the fill, so
  `open 86.75, slip 0.5, stop 87` accepts a long that has already gapped through its
  stop and then stops out on its first bar. Recommendation: reject when either the
  open or the fill is on the wrong side. `pv.entry_straddle_open_below_stop`.
- **OQ-5 Refusal shape.** Names (`unsupported-missing-interval` is the author's
  guess; `unsupported-incomplete-terminal-run` is from PR 464), whether the run is
  refused up front or at the offending bar, and whether earlier records are kept.
  Fixtures assert only that the run is refused (`outcome`); kind and bar index are
  suggestions. Recommendation: refuse the whole run and return no trades, so partial
  results cannot be mistaken for a finished test.
- **OQ-6 Rules the lifecycle cannot reach.** Not fixtures, only a finding. With the
  full-profile flip gate the earliest Setup 9 is bar 13, which is also the first bar
  with ATR14, so "insufficient warm-up" cannot occur on a gapless series; the guard
  matters only if a decision is injected, or after a gap once gaps are supported.
  `blocked_in_position` cannot occur: consecutive eligible E1 decisions are at least N+1 = 5
  bars apart (a delayed buy at age 4 and an immediate sell nine bars into a run
  fit exactly), so the 4-bar hold has already ended at the open of the bar where
  the next decision is taken. The independent reviewer derived at least 13 bars for
  E2 against a 12-bar hold; the author built no E2 case for it. The exact-adjacent
  case is `e1.exit_precedes_same_bar_decision`. `simultaneous_signal` cannot occur. For E1 an eligible delayed decision comes at most
  4 bars after its Setup 9, and an opposite Setup 9 needs at least 9 more bars. For E2 at
  most one side holds an unfinished Countdown. A Go unit test that injects decisions is the only way to
  cover the unreachable branches.
- **OQ-7 Broker fee distinction.** PR 464 keeps the broker's `trade.pnl` versus
  realized entry-fee difference. All fixtures use no fees, so `gross_pnl` is not
  affected; a fee case needs the owner's cost inputs.

## How to check

```
node fixtures/sequential-execution-e1-e2.v1/tools/validate.mjs --hash
node fixtures/sequential-execution-e1-e2.v1/tools/harness.mjs
```
