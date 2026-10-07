# Master structural: native offline reference (HT-177)

`masterStructural` is a separate, strict `dsl v7` family. It implements the
frozen `v10-phase0-floor-half-reference-v1` reference contract without changing
the qualified `regimeEngine` v9 family. Its only execution entry point is the
native offline `engine/master.Run`, exposed by `heisentick master-report`.
Parsing is not permission to execute in the generic broker, grid, prefix or
engine-WASM routes. No registry, broker, deployment or browser execution
adapter is added.

## Complete source

```strat
dsl v7
strategy "Master structural source reference" {
  description "Fixed offline reference interpretation, synthetic example"
}
market {
  master timeframe M30 from M5
}
setup {
  type: master structural
  master profile v10-phase0-floor-half-reference-v1
  master mode SOURCE_HISTORICAL_REFERENCE
}
```

Exactly two case-sensitive mode identities are supported:
`SOURCE_HISTORICAL_REFERENCE` and `PROTECTED_STABLE_REFERENCE`. Each binds its
H4 horizon and lifecycle together. They cannot be independently combined,
changed, omitted, supplied as a list or replaced by legacy v9 modes.

The three blocks are required and may be reordered or written inline.
`strategy` requires a nonempty double-quoted name. Its optional double-quoted
`description` defaults to the empty string. Strings use JSON escapes.
`timeframe`, `type`, `profile` and `mode` must each appear exactly once in the
indicated block. Keywords are case-insensitive; the profile and mode values
are case-sensitive. `#` begins a comment outside a string. The strict scanner
consumes the entire source. Unknown or duplicate blocks/directives, malformed
Unicode, alternate quoting, semicolons, trailing tokens and generic
risk/session/management clauses fail closed.

Authored reserved selectors and `master mode`, `master profile`,
`master timeframe` or `masterStructural` clauses select the strict parser even
if malformed, mixed with another family or followed by a legacy selector.
Quoted metadata and genuine symbol-list arguments stay opaque. A damaged quote
cannot hide a subsequent authored selector from the strict admission guard.

## Closed configuration

The root contains exactly `dslVersion`, `name`, `description`, `setupType` and
`masterStructural`. `dslVersion` is 7; `setupType` is `masterStructural`.
The family object contains exactly `contractVersion`, `mode` and `profile`.
`contractVersion` is `master-structural-config-v1`.

The [schema](../schemas/master-structural-config-v1.schema.json) binds each
mode to its complete fixed profile, including a mode-specific `h4.horizon`
and `lifecycle`. Every nested profile field is mandatory and closed. No tick,
lot, phase, H4 horizon, quantity, signal, allocation, cost, management or
parameter-grid override is accepted. The two branches cannot be mixed.

The public Go surface is:

- `DecodeMasterStructural(Config) (MasterStructuralSpec, error)` validates the
  entire closed map without generic defaults; the spec has only `Mode`.
- `DecodeMasterStructuralConfigJSON([]byte) (Config, error)` additionally
  rejects duplicate decoded keys, malformed Unicode, trailing documents and
  numeric changes before lossy float conversion. Equivalent exact nonzero decimal
  spellings are accepted; nearly equal decimals are not. Fixed zero fields use
  canonical integer `0` under the inherited strict number decoder; alternative
  zero spellings such as `0.0` and `0e3` are rejected. This lexical restriction
  does not change the decoded numeric policy.
- `MasterStructuralProfile(mode)` returns a fresh independent map of the
  fixed policy for a valid mode, or nil for an unsupported mode.
- `IsMasterStructuralReserved(Config)` detects intent before generic defaults,
  dispatch and empty-data shortcuts. A reserved object key is sufficient even
  if null/scalar, attached to a legacy setup, or case/hyphen/underscore/spaced.
  Canonical and typed `FamilyID` discriminators are handled. This guard does
  not validate or accept aliases.
- Mode constants are `MasterStructuralSourceHistoricalReference` and
  `MasterStructuralProtectedStableReference`.

Every successful parse includes the warning constant
`MasterStructuralDedicatedRunnerRequired`, whose value is
`master-structural-native-dedicated-runner-required`.

## Fixed signal, clock and arithmetic

- XAUUSD M5 reference OHLCV is aggregated to UTC phase-zero M30 buckets using
  first/max/min/last/sum. Source rows must have unique ordered exact M5 opens.
  No interpolation is performed. Partial observed native bars stay in
  indicator history; 40 consecutive complete observed M30 bars are required
  at a decision. Whole missing bars do not reset that observed-bar count.
- HMA-style lengths 9/21 use floor-half and round-half-up square-root lengths;
  25 is diagnostic only. The cross is strict signed, with previous equality
  qualifying. Chart Supertrend alignment, strict current-inclusive SMA20
  volume and ATR14/close × 100 >= 0.08 are required. This threshold is price
  volatility, not allocation.
- Wilder ATR uses first-n-finite-TR SMA initialization. Chart Supertrend uses
  the qualified v9 factor-3 ATR10 recurrence, +1 bearish and -1 bullish.
- H4 is aggregated from the observed M30 bars at UTC 00/04/08/... boundaries,
  with M5 count summed. It does not require eight complete constituents.
  Nominal availability is bucket start + 4h. H4 uses the same Supertrend
  recurrence, including initial direction +1 before a line exists. Missing
  lines are null; the gate uses mapped direction alone. A future nominal
  close cannot be queried.
- SOURCE queries the latest H4 nominal close <= M30 decision close; PROTECTED
  queries <= M30 bar open. H4 finalization precedes the source M30 decision at
  equal timestamps. Both mappings remain explicit in the indicator report.
- Entry is flat-only at the next actually observed native open. Warmup signals
  do not carry across trade start. Warmup < start < exclusive end must be exact
  UTC M30 boundaries, with first source open at warmup and last source close
  at end. The final completed bar handles exits/management but emits no entry.
  No at/after-end source quote affects the run; no terminal liquidation is
  fabricated.
- Initial equity is 10000 USD, allocation 0.1 of then-current equity, and
  point value 1. Quantity is exactly
  `floor((equity * 0.1 / actualEntry + 1e-12) / 0.1) * 0.1`.
  The epsilon is applied before division by the 0.1 step. A nonpositive
  rounded quantity terminates the run with `quantity_below_step` before a fee
  or position is created. Nonfinite/invalid arithmetic fails closed.
- Price arithmetic is continuous float64 reference arithmetic. No tick
  rounding is applied. Pine protected-control mintick rounding is a separate,
  uncompiled policy with no parity claim. Fixed fee is 0.50 USD per unit per
  side; entry debits cash immediately, exit returns gross less its exit fee.
  One unit as one ounce remains an illustrative, unverified assumption.

## Two indivisible lifecycles

SOURCE anchors to signal close, seeds directional extrema with signal high/low
and keeps post-fill extrema separately. The entry bar is unprotected until its
close. Each surviving completed bar updates extrema and recomputes non-sticky
activation at signal close +/- 2 × current ATR14. Before activation, the stop
is directional-best-of anchor +/- 2ATR and chart ST; while active it is bare
ST. Target is anchor +/- 4 × current ATR14. Widening, ATR-driven deactivation,
pre-entry-extreme activation and target changes remain explicit. There is no
explicit adverse-regime exit. Wrong-side stops schedule a next-observed-open
market exit. Newly marketable targets remain non-latched limits assessed on
the subsequent open/path, preserving possible gap-back behavior.

PROTECTED freezes signal ATR. Initial distance is the lesser of 2 × signal
ATR and a positive direction-adjusted signal-close-to-ST distance, otherwise
2 × signal ATR. The bracket is attached to the pending entry, translated from
actual fill, and active on the whole observed entry-bar path. Stop distance is
fixed; target is fill +/- 4 × signal ATR. Extrema are post-fill only.
Activation at fill +/- 2 × signal ATR latches permanently. Before activation
the initial stop does not change. After activation, chart ST may ratchet only
when it agrees with position direction and is strictly on the proper side of
current reference close; it cannot loosen. A completed adverse direction
transition schedules `regime_flip_market_exit` at the next observed open while
its bracket remains represented as live. The pending forced exit has opening
priority over bracket evaluation.

Activation includes exact equality at the 2ATR threshold. Execution barriers,
proper-side tests and ratchets use the contract's exact inequalities, never a
comparison tolerance. Diagnostic widening, target-change and hypothetical
source lock-switch comparisons retain 1e-9. Source hypothetical relaxation is
separate from actual live-stop widening. Protected rejected worse-ST candidates
are diagnostic rejections, not widening or source relaxation.

## Native path and evidence limits

Every path is the aggregated M30 OHLC path, never constituent M5 traversal.
Opening gaps are assessed before the path. The path is O-H-L-C only when high
is strictly nearer to open than low; ties use O-L-H-C. Gap barriers fill at the
actual observed open, allowing target improvement. Intrabar barrier fills are
interval-censored to native close. Long entry uses reference open + spread;
short liquidation shifts the whole reference path by spread. Short entry and
long exit use the bid-style reference. This is an offline cost scenario, not
verified quote or broker provenance.

A pending market entry is never canceled because the next bar will later prove
incomplete. Its fill uses the first real observed M5 open and timestamp, with
partial traversal flagged. This deliberately retains the qualified v9
causality correction and differs from the supplied Python's future-completeness
cancel. No slippage, funding, liquidity, margin or broker-minimum-stop guarantee
is claimed.

The two `family-master-structural-*.strat` conformance fixtures are synthetic
parse-only controls. No generic runnable case is added. Tests cover the
mode-coupled schema, every fixed field, selector bypasses, mixed families,
duplicate keys and exact numeric admission. Historical comparison is a
separate gated step; this language addition alone makes no performance,
TradingView parity, live execution, release or production-adoption claim.
