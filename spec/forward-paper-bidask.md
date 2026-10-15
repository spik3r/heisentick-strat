# Sampled bid/ask paper contract v1

This is an independently reviewed Go execution model. The native CLI is
`heisentick paper-bidask --request-file=<regular file>`. It reads one bounded JSON
request and returns one complete candidate success or safe refusal envelope.
It has no network, feed, broker-order, authentication, account-balance lookup,
consumer activation or release authority. Existing paper-prefix, old sessions,
conformance and existing public WASM APIs are unchanged.

The file command's qualified native hosts are Linux AMD64 and ARM64. Its secure
native IO implementation is excluded from Go's js/wasm command-package build.
On js/wasm, invoking that command always returns the fixed refusal
`PAPER_BIDASK_NATIVE_COMMAND_UNAVAILABLE`, without interpreting arguments,
opening files or executing the model. There is no filesystem fallback or empty
success. Writer errors and short writes remain `PAPER_BIDASK_OUTPUT` failures.
This compatibility stub keeps existing command-package WASM tests available;
it does not expose a public bid/ask WASM model API. Other host qualification and
the separately owned public Go/WASM bridge require their own evidence.

The model is `sampled-straddle-full-bar-stop-first-v1`, request schema
`heisentick-paper-bidask-request-v1`, result schema `heisentick-paper-bidask-v1`.
A success has `mode: replay-verified-candidate`. A refusal has exactly `schema`,
`status: refused`, and a fixed `PAPER_BIDASK_*` code. Both are complete JSON
stdout documents; the consumer must check the envelope, not just process exit.
Filesystem paths, source text, raw account identities and provider bodies never
appear in refusals. Subprocess/output/commit failure never advances a consumer
cursor. There is no fallback financial computation.

## Public WASM transport

`engineRunPaperBidAsk(requestJSON)` exposes the same Go model from
`enginewasm.wasm`. It requires exactly one primitive JavaScript string and
returns one complete JSON string. The bridge calls the unchanged Go
`DecodePaperBidAskRequest`, `RunPaperBidAsk` and `EncodePaperBidAskResult` APIs;
it does not parse DSL, compute financial values, apply effects, repair input,
choose defaults, reorder records or introduce execution policy.

Success is exactly the Go encoder's document on that target. Refusal has exactly
`schema`, `status: refused` and a fixed `PAPER_BIDASK_*` code, matching the native
command rather than the older paper-prefix transport's different error shape.
Wrong arity or nonprimitive arguments produce `PAPER_BIDASK_ARGUMENTS`; invalid
Unicode, malformed JSON and oversized input produce `PAPER_BIDASK_INPUT`.
Known typed producer refusals retain their codes; unexpected errors or panics
produce `PAPER_BIDASK_INTERNAL`, without exception/source/path disclosure.
An ordinary refusal is a successful transport invocation, not financial success.

Original UTF-16 is checked before conversion can replace an unpaired surrogate.
The original code-unit count and actual UTF-8 bytes are bounded; exact 16 MiB
input is allowed, with larger input refused. The Go decoder owns strict JSON
shape, duplicate keys, escaped Unicode, depth and all model limits. A production
JavaScript wrapper must pass raw JSON through without parsing and reserializing
it, and must not convert decimal or nanosecond strings to numbers. All native
input/output, source, record, bar, event, quote, tier and receipt limits below
remain unchanged. Finite producer limits are not browser performance promises.
Existing consumer worker limits (2 MiB input, 8 MiB result, 15 seconds and
10000 bars) remain separate and are not expanded by this export.

Producer platform identity is truthful: native and js/wasm have different
GOOS/GOARCH, which propagate into binding and causal IDs and replay receipts.
Receipts are verified on their originating target; moving a receipt between
targets is not supported. Same-target repeated prefixes emit zero new effects.
The public transport does not authenticate declared artifact/provenance facts,
resolve episode races, apply a durable transaction, or provide session isolation,
fencing, Stop/restart, consumer activation or release approval.

The actual public-export qualification compares complete native/WASM financial,
clock, episode and protection payloads exactly, independently checks enumerated
target-bound IDs and validates target-local receipt/replay behavior. Same-target
Go tests compare the full unmodified bridge document with the public library API.
Transport tests reject malformed/oversized inputs and require later successful
calls and all prior exports to remain usable. Test-side JSON/hash handling is
not a second financial or DSL engine.

## Supported source and authority

The initial matrix is ordinary single-timeframe ORB market entries with source
absolute or existing entry-fill-distance brackets, ordinary Go breakeven
(including defaults), MaxHoldBars, UTC-slot opening closure and preserved-open
endpoints. Causal same-timeframe gates derived from supplied midpoint candles
retain their ordinary Go behavior. HTF/source-entry, dedicated contexts,
nonmarket/no-stop/no-target orders, partial exits, generic trailing and other
families are unqualified. Seasonal legacy paper qualification remains a distinct
unavailable capability, not a generic parser refusal.

The native public entry consumes the independently reviewed shared authored-
directive/coverage helper through the ORB authored/effective-config guard, with
the exact source and unmodified successful ordinary Go Parse config. A successful
Go parse alone does not qualify ignored authored intent. Any enabled authored
partial or trailing instruction is refused even if subsequently overwritten.
No source-hash/research whitelist, private approval list or JavaScript parser is
part of source admission. The guard rejects uncovered authored text and ignored
clauses before model execution. Its occurrence receipt further excludes partial,
trailing, HTF and source-entry features in this model. Effective configuration
checks remain a second defense. Source refusal is `ADMISSION`, route mismatch is
`ROUTE_UNSUPPORTED`, and otherwise qualified authored partial/trailing is
`PARTIAL_UNQUALIFIED` / `TRAILING_UNQUALIFIED` (all with the fixed prefix above).
All retained same-timeframe context categories use ordinary causal Go columns:
session phase, day/movement, local weekday/hour, open location, prior day, day
theme, RMV and ordinary seasonality. There is no alternative context authority.

The runner uses the actual flat ORB `onBar`/`captureEntries` seam, one bar at a
time. Warmup builds causal columns but never invokes actionable handlers. Seen
and cooldown mutations survive every expiry, invalid geometry and no-fill.
It cannot advance to another family bar while a captured original episode is
unresolved. There is one broker protection position and one dedicated financial
collector. No legacy midpoint entry, close or accounting collector is called.

The accepted narrower 13-path frontier does not edit broker.go. Held management
calls unchanged `moveStopToBreakeven`. Only the exact existing MaxHoldBars and
slotEnd predicates are restated as side-price model dispatch; exhaustive tests
compare them with the original helpers. All 21 prior instruction pins remain
frozen. The next explicitly separate expansion is partial-exit and generic-
trailing management, including partial fees and logical-trade completion.
That work must return for design/ownership review rather than grow a copied
strategy-management implementation.

## Input binding and observations

`source`, Go config digest, route/storage strategy identity, principal/account/
run/epoch digests, server-verified artifact digest, producer identity, family/profile,
signal recipe, sizing policy, activation, product, economics and policy
are immutable bindings. Future records do not change earlier decision, position
or event IDs. A digest is an identifier, not authentication or evidence of a
successful database transaction. The server owns authentic source/provenance
and journal facts, including verification of the supplied artifact digest against
the process-owned executable. The native library does not authenticate its
caller-declared artifact identifier. The first product is attested OANDA v20 XAU_USD, USD account,
USD per troy ounce, troy-ounce quantity, multiplier one. No currency conversion,
real-account balance or display-precision-to-tick-size inference exists.

Records use contiguous string sequences starting at 1, one bound epoch and
nondecreasing durable commit times. Exactly one payload matches `kind`:

- `bar`: complete independent midpoint, bid and ask OHLC on the route UTC grid,
  exact open/close/receipt times, immutable provider/provenance digests and
  original provider price-count volume. Midpoint extrema are never synthesized
  from side extrema, and a quote poll never supplies candle volume.
- `episode`: refers to the actual preceding Go decision's signal-record sequence,
  original admission and deadline. It cannot create a host-authored signal.
  Admission follows the signal's durable candle; deadline cannot exceed the
  next candle close, maximum signal age or source slotEnd, whichever is first.
- `quote`: original provider/request/receipt/qualification times, account and
  instrument, tradeability and original bid/ask tiers. A nonempty episode signal
  sequence commits that episode's sole original quote outcome. Request start
  follows episode commit. Qualified freshness and original deadline apply.
- `seal`: recovery or deadline expiry of an unresolved original episode. Recovery
  can seal before deadline; deadline seals cannot. New recovery quotes cannot
  rescue the expired decision, and a seal cannot override a committed quote.
- `coverage-gap`: explicit disconnect, restart or missing-sample interval.

Prices and quantities are nonnegative exact decimal JSON strings, at most 12
integer and 12 fractional digits; prices must be positive. Timestamps are UTC
RFC3339 strings with fixed-width date/time fields, a period before any one-to-nine
fractional digits, and a literal Z. Commas, unpadded fields and subnanosecond
precision refuse before parsing; no truncation can create a grid tie. Supported
nanoseconds are preserved exactly, never binary64 JSON numbers. Completed
bars arrive in chronological interval order; no out-of-order bar buffer exists.
Quote source times are a separate stream, because completed candles naturally
arrive after later quotes. Equivalent price/tradeability/depth at the same
provider timestamp is a duplicate regardless of HTTP/provenance identifiers;
changed content at that timestamp conflicts, even after intervening newer quotes
or a closed/unresolved position. A bounded prefix-wide canonical timestamp index
retains this identity and is included in the authoritative replay-state digest.
An appended quote with an already-resolved original episode reference still
refuses under the separate episode state machine; repeated observation records
do not acquire permission to reopen or recommit an episode. A late quote never gets sorted
back into history. A late straddle observation freezes exposed chronology.

## Exclusive execution phases and ordering

For entry quote source time t and duration Δ, B is the smallest grid boundary
B >= t. Entry exactly on B starts full-bar authority immediately. Otherwise only
later admitted quote times in (t, B) can touch that position's bracket. The
entry-straddling candle's pre-entry extrema are never eligible. A quote at B or
later is exclusively a coverage observation; it cannot introduce a competing
exit or an interim financial mark. Delayed completed bars leave the last
financial mark explicitly as-of, with pending completed-interval resolution. A
new quote inside a full interval already resolved by a completed bar cannot
regress that mark or label the consumed interval pending again.

Sampled exits use the observed liquidation-side price plus adverse modeled
costs, never a fabricated stop-level tick fill. Full-size top-side exit liquidity
and original quote qualification are required. If insufficient, exposure becomes
unresolved; no tier walking, partial fill, closeout-price fallback or later
favorable rescue occurs.

Eligible full intervals start at B. Their exact priority is:

1. Due UTC-slot closure at completed liquidation-side OPEN.
2. Existing stop opening gap, at liquidation-side OPEN.
3. Existing stop touch, at stop level.
4. Existing target touch, at target level.
5. Unchanged ordinary breakeven, then MaxHoldBars close.
6. Final liquidation-side account mark.

There is no opening-target precedence. A target-opening gap followed by a stop
touch still uses stop-first. Timed exits use completed liquidation-side CLOSE
and the original signal index, not elapsed quote duration. Source slot entries
at or after slotEnd are refused. Breakeven uses completed midpoint close, actual
modeled entry's binary64 family operand, current SL distance, existing ATR offset
and tighten-only behavior. Protection events publish old/new versions and the
future effective boundary. A tightened stop never acts against elapsed extrema.

Signal interval, quote provider time, request start, receipt, qualification,
durable record commit, modeled boundary and resolving interval are separate.
Modeled OHLC exits have no observed provider tick time. `modelEffectiveT` on an
intrabar modeled bracket is its resolving close, not a claim about unknown first
touch. A slot/gap OPEN uses the interval open as model-effective time and the
completed observation's receipt/commit independently.

Missing eligible completed intervals, sampled coverage loss or unfillable
sampled exits freeze unresolved exposure and refuse new entries. Later candles
cannot reconstruct missing chronology. No holiday calendar is inferred. This
version has no discontinuous-exposure resume policy or fabricated liquidation.

## Economics and numeric contract

`decimal-rational-family-shortest-v1` uses Go big.Rat for input decimals, costs,
quantity flooring, cash, marked P&L and statistics. Go family arithmetic remains
binary64 and unchanged. Family operands cross to financial rationals via their
shortest round-trip decimal; source entry-distance protection first uses the
ordinary binary64 entry-minus/plus-distance interpretation in the single broker
position. This declared bridge is not tick quantization or an epsilon.

For side sign s (+1 long, -1 short), side-entry price P, stop SL:

    a(P) = fixed slippage + abs(P) * basis points / 10000
    E = P + s * a(P)
    X = SL - s * a(SL)
    loss_per_unit = s * (E - X) + entry_fee + exit_fee
    Q = floor(risk_USD / loss_per_unit / quantum) * quantum

Source Go riskUSD is used. Quantum/minimum/maximum, explicit no-cap versus
notional cap and entry-side liquidity are enforced. Quantity is never reduced
to fit liquidity, maximum or notional cap; each breach is an explicit no-fill.
Invalid resulting geometry or a stop already breached by the entry liquidation
side is refused. Source absolute brackets are not clamped or silently re-anchored.
Fees may be explicitly zero. Spread is already in side prices and is not charged
again. Gap losses may exceed the stop budget; liquidity is observed, not reserved.

Balance includes paid fees. Unrealized P&L is liquidation-side MTM without
hypothetical future slippage or exit fees. Equity = balance + unrealized. Marks
are entry/sampled-straddle observations and post-resolution eligible side bars;
terminal modeled closes also mark the resulting balance. There is no continuous
or tick-level drawdown claim. Peak and maximum drawdown are over this declared
committed sample only. Net completed logical-trade statistics include entry and
exit commissions. Exact cash is serialized as decimal strings; win rate/profit
factor use exact rational strings and explicit no-trades/no-losses status.

Invented independently calculated checks: bid100/ask102, fixed0.1 +10bps,
fee0.1/ounce/side, quantum0.25, starting1000. Long SL98/risk46 gives E102.202,
Q10, entry equity976.980, stop97.802, final954. Short SL104/risk46.04 gives
E99.800, Q10, entry equity977, stop104.204, final953.960. Long bid-close110
nets73.880; long stop-gap bid-open96 loses65.980. These are arithmetic fixtures,
not historical returns or broker promises.

## Recovery, limits and qualification

Replay verifies the complete earlier prefix and its receipt, then the complete
extended prefix; no host-supplied financial state is restored. Receipts bind
input, event and authoritative state digests and cursors. Same-prefix retries
return zero new events and identical snapshots/receipt. Corrupt earlier input or
receipt refuses atomically. Full replay is not constant-work streaming.

Exactly-once application is a separate consumer transaction. It must arbitrate
one original quote commitment versus expiry, fence competing epochs/leases and
atomically commit native envelope, unique effects, full account/protection,
episode outcome, input cursor and receipt. A dropped acknowledgement returns the
existing committed result. Timeout, malformed/partial output or failed commit
leaves the previous pointer intact. Native consistency cannot authenticate facts.

Limits: 16 MiB input/output, 64 KiB source, 8192 records, 4096 bars, 32768 events,
8192 distinct quote identities (also bounded by the total record limit),
8 tiers per quote side, JSON depth64, bounded decimal digits and receipt4096
bytes. CLI rejects special files and symlinks before reading, including FIFO
nonblocking refusal. Reaching a bound refuses; history is never truncated/reset.
The consumer must separately enforce subprocess time/memory, concurrency,
principal isolation, sessions, retained journal storage, Stop/restart and old/new
session coexistence. Finite native limits do not establish those server gates.

Mandatory proof includes independent numerical expectations; every split and
repeat; warmup; actual long/short family execution; original committed quote and
recovery seals; nanosecond/grid ties; pre-entry extrema; delayed candles; duplicate,
conflicting and late quotes; missing coverage; slot/stop/target/management order;
future-only breakeven; exact min/max/quantum/liquidity/cost cases; strict mutation
and resource refusals; actual native CLI; unchanged existing conformance and all
21 pins; actual AMD64/ARM execution and separate public-WASM non-regression.
Native source or synthetic success is not operational Forward readiness. Release,
strict consumer validator, authenticated durable feed/start and final-head CI
remain distinct gates. OHLC-only historical backtests are a different model;
parity requires identical recorded MBA candles, sampled quotes, coverage and
recovery facts. No historical outcome study is part of this tranche.

## Dual EMA resumption paper extension

The producer also admits the long-only dual EMA resumption family on XAUUSD
4h, zone bars. The authored-source audit permits metadata, the declared route,
EMA lengths/rise/Wilder ATR, initial ATR stop, close ATR trail, slow-EMA
fallback, long-only side and USD risk. Other authored filters, generic
management, family resets and route overrides are refused. Admission does not
mean the consumer has installed or qualified this producer version.

Warm-up advances the existing Go family's indicators and discards its entry
and exit orders. Activation starts flat. A fresh completed-bar signal captures
one next-open order; its original episode can fill from the first qualified
observed ask, subject to the existing age, liquidity, slippage and risk-budget
rules. It cannot execute the historical midpoint open retrospectively.

A targetless position and its entry/protection events carry `noTarget: true`
and decimal placeholder `target: "0"`. The flag means there is no target;
zero is never an executable level. ORB records omit the flag and retain their
existing target contract. Initial stops are anchored to the executable fill.
The existing family tightens its stop from a completed midpoint close; the
protection version takes effect only in later observations/intervals. A
below-slow-EMA signal schedules an exit at the first fresh, tradeable
liquidation-side quote after that close, before bracket checks. Missing or
unfillable exit observations leave exposure unresolved; a later completed
bar cannot retroactively supply its open as an executable fill. Stop gaps retain the
side-open fill, and ordinary stops retain the side-bar stop-first model.

Replay receipts bind every dual-family indicator, bounded slow-EMA history,
tracked entry/best close and pending slow-EMA exit. They remain full-prefix
replay receipts, not incremental checkpoints or authenticated feed evidence.
Existing discontinuous-exposure refusal still applies, including closures for
which the consumer has no reviewed calendar coverage contract. Financing is
not modeled by this extension. These limitations must remain visible before
an unattended multi-day run can be qualified; neither zero financing nor a
weekend price path may be inferred from absent observations.
