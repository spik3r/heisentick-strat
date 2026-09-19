# Strat conformance corpus

Fixtures that pin what a Strat program means: what it compiles to (`parse/`),
which trades it produces on a frozen bar slice (`run/`), and the hand-derived
execution rules (`semantic/`). The Go engine in this repository generates the
parse and run goldens; every other implementation conforms to them.

## Who generates, who conforms

```
Go engine  --regen-->  conformance/{parse,run}/*.json  <--check--  JS runtime (heisentick), WASM builds
```

- **Go generates.** `go run ./cmd/conformance regen` writes every
  `parse/<case>.cfg.json`, every `run/<case>.trades.json` the engine
  implements, and the scoreboard in `metadata.json`. Nothing else writes
  these files. CI runs `go run ./cmd/conformance check`, so a golden and the
  engine cannot drift apart on `main`.
- **JS and WASM conform.** The app's `pnpm run dsl:conformance` runs its
  JavaScript runtime over the pinned release's corpus and must reproduce
  each golden exactly. WASM is compared the same way. A mismatch is triaged
  as a Go bug, a JS bug, a runtime difference or an unresolved semantic, and
  fixed at the source. It is never resolved by regenerating from JS.
- **`semantic/` is hand-derived.** No engine writes `expected.json` there;
  see `semantic/README.md`. Go is checked against those cases first, then JS
  and WASM against Go (plan principle 4: correctness, not agreement).

## Regeneration is a reviewed PR

A golden changes only in a pull request whose description names, per changed
file, the fixture change or the engine bug behind it. The steps:

1. Make the engine or fixture change on its own branch.
2. `go run ./cmd/conformance check` shows which goldens now differ and where
   the first differing byte is. Read each diff. An unexpected change to a
   trade, price, size, stop, target, reason or timestamp is a stop signal,
   not something to regenerate over.
3. `go run ./cmd/conformance regen`, then `git diff -- conformance/`.
   Every changed file must be explained in the PR body.
4. A semantic change bumps the minor version and gets a `CHANGELOG.md`
   entry (see `agents.md`, "Release and adoption").

`regen` refuses to write a golden for a run case listed as TODO in
`engine/run_todo.go`, and fails on a fixture that is in neither list, so a
new fixture is classified before it lands. `check` also fails when a golden
exists for a TODO case, because nothing would have generated it.

## Coverage classes

Coverage is reported in four classes and never summed
(`backlog/plans/2026-09-18-one-engine-migration.md`, "Correctness", in the
app repository):

| Class | Meaning | Where it shows |
|---|---|---|
| parsing | The program compiles to the expected config and diagnostics. | `parse/<case>.cfg.json`. A no-trade run golden counts here only. |
| executable | The route is admitted and a run golden exists. | `metadata.json`, `goRun.implemented`. |
| exercised | Run goldens with at least one trade per exit reason the strategy can produce. | The `reason` values across `run/<case>.trades.json`. |
| tested-unsupported | A route or feature is deliberately refused, with a fixture asserting the refusal. | `metadata.json`, `goRun.todo` names the case and the reason. |

## Layout

```
conformance/
├── README.md
├── metadata.json            counts, notes and the Go run scoreboard (generated)
├── parse/<case>.strat       input program
├── parse/<case>.cfg.json    golden: header plus cfg, errors, warnings, diagnostics
├── run/<case>.strat         strategy under test
├── run/<case>.fixture.json  bar slice and execution options (hand-maintained)
├── run/<case>.trades.json   golden trade list (generated)
└── semantic/                hand-derived fixture set A (see its README)
```

### `metadata.json`

```json
{
  "schema": "dsl-conformance-metadata-v1",
  "parseCases": 43,
  "parseCategories": { "deployed-book-dsl": 8, "diagnostic": 9, "setup-family": 26 },
  "runCases": 32,
  "goRun": {
    "implemented": ["deployed-dsl-dual-ema-resumption-xauusd-four-hour", "..."],
    "todo": { "<case>": "reviewed reason the engine does not run it yet" }
  },
  "missingDeployedDsl": ["..."],
  "notes": ["..."]
}
```

`goRun` is the scoreboard consumers read instead of assuming: every
`run/*.fixture.json` is in exactly one of the two lists. `regen` rewrites the
counts and `goRun`; `missingDeployedDsl` and `notes` are kept as they are.

### Parse cases

`parse/<case>.strat` is the input. `parse/<case>.cfg.json` is:

```json
{
  "schema": "dsl-conformance-parse-v1",
  "case": "case-name",
  "category": "setup-family | deployed-book-dsl | diagnostic",
  "description": "what the case pins",
  "source": "origin of the Strat text",
  "cfg": {},
  "errors": [],
  "warnings": [],
  "diagnostics": []
}
```

`case`, `category`, `description` and `source` are the header: they record
where the program came from and are kept by `regen`. `cfg`, `errors`,
`warnings` and `diagnostics` are the parser's output. Diagnostic cases carry
the structured error/warning mirror with `line` and `col`; spec-level
diagnostics use `null` for both.

### Run cases

`run/<case>.strat` is the strategy. `run/<case>.fixture.json` holds the
market slice and fixed execution options: symbol, timeframe, higher-timeframe
bars, range method, start equity, fill mode and slippage. Bar rows are
`[t, o, h, l, c, v]` with `t` in Unix milliseconds. Fixtures stay under 1 MB.

`run/<case>.trades.json` is the golden: the trade list the Go engine produces
against that fixture, with `tradeCount` and the fixture's identifying fields.
A conforming engine compiles the program, builds the same context from the
fixture bars, runs with the listed costs and reproduces the list exactly:
trade count, entry and exit index and timestamp, side, reason, size, stop and
target, and prices after the 15-significant-digit rounding the serializer
applies. The runtime-only `partial` flag is not part of the golden.

Case families: `deployed-*` are deployed-book strategies on their own route;
`family-*` give every setup family not already covered by a deployed case at
least one trade; `money-*` isolate risk sizing, partial exits and the
stop-distance gate on fixtures the other cases already use.

## Adding a case

Parse case:

1. Add `parse/<case>.strat`.
2. Add `parse/<case>.cfg.json` containing only the header fields (`case`,
   `category`, `description`, `source`).
3. `go run ./cmd/conformance regen` fills in the parser output.

Run case:

1. Add `run/<case>.strat` and `run/<case>.fixture.json`
   (`dsl-conformance-run-fixture-v1`; copy a neighbouring fixture's shape).
2. Add the case name to `implementedRunCases` in `engine/conformance.go`,
   or to `runConformanceTODO` in `engine/run_todo.go` with the reason the
   engine cannot run it yet.
3. `go run ./cmd/conformance regen`. A TODO case gets no golden.
4. Commit the fixture, the golden and the list change together; say in the PR
   which coverage class the case adds.

The bar slices for the existing fixtures were cut from the app's data tree
(`data/<symbol>/<tf>.json`); each fixture's `source` field records the slice.
