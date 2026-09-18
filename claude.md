# Claude Instructions

See `agents.md` for the single source of truth for AI-agent instructions.
This file exists only so Claude can find the repo conventions.

## Quick Reference

- Run `go test ./...` before any commit.
- Run `gofmt -l .` before any commit.
- Run `go vet ./...` before any commit.
- Conformance corpus must remain green.
- Consumers pin tags; no floating versions.
