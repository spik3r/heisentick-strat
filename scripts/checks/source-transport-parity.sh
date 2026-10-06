#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
wasm_dir="$(go env GOROOT)/misc/wasm"
if [[ ! -f "$wasm_dir/wasm_exec.js" ]]; then wasm_dir="$(go env GOROOT)/lib/wasm"; fi
[[ -f "$wasm_dir/wasm_exec.js" && -f "$wasm_dir/go_js_wasm_exec" ]] || { echo 'Go WASM runner missing' >&2; exit 1; }
go test ./internal/sourcetext -count=1
GOOS=js GOARCH=wasm go test -exec "$wasm_dir/go_js_wasm_exec" ./internal/sourcetext -count=1
go build -o "$tmp/dsl-native" ./cmd/dslwasm
go build -o "$tmp/engine-native" ./cmd/enginewasm
GOOS=js GOARCH=wasm go build -o "$tmp/dsl.wasm" ./cmd/dslwasm
GOOS=js GOARCH=wasm go build -o "$tmp/engine.wasm" ./cmd/enginewasm
node scripts/checks/source-transport-parity.mjs "$tmp/engine-native" "$tmp/engine.wasm" "$tmp/dsl-native" "$tmp/dsl.wasm" "$wasm_dir/wasm_exec.js"
