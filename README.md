# heisentick-strat

The Strat language and its Go engine: specification, language docs,
conformance corpus (parse, run and semantic fixtures), Go parser, backtest
engine, market-data reader, context columns, the `heisentick` CLI and the
WASM builds. Consumers pin tagged releases.

Status: pre-extraction. This repository holds the plan and the rehearsed
script for moving the tree out of `spik3r/heisentick` (ticket T-B2 of the
one-engine migration). No code has moved yet. See `docs/extraction-plan.md`.

## What this repo is

- `spec/`: the language specification (`dsl-spec.md`, one page per setup
  family), the grammar manifest and its schema, the parse/run/trade JSON
  schemas, getting-started and editor docs.
- `dsl/`: the Go Strat parser (today `strat/implementations/server-runtime`).
- `engine/`: the Go backtest engine (today `go/native`).
- `marketdata/`, `contextcols/`, `data/`: BTB1 decoding and column types,
  causal context columns, series path validation and loading.
- `cmd/heisentick`: report and grid CLI. `cmd/dslwasm`, `cmd/enginewasm`:
  the parser and full-engine WASM builds.
- `conformance/`: parse and run goldens, plus the hand-derived semantic
  fixture set. Read-only outside a reviewed regeneration.
- `examples/`: small compiling `.strat` files.

## What this repo is not

- No application code: UI, backend routes, dev server, Solid frontend.
- No JavaScript Strat compiler. `strat/implementations/browser-runtime/` and
  `strat/tools/` stay in the app until the JS engine is deleted (M8).
- No strategies. The canonical `.strat` sources live in the app's
  `strategies/source/`; two archived ones are copied here as engine test
  fixtures only.
- No market data and no infrastructure.

## Release and adoption

```
change here → tagged release (semver; CHANGELOG names the semantic change)
→ consumer PR bumps the pin (release tag + asset digests)
→ consumer CI runs against the pinned corpus → merge
```

Consumers (`heisentick`, `heisentick-strategy-validation`) read the release
assets, not the Go module: `heisentick-<os>-<arch>` binaries,
`dslwasm.wasm`, `enginewasm.wasm`, `conformance.tar.gz` (the `conformance/`
and `spec/` trees) and `SHA256SUMS`. No floating versions.

## Development

```sh
gofmt -l .          # must print nothing
go vet ./...
go test ./...
go build -trimpath -o cmd/heisentick/heisentick ./cmd/heisentick
GOOS=js GOARCH=wasm go build -trimpath -o cmd/dslwasm/dslwasm.wasm ./cmd/dslwasm
GOOS=js GOARCH=wasm go build -trimpath -o cmd/enginewasm/enginewasm.wasm ./cmd/enginewasm
```

Module `github.com/spik3r/heisentick-strat`, Go 1.22, no external
dependencies.

## Extraction docs

- `docs/extraction-inventory.md`: what moves, sizes, and every importer in
  the app that the removal PR has to touch.
- `docs/layout.md`: directory mapping, import-rewrite table, fixture paths.
- `docs/extraction-plan.md`: rehearsal results, ordered T-B2 steps, open
  questions.
- `scripts/extract.sh`: the script that does the move. Dry run by default.
