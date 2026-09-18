# Setup family: `double top bottom` (doubleTopBottom)

Status: specified (2026-07-03)

Part of `strat/docs/dsl-spec.md` §9; authoring rules and the claim tracker live in
`plans/COMPLETED.md` (spec-families closeout).

## Type phrase

```
setup {
  type: double top bottom
}
```

## Purpose

Double top/bottom trades reversal breaks after two similar pivots form within
a bounded time gap. Double tops use paired swing highs and enter short when
price breaks the neckline; double bottoms use paired swing lows and enter long
when price breaks the neckline.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `pivot window N` | Use `N` candles on each side to confirm swing highs/lows. | `2` | `doubleTopBottom.pivotWindow` |
| `peak gap N to M candles` | Require the second pivot to be at least `N` and at most `M` candles after the first. Missing `to` is an error. | `5 to 72` | `doubleTopBottom.minPeakGap`, `doubleTopBottom.maxPeakGap` |
| `peak difference max X ATR` / `peak diff max X ATR` | Cap the price difference between the two highs or two lows. `under` and `below` are accepted instead of `max`. | `0.5` | `doubleTopBottom.maxPeakDiffAtr` |
| `neckline touch X ATR` / `neckline tolerance X ATR` / `neckline within X ATR` | Require the second pivot confirmation close to clear the pivot-side trigger by `X` ATR before the neckline-break condition is tested. | `0.25` | `doubleTopBottom.necklineTouchAtr` |
| `neckline depth X ATR` | Require pattern height from pivots to neckline to be at least `X` ATR. | `0` | `doubleTopBottom.necklineDepthMinAtr` |
| `breakout buffer X ATR` / `breakout by X ATR` | Offset the neckline break threshold by `X` ATR. | `0.05` | `doubleTopBottom.breakoutBufferAtr` |
| `confirm within N candles` | Require the neckline break within `N` candles after the second pivot. | `12` | `doubleTopBottom.confirmCandles` |
| `choch within N candles lookback M` | Require a change-of-character pivot break near the second pivot before entry. | off (`chochCandles: 0`, lookback `4`) | `doubleTopBottom.chochCandles`, `doubleTopBottom.chochLookbackCandles` |
| `structure break within N candles lookback M` | Alias for the CHOCH requirement. | off (`chochCandles: 0`, lookback `4`) | `doubleTopBottom.chochCandles`, `doubleTopBottom.chochLookbackCandles` |

## Defaults and interactions

`type: double top bottom` rebases shared defaults to stop padding `0.35 ATR`,
min stop `0.4 ATR`, max stop `3 ATR`, target `1R`, breakeven after `0.5R`
plus `0.05 ATR`, and a 12-candle cooldown. Runtime params default to all
sessions, both sides, and HTF bias off unless the shared higher-timeframe
directive sets it to `notAgainst`.

Swing pivots are causal: a pivot is only captured after `pivotWindow` future
candles exist. Short setups use the last two qualifying highs and the lowest
low between them as neckline; long setups use the last two qualifying lows and
the highest high between them as neckline. Stops are placed beyond the pattern
extreme plus stop padding, then checked against shared min/max stop limits.
The shared side, session, target, breakeven, partial, max-hold, cooldown, and
fixed-risk directives feed runtime params. The parser accepts the type alias
`double top/bottom`.

## Example

```dsl
dsl v7
strategy "Double Top Bottom Spec Example" {
  description "Documented double-top and double-bottom reversal example."
}

market conditions {
  slices(XAUUSD 1h)
  sessions(london, ny)
  day type in (ranging, choppy, trending)
  movement below 0.95
}

setup {
  type: double top bottom
  pivot window 2
  peak gap 5 to 72 candles
  peak difference max 0.5 ATR
  neckline touch 0.25 ATR
  neckline depth 0.75 ATR
  breakout buffer 0.05 ATR
  confirm within 12 candles
  choch within 4 candles lookback 4
}

filters {
  higher timeframe must agree
  side both
}

risk {
  stop beyond pattern extreme by 0.35 ATR
  stop size min 0.4 max 3
}

target {
  target 1R
}

management {
  move stop to breakeven after 0.5 R plus 0.05 ATR
  wait 12 candles after trade
}

execution {
  risk 200 USD
}
```
