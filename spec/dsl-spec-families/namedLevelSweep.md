# Setup family: `named level sweep` (namedLevelSweep)

Status: specified (2026-09-26)

Part of `spec/dsl-spec.md` §9; design plan and naming rationale live in
`heisentick-backlog/plans/2026-09-26-dsl-named-level-sweep.md` and
`heisentick-backlog/tasks/HT-054-dsl-named-level-sweep.md`. Phase A (this
file, parser phrases in both languages, and parse conformance) is specified
and implemented; the runtime (the actual entry/stop/target evaluation on a
bar series) is Phase B and not yet implemented — see "Status of the runtime"
below.

## Why this name, not `level sweep`

"Level sweep" already names two different things in the shipped code:
`family_level_sweep.go` (this repo's Go mirror of the `failed breakout`
family — a *range/channel edge* sweep-and-reclaim, not a named level) and a
dead, never-wired JS stub at `engine/dsl/setups/levelSweep.js` in the app
repo (deleted as part of this family's Phase A). "Named" is the load-bearing
word: this family fires on a sweep or approach of a *named* level you have
already declared in `levels { priority(...) }` — `PDH`/`PDL`, `WH`/`WL`,
`AH`/`AL`/`LH`/`LL`/`NH`/`NL`, `DO`/`DH`/`DL`, `RN<step>`, a custom
`support`/`resistance` key, and so on — never the detected range or channel
edge that `failed breakout` reads.

## Type phrase

```
setup {
  type: named level sweep
}
```

## Purpose

Trades a wick through (or a close approach to) one named level from the
shared `priority(...)` universe, followed by a close back on the level's
origin side. Two entry-trigger verbs cover the two geometries strategies in
this family actually use — a *true* sweep, where the wick must trade beyond
the level, and a lenient *test*, where the wick only needs to approach
within a tolerance and need not cross at all.

**Direction mapping** is fixed by the level's own type, not configurable:
sweeping/approaching a high-type level (`PDH`, `WH`, `AH`, `LH`, `NH`, `DH`,
a `resistance` key) defaults to a short trade (price rejects downward from
the top); a low-type level (`PDL`, `WL`, `AL`, `LL`, `NL`, `DL`, a `support`
key) defaults to a long trade. `then signal <side>` can still name either
side explicitly — for a level whose key name carries no polarity (`VWAP`,
`EMA`, `POC`, `VAH`, `VAL`, `DO`, `CAM_*`, `RN<step>`, or a custom key) there
is no default to override; for a high/low-type key, naming the opposite side
is a compile *warning*, not an error (§ Compile diagnostics below) — a
future strategy may legitimately want the countertrend direction.

**One signal per level per day** is the default invalidation rule (Phase B):
a second sweep of the same level on the same day does not re-signal even if
the reclaim geometry re-forms, mirroring `failed breakout`'s per-day dedup.

## Phrases

| Phrase shape | Meaning | Config keys |
| --- | --- | --- |
| `when price sweeps <LEVEL> [by at least X ATR] and closes back above\|below it [by at least Y ATR] [within N candles] then signal <side> [grade G]` | A *true* sweep: the wick must trade beyond `<LEVEL>` (by at least `X` ATR if given; any strict crossing if `X` is omitted — there is no "by at least 0 ATR" spelling for that), then closes back past it by at least `Y` ATR (default `0`), within `N` candles (default `1` = same bar). The close direction (`above`/`below`) must agree with `<side>`'s polarity — `above` for `long`, `below` for `short` — a compile error otherwise. | `namedLevelSweep.rules[<LEVEL>] = {mode: "sweep", sweepAtr (nullable), reclaimAtr, reclaimCandles, side, grade?}` |
| `when price tests <LEVEL> within X ATR and closes above\|below it [by at least Y ATR] [within N candles] then signal <side> [grade G]` | A lenient tap: the wick only needs to reach within `X` ATR of `<LEVEL>` (need not cross it at all — `X` is required, there is no default "how close counts"). Closes past it by at least `Y` ATR (default `0`), within `N` candles (default `1`). Same close-direction/side agreement rule as `sweeps`. | Same shape, `mode: "test"`, `sweepAtr` required |
| `stop below the signal candle and the level by X ATR` | For a long setup: stop at `min(signal-bar low, level price) - X * ATR` — the further-below of the two. | `namedLevelSweep.stop.long = {paddingAtr: X}` |
| `stop above the signal candle and the level by X ATR` | For a short setup: stop at `max(signal-bar high, level price) + X * ATR` — the further-above of the two, same underlying formula mirrored. | `namedLevelSweep.stop.short = {paddingAtr: X}` |
| `trigger { no confirmation candle }` | Disables the confirmation-candle gate — a clearer synonym for `candle in (any)` for a setup whose original has no candle-shape requirement at all. Compiles to the same `triggerCandles: ["any"]` / `triggerExplicit: true` that `candle in (any)` already produces; offered as shared trigger vocabulary, not a family-locked spelling. | `triggerCandles`, `triggerExplicit` |
| `prior day type in (<types>)` / `not in (...)` | Already-existing shared phrase (§4/§6 of `dsl-spec.md`) reading `classifyPriorDayType`'s enum (`range`, `outside`, `trend`, `wide`, `narrow`) — noted here because it pairs with `prior day range at least` below in this family's own examples, not because it changed. | `priorDayTypes` / `blockedPriorDayTypes` |
| `prior day range at least X ATR` | New day-shape gate, shared vocabulary (not family-locked): reject unless `priorDay.h - priorDay.l >= X * ATR`. | `priorDay.minRangeAtr` |

`sessions(...)`, `side long only`/`side both`, `higher timeframe must agree`,
`candle in (...)`, `move stop to breakeven after X R plus Y ATR`,
`wait N candles after trade`, `target <N>R`, `stop size min A max B`,
`risk 200 USD`, and `priority(...)` are all pre-existing shared directives;
none of them change for this family.

## Compile diagnostics

- An unknown level key in a `when` line reuses the exact `priority(...)`
  known-level check, e.g.: `unknown level "NOTALEVEL" in entry rule —
  known: PDH, PDL, PDO, PDC, DO, DH, DL, WH, WL, AH, AL, LH, LL, NH, NL,
  range.high, range.low, CAM_R3, CAM_R4, CAM_S3, CAM_S4, channel.high,
  channel.low, VWAP, EMA, POC, VAH, VAL, RN<step> (round numbers), or a
  defined S/R/TL name`.
- A negative `X` in `by at least X ATR` / `within X ATR` is a compile
  error: `sweep/test distance must be >= 0`.
- `tests <LEVEL>` with no `within X ATR` clause is a compile error:
  `"tests" entry rule requires "within X ATR" — there is no default
  tolerance for "tests"`.
- The close direction (`above`/`below`) not matching `then signal <side>`'s
  polarity is a compile error, e.g.: `close direction "above" does not
  match "then signal short" — use "below"`.
- `then signal <side>` naming a side that contradicts a *known* high/low
  level key's default polarity is a compile **warning**, not an error, e.g.:
  `PDH is a high-type level; 'then signal long' trades against its default
  polarity — confirm this is intended`.
- `type: named level sweep` with no `when price sweeps|tests ...` line at
  all is a compile warning: `named level sweep declares no entry rule —
  add a "when price sweeps ..." or "when price tests ..." line, or nothing
  will ever signal`.

## Status of the runtime

Phase A (this document, the JS and Go parser phrases, and parse
conformance) is implemented. The engine side —
`engine/family_named_level_sweep.go` (Go) and
`engine/dsl/setups/namedLevelSweep.js` (JS): the actual sweep/test geometry,
the unified stop formula, and the one-per-level-per-day dedup — is Phase B
and not implemented yet. A `.strat` source using this family parses and
compiles cleanly under both languages today, but does not yet execute
against a bar series; `namedLevelSweep` is deliberately left out of the Go
engine's `implementedFamily(...)` allow-list until Phase B lands, so running
one produces a clear "setup family is not implemented" error rather than
silently falling back to another family's runtime.

## Examples

The three worked ports below are the family's design cases (plan §2); all
three compile without error under both the JS and Go parsers today.

### `dslDailySndRetestXauusdFourHour` — a lenient `tests` port

```dsl
dsl v7
strategy "DSL Daily SND Retest XAUUSD 4H" {
  description "Daily PDH/PDL retest on the 4h chart during London/NY, gated by prior-day type and minimum prior-day range."
}

market conditions {
  slices(XAUUSD 4h)
  sessions(london, ny)
  prior day type in (range, outside)
  prior day range at least 0.5 ATR
}

levels {
  priority(PDL, PDH)
}

filters {
  side long only
}

trigger {
  candle in (pin, engulf, outside)
}

setup {
  type: named level sweep
}

entry {
  when price tests PDL within 0.35 ATR and closes above it then signal long
}

risk {
  stop below the signal candle and the level by 0.15 ATR
  stop size min 0.4 max 4.0
}

target {
  target 4.0R
}

management {
  move stop to breakeven after 0.75R plus 0.05 ATR
  wait 4 candles after trade
}

execution {
  risk 200 USD
}
```

### `dslInstitutionalLiquiditySweep` — a true `sweeps` port, both sides

```dsl
dsl v7
strategy "Institutional Liquidity Sweep" {
  description "Fade weekly-high/low liquidity sweeps that agree with the higher-timeframe trend."
}

market conditions {
  slices(XAUUSD 1h, USDJPY 1h, GBPUSD 1h)
}

levels {
  priority(WH, WL)
}

filters {
  higher timeframe must agree
  side both
}

trigger {
  no confirmation candle
}

setup {
  type: named level sweep
}

entry {
  when price sweeps WH and closes back below it then signal short
  when price sweeps WL and closes back above it then signal long
}

risk {
  stop above the signal candle and the level by 0.2 ATR
  stop below the signal candle and the level by 0.2 ATR
}

target {
  target 3.0R
}

execution {
  risk 200 USD
}
```

### `priorSessionSweepReversal` — a `tests` port with a required reclaim margin

Optional port (plan §2.3/§6): its zone tolerance is percent-of-price, not
ATR, so the figures below approximate the original geometry; kept only if
Phase C's trade-by-trade parity run clears the 99% bar.

```dsl
dsl v7
strategy "Prior-Session Sweep Reversal" {
  description "Fade liquidity sweeps of the prior London session high/low: test the level, close back beyond it by a margin, then take the reversal."
}

levels {
  priority(LH, LL)
}

filters {
  side both
}

trigger {
  no confirmation candle
}

setup {
  type: named level sweep
}

entry {
  when price tests LH within 0.12 ATR and closes below it by at least 0.12 ATR then signal short
  when price tests LL within 0.12 ATR and closes above it by at least 0.12 ATR then signal long
}

risk {
  stop above the signal candle and the level by 0 ATR
  stop below the signal candle and the level by 0 ATR
}

target {
  target 2.0R
}

execution {
  risk 200 USD
}
```
