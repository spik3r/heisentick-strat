# Extraction inventory

What moves from heisentick to heisentick-strat in T-B2, what stays, and who
outside the moved trees depends on each path. Measured on a fresh clone of
`spik3r/heisentick` at `32bb7a5` (2026-09-18, "Decision log: contracts
v0.1.0 conventions, runner-per-repo"). Sizes are `du -sk`; importer counts
are files outside `strat/` and `go/` that mention the path (grep, excluding
`node_modules`). Breakdown by kind follows the table.

## Trees

| Path | Size | Files | Importers outside `strat/`+`go/` | Owner after T-B2 |
|---|---|---|---|---|
| `go/native/` | 844 KB | 116 | 18 (docs, backlog, `file-budget.json`, `go.yml`, `checkPrEvidence.mjs`) | strat: `engine/` |
| `strat/implementations/server-runtime/` | 304 KB | 46 | 18: 5 JS tests read Go source files (below), `checkConformanceCorpus.mjs` reads `types.go`, `strat/tools/grammarTables.mjs` writes `grammar_tables_generated.go`, `file-budget.json` (2 entries), `go.yml`, docs | strat: `dsl/` |
| `go/dsl/` | 16 KB | 4 | 17 (docs mention the facade; `agents.md` "preserve the `go/dsl` compatibility facade") | strat: 1 test kept in `dsl/`, facade dropped |
| `go/engine/` | 8 KB | 2 | 12 (docs, plan) | dropped (facade) |
| `go/marketdata/` | 12 KB | 3 | 2 | strat |
| `go/contextcols/` | 864 KB | 17 | 7 (`scripts/data/dumpContextColumns.mjs` writes its `testdata/`) | strat |
| `go/data/` | 8 KB | 2 | 2 | strat |
| `go/testsupport/` | 8 KB | 2 | 1 | strat |
| `go/cmd/heisentick/` | 164 KB | 19 | 9: `scripts/lib/goReportEngine.mjs` (binary path default `go/cmd/heisentick/heisentick`), `checkEngineParity.mjs`, `test/strategyReportGoEngine.test.mjs`, `engine-parity.yml`, `go.yml`, `.gitignore`, docs | strat: `cmd/heisentick/`; app consumes the release binary |
| `go/cmd/dslwasm/` | 16 KB | 4 | 3: `frontend/solid/scripts/buildDslWasmAssets.mjs` (builds the committed 4 MB `frontend/solid/public/dsl-wasm/*.wasm`), `scripts/dsl/dslWasmBrowserCorpus.mjs`, `scripts/dsl/dslWasmPerformanceBaseline.mjs`; plus `test/dslWasmRuntimeBridge.test.mjs` runs `go build ./cmd/dslwasm` | strat: `cmd/dslwasm/`; app consumes the release `.wasm` |
| `go/cmd/enginewasm/` | 16 KB | 4 | 4 (`scripts/spikes/enginewasm/*`, S1 report) | strat: `cmd/enginewasm/` |
| `go/README.md` | 8 KB | 1 | — (still says `module backtester`; stale) | superseded by this README |
| `go.mod` | 27 B | 1 | 6 (`go.yml`, `engine-parity.yml` cache keys; `checkPrEvidence.mjs`; plans) | strat (`go.sum` does not exist; no deps) |
| `strat/conformance/parse/` | 356 KB | 86 | 31 for `strat/conformance` as a whole: `checkConformanceCorpus.mjs`, `buildConformanceCorpus.mjs --regen`, `checkEngineParity.mjs`, `semanticFixturesCheck.mjs`, `engineOfRecordInventory.mjs`, `backendStrategyConformance.mjs`, `test/dslConformance.test.mjs`, `test/dslSpecExamples.test.mjs`, `test/backendStrategyCatalog.test.mjs`, `go.yml`, docs/plans | strat: `conformance/parse/`; app reads `conformance.tar.gz` from the release |
| `strat/conformance/run/` | 14.5 MB | 96 | (same) | strat: `conformance/run/` |
| `strat/conformance/semantic/` | 260 KB | 61 (15 cases) | `scripts/checks/semanticFixturesCheck.mjs` (`dsl:semantic:check` in `ci`) | strat: `conformance/semantic/` |
| `strat/conformance/README.md` | 4 KB | 1 | — | strat |
| `strat/specification/` | 116 KB | 7 | 11: `strat/tools/grammarManifest.mjs` (`GRAMMAR_MANIFEST_PATH`), `engine/dsl/phrases.js`, two generated files under `engine/dsl/grammar/`, `docs/dsl-phrase-catalog.*`, `test/stratContractSchemas.test.mjs`, `go.yml` | strat: `spec/` (see Q3) |
| `strat/docs/` | 216 KB | 30 | 43: 26 are one-line compatibility pages under `docs/dsl-spec-families/` and `docs/dsl-spec.md`; `test/dslSpecExamples.test.mjs` and `test/dslSpecFamilies.test.mjs` compile every ```dsl fence; `HelpPage.jsx`, `engine/dsl/scaffoldTemplate.js`, skills | strat: `spec/` (see Q4) |
| `strat/examples/` | 12 KB | 3 | 2 (`test/dslExamples.test.mjs`) | strat: `examples/` |
| `strat/tools/` | 40 KB | 5 | 6: `scripts/build/dslGrammar*.mjs`, `dslPhraseCatalog.mjs`, `scripts/dsl/dslLsp.mjs`, `test/stratToolsCompatibility.test.mjs` | **app** (imports `#strat/browser/*`) |
| `strat/implementations/browser-runtime/` | 236 KB | 29 | 49: 28 files under `engine/dsl/`, `package.json` (`#strat/browser/*`), `server.mjs`, `buildStaticSite.mjs`, `dslLint.mjs`, `checkImportBoundaries.mjs`, `app-deploy.yml` paths, 4 tests | **app** until M8 |
| `strat/README.md` | 4 KB | 1 | — | app (rewrite to describe what remains) |

Total moving: about 18 MB, 207 Go files, 245 corpus files, 37 spec/doc
files, 3 examples.

## Browser runtime and `strat/tools` stay

`strat/implementations/browser-runtime/` is the JavaScript Strat compiler.
It is imported by the JS engine (`engine/dsl/**` re-exports it through the
`#strat/browser/*` alias), the dev server, the static-site build, the lint,
the LSP and the phrase catalogue. The migration plan deletes the JS engine in
M8; until then the app runs on it. Moving it now would either split one
package.json workspace across two repos or force the app to vendor it back
in the same PR. Recommendation: it stays, with the plan's "pinned JS
compatibility package" reduced to "nothing moves until M8". `strat/tools/`
imports the browser compiler (`grammarCanonicalExamples.mjs`, `lsp.mjs`,
`phraseCatalog.mjs`), so it stays with it. Its two path constants
(`strat/specification/manifest.json`, `strat/implementations/server-runtime/grammar_tables_generated.go`)
are the coupling that T-B2 has to cut; see Q3.

## App-side importers by kind

The app-side removal PR has to touch each of these. Counts are files.

**JS reads Go source as text (break on removal, 5 tests + 1 check):**
`test/dslGrammarTables.test.mjs` (regenerates
`grammar_tables_generated.go` and diffs it), `test/dslGrammarManifest.test.mjs`
(reads `parser.go`, `grammar_tables_generated.go`, `types.go`),
`test/dslGrammarValidation.test.mjs`, `test/dslHandlerBindings.test.mjs`
(reads `handler_bindings.go`), `test/dslCompilerRelocation.test.mjs`,
`scripts/checks/checkConformanceCorpus.mjs` (reads family constants from
`types.go` for the coverage audit).

**JS builds or runs Go (7):** `scripts/lib/goReportEngine.mjs`
(`ENGINE_GO_BIN` or `go/cmd/heisentick/heisentick`),
`scripts/checks/checkEngineParity.mjs`,
`frontend/solid/scripts/buildDslWasmAssets.mjs`,
`scripts/dsl/dslWasmBrowserCorpus.mjs`,
`scripts/dsl/dslWasmPerformanceBaseline.mjs`,
`test/dslWasmRuntimeBridge.test.mjs` (`go build ./cmd/dslwasm`),
`scripts/spikes/enginewasm/build.mjs`.

**JS reads the corpus or spec (12):** listed in the table rows for
`strat/conformance` and `strat/specification`. After the tag they read the
same files from the unpacked release asset (`conformance.tar.gz`, which
should also carry `spec/`) at a path the app pins by digest.

**package.json:** `imports["#strat/browser/*"]` (stays); scripts
`dsl:conformance`, `dsl:semantic:check`, `parity:engine`, `dsl:lint:strict
--source=strat`, `ci` (unchanged names, changed inputs).

**Workflows:** `go.yml` (delete; replaced by "JS matches the pinned corpus"),
`engine-parity.yml` (`go build ./cmd/heisentick` from `go/` becomes a
release-binary download), `pr-checks.yml` (two `setup-go` steps become
unnecessary once no test shells to `go`), `api-build.yml` and
`app-deploy.yml` `paths:` filters (`strat/**`,
`strat/implementations/browser-runtime/**`; the latter stays valid),
`runner-canary.yml` (`setup-go`, unrelated).

**Check configs:** `file-budget.json` (8 entries: 6 under `go/`, 2 under
`strat/implementations/server-runtime/`), `scripts/checks/fileBudget.mjs`,
`scripts/checks/checkPrEvidence.mjs` (Go evidence rule matches `go.mod`,
`go/`, `server-runtime/`), `scripts/checks/checkImportBoundaries.mjs`
(`SCAN_ROOTS` includes `strat`; the `strat`→`engine` rule stays for the
browser runtime), `test/workflowPaths.test.mjs` (asserts the
`engine-parity.yml` build line), `test/fileBudget.test.mjs`.

**Docker:** `iac/api.Dockerfile.dockerignore` already excludes `go`; the API
image does not build Go. D-6 later adds a stage that copies the release
binary.

**Other:** `.gitignore` (`go/cmd/heisentick/heisentick`), `agents.md` (Repo
Layout, Validation Conventions for `go/**` and the facade rule),
`docs/codebase-map.md`, `docs/architecture.md`,
`docs/dsl-js-go-parity-workflow.md`, `skills/dsl-strategy-development/SKILL.md`,
`backlog/plans/*` (historical; leave).

## Not moving, on purpose

- `strategies/source/*.strat` (108 files) and `engine/strategies/*`: the
  app's strategies, not the language. Two engine tests read two archived
  sources; they get frozen copies under `engine/testdata/strategies/`.
- `data/`, `data-sample.zip`: market data. `cmd/heisentick` takes
  `--data-root`.
- `scripts/build/buildConformanceCorpus.mjs --regen`: generates the corpus
  from the JS engine. M2 replaces it with a Go regen command; until then the
  corpus is frozen at the tag and regen happens nowhere (see Q2).
- `scripts/checks/semanticFixturesCheck.mjs`: validates
  `conformance/semantic/` against its schema in Node. Keep running it in the
  app against the release asset until a Go equivalent exists.
