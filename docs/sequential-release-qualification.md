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
