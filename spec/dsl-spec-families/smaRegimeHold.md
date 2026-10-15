# Setup family: `sma regime hold` (`smaRegimeHold`)

Status: specified

This family implements a completed-close long/cash regime. It becomes eligible
when the first full SMA window is available. If that close is above its SMA,
it queues a fixed one-unit long entry for the next bar's open. A close at or
below the SMA queues an exit for the next bar's open. Equality is cash. The
initial eligible close seeds the regime directly; no prior cross is required.

```dsl
dsl v7
strategy "SMA Regime Hold example" {
  description "Long above the SMA, otherwise cash."
}
market conditions { slices(SPYCFD 1d) }
setup {
  type: sma regime hold
  sma period 200
}
```

The family has no stop, target, leverage, short entry, financing, dividend,
cash-yield, or monetary contract-size model. Its engine result is a fixed
one-unit price-series backtest. It does not model the ETF total-return and cash
economics needed for a listed-SPY baseline, and it is not a funded portfolio
return. A CFD source remains a distinct product with its own costs and
financing. Forward execution still requires an explicit notional/exposure and
cash-transition contract; this stopless family is a research/backtest rule,
not a ready-to-forward strategy.
