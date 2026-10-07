# HT-175: Regime Engine native offline interpretation

Canonical task: [HT-175](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-175-regime-engine-dsl-historical-test.md).

This is a fixed Go interpretation of the reviewed v9 Regime Engine, not a
TradingView compiler or parity claim. It is separate from the original KitKat
level strategy and from any newer, uncompiled audit-baseline draft. No parameter
search, browser/app adoption, production execution, tag or consumer pin is part
of this change. A minor release and explicit adoption would be separate work.

## Runnable boundary

`dsl.Parse` compiles `type: regime engine`. Only `engine/regime.Run` and
`heisentick regime-report` execute it. Generic Run/PrepareRun/shared/grid,
report, prefix/checkpoint and engine-WASM APIs reject the reserved family with
`regime-engine-native-dedicated-runner-required`. A parser success does not mean
that the app, engine WASM or ordinary broker can run it. No existing family is
relabelled runnable. Existing parse/run conformance goldens are preserved;
new parse cases and invented dedicated-runner tests are identified separately.

The generic broker would change the strategy's both-touch ordering, delayed
protection, notional sizing and retained terminal exposure. The dedicated Go
runner therefore owns all execution semantics for this fixed offline study.
There is no independent JavaScript strategy implementation.

## DSL

```text
dsl v7
strategy "Regime Engine v9 source interpretation" {
  description "Native offline reference model; Pine parity unverified"
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

The other supported mode is `audit-baseline-v1`. All other strategy parameters
are fixed in the closed `v9-floor-half-v1` profile; altering them is an error,
not a silently ignored option. The compiled config includes the complete profile.
The exact v9 supplied Pine identity is SHA-256
`1db4948c0698dc6d1b621a29b46225f27f8d01227e87aa1c4131e8eda33d7810`.

## Signals

- UTC M30 bars aggregated from observed M5 OHLCV. No interpolation. Exactly six
  unique M5 grid slots makes a complete bar; partial closed bars remain in the
  indicator history. Eligibility requires the current and previous 39 observed
  native bars complete. Session/weekend gaps between full bars do not reset it.
- HMA9 crosses HMA21 strictly at the completed native close, with prior equality
  counted. Odd halves use floor; square-root lengths round half up. HMA25 is
  diagnostic only. These conventions interpret potentially ambiguous supplied
  Pine arithmetic, not a verified Pine compilation.
- Same-bar Supertrend direction must agree: Wilder ATR10, factor3, initial
  bearish state and strict band crossings. Volume must strictly exceed its
  current-inclusive SMA20. There is no delayed signal/gate memory.
- Exit ATR is Wilder ATR14, seeded with the first 14 true ranges. First-bar TR
  is high-low. There are no original KitKat levels, sessions or extra filters.

## Two different execution policies

`source-like-v1` preserves the v9 reference interpretation. Signal close is the
anchor. A flat signal enters at the next observed native open; the entire entry
bar has no protective bracket. At its close, stop becomes max(anchor-2*ATR, ST)
for long or min(anchor+2*ATR, ST) for short; target is anchor ±3.5*currentATR.
Subsequent completed closes recompute both, so stop widening and target movement
are allowed and recorded. A newly wrong-side stop activates a market exit at
the next observed open, even if that opening rebounds beyond the stop. There
is no explicit regime-flip exit. A newly marketable target is not latched: the
regular limit is re-evaluated at the next open/path, matching the previous
reference model. This target-activation convention is not certified Pine parity;
the report counts such edits explicitly.

`audit-baseline-v1` is a declared different exit policy with the same signals.
At signal, freeze ATR and initial distance min(2*ATR, valid positive structural
distance to ST), falling back to 2*ATR. Translate that distance from the actual
entry fill. Attach the bracket immediately, including entry-bar protection.
Freeze the target at entry ±3.5*signalATR. Ratchet only toward a valid same-regime
ST line; a fresh adverse completed-bar regime flip schedules a next-open exit.
Do not confuse this with a later audit draft or a faithful rewrite of the source.

## Chronology, paths, costs and missing data

1. Fill a pending entry at the first actually observed M5 open in the next
   native bucket. Retain actual fill timestamp, bucket timestamp and native
   close separately. Do not backdate fills if a bucket's opening M5 is missing.
2. Execute previously forced exits before barrier traversal. Opening stop gaps
   precede target gaps; both fill at the opening price, including favorable
   limit gaps. Traverse O-H-L-C if high is strictly nearer open, else O-L-H-C;
   ties are low-first. Intrabar hit timestamps represent the native close of
   the interval, not an observed trade tick.
3. Calculate signals and surviving-position amendments only after the native
   close. Never use future completeness to cancel a pending entry. The prior
   Python model's partial-entry cancellation was retrospective; this causal
   correction is disclosed and partial entry/traversal counts are emitted.

The fixed study uses equity10000, quantity=pre-entry-fee equity ×0.10 / actual
entry, pointvalue1, fee0.50/unit/side, and spread0 or1. Ten percent is notional
allocation, not risk. Quantity is continuous; one unit=one ounce is an
illustrative mapping, not verified broker contract identity. Spread stress
conditionally treats reference OHLC as bid-style: long entry pays spread,
short liquidation path pays spread. Actual feed quote side/provider/volume
units remain unverified. No slippage, financing, swaps, lot/tick rounding or
other contract costs are included. `costComplete` and `pineParityVerified` are
always false.

## Boundaries and terminal state

All endpoints are explicit UTC M30 boundaries. The context must begin with an
observed M5 opening at `warmup-from`; the M5 bar ending exactly at `trade-to`
must exist, establishing the declared complete-history watermark. An unclosed
or short tail is refused, not silently treated as complete history. Other
partial-but-closed buckets remain in the study. This is a bounded historical
API, not a resumable live-prefix API.

Start flat. Native candles opening before `trade-from` supply indicator context
only; even a signal at the close equal to `trade-from` cannot enter. A final
native candle whose close equals `trade-to` resolves its path and close-time
management, but cannot create a new signal. No quote at/after `trade-to` is
consumed. Later input bars cannot alter fixed-cutoff indicators or execution;
the CLI still hashes the complete supplied byte snapshot for provenance.

Final exposure stays open; there is no invented end-of-test close. The report
separates closed P&L/PF/closed-equity drawdown from cash after any open entry fee,
unrealized gross mark and a separately labelled hypothetical closing fee.
The closed drawdown is not floating/path drawdown. Undefined PF is JSON null,
not infinity. Native indicators, candidate/accepted signals, closed trades,
order edits and final position/order state are retained.

## Reproduction

Build with Go1.22 or newer and use the owned M5 BTB1 file (six OHLCV columns).
This command does not acquire data or require a TradingView export:

```sh
go build -o heisentick ./cmd/heisentick
./heisentick regime-report \
  --dsl-file=source-like.strat --m5-file=owned-5m.bin \
  --warmup-from=2025-12-01T00:00:00Z \
  --trade-from=2026-01-02T00:00:00Z \
  --trade-to=2026-10-01T00:00:00Z --spread=0 > source-like-spread0.json
```

The frozen comparison is exactly two modes × two spreads (0 and1), on the same
window. Its private output includes DSL/config/full-data SHA-256 identities;
raw market data and historical outputs do not belong in this repository.
Parameters and costs are frozen before results. Comparison to the independently
audited Python interpretation is per indicator/signal/trade/exposure, not just
headline performance. Neither matching nor profitability proves TradingView or
broker parity. Existing v9 source and baseline portfolios are compared as their
own simulations, not treated as identical trades under different exits.

## Qualification

Hand-derived tests cover HMA floor halves, Wilder seed/gaps, strict Supertrend
bands, current-inclusive volume, complete-bar admission, source delayed bracket,
audit frozen/ratchet/flip semantics, both sides and path ties, gap and activated
stop priority, conditional spread and both fees, missing-open timestamps,
marketable-target edits, terminal cash reconciliation and data refusal. A
deterministic invented OHLCV path runs through the full public pipeline with
actual synthetic trades and proves future-mutation/append invariance. Generic
native/CLI and fixture/column WASM refusal tests protect the execution boundary.
No old golden is regenerated to hide a failure. Run `go vet ./...`,
`go test ./...`, `go run ./cmd/conformance check` and both WASM builds before
publication; independent semantic review is required before historical runs.
