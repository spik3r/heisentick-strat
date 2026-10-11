# Paper-prefix producer release checklist

Canonical task: [HT-243](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-243-forward-go-producer-parity-gaps.md).
The bounded documentation claim is recorded in [backlog PR491](https://github.com/spik3r/heisentick-backlog/pull/491).
This checklist prepares an **unpublished v0.31.0 candidate**. It is not a release
receipt or permission to tag, publish, repin consumers or activate execution.
The release coordinator owns those separate decisions. Do not create an RC or
other test `v*` tag: the existing release workflow publishes from such tags.

## Attribution and source identity

- Published baseline: annotated `v0.30.0`, tag object
  `e2d52e8a5abade8114ccb0bedcce01585db71913`, commit
  `4ab0fde8dfcfd3f2b33e4ea0af6186474c2b6ffa`, tree
  `36873545996ffbcb34d2e36d90cab261792c4695`.
  [Release 409449478](https://github.com/spik3r/heisentick-strat/releases/tag/v0.30.0)
  was published at **2026-10-11T05:46:36Z** with eight assets. The five formerly
  Unreleased entries present at that tag now belong to its dated heading.
- Post-tag [PR118](https://github.com/spik3r/heisentick-strat/pull/118), merge
  `5b320fe5abe9039970d79bd0bf4bfe7c9abc3462`, corrects arithmetic investigation
  evidence. It does not change production arithmetic or resolve every observed
  architecture difference.
- [PR119](https://github.com/spik3r/heisentick-strat/pull/119), merge
  `ae7632c9651f2797a1e0de32b5da699da3612dd4`, adds source inspection.
- [PR120](https://github.com/spik3r/heisentick-strat/pull/120), merge
  `1b114cc8c5bdb332e3013f8d9bfd39f9edeadc23`, tree
  `aa1238a1980fee2792530f8502281929b96fd63a`, adds Go paper execution,
  accounting and replay verification.
- [PR121](https://github.com/spik3r/heisentick-strat/pull/121), merge
  `b2d77e83b1b8c3aa19872939828c76e812a787c1`, tree
  `b3fb78d0138141455fbd984c531c561a4d188482`, adds public Go WASM transport.
  Its exact reviewed head was `fcdf41c96132dd29e5e3c14d066fd3f677664040`;
  [CI38137270657](https://github.com/spik3r/heisentick-strat/actions/runs/38137270657)
  passed. This is the documentation source baseline, **not the final release
  commit**. Its [postmerge run](https://github.com/spik3r/heisentick-strat/actions/runs/38138064210)
  and any later documentation-merge CI must be checked separately.

At preparation, the authenticated 35-release listing and matching tag refs
contained no v0.31.0. Recheck both immediately before the release decision,
including draft releases. A tag collision, partial release or lookup error
requires explicit recovery review, never overwrite or assumed absence.

## Exact supported surface and limits

The new surface is described by [the paper-prefix contract](../spec/forward-paper-prefix.md).
It is additive to existing prefix, Sequential and generic run contracts:

- Native `heisentick paper-prefix --request-file=...` and public
  `engineRunPaperPrefix` invoke the same Go execution/accounting producer.
  Ordinary ORB admits close and next-open fills; legacySetup9 admits close fills
  only. Paper proof covers ORB and raw `seq.legacy.setup9.v1`. The producer also
  admits `seq.legacy.setup9_perf_seasonal.v1`, but the paper tests contain **zero
  seasonal-specific paper cases**. Separate generic legacy oracle coverage
  (52 raw / 52 seasonal runs through `engineRunFixture`) does not qualify seasonal
  paper accounting or checkpoint recovery. Consumers must exclude seasonal paper
  admission until that separate qualification passes; no producer refusal is
  claimed. Source-entry, HTF, Full Sequential E1/E2 and special or dedicated
  families are outside this paper API.
- Explicit positive starting equity, nonnegative fees/slippage, multiplier one,
  source/config/route identity, closed-bar watermark and immutable activation
  boundary are required. Caller labels and close watermarks do not authenticate
  instruments, providers, calendars, costs or actual feed freshness.
- Go owns partial/final trade accounting, pending/open state and close-bar MTM.
  There is no synthetic endpoint liquidation. Checkpoint recovery recomputes
  and verifies the earlier prefix before the extension; it is full-prefix work,
  not constant-work streaming or portable cross-build/target checkpoint state.
- Native request/output caps are 128 MiB; public WASM request cap is 16 MiB
  of UTF-8, with original invalid UTF-16 rejected, and output cap is 128 MiB.
  Source is at most 8 MiB, input at most 500,000 bars and events at most 2,000,000.
  These are rejection bounds, **not browser memory, latency or capacity promises**.
  An independently reviewed under-cap stress request reached output refusal
  after roughly 44 seconds and 3.76 GiB Node/WASM peak RSS. Consumers require
  separately qualified smaller bounds, timeout/cancellation and worker isolation.
- [Source inspection](sequential-source-inspection.md) preserves Go parser output
  and binds source/config bytes. Full E1/E2 metadata distinguishes generic
  execution from fixture-only accounting and unsupported market/endpoint/prefix
  capabilities. Inspection cannot grant paper execution or source accounting.

## Freeze and qualify the final candidate

- [ ] Independently review the exact documentation diff and preserve all source,
  workflow, schema, fixture, golden and qualification-manifest bytes outside the
  two claimed documentation paths. Verify ancestry from the published tag and
  the approved PR121 merge; record final commit, tree, clean status and complete
  file/mode inventory in the release receipt after documentation integration.
- [ ] Keep v0.31.0 explicitly unpublished/undated until the authorized release
  decision. Record the actual release date when publication occurs; do not
  manufacture a dated release from candidate preparation.
- [ ] Require complete ordinary CI on the exact final PR head and merged main
  commit, including native ARM64 and actual Node-hosted Go WASM gates. A prior
  head, same source tree, browser-test waiver or cross-build alone is not this
  final-head CI result. Preserve every failed, skipped, blocked and unrun stage.
- [ ] Resolve the workflow's Go 1.22.x to the qualified **Go 1.22.12** toolchain;
  record actual Go/Node/Python versions and target. Use ordinary compiler flags,
  `CGO_ENABLED=0`, `-trimpath` and matching `wasm_exec.js`. The arithmetic gate
  pins Go 1.22.12 and CPython 3.12.x; a changed toolchain requires requalification.
- [ ] Run formatting, vet, uncached full native tests and conformance check.
  Preserve all **244 v0.30.0 goldens: 135 parse and 109 run** byte-for-byte.
  Do not confuse these with the older 128 v0.24.1 goldens (68 parse / 60 run).
  No corpus regeneration is part of this release-preparation slice.
- [ ] Keep all **21 instruction pins** exactly as approved after PR120 and
  preserved by PR121: seven complete-function pins for each of linux-amd64,
  linux-arm64 and js-wasm. PR120's six reviewed observer-related replacements
  are already part of this baseline; do not compare them to v0.30.0 as though
  the paper observer never changed a complete function. Do not rewrite hashes,
  relax inventory admission, use tolerances or disable architecture checks.
- [ ] Re-run the unmodified Sequential aggregate with the pinned prechange
  baseline `f8aba9ef13e22ace21ce31f2ed06cd86ad53464d`, tree
  `8e4ef994a636f2125bab2bb909c28fca5db2351d`, and retain its per-target exact
  arithmetic proof. Its 88 cases / 64 exact arithmetic pairs are separate from
  the 58 binding independent accounting variants and two provisional variants.
  These counts do not establish universal native/WASM numerical equality.
- [ ] Execute actual public paper native/WASM proof on final candidate bytes,
  including the existing 52 exact comparisons, 15 resume splits and 57 refusal/
  liveness cases. Preserve nonzero fees, partial/open/pending states, activation,
  invalid Unicode, byte bounds and same-target retry/recovery. Re-run the
  142-comparison source-inspection proof and existing unrelated producer gates.
  Node-hosted Go WASM is not browser or market-data qualification.

The checked-in commands remain authoritative. In addition to full CI, use
`scripts/checks/paper-prefix-parity.mjs` with the native CLI, actual engine WASM
and matching shim; `scripts/checks/sequential-source-inspection.mjs` also needs
an auxiliary native `cmd/dslwasm` build. Run
`scripts/checks/sequential-backtest-parity.mjs` with the pinned baseline and
ordinary toolchain. Keep all build/evidence outputs outside the clean source
checkout. Local test-only `-buildvcs=false` output cannot replace release assets.

## Build and verify all eight actual assets

Use the existing release workflow without changing triggers, permissions,
runner selection, publication guards or asset names. Before tagging, build from
an isolated clean checkout of the actual final commit; a changed commit requires
new VCS-stamped assets even when a runtime source tree is otherwise unchanged.

- [ ] Produce exactly three native CLIs: `heisentick-linux-arm64`,
  `heisentick-linux-amd64`, `heisentick-darwin-arm64`.
- [ ] Produce `dslwasm.wasm`, `enginewasm.wasm`, the matching Go-toolchain
  `wasm_exec.js`, `conformance.tar.gz` and `SHA256SUMS`: eight assets total.
- [ ] Inspect all five executable/module build stamps for the exact final
  revision, `vcs.modified=false`, expected target, Go toolchain, CGO disabled and
  trimmed paths. Confirm source/index remain clean through the whole build.
  The shim must come from that same Go installation.
- [ ] Verify exactly seven basename-only entries in `SHA256SUMS`; independently
  hash all eight files, including the checksum file itself. Inspect every
  archive member path/mode/content against tracked `conformance/`, `spec/` and
  `examples/`; archive mtimes may alter its outer hash, not extracted bytes.
- [ ] After coordinator-approved publication, record annotated tag object and
  dereferenced commit, release ID/date, successful release-run identity and all
  asset IDs, names, sizes and actual SHA256 digests. Download every published
  asset and verify the actual bytes against both the checksum file and receipt.
  Successful tagging or a release-page listing alone is insufficient.
- [ ] Run the qualified native/public-WASM and inspection proofs again against
  downloaded assets. Rebuild auxiliary test executables only from the same
  final source/toolchain; identify them separately from the eight shipped assets.
  Do not call a rebuilt local executable a downloaded release asset.

The private validation-package artifact-quota blocker is separate from Strat's
public release assets. Do not infer that either quota or the validation package
was fixed by a successful producer build or publication.

## Consumer handoff and stopping boundary

Deliver the exact release receipt and supported-surface limits to the relevant
consumer owners. Consumer pins must bind the tag and verified native/parser/
engine/shim/archive bytes together; no floating main/latest, caller-supplied
producer identity, JS execution/accounting fallback or altered digest is allowed.

Browser resource qualification, consumer full CI, source/option/data admission,
authenticated start/stop, principal isolation, atomic durable journal/checkpoint
publication and end-to-end replay remain their own gates. Releasing this producer
does not complete universal Backtester/Forward parity, source-accounting support,
Full E1/E2 paper execution, historical research approval, deployment or trading
activation. It places no external order. Keep HT-243 open for its wider work.
