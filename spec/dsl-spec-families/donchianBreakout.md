# Setup family: `donchian breakout` (donchianBreakout)

Status: specified (2026-10-16)

Specified against `dsl/parser_donchian_breakout.go`,
`engine/family_donchian_breakout.go`, the JavaScript reference in the
heisentick app (`engine/dsl/setups/donchianBreakout.js`,
`strat/implementations/browser-runtime/compiler/parseSetups/donchianPhrases.js`,
`strat/implementations/browser-runtime/compiler/finalizeDonchianBreakout.js`),
the `*donchian-breakout*` parse and run conformance cases and the `dcb-*`
semantic cases.

Part of `spec/dsl-spec.md` §9.

## Purpose

Donchian breakout is a causal, single-symbol channel breakout for trend
following. At the close of bar `i` it compares that close with the highest
high and lowest low of the `N` bars before it:

```text
upper = max(high[i - N] .. high[i - 1])
lower = min(low[i - N] .. low[i - 1])
long  when close[i] > upper
short when close[i] < lower
```

Bar `i` is not part of its own channel, so the rails are fixed before the bar
trades. Both comparisons are strict: a close equal to a rail is not a
breakout. An optional trend gate requires the close to be on the trend side of
an EMA of closes.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `type: donchian breakout` | Selects the family. This exact spelling is the only type phrase. | Not selected unless declared. | `setupType = "donchianBreakout"`, initializes `donchianBreakout` |
| `channel N candles` | Positive integer channel length. | Required; no default. | `donchianBreakout.channelBars` |
| `trend ema N` | Positive integer EMA length for the trend gate. Long needs `close > EMA`, short needs `close < EMA`. | Off. | `donchianBreakout.trendEmaLen` |

`channel N candles` is a setup-section phrase. It is unrelated to the
filters-section channel gate (`channel active within N candles`, `channel width
...`). Both lengths must be finite, positive, integral numeric values
representable by the engine's signed `int`. The parser checks the exact
unsuffixed token: `3`, `3.0` and `3e0` are accepted, while `3R`, `3%`,
fractions, zero, negatives, nonfinite values and failed conversions are refused.
For the parser's binary64 value `x`, the upper bound is exclusive:
`x < 2^(strconv.IntSize - 1)`. There is no additional lookback policy cap.

The EMA uses `alpha = 2 / (N + 1)`, is seeded with the first close of the run's
bar series and updates on every bar, including bars spent in a position. Its
value at bar `i` therefore depends on where the series starts; the warm-up
below keeps that effect small but does not remove it.

The signal is unavailable while `i < max(channelBars, trendEmaLen)`. A
missing or non-finite high or low inside the channel fails closed, as does a
missing, nonfinite or non-positive ATR at the signal bar. A nonfinite signal
close also fails closed.

## Entry, stop and target

The family signals at the close. The default execution mode fills at that
close, with the shared slippage rules. The shared `costs.fillOn: "open"` mode
(and its `"nextOpen"` alias) instead fills at the next bar's open. In either
mode the initial stop and target remain anchored to the signal close, not the
slipped or next-open fill. The stop uses the context ATR of the signal bar
(the 14-bar ATR the shared stop phrases use):

```text
long  stop = close[i] - k * ATR[i]
short stop = close[i] + k * ATR[i]
```

`k` comes from `stop N ATR` and defaults to 2. Its exact unsuffixed numeric
token must convert successfully to a finite, strictly positive value. Zero,
negative, nonfinite, overflowing and underflow-to-zero multiples are refused.
No other stop phrase is accepted: `stop beyond last N candle extreme ...`,
`stop size min X max Y` and the other shared stop shapes are compile errors,
because the family would otherwise keep the ATR stop and silently ignore them.

There is no target by default. `target 0R` (including decimal zero spellings
such as `0.0R`) explicitly means the same thing. A finite, positive `target NR`
adds a target at `close[i] ± k * ATR[i] * R` (config key `target.dcbR`). The
spelling is one unsigned decimal component followed by a case-insensitive
`R`: `1R`, `1.R`, `.5R` and `0R` are valid. Signs, exponents, `.R`, repeated
decimal points, NaN/Inf and out-of-range decimals are refused. A mathematically
nonzero decimal that converts to zero is refused rather than disabling the
target; a representable positive subnormal is accepted by the parser.

Before entry, the engine requires a finite stop strictly on the loss side of
the signal close and a finite, positive stop distance. An enabled target must
likewise be finite, strictly on the profit side, and have a finite, positive
distance. Overflow and prices that round back to the signal close fail closed
with no entry. The stop and target calculations retain their existing
left-to-right product evaluation order.

### Direct engine configuration

The direct-config boundary validates `donchianBreakout.channelBars` as a
required positive checked integer. It accepts `int`, `int64`, `float32` and
`float64`; integer values are range-checked without first rounding through
binary64, while floating values must be finite, integral and below the same
exclusive signed-int bound. It does not coerce strings, booleans or nulls.
`trendEmaLen` absent or numeric zero means off; an explicit positive value uses
the same count checks. DSL `trend ema 0` remains invalid.

An absent stop or empty stop map means fixed 2 ATR. A missing `type` means
`atr`, and missing `atrMult` means 2; present values must respectively be
`"atr"` and finite, positive numeric data. The inherited `paddingAtr`,
`minAtr`, `maxAtr` and `extremeCandles` fields may be absent or numeric zero;
nonzero overrides are unsupported. An absent or empty target map, or absent
or numeric-zero `dcbR`, means no target. Present `dcbR` must be finite and
nonnegative. Inherited inert target fields such as `edge`, `fallbackR` and
`minR` do not enable a Donchian target.

An absent or empty `entryMode` map means market; a present type must be
`"market"`. `entryTf` may be absent or `"current"`; `sourceTimeframe` may be
absent or an empty string. Explicit nulls, wrong types and unsupported values
are refused rather than silently replaced by defaults. Both ordinary maps and
`dsl.Config` maps are accepted without mutating the caller's config.

## Shared risk, management, and guard phrases

The type starts with:

| Setting | Family default | Generic default it replaces |
| --- | --- | --- |
| Stop | fixed 2 ATR from the signal close | recent extreme + padding with bounds |
| Target | none | 1R |
| Breakeven | off (`atR 0`) | 0.75R |
| Trail | off; `trail N ATR` without `after` starts at 0R | trigger 1R |
| Cooldown | 0 candles | 3 |
| Max hold | none | none |

The supported target phrase, shared breakeven, partial, trail,
`wait N candles after trade`, `maxHoldCandles`, side, fixed-risk and HTF-guard
phrases override those values
and keep their usual meaning. `wait N candles after trade` measures from the
accepted entry bar, as in `price momentum`.

The ATR trail is close-based and only tightens. With trigger 0R it moves the
stop to `close - N * ATR` (long) on any bar whose close is at or above entry
and only if that is tighter than the current stop.

For a position that remains open, management runs in this order: the shared
pre-handler ATR trail, then the family's shared partial step, breakeven and
max hold. There is no second family trail. No-target positions keep absent
(`null`) target metadata on both partial and final trade records.

`higher timeframe must not oppose entry` has the `price momentum` meaning:
missing HTF context fails closed, an opposing completed direction rejects the
entry, and a flat completed HTF state permits either side.

### Market gate defaults

Entries pass through the shared market gates. Three generic defaults would
drop valid breakouts on a trend route, so the family replaces them unless the
strategy authors that gate itself:

| Gate | Family default | Replaced when the strategy authors |
| --- | --- | --- |
| Trade window | `unrestricted` (every bar of the route) | `sessions(...)`, `windows(...)` or any `trade window ...` |
| Day type | `trending, ranging, choppy` | `day type in (...)` or `regime ...` |
| Movement efficiency cap | 1 (off) | `movement below X`, `maxMovementEr`, `maxEr` or `trendiness` |

The rule is applied once after parsing, so section order does not matter. All
other filters (local hour and weekday, New York hour, session phase, seasonality,
candle and grade filters) apply as authored.

On the XAUUSD 4h research rule these defaults matter: with the generic ones
the rule takes 199 trades instead of 352.

### Not supported

These are compile errors: source-timeframe execution, any authored `enter`
directive (including market, close, next-open, limit, empty or malformed
forms), any target-section phrase other than valid `target NR`, and any stop
phrase other than valid `stop N ATR`. The authored-entry refusal is independent
of the shared fill mode described above. Invalid stop, target and entry
directives remain refused even if later valid directives overwrite their
config values, and whether authored before or after family selection.

## Example

```dsl
dsl v7
strategy "Donchian Breakout Spec Example" {
  description "Long-only 4h channel breakout with an EMA trend gate and an ATR trail."
}

market conditions {
  slices(XAUUSD 4h)
}

setup {
  type: donchian breakout
  channel 55 candles
  trend ema 300
}

filters {
  side: long only
}

risk {
  stop 2 ATR
}

management {
  trail 3 ATR after 0R
}

execution {
  risk 200 USD
}
```

This is the gold 4h research rule from the heisentick app
(`strategies/evidence/gold-donchian-trend-4h-2026-10-16.md`). On XAUUSD 4h
from 2003-05-05 to 2026-09-18 the JS reference gives 352 trades, PF 1.87 raw,
1.84 at 0.06 slippage and 1.80 at 0.12, trade for trade identical to the
hand-written research script. Adding `target 100R` changes no trade.
