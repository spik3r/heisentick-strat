# Frozen synthetic manager and fee trace

`engine/frozencontrol` connects the actual frozen DSL compiler, original-byte
input admission, quote-derived completed context, lifecycle, side-aware fills,
three existing lock managers and an exact fee ledger. The coordinator is
unexported and unregistered. Its only authority implementation and constructor
live in `_test.go`; ordinary Go/WASM, prepared and prefix execution still refuse
this family. The existing profile and all prior primitives remain unchanged.

This is a complete invented mechanics trace, not a historical strategy result,
recovered entry system or source qualification. The explicit +1R activation,
structural risk and common 2R target remain unverified research controls.

## Frozen before implementation

The 14 packets in `engine/frozencontrol/testdata/` cover seven cases in each
of two directions. Before coordinator code existed, the reviewed Go compiler
and already merged primitives generated the literal config/policy/input/cost
identities, completed windows, Camarilla ladders, lifecycle events and manager
counters. Native and Go/WASM produced byte-identical packets. The manifest SHA-256
is `6326a0f106a941e15cae34a86514badee3278bdc055a7e346e111210bfd03938`.
Fill and fee expectations were hand-derived separately, before the coordinator.
Tests compare the real compiler, context and primitive output to that freeze,
then check actual fills, cash entries and closed-trade net R against the arithmetic.

There are 45 invented paired quotes per case: one prior-day opening, four
explicit price changes in each of nine M5 buckets, and eight evaluation quotes.
The test witness specifies the complete finite quote-change path, including a
hold only where the fixture explicitly qualifies continuity. The calendar is a
finite two-day UTC fixture in January 1970. Opening 3800 with later prices near 100
is deliberately discontinuous toy arithmetic for the /3800 distance, not a feed.

The real compiler produces each of three explicit mode configs. Their common
policy must match after projecting the lock mode to none; metadata and each
assumptions reference retain the established policy-digest rules. All refs bind
the received bytes. Each assumptions payload binds its actual policy digest.
The test request explicitly supplies riskBudgetUSD="1", with no inferred default.
The entire bounded transport, including the three DSL controls and extra leaves,
is capped at 1 MiB; each admitted input also retains HT-172's existing limit.

## Authority and causality

HT-172 RunReady, ContinuityVerified and SourceOrderVerified remain false. The
new package has no production witness, public runner or fallback accepting a
label, digest or boolean as qualification. Test construction verifies the frozen
literal packet, then actually decodes the original three-column CSV into a typed
snapshot and compares its normalized records with the admitted snapshot. CSV
numbers retain original tokens through the strict decoder, including exact raw
Bid/Ask order. Empty/dropped/reordered/mismatching records fail. Adversarial tests
rebind all hashes coherently so this check, rather than a stale digest, refuses
corrupted original data.

Context is aggregated from the admitted quotes, not supplied independent bars.
The witness proves each finite source interval and completed-context key;
`FreezeWindow`, `Camarilla` and `FreezePriorOpening` construct the existing typed
context. Its IDs must match the ordering document's bindings. No uncompleted
bar or future evaluation quote contributes to those windows. The stored M30
level is created at its completed boundary before evaluation, and initializes
without an invented quote predecessor. The first evaluation inside quote resets
it before the later crossing. Warm-up never opens a position.

One common opportunity is resolved before management fan-out. On the first
admission-ready quote, every required signal-frozen context must precede the
actual fill millisecond, including for no-lock. Equal milliseconds reject all
three managers without selective waiting or retry. Missing prior opening also
rejects the common opportunity. All three seeds must validate before any common
entry is committed. Each branch is a separate matched management counterfactual with one position.
Each manager then receives the same entry key, price,
initial stop, quantity and target. There are no additions or compounding.

Entry and amendment acceptance each require a subsequent source observation.
The baseline explicitly sets both latencies to zero; the amendment is accepted
on a later source sequence in the activation millisecond. Old stop/target
protection keeps precedence over acceptance. Unknown continuity is checked at
each transition, not imposed as a retrospective all-or-nothing case exclusion.
In the unknown-path variants, entry and both locks are accepted through ordinal 41;
transition 41-to-42 loses authority and all live positions become terminal
unresolved. No hold, exit, zero return or later repair is invented across it.

## Fill and fee contract

`frozen-quote-costs-pilot-v1.schema.json` describes the exact pilot cost leaf.
Its raw decoder reuses the strict Unicode/key/number boundary, requires all
fields, and rejects unsupported terms. Currency is USD; quantity is troy ounces;
spread is already present in paired quotes and is never charged twice.
Commission is an exact nonnegative USD-per-ounce token, charged once at entry
and once at an actual exit. Positive tokens must be finite/positive at the
existing numeric boundary; their unrounded decimal value drives the ledger.
Only explicitly zero extra price penalty and synthetic intraday financing are
implemented. Nonzero penalties or an inferred financing default reject.

Long enters Ask and liquidates Bid; short enters Bid and liquidates Ask.
An existing manager's stop-due event fills at the observed liquidation quote,
including the price gap. Target-due fills at the exact target threshold with no
favorable quote-gap improvement. Unrepresentable exact structural-stop or target geometry fails
closed instead of silently changing its fill price.

Quantity is explicit USD1 price risk divided by the fixed initial price risk;
commission is excluded from price R0 by the reviewed profile. All ledger
arithmetic is exact rational arithmetic. Terminating decimal strings are rendered
exactly; other values retain an exact fraction. Rendering never controls a price
threshold or monetary identity. Closed net includes both entry and exit fees,
and cash conservation proves the entry fee is not lost or charged a second time.
An unresolved trade retains only known ledger entries, with null exit, gross,
netUSD and netR. The trace is not an existing report/PF/drawdown adapter.

## Hand-checkable outcomes

The long baseline enters 100 with stop 90, target 120 and quantity 0.1 oz. The rolling
six-M5 momentum is positive. The completed M15 H133.77 / L90.01 / C106 produces
R1=110.01133333333334, so at liquidation 110 the most protective eligible pivot is
C106. SCALE uses the frozen opening 3800/3800 and proposes 101. Both are accepted
before a target can exit. Subsequent Bid quotes 105.50, 100.50, 89.50 close PIVOT,
SCALE and no-lock respectively. At USD 0.03/oz/fill, each pays 0.003 entry and0.003
exit: net R is 0.544, 0.044 and−1.056.

The short companion has entry 100, stop 110, target 80, PIVOT94 and SCALE99. It uses
its declared Bid-crossing source bars, then Bid entry and Ask exits 94.50, 99.50,
110.50. Its fee and net-R results are identical. This is an explicit directional
companion, not a blind swap that would change the numerical Bid-high stop rule.

The artificial USD 0.30/oz/fill stress yields PIVOT 0.49R, SCALE −0.01R and no-lock
−1.11R. This proves that a gross-profitable lock can be net-negative; it is not a
broker-cost claim. The target-gap companion exits at 120/80 with net 1.994R, not at
the better observed 121/79. A target at the pending amendment's next quote cancels
that request before acceptance. The base case still demonstrates accepted PIVOT
treatment, while the same-millisecond and missing-opening controls have no fills
or fees. Unknown-path cases retain only the known −0.003 entry-fee cash debit.

## Validation and remaining gates

Run `bash scripts/checks/frozen-level-synthetic-trace-parity.sh`. It compares all
14 full event/fill/fee traces byte-for-byte in native and Go/WASM, plus raw cost
controls. Full Go formatting/vet/tests, race testing, 114/100 conformance, native
and WASM builds and all existing compiler/input/primitive/lifecycle/refusal checks
remain required before a source-only checkpoint merge. The frozen fixture files
must not be regenerated to accommodate coordinator output.

The pilot rejects evaluation horizons that could reach its unimplemented timeout
model. Arbitrary source/calendar/schedule/timeout qualification, broader execution
penalties/financing, multi-opportunity serial accounts, current report integration,
server admission, historical research, production registration, release and app
adoption remain separate gates. The private ML reference work and release-readiness
harness have no dependency or write overlap with this synthetic coordinator.
