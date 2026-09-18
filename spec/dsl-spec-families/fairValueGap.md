# Setup family: `fair value gap` (`fairValueGap`)

Status: TODO (semantics specified 2026-08-13; parser, runtime, Go, and
conformance implementation is assigned to the adjacent lanes)

Part of `strat/docs/dsl-spec.md` §9. This document is the normative contract for the
family. It deliberately does not add a strategy registry entry or imply that
an FVG setup has passed the strategy promotion gates.

## Prior art checked

Before writing this contract, the required prior-art searches were run for
`FVG`, `fair value gap`, `imbalance`, `ICT`, `supply demand`, and `Fibonacci`.
The indexed search reported:

| Query | Result | Interpretation |
| --- | --- | --- |
| `FVG` | No direct match | No indexed FVG strategy or family. The tool warns that 161 of 276 notes are incomplete, so this is not proof that no older experiment exists. |
| `fair value gap` | 15 matches | Adjacent value-area, volume-profile, opening-gap, and audit notes; none specifies a causal three-candle FVG primitive. |
| `imbalance` | No direct match | No indexed imbalance strategy or DSL primitive. |
| `ICT` | 2 matches | `daily-strict-nr7-close-break-fixed-universe-2026-08-11.md` is a rejected daily range study; `candidate-queue-verdicts-2026-07-19.md` is an uncategorized queue note. Neither defines FVG semantics. |
| `supply demand` | 18 matches | The existing supply-demand family and recent aggregate/rejection screens are adjacent zone logic, not FVG logic. The FVG family must not silently reuse supply-demand base/departure parameters. |
| `Fibonacci` | 1 direct strategy match | `cross-asset-fib-continuation-index-gold-2026-08-12.md` rejects the fixed-universe continuation screen. Fibonacci remains a separate confluence or target concept. |

The case-insensitive corpus fallback found no existing `FVG`, `fair value gap`,
or `imbalance` source, setup family, parser phrase, or conformance case. The
closest documented neighbors are `supplyDemand` and `fibContinuation`; they
were used for vocabulary and documentation shape only.

## Type phrase

```text
setup {
  type: fair value gap
}
```

The canonical compiled family id is `fairValueGap`. `type: fvg` is an alias
for the same family. The alias must not create a second runtime behavior or
second conformance identity.

## Purpose

Fair value gap identifies a three-bar displacement gap and waits for a later
bar to retest that gap. A bullish gap supplies a long setup; a bearish gap
supplies a short setup. The family is intentionally narrower than the ICT
umbrella: apart from the optional sweep gate below, it does not infer CHOCH, order blocks, premium or
discount, or Fibonacci confluence. Those remain separate shared filters or
future setup families.

## Formation and zone geometry

Index source bars by their completed-bar position. At the close of source bar
`i`, with `i - 2`, `i - 1`, and `i` all available, form exactly one candidate:

- bullish when `bar[i-2].high < bar[i].low`;
- bearish when `bar[i-2].low > bar[i].high`.

The inequalities are strict. A touching boundary is not a gap. The middle bar
`i - 1` is part of the three-bar pattern but does not define the gap bounds.
The middle bar `i - 1` is the displacement candle. The newest bar `i` is the
creator/formation bar whose close makes the gap observable; it supplies the
newest extreme but is not the displacement body used by the threshold.

The zone is a closed price interval:

| Direction | Lower bound | Upper bound | Proximal edge | Far edge |
| --- | --- | --- | --- | --- |
| Bullish | `bar[i-2].high` | `bar[i].low` | upper bound | lower bound |
| Bearish | `bar[i].high` | `bar[i-2].low` | lower bound | upper bound |

The gap width must be at least the configured `gap minimum` times the ATR
available at the creator-bar close. The middle candle body size
`abs(close - open)` must be at least the configured
`displacement minimum` ATR.
Both gates use only completed source-bar data through `i`. Missing, non-finite,
zero, or otherwise invalid ATR rejects the formation rather than disabling the
gate. Invalid or inverted bounds also reject it.

## Sweep gate (liquidity sweep before displacement)

`sweep required` narrows the family to ICT-style setups in which price first
raids a swing point of the opposite side before the displacement move. The gate
is evaluated only from completed bars up to the displacement candle and stored
with the gap at formation, so admission never re-runs it on later data.

Definition. Index the displacement candle `d` (= formation `i - 1`). With pivot
width `k` (`sweep pivot`), a pivot at `j` exists when, for the gap side:

- long: `bar[j].low < bar[j-n].low` for `n = 1..k`, and
  `bar[j].low <= bar[j+n].low` for `n = 1..k`;
- short: mirrored on highs (`>`/`>=`).

Left-side comparisons are strict; right-side comparisons allow equality so
pooled equal lows/highs form one liquidity pool. The pivot's right flank must
complete at or before `d` (`j + k <= d`). A sweep exists when some bar
`m` in `[j + k, d]` trades strictly through the pivot extreme
(`low < pivot low` for long, `high > pivot high` for short). The formation is
rejected when no qualifying pivot-plus-sweep pair lies in the window of
`sweepLookbackCandles` candles ending at `d`. Pivot detection partners that
would look past `d` are never inspected. `sweep off` (the default) admits every
formation.

## Phrases

| Phrase shape | Meaning | Default | Config key |
| --- | --- | --- | --- |
| `gap minimum X ATR` | Minimum strict width of the three-bar gap, measured at the creator-bar close. `X` must be finite and positive. | `0.1 ATR` | `fairValueGap.minGapAtr` |
| `displacement minimum X ATR` | Minimum middle-candle body size at the completed formation close. `X` must be finite and positive. | `0.8 ATR` | `fairValueGap.minDisplacementAtr` |
| `retest within N candles` | Maximum number of later source candles in which a retest may be admitted. `N` must be a positive integer. | `8` | `fairValueGap.retestCandles` |
| `entry at midpoint` | Set the causal entry reference to `(lower + upper) / 2`. | midpoint | `fairValueGap.entryReference` |
| `entry at proximal edge` | Set the causal entry reference to the edge nearest the creator bar: upper for bullish, lower for bearish. | midpoint | `fairValueGap.entryReference` |
| `sweep required [within N candles]` | Require a liquidity sweep of a swing pivot within the last `N` candles before the displacement candle. Default off; default window `30`. | off | `fairValueGap.sweepRequired`, `fairValueGap.sweepLookbackCandles` |
| `sweep pivot N candles` | Fractal pivot width used by the sweep gate (left/right bar count). `N` must be a positive integer. | `2` | `fairValueGap.sweepPivotWidth` |
| `sweep off` | Disable the sweep requirement (also the default when the phrase is absent). | — | `fairValueGap.sweepRequired` |

The family accepts the shared `side`, `source timeframe`, higher-timeframe,
session, trigger, risk, target, management, and execution directives. A
`source timeframe` changes which completed bars form and age the gap; it does
not permit a forming higher-timeframe candle to create or retest one. A lower
entry timeframe is an execution detail under the shared source-timeframe
contract and cannot change the source gap's geometry or lifecycle.

## Causal lifecycle

1. At the close of creator bar `i`, evaluate the formation and create an active
   gap only when all gates pass. The gap does not exist during bar `i`.
2. Start the retest clock at the next completed source bar, `i + 1`. The
   creator bar can never retest the gap it creates.
3. A later bar retests when its completed range intersects the closed zone:
   `bar.low <= upper && bar.high >= lower`. A touch at either boundary counts
   as a retest; formation itself still requires strict separation.
4. Before admitting a retest, invalidate a bullish gap when a completed bar
   closes below its far edge, or a bearish gap when it closes above its far
   edge. An invalidating bar cannot also produce an entry.
5. A valid retest may produce at most one signal for that gap. Once a signal is
   emitted, the gap is consumed and cannot emit a second signal. A retest that
   is not admitted by shared filters leaves the gap active until expiry or
   invalidation; this keeps setup state separate from broker admission.
6. Expire the gap after the declared retest window. No later bar may revive an
   expired or invalidated gap. If several active gaps qualify on one bar, use
   the newest creator first and admit at most one family signal on that bar,
   subject to the normal broker/cooldown rules.

All comparisons above use completed bars. No future bar, later close, or
post-entry excursion may affect formation, retest, invalidation, or entry
reference.

## Entry-reference decision

`entry at midpoint` and `entry at proximal edge` select a deterministic
`entryReference` value for setup metadata and stop/target calculation. The
runtime submits a one-bar resting limit order at that reference after the
completed retest bar. The broker records a fill only if a later completed bar
trades through that limit; no intrabar future information is used.

## Defaults and interactions

The implementation must mirror these defaults in JavaScript and Go:

```text
fairValueGap.minGapAtr = 0.1
fairValueGap.minDisplacementAtr = 0.8
fairValueGap.retestCandles = 8
fairValueGap.entryReference = midpoint
```

The family must fail closed on unknown entry-reference values, non-positive or
non-integer retest windows, malformed numeric tails, missing ATR, unavailable
source bars, non-finite prices, and invalid zone bounds. Shared setup defaults
remain authoritative where this family does not define a value; the family
must not copy supply-demand or Fibonacci defaults merely because those are
nearby concepts.

`side long only` admits bullish gaps only. `side short only` admits bearish
gaps only. `side both` admits both directions. Shared trigger and broker gates
run after the family has identified a valid retest; a rejected broker signal
does not mutate the gap into a new formation.

## Implementation fixture

The following source is the contract fixture for the parser/runtime lanes. It
is shown as text while the family is `TODO`; the implementation lane should
promote it to the repository's compiling DSL-example fixture when the JS and
Go parsers accept the family.

```text
dsl v7
strategy "Fair Value Gap Spec Fixture" {
  description "Causal three-bar FVG formation and later retest."
}

market conditions {
  slices(XAUUSD 4h)
}

setup {
  type: fair value gap
  gap minimum 0.1 ATR
  displacement minimum 0.8 ATR
  sweep required within 12 candles
  sweep pivot 2 candles
  retest within 8 candles
  entry at midpoint
}

filters {
  higher timeframe must agree
  side both
}

execution {
  risk 200 USD
}
```

The fixture deliberately leaves entry, stop, and target policy to the shared
language. A research strategy must declare those explicitly before any
measurement, and its 4h results must be judged under the strategy-research
loop's raw → realistic → harsh stages.
