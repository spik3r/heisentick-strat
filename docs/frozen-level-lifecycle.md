# Frozen-level lifecycle and admission events (HT-170)

This is a second isolated Go producer checkpoint after
[HT-169](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-169-gold-causal-producer-primitives.md).
Canonical task: [HT-170](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-170-causal-level-lifecycle.md).
It adds no DSL family or execution adapter. All fixtures are invented.

## Versioned hypothesis contract

`LevelSpec.Version` must be `frozen-level-lifecycle-v1`. Provider ID, context
window ID, context source, paired-quote source, ordering-proof reference,
known/creation events, expiry, direction, level price and explicit policy all
participate in the content-derived level ID. Opposite directions have independent
IDs; there is no implicit shared OCO pair. A changed assumption creates a distinct
identity. Neither this identity nor a nonempty proof string certifies a feed;
a future qualified adapter must substantiate and bind those references.

The supported policy is explicitly `one-success-no-retry`. All freshness and
latency values are required, with no inherited defaults. The archived stored-M30
reference, mandatory six-M5 gate, half-hour expiry and +1R activation are research
assumptions, not recovered original-trader rules. This component does not impose
them or infer their best values. It consumes a previously generated level and
outputs events only. Source quality, calendar, completed windows and momentum
remain separate inputs; no final target or later fill may generate a level.

## Ordering and knowledge

`KnownAt` and `CreatedAt` are ordered event keys in the explicitly declared
`OrderingProof` domain. The proof must establish how context-completion and
creation events sit amongst the stable source observations. Merely assigning
sequence numbers to ambiguous records is not proof. Knowledge may equal creation;
it may not follow it. Quotes must be strictly later than creation and each prior
observation in `(timestamp, sequence)` order. Equal-millisecond quotes are valid
only with strictly increasing sequence under that same qualified order.

Observation availability is separate, must be monotonic and cannot precede its
source timestamp. A creation predecessor is optional, but if supplied it must be
strictly before creation, already available by creation, fresh, source-matched
and valid. It is copied, never retained as mutable caller state. Context and quote
identities are explicit and may refer to separately versioned aggregates of a
feed; the package does not assert their unproved relationship.

The existing lock primitive is unchanged: it still requires its contexts frozen
strictly before the actual entry millisecond. This lifecycle's explicit ordered
creation events do not silently relax that rule. A later integration needs to
reconcile and independently prove any same-millisecond context-to-entry path;
this checkpoint claims only event-level lifecycle/admission behavior.

## Crossing and reset

The trigger side is bid in both directions, an explicit research convention,
not an ask-triggered broker buy-stop model. Long crosses when a fresh predecessor
bid is strictly below the level and the new bid is at or above it. Short reverses
the inequalities. Exact decimal-rational comparisons preserve off-grid prices and
distinguish a threshold from an adjacent representable price without epsilon.

Without a predecessor, the first quote establishes state and emits `reset`.
An already-outside quote cannot signal. A valid inside observation arms the level;
a later crossing emits `crossed` and a unique signal ID. Repeated equality at the
boundary never re-arms it. An already-proved fresh predecessor can establish a
crossing on the first post-creation quote, including a later source sequence in
the same millisecond.

A fresh current observation after an overly old predecessor emits
`reset/stale-predecessor` and establishes a new baseline without a crossing. If it
is outside, another inside/reset observation is needed. By contrast a current
observation older than its maximum allowed receipt age is invalid input, not a
signal: the level is invalidated. Both age boundaries are inclusive.

## Admission events, never execution

A crossing while the caller's one-position guard is busy emits `skipped` and
leaves the level active. It does not consume the successful use or queue an
implicit later entry. Another opportunity requires another fresh crossing.
Matched versus serial runs must manage their distinct guards externally; this
component does not run either account model or decide exposure itself.

An unblocked crossing emits `submitted`, which means an admission attempt is
pending, not that a broker order was transmitted. Submission starts when the
signal is available; its explicit latency adds to that timestamp. An eligible
observation must be strictly later in source order, have a source timestamp at or
after that submission boundary, be fresh/valid and still before expiry. Earlier
source events arriving late cannot become executable merely through delayed
receipt. Pending observations before submission are retained for ordering but
produce no admission. At zero latency, a later sequence at the same timestamp is
allowed when both source and availability conditions genuinely hold.

The first eligible observation emits `admission-ready`. It has no price, stop,
size, trade result or implicit fill. The caller must resolve that exact event
before supplying another observation:

- `consumed`: the external caller attests a successful execution; the level is
  terminal and cannot be used twice.
- `rejected`: external execution/risk validation rejected the attempt; terminal
  invalidation, without retry.
- `busy`: the caller's position guard blocks execution; return to active state,
  with a later fresh crossing required.

Busy at the admission observation produces the same skip without a ready event.
Unsupported resolutions, wrong keys or a resolution before readiness are errors
without changing state. Supplying a later observation before resolving readiness
invalidates the level, preventing selection of a better later price. A subsequent
quote back inside the threshold does not erase the previously causal signal;
only the future execution adapter can determine fillability and risk geometry.

## Expiry, invalid inputs and observability

Expiry is half-open. A quote whose source OR availability timestamp is at/after
expiry expires the level before crossing or admission. A pending attempt cannot
outlive expiry. If submission cannot occur before expiry, the crossing is logged
and the level expires without admission. Latency is compared with the remaining
time to expiry before addition, so even a delay beyond the representable timestamp
range preserves the same crossed-then-expired event audit. Expiry is recognized on an actual
observation; its declared effective time remains in every event. No future quote
is backdated to invent an earlier fill or closure.

Malformed/nonfinite/crossed quotes, wrong source identity, duplicate/out-of-order
events, availability regression and unverified gaps emit invalidation and error.
They cannot be discarded and followed by a repaired continuation on the same
level. Stale-predecessor resets are intentionally distinct from malformed or stale
current observations. Continuity is a qualified external assertion; the presence
of another quote does not establish it. Terminal levels reject every later call. This checkpoint retains state within a
lifecycle instance; it does not implement durable resume or a global registry.
The later owning engine must keep one instance per level ID and retain consumed
IDs across its supported lifetime rather than recreate a consumed level.

Every event includes contract version, level ID, event/availability time, expiry,
reason and the most recent signal ID when one exists. No event asserts P&L,
broker execution, input completeness or a recovered rule.

## Validation and remaining gates

```sh
gofmt -l .
go vet ./...
go test ./... -count=1
go run ./cmd/conformance check
bash scripts/checks/frozen-level-primitives-parity.sh
bash scripts/checks/frozen-level-lifecycle-parity.sh
```

The lifecycle script runs all lifecycle fixtures in native and Go/WASM and
byte-compares both the long/short event trace and the independent maximum-time
trace. The committed independent exported-API regressions include 528 crossing
scenarios and the formerly failing overflow/expiry audit case. These tests cover outside reset, proved
predecessor, exact crossings, equality, source order, busy recrosses, one use,
freshness, expiry, delayed availability/submission, malformed input, unknown gaps,
wrong/late resolutions and source/assumption identity. It is primitive/event
parity, not a new production bridge or executable strategy.

The prior lock files, shared parser/runtime/broker/report, old quote-preflight
contract and existing conformance registration/goldens remain unchanged. This
revision is tested on independently reviewed merged PR85 and PR86, producer
main 1e862d4eef53ea17a2071d8f265f1c046b955165. All existing source files and
clock-range conformance are preserved; no competing PR85 implementation is added. Full quote execution,
stop/target/cost ledger, timeout/break handling, qualified source ingestion,
typed DSL/schema/bridge/corpus, server admission and historical evaluation remain
separate stages. No release, consumer pin, deployment, acquisition or trading is
included. Independent review and exact-head CI precede parent-owned integration.
