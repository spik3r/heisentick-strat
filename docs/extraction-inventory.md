# Extraction Inventory — T-B2 prep

Source repo: `heisentick` (app). Destination: `heisentick-strat` (this repo).

## Summary

| Category | Files | Size | Importers (app-side) | Owns after move |
|---|---|---|---|---|
| `go/` (all packages + cmd) | 177 .go + go.mod/go.sum + README | ~8 MB | 0 (all imports internal to go/) | `heisentick-strat` |
| `dsl-conformance/` (parse + run fixtures + semantic) | ~170 files | ~13 MB | 0 external importers; app scripts read via `pnpm run dsl:conformance` | `heisentick-strat` |
| `engine/dsl/` (JS parser, compiler, grammar, setups, spec) | ~30 JS files | ~712 KB | ~120 imports across app scripts, tests, frontend | App keeps; JS parser stays until M8 |
| `dist-site/strat/implementations/browser-runtime/` | ~30 JS files | ~236 KB | 0 (only re-exported from `engine/dsl/spec/`) | App keeps (M8) |
| `go.mod` / `go.sum` | 2 | ~5 KB | — | `heisentick-strat` |
| CI workflow `.github/workflows/go.yml` | 1 | 1.8 KB | Triggers on `go/**` | App rewrites to pin `heisentick-strat` tag |
| CI workflow `.github/workflows/engine-parity.yml` | 1 | 1.9 KB | Builds `go/cmd/btgo` binary | App rewrites to pin `heisentick-strat` tag |
| `file-budget.json` (go entries) | 1 | 8 entries | `pnpm run files:check` | Remove go/ entries from app after move |
| `package.json` scripts referencing `go/` | 0 direct | — | — | No changes needed (scripts call `go build` from `go/`) |
| App scripts importing `#engine/dsl/*` | ~80 files | — | — | App keeps; JS parser stays until M8 |

## Detailed tree: what moves

### go/ (all packages)

```
go/
├── go.mod              (module backtester/go, go 1.22)  → becomes github.com/spik3r/heisentick-strat
├── go.sum              → moves
├── README.md           → moves
├── cmd/
│   ├── btgo/           → cmd/heisentick (report CLI)
│   ├── dslwasm/        → cmd/dslwasm (WASM parser build)
│   └── heisentick/     → built binary (excluded from move; in .gitignore)
├── dsl/                → dsl (Go DSL parser)
├── engine/             → engine (Go strategy engine / native runner)
├── marketdata/         → marketdata (bar loader)
├── contextcols/        → contextcols (context column builder)
└── data/               → data (fixture loader)
```

No files outside `go/` import `backtester/go/` directly. All Go imports are internal to the `go/` module.

### dsl-conformance/ (corpus)

```
dsl-conformance/
├── metadata.json
├── README.md
├── parse/              (41 parse cases: .strat + .cfg.json)
├── run/                (29 run cases: .strat + .fixture.json + .trades.json)
└── semantic/           (12 semantic cases: fixture.json + expected.json + strategy.strat + RATIONALE.md)
```

Consumed by:
- `scripts/checkConformanceCorpus.mjs` (reads `dsl-conformance/`)
- `scripts/buildConformanceCorpus.mjs` (writes `dsl-conformance/`)
- `scripts/checkEngineParity.mjs` (reads `dsl-conformance/` for parity goldens)
- App CI via `pnpm run dsl:conformance`

No direct JS/Go imports; accessed via filesystem reads.

### engine/dsl/ (JS parser — STAYS in app until M8)

This directory stays in `heisentick` because:
1. It is the production parser used by the app's DSL editor, lint, embed, and conformance checks.
2. The plan says the JS engine is deleted only in M8 after WASM parity is verified.
3. Moving it now would break all app scripts that `import ... from '#engine/dsl/...'`.

After M8, `engine/dsl/` is deleted. The canonical source becomes `strat/` (spec, tools) and the Go parser in `heisentick-strat`.

### strat/ directory (from worktree — proposed layout)

The `strat/` directory in the worktree contains:
- `specification/` — language spec schemas, manifest
- `docs/` — DSL spec, phrase reference, getting started, spec families
- `examples/` — worked example `.strat` files
- `tools/` — grammar manifest, phrase catalog, LSP, grammar tables
- `conformance/` — parse, run, and semantic fixtures (this is the source of truth for `dsl-conformance/`)
- `implementations/server-runtime/` — Go parser (duplicate of `go/dsl/` in the worktree)
- `implementations/browser-runtime/` — JS parser (duplicate of `engine/dsl/spec/` in the worktree)

**Decision: browser-runtime does NOT move now.** Justification:
- The browser-runtime JS files are re-exported from `engine/dsl/spec/` which stays in the app.
- Moving them would break the `#engine/dsl/spec/*` import graph.
- The plan says JS is deleted in M8 after WASM parity. Moving it first adds churn with no benefit.
- The browser-runtime is ~30 small re-export shims; it moves as part of M8 cleanup.

### Files outside that import from moved trees

| File | References | Action at extraction |
|---|---|---|
| `.github/workflows/go.yml` | `go/**`, `go/go.mod` | Rewrite to `paths: [go/**]` in heisentick-strat; app drops the workflow |
| `.github/workflows/engine-parity.yml` | `go/go.mod`, builds `go/cmd/btgo` | App rewrites to build from `heisentick-strat` release artifact |
| `scripts/fileBudget.mjs` | Regex `/_generated\.go$/` | Keep (still valid for any local Go) |
| `scripts/checkConformanceCorpus.mjs` | `go/dsl/types.go` | App rewrites to `heisentick-strat` release artifact |
| `scripts/dslGrammarTables.mjs` | `go/dsl/grammar_tables_generated.go` | App rewrites or drops (grammar tables stay in strat) |
| `scripts/dumpContextColumns.mjs` | `go/contextcols/testdata/` | App rewrites or drops |
| `file-budget.json` | 8 `go/` entries | Remove those entries after move |
| `scripts/checkPrEvidence.mjs` | `go/` pattern match | Remove or update pattern |
| `scripts/lib/goReportEngine.mjs` | `go build` command | Rewrite to use `heisentick-strat` binary |
| `scripts/lib/backendStrategyConformance.mjs` | `go/engine/run_todo.go`, `go/engine/conformance_test.go` | Rewrite to `heisentick-strat` paths |
| App scripts importing `#engine/dsl/*` | ~120 imports | No change — JS parser stays until M8 |
