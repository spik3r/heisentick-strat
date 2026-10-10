# Derivations

One entry per case. Numbers are for the long variant; the short variant is the exact mirror (price -> 200 - price) unless the case says otherwise. ATR is 10 unless stated, so the buffer is 1.0. Setup: closes 100,100,100,100,101, then 104-i from bar 5, so a buy Setup 9 completes at bar 13 (flip gate met at bar 5).

Costs: one adverse slippage applies to every fill (entry and each exit). Where an entry quotes an exit level and a P&L, those are before the exit offset; the last sentence of the entry gives the exit fill and the P&L after it.

## atr.json

### atr.warmup_boundary

ATR14 is unavailable before bar 13 and exact from bar 13.

Every bar has TR=10, including bar 0 (TR[0]=high-low). Index 12 has only 13 bars: unavailable. Index 13 is the first with 14 TRs: 10.

### atr.true_range_gaps

True range uses the previous close across price gaps.

Opens gap from the prior close by -4..+4, so TR often exceeds high-low via |high-prevClose| or |low-prevClose|. Values come from the SMA of the last 14 TRs; bar 0 uses high-low.

## e1.json

### e1.target_basic

Immediate E1 hits 2R target.

ATR14(13)=10 (TR=10 on bars 0-13). Setup bars 5-13; lowest low 88 (bar 7). S=88-1.0=87.0. Open[14]=91, slip 0.5 -> F=91.5, R=4.5, TP=100.5, size 90/4.5=20. High[16]=101>=100.5: exit 100.5, held 3, +2R, 180. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 100; gross P&L 170, 1.8888888889R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.stop_basic

Immediate E1 stopped out.

Same entry as e1.target_basic. Low[15]=86.5<=87 and open 92>87 so exit at 87.0: -4.5*20=-90, -1R. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 86.5; gross P&L -100, -1.1111111111R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.stop_touch_equal

Low exactly equal to stop counts as a hit.

Low[15]=87.0 == S. Touch is a hit (broker uses l<=sl). Exit 87.0. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 86.5; gross P&L -100, -1.1111111111R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.stop_gap_open_through

Bar opens below carried stop: exit at open.

Open[15]=85<=S=87: exit at the open 85 (worse of stop and open). pnl=(85-91.5)*20=-130, R=-6.5/4.5. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 84.5; gross P&L -140, -1.5555555556R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.stop_target_same_bar

One bar reaches both stop and target: stop first.

Bar 15 high 101>=TP 100.5 and low 86<=S 87. Stop-first, exit 87.0. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 86.5; gross P&L -100, -1.1111111111R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.entry_bar_stop

Stop touched on the entry bar.

Entry bar is held bar 1 and is checked. Low[14]=86.5<=87: exit 87.0 on bar 14, held 1, -90. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 86.5; gross P&L -100, -1.1111111111R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.entry_bar_target_touch

High exactly equal to target on the entry bar.

High[14]=100.5 == TP: hit. Exit 100.5 on bar 14, held 1, +180. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 100; gross P&L 170, 1.8888888889R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.entry_bar_both

Entry bar reaches both stop and target: stop first.

Bar 14 high 101 and low 86: stop first, exit 87.0. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 86.5; gross P&L -100, -1.1111111111R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.time_exit_open_e4

Time exit at the open of e+4.

e=14. Bars 14-17 are held bars 1-4 with no hit. Bar 18 open 96 exits (time_exit), held 4: (96-91.5)*20=90, R=1.0. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 95.5; gross P&L 80, 0.8888888889R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.time_exit_before_stop

Time exit precedes the stop check of bar e+4.

Bar 18 opens 95 then trades to 80 (would hit the stop). Time exit first at open 95: 70. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 94.5; gross P&L 60, 0.6666666667R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.time_exit_before_target

Time exit precedes the target check of bar e+4.

Bar 18 opens 97, high 105>=100.5. Time exit at 97, no target credit: 110. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 96.5; gross P&L 100, 1.1111111111R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.time_exit_gap_below_stop

Time exit wins when bar e+4 gaps through the stop.

Bar 18 opens 85<=S. Reason is time_exit (not stop); price is the open 85: -130. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 84.5; gross P&L -140, -1.5555555556R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.stop_last_checked_bar

Stop on bar e+3 is a stop, not a time exit.

Bar 17 low 86<=87: stop at 87.0, held 4. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 86.5; gross P&L -100, -1.1111111111R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.target_last_checked_bar

Target on bar e+3.

Bar 17 high 101>=100.5: target 100.5, held 4. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 100; gross P&L 170, 1.8888888889R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.slip_zero

No slippage: target from the unslipped fill.

slip 0: F=91, R=4, TP=99, size 22.5. High[16]=101: exit 99, +180. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0 gives 99; gross P&L 180, 2R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.slip_large

Slippage 1.5: target measured from the slipped fill.

F=92.5, R=5.5, TP=103.5, risk 110 -> size 20. Pre-slippage R (4.0) would give TP 99.0 or 100.0. High[16]=104: exit 103.5, +220. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 1.5 gives 102; gross P&L 190, 1.7272727273R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.gap_up_entry

Entry opens far above the close (price gap).

Open[14]=99 slip 0.5: F=99.5, R=12.5, TP=124.5, risk 125 -> size 10. No hit through bar 17; time exit at open[18]=103: 35. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 102.5; gross P&L 30, 0.24R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.wrong_side_gap_through

Open gaps below the stop: entry rejected.

Open[14]=80, F=80.5 < S=87 (long: S>=F is wrong side). Rejected, no trade.

### e1.wrong_side_equal

Fill exactly on the stop (zero distance): rejected.

Open 86.5 + slip 0.5 = 87.0 == S. Zero risk distance, rejected.

### e1.wrong_side_zero_slip

Open exactly on the stop, no slippage: rejected.

Open 87.0 == S, slip 0: F==S, rejected.

### e1.tiny_distance_entry_bar_target

Smallest accepted distance, target on entry bar.

Open 87.25 slip 0.5: F=87.75, R=0.75, TP=89.25, risk 75 -> size 100. Low[14]=87.25>87, high 89.5>=89.25: target on bar 14, +150. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 88.75; gross P&L 100, 1.3333333333R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.no_next_bar

Decision on the final bar has no next bar.

Series ends at bar 13 (the Setup-9 bar). Decision recorded, no order: no_next_bar.

### e1.delayed_age1

Delayed perfection at age 1; ATR frozen at the decision bar.

Setup 9 at 13 is imperfect (min(l8,l9)=91 !< min(l6,l7)=90). Bar 14 low 86 < 90: delayed perfection, d=14, age 1. ATR14(14)=(13*10+24)/14=11 (not 10). Anchor stays bars 5-13: lowest low 88, so S=88-1.1=86.9 (bar 14 low 86 is outside the anchor). Open[15]=95, F=95.5, R=8.6, TP=112.7, risk 86 -> size 10. Time exit at open[19]=101: 55. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 100.5; gross P&L 50, 0.5813953488R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.delayed_age4_fill_outside_window

Delayed perfection at age 4 is eligible and fills after the window.

Bars 14-16 have lows 90, 91, 91 (90 is equal to the threshold, not below). Bar 17 low 89.5<90: d=17, age 4, eligible. Fill at open[18] (age 5, outside the window). ATR14(17)=(100+10+10+10+24)/14=11, S=88-1.1=86.9. Open[18]=100, F=100.5, R=13.6, TP=127.7, risk 136 -> size 10. Time exit at open[22]=105: 45. Bar 20 closes at 97, equal to close[16], so the rising closes of bars 14-19 cannot grow into an incidental sell Setup 9. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 104.5; gross P&L 40, 0.2941176471R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.delayed_age5_expired

Delayed perfection at age 5 is expired: no order.

Bars 14-17 have lows >= 90. Bar 18 low 89.5<90: delayed perfection fires once at age 5. E1 treats it as signal_expired: no order, no trade. Bar 19 low 89 does not create a second opportunity (one per episode).

### e1.imperfect_no_decision

Imperfect Setup 9 and no later qualifying low: no decision.

Same imperfect Setup as the delayed cases. Bars 14-19 keep lows >= 90 (threshold is strict), so no perfection event occurs and E1 records nothing.

### e1.equality_not_perfected

Equal lows do not perfect the Setup: no immediate decision.

Bar 11 low 91 and bar 13 low 91: min(l8,l9)=91 is not < min(l6,l7)=91. Strict inequality fails, so no immediate E1. Bars 14-15 keep lows >= 91 so no delayed event.

### e1.anchor_excludes_pre_setup_bar

A deep low one bar before Setup bar 1 is not in the E1 anchor.

Bar 4 (the bar before Setup bar 1) has low 81 and TR 20; bar 0 is flat (TR 0) so ATR14(13)=(0+20+12*10)/14=10. Anchor bars 5-13: lowest low 88, S=87.0. A start at bar 4 would give 80.0. Entry and target as e1.target_basic: +180. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 100; gross P&L 170, 1.8888888889R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.anchor_includes_setup_bar1

The lowest low is on Setup bar 1.

Bar 5 (Setup bar 1) has low 81 and TR 20; bar 0 is flat so ATR14(13)=10. Anchor bars 5-13 includes it: S=81-1.0=80.0. F=91.5, R=11.5, TP=114.5, risk 115 -> size 10. No hit; time exit at open[18]=96: 45. A start at bar 6 would give 87.0. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 95.5; gross P&L 40, 0.347826087R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.anchor_includes_setup9_bar

The lowest low is on the Setup-9 bar.

No early wick: lows fall 98 to 90 across bars 5-13, so the extreme is the Setup-9 bar low 90. S=90-1.0=89.0. F=91.5, R=2.5, TP=96.5, risk 50 -> size 20. High[15]=97: target, +100. An anchor ending at bar 12 would give 90.0. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 96; gross P&L 90, 1.8R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e1.exit_precedes_same_bar_decision

A decision on the bar whose open closed the previous position is not blocked.

First episode: delayed age 4 (d=17), fill at open[18]=100 (F=100.5), time exit at open[22]=105. Closes 95..105 on bars 14-22 are a sell run: sell Setup 9 completes at bar 22 and is perfected (max(h21,h22)=107 > max(h19,h20)=106). The first position left at the open of bar 22, so at that close the book is flat and the second opportunity is accepted. Second: sell side, anchor bars 14-22 (highest high 113.5, bar 17), ATR14(22) from bars 9-22, entry at open[23]=105 with slip 0.5 (F=104.5), time exit at open[27]. The long variant therefore also holds a short trade; the short variant a long trade. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 104.5; gross P&L 29.4117647059, 0.2941176471R. Exit levels and P&L quoted earlier in this entry are before the exit offset. Exit fill, trade 2 (short in the long variant): the exit level moved against the trade by slippage 0.5 gives 107.5; gross P&L -30.3907380608, -0.3039073806R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

## e2.json

### e2.target_basic

E2 completes at bar 27 and hits the target.

Setup 9 at bar 13 (closes 99..91, flip gate met at bar 5). Countdown counts bars 13-20 (counts 1-8; bar 8 close 84), the bounce at 21 and the up-close at 22 do not count, then 23-26 give counts 9-12 and bar 27 (close 78<=low[25]=79, low 77<=close8 84) completes: d=27. ATR14(27)=10. Anchor bars 5-27, lowest low 77 (bar 27): S=76.0. Open[28]=80, slip .5: F=80.5, R=4.5, TP=89.5, size 20. High[30]=90: target, held 3, +180. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 89; gross P&L 170, 1.8888888889R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e2.anchor_setup_wick

Lowest low is in an early Setup bar.

Bar 7 low is 60 (wick). The anchor starts at the admitting Setup bar 1 (bar 5), so 60 is inside: S=60-1.0=59.0. F=80.5, R=21.5, TP=123.5, risk 215 -> size 10. No hit; time exit at open[40]=83: 25. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 82.5; gross P&L 20, 0.0930232558R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e2.anchor_countdown_wick

Lowest low is on a Countdown bar before the completion bar.

Bar 26 (count 12) low 56 with TR 24, so ATR14(27)=(13*10+24)/14=11. Anchor includes it: S=56-1.1=54.9. F=80.5, R=25.6, TP=131.7, risk 256 -> size 10. Time exit at open[40]=83. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 82.5; gross P&L 20, 0.078125R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e2.anchor_excludes_pre_setup_bar

A deep low before Setup bar 1 is not in the anchor.

Bar 4 (one before Setup bar 1) has low 40. Anchor starts at bar 5, so S stays 76.0. Same trade as e2.target_basic. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 89; gross P&L 170, 1.8888888889R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e2.anchor_excludes_entry_bar

A lower low on the entry bar does not move the stop.

Bar 28 low 74<76 stops the position on its first bar (exit 76.0, held 1, -90) but the stop level is still 76.0, from bars 5-27. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 75.5; gross P&L -100, -1.1111111111R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e2.time_exit_open_e12

Time exit at the open of e+12.

e=28, held bars 28-39 (12 bars), no hit. Open[40]=84 exits: (84-80.5)*20=70. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 83.5; gross P&L 60, 0.6666666667R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e2.time_exit_before_stop

Time exit precedes the stop check of bar e+12.

Bar 40 opens 83 and trades to 70. Time exit at 83: 50. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 82.5; gross P&L 40, 0.4444444444R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e2.time_exit_before_target

Time exit precedes the target check of bar e+12.

Bar 40 opens 85, high 95>=89.5. Time exit at 85, no target credit: 90. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 84.5; gross P&L 80, 0.8888888889R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e2.stop_last_checked_bar

Stop on bar e+11 is a stop.

Bar 39 low 75<=76: stop at 76.0, held 12. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 75.5; gross P&L -100, -1.1111111111R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e2.target_last_checked_bar

Target on bar e+11.

Bar 39 high 90>=89.5: target 89.5, held 12. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 89; gross P&L 170, 1.8888888889R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e2.stop_target_same_bar

Both levels in one bar: stop first.

Bar 30 high 95, low 75: stop first, exit 76.0. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 75.5; gross P&L -100, -1.1111111111R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e2.stop_gap_open_through

Gap open below the carried stop.

Open[30]=74<=76: exit at 74; (74-80.5)*20=-130. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 73.5; gross P&L -140, -1.5555555556R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e2.wrong_side_gap_through

Open gaps below the stop: rejected.

Open[28]=70, F=70.5 < S=76: rejected.

### e2.slip_target_from_fill

Slippage 1.0: 2R from the slipped fill.

F=81.0, R=5.0, TP=91.0, risk 100 -> size 20. High[30]=91.5: target 91.0, +200. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 1 gives 90; gross P&L 180, 1.8R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e2.no_next_bar

Completion on the final bar: no_next_bar.

Series ends at bar 27 (completion). Decision recorded, no order.

### e2.first_eligible_completion

Earliest possible E2 decision: bar 25.

Pure slide: counts on every bar from 13, so count 13 at bar 25 (low 78<=close8 84). ATR14(25)=10 over bars 12-25. Anchor 5-25, lowest low 78: S=77.0. Open[26]=81, F=81.5, R=4.5, TP=90.5. High[28]=91: target, +180. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 90; gross P&L 170, 1.8888888889R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### e2.deferred_completion

Completion after a 13-vs-8 deferral: decision at the completing bar.

Bars 0-29 are terminal_13_deferral.strict.buy (accepted core fixture): countdown_defer at bar 27 (count stays 12), countdown_complete at bar 29. So d=29, not 27. ATR14(29)=71/14=5.0714285714. Anchor bars 5-29; lowest low 80 (bar 28, a bar after the deferral that neither counts nor defers). S=80-0.50714285714=79.49285714. Open[30]=84, F=84.5, R=5.00714286, TP=94.51428571, risk 50 -> size 9.98573. Bars 30-41 stay inside; time exit at open[42]=85. Continuation from bar 30 is new. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 84.5; gross P&L 0, 0R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

Source bars: plans/sequential-full-fixtures/terminal_13_deferral.json case terminal_13_deferral.strict.buy, bars 0-29, blob 5211cf66c8adf0c9ee9f5aec7527ddaa02f1a4a5.

### e2.overlap_admitting_setup_anchor

A later same-side Setup 9 does not move the anchor start.

Bars 0-27 are countdown_overlap.active_preserved.buy: Setup 9 at 13 admits the Countdown; a second same-side Setup 9 at bar 24 replaces the latest Setup reference but not the active Countdown; completion at bar 27. The anchor starts at the ADMITTING Setup bar 1 (bar 5), not bar 16. Bar 8 low is changed from the source value to 70 (bar 8 low is read by no Setup, perfection or Countdown test) so the two readings differ: S=70-0.25714286=69.74285714 (a start at bar 16 gives 79.74). ATR14(27)=18/7. Open[28]=84, F=84.5, R=14.75714286, risk 150 -> size 10.1647. Time exit at open[40]=85. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 84.5; gross P&L 0, 0R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

Source bars: plans/sequential-full-fixtures/countdown_overlap.json case countdown_overlap.active_preserved.buy, bars 0-27, bar 8 low set to 70, blob 50bf7fbb52287de96b559d549be9e7ac90462858.

### e2.setup22_collision_completion

Completion and Setup-22 recycle on one bar: the decision still stands.

Bars 0-26 are terminal_13_setup_22_collision.main.buy: countdown_complete then countdown_recycle on bar 26 (core order: complete first). The E2 decision is made on bar 26. ATR14(26)=16/7=2.2857142857. Anchor 5-26, lowest low 77: S=77-0.22857143=76.77142857. Open[27]=80, F=80.5, R=3.72857143, risk 40 -> size 10.727. Time exit at open[39]=82. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 81.5; gross P&L 10.7279693487, 0.2681992337R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

Source bars: plans/sequential-full-fixtures/terminal_13_setup_22_collision.json case terminal_13_setup_22_collision.main.buy, bars 0-26, blob 190d039a5f592d6c8d38dafd022e91696168e9f9.

### e2.anchor_includes_setup_bar1

The lowest low is on the admitting Setup bar 1.

Bar 5 low is 50 (TR 51, outside the ATR window bars 14-27). The anchor starts at bar 5, so S=50-1.0=49.0. F=80.5, R=31.5, TP=143.5, risk 315 -> size 10. No hit; time exit at open[40]=83: 25. A start at bar 6 would give 76.0. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 82.5; gross P&L 20, 0.0634920635R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

## refusals.json

### e1.refuse_open_at_end_entry_bar

Dataset ends on the entry bar with the position open.

Entry fills at open[14]; bar 14 has no stop or target hit and the series ends. No end-of-test liquidation: typed unsupported-incomplete-terminal-run.

### e1.refuse_open_at_end_last_checked_bar

Dataset ends on bar e+3 with no exit: refused, not time-exited.

Bars 14-17 are held bars 1-4 with no hit. The time exit would be at open[18], which does not exist. The dataset ends with an open position: refusal. (With a bar 18 the same series time-exits, see e1.time_exit_open_e4.)

### e1.refuse_open_at_end_gap_entry

Gap entry then dataset end: refused.

Open[14]=99 gap entry fills, nothing hits, series ends at bar 16 with the position open.

### e1.refuse_missing_interval_before_decision

Missing interval inside the Setup: refused.

open_ms skips one 5m step before bar 10 (bars 0-9 contiguous, bar 10 opens two steps after bar 9). The run is refused as unsupported input; no data_gap reset and no calendar handling.

### e1.refuse_missing_interval_during_position

Missing interval after entry: refused.

Hole before bar 16 (after the fill). The run is refused although a stop or target follows; no exit is inferred across the hole.

### e1.refuse_missing_interval_after_exit

Missing interval after the trade closed: still refused.

The target exits at bar 16, then a hole precedes bar 17. Refusal is dataset-wide: a hole anywhere refuses the run.

### e2.refuse_open_at_end_last_checked_bar

Dataset ends on bar e+11 with no exit: refused.

e=28; bars 28-39 are held bars 1-12 with no hit; bar 40 (the time exit) does not exist. Refusal, no liquidation.

### e2.refuse_open_at_end_entry_bar

Dataset ends on the entry bar with the position open.

Bar 28 fills and nothing hits; the series ends.

### e2.refuse_missing_interval_in_countdown

Missing interval inside the Countdown: refused.

A hole before bar 18 refuses the run. No reset, no completion across the hole.

## provisional.json

### pv.cap_binds_reduces_size (provisional, OQ-1)

Notional cap binds: size reduced to cap/fill.

As e1.target_basic but max_notional 1500. Risk size 20 gives notional 20*91.5=1830>1500, so size=1500/91.5=16.3934426 and cap_binds=true. Short mirror: F=108.5, size 1500/108.5. Assumes reduce-not-reject and notional = size*fill price (OQ-1). Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 100; gross P&L 139.3442622951, 1.8888888889R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### pv.cap_equal_not_binding (provisional, OQ-1)

Notional exactly equal to the cap does not bind.

Open[14]=99.5, slip 0.5: F=100 (also 100 after mirroring). S=87, R=13, risk 130 -> size 10, notional 1000 == cap 1000: cap_binds=false, size unchanged. Time exit at open[18]=103: 30. Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 102.5; gross P&L 25, 0.1923076923R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### pv.target_gap_open_beyond (provisional, OQ-2)

Bar opens beyond the target: fill at the target level.

Open[15]=103 > TP 100.5. Recommended and existing-broker reading: the target level 100.5 is the fill level (no gap credit), less slippage 0.5 = 100.0. The alternative credits the open (103 less 0.5 = 102.5). Exit fill, trade 1 (long in the long variant): the exit level moved against the trade by slippage 0.5 gives 100; gross P&L 170, 1.8888888889R. Exit levels and P&L quoted earlier in this entry are before the exit offset.

### pv.entry_straddle_open_below_stop (provisional, OQ-4)

Open below the stop but slipped fill above it.

Open[14]=86.75 < S=87, slip 0.5 -> F=87.25 > S. The contract rule tests the fill (S>=F), which would accept; the open already gapped through the stop. Recommendation: reject (test both open and fill). Expectation: stop_breached_at_entry.

