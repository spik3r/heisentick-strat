# Setup family: `dual ema resumption` (dualEmaResumption)

Status: specified

This DSL v7 family implements the frozen long-only XAUUSD 4h research rule.
It enters at the next bar open when price closes back above the fast EMA while
the fast EMA is above a rising slow EMA. The initial stop is measured from the
slipped fill. A close-based ATR trail activates on the next bar, and a close
below the slow EMA exits at the next open before that bar's intrabar stop check.

```dsl
dsl v7
strategy "Dual EMA Resumption example" {
  description "Frozen long-only example."
}
market conditions { slices(XAUUSD 4h) }
setup {
  type: dual ema resumption
  ema fast 20
  ema slow 80
  ema slow rise 12
  ema wilder atr 20
  stop initial 2.5 ATR
  trail close 3 ATR
  fallback below slow ema
}
filters { side long only }
execution { risk: 200 USD }
```

`fallback below slow ema` is documentary. The slow-EMA fallback exit is part
of the frozen rule and runs whether or not the phrase is authored: the parser
accepts the phrase without writing any config, and omitting it produces an
identical backtest. Write it for readability, but do not read its absence as
disabling the exit — the family has no way to express that.

The family has a deliberately narrow surface. It accepts route slices, the
seven setup phrases above, `side long only`, and risk USD. Generic session,
context, trigger, target, breakeven, partial, cooldown, max-hold, grade, and
source-timeframe directives are rejected because they would change the frozen
rule or imply behavior the runtime does not provide.
