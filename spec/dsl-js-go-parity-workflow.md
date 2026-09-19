# JS and Go DSL parity workflow

The JavaScript implementation is the language authoring surface and corpus
generator. The Go implementation consumes the same committed conformance
fixtures. Parser-visible changes are complete only when both implementations
agree in the same pull request.

## Parity boundary

The maintained parity contract covers three boundaries:

1. **Parser output:** every `strat/conformance/parse/*.strat` source must produce
   the committed config, errors, warnings, and structured diagnostics in the
   matching `*.cfg.json`. `pnpm run dsl:conformance` checks the JavaScript side;
   `go test ./strat/implementations/server-runtime` from the repository root
   checks the canonical Go parser.
2. **Run corpus:** cases under `strat/conformance/run/` pin the source, bar
   fixture, execution options, and resulting trade envelope. The JavaScript
   checker and Go engine tests must reproduce the committed results for their
   supported cases.
3. **Context columns:** `scripts/data/dumpContextColumns.mjs` produces the committed
   JS reference fixtures under `go/contextcols/testdata/`, and
   `go/contextcols/parity_test.go` compares Go columns with those fixtures.

This is **not full JavaScript-runtime parity**. JavaScript-only runtime paths,
setup behavior outside the run corpus, chart integration, report formatting,
and every possible combination of config values are not guaranteed merely
because the parity gates pass. Add a focused run or context-column fixture when
a change needs to extend the guaranteed boundary.

## File pairing map

The files are paired by responsibility rather than line-for-line structure.
When one side changes, inspect every file in its row.

| Responsibility | JavaScript | Go |
| --- | --- | --- |
| Parse orchestration, defaults, versioning, validation, diagnostics | `strat/implementations/browser-runtime/compiler/compile.js`, `defaultConfig.js`, `diagnostics.js`, `directiveHeads.js` | `strat/implementations/server-runtime/parser.go`, `types.go` |
| Physical/logical lines, inline sections, normalization, tokenization | `strat/implementations/browser-runtime/compiler/compile.js`, `inlineDirectiveShape.js`, `lexicon.js`, `vocabulary.js` | `strat/implementations/server-runtime/parser_lexing.go`, `parser_values.go` |
| Market/session/filter gates and entry conditions | `strat/implementations/browser-runtime/compiler/parseFilters.js`, `sessionConfig.js`, `compile/gateAssembly.js` | `strat/implementations/server-runtime/parser_filters.go` |
| Levels and higher-timeframe resolution | `strat/implementations/browser-runtime/compiler/parseLevelLine.js`, `parseLevels.js`, `compile/levelResolution.js`; runtime resolver `engine/dsl/spec/levelResolver.js` | `strat/implementations/server-runtime/parser_filters.go`, `parser_setup_phrases.go`, `timeframe.go` |
| Setup type defaults and setup dispatch | `strat/implementations/browser-runtime/compiler/parseSetups.js`, `compile/setupLowering.js` | `strat/implementations/server-runtime/parser_setups.go`, `parser.go` |
| Range/break setup phrases | `strat/implementations/browser-runtime/compiler/parseSetups/rangeBreakPhrases.js` | `strat/implementations/server-runtime/parser_setup_phrases.go` |
| Continuation setup phrases | `strat/implementations/browser-runtime/compiler/parseSetups/continuationPhrases.js` | `strat/implementations/server-runtime/parser_setup_phrases.go` |
| Pattern setup phrases | `strat/implementations/browser-runtime/compiler/parseSetups/patternPhrases.js` | `strat/implementations/server-runtime/parser_setup_phrases.go` |
| Fade/exhaustion setup phrases | `strat/implementations/browser-runtime/compiler/parseSetups/fadePhrases.js` | `strat/implementations/server-runtime/parser_setup_phrases.go` |
| Stop, target, partial, breakeven, trail, hold, and risk lowering | `strat/implementations/browser-runtime/compiler/parseRiskManagement.js`, `compile/managementLowering.js` | `strat/implementations/server-runtime/parser_management.go`, `parser.go` |
| Parse corpus: Go generates, JS checks | `scripts/checks/checkConformanceCorpus.mjs` (conformer) | `cmd/conformance` (generator), `dsl/conformance_fixtures_test.go`, `dsl/conformance_test.go` |
| Run corpus: Go generates, JS checks | `engine/dsl/spec/runtime*.js`, `scripts/checks/checkConformanceCorpus.mjs` (conformer) | `cmd/conformance` (generator), `engine/conformance_test.go` and the relevant `engine/*.go` family/runtime files |
| Context-column reference generation | `engine/dsl` context builders used by `scripts/data/dumpContextColumns.mjs` | `go/contextcols/*.go`, `go/contextcols/parity_test.go` |

The `compile/` phase split is intentionally asymmetric with Go: level
resolution, setup lowering, gate assembly, and management lowering all feed the
single Go parser dispatch. The `parseSetups/` family split is the finer-grained
JS view of `parser_setups.go` (type/default setup state) plus
`parser_setup_phrases.go` (family phrase handlers).

The old `engine/dsl/spec/*.js` compiler paths remain as compatibility façades
for existing imports. They must not become a second implementation; new
browser compiler changes belong under
`strat/implementations/browser-runtime/compiler/`.

## Parser-change checklist

For any change that can alter config, errors, warnings, diagnostics, or source
positions:

1. Update the JavaScript and matching Go parser files in the same branch and
   pull request. The same rule applies when the change starts on the Go side.
2. Update the normative language docs under `strat/docs/dsl-spec*` when user-facing
   syntax or semantics change.
3. Add or adjust a parse fixture. If runtime behavior is part of the contract,
   add or adjust a run fixture too.
4. Regenerate only through the procedure below. Never hand-edit a generated
   config, fixture, trade file, or `strat/conformance/metadata.json`.
5. Review the generated diff before validation. Unrelated fixture churn means
   the regeneration is not ready to commit.
6. Run all JavaScript and Go gates listed under [Validation](#validation).

Internal refactors that cannot change parser output do not require a Go edit or
corpus regeneration, but the parity gates still guard that claim.

## Corpus regeneration and review

The Go engine in `heisentick-strat` generates the parse and run goldens; the
JavaScript runtime conforms to them. Start from a clean branch containing
only the intended parser/engine source and fixture edits. From the
`heisentick-strat` repository root run:

```bash
go run ./cmd/conformance check    # which goldens differ, and where
go run ./cmd/conformance regen    # rewrite parse and run goldens and metadata.json
git status --short -- conformance
git diff -- conformance/metadata.json conformance/parse conformance/run
```

Review every changed golden. Parse diffs must correspond to intended config or
diagnostic changes. Run diffs must correspond to intended execution changes;
unexpected trade, price, size, stop, target, reason, or timestamp changes are a
stop signal. Include the reviewed generated diff and the matching Go source
change in one narrow pull request whose body names the fixture or bug behind
each changed file. `regen` is an explicit reviewed operation, not a repair
command for a failing parity test, and it is never run from the JavaScript
side: a JS mismatch is a JS bug, a Go bug or an unresolved semantic, fixed at
the source. The app picks the new goldens up through a tagged release and a
`strat-release.json` bump. `conformance/README.md` has the full rule.

## Validation

Run from the repository root:

```bash
pnpm run dsl:lint:strict
pnpm run dsl:embed:check
pnpm run dsl:conformance
node --test test/engine.test.mjs test/dslDiagnostics.test.mjs
```

Then run the complete Go gate from `go/`:

```bash
gofmt -l .
go vet ./...
go test ./...
```

`gofmt -l .` passes only when it prints nothing. For a focused parser check,
run `go test ./strat/implementations/server-runtime` from the repository root;
for focused context parity, run
`go test ./contextcols`. Finish with `git diff --check` at the repository root.

### Optional browser WASM observer

The JavaScript parser remains authoritative. The browser observer compares the
Go/WASM worker with committed parse fixtures after the harness has checked those
same fixtures with the JavaScript parser and native Go envelope. It neither
captures editor source nor sends telemetry.

Build the current Solid artifact first, then run the preflight and serve the
test-only harness:

```bash
pnpm run frontend:build
node scripts/dsl/dslWasmBrowserCorpus.mjs --json
node scripts/dsl/dslWasmBrowserCorpus.mjs --serve --port=5197
```

Open the printed URL in a **fresh browser tab**. The URL must retain
`?dsl-wasm=1`; a passing page reports the fixture count in its result text.
The harness is served only by the local command and adds no product route,
global, or production-only parser API. If browser automation is unavailable,
record that limitation rather than claiming the browser observer ran; the Node
preflight is useful, but not a substitute for the browser worker check.

## Troubleshooting and ownership

- A JS conformance failure after an internal split usually means exports,
  defaults, diagnostics, or dispatch order changed. Compare the parse golden
  before considering regeneration.
- A Go parse mismatch reports the config or diagnostic envelope that diverged.
  Use the pairing table to update the corresponding Go handler; do not weaken
  the fixture assertion or regenerate around an unmirrored parser change.
- A run mismatch can come from parsing, context construction, setup execution,
  management, or broker semantics. First confirm parse parity, then isolate the
  first differing trade field. Extend the run corpus deliberately when a new
  runtime guarantee is required.
- A context-column mismatch should be investigated as a causal column
  implementation difference. Regenerate `go/contextcols/testdata/` only when
  the intended JS reference semantics changed, then review that diff just like
  other goldens.
- JavaScript parser authors own the corpus generation and review. Go parser or
  engine authors own consuming every committed applicable fixture. The pull
  request author owns proving both sides agree; a follow-up PR is not an
  acceptable home for the matching parser change.

See also `strat/conformance/README.md` for fixture schemas and coverage rules.
