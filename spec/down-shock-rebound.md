# Down-shock rebound research setup

`down shock rebound` implements HT-086's portable rule. It does not run an
Isolation Forest model. All twelve historical ML/rule entry and exit arms
failed the study's screening gate. This setup is for reproducible chart
inspection; it supplies no trading or Forward approval.

```dsl
dsl v7
strategy "Down Shock Rebound Research" {
  description "Completed 15m context with causal 1m execution."
}
market conditions {
  slices(XAUUSD 1m)
}
setup {
  type: down shock rebound
  source timeframe 15m
  entryTf 1m
  shock entry reversal
  shock target off
}
execution {
  risk 200 USD
}
```

## Signal and warm-up

Use weekday source bars only. At the completed 15m close, require log return
from the preceding retained source close divided by prior volatility to be
at most -3. Volatility is the square root of a Parkinson variance EWMA with
span 460, `adjust=False`, requiring 460 prior observations and floored at
1e-6. The current candle does not enter that volatility estimate.

The current logarithmic high/low range must be at least twice the median of
the previous 20 retained observations in the same UTC 15m slot, requiring at
least 10. The comparison adds 1e-6 to both ranges and clips the log ratio to
[-5, 5], matching the Python feature definition. ATR14 is a simple mean of
true range including the completed source candle. Normalizers update
causally. Source history preceding chart coverage provides warm-up but its
old signals are never replayed onto the first chart candle.

## Entry and exits

- `shock entry immediate`: first available 1m open at or after source close.
- `shock entry reversal`: first bullish completed 1m candle closing above the
  preceding minute's high within the next 15 minutes, then the next open.
  Both the preceding and following minute must exist without a gap.
- `shock target off`: no profit target.
- `shock target X ATR`: fixed target X source ATR14 above quoted entry.
- `shock target X bp`: fixed target X basis points above quoted entry.

Targets must be finite and positive. Defaults are immediate entry and target
off. Every trade has a fixed stop 2 source ATR14 below quoted entry and a
240-minute elapsed-time limit. Normal time exits use the minute close; if no
candle ends at the deadline, exit at the first available open after it. Skip
entries delayed over two hours. Holding gaps over two hours exit at the next
available open before checking that candle's bracket. Stop wins a same-bar
stop/target tie. A stop gap fills at the worse open; a target fills at its
level. One position can be open. Signals that arrive while holding are
consumed without scheduling a later entry.

Stops, targets, and risk size use the quoted open before broker costs. Size is
USD risk divided by 2 source ATR14. The shared broker applies configured
slippage and fees; spreads must be supplied through the existing cost model.
The study's fixed $0.24 round-trip assumption is not embedded in the DSL.

Only strategy metadata, market routes, source/entry timeframe, shock clauses,
and USD risk directives are supported. Other authored filters or management
are rejected rather than silently ignored. This family always uses next-open
entry regardless of the general `fillOn` option. It uses a separate source
boundary loop because the generic scheduler waits for a later chart close.

Seven synthetic run fixtures cover both entry rules and all three exit
choices, with a canonical family fixture for coverage auditing. Focused tests
also cover normalizer parity, causal prefixes, entry and holding gaps,
stop/target ties, and route refusal. Existing run goldens do not change.
