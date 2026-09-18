# heisentick-strat

Strat language and Go strategy engine: spec, docs, semantic fixtures,
Go-authoritative conformance corpus and regen command, Go parser, engine,
shared report and validation packages, market-data reader, context columns,
`heisentick` CLI, and WASM build.

## What this repo is

- The Strat language specification and documentation
- The Go parser, engine, and CLI (`heisentick report|grid`)
- The conformance corpus (parse + run + semantic goldens)
- WASM build of the parser
- Tagged releases consumed by `heisentick` and `heisentick-strategy-validation`

## What this repo is NOT

- No app code (UI, backend routes, frontend, server)
- No AWS infrastructure (Terraform, Lambda, Step Functions)
- No strategy sources (`.strat` files live in the app's `engine/strategies/dsl/`)
- No market data (candle files, manifests)
- No deployment configuration

## Release and adoption

Consumers pin version tags. Semantic changes bump minor and are named in
CHANGELOG.md. No floating versions (`latest`, branch refs). Each dependency
bump is one consumer PR owned by that consumer's lane.

## Development

```bash
# Run tests
go test ./...

# Format check
gofmt -l .

# Vet
go vet ./...

# Build CLI
go build -trimpath -o cmd/heisentick/heisentick ./cmd/heisentick

# Build WASM parser
GOOS=js GOARCH=wasm go build -trimpath -o cmd/dslwasm/dslwasm.wasm ./cmd/dslwasm
```

## Repository layout

- `dsl/` — Go DSL parser
- `engine/` — Go strategy engine
- `marketdata/` — Go bar loader
- `contextcols/` — Go context column builder
- `data/` — Go fixture loader
- `cmd/heisentick/` — report CLI
- `cmd/dslwasm/` — WASM parser build
- `dsl-conformance/` — machine-checked DSL corpus
- `spec/` — language spec schemas
- `tools/` — grammar manifest, phrase catalog, LSP, grammar tables
- `docs/` — reference material
- `scripts/` — corpus regen, conformance check
