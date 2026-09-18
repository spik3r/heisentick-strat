# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/). Versions follow
semver; a semantic change to the parser, engine or corpus bumps the minor
version and is named here.

## [Unreleased]

Nothing yet.

## [0.1.0] — 2026-09-19

Mechanical extraction of `strat/` and `go/` from `spik3r/heisentick` at
`4db7c3aa`. No parser, engine or corpus semantics changed. Import paths are
`github.com/spik3r/heisentick-strat/...`; the `go/dsl` and `go/engine`
alias facades are gone. Fixture-path changes in six test files and two
frozen `.strat` copies under `engine/testdata/strategies/` make
`go test ./...` pass in the new tree. The conformance corpus is
byte-identical to the app's at that commit (43 parse, 32 run, 15 semantic
cases).
