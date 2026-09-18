# Strat conformance corpus

This directory contains language-agnostic fixtures generated from the current
JavaScript Strat compiler and runtime. A future engine implementation should
treat these files as golden test vectors.

Regenerate with:

```bash
node scripts/build/buildConformanceCorpus.mjs --regen
```

## Regeneration and parity rule

`scripts/build/buildConformanceCorpus.mjs --regen` is the only supported way to
change committed parse or run goldens. Treat any golden update as a reviewed
corpus-regeneration PR, not as an incidental side effect of another change.

Parser-visible changes must preserve JS and Go parity in the same PR. If a JS
DSL parser change alters compiled config, diagnostics, or trade output, mirror
the behavior in `strat/implementations/server-runtime` and keep
`pnpm run dsl:conformance` plus `go test ./...` green. The same rule applies in
reverse for Go parser changes: either the JS
behavior and corpus already match, or the PR must update the JS side through the
reviewed regen path. A JS-only or Go-only parser drift should fail CI through the
conformance corpus or the Go test suite, not reach users as a silent divergence.

The generator is explicit on purpose. It writes stable JSON with sorted object
keys and no timestamps. If full source data is outside the worktree, set
`CONFORMANCE_SOURCE_DATA_DIR=/path/to/data`; otherwise existing committed run
fixtures are reused.

## Parse fixtures

`parse/<case>.strat` is the input Strat source.

`parse/<case>.cfg.json` has this shape:

```json
{
  "schema": "dsl-conformance-parse-v1",
  "case": "case-name",
  "category": "setup-family | deployed-book-dsl | diagnostic",
  "source": "origin of the Strat text",
  "cfg": {},
  "errors": [],
  "warnings": [],
  "diagnostics": []
}
```

For diagnostic cases, `cfg` is still the compiler output and `diagnostics`
contains the structured error/warning mirror with line and column fields.
Spec-level diagnostics use `line: null` and `col: null`.

## Run fixtures

`run/<case>.strat` is the Strat strategy under test.

`run/<case>.fixture.json` contains the market slice and fixed execution options:
symbol, timeframe, higher timeframe bars, range method, start equity, fill mode,
and slippage. Bar rows are `[t, o, h, l, c, v]` with `t` in Unix milliseconds.

`run/<case>.trades.json` is the golden trade list produced by the JS engine
against that fixture. A conforming engine should compile Strat, build the same
context from the fixture bars, run with the listed costs, and reproduce the
trade list.

Check drift with:

```bash
pnpm run dsl:conformance
```

The checker also audits setup-family coverage. It compares the JS parser's
recognized setup families with the Go DSL family constants, then requires every
family to have parse coverage and either a `family-*` run fixture or a declared
deployed-book run fixture. The failed-breakout runtime is covered by the
level-sweep corpus case, matching the existing sweep/reclaim semantics.

### Family run cases (`run/family-*`)

One run case per setup family not already covered by a deployed-book case
(flagContinuation, trendPullback, and sessionBreakHold are exercised by the
`deployed-*` cases). Sources are the same registered strategies the parse
corpus uses; slices are tuned in `FAMILY_RUN_CASES`
(scripts/build/buildConformanceCorpus.mjs) so every family produces at least one
golden trade. `family-supply-demand` runs on EURUSD (the family's XAUUSD
research copy produces no trades on available history); all other cases run
on XAUUSD. The generator refuses to write a zero-trade family case unless
`CONFORMANCE_ALLOW_EMPTY=1` is set for slice probing.
