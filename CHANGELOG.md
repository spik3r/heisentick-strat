# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/). Versions follow
semver; a semantic change to the parser, engine or corpus bumps the minor
version and is named here.

## [Unreleased]

Extraction dry run only; no code has moved. Added the inventory, target
layout, ordered T-B2 steps and `scripts/extract.sh`, rehearsed against
heisentick `32bb7a5`: `go build` passes after the pure import rewrite;
`go test` needs 11 fixture-path lines in 6 files plus two copied `.strat`
fixtures (see `docs/extraction-plan.md`).

## [0.1.0] — planned

Mechanical move of `strat/` and `go/` from heisentick at the frozen commit.
No behavioural change.
