# Recovery/current-main producer integration, 2026-10-05

Implemented by Byte (OpenAI assistant), independently reviewed. This is a
prospective local integration checkpoint, not a release or consumer adoption.
It combines recovery `475a67997faaddd6586e65fe2f21bba845b13158` with current
main `9eef5c744f54307d41d1ddfa94036e48196f8bef`, preserving both histories.
The separate Failed Breakout numeric-target patch is deliberately excluded.

## Integration decisions

- Keep both the strict-source audit and timed-return parser validation.
- Keep both the draft adapter authorization field and typed timed calendar.
- Keep calculation-source and timed-calendar fixture fields independently.
- Preserve all four main commits: quote/clock preflight, timed native family,
  calendar/family guard, and opening-range minute parsing.
- Apply reserved draft/VP identity checks before timed native dispatch. The
  first mechanical merge could execute a timed trade under a reserved ID.
- Reject supplied timed calendars in the generic composition bridge before it
  converts the fixture. The mechanical merge discarded that field there.
- Existing interactive/draft/prefix/composition/VP input allowlists keep
  refusing timed calendars. This does not qualify timed interactive execution.

The two integration failures have red/green regressions. Four textual conflict
resolutions were reviewed independently. No target correction, new registry
caller, default switch, main merge, release, pin or deployment is included.

## Fresh verification

Official Go 1.22.12 linux/amd64; toolchain archive SHA-256:
`4fa4f869b0f7fc6bb1eb2660e74657fbf04cdd290b5aef905585c86051b34d43`.
Use `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off`.

- Format and vet passed; all 14 packages passed.
- The author JSON log recorded 2,889 passing test/subtest events, zero failures
  and zero skips. An independent reviewer reran all packages successfully.
- All 71 parse and 62 run goldens are unchanged from current main.
- The ten app-owned source files referenced by `composition_manifest_v1.json`
  were fetched at its pinned `a7bc97689838d7cc5e2d7fdf462b9ca2f5dbfd28`
  revision and each SHA-256 verified. Setting `HEISENTICK_APP_SOURCE_ROOT`
  to that separate private source directory exercised all four app-source and
  frozen-wrapper-oracle tests that otherwise skip. No private source files
  were added to this repository.
- Native engine and both engine/parser WASM targets built.
- `source-assertions-parity.mjs`: 75 complete native/WASM byte-equal responses,
  including 12 assertion-free responses byte-equal to a fresh exact-recovery
  `475a679` build. These include expected refusals; not every case is a run.
- The broader `native-wasm-fixture-parity.mjs` gate also passed, retaining its
  declared numeric-drift policy for Dual EMA; do not call that entire gate
  byte-exact.

Reproduce from the exact integration tree and separately materialized pinned
app sources:

```sh
gofmt -l .
go vet ./...
HEISENTICK_APP_SOURCE_ROOT=/private/pinned-app-source go test -json ./...
go run ./cmd/conformance check
go build -trimpath -o /build/engine ./cmd/enginewasm
GOOS=js GOARCH=wasm go build -trimpath -o /build/engine.wasm ./cmd/enginewasm
GOOS=js GOARCH=wasm go build -trimpath -o /build/parser.wasm ./cmd/dslwasm
# Build /build/recovery-engine in a clean exact-475a679 clone.
node scripts/checks/source-assertions-parity.mjs /build/engine \
  /build/engine.wasm /path/to/go/misc/wasm/wasm_exec.js /build/recovery-engine
node scripts/checks/native-wasm-fixture-parity.mjs
```

## Remaining gates

The target fix must be integrated and requalified separately. Consumer
current-main reconciliation, registered caller/source assertion expansion,
absent-bracket prefix serialization, actual browser/UI transport qualification,
resource budgets, all JS caller migration, import guard, and released artifact
pins remain open. The source-pinned composition oracle is historical test
coverage, not current registry adoption or a profitability statement.
