# Target layout

Module `github.com/spik3r/heisentick-strat`, `go 1.22` (copied from the app's
`go.mod`). No external Go dependencies; the app has no `go.sum` and neither
will this repo until one is needed.

Source of truth for the mapping: `scripts/extract.sh` (`MAPPING`,
`IMPORT_RULES`). This page explains it. Measured against heisentick
`32bb7a5` (2026-09-18).

## Directory mapping

| heisentick (source) | heisentick-strat | Files | Note |
|---|---|---|---|
| `go/native/` | `engine/` | 116 | Already declares `package engine`; no rename. |
| `strat/implementations/server-runtime/` | `dsl/` | 46 | `package serverruntime` becomes `package dsl` (45 files, one line each). All its tests are in-package, so nothing else refers to the old name. |
| `go/dsl/facade_contract_test.go` | `dsl/facade_contract_test.go` | 1 | External test (`package dsl_test`) that locks the parser's public names. Kept; it now tests the implementation directly. |
| `go/dsl/{doc.go,facade.go,README.md}` | dropped | 3 | Alias-only facade that existed to preserve the `backtester/go/dsl` import path. Every importer is rewritten anyway, so the path it preserved no longer exists. |
| `go/engine/{facade.go,facade_test.go}` | dropped | 2 | Same: forwarders from `go/engine` to `go/native`. The test imports both packages, which become one path, so it cannot compile after the move. |
| `go/marketdata/` | `marketdata/` | 3 | |
| `go/contextcols/` | `contextcols/` | 17 | Includes `testdata/` (4 JSON parity fixtures, 840 KB). |
| `go/data/` | `data/` | 2 | |
| `go/testsupport/` | `testsupport/` | 2 | Repo-root and corpus discovery; needs the path fix below. |
| `go/cmd/heisentick/` | `cmd/heisentick/` | 19 | Includes `goldens/` (package-relative, unchanged). |
| `go/cmd/dslwasm/` | `cmd/dslwasm/` | 4 | |
| `go/cmd/enginewasm/` | `cmd/enginewasm/` | 4 | |
| `strat/conformance/` | `conformance/` | 245 | `parse/`, `run/`, `semantic/`, `README.md`. Byte-identical after the move (checked with `diff -r`). |
| `strat/specification/` | `spec/` | 7 | `manifest.json`, `manifest.schema.json`, `schemas/`. |
| `strat/docs/` | `spec/` | 30 | `dsl-spec.md`, `dsl-spec-families/`, getting started, phrase reference, editor setup, parity workflow. No filename collides with the specification files. |
| `strat/examples/` | `examples/` | 3 | |
| `go.mod` | `go.mod` | 1 | Rewritten: module line only. |

Stays in the app (see `extraction-inventory.md` for why):
`strat/implementations/browser-runtime/`, `strat/tools/`, `strat/README.md`,
`go/README.md` (its content is superseded by this repo's README).

Result: 207 Go files, 9 packages, about 18 MB, of which 15 MB is the run
corpus.

```
heisentick-strat/
├── go.mod
├── cmd/{heisentick,dslwasm,enginewasm}/
├── engine/            <- go/native
├── dsl/               <- strat/implementations/server-runtime
├── marketdata/ contextcols/ data/ testsupport/
├── conformance/{parse,run,semantic}/
├── spec/              <- strat/specification + strat/docs
├── examples/
├── docs/              this repo's own docs (extraction, layout)
├── scripts/extract.sh
└── .github/workflows/{ci,release}.yml
```

## Import rewrite table

Applied with `perl -pi` on every `*.go` under the destination. Counts are
files touched in the destination (facade files are gone by then).

| Old import | New import | Files |
|---|---|---|
| `"heisentick/go/native"` | `"github.com/spik3r/heisentick-strat/engine"` | 2 |
| `"heisentick/go/engine"` | `"github.com/spik3r/heisentick-strat/engine"` | 9 |
| `"heisentick/go/dsl"` | `"github.com/spik3r/heisentick-strat/dsl"` | 73 |
| `"heisentick/strat/implementations/server-runtime"` | `"github.com/spik3r/heisentick-strat/dsl"` | 0 (only the dropped facade imported it) |
| `"heisentick/go/marketdata"` | `"github.com/spik3r/heisentick-strat/marketdata"` | 83 |
| `"heisentick/go/contextcols"` | `"github.com/spik3r/heisentick-strat/contextcols"` | 27 |
| `"heisentick/go/data"` | `"github.com/spik3r/heisentick-strat/data"` | 1 |
| `"heisentick/go/testsupport"` | `"github.com/spik3r/heisentick-strat/testsupport"` | 5 |
| `package serverruntime` (line 1 of each `dsl/*.go`) | `package dsl` | 45 |

The two `native "…/engine"` aliases (`cmd/enginewasm/bridge.go`,
`bridge_test.go`) keep their alias; the package is named `engine` so the
alias is now redundant but harmless. `gofmt` reorders one import block
(`cmd/enginewasm/bridge.go`) because the new paths sort differently; the
script runs `gofmt -w` on whatever `gofmt -l` reports.

No file imports both `go/engine` and `go/native` except the dropped
`go/engine/facade_test.go`, so merging the two paths creates no duplicate
imports. The script fails if any `"heisentick/` import string survives.

## Where tests find fixtures after the move

Every test that reads from outside its own package goes through one of
these. The pure rewrite leaves them pointing at the app's layout; the
`--fix-paths` stage of `scripts/extract.sh` applies the changes below.

| File | Before | After |
|---|---|---|
| `testsupport/reporoot.go` `RepoRoot()` | walks up until a dir has both `package.json` and `go.mod` | `go.mod` only (this repo has no `package.json`) |
| `testsupport/reporoot.go` `StratConformanceRoot()` | `<root>/strat/conformance` | `<root>/conformance` |
| `testsupport/reporoot_test.go` | expects root at `../..` from the helper, chdirs to `<root>/go`, expects `strat/conformance` | `..`, `<root>/cmd`, `conformance` |
| `cmd/enginewasm/bridge_test.go` | `../../../strat/conformance/run/deployed-*.fixture.json` | `../../conformance/run/deployed-*.fixture.json` |
| `dsl/contract_schemas_test.go` | `<root>/strat/specification/schemas` | `<root>/spec/schemas` |
| `engine/sweep_rule_grade_test.go`, `engine/limit_entry_test.go` | `../../strategies/source/{dslRoundNumberConfluence,dslFailedBreakoutLimitEntry}.strat` (app files, not moved) | `testdata/strategies/<same>.strat`, frozen copies; both strategies are archived in the app (`engine/strategies/archiveManifest.js`) |

Callers that already use `testsupport.StratConformanceRoot()` or
`MustRepoRoot()` (`cmd/dslwasm/envelope_test.go`,
`cmd/heisentick/main_test.go`, `engine/conformance_test.go`,
`dsl/conformance_fixtures_test.go`, `dsl/contract_schemas_test.go`) need no
edit beyond the helper change. `cmd/heisentick/data_root_test.go` writes its
own temporary `go.mod` and is unaffected. `cmd/heisentick/goldens/` and
`contextcols/testdata/` are package-relative and move with their packages.

## CLI data root

`cmd/heisentick` resolves `--data-root` by walking up from the working
directory to the nearest `go.mod` and appending `data/`
(`cmd/heisentick/data.go` `findRepoRoot`). In this repo that resolves to
`<repo>/data/`, which is the Go package, not a market-data directory. The
behaviour is unchanged by the move; callers in the app already pass
`--data-root` explicitly (`scripts/lib/goReportEngine.mjs`). Whether the
default should change is an open question in `extraction-plan.md`.
