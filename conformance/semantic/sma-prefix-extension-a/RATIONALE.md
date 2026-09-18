# sma-prefix-extension-a

Semantic under test (with `sma-prefix-extension-b`): appending future bars
never changes an earlier decision. This is the base series; `-b` is a strict
prefix extension of it and the validator checks that `-b`'s bars start with
these bars and that `-b`'s first trade equals this trade.

## Spec text

`strat/docs/dsl-spec.md` §10 "Evaluation order and causality":

> No strategy-visible value may depend on the current bar's future or on
> later bars

`strat/docs/dsl-spec-families/smaGoldenCross.md`: SMAs of completed closes;
entry and exit at the next bar open (see `sma-cross-next-open-entry-exit`).

## Bars and derivation

Identical to `sma-cross-next-open-entry-exit` (bars 0-10). Golden cross at
the close of bar 5 → entry bar 6 open 2020; bearish cross at the close of
bar 9 → exit bar 10 open 2020; reason `rule`.

## Spec gaps and assumptions

None beyond those listed for `sma-cross-next-open-entry-exit`.
