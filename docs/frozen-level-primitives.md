# Frozen-level research primitives (HT-169)

Status: isolated producer capability checkpoint. Not DSL syntax, a strategy
registration, a quote backtester, a release, or a consumer adoption.

Canonical task: [HT-169](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-169-gold-causal-producer-primitives.md).

## Scope and source fidelity

`engine/frozenlevels` is Go code shared by native and WASM compilation. It adds
no JavaScript strategy implementation and changes no existing family. It supplies
causal source-window, prior-opening and one-shot lock-management primitives for
the reviewed evidence-linked gold research plan. All committed prices and dates
in tests are invented; no private statement or market rows are published.

The primitive contract is `frozen-level-primitives-v1`. A caller must retain this
version, source identity, calendar qualification and explicit assumption ID.
Changing an assumption is a new registered research configuration, not a repair
of observed trading. There is no profitability-selected default.

| Component | Meaning retained in this checkpoint |
| --- | --- |
| Prior opening /3800 | Evidence-inspired final lock LOCATION only; divisor is a named convention, not uniquely recovered code |
| Initial stop | Supplied structural/reference risk; its original rule is unidentified |
| Activation | Explicit positive R threshold with assumption ID; no inferred threshold or implicit +1R |
| Common target | Fixed fill-relative 2R control for the lock-isolation primitive |
| Entry level and momentum | Candidate windows and completed-source open-to-close direction; not a recovered entry instruction |
| Calendar | Explicit finite qualified day ledger; no real broker calendar is bundled or extrapolated |
| Historical result | None computed; fixture events are not trades or a P&L claim |

## Completed context

`FreezeWindow` consumes M5 buckets with half-open `[start,end)` source intervals,
separate `knownAt`, `complete` and source identity fields. Its three distinct kinds
are rolling six completed M5 bars, preceding clock-aligned M30, and preceding
clock-aligned M15. Clock alignment is UTC epoch alignment, not a broker-date
alias; any future local-clock anchoring needs another explicit contract.

A rolling decision at 00:15 has reference 23:45–00:15; clock M30 at that decision
has reference 23:30–00:00. Exactly every required slot must be present, completed,
known by decision time, valid and from the same source. Duplicate/missing slots,
late publication and mixed sides fail closed. Later and unrelated rows cannot
change a frozen value. Input ordering is immaterial because source timestamps
identify slots; no OHLC is fabricated for a gap.

The output includes H/L/C, first open, bounds, actual known-at, freeze time,
source identity and a content ID. `Camarilla` produces C and S/R1–4 from that
specific frozen input. It does not select labels by a later fill. `NetDirection`
compares the FIRST OPEN with LAST CLOSE, which differs from close-to-close ROC.
A complete bucket is still a caller assertion about source quality; this package
does not turn one sparse quote into a proof of continuous market coverage.

## Qualified prior opening

`Calendar` is a contiguous explicit sequence of civil dates and UTC bounds, with
an ID, version, qualification reference and a trading/closed status for each day.
It admits 23/24/25-hour days and offsets in [-12,+14] hours, to minute precision.
No operating-system timezone, inferred weekend closure, or forward DST recurrence
is used. Requests outside the ledger fail. The March transition and holiday used
in the tests are expressly synthetic, not a historical broker schedule.

`FreezePriorOpening` skips verified closed days only. The immediately preceding
trading date must have exactly one qualified first bid-side opening record with
matching calendar identity, source identity, valid event order and knowledge
available at the freeze time. A missing trading date is never replaced by an
older observed date, today's open, a UTC daily open, or the first late download.
The input qualification reference must establish why it is the FIRST qualified
opening; this package cannot itself certify a feed. This stricter missing-date
behavior is deliberate versus archival observed-date-only lookup.

`PriorOpenScaleLocation` divides the qualified opening by 3800 and preserves the
archival `decimal(str(float quotient))`, half-even-to-cents rule. It adds/subtracts
that distance from the actual entry, then snaps the stop towards entry on the
explicit grid. The final addition and grid quantization use decimal rational
arithmetic, not an epsilon-based floating snap. This is an explicit numerical
contract; boundary differences from an archival float epsilon are not silently
claimed as parity. Zero/nonprofitable snapped locks fail. Grid is a research
assumption, not proof of broker tick size or minimum stop distance.

## One-shot lock transition

`LockTracker` starts from an ALREADY ESTABLISHED position, explicit risk and
entry observation. It neither creates an entry nor calculates a fill. Its target
is common fill-relative 2R across no-lock, scale-lock and frozen-pivot-lock arms.
Scale and pivot contexts must have been frozen strictly before the actual entry
event millisecond. Entry observation availability never extends that cutoff.
Because context currently carries no source-sequence key, equal-millisecond
context is rejected; a later sequenced-context adapter must prove ordering before
relaxing that conservative rule. Seeds already at or beyond the common target
are rejected rather than admitted as nonterminal positions. Input slices are copied; later caller edits cannot change
management levels.

Observation keys are `(timestamp,source sequence)`. A larger sequence at the
same millisecond is a strictly subsequent event. Availability must be monotonic
and no earlier than the source event. On every later admitted quote:

1. If continuity is not verified, emit `unresolved` and stop. A later price cannot
   repair an unknown path or silently score it as no trade/zero return.
2. Check OLD active stop and common target on the liquidation side (bid for long,
   ask for short). Emit `exit-due`, without an execution price. Cancel pending
   amendments first. A target/stop quote never submits a retrospective lock.
3. If an amendment is pending and its availability-time latency has elapsed,
   accept it only if still profitable, tightening and at least one grid behind
   liquidation. Otherwise reject it. Old stop remains live until acceptance.
4. If not previously attempted and activation is reached, request a scale lock
   or the most protective eligible frozen pivot. No eligible level rejects the
   attempt. Every activation, eligibility, submission, acceptance, rejection and
   cancellation is counted. A rejected attempt never retries.

All lock prices are interpreted as the exact rational value of their shortest
base-10 input float representation. Risk, 2R target, activation, raw eligibility,
stop/target crossing and request/acceptance admissibility use that same arithmetic
with no epsilon. Off-grid entries and raw pivots are retained without rounding.
Only proposed stops are quantized on the declared decimal grid, towards entry.
Exact internal thresholds and stops remain rational; float projections are output
only and never drive decisions. Pivot raw eligibility is evaluated before snapping. Latency governs amendments only; no stop-trigger reaction delay is
claimed. Timeout, break, expiry, crossing freshness, entry admission and broker
minimum distance rules must be implemented in the later coordinated engine
adapter. The continuity boolean must come from a qualified source adapter, not
from the existence of the next quote.

The independent review regressions cover exact and immediately adjacent
representable-price boundaries in both directions, delayed entry notifications,
equal-millisecond context, and already-due seed targets. The first reviewed
candidate failed these cases; this revision fixes them without changing the
activation assumption or any existing family.

The mandatory invented example uses entry 100, initial stop 90, common target
120 and pivots 106/112/127. Bid 110 requests 106; a strictly later quote at 109
accepts it before target. A short mirror uses ask. Other fixtures prove rejection,
old-stop/target precedence, no retry, missing path and no-lock control behavior.
The old nearest-pivot target ordinarily preempts a same-ladder lock; a separate
hand-derived sub-grid example records that tiny gross locks can remain net losses.
No target-only experiment is implemented in this checkpoint.

## Validation and current gates

Run with Go 1.22 and Node available for the standard Go WASM test runner:

```sh
gofmt -l .
go vet ./...
go test ./...
go run ./cmd/conformance check
bash scripts/checks/frozen-level-primitives-parity.sh
go build ./cmd/heisentick
GOOS=js GOARCH=wasm go build -o /tmp/dslwasm.wasm ./cmd/dslwasm
GOOS=js GOARCH=wasm go build -o /tmp/enginewasm.wasm ./cmd/enginewasm
```

The parity script runs the same hand-derived Go tests under WASM and byte-compares
a full context/scale/lock event trace with native. It is PRIMITIVE parity. It does
not claim an enginewasm bridge endpoint, DSL family conformance, JS runtime parity,
or a fix to the separately known app weekly-parity fixture/metadata defect.
Existing producer conformance goldens and corpus registration are unchanged.

Independent semantic review and exact-head CI remain required. Shared parser,
family dispatch, broker fill/cost logic, report fields and corpus registration
must be coordinated after HT-167/PR85 review; this package does not cherry-pick
that branch. Follow-on work must add qualified quote entry/exit execution and fee
ledger reconciliation, typed DSL/schema/corpus support and fail-closed server
admission before historical jobs. No server data has been qualified by these
synthetic tests. Release, app pin, server jobs, deployment and strategy adoption
remain integrator/user gates. Before adoption, rollback is simply not integrating
this additive package; there is no production caller or default to restore.
