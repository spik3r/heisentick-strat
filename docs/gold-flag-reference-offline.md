# HT-182: PR388 gold flag native offline reference

Canonical task: [HT-182](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-182-gold-flag-reference-dsl.md).

This is a strict Go-authoritative implementation of the fixed PR388 gold flag
recipe and its corrected deterministic stress scenario. It is not an observed
execution ledger or an established trading edge. The received simulator is
pinned at app commit `53fbf51c0aeda4c6be50933a92222ee4590e1142`, source blob
`dc39aa5e6d6fec7b70ac7ad58e0b1298f3965a39`. No JavaScript strategy runtime,
release, app registration/adoption, forward observer, deployment or orders are
included.

## Run boundary

Only `engine.RunGoldFlagReference` and `heisentick gold-flag-report` execute
this family. Generic report/grid, prepared/shared, prefix/checkpoint and engine
WASM routes fail closed. The parser WASM can compile the source; parsing does
not grant execution support. The complete strict grammar and immutable profile
are in [the family specification](../spec/dsl-spec-families/goldFlagReference.md).

```text
heisentick gold-flag-report --dsl-file=reference.strat --m15-file=owned-input.bin \
  --from=2020-01-01T00:00:00Z --to=2020-07-01T00:00:00Z --cost=0.06
```

Every flag is required exactly once. Unknown, repeated or missing flags fail.
The BTB1 input must have exactly six OHLCV columns and exact byte length. Source
rows must have unique strictly ordered exact UTC M15 opens, positive coherent
finite OHLC and finite nonnegative volume. Date boundaries must be exact UTC
RFC3339 ending `Z` and M30-aligned. Select observations in `[from,to)` before
aggregation and ATR initialization. There is no implicit pre-start warmup.
An empty selection fails. The end is an explicit selection watermark, not proof
that all intervening market slots were observed. Leading/trailing closure or
missing intervals are not synthesized.

Output has complete DSL/config/data SHA256 identities, the full fixed config,
M30/ATR/HTF snapshots, every detector candidate, admitted order states, event
intervals, gaps, partial-bucket metadata and terminal exposure. It is encoded
fully before stdout is written. Keep real inputs and outputs private; public
fixtures contain invented prices only.

## Fixed detector and clocks

M15 rows aggregate into UTC phase-zero M30 with first/max/min/last/sum. Partial
observed buckets remain in history; absent buckets are not inserted. H4 and
UTC-day bars aggregate observed M30 buckets. Their nominal availability is
bucket start plus duration, including partial observed HTF bars. A snapshot
uses only HTF bars with nominal close no later than the M30 decision close;
HTF finalization precedes an equal-time decision. This is not a provider
calendar or publication-latency certificate.

ATR14 is the source-specific Wilder calculation: omit TR0, average TR1..TR14
at index14, then `(previous*13+TR)/14`. The generic engine SMA ATR and v9's
TR0-inclusive initialization are intentionally not substituted. A six-row pole
precedes a six-row flag. Pole height is at least two signal ATR; flag range is
at most 0.6 pole and 1.5 ATR; retracement is at most half the pole. First tied
pole extrema determine ordering. Bull needs low before high, bear the reverse.
H4 uses twelve completed observed buckets and day uses five. Either qualifying
location suffices: long flag high at least prior high minus ATR, or short flag
low at most prior low plus ATR. This allows flags beyond the extreme. No volume,
directional-efficiency or normalized-ATR filter is present.

Signals consume cooldown before minimum-risk rejection. Skip the next six
observed M30 rows; the next candidate is eligible at signal+7. Place the stop
entry at the directional flag edge plus 0.1 ATR and the opposite stop at the
other edge minus 0.1 ATR, mirrored for shorts. Planned risk must be at least
0.4 ATR. The order becomes eligible on the next observed row and remains live
through signal+4 inclusive; expiry is that row's nominal close. Gaps do not
advance this observed-row clock. The separate two-hour timestamp is diagnostic
and never an expiry rule.

Fill-relative target is two times the distance from actual reference entry to
the fixed opposite stop. If no bracket exits, close at fill+24, after its
opening/intrabar brackets: the twenty-fifth observed holding candle. One
position suppresses new candidates through its exit row. No new order is
created on the last observed row. Keep unfinished pending/open states, with a
mark on open exposure; never fabricate terminal liquidation.

## Canonical broker reuse and stress policy

The engine reuses `broker.openPosition`, `resolveIntrabarExit`, `exitAfterBars`
and `closePosition`. The family adds pending-stop admission and evidence
projection. A private position opt-in gives a known opening target priority over
unordered later extremes. Its zero default preserves every prior broker family;
it is not a generic DSL switch. Unit quantity is a reference accounting device,
not allocation. No quantity or equity feedback affects the recipe.

For pending orders, an open beyond the opposite boundary cancels before later
extremes; an open at/beyond entry fills at that observed open. The initial
bracket is attached immediately. A later adverse opening stop fills at actual
open; an opening target fills at its boundary without price improvement. No
filled position is retroactively cancelled. The entry bar checks both brackets.
Unordered dual entry/cancellation selects entry-then-stop, while unordered dual
brackets select stop. These choices define `PR388_CAUSAL_STRESS_V1`; they are
not inferred tick paths. Local feasible alternatives remain attached to every
ambiguous scenario order, including cancellation and reachable targets.

The result's ambiguity metadata is explicitly at native M30 resolution. A
separate source-M15 evidence audit may narrow those alternatives without
replacing this fixed coarse scenario. Finer corroborating feeds cannot promote
an ordering without qualified source/quote/timestamp lineage. Local alternatives
are not propagated through later occupancy/candidates and are not portfolio
return or profit-factor bounds. Gaps and partial candles do not prove absence
of unobserved price events.

Known opening events are points `[t,t]` at the first actually observed M15
open, including a missing first constituent. Proven non-opening crossings use
`(first observed open,last observed constituent close)`, excluding both
endpoints. Close decisions use `[nominal close,nominal close]`. Events retain
ordered predecessor IDs; same-candle fill must precede its selected exit.
These coordinates do not measure publication latency or venue execution.
When comparing a saved zero-entry-gap record that cannot distinguish opening
equality from a later touch, its older `[observed open,envelope close)` interval
is a conservative enclosure; the source-price-aware runner may narrow it.

## Costs and qualification

The five supported per-fill price-point costs are `0`, `0.06`, `0.15`, `0.25`
and `0.50`. They are diagnostic deductions only. Gross R is directional price
change divided by actual entry-to-stop distance. Net R follows the audited
arithmetic `(direction*(exit-entry)-2*cost)/risk`. Costs do not change fills,
levels, risk, sizing, ordering or signal selection. No historical spread,
commission, liquidity, latency, financing or broker contract accuracy is
claimed. The CLI emits per-order outcomes, not an aggregate edge verdict.

Tests cover exact detector boundaries, ATR/HTF initialization and prefix
independence, mirrored order paths, partial/gap clocks, strict cooldown,
one-position occupancy, exit-row skipping, terminal states, costs and every
unsupported admission route. Existing corpus goldens must remain unchanged.
Private qualification compares full snapshots and exact order fields against
frozen owned evidence, stopping on unexplained differences. Such parity proves
an implementation of the scenario, not profitability or live readiness.
