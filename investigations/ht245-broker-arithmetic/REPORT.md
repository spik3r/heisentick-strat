# HT-245: broker arithmetic investigation and evidence correction

Investigation only. Producer engine/context source, frozen goldens, runtime
fingerprints, releases and consumer pins are unchanged. The only qualification
manifest amendment admits the three investigation Go executables to the exact
source inventory; production dependency closures and instruction pins are unchanged.

Task: [HT-245](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-245-broker-arm64-fused-arithmetic.md).
Original investigation: Claude, Strat PR118 at `6dfa80eb6e36100a4fcc7fa951a293d05c400c32`.
Evidence correction: Astra, following the [independent review](https://github.com/spik3r/heisentick-strat/pull/118#issuecomment-6106782457)
and the [additive takeover claim](https://github.com/spik3r/heisentick-backlog/blob/b0ca3b5d7828e3c67cea5ad85cf58d80fc68b3f4/tasks/HT-245-broker-arm64-fused-arithmetic.md)
in [backlog PR482](https://github.com/spik3r/heisentick-backlog/pull/482). Original author text and receipts are
retained in [the historical report](results/author-report-6dfa80e.md) and the
existing `results/` files. Those historical outputs are not current v2 run evidence.

## Findings and observation boundary

The harness reads `RunFixtureCase(...).Trades`. The engine serializes its numeric
trade fields to **15 significant decimal digits** before returning them. Our bit
patterns and ULP distances describe those serialized public values, not raw broker
intermediates. Exact equality here does not prove identical internal arithmetic,
unchanged level history or equality of unrecorded outputs.

The original ARM64 investigation reports changes in serialized P&L, sizes,
stop/target levels and exit prices. It also reports an exit-index change from 962
to 963 in three of six constructed channel-break-hold boundary cases. Its frozen
corpus, fee variants and boundary summaries remain useful historical observations,
but their original source/runtime/receipt binding was incomplete. Those historical
files alone do not establish independently reproduced
ARM64 Go1.22.12 runtime evidence; the fresh actual execution below does.

Independent actual Go1.22.12 execution at the original PR head established:

- Real Linux AMD64v1, real AMD64v3 and Node24.19.0 Go WASM each attempted 1,000
  cases: 109 frozen, 231 fee variants, 654 scaled and six boundary edits. Each
  produced 976 successful cases and 24 matching strict-input refusals. Every
  successful serialized trade output agreed across these three targets.
- The six boundary exit indices were `[963,963,962,962,962,963]`. The three cases
  reported to flip on ARM64 are scales 2, 3 and 4, which exited at 962 here.
- The separate 3,240-case search on actual AMD64v1 produced 3,120 successes and
  120 strict-input refusals. That earlier larger search was not cross-target proof.
- The actual expression micro on AMD64v1/v3 and WASM returned quantity 0.5. An
  explicit mathematical FMA model returned fused equity bits `40c298d49616b203`
  and quantity 0.4, versus separately rounded `40c298d49616b204` and quantity 0.5.
  This is an expression witness on invented inputs, not an engine sizing run.
- ARM64 Go1.22.12 cross-compilation confirmed fused instructions at the cited
  broker/channel sites. Compilation does not establish ARM64 runtime execution.

Fresh execution with the corrected tools is recorded in the
[native matrix summary](results/correction/native-summary.json): **4,240 attempts,
4,096 successful cases and 144 expected refusals per target** on actual Linux
AMD64v1, Linux AMD64v3 and Node24.19.0 Go WASM, all using Go1.22.12. Across the three
targets that is 12,720 attempts. All successful serialized trade values and
recorded decisions agree, including the full 3,240-case search (3,120 successes,
120 refusals and 30,848 trades per target). Empty boundary-search adds no cases.
The complete [six-case boundary rows](results/correction/native-boundary-rows.json)
retain the observed exit indices on every target.

These local runs used a worktree at original HEAD `6dfa80e` with the recorded
correction changes. Their exact compiler-source and executable-tool digests match
published frozen executable commit
[`4a9448bacd6362a1070c238beaadbe878a4689f8`](https://github.com/spik3r/heisentick-strat/commit/4a9448bacd6362a1070c238beaadbe878a4689f8),
tree `fe01c041646444889e4bfbee4f26a2457d4ab752`; we do not claim that later commit
was checked out for these earlier local executions. The source digest is
`0f4269f437219c6036dc9175f3d4b303e6e052e6fe0347079de2dae7d3e38b04` and the
executable-tools digest is
`42010b71adfeb966b0eeccde946fc19a04fb7a46559e2bba8650fc8e05d32e36`.

Actual native Linux ARM64 production and diagnostic runs are now complete as
separate evidence sets. They were executed at that exact published commit, with
a clean source checkout verified before and after, and the installed Go and
compiler bytes checked against the official Go1.22.12 Linux ARM64 distribution.
Node22.23.2 orchestrated these native runs; this does not claim an additional
Node22 WASM execution. [ARM summary](results/correction/arm-summary.json).

### Fresh actual ARM64 findings

Both production ARM64 and the diagnostic ARM64 build attempted 4,240 cases,
producing 4,096 successes and 144 expected refusals each. Together with local
AMD64v1/v3 and NodeWASM, the five configurations executed **21,200 attempts:
20,480 successful cases and 720 refusals**. All exact source, tools, numeric
input, compiler/build and receipt bindings validate before comparison.

The following counts compare successful serialized cases against AMD64v1;
refusals remain outside every identical/differing count.

| Set | Successful cases | Production ARM64 | Diagnostic ARM64 with no-FMA flag |
|---|---:|---|---|
| Frozen corpus | 109 | 109 identical | 108 identical, 1 path |
| Fee variants | 231 | 32 identical, 199 P&L-only | 231 identical |
| Six scaled prices | 630 | 605 identical, 25 path | 624 identical, 6 path |
| Thirty-scale search | 3,120 | 3,033 identical, 83 path, 4 P&L-only | 3,120 identical |
| Boundary patches | 6 | 3 path, 3 decision | 3 path, 3 decision |

The 24 and 120 scaled refusals match on every configuration. The original frozen
1,035 trades agree on production ARM64 as well as all three local targets.
In the fee variants, **269 of 3,051 reported P&Ls differ**, including nine
zero/residual sign changes and exactly **three changes to `pnl > 0`**; the largest
absolute serialized P&L gap is approximately `1.0e-11` currency. No recorded trade
count, side, entry/exit index, reason, rule, tag or partial flag differs across
the 4,090 successful cases without constructed boundary patches.

All six production ARM64 boundary cases exit at bar 963. AMD64v1/v3 and WASM exit
at `[963,963,962,962,962,963]`, so **scales 2, 3 and 4 change their exit decision**.
The diagnostic no-FMA build also exits all six at 963. Both the original channel
architecture constants remain present in that diagnostic. Therefore these fresh
runs prove an architecture-dependent decision in the constructed examples, but
do not isolate the constants from fusion or establish that fusion alone causes it.
The recorded [ARM boundary rows](results/correction/arm-boundary-rows.json) and
[complete cross-target comparisons](results/correction/cross-target/) contain both
old/new exit indices and all serialized differences, including decision-case P&L
changes that the historical comparator returned before recording.

The diagnostic eliminates the observed fee and search differences; its remaining
one frozen and six scaled path differences all belong to channel break hold.
That is useful diagnostic evidence, not a proposed production compiler setting.
No scratch-copy constant ablation or dedicated Master engine sizing run was added.

## Correct refusal and equality denominators

The four full-Sequential fixtures bind their admitted bars to their strict
input. Scaling the bars without changing that binding correctly refuses the
input. **No engine admission rule is changed.** The six-factor manifest has
24 refusals out of 654 attempts, and the thirty-factor manifest has 120 out of
3,240. The manifests now declare those exact expected errors without changing
any fixture source, decimal scale, costs or bar-patch value.

| Manifest | Attempts per target | Successful cases | Expected strict refusals |
|---|---:|---:|---:|
| Frozen run corpus | 109 | 109 | 0 |
| Fee-injected cancellation variants | 231 | 231 | 0 |
| Six scaled prices | 654 | 630 | 24 |
| Thirty-scale search | 3,240 | 3,120 | 120 |
| Boundary patches | 6 | 6 | 0 |
| Boundary search | 0 | 0 | 0 |

Thus the four sets without constructed boundary patches comprise **4,234 attempts, 4,090 successful cases and
144 refusals**, not 4,234 successful arithmetic runs. Matching refusals are
reported separately and never contribute to identical-success or trade counts.
For successful cases, `decision` means a changed trade count, side, entry/exit
index, reason, rule, tag or partial flag; `path` means a changed serialized price,
level, size or points value; `pnl-only` means only reported P&L differs. These
classes do not describe unobserved raw intermediate paths.
An unexpected error, including equal file-read failures on two targets, fails
the run. A declared refusal becoming successful also fails until its expectation
is reviewed.

For interpreting the old summaries, subtracting the independently identified
refusals changes the historical scaled ARM64 totals to 605 identical successes
plus 25 serialized-path differences out of 630 successes. The search totals
become 3,033 identical, 83 path and four P&L-only out of 3,120 successes. These are
corrections to historical summary arithmetic, not newly bound ARM runtime results.
The historical search log does not record the no-FMA target that appears in its
comparison JSON; its claimed 3,240 identical no-FMA comparisons are **withdrawn
as established evidence**. The fresh bound diagnostic above now independently
records 3,120 successful identical comparisons and 120 separate refusals; it does
not repair the provenance of the historical file or turn its 3,240 attempts into
3,240 successes.

The historical cancellation summary, now reproduced by the fresh production
ARM64 run above, reconciles 269 differing reported P&Ls out
of 3,051 trades, nine zero/residual sign changes, and exactly three changes to
`pnl > 0`. These are distinct counts. Serialized zero-to-residual bit distances
are not a useful measure of small relative error near zero. Only one of the
109 frozen fixtures has a nonzero fee; the frozen corpus is not fee-free.

## Channel constants and causal limits

`normalizedChannelRail` adds `9.4e-12` and `normalizedChannelWidthATR` subtracts
`1.565e-12` on ARM64. Both architecture branches remain unchanged. The original
scratch-copy ablation reports a boundary flip with FMA on and constants removed,
and equality with both FMA and constants removed. Its archived procedure and
output remain historical author evidence, with incomplete provenance. No refreshed
constant-ablation proof is claimed by this correction. The old destructive,
unbound entrypoint now stops with an explanation; its original text is retained
under `results/historical-tools/` for audit. The new production and diagnostic
ARM64 runs keep those constants intact; their persisting boundary flips are
reported above without attributing them to a single mechanism.

The boundary generator compares serialized initial and final SL/TP as a
**heuristic** for choosing candidates. Equality of those endpoints cannot prove
that the raw level never moved, or that the edited bar cannot affect a trail.
Every generated candidate requires a fresh execution. The patch replaces the
whole exit bar, including open and close; it is not solely a low/high edit.

## Reproduction and binding

Use a new output directory for every invocation and an explicit compiler. The
independent successful Go1.22 WASM retry used a minimal inherited environment;
an earlier oversized environment failed the loader before case execution. That
failed attempt and its successful retry are both retained in the independent
review receipt. An arbitrary inherited environment is not qualified by these runs.

```sh
env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" \
  HT245_GO=/absolute/path/to/go1.22.12/bin/go \
  HT245_WORK=/task-local/path/cache \
  bash investigations/ht245-broker-arithmetic/run-all.sh \
  /task-local/path/new-results amd64v1 amd64v3 wasmjs
```

On actual ARM64, use `arm64`; `arm64nofma` is a separately labelled diagnostic
with `-gcflags=all=-d=fmahash=n`. The latter is an undocumented compiler debug
setting, not a production build recommendation. `wasip1` requires wasmtime.
Only requested targets run; an unavailable or failed requested target makes the
run fail. The default is actual native plus Node-hosted Go WASM. Rosetta execution
on Darwin ARM64 is labelled as translation, not native AMD64.

`run-all.sh` executes all six committed manifests and never regenerates or
redirects output into tracked inputs. Generators are explicit separate commands
and accept only receipts belonging to a complete validated current run. Empty
boundary-search manifests are recorded as zero cases, not successful tests of
unconstructed boundaries.

Each run contains its exact `manifest.json`, `identity.json`, `run.json`, runtime
identity JSON and JSONL receipts. The runner records actual compiler and compiler
binary digests, flags, runner/version, built binary digest and runtime settings.
It embeds the exact source-file digest in the executed harness binary. Git HEAD
and working changes are recorded separately; an uncommitted harness is never
claimed to be identified by HEAD alone. VCS stamping is disabled explicitly for
portable worktree builds; the content digest is the executed-source binding.

The run validates every expected unique case ID, normalized input digest,
expected refusal, row shape, runtime identity and source digest. It rechecks
source, tools, manifest and fixture bytes after execution. A fresh directory and
atomic per-target publication prevent stale or partial receipts entering a
comparison. The comparator uses the run envelope instead of scanning arbitrary
JSONL files, checks all bound digests, records both sides of decision differences,
and binds its summary to the exact input run and receipt hashes.

Cross-machine comparisons accept multiple complete run directories:

```sh
node investigations/ht245-broker-arithmetic/compare.mjs \
  /amd64-results/boundary-patches /arm64-results/boundary-patches \
  --ref linux-amd64v1-go1.22.12 --json /output/boundary.compare.json
```

The complete raw local bundle is durably retained as
`ht245-native-v3-evidence.zip` (12,291,712 bytes), SHA-256
`a68e9bf9840c62bce25879189f0b41dc213163d961114e944039372d528c1e9a`.
The [archive receipt](results/correction/native-archive.json) records Library item
`libfile_2d4f1dc7bf9c819197bd281e751f0246`, version 0, and its backing file identity.
This is an owner-access-only Library artifact, not a public or collaborator-access
link. Repository readers can reproduce from the committed inputs and commands;
raw receipt access requires the owner to provide the archive. The compact summary
retains SHA-256 values for all 72 raw evidence files, while per-run build/source/
runtime identities and complete representative boundary rows are kept in Git.

The separate native ARM archive is `ht245-arm64-qualification-evidence.zip`
(9,502,875 bytes), SHA-256
`9d444d355cad23292a72ce78339bf864877ea0508f58ca837938bcb1a6c4d29f`.
Its [archive receipt](results/correction/arm-archive.json) records owner-only
Library item `libfile_36fd4acce9988191ae2e5db17e879a74`, version 0. The ARM summary
inventories all 85 transferred files (84 evidence entries plus their inventory); build caches/binaries and unrelated
host logs are excluded. The source/build/runtime identities and full boundary rows
are separately reviewable in Git. Neither Library artifact is a public link.

The numeric inputs, compiler-input content, harness and executable tools must
match across bundles. Native runtime OS/architecture, actual compiler and runner
remain distinct identities. Reports, result archives and later evidence-only
commits are not executable inputs; their final commit can differ from the
executed tool commit without claiming that later report text was executed.

## Exact-source admission

[Source-admission proof](results/correction/source-admission.json) records the
three actual Go1.22.12 production dependency closures and all 21 unchanged
instruction hashes/counts before admission. All 261 previously admitted source
hashes match. Only these three entries were added to the top-level `sources` map:

- `investigations/ht245-broker-arithmetic/harness/main.go`
- `investigations/ht245-broker-arithmetic/micro/main.go`
- `investigations/ht245-broker-arithmetic/probe/main.go`

The manifest schema, compiler/build configuration, source parents, three runtime
build-input closures, accounting schedules and all instruction pins are unchanged.
None of the investigation executables enters a production runtime closure. The
exact-source validator and scanner are unmodified; unknown or altered inputs
continue to fail qualification.

## Decision and remaining production work

The confirmed decision flips are confined to the three constructed
`channelBreakHold` cases (the fixture uses the channel 0.5 ATR / 1R policy);
serialized numeric path differences also occur in other families. Full Sequential
[dispatches to its own scheduler](../../engine/runner.go#L50), without those
channel helpers, while sharing broker fill, cost and exit accounting. That
accounting has a separate [exact pinned per-target contract](../../docs/sequential-metrics-contract.md#L159)
and 88-case native/WASM qualification. The relevant production source matches
`v0.30.0`. These findings do not establish a new Sequential decision defect or
clear broader release or consumer gates; they are not an all-strategy portability
claim.

Leave production arithmetic unchanged in this investigation. The current evidence
justifies a separate reviewed decision about the channel architecture constants,
followed by a narrowly scoped correction and conformance-impact assessment if the
owner selects one. It does not authorize broad broker splits, golden regeneration,
runtime fingerprint renewal, releases or consumer pin changes.

Dedicated Master, Regime, gold-flag, adaptive-flag and range-reversion engine routes
were not newly exercised by this harness. The Master quantity witness establishes
possibility at the expression level only. No historical-market or economic
performance conclusion is drawn from these invented cases. The pinned JS reference
work in HT-228/HT-229 remains separate; this cross-target harness does not claim a
new broad JavaScript oracle qualification.

## Validation

[Independent executable review](results/correction/independent-executable-review.json)
approved the frozen tools after reproducing the stale-output and cross-source
generator protections, concurrent exclusive-output ownership, and an actual
25-case native/WASM probe (one success and 24 declared refusals on each target).
Its initially oversized inherited environment correctly failed the Go1.22 WASM
loader; a recorded minimal-environment retry passed. No failed attempt was relabelled.

[Independent ARM review](results/correction/independent-arm-review.json) validated
all 12 returned production/diagnostic envelopes, all 84 archived evidence entries
and strict comparisons against each of the three local references.

Correction checks: twelve investigation Node regressions, 19 unchanged exact-source
proof regressions, Go1.22.12 formatting, vet, full tests and conformance
(135 parse goldens and 109 run goldens) passed locally. All three production
closures and all 21 instruction hashes match. The unchanged full Sequential
qualification now passes all 88 cases with 64
raw arithmetic pairs, 47,740 exact arithmetic checks, 92,156 float-bit checks and
zero unexplained mismatches. Its codegen gate admits exactly 264 source inputs.
The full fresh AMD64v1/v3 and NodeWASM matrix passed with the bound counts above.
Actual native ARM64 production and diagnostic execution also passed their
complete expected case sets, with the measured differences above and no unexpected
errors. These are investigation results, not a production arithmetic correction.
Exact-head normal PR CI and independent review remain integration gates; local tests are
not a substitute for them.
