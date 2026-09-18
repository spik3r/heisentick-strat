# Setup family: `supply demand` (supplyDemand)

Status: specified (2026-07-03)

Part of `strat/docs/dsl-spec.md` §9; authoring rules and the claim tracker live in
`plans/COMPLETED.md` (spec-families closeout).

## Type phrase

```
setup {
  type: supply demand
}
```

## Purpose

Supply-demand finds a compact base, waits for an impulsive departure that
creates a supply or demand zone, then trades the later retest of that zone.
Demand zones produce long setups; supply zones produce short setups. Optional
first-touch, rejection, reaction, CHOCH, and broken-zone flip phrases tighten
how price must return to the zone before entry.

## Phrases

| Phrase shape | Meaning | Default | Config keys |
| --- | --- | --- | --- |
| `patterns(<list>)` / `pattern <list>` | Restrict accepted base-departure patterns. Values are `DBR`, `RBR`, `RBD`, `DBD`; long-form aliases `drop-base-rally`, `rally-base-rally`, `rally-base-drop`, and `drop-base-drop` normalize to those ids. | `DBR, RBR, RBD, DBD` | `supplyDemand.patterns` |
| `base N to M candles` | Require the base to span from `N` to `M` candles. Missing `to` is an error. | `2 to 5` | `supplyDemand.minBaseCandles`, `supplyDemand.maxBaseCandles` |
| `base body max X ATR` | Cap the largest body in the base by ATR. `under` and `below` are accepted instead of `max`. | no cap (`Infinity`) | `supplyDemand.maxBaseBodyAtr` |
| `consolidation tight max X ATR` / `consolidation body max X ATR` | Same body-size cap as `base body max`. | no cap (`Infinity`) | `supplyDemand.maxBaseBodyAtr` |
| `consolidation width max X ATR` | Cap full zone width by ATR. | `1.2` | `supplyDemand.maxZoneAtr` |
| `source leg at least X ATR within N candles` / `source at least X ATR within N candles` | Require the incoming leg before the base to have moved at least `X` ATR over `N` candles. | `0 ATR within 4 candles` | `supplyDemand.minSourceAtr`, `supplyDemand.sourceCandles` |
| `impulse at least X ATR within N candles` | Require the departure move after the base to travel at least `X` ATR over `N` candles. | `1.5 ATR within 4 candles` | `supplyDemand.impulseAtr`, `supplyDemand.impulseCandles` |
| `departure at least X ATR within N candles` | Alias for the departure impulse requirement. | `1.5 ATR within 4 candles` | `supplyDemand.impulseAtr`, `supplyDemand.impulseCandles` |
| `departure body at least X` | Require at least one aligned departure candle body/range fraction to reach `X`. | `0` | `supplyDemand.minDepartureBodyToRange` |
| `zone width max X ATR` | Cap the detected zone width by ATR. | `1.2` | `supplyDemand.maxZoneAtr` |
| `zone bounds proximal distal` | Use wick-to-body bounds: demand is wick low to body high; supply is body low to wick high. | `wick` | `supplyDemand.zoneBounds` |
| `zone bounds body` | Use the base body range as the zone. | `wick` | `supplyDemand.zoneBounds` |
| `zone bounds wick` | Use the full base wick range as the zone. | `wick` | `supplyDemand.zoneBounds` |
| `retest within N candles` | Expire zones older than `N` candles. | `72` | `supplyDemand.maxZoneAgeCandles` |
| `retest tolerance X ATR` | Expand the touch test around the zone by `X` ATR. | `0.2` | `supplyDemand.retestToleranceAtr` |
| `retest max N` / `retest maximum N` | Allow at most `N` touches before a zone is rejected. | `1` | `supplyDemand.maxRetests` |
| `retest first touch only` | Require first touch only. | `1` | `supplyDemand.maxRetests` |
| `retest must reject zone` | Require the signal bar, or an allowed reaction bar, to close back outside the zone in the trade direction. | off (`0`) | `supplyDemand.requireRejectionClose` |
| `reaction within N candles` / `quick reaction within N candles` | After a touched zone fails the rejection close, allow up to `N` later candles for the rejection. `0` disables delayed reactions. | `0` | `supplyDemand.maxReactionCandles` |
| `price bounce within N candles` | Alias for the reaction window. | `0` | `supplyDemand.maxReactionCandles` |
| `choch within N candles lookback M` | After the zone touch, wait for a change-of-character close through the recent structure high/low. The structure level uses `M` candles of lookback and must confirm within `N` candles. | off (`chochCandles: 0`, `chochLookbackCandles: 3`) | `supplyDemand.chochCandles`, `supplyDemand.chochLookbackCandles` |
| `structure break within N candles lookback M` | Alias for the CHOCH requirement. | off (`chochCandles: 0`, `chochLookbackCandles: 3`) | `supplyDemand.chochCandles`, `supplyDemand.chochLookbackCandles` |
| `zone break X ATR` | Invalidate a demand zone when price closes below it by `X` ATR, or a supply zone when price closes above it by `X` ATR. | `0.05` | `supplyDemand.invalidationAtr` |
| `broken zones can flip` / `zone flip` | Permit one broken demand zone to become supply, or one broken supply zone to become demand. | off (`0`) | `supplyDemand.flipBrokenZones` |
| `wait N candles after zone` | Minimum delay after zone creation before retests can enter. | `1` | `supplyDemand.minWaitCandles` |
| `overlapping zones off` / `overlap zones off` | Skip a newly detected unused zone when it overlaps an existing unused zone of the same type. The current parser also treats `skip` and `reject` as enabling the skip. | allow overlaps (`0`) | `supplyDemand.skipOverlappingZones` |

## Defaults and interactions

`type: supply demand` rebases shared defaults to a zone-retreat profile:
stop padding `0.25 ATR`, min stop `0.4 ATR`, max stop `3 ATR`, target
`1R`, breakeven after `0.5R` plus `0.05 ATR`, and a 12-candle cooldown.
The setup starts with higher-timeframe bias enabled in runtime params unless
`higher timeframe off` is used.

The setup detects zones before trying entries on each bar. Entries are sorted
newest-zone first, mark the chosen zone used, and carry `supplyDemand` metadata
including zone bounds, pattern, base indexes, flip state, and CHOCH level. If
`trigger candle in (...)` is explicitly set, the runtime uses those shared
trigger settings; otherwise `useTrigger` is off for this family even though the
compiled `triggerCandles` list remains the core default. Shared `day type`,
`movement below`, sessions, side filters, stops, targets, partials, max hold,
cooldown, and fixed-risk directives feed the setup params.

## Example

```dsl
dsl v7
strategy "Supply Demand Spec Example" {
  description "Documented supply-demand zone retest example."
}

market conditions {
  slices(EURUSD 1h)
  sessions(london, ny)
  day type in (trending, ranging)
  movement below 0.95
}

setup {
  type: supply demand
  patterns(DBR, RBR)
  source leg at least 0.5 ATR within 4 candles
  base 2 to 5 candles
  base body max 0.55 ATR
  zone width max 1.2 ATR
  zone bounds proximal distal
  impulse at least 1.5 ATR within 4 candles
  departure body at least 0.55
  retest within 48 candles
  retest tolerance 0.2 ATR
  retest first touch only
  retest must reject zone
  reaction within 2 candles
  choch within 3 candles lookback 4
  zone break 0.05 ATR
  broken zones can flip
  overlapping zones off
  wait 1 candle after zone
}

filters {
  higher timeframe must agree
  side both
}

triggers {
  candle in (any)
}

risk {
  stop beyond zone break by 0.25 ATR
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
