# Module Layout — heisentick-strat

Module: `github.com/spik3r/heisentick-strat`, go 1.22.

## Directory structure

```
heisentick-strat/
├── go.mod                  (module github.com/spik3r/heisentick-strat, go 1.22)
├── go.sum
├── dsl/                    (Go DSL parser — was go/dsl)
├── engine/                 (Go strategy engine — was go/engine)
├── marketdata/             (Go bar loader — was go/marketdata)
├── contextcols/            (Go context columns — was go/contextcols)
├── data/                   (Go fixture loader — was go/data)
├── cmd/
│   ├── heisentick/         (report CLI — was go/cmd/btgo)
│   └── dslwasm/            (WASM parser build — was go/cmd/dslwasm)
├── conformance/            (corpus: parse + run + semantic fixtures)
│   ├── parse/
│   ├── run/
│   └── semantic/
├── spec/                   (language spec schemas + manifest)
├── docs/                   (DSL spec, phrase reference, getting started)
├── examples/               (worked example .strat files)
├── tools/                  (grammar manifest, phrase catalog, LSP, grammar tables)
├── scripts/                (corpus regen, conformance check)
├── .github/workflows/
│   ├── ci.yml
│   └── release.yml
├── js/                     (proposed: npm package @heisentick/strat, future)
├── README.md
├── CHANGELOG.md
├── agents.md
├── claude.md
└── codex.md
```

## Package path mapping

Old path (in heisentick) | New path (in heisentick-strat) | Package name
---|---|---
`backtester/go/dsl` | `github.com/spik3r/heisentick-strat/dsl` | `dsl`
`backtester/go/engine` | `github.com/spik3r/heisentick-strat/engine` | `engine`
`backtester/go/marketdata` | `github.com/spik3r/heisentick-strat/marketdata` | `marketdata`
`backtester/go/contextcols` | `github.com/spik3r/heisentick-strat/contextcols` | `contextcols`
`backtester/go/data` | `github.com/spik3r/heisentick-strat/data` | `data`
`backtester/go/cmd/btgo` | `github.com/spik3r/heisentick-strat/cmd/heisentick` | `main`
`backtester/go/cmd/dslwasm` | `github.com/spik3r/heisentick-strat/cmd/dslwasm` | `main`

## Import rewrite rules

The extraction script (`scripts/extract.sh`) applies these `sed` substitutions to every `.go` file after the copy:

| Old import path | New import path | sed command |
|---|---|---|
| `"backtester/go/dsl"` | `"github.com/spik3r/heisentick-strat/dsl"` | `s\|"backtester/go/dsl"\|"github.com/spik3r/heisentick-strat/dsl"\|g` |
| `"backtester/go/engine"` | `"github.com/spik3r/heisentick-strat/engine"` | `s\|"backtester/go/engine"\|"github.com/spik3r/heisentick-strat/engine"\|g` |
| `"backtester/go/marketdata"` | `"github.com/spik3r/heisentick-strat/marketdata"` | `s\|"backtester/go/marketdata"\|"github.com/spik3r/heisentick-strat/marketdata"\|g` |
| `"backtester/go/contextcols"` | `"github.com/spik3r/heisentick-strat/contextcols"` | `s\|"backtester/go/contextcols"\|"github.com/spik3r/heisentick-strat/contextcols"\|g` |
| `"backtester/go/data"` | `"github.com/spik3r/heisentick-strat/data"` | `s\|"backtester/go/data"\|"github.com/spik3r/heisentick-strat/data"\|g` |
| `"backtester/go/testsupport"` | `"github.com/spik3r/heisentick-strat/testsupport"` | `s\|"backtester/go/testsupport"\|"github.com/spik3r/heisentick-strat/testsupport"\|g` |

Module declaration rewrite:

| Old | New | sed command |
|---|---|---|
| `module backtester/go` | `module github.com/spik3r/heisentick-strat` | `s\|^module backtester/go$\|module github.com/spik3r/heisentick-strat\|g` |

## go.mod

```go
module github.com/spik3r/heisentick-strat

go 1.22
```

No external dependencies. All imports are internal.

## JS side (future — not part of T-B2)

Under `js/` as npm package `@heisentick/strat`, exporting:
- Corpus loader (reads `conformance/` fixtures)
- Browser-runtime parser (if it moves at M8)

The `js/` directory is not created in T-B2. It is part of M8 (JS removal from app).

## Conformance corpus

The `conformance/` directory moves from `dsl-conformance/` in heisentick. After the move:
- `heisentick-strat/conformance/` is the canonical source
- App reads it via the `heisentick-strat` release asset, not a Go import
- Regeneration command: `go run ./scripts/buildConformanceCorpus.mjs --regen` (in heisentick-strat)
