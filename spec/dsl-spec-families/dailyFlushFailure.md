# Setup family: `daily flush failure` (dailyFlushFailure)

Status: specified (2026-08-14)

Part of `strat/docs/dsl-spec.md` §9.

## Purpose

This long-only daily index setup looks for a failed downside extension after an unusually wide bearish flush. It runs on a retained Monday–Friday stream: weekend bars do not affect the first-TR-seeded recursive ATR(20), setup adjacency, fills, stops, hold counts, or mark-to-market. This seed is part of the family; it is not the conventional SMA warm-up variant of Wilder ATR.

## Phrases

| Phrase | Meaning | Default |
| --- | --- | --- |
| `type: daily flush failure` | Select the family and its exact execution model. | Required |
| `flush range at least X ATR` | The prior retained bearish candle must be at least X times its retained-stream ATR. | Required |
| `flush close in bottom X percent` | The flush close must be within the bottom X% of its range. | Required |

The trigger candle must trade below the flush low, close above its open, and close above the flush close. Entry is at the next retained weekday open with adverse slippage. The stop is below the lower of the flush and trigger lows by the authored ATR padding. The actual slipped fill-to-stop distance must satisfy the inclusive shared stop bounds.

The family has no profit target or stop management. `maxHoldCandles N` counts retained weekday bars from the entry bar; after N bars survive the stop, it exits at the next retained weekday open. A stop is active on the entry bar, and a gap through the stop exits at the worse open. An exit blocks a new signal on that retained bar.

## Example

```dsl
dsl v7
strategy "Daily Flush Failure Example" {
  description "Failed downside extension on daily US indices."
}

market conditions {
  slices(NAS100 1d, US500 1d)
}

setup {
  type: daily flush failure
  flush range at least 1.25 ATR
  flush close in bottom 25 percent
}

risk {
  stop beyond last 2 candle extreme by 0.1 ATR
  stop size min 0.5 max 3
}

management {
  maxHoldCandles 10
}

execution {
  risk 200 USD
}
```
