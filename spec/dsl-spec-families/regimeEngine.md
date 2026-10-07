# Regime engine: native offline study (HT-175)

`regimeEngine` is a Go-only, dedicated-runner family under `dsl v7`. It is an
explicit XAUUSD M5-to-M30 interpretation of the preserved Regime Engine source,
not certified Pine/TradingView parity. Parsing does not admit the configuration
to the generic broker, grid, prefix, or browser execution paths. The native
entry point is `engine/regime.Run`, exposed by `heisentick regime-report`.

## Complete source

```strat
dsl v7
strategy "Regime engine source-like control" {
  description "Fixed audited interpretation, synthetic example"
}
market {
  regime timeframe M30 from M5
}
setup {
  type: regime engine
  regime profile v9-floor-half-v1
  regime mode source-like-v1
}
```

The only other mode is `audit-baseline-v1`. A mode must be authored explicitly;
there is no implicit mode, extra early-bracket variant, or parameter grid.

The three blocks are required, may be reordered, and may appear inline.
`strategy` requires a nonempty double-quoted name. Its `description` is optional
and defaults to the empty string. Description/name strings use JSON escapes.
The timeframe, type, profile, and mode directives are each required exactly
once in the indicated block. Keywords are case-insensitive; profile and mode
identities are case-sensitive. `#` starts a comment outside a string.
Semicolons, alternate quoting, unknown directives, unconsumed tokens, repeated
blocks/keys, generic risk/session/management clauses, and missing braces fail.
No legacy parser defaults are inherited.

Malformed or relabeled reserved selectors and authored `regime mode`,
`regime profile`, and `regime timeframe` directives are sent to the strict
scanner even if a later legacy family selector appears. Legacy free metadata,
quoted text, symbol-list values, and old deprecated `regime` phrases do not
select this family merely by mentioning it.

## Fixed `v9-floor-half-v1` profile

The normalized configuration records every fixed value in
`regimeEngine.profile`, including `symbol: XAUUSD`:

- Source M5 reference OHLCV, native UTC M30 aggregation
- HMA fast 9 and slow 21; HMA25 is diagnostic only
- HMA half-length uses floor; square-root length uses round-half-up
- Supertrend Wilder ATR10, factor 3, first finite-window SMA seed
- Volume strictly greater than the current-inclusive SMA20
- Wilder ATR14 with the same seed convention; stop multiple 2, target 3.5
- The latest 40 observed native bars must each contain all six exact M5 slots
- Notional fraction 0.1, chart point value 1, continuous quantity
- Fee 0.5 per unit per side; one unit interpreted as one ounce is an
  illustrative assumption, not a verified broker contract
- Terminal policy `keep-open`

Missing M5 bars are never interpolated. Partial native bars remain in indicator
history. Session/weekend gaps between complete observed native bars do not
reset the 40-bar condition. A strict HMA signed cross at a native close counts
prior equality and must agree with current Supertrend direction and current
volume. There is no delayed gate memory, scale-in, or inherited session filter.

## Two execution policies

`source-like-v1` schedules an accepted flat-position signal for the next
observed native open. Quantity is equity × 0.1 / actual entry price. The entry
fee is debited immediately. The signal close anchors dynamic ATR stop/target
levels. The first bracket becomes live after the entry candle closes, leaving
that candle's intrabar path unprotected. Native-close edits use current ATR and
Supertrend, can widen the stop, and can change the target. An invalid wrong-side
stop schedules a next-open market exit rather than an advantageous retro-fill.
There is no explicit regime-flip close. A newly marketable target remains a
regular non-latched limit for the next open/path; this is an explicit offline
convention, not a certified Pine target-activation rule.

`audit-baseline-v1` uses the same signals but freezes signal ATR and initial
risk: the lesser of 2 × ATR and a positive direction-adjusted structural
Supertrend distance; otherwise 2 × ATR. Its bracket is attached to the pending
entry, translated from the actual fill, and live on the entry bar. The target
is frozen at 3.5 × signal ATR. A valid same-regime Supertrend can only ratchet
the stop. A completed-bar opposite regime flip schedules a next-open exit.

Both policies use the declared native OHLC chronology: opening gaps first;
then O-H-L-C if the high is strictly closer to open than the low, otherwise
O-L-H-C, including ties. Bid-style reference prices support explicit spread
stress, including the whole short liquidation path. Fees, spread and the
assumed path are research inputs/conventions, not an execution-quality claim.
Stop/target path fills are interval-censored to native close; gap and market
fills use actual observed open timestamps. No slippage, financing, contract
rounding, or terminal liquidation is fabricated.

A pending entry is not canceled using knowledge of the next bar's future
completeness. If an opening constituent is absent, the first actually observed
M5 open and its timestamp supply the fill. Explicit trading endpoints are UTC
M30 boundaries and require source coverage through the completed history
watermark. No quote at/after the exclusive endpoint is consumed. The final
completed candle may execute exits and update surviving orders but cannot emit
a new entry signal. Indicator context before the trade start cannot leak an
entry into the trade window. Final open exposure and pending signals are kept.

## Configuration boundary

The root has exactly `dslVersion`, `name`, `description`, `setupType`, and
`regimeEngine`. The latter has exactly `contractVersion`, `mode`, and `profile`.
`contractVersion` is `regime-engine-config-v1`; the profile is completely fixed.
The [closed schema](../schemas/regime-engine-config-v1.schema.json) rejects
missing, unknown, null, altered, and unsupported values.

`DecodeRegimeEngine(Config)` validates this complete shape and returns a
`RegimeEngineSpec` whose sole variable policy field is `Mode`.
`DecodeRegimeEngineConfigJSON` is the raw JSON boundary: it also rejects
duplicate decoded keys, malformed Unicode, multiple documents, and numeric
mutations before a lossy float conversion. A generic JSON-to-map decoder cannot
recover duplicates or lost numeric precision, so it is not equivalent to this
raw boundary.

`IsRegimeEngineReserved` is an admission guard, not a validator. Presence of a
reserved family key is sufficient even if its value is null/scalar or a legacy
setup label is attached. Key/discriminator case variants, typed `FamilyID`
values, and compact/spaced/hyphenated/underscored family names remain reserved;
only the canonical closed configuration is valid.

Every successful parse includes the stable warning
`regime-engine-native-dedicated-runner-required`. This warning does not imply
that generic execution is available or that historical validation is complete.

## Evidence scope

The two `family-regime-engine-*.strat` conformance cases are synthetic parsing
fixtures, one for each mode. Strict grammar/config/schema mutation tests live
in `dsl/regime_engine_*_test.go`; the dedicated runner has independent synthetic
semantics tests under `engine/regime`. Existing parse and run goldens are
unchanged. This addition makes no historical performance, strategy-selection,
production-adoption, release, or broker-parity claim.
