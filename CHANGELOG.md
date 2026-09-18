# Changelog

All notable changes to heisentick-strat will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Initial extraction from heisentick repo.
- Go DSL parser, engine, marketdata, contextcols, data packages.
- Report CLI (`heisentick report|grid`).
- WASM parser build (`dslwasm`).
- Conformance corpus (parse + run + semantic fixtures).
- CI workflows (go test, go vet, gofmt).
- Release workflow (linux/arm64, darwin/arm64, wasm, conformance.tar.gz).

### Known issues
- Two engine tests read `.strat` files from app's `engine/strategies/dsl/` (needs STRAT_SOURCE_ROOT or skip).
- Tests under `cmd/` need relative path fixes for conformance fixtures.
