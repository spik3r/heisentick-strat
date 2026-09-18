# T-B2 extraction plan and dry-run results

Ticket T-B2 (`backlog/plans/2026-09-18-one-engine-migration.md`, A.4):
mechanical move of `strat/` + `go/` to heisentick-strat, tag `v0.1.0`,
heisentick pins it. Acceptance: corpus green in both repos at the tag; the
diff is relocation, import/path/adoption changes only; no semantic changes.

This page records what `scripts/extract.sh` does when run against the real
tree, and the ordered steps for the day the freeze is announced. Nothing has
moved yet; this PR is the rehearsal.

## Dry-run result

Source: fresh clone of `spik3r/heisentick` at `32bb7a5` (2026-09-18).
Baseline in the source: `go build ./...`, `go vet ./...`, `go test ./...`
all pass (11 packages). Rehearsed against a copy of that clone into a scratch
directory; the source was untouched afterwards (`git status` empty).

### After the pure rewrite (copy + import rewrite only)

| Check | Result |
|---|---|
| `gofmt -l` | 1 file (`cmd/enginewasm/bridge.go`): import order changes because `github.com/...` sorts before `heisentick/...` did. `gofmt -w` fixes it. |
| `go vet ./...` | pass |
| `go build ./...` | **pass** (9 packages) |
| `go test ./...` | 3 packages pass (`contextcols`, `data`, `marketdata`); 6 fail |
| `GOOS=js GOARCH=wasm go build ./cmd/dslwasm`, `./cmd/enginewasm` | pass |
| `conformance/` vs `strat/conformance/` | byte-identical (`diff -r`) |

Content diff versus the source, excluding renames: `engine/` 310 lines
(155 import lines, two per diff side), `dsl/` 94 (45 package lines plus the
kept facade test's import), `cmd/` 60, `contextcols/` 20. All of it is
import or package lines.

### Test failures after the pure rewrite, by cause

| Class | Failing | Cause | Fix (applied by `--fix-paths`) |
|---|---|---|---|
| A. Repo-root marker | 5 packages abort at the first call: `cmd/dslwasm`, `cmd/heisentick`, `dsl`, `engine` panic in `testsupport.MustRepoRoot()`; `testsupport` itself fails `TestRepoRootAndConformanceRootIgnoreWorkingDirectory`. Every corpus-driven test sits behind this panic, so the package-level count understates it. | `testsupport.RepoRoot()` walks up until a directory holds both `package.json` and `go.mod`. This repo has no `package.json`. `StratConformanceRoot()` also appends `strat/conformance`. | `reporoot.go`: marker is `go.mod` only; corpus root is `<root>/conformance`. `reporoot_test.go`: root is `..` from the helper (was `../..`), alternate cwd is `cmd/` (was `go/`), expected corpus path drops `strat/`. Two files, 6 lines. |
| B. Hard-coded relative corpus path | `cmd/enginewasm` `TestBridgeMatchesFixtureLoader` (0 fixtures found) | `filepath.Glob("../../../strat/conformance/run/deployed-*.fixture.json")` counted directory depth from `go/cmd/enginewasm`. | `../../conformance/run/...`. One line. |
| C. Spec schemas path | `dsl`: `TestVersionedStratContractSchemas`, `TestGoSchemaValidationRejectsMalformedContractMutations`, `TestGoSchemaValidationRejectsUnsupportedSchemaCombinations`, `TestGoParseProjectionSatisfiesParseResultSchema` (4 tests, visible once class A is fixed) | `contract_schemas_test.go` reads `<root>/strat/specification/schemas`. | `<root>/spec/schemas`. One line. |
| D. Tests read app strategy sources | `engine`: `TestArchivedFailedBreakoutLimitEntryParams`, `TestArchivedRoundNumberConfluenceGradesReachEngineParams` (2 tests, visible once class A is fixed) | `os.ReadFile("../../strategies/source/<id>.strat")`; `strategies/source/` is the app's and does not move. | Copy the two files to `engine/testdata/strategies/` and read from there. Both strategies are archived (`engine/strategies/archiveManifest.js`), so the frozen copy cannot drift from a live source. Alternative considered: a `STRAT_SOURCE_ROOT` env var with `t.Skip` when unset. Rejected: it makes two tests silently optional in CI. |
| E. Embeds | none | No `//go:embed` in the moved tree. | — |
| F. Tests shelling to pnpm/node | none | No `exec.Command` in the moved tree. The reverse exists: 6 JS tests and checks in the app read Go source files or build Go binaries (inventory, "App-side importers"). | app-side PR |

Total: 11 changed lines in 6 files plus 2 copied fixtures. With
`--fix-paths`, `gofmt`, `go vet`, `go build` and `go test ./...` all pass
(9 packages), and both WASM targets build.

Not exercised in this rehearsal: `pnpm run parity:engine` against the
release binary (needs the app's data and the app-side PR), and CI on the
self-hosted pool (this repo has no runner yet; see the decision log,
"CI runners for new repos").

## Ordered T-B2 steps

Pre-conditions: T-A1 done (contracts `v0.1.0` exists; nothing in the moved
tree imports it yet, so there is no `go.sum` to add); S1 done (#93); open
questions below answered; heisentick `main` green.

1. **Freeze notice.** Add to `backlog/plans/IN-PROGRESS.md` Active Claims:

   > | `<date>` AEST | `<agent>` / one-engine-t-b2 | **Extraction freeze**: `strat/` and `go/` are frozen from now until the T-B2 pair of PRs merges. Do not open PRs that touch those trees; rebase outstanding branches onto the new tree afterwards. Tracking: heisentick-strat PR `<n>`, heisentick PR `<n>`. | Claimed |

   Post the same text in the lead's channel. Record the frozen commit SHA in
   both PR descriptions.

2. **Fresh clone at the frozen SHA.** `git clone --depth 1 --branch main
   https://github.com/spik3r/heisentick <tmp>`; confirm `git rev-parse HEAD`
   equals the SHA in the notice. Never run the script against a working
   checkout.

3. **Copy and rewrite.** On a `codex/t-b2-extraction` branch of this repo:
   `scripts/extract.sh --apply --fix-paths <tmp> .`. Review the output: the
   import table must match `docs/layout.md`; `gofmt` must list at most
   `cmd/enginewasm/bridge.go`; `go test ./...` must pass. Commit in two
   commits so the reviewer can diff them separately: (a) copy + import
   rewrite (`git diff --stat -M` should show renames plus import lines
   only), (b) the `--fix-paths` changes and the two fixture copies.

4. **Repo docs.** Update `README.md` (layout, commands), delete
   `docs/extraction-*.md` and `scripts/extract.sh` in the same PR or the one
   after (they have served their purpose), and change `ci.yml` `paths:` if
   the mapping changed since this rehearsal. Open the PR, get the lead's
   review, squash-merge.

5. **Tag.** `git tag -a v0.1.0 -m "Extraction of strat/ and go/ from heisentick <sha>"`,
   push the tag. `release.yml` builds `heisentick-linux-arm64`,
   `heisentick-darwin-arm64`, `dslwasm.wasm`, `enginewasm.wasm`,
   `conformance.tar.gz` (which packs `conformance/` and `spec/`). Record the
   asset digests.

6. **App-side removal PR** (heisentick, one PR, same day):
   - `git rm -r go/ strat/implementations/server-runtime strat/conformance
     strat/specification strat/docs strat/examples go.mod`; rewrite
     `strat/README.md` to describe the browser runtime and tools that remain.
   - Pin: a `strat-release.json` (or the existing manifest pattern in
     `frontend/solid/public/dsl-wasm/manifest.json`) naming `v0.1.0` and the
     digests; a fetch script that downloads `conformance.tar.gz`, the
     `heisentick` binary for the host arch and `dslwasm.wasm`, verifies the
     digests and unpacks under an ignored `.strat-release/` directory.
   - Point the corpus readers at the unpacked directory
     (`checkConformanceCorpus.mjs`, `semanticFixturesCheck.mjs`,
     `checkEngineParity.mjs`, `engineOfRecordInventory.mjs`,
     `backendStrategyConformance.mjs`, `buildConformanceCorpus.mjs`,
     `test/dslConformance.test.mjs`, `test/dslSpecExamples.test.mjs`,
     `test/dslSpecFamilies.test.mjs`, `test/dslExamples.test.mjs`,
     `test/stratContractSchemas.test.mjs`, `test/backendStrategyCatalog.test.mjs`),
     `strat/tools/grammarManifest.mjs` at the unpacked `spec/manifest.json`,
     and `goReportEngine.mjs` / `buildDslWasmAssets.mjs` /
     `dslWasmBrowserCorpus.mjs` / `dslWasmPerformanceBaseline.mjs` at the
     downloaded binaries instead of `go build`.
   - Tests that read Go source as text (`dslGrammarTables`,
     `dslGrammarManifest`, `dslGrammarValidation`, `dslHandlerBindings`,
     `dslCompilerRelocation`, `dslWasmRuntimeBridge`): move the Go-file
     assertions to this repo as Go tests (Q3) or delete them; the JS-side
     assertions stay.
   - CI: delete `go.yml`; `engine-parity.yml` downloads the pinned binary
     instead of `go build` from `go/`; drop `setup-go` from `pr-checks.yml`
     once nothing shells to `go`; `api-build.yml` `paths:` loses `strat/**`
     unless the browser runtime should still trigger it; `test/workflowPaths.test.mjs`
     follows. `file-budget.json` loses its 8 `go/`/server-runtime entries;
     `checkPrEvidence.mjs` drops the Go evidence rule; `.gitignore` drops
     `go/cmd/heisentick/heisentick` and gains `.strat-release/`.
   - `agents.md`: Repo Layout and Validation Conventions stop describing
     `go/**`, the `go/dsl` facade rule and "mirror in
     `strat/implementations/server-runtime`"; parser changes now go
     producer → release → pin PR.
   - Docs: `docs/codebase-map.md`, `docs/architecture.md`,
     `docs/dsl-js-go-parity-workflow.md`, the 26 one-line compatibility pages
     under `docs/dsl-spec*` (point at the release or the strat repo URL),
     `skills/dsl-strategy-development/SKILL.md`.
   - Merge when `pnpm run ci` is green with the pinned assets. Clear the
     freeze row.

7. **Rollback.** Before the app PR merges: close it; the tag stays, harmless.
   After it merges: revert the app PR (one squash commit, so one revert);
   the Go tree returns at the frozen SHA and `go.yml` runs again. Do not
   delete the tag; cut `v0.1.1` if the extracted tree needs a fix. Any
   parser or engine change made in this repo between tag and revert must
   be re-applied to the app tree by hand, which is why the freeze covers both
   PRs and the window should be one day.

## Open questions for the lead

1. **Facades.** `go/dsl` and `go/engine` are alias-only packages preserving
   the `backtester/go/*` import path. The rehearsal drops them (their
   importers point at `dsl/` and `engine/` directly; the `go/dsl` contract
   test is kept). The only alternative that keeps them is a `compat/` tree
   nobody imports. Confirm dropping.
2. **Corpus regen between v0.1.0 and M2.** `buildConformanceCorpus.mjs
   --regen` needs the JS engine and stays in the app, but the goldens move
   here. Until M2's Go regen exists, a golden change means: regen in the app
   against the unpacked release, copy the changed files into a strat PR,
   release, pin. Acceptable as a temporary loop, or should regen output be
   blocked entirely until M2 (the plan says "freeze the prior goldens
   first")?
3. **Grammar tables and the manifest.** `spec/manifest.json` is the source
   for both `engine/dsl/grammar/*.generated.js` (app) and
   `dsl/grammar_tables_generated.go` (here). The generator
   (`strat/tools/grammarTables.mjs`) needs the JS compiler and stays in the
   app; five app tests diff generated Go text against the manifest. Proposal:
   the app reads the manifest from the pinned release; a manifest change is
   a strat PR that also commits the regenerated Go table (run the app's
   generator against the branch and paste); a Go test here hashes
   `spec/manifest.json` into the generated file's header so drift fails CI.
   The alternative is to leave `strat/specification/` in the app until M8
   and have this repo pin the manifest the other way round. Decide before
   step 3.
4. **`strat/docs` → `spec/`.** Merging docs and schemas into one directory
   follows the ticket brief and produces no filename clash, but the app's
   `test/dslSpecExamples.test.mjs` and `test/dslSpecFamilies.test.mjs`
   compile every ```dsl fence in those pages with the JS parser. After the
   move they run against the release copy; a spec-doc change in this repo is
   only checked against the JS parser once the app pins it. Is that
   acceptable, or should those two tests be ported to Go as part of M2?
5. **`cmd/heisentick` default data root.** Without `--data-root` the CLI
   uses `<nearest go.mod>/data`, which in this repo is the `data` Go
   package. Behaviour is unchanged by the move and every caller passes the
   flag. Leave as is (recommended, not T-B2's job) or make the flag
   required in `v0.1.0`?
6. **Release contents.** `release.yml` now builds three `heisentick`
   binaries (linux/arm64 for Lambda and the API image per D-6, linux/amd64
   because the GitHub-hosted fallback runner that `engine-parity.yml` may
   land on is amd64, darwin/arm64 for laptops), `dslwasm.wasm`,
   `enginewasm.wasm` (T-F0 needs it), `conformance.tar.gz` containing
   `conformance/` and `spec/`, and `SHA256SUMS`. Confirm the list, in
   particular whether `spec/` belongs in the corpus archive or in its own
   asset.
7. **Runner.** This repo needs its own self-hosted runner instance
   (`oracle-vm-4`?) and `CI_RUNNER_MODE=self-hosted` before `ci.yml` uses
   the pool; until then it runs on `ubuntu-24.04`. Who registers it, and
   before or after the tag?
