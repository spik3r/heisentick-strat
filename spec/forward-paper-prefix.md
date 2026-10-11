# Go paper prefix, version 1

Canonical ownership: [HT-243](https://github.com/spik3r/heisentick-backlog/pull/491).
This additive API does not change `RunPrefix`, its resumable checkpoints or the
old `forward-prefix` command. It does not place external orders.

- Go: `RunPaperPrefix(PaperPrefixRequest) (PaperPrefixResult, error)`
- Decode strict transport: `DecodePaperPrefixRequest([]byte)`
- Native: `heisentick paper-prefix --request-file=<path>`
- Success: one JSON candidate containing the successor checkpoint
- Failure: fixed `PAPER_PREFIX_*` code, nonzero CLI exit, no success document

The request may come from an inherited, process-owned `/proc/self/fd/N` path.
The CLI opens and bounds the bytes; the consumer owns descriptor/inode and
executable authentication, timeout/cancellation and atomic durable publication.
A complete stdout write is still only a candidate until the consumer commits it.

## Public WASM transport

The engine WASM artifact exposes `engineRunPaperPrefix(requestJSONString)`.
It accepts exactly one primitive JavaScript string, containing the request JSON
described below. Success returns the exact producer-encoded result JSON string,
with no financial projection, endpoint liquidation, or added success envelope.
The bridge delegates unchanged to `DecodePaperPrefixRequest`, `RunPaperPrefix`
and `EncodePaperPrefixResult`.

The interactive WASM request has a smaller explicit **16 MiB UTF-8 byte limit**.
The native producer retains its 128 MiB input limit; producer output remains
bounded at 128 MiB. An early original UTF-16 code-unit length check bounds the
conversion work, then original UTF-16 validity is checked before Go string
conversion, followed by the actual UTF-8 byte check. Unpaired JavaScript
surrogates and unpaired JSON surrogate escapes both refuse, rather than being
silently repaired. No object coercion or implicit `JSON.stringify` is performed.

Failures return a JSON string of the exact form
`{"ok":false,"error":{"code":"PAPER_PREFIX_INPUT"}}`. Wrong arity/type,
including BigInt, returns `PAPER_PREFIX_ARGUMENTS`. A transport size overflow
returns `PAPER_PREFIX_INPUT_LIMIT`; invalid Unicode/JSON returns
`PAPER_PREFIX_INPUT`. Known typed producer error codes are preserved; unexpected
errors or adapter panics return `PAPER_PREFIX_INTERNAL`. No source, OS path,
exception text or stack is returned. A rejected call leaves the instance usable
for later calls and existing exports. Consumers own worker cancellation,
runtime capacity limits, artifact verification and durable publication.
Consumers must reject `ok:false`, require the complete successful schema and
binding/account/event/checkpoint document, and never convert a missing success
field to a zero/empty execution result. A failure has no success schema, account,
events or checkpoint and cannot be published as an accepted candidate.

The WASM result retains its actual `js/wasm` producer stamp. Binding, causal
position/event IDs, and open-position private checkpoint state digests are
therefore target-bound. Checkpoints are valid only for their unchanged producer
binding; native/WASM checkpoint migration is explicitly refused. Each target's
same-target replay/resume must reproduce its full exact public result. The
public parity check compares all financial/event/account/pending fields exactly,
independently validates target-bound identities, and checks opaque checkpoint
replay/tamper refusal without pretending those receipts are portable.

The export uses the same producer family/option admission as the native command;
transport does not independently widen it or admit HTF/source-entry. Endpoint is still open. The public-export
proof is actual native CLI versus instantiated Go WASM under Node, including
nonzero fees, partial/open/pending states, activation and recovery. This is
separate from Go tests merely compiled to WASM, browser qualification, tagged
assets, consumer pin/adoption, and operational availability.

## Request

The schema is `heisentick-paper-prefix-request-v1`. Required exact keys:

- `source`: exact UTF-8 DSL source, compiled only by Go
- `route`: `strategyId`, `symbol`, `timeframe`, `rangeMethod`
- `units`: `quoteCurrency`, `priceUnit`, `quantityUnit`, `multiplier`
- `costs`: `startEquity`, `feePerUnit`, `fillOn`, `slippage`, `slippageBps`
- `activationFromT`, `closedThroughT`: exact integer Unix milliseconds
- `bars`: OHLCV rows `[T,O,H,L,C,V]`
- `checkpoint`: optional opaque returned checkpoint; omitted/null means fresh

`schema` is also required. Unknown/case-altered/duplicate keys, missing or null
required values, invalid Unicode including unpaired JSON surrogate escapes,
wrong types and nonfinite values refuse. Costs do not acquire legacy defaults:
startEquity is positive; fees and both slippage components are nonnegative.
FillOn is exactly `close` or `nextOpen`. Slippage uses the existing adverse rule:
absolute slippage plus absolute reference price times Bps divided by 10,000.
FeePerUnit is a fee per quantity unit on each executed side.

Supported fixed timeframes: 1m, 5m, 15m, 30m, 1h, 4h, 1d. T means the candle's
OPEN, not its close. Every bar must have closed by the caller's `closedThroughT`:
T + declared timeframe duration <= watermark. T is nonnegative, exactly
representable in binary64 milliseconds; rows strictly increase, gaps are whole
multiples of the declared duration, OHLC bounds are coherent, volume is finite
and nonnegative. This caller assertion does not establish genuine feed data,
market-calendar correctness, or that a still-forming candle really closed.

Units use the existing abstract linear multiplier-one broker arithmetic.
Multiplier must equal one. The three nonempty labels are caller-declared
provenance, not verified instrument/contract mapping or currency conversion.
Source risk/sizing and declared routes stay Go-owned; route overrides and
arbitrary caller config/capability flags are not accepted.

## Capability boundary

This version admits ordinary openingRangeBreakout, faithful legacySetup9 and the
bounded VWAP-extension-fade class below, without source-entry or HTF requirements.
Other families and required HTF/source
context are explicit refusals. HTF duration inference and gap projection need a
separate causal prefix-join qualification. They are never silently omitted.

ORB uses its existing close or next-open fills. The archived legacySetup9
profile intentionally opens at the signal close; requesting nextOpen refuses.
Changing that would be a new strategy semantic profile, not an option fix.
Full Sequential E1/E2 is outside this API, as are special/dedicated families.
No successful parse, source inspection or caller flag widens this boundary.

### VWAP-extension-fade paper option class

VEF requires close fill, 15m, zone range context, USD quote currency and the same
abstract linear multiplier-one accounting. Symbol/timeframe must satisfy the
authored Go route. Symbol, unit labels and caller watermarks are assertions, not
verified instrument/provider facts. There is no strategy-ID or approved-source
hash whitelist. Renamed/independently authored sources in the qualified class are
admitted with their own exact identities; no research result is required.

The class includes both sides, named sessions/phases, current-day regime and
movement-ER gates, distance from current close-volume VWAP, range-day requirement,
tail/close quality, recent-extreme ATR-buffered stops with min/max bounds, VWAP
target and existing fixed-R fallback/minimum, fixed positive USD risk, optional
close-triggered breakeven and observed-bar cooldown/max-hold. Zero lookback keeps
the ordinary current-bar-extreme behavior. Bar counts must be integral,
nonnegative and no greater than the prefix bar limit. Other thresholds must be
finite and within their stated domains; quality fractions are in [0,1], risk,
VWAP distance and target R values positive, and stop min/max coherent. Selected research fixture
thresholds are unchanged, including risk50, 3ATR, 0.25R breakeven, 24-bar hold and
12-bar cooldown. These are fixture inputs, not admission requirements.

Admission checks the complete parsed config, authored logical directives and
source-coverage issues from the exact ordinary parser lexical pipeline. The
additive Go `AuditOrdinarySource` view exposes dropped inline-leading text,
trailing text after accepted inline closing braces, and residual structural
braces that normalization would erase. Any coverage issue refuses. The ordinary
nil-audit parser uses the same implementations and retains its existing results;
this paper-only refusal policy does not repair or change source-language parsing. This matters because a final
config can erase ignored or overwritten source clauses. Unsupported clauses
refuse with `PAPER_PREFIX_ADMISSION`, even if their final config equals
a default. Go Parse remains the sole config authority and its result is unchanged.
This paper guard accepts a bounded set of complete authored forms, including the
unchanged fixture and its listed aliases: quoted metadata, `symbols`/`timeframes`
or `slices`, named `sessions`, `session phase in`, `day type in` with optional
`strictly`, `movement below`/`trendiness below`, `type: vwap extension fade`,
`distance from vwap at least N ATR`, `require range day`, `target vwap`,
`side both/long/short` with optional `only`, `tail rejection at least N`,
`close location at least N`, recent-extreme stop with `by N ATR`, stop-size
`<= N ATR` or `between N and N ATR`, `target N R`, `minimum reward N R`,
`move stop to breakeven after N R plus N ATR`, `breakeven off`,
`trail off`, `maxHoldCandles`, `wait N candles after trade`, and
`risk N USD`. Case/spacing follows the existing parser. Unsupported authored
forms are explicit refusals; a later directive cannot erase their admission
failure. A single family type must precede setup/risk/target/management clauses.

NextOpen, source/HTF dependencies, enabled partial/trailing management, limit
entry, alternate stop/target implementations, grading/size tiers and unqualified
prior-day/seasonality/session-bias/RMV/level/context filters remain refused.
Existing VEF does not execute partials or generic trailing; their broker observer
hooks do not supply those absent family semantics. Quote-side/bid-ask execution,
venue costs, rollover and live broker stop updates are separate contracts.

### Opening-range-breakout paper option class

ORB admission checks source coverage with Go `AuditOrdinarySource`, every authored
logical directive, and every effective config leaf before `PrepareRun`. Any
coverage issue, ignored/unsupported clause, unknown nested field, missing required
field, invalid value or unsupported family switch refuses with
`PAPER_PREFIX_ADMISSION`. The sole config authority remains ordinary Go Parse;
this family-local guard does not introduce another parser or change its results.
Source and config are both necessary: a later off, reset or equivalent value
cannot erase an unsupported occurrence. No source hash, strategy identity,
research approval or profitability test grants capability.

The supported matrix follows the actual ordinary ORB and shared broker consumers:

| Authored options | Admitted config leaves / execution |
| --- | --- |
| DSL v6/v7, quoted strategy/name/description, symbols/timeframes/slices | Metadata and declared routes; seven fixed timeframes above; the request must satisfy the compiled route |
| `type: opening range breakout`; first N candles/bars/minutes, every N hours UTC, `break beyond [range] edge [by/buffer N ATR]`, hold N candles/bars | `openingRangeBreakout.firstCandles/firstMinutes/utcSlotMinutes/openingSessions/breakBufferAtr/holdCandles`; closed-prefix range formation, existing buffer and held-close test |
| Sessions, trade window unrestricted/in segments/minutes/UTC hours, side/direction | Shared session flags, exact window mode/segments/bounds, `allowLong/allowShort`; named windows use the existing fixed UTC+10 clock, UTC slots use the existing epoch-aligned clock |
| Session phase, day type with strict/ER escape, movement/trendiness, local weekday/hour allow/block | `sessionPhases`, `dayTypes/dayTypeErEscape/maxMovementEr`, local/blocked weekdays/hours; supplied same-timeframe causal context |
| Open location, prior-day type allow/block and range minimum, day theme | `openLocations`, prior/blocked prior types, `priorDay.minRangeAtr`, `dayThemes`; existing completed-day/current-prefix gates |
| Tail rejection, close location, RMV ATR period/lookback/comparison | `tailRejectionMin`, `closeLocationMin`, exact `relativeMeasuredVolatility` fields; unavailable context blocks the entry |
| Ordinary seasonality intraday/day-of-week/week-of-year/monthly, all or finite-day lookback, classifications/supports-entry, min samples | Exact `seasonalityFilters` row shapes; existing strictly prior-bar classification, including unavailable/min-sample behavior. This is distinct from the unchanged archived legacy seasonal profile |
| Opposite-range-edge stop with padding/min/max, stop size bounds, fixed ATR/pips | Exact `stop.type/atr/pips/paddingAtr/minAtr/maxAtr`; fixed pips require the existing reviewed instrument pip mapping |
| Target N R, N range, N pips, positive USD risk | `target.orbR/orbRange/pips`, `riskUsd`; ordinary pip then positive-range then R target precedence |
| Move stop to breakeven / off / existing v6 ATR spelling, partial half/quarter/fraction/percent at R with optional breakeven, trail ATR after R / off | Exact `breakeven`, `partial`, and `trail` leaves; the working ordinary close-triggered management remains admitted, including partial fees and current open protection |
| Wait N candles/bars after trade, maxHoldCandles (v6 maxHoldBars alias) | `cooldownCandles`, `maxHoldCandles`; existing rounded cooldown and fractional observed-bar hold semantics are preserved |
| Explicit higher-timeframe off / entryTf current | Neutral context only; enabled authored HTF/source-entry intent refuses even if later disabled |

Complete forms and aliases, including inline/comment/order cases, are exercised
in `paper_prefix_orb_test.go`; invented public execution fixtures live in
`engine/testdata/paper_orb/cases.json`. Quoted metadata is bounded to its complete
value, so trailing unconsumed text cannot hide behind a name. Family-dependent
setup, stop, target and management forms require an earlier ORB selector;
repeated ORB selection retains the ordinary reset behavior. Generic market
clauses may precede the selector. Known valid overrides retain ordinary precedence.

All numeric occurrences must be finite before later overrides. Range and hold
counts are positive integral values below 2^31; UTC slot hours remain 1–24.
Cooldown stays nonnegative and below 2^31−1. Risk, fixed stops and R/pip targets
are positive, quality fractions are within [0,1], stop bounds are coherent,
partial fractions/triggers are positive, and RMV/seasonality retain their exact
parser domains and field shapes. Fractional max-hold thresholds remain valid.
No arbitrary maximum equal to a selected research fixture is imposed.

Refused authored options include target opposite-range-edge/minimum/fallback,
retest (including off), New York hour filters, source `range method`, bare
`break buffer`, source/HTF dependencies, alternate family selectors, limit/entry
mode or distance, recent-extreme stop lookback, trigger/candle/grade/size-tier,
levels and any other unconsumed context or unknown directive. Some ordinary
parser forms map to defaults or are ignored: successful parsing is not evidence
that ORB executes them. Retest runtime parameters exist but the ordinary ORB
retest parser is not wired; implementing the documented retest language and its
causal initial-breakout/retest behavior is a separately owned follow-up, not a
removal of working support in this guard.

ORB range slots close a carried position at the next observed slot bar's OPEN,
before bracket processing. Partials, breakeven and generic trailing retain their
existing ordering and arithmetic. Every invented positive corpus case compares
complete native/public-WASM output and all prefix splits; paired gate cases,
hand management expectations and public refusal recovery prevent admission
checks from substituting for execution evidence. The internal occurrence receipt
retains partial/trailing and typed context categories for narrower future models;
it grants no new public execution contract and changes no checkpoint shape.

## Activation and endpoint

Activation is immutable and inclusive at an exact bar-open timestamp. A prefix
ending entirely before activation is a neutral flat account with no events.
Once the prefix reaches/passes activation, that exact timestamp must exist;
missing boundaries refuse rather than round over a gap.

`PrepareRun` computes causal indicators and the legacy counter over all context
bars. The unchanged broker execution loop starts at the activation index. ORB
ranges and legacy counters do not need historical broker mutation; a historical
ORB breakout therefore cannot consume the later live daily/slot opportunity.
No historical order, pending entry, position or fee crosses activation. VEF
indicators are also computed from context bars, while historical broker seen and
cooldown mutation is suppressed. A fresh active setup can reuse a key that would
have been consumed by a preactivation trade. This qualification is specific to
these families, not a generic warm-up assumption.

VEF preserves ordinary source behavior: cooldown is from the last qualifying
setup attempt, including an attempt rejected by shared entry gates, not its exit.
Its seen key is side, phase and rounded VWAP tenth. Seen keys reset lazily on the
next qualifying attempt in a new fixed UTC+10 day; VWAP resets at UTC midnight.
ATR uses a 14-observed-bar simple mean with partial seeding, not a mandatory
14-bar warm-up. The existing ordinary source projection has unavailable context
for a one-row prefix because duration cannot yet be inferred. Once another row
exists, the first row's VWAP equals its close, so the strictly positive admitted
distance prevents a retroactive first-row entry. The selected close phase intersects trade windows at UTC08–09
and UTC11–14; these names are not a DST-adjusted venue close.

Intrabar exits precede management. Breakeven uses completed close and CURRENT SL
distance and applies its tightened stop to subsequent bars. Wick-only progress
does not qualify; repeated current-SL/ATR updates may tighten again. Carried stop
gaps fill at open, unordered stop/target bars select stop first, and close entries
are not retrospectively tested against their own signal-bar extremes. Time exit
uses observed bar-index difference, so gaps can extend elapsed holding time.

The final supplied bar is marked after execution. There is no synthetic
end-of-test liquidation. Actual pending entries/exits and open positions remain.

## Success, binding and identity

Schema `heisentick-paper-prefix-v1`, mode `replay-verified`. `binding` echoes:

- `contract`
- `source`: bare lowercase SHA256 and UTF8 byte length
- `configIdentity`: whole canonical Go config SHA256 and encoding name
- exact `route`, `units`, `costs`, `activationFromT`
- `producer`: actual available Go version, GOOS/GOARCH, module version, and
  nullable VCS revision/modified stamps

`bindingDigest` hashes that object using Go encoding/json. All new digests and
IDs are bare lowercase 64-character hex. Producer stamps are not binary
attestation; the app must verify exact released executable asset bytes. Binding
includes the target architecture because existing broker arithmetic has a
reviewed platform-specific fusion schedule. Cross-build/target checkpoint
migration is not implicitly supported.

The result also includes `processedBars`, `lastBarT` (bar-open), `closedThroughT`,
`inputDigest` over rows plus close watermark, `eventRange`, `events`, `account`,
`completedTrades`, `openPositions`, `pending`, and embedded `checkpoint`.
Pending exposes market/limit/exit counts and an opaque state digest.

Position IDs bind immutable identity and causal entry index, timestamp, side and
within-bar ordinal. Event IDs bind immutable identity, kind, position identity,
bar index/timestamp and within-bar ordinal. Future bars, full inputDigest and
checkpoint digests never enter earlier IDs. The app scopes these IDs by its
principal/run namespace; it never substitutes one position ID for every effect.

Events are `entry`, `partial`, `exit` and `mark`. Their IDs, index, original bar T,
within-bar ordinal and actual realizedBefore/After are explicit. Execution effects
carry side, fill price, size, nominal fee, nullable stop/target, and exit credit
on exits. `completedNetPnl` appears only when the logical position fully closes.
Mark events carry the authoritative post-action account snapshot. There is one
mark per active bar. Same-bar transitions preserve actual broker ordering.

## Account and completed-trade definitions

`realizedNet` is the actual broker accumulator, including entry and exit fees.
It is not reconstructed by summing legacy Trade.PnL. Legacy trade PnL is exit
credit and excludes the original entry fee. At each active bar:

1. mark remaining quantity at the admitted close, with no hypothetical exit fee
2. cash = starting equity + actual broker realized
3. equity = cash + open unrealized
4. compare against the initial-equity-based peak, then update drawdown maxima

Observer-only operations explicitly round to binary64; existing broker financial
expressions remain unchanged. Account includes startEquity, cash/equity,
realized/unrealized, cumulative entry/exit/total nominal fees, peak equity,
current/max money drawdown and current/max drawdown fractions. A fraction of
0.1 means 10%; equity below zero can produce drawdown above 1.

One logical trade spans entry, any partial exits, and final flatten. Accumulate
its actual raw exit credits, then subtract its original entry fee exactly once.
There is no invented partial entry-fee allocation. Only final flatten increments
count/wins/losses/breakeven and gross win/loss amounts. GrossLosses is positive.
An open position affects MTM but is absent from completed-trade statistics.

WinRate is wins/count, null for zero completed trades. ProfitFactor is gross wins
/ gross losses when losses are positive, including exactly zero for loser-only
runs. Otherwise it is null, with status `no-trades` or `no-losses`; a defined
ratio has status `defined`. Breakeven-only is `no-losses`. No Infinity.

Floating-point rounding means account realized need not equal a subtraction of
large equity values, nominal fee sums or regrouped completed-trade nets. The
actual mutation chain and broker accumulator are authoritative. All reported
numbers must be finite; overflow refuses the whole candidate.

## Replay checkpoint and retry

Checkpoint schema `heisentick-paper-replay-checkpoint-v1` binds immutable identity,
input rows/close watermark, processed count, last bar, total event count and a
digest of all required broker/account/logical-trade accumulator state. It is an
opaque deterministic replay receipt, not trusted caller-authored mutable state.

On a checkpoint request, replay the exact acknowledged prefix and compare the
entire recomputed receipt. Only then replay the full extended input and return
events from the acknowledged total through the new total. `eventRange.from` is
the previous total, `to` the new total, and events.length equals to minus from.
Account/open/pending/statistics are cumulative current snapshots.

`openPositions` is the authoritative end-of-prefix protection snapshot, including
current SL/TP and immutable initial protection. Consumers atomically replace it
with account/checkpoint/cursor, even when the delta contains only a mark. Entry
and exit effects carry protection at execution time; marks do not constitute
protection-change events. No complete historical stop-change event stream or
broker protection update is claimed. VEF replay receipts additionally bind
last-attempt index/presence and the seen local day/ordered keys. These are
read-only projections into the replay digest; no caller state is restored. The
optional VEF projection is absent for ORB/legacy, preserving their receipt bytes.

Exact input plus its returned checkpoint gives zero events and the identical
checkpoint. Reusing an older checkpoint with the same extended input reproduces
the exact prior delta, allowing crash retry. Changed historical rows, immutable
binding, old schema, malformed receipt or invented state digest refuse. Hashes
are integrity/binding checks, not signatures; principal authorization and durable
checkpoint authenticity belong to the app. No prior mutable state is restored.

## Resource and verification limits

Source <=8 MiB, chart prefix <=500,000 bars, input/output <=128 MiB, effects
<=2,000,000. Any excess refuses; no truncation or omitted mark history. App
transport may use smaller explicit bounds. Work is full-prefix context/replay,
not O(1) streaming; a restart can perform two complete replays. Capacity must be
measured for each consumer's operational limits. No indefinite-history claim.

Tests use invented local bars, hand-derived account expectations, both supported
fill modes, partial exits, causal activation, every meaningful split and corrupt
receipts. Existing conformance and frozen Sequential outputs must remain exact.
Additive observer calls alter six whole-function instruction pins even though
financial expressions are unchanged. Their adoption requires independent
full-function/financial-subsequence review and actual ARM64, AMD64 and WASM proof;
all other fifteen pins remain exact. Source implementation alone is not release
or arithmetic qualification. Release/checksums, consumer pin, authenticated
admission and durable lifecycle E2E remain required before user availability.

The VEF admission/receipt addition itself changes no broker expression, observer
call or arithmetic instruction pin. Its four source/helper inventory changes
and target dependency closures require review against the frozen parent; all 21
previously qualified instruction maps must remain byte-identical.

The ORB source-admission addition changes no broker expression or arithmetic
instruction pin. Only source/build inventories are extended; all 21 qualified
instruction maps and existing conformance goldens remain unchanged.
