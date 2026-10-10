# Sequential bounded producer qualification

## Candidate status

The baseline is `f8aba9ef13e22ace21ce31f2ed06cd86ad53464d`, tree
`8e4ef994a636f2125bab2bb909c28fca5db2351d`. The new companion candidate is
under implementation/review in HT-229, backlog PR #467. No candidate commit,
tag, published asset or consumer pin is accepted by this working document.

The latest published baseline is v0.29.0. The producer's semantic-version
policy implies a future minor release for Sequential additions; any proposed
version must be rechecked against actual tags after candidate qualification.
Do not create a draft/RC `v*` tag to test packaging: the release workflow
publishes from such tags.

## Supported candidate surface

- Four exact source hashes in `sequential-source-freeze.json`.
- Faithful legacy Setup-9 raw/seasonal, preserving close fills, signed-counter
  wrap and final-close liquidation.
- Full synthetic E1/E2 next-open policies, preserving accepted costs,
  fractional cap downsizing, structural stops, targets, time exits and typed
  contiguous-input/incomplete-terminal refusals.
- A separate Go companion projection governed by
  `sequential-metrics-contract.md`, using the same actual broker execution.
- Headline, completed trades and close-bar equity only. The existing report
  CLI, P1, real calendars, policy checkpoints, grid/HTF, historical evaluation,
  paper-forward and consumer activation are outside this qualification.

## Required evidence before readiness

1. Source/base/claim identity and independent semantic review, including the
   bounded HT-245 observation-hook decision and adaptive resource/domain proof.
2. Existing formatting, vet, full native tests and conformance unchanged;
   every old golden remains byte-identical. Preserve the legacy oracle,
   143-case core prefix/restart suite and independent E1/E2 binding corpus.
3. Exact four-source parser and genuine native/Node-WASM exported companion
   checks, meaningful mirrored trades, typed refusals and repeated-run isolation.
4. Prechange baseline, current capture-off and current capture-on comparison
   under actual amd64 and ARM64 compilers plus Node-WASM. Capture hooks are
   not presumed FMA-neutral. Classifications/counts/streaks are exact gates.
5. Metric boundary tests: all-fee accounting, null reasons, same-bar events,
   last-bar replacement, mark alignment, independent currency/percentage DD,
   cancellation, finite bounds, overflow and no successful partial payload.
6. Independent source review, reviewed adaptive fingerprint renewal, final
   exact-head CI and candidate schema validation. Real-browser coverage is
   recorded only if executed using an already available bounded route; no
   installation or three-browser matrix is required by this slice.

Commands and results must be appended after they execute. A failed or unrun
check is not a pass. Local `-buildvcs=false` artifacts are test inputs only,
never substituted for revision-stamped release assets.

## Release and consumer handoff

After the final candidate qualifies and merges, freeze its actual commit/tree,
resolve the normal release version and build through the existing release
workflow without overwriting assets. Verify all expected native/parser/engine
WASM/shim/corpus/checksum assets, their revision/toolchain metadata and actual
hosted bytes. Rerun qualified exports against downloaded artifacts.

Only then may HT-230 pin and activate the dedicated route. Its dormant first
slice has an intentionally null release receipt and unavailable metrics.
Before activation, summaries and every visible derived widget must honor Go
capabilities; no JS statistics/equity/P&L fallback or fabricated zero is valid.
The inactive frontend import receives a separately reviewed exact-blob delta.
HT-231 outcome authorization remains false until its separate owner decision.

## Local review evidence — 2026-10-10

Independent resource/domain review accepted the bounded broker hook change at
SHA256 `6e13cacd789c1a993bcda089430a0ac73a778a6399177599376cc73260d5957a`.
Adaptive's zero-value constructor cannot enable the private Sequential collector;
reset clears it. Measured broker layout on amd64 and Node/WASM is 9,688 bytes,
up exactly one eight-byte pointer from 9,680. No adaptive per-row capture or
output field is added; the existing 93,195,064-byte output-allocation bound
remains applicable (it is not an RSS guarantee). All seven other reviewed
constructor fingerprints remain unchanged. Renewal acknowledges this proof,
not cross-architecture arithmetic neutrality or closure of HT-245.

Official Go 1.22.12 linux/amd64 and Node 24.19.0 actual export checks passed
88 records: four source parsers, 64 successful companion executions, 18 shared
native/WASM refusals, and two raw-UTF16 WASM transport refusals. The successes
include all eight source/side tapes, non-power-of-two cancellation and adjacent
binary64 fees. Prechange f8, current capture-off and capture-on generic/column
outputs matched exactly on the executed amd64/WASM surfaces. All 148 emitted
success/failure envelopes validated against the versioned schema. Fixture and
result hashes were verified against the saved bytes. Native conformance passed
133 parse and 108 run goldens without modifying any golden. Native vet passed.

The first full native test invocation passed 23 packages and failed only the
intentionally stale broker fingerprint before the independent proof review.
After renewal, the full uncached native suite passed all 24 packages. Exact-head
CI and native ARM64 parity are mandatory and pending. Browser execution is unrun.
This is test-artifact evidence, not published-release or consumer activation.

### Native ARM64 gate stopped — 2026-10-10T11:08:29Z

Producer draft PR #113 head `5849fc3686c548b00b17f74c3c25baa968b67997`
failed the new exact comparison in CI run `38047049563`. The first mismatch was
`family-legacy-setup9-fall-costs`: raw account net was
`-123.62037356029629` on native ARM64 and `-123.62037356029626` on WASM.
The shown retained trades, nominal per-trade accounting, counts and streaks
matched. Return and expectancy propagated the account-net difference. This is
an actual failed gate, not accepted tolerance or a qualified ARM64 result.

Independent official Go 1.22.12 optimized ARM64 cross-compilation shows the
same entry `FMSUBD` in baseline broker.go and the observed candidate. Capture
branches converge before that inherited mutation. An explanatory `math.FMA`
replay using captured sizes and exit credits reproduces both totals exactly,
with the first divergence at the second entry, original bar 250. Cross-compiled
assembly and replay explain the mechanism; neither substitutes for native
execution or proves the later cancellation cases.

The next diagnostic revision evaluates all frozen cases before failing on any
exact cross-target mismatch. Same-architecture prechange/capture-off/capture-on,
input/result invariants, signs, counts, streaks and null categories remain strict.
No tolerance, rounding, shadow account or broker arithmetic change is introduced.
Any cross-architecture bound for account net, prefix equity, MTM or derived
metrics needs its own independent operation/operand propagation review. A
canonical arithmetic change is a separate HT-245 compatibility decision.

### Approved qualification refinement and integrated source — 2026-10-10

At 12:59:03 UTC the owner approved preserving engine outputs while replacing
the former universal raw accounting equality check with exact results for each
verified target's arithmetic. No tolerance allowance or runtime change was
approved. Backlog HT229 PR467 records the genuine decision, three test-only
path amendment and implementation start at head
`ae185f797233a6a67339b21f0bd171034cc9e454`.

The historical ARM64 failures above remain failures under their former gate.
The complete raw diagnostic at head
`668c1b24cf9b30ff65aed9722e8ba068bbc18951`, run `38050061077`, evaluated all
88 cases; all nine same-target checks passed 64 cases each. The independent
prospective exact replay reproduced all 87 differing raw numeric leaves after
actual ARM evidence arrived, with no unexplained or categorical difference.
This evidence motivated the approved refinement; it is not a retrospectively
changed CI result.

The final candidate integrates producer main
`2196722113496a5ccd88850cbb441c02f120a66f`, tree
`89c1ca1fafce0978ca7dac26bd3803b7ebd6ce3e`, including separately qualified
PR108 supply/demand fixes. All 23 main paths are preserved; shared runner and
CHANGELOG changes are combined. Conformance is now 134 parse / 109 run cases.
The new source/instruction pins and every affected check must be qualified on
this final union; the prior source's passing analyses do not transfer by name.

The stdlib-only exact verifier requires CPython 3.12.x and pinned Go1.22.12
ordinary builds. Unknown/missing tools, options, sources or instruction
schedules fail explicitly. Arithmetic self-tests and malformed/corrupted-proof
tests must pass before the real native/Node-WASM run. The receipt keeps raw
cross-target equality distinct from exact target-arithmetic qualification.
All same-target, upstream operand, identity, category/sign, refusal, old-golden
and generic-output gates remain exact. No broken or incomplete evidence is
converted to an explained mismatch.

Independent verifier review and fresh actual amd64, ARM64 and Node-WASM runs
are pending at the time this section is authored. No new pass, release or
activation is asserted here. Claude's independent HT250 economic corpus remains
pinned separately with its authored binding/provisional/injected-only labels;
its local diagnostic success is not an ARM64 or release qualification.

### Final-union local runtime checks — 2026-10-10T13:15Z

Before verifier publication, official Go1.22.12 linux/amd64 passed all 24
uncached packages, full vet, formatting and conformance (134 parse / 109 run,
zero TODOs) on the working source union above. No golden was regenerated.
Focused Sequential/SupplyDemand engine/report race tests passed, and the full
seqcore race suite passed separately in 40.091 seconds. These race checks use
race instrumentation and are separate from ordinary-build numeric evidence.

Independent final-union resource review measured broker size again on actual
amd64 and actual Node-hosted Go/WASM: main219 is 9,680 bytes; the union is
9,688; flagParams is 3,192 on both. The one-pointer delta and unchanged private
Sequential-only collector admission remain verified. This is fixed-size
resource evidence, not a new RSS or whole-runtime allocation bound.

Independent instruction regeneration and rounding-oracle checks are preliminary
while the verifier is being reviewed. Final verifier hashes, exact-head CI and
actual ARM64 qualification are still pending; these local results do not clear
those gates.

### Frozen verifier review and AMD64/WASM evidence — 2026-10-10T13:21Z

Independent review accepted the four frozen test-only verifier files after
19 self-test groups, 4,272 independent rounding-oracle checks, 31 corrupted
evidence probes, 16 source/schedule/options probes and six baseline provenance
probes. Missing Python produces an explicit failed, incomplete receipt.
The reviewer independently rebound and replayed all 64 saved successful pairs:
47,740 exact arithmetic checks and 92,156 complete raw-bit checks passed.

The fresh final-union run used official Go1.22.12 linux/amd64, Node24.19.0 and
CPython3.12.14. All 88 cases completed, each of the nine same-target checks
passed 64 cases, and 19 raw-evidence guards passed per target. Both
`qualificationPass` and `rawCrossTargetExact` are true on these actual AMD64
and Node-hosted Go/WASM executions. Receipt SHA256:
`f26cdc67752aa9b47188348502e5427707d47b56f7722ff5b2f84b82557231a6`.

Frozen verifier SHA256 identities:
- parity script: `16144e04619c1251ca227330678d612e42d3b30c28a632c26ac0d2b066d415e8`
- exact replay: `3637cb29d408aa3dbc2e9c1bec73e42d45b52c5b59a4791135dfefeffa1476d4`
- self-tests: `e3e1a5686c6844623b885f35613f237e14c943d37d7313aa7b045d7b9e7a98a3`
- source/schedule manifest: `5bcdcbdfdee7d338e272c1033bb9d2f38bb3c8d6c8b12d6efa1f1fa9a140b0d7`

Count correction: the earlier local paragraph's “148 emitted success/failure
envelopes” is inconsistent with the stated 88-case breakdown. That complete
breakdown produces 166 companion envelopes: 128 successful, 36 shared-refusal
and two WASM-only refusal envelopes; source-parser records are separate. The
historical actual-ARM artifact's independent schema check validated all 166.
The earlier count is retained above as dated history, not current evidence.

Fresh final-head ARM64 execution and exact-head CI remain mandatory. Cross-
compiled ARM instruction verification and historical replay do not clear them.
No tag, published asset, release pin, historical outcome or consumer activation
is qualified by these local results.
