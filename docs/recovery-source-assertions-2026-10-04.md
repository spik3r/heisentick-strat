# Source-assertion recovery checkpoint, 2026-10-04

Implemented by Byte (OpenAI assistant). This is a reconstruction, not a recovered
copy of the later unpublished cloud commits. The prior workspace disappeared;
its source increments and detailed logs were unavailable. No old pass claim or
lost commit hash validates this checkpoint. The fresh base is
`fe4c81db72c1f156124ccea9b766eabf35ca650a` on the published migration branch.
The new recovery branch is `codex/one-engine-recovery-20261004`.

Canonical task and recovery claim:
[WP-go-draft-prefix at 63481f2](https://github.com/spik3r/heisentick-backlog/blob/63481f2afa5f0f54fe0efd1c8abc0612dcbeb36c/tasks/WP-go-draft-prefix.md).
User authorization now permits feature-branch checkpoints. Connector-created
commits use connected-account metadata; this does not mean the account owner
personally authored the code. No PR, main merge, release or default change is
part of this checkpoint.

## Bounded result

Optional source assertions in the existing strict draft inspection, finalized
execution and raw-prefix operation check the registered Dual daily/4h parameter
shape. Every value is checked against Go-parsed source, never installed as an
override. Exact context/preference metadata remains visible. See
[the contract](../spec/interactive-draft-v1.md#optional-registered-source-assertions).
Caller-owned canonical source/build identity is still required. Daily is archived;
Dual 4h is active Watch. This adds no registered CLI adoption.

Registry evidence was freshly read from app
`84eb8d2e81417cfe98b39629a49b74edd09840e3`, the two registered wrappers,
canonical .strat sources, source-parameter derivation, context-requirement
resolver and strategy index. Nine keys are required. Sessions is an inherited
capability declaration, not a new Dual session filter. Zone is the wrapper's
preference, while actual source execution remains pivot.

## Fresh validation

Official Go 1.22.12 linux/amd64 archive, SHA-256
`4fa4f869b0f7fc6bb1eb2660e74657fbf04cdd290b5aef905585c86051b34d43`,
was verified against the current official download inventory before extraction.
Validation uses `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off`.

- Baseline before edits: format, vet, all 14 packages, 68 parse and 60 run goldens
- Reconstructed tree: format, vet, all 14 packages, same unchanged goldens
- Independent read-only contract/source review and focused Go tests passed
- Native engine and both engine/parser WASM targets built with `-trimpath`
- 75 synthetic complete native/WASM outputs byte-exact: 20 successes, 55 refusals
- 12 assertion-free native outputs also byte-exact against a fresh baseline build
- All nine parameter mismatches, malformed claims, duplicate keys, source drift,
  wrong route and unqualified HA prefix are refused; test coverage additionally
  verifies each missing/null/type-invalid parameter and exact input provenance

Committed evidence under
`scripts/spikes/enginewasm/evidence/recovery-source-assertions-20261004/`
records raw gate output and every parity case's exact input/output digests.
The initial parity build came from the reviewed uncommitted recovery tree; the
native binary's VCS stamp reflects that. Rebuild final remote source before
claiming artifact identity for a consumer. No binary is a released asset.

Reproduce with the exact producer tree, Go 1.22.12 and a clean real clone of the
baseline (all paths below are caller-chosen build locations):

```sh
gofmt -l .
go vet ./...
go test ./...
go run ./cmd/conformance check
go build -trimpath -o /build/engine ./cmd/enginewasm
GOOS=js GOARCH=wasm go build -trimpath -o /build/engine.wasm ./cmd/enginewasm
GOOS=js GOARCH=wasm go build -trimpath -o /build/dsl.wasm ./cmd/dslwasm
# Build /build/baseline-engine from fe4c81d using the same compiler.
node scripts/checks/source-assertions-parity.mjs /build/engine \
  /build/engine.wasm /path/to/go/misc/wasm/wasm_exec.js /build/baseline-engine
```

The first baseline build attempted a Git worktree. Go 1.22 searched the workspace
parent's placeholder .git and could not obtain VCS status. A separate real local
clone at the exact baseline solved that; VCS checks were not disabled.

## Open gates

The consumer snapshot is partial and unchanged. Native/browser transports must
still snapshot and bind actual registry fields before enabling any caller.
Active registered Dual 4h basket adoption is next, then distinct daily/protected
SMA contracts. No general DSL, archive-policy or profitability claim is made.
Existing absent-bracket prefix serialization is deliberately unchanged here.

Full consumer checkout, exact data archive, full-suite/test-environment resolution,
current-main integration, browser/Forward qualification, resource and aggregate
release gates remain open. The earlier denied consumer full-suite request was
not resumed. Main's targetless-trail handling and migration's native recovery
interlocks still need a coordinated integration review. Existing CI runs on PRs
or main pushes, so a feature checkpoint alone need not create a CI run.
