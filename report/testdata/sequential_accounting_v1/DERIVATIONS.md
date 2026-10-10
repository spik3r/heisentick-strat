# Rational expectation projection

The case JSON retains every authored bar, cost, risk, profile, decision input,
mirror constant, trade, mark, headline, drawdown point and binary64 diagnostic
under the unchanged economic fields listed in IMPORT.json. Decimal/rational
strings are not rounded or recalculated by this projection.

The full bars and expected records are self-contained. A prefix field links
shared Full-case prefixes to the already public execution corpus; its numeric
through-index and bar hash are preserved. Free-form authoring/history narratives
are excluded from this public packet. Their removal does not promote a case or
change an expectation.

Status inventory:

- leg.target_fees_open_marks: binding
- leg.stop_fees_loser: binding
- leg.stop_gap_open_fees: binding
- leg.liq_gross_win_net_loss: binding
- leg.liq_exact_flat: binding
- leg.signal_on_last_bar: binding
- leg.liq_zero_costs_flat: binding
- leg.no_trades_default_equity: binding
- leg.dd_split_four_trades: binding
- leg.winner_then_flat: binding
- leg.equity_below_zero_transient: binding
- leg.equity_negative_end: binding
- leg.default_start_equity_trade: binding
- full.e1_target_fees: binding
- full.e1_stop_fees: binding
- full.e1_stop_gap_fees: binding
- full.e1_time_exit_fees: binding
- full.e1_gross_win_net_loss: binding
- full.e1_exact_flat: binding
- full.e2_time_exit_fees: binding
- full.e2_target_fees: binding
- full.e2_stop_equity_negative_end: binding
- full.e1_decision_no_trade: binding
- full.e1_no_next_bar: binding
- ref.e1_open_at_end_entry_bar: binding
- ref.e1_open_at_end_last_checked_bar: binding
- ref.e2_open_at_end_last_checked_bar: binding
- ref.nonpositive_start_equity: binding
- ref.overflow_entry_fee: binding
- leg.same_bar_exit_reentry: provisional
