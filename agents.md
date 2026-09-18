# Agent Instructions

This file is the single source of truth for AI-agent instructions in this
repository. Other harness files should point here instead of duplicating rules.

## What this repo is

`heisentick-strat` owns the Strat language and Go strategy engine: spec,
docs, semantic fixtures, Go-authoritative conformance corpus and regen command,
Go parser, engine, shared report and validation packages, market-data reader,
context columns, `heisentick` CLI, and WASM build. Tagged releases.

Consumers (`heisentick`, `heisentick-strategy-validation`) pin a version tag.

## What this repo is NOT

- No app code (UI, backend routes, frontend, server)
- No AWS infrastructure (Terraform, Lambda, Step Functions)
- No strategy sources (`.strat` files live in the app's `engine/strategies/dsl/`)
- No market data (candle files, manifests)
- No deployment configuration

## Release and adoption rule

Consumers pin tags. Semantic changes bump minor and are named in CHANGELOG.md.
No floating versions (`latest`, branch refs). Each dependency bump is one
consumer PR owned by that consumer's lane.

## Baseline Workflow

1. Start with `git status --short` and preserve user edits.
2. Prefer small, coherent, validated changes.
3. Run `go test ./...` and `gofmt -l` after changes.
4. Commit coherent slices when the user asks for committed progress.

## Validation Conventions

- Always run `git diff --check` before committing.
- Run `gofmt -l .` before committing.
- Run `go vet ./...` before committing.
- Run `go test ./...` before committing.
- Conformance corpus must remain green: `go run ./scripts/checkConformanceCorpus.mjs`

## Repo Layout

- Root `*.md`: only `README.md`, `CHANGELOG.md`, and agent pointers
  (`agents.md`, `claude.md`, `codex.md`).
- `docs/`: reference material (layout, extraction plan, inventory).
- `dsl/`: Go DSL parser.
- `engine/`: Go strategy engine.
- `marketdata/`: Go bar loader.
- `contextcols/`: Go context column builder.
- `data/`: Go fixture loader.
- `cmd/`: CLI binaries (`heisentick`, `dslwasm`).
- `dsl-conformance/`: machine-checked DSL corpus (parse + run + semantic goldens).
- `spec/`: language spec schemas and manifest.
- `tools/`: grammar manifest, phrase catalog, LSP, grammar tables.
- `scripts/`: corpus regen, conformance check, extraction script.
