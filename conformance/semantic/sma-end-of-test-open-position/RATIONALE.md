# sma-end-of-test-open-position

Semantic under test: a position still open on the last bar is reported once,
with its own exit reason (`end-of-test`), never as `tp`, `sl` or a rule exit.

## Spec text

`strat/docs/dsl-spec-families/smaGoldenCross.md`: entry at the next bar open
after a golden cross; exit only on a bearish cross (see
`sma-cross-next-open-entry-exit`).

`strat/docs/dsl-spec.md` has no end-of-data rule. The migration plan
(`backlog/plans/2026-09-18-one-engine-migration.md`, "Correctness", layer A)
requires "end-of-test liquidation (reported as a distinct exit reason, never
a normal exit)".

## Strategy

`sma fast 2`, `sma slow 3`, `side long only`.

## Bars

Bars 0-8 of `sma-cross-next-open-entry-exit` (same table, cut after bar 8):
golden cross at the close of bar 5, entry at bar 6 open 2020; SMA2 stays above
SMA3 through bar 8 (2035 vs 2033.33), so no bearish cross occurs.

| k | open | high | low | close |
| --- | --- | --- | --- | --- |
| 6 | 2020 | 2030 | 2020 | 2030 |
| 7 | 2030 | 2040 | 2030 | 2040 |
| 8 | 2040 | 2040 | 2030 | 2030 |

## Derivation

1. Entry: bar 6 at 2020 (as in `sma-cross-next-open-entry-exit`).
2. No bearish cross by bar 8; the data ends with the position open.
3. The report must still account for the position: one trade, exit on the
   last bar (index 8), reason `end-of-test`.
4. The family has no stop or target, so `tp`/`sl` cannot be the reason under
   any reading.

Expected: entryIndex 6 / 2020, exitIndex 8, exitT = bars[8].t, exit 2030,
reason `end-of-test`.

## Spec gaps and assumptions

- SPEC GAP: no liquidation price is specified. The fixture takes the last bar
  close (2030) as the most conservative observable price; a reviewer may
  choose to compare `exit` loosely for this case until the spec decides.
- SPEC GAP: the exit-reason name. This set uses `end-of-test`; `eod` is
  reserved for an end-of-day flatten so the two are never conflated.
