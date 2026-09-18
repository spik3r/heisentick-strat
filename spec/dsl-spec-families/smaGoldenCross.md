# Setup family: `sma golden cross` (smaGoldenCross)

Status: specified

This family translates a Pine-style long-only SMA crossover rule. It computes
simple moving averages of completed bar closes, enters long at the next bar
open when the fast SMA crosses above the slow SMA, and queues a long close at
the next bar open when the fast SMA crosses below the slow SMA.

```dsl
dsl v7
strategy "SMA Golden Cross example" {
  description "Long-only 50/200 SMA crossover."
}
market conditions { slices(XAUUSD 1d) }
setup {
  type: sma golden cross
  sma fast 50
  sma slow 200
}
filters { side long only }
```

The exact source translation has no stop, target, trailing, session, or other
risk-management semantics. Backtest sizing is a fixed one-unit entry. The
engine's closed-bar runner can evaluate this exact rule, but the deployed
Forward service rejects its stopless/targetless opened position as
`incomplete-trade-plan`.

An explicit protected execution-test variant may author all three of these
setup directives together:

```dsl
setup {
  type: sma golden cross
  sma fast 50
  sma slow 200
  sma atr 14
  sma stop 2 ATR
  sma target 3 ATR
}
```

`sma atr N` selects a positive integer ATR length; `sma stop X ATR` and
`sma target Y ATR` select finite positive ATR multiples. Partial or malformed
protected bundles are rejected. The protected execution variant uses the
parent runtime's Wilder ATR14 policy, seeded from the initial 14 true ranges,
with the same next-open order anchor. It is a separate execution-test rule,
not a change to the exact stopless Pine translation.

Pine-only visuals such as `plot`, `plotshape`, and `barcolor` are not part of
the strategy runtime. The fast SMA's default-off plot remains absent, the
slow SMA's default-on plot is not reproduced, and bar colors are not
reproduced.
