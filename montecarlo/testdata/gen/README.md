# Fixture oracle

`oracle.mjs` is the JS reference oracle for `montecarlo/testdata/*.json`. It
holds verbatim copies of `maxDDfrac`, `shuffleInto`, `mulberry32`, `pctl`
and the sampling loop from heisentick `scripts/strategy/monteCarlo.mjs`,
evaluates them on the fixture inputs, and writes `{input, expected}` files.
The Go tests compare against `expected` exactly.

Rerun from the repository root with Node 20 or newer:

```sh
node montecarlo/testdata/gen/oracle.mjs
```

Change a fixture by editing the `cases` list in the script and rerunning it,
never by editing the JSON.
