# Agent Instructions

This file is the single source of truth for AI-agent instructions in this
repository. `claude.md` and `codex.md` point here.

## What this repo is

`heisentick-strat` owns the Strat language and its Go engine: spec, docs,
conformance corpus (parse, run, semantic), Go parser, engine, market-data
reader, context columns, `heisentick` CLI and the WASM builds. It ships
tagged releases; `heisentick` and `heisentick-strategy-validation` pin them.

Until T-B2 lands the repo holds only the extraction plan and script. Read
`docs/extraction-plan.md` before touching anything.

## What this repo is not

No app code, no JavaScript compiler (stays in the app until M8), no
strategies (`.strat` sources live in the app's `strategies/source/`), no
market data, no infrastructure.

## Release and adoption

Consumers pin tags and asset digests. A semantic change bumps the minor
version and is named in `CHANGELOG.md`. No floating versions (`latest`,
branch refs). Each dependency bump is one consumer PR owned by that
consumer's lane. Cross-repo work lands producer → tagged release → consumer
pin PR; never assume two PRs merge atomically.

## Working rules

- Start with `git status --short` and preserve user edits.
- `codex/<short-topic>` branches; one ticket, one bounded write set, one PR.
- Conformance goldens are read-only. A golden changes only in a reviewed
  corpus-regeneration PR that says why. Parser-visible changes need a
  fixture or golden in the same PR.
- Engine semantics, schema changes and statistical definitions need an
  independent review.
- Commit messages explain the change. Run `git diff --check` first.

## Validation

Before every commit that touches Go:

```sh
gofmt -l .      # must print nothing
go vet ./...
go test ./...
```

Workflow changes: keep `runs-on` exactly
`${{ fromJSON(vars.CI_RUNNER_MODE == 'self-hosted' && '["self-hosted","linux"]' || '["ubuntu-24.04"]') }}`.
Self-hosted runners are registered per repository; this repo has none yet.

## Repo layout

Root `*.md`: `README.md`, `CHANGELOG.md`, `agents.md`, `claude.md`,
`codex.md` only. `docs/` holds this repo's own documents (extraction,
layout). After T-B2: `spec/`, `dsl/`, `engine/`, `marketdata/`,
`contextcols/`, `data/`, `testsupport/`, `cmd/`, `conformance/`,
`examples/`; see `docs/layout.md`.
