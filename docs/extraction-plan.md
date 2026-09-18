# Extraction Plan — T-B2

Ticket: T-B2 (Lane B Go core & reporting)
Plan reference: `2026-09-18-one-engine-migration.md` Appendix A.4

## Goal

Mechanical move of `strat/` + `go/` from heisentick to heisentick-strat, tag v0.1.0, heisentick pins it. No semantic changes. Corpus green in both repos at the tag.

## Pre-conditions

1. Freeze announced in `heisentick/plans/IN-PROGRESS.md`.
2. Spike S1 (WASM) and S2 (ARM Lambda) complete with numbers.
3. Contracts v0.1 shape approval from lead (T-A1).
4. `heisentick` CI green on current main.

## Ordered steps

### Step 1: Freeze notice (in heisentick)

Add to `heisentick/plans/IN-PROGRESS.md`:

```
| <date> | <agent> | T-B2: Strat + Go extraction freeze | Freeze on strat/ and go/ trees; no edits to go/ or dsl-conformance/ until extraction PR lands |
```

### Step 2: Create heisentick-strat scaffold (done)

This repo already has go.mod, agents.md, claude.md, codex.md, README.md, CI workflows, docs, and scripts/extract.sh.

### Step 3: Copy Go tree

| Source (heisentick) | Destination (heisentick-strat) |
|---|---|
| `go/dsl/` | `dsl/` |
| `go/engine/` | `engine/` |
| `go/marketdata/` | `marketdata/` |
| `go/contextcols/` | `contextcols/` |
| `go/data/` | `data/` |
| `go/cmd/btgo/` | `cmd/heisentick/` |
| `go/cmd/dslwasm/` | `cmd/dslwasm/` |
| `go/go.mod` | `go.mod` (rewrite module) |
| `go/go.sum` | `go.sum` |

### Step 4: Copy conformance corpus

`dsl-conformance/` stays as `dsl-conformance/`.

### Step 5: Rewrite Go import paths

sed substitutions: `backtester/go/*` to `github.com/spik3r/heisentick-strat/*`.

### Step 6: Fix test fixture paths

Tests under `cmd/` use 3 levels of `..` (from `go/cmd/`). After flattening they need 2 levels. Tests under `engine/` and `dsl/` use 2 levels and stay correct. Two tests read `engine/strategies/dsl/*.strat` from the app and need a `STRAT_SOURCE_ROOT` env var or skip.

### Step 7: Validate

gofmt, go vet, go build, go test.

### Step 8: Commit and tag v0.1.0

### Step 9: App-side removal PR (in heisentick)

Remove go/, dsl-conformance/, go.yml, engine-parity.yml. Update scripts to use heisentick-strat release artifact. Pin heisentick-strat v0.1.0.

### Step 10: CI changes in app

The app CI no longer runs `go test` from `go/`. Instead it builds the btgo binary from heisentick-strat release and runs parity checks.

### Step 11: Rollback

If extraction breaks app CI: revert the app-side PR, re-add `go/` and `dsl-conformance/` from main. heisentick-strat stays as-is (it does not affect the app until pinned).

## Estimated diff size

- heisentick-strat: ~200 Go files, ~170 conformance files, ~8 MB total
- heisentick (removal PR): ~200 files deleted from go/, ~170 from dsl-conformance/, ~10 files modified (scripts, workflows, configs)

## Open questions for the lead

1. **Strategy source fixtures**: `engine/limit_entry_test.go` and `engine/sweep_rule_grade_test.go` read `.strat` files from `engine/strategies/dsl/` (in the app). Should we: (a) copy those 2 `.strat` files to heisentick-strat, (b) add a `STRAT_SOURCE_ROOT` env var, or (c) skip those tests in heisentick-strat?

2. **Browser-runtime JS parser**: Does `strat/implementations/browser-runtime/` move now or stay until M8? Recommendation: stays. The JS parser is re-exported from `engine/dsl/spec/` which stays in the app. Moving it first adds churn with no benefit.

3. **go.sum**: Should we vendor dependencies or keep the sum file minimal? Currently the Go tree has zero external dependencies.

4. **Tag naming**: Should the first tag be `v0.1.0` or `v0.0.1`? The plan says `v0.1.0`.

5. **Corpus as release asset**: The app will consume the corpus as a `conformance.tar.gz` release asset, not a Go import. The release workflow already builds this. Confirm the app-side download mechanism.

6. **CLI binary naming**: The report CLI is currently `btgo`. Should it be renamed to `heisentick` in the new repo? The extraction script already renames `go/cmd/btgo/` to `cmd/heisentick/`.

7. **Forward runtime**: The plan says the forward runtime moves to a native Go binary (D-6). This is a separate ticket, not part of T-B2. Confirm.

8. **file-budget.json**: The 8 `go/` entries in heisentick's `file-budget.json` need to be removed in the app-side PR. Confirm timing.
