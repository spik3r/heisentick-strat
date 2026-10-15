# Parabolic SAR: chart-series filter contract (HT289)

This additive DSL v7 feature is computed by Go in native and Go/WASM runtimes.
It supplies SAR, direction and reversal columns and optional entry predicates.
It does not move stops, close positions, or create a strategy.

## Syntax and config

```strat
dsl v7
filters {
  sar start 0.02
  sar step 0.02
  sar max 0.2
  sar supports entry
}
```

All three numbers are acceleration factors (not percentages). Defaults are
start=0.02, step=0.02, max=0.2. Each parameter may appear at most once; order is
free. Require finite `0 < start <= max <= 1` and `0 < step <= max`. Values above
one, zero, negative, NaN and infinity are rejected. Start and step can differ.
This intentionally bounded configuration is not TA-Lib's permissive range.

Exactly zero or one predicate may be authored:

| Directive | Closed signal-bar condition |
| --- | --- |
| `sar trend bullish` | published direction +1 |
| `sar trend bearish` | published direction -1 |
| `sar close above` | close strictly greater than SAR |
| `sar close below` | close strictly less than SAR |
| `sar reverses bullish` | published direction changes -1 to +1 |
| `sar reverses bearish` | published direction changes +1 to -1 |
| `sar supports entry` | +1 for a proposed long; -1 for a proposed short |

Parameter-only directives compute the series without gating entries. Other
filters still apply. Duplicate parameters/predicates, trailing operands, wrong
sections, versions before v7, and unknown config fields are errors. Compact
single-line filters have the same meaning as multiline filters.

The emitted config is `parabolicSAR: {start, step, max, predicate?}`; predicate
values are `bullish`, `bearish`, `closeAbove`, `closeBelow`, `flipBullish`,
`flipBearish`, or `supportsEntry`. Direct configs use the same decoder, with
omitted numeric fields defaulted. Nulls and numeric strings are not numbers.
See [the field schema](schemas/parabolic-sar-config-v1.schema.json); the Go
decoder additionally enforces the cross-field inequalities and route support.
The generic parse-result schema deliberately keeps config implementation-owned.

## Availability and exact numerical variant

Indices below are zero-based and refer only to observed, closed chart bars.
`contextcols.ComputeParabolicSAR` returns aligned `SAR []float64`, `Trend []int8`
and `Flip []int8`. An unavailable row is NaN/0/0. Empty and one-bar series have
no available row. No output at index zero is backfilled from the second bar.

Initialization, immediately before processing bar 1:

- If close[1] > close[0], seed rising: SAR=low[0], EP=high[1].
- Otherwise, including equal closes, seed falling: SAR=high[0], EP=low[1].
- AF=start. Initialization itself is not a published flip.

For each bar i from 1 onward, in this exact order:

1. Calculate delta=EP-SAR, advance=AF*delta, candidate=SAR+advance with separately
   rounded binary64 operations (no fused multiply-add).
2. Test penetration against the current bar before applying prior-bar clamps.
   Rising reverses only if candidate > low[i]; falling only if candidate < high[i].
   Equality does not reverse. On rising-to-falling reversal set
   SAR=max(high[i], old EP), EP=low[i], AF=start. On falling-to-rising reversal
   set SAR=min(low[i], old EP), EP=high[i], AF=start. At most one reversal per bar.
3. If this is neither bar 1 nor a reversal bar, update EP only on a strict new
   high in rising mode or strict new low in falling mode. Then increment
   AF=min(AF+step,max). An equal extreme does not increment AF. The updated AF
   affects the next bar, not the candidate already calculated.
4. Rising SAR is clamped down to low[i-1] and, when i>1, low[i-2]. Falling SAR
   is clamped up to high[i-1] and, when i>1, high[i-2]. This also applies after a
   reversal. The current bar is not included in these prior-bar clamps.
5. Publish SAR and direction. Publish Flip equal to the new direction only
   when i>1 and direction differs from the preceding published direction.
   A reversal during the initialization bar therefore has Flip=0.

SAR is causal but stateful: prefix outputs are bit-identical to the same
indices in a longer input. Truncating prior history can change all later
values. Timestamp gaps advance once per supplied bar, with no interpolation,
reset, or multiplication by elapsed time. No fixed stabilization length beyond
the two-bar availability rule is promised.

## Provenance and variant differences

The recurrence, strict penetration, close-based bootstrap and prior-two-bar
clamps are consistent with [TradingView's official SAR description](https://www.tradingview.com/support/solutions/43000502597-parabolic-sar-sar/).
That description notes the original method leaves initialization unspecified.
The exact choices above preserve the existing producer's private
`engine/regimeadaptive/indicators.go` PSAR convention at base
`08ee262ce2f653aff7c4f5f7579097e3d5435379`. That kernel and its fixed
0.018/0.018/0.20 caller are unchanged. The new public defaults are 0.02/0.02/0.20.
No external source code is copied.

[TA-Lib's official SAR implementation](https://github.com/TA-Lib/ta-lib/blob/main/src/ta_func/ta_SAR.c)
uses a directional-movement bootstrap and inclusive penetration checks, with
its own output/update timing. It is a different variant. Neither TA-Lib
numerical equivalence nor chart-platform bit parity is claimed.

## Hand-derived reference

For start=.25, step=.25, max=.5 the arithmetic below is exactly representable
in binary64. Candidate is before reversal and clamp; EP/AF are after the row.

| i | high | low | close | candidate | SAR | trend | EP | AF | flip |
| -: | -: | -: | -: | -: | -: | -: | -: | -: | -: |
| 0 | 10 | 8 | 9 | — | NaN | 0 | — | — | 0 |
| 1 | 12 | 10 | 11 | 9 | 8 | +1 | 12 | .25 | 0 |
| 2 | 14 | 12 | 13 | 9 | 8 | +1 | 14 | .5 | 0 |
| 3 | 16 | 14 | 15 | 11 | 10 | +1 | 16 | .5 | 0 |
| 4 | 15 | 9 | 10 | 13 | 16 | -1 | 9 | .25 | -1 |
| 5 | 13 | 7 | 8 | 14.25 | 16 | -1 | 7 | .5 | 0 |
| 6 | 10 | 6 | 7 | 11.5 | 15 | -1 | 6 | .5 | 0 |
| 7 | 18 | 16 | 17 | 10.5 | 6 | +1 | 18 | .25 | +1 |
| 8 | 19 | 17 | 18 | 9 | 6 | +1 | 19 | .5 | 0 |

The reflected fixture (high'=30-low, low'=30-high, close'=30-close) has
SAR'=30-SAR and reversed signs. Tests also cover a candidate touch (no reversal),
constant prices (equal-close falling seed, no flips), tied extremes (no AF
increase), gap reversals, initial reversal, invalid suffixes and every prefix.

In the invented named-level fixture, the low of 95 at index 5 starts a rising
segment. Its SAR after indices 5–9 is 95, 95, 95, 95.12, 95.2376. At signal
index 10 the candidate is approximately 95.352848; low=94 causes a bearish
reversal to SAR=101. Close is exactly 101. Bearish-trend and bearish-reversal
filters admit the existing long entry unchanged; bullish, close-above,
close-below, bullish-reversal and supports-entry filters reject it. This
separates a reversal event and direction from strict close/SAR equality.

## Input, route and execution contract

The reusable indicator validates H/L/C shape and finite H/L/C geometry
(low <= close <= high). It uses T only for length and does not inspect O or V.
Invalid options or shape make the whole result unavailable. An invalid row or
nonfinite arithmetic intermediate makes that row and the remaining suffix
unavailable, retaining the valid prefix. It never silently reseeds.

The public engine opts into stricter admission when `parabolicSAR` is present:
valid existing column shapes, finite OHLC/T, low <= open/close <= high, strictly
increasing timestamps, and complete six-value fixture rows. JSON null/dropped
rows are rejected. Existing volume rules still apply. A finite input whose SAR
recurrence overflows remains unavailable from that point; predicates fail closed.
These checks do not change malformed-input behavior for strategies without SAR.

Only the ordinary chart-timeframe broker families listed by
`dsl.sarBrokerFamilies` support this feature. Dedicated/legacy/sequential,
clock-range, timed-return, daily-flush, down-shock, weekend and intra-hour direct
runners reject SAR, including parameter-only requests. Explicit source-timeframe
configs and alternate entry timeframes are rejected. Native or fixture source
labels must be empty or equal the chart timeframe. This avoids ambiguous signal
clocks; no source aggregation contract is introduced here. Existing closed paper
prefix/bid-ask admission profiles also reject SAR; those independently reviewed
source profiles are not widened. Their internal prepared-broker reset helpers
preserve state correctly, without granting public profile admission.

Predicates run at closed signal-bar admission, before order capture, limit
placement or next-open queuing, including the ordinary SMA/dual-EMA direct
queue sites. Pending orders are not re-filtered using their fill bar's future
OHLC. Reversals do not cancel orders or exit positions. Warm-up/unavailable
values reject every authored predicate. Numeric parameters participate in the
shared-context cache identity; predicate-only changes reuse the same columns.

Native tests pin hand-derived values and causal behavior. A fixed-bit default-
parameter fixture additionally pins separately rounded decimal recurrence
values, derived using the existing independent rational RN-even proof helper:
SAR[3]=0x40207ae147ae147b, SAR[4]=0x4021694467381d7e and
SAR[6]=0x402fa3d70a3d70a4 on the nine-bar reference input. The same Go tests
run under actual Node-hosted Go/WASM, and bridge checks compare native/WASM
parser envelopes and run outputs against Go-generated corpus goldens. This is
runtime/conformance evidence, not browser qualification or trading performance.

## Existing-runtime preservation review

SAR uses immutable per-preparation state attached through one pointer at the
end of the private broker. The historical `contextcols.Columns`,
`contextcols.Options`, `flagParams`, and private regimeadaptive PSAR source are
byte-identical to the base above. Native layout comparison against the base
broker definition preserves all 94 existing field names, types and offsets;
size changes from 9,696 to 9,704 bytes on linux/amd64. Every reset clears the
pointer; ordinary prepared runs reattach their validated state explicitly.

The existing adaptive runtime constructs a broker with no SAR state and calls
its unchanged open/close routines. It does not reach the new ordinary admission
gates or reset helper. Its closed config/source admission does not accept SAR.
No SAR slices are allocated on that path. Output types and domains, N snapshots
and states, at most N orders/gaps, at most 4N events and at most 26 event links
remain unchanged. The independently reviewed 93,195,064-byte JSON allocation
bound at 4,096 retained rows remains valid; it was never a process/RSS limit.
Only the changed broker source fingerprint needs renewal, with the other seven
constructor fingerprints preserved.

Compile-only qualification with Go 1.22.12 confirms all seven pinned arithmetic
instruction streams are unchanged on each of linux/amd64, cross-compiled
linux/arm64 and js/wasm (21 total). The official extractor/verifier and old
instruction hashes are unchanged. Only reviewed production-source and build-
input hashes are renewed; sourceParents is preserved. No native ARM64 execution
is claimed. Combined-source qualification includes main
`d861478960fd3c5d8d55e34f158c506b5802be08`: its long-H1 native profile
implementation, limits, admission checks and source-inventory updates are
preserved. The source observation SHA-256 is
`bc092e5ed5e4a8977b135a45b1a1c17b077adb6f0a8a42db1bbde252acddf623`;
the old/new broker layout receipt SHA-256 is
`c28e2a78eb9557f4aa3e03d1bd8c7b5bae5fb2c74b030fc1910eb62d153a3ec5`.
Full compiler listings are retained losslessly as gzip in the task evidence.

Reproduce the committed source qualification with:

```sh
python3 scripts/checks/sequential-accounting-proof.py codegen "$PWD" "$(command -v go)"
python3 scripts/checks/sequential-accounting-proof-test.py
```

The official codegen command checks the actual native host and js/wasm. The
separate ARM64 cross-compilation comparison is additional evidence, not a
substitute for that command or for native ARM64 execution.
