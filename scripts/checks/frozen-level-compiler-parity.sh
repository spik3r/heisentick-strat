#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
runner="$(go env GOROOT)/misc/wasm/go_js_wasm_exec"
if [[ ! -f "$runner" ]]; then runner="$(go env GOROOT)/lib/wasm/go_js_wasm_exec"; fi
[[ -f "$runner" ]] || { echo 'Go WASM runner missing' >&2; exit 1; }
go test ./dsl ./engine ./cmd/enginewasm -count=1 -run '^TestFrozen' -v > "$tmp/native.log"
GOOS=js GOARCH=wasm go test -exec "$runner" ./dsl ./engine ./cmd/enginewasm -count=1 -run '^TestFrozen' -v > "$tmp/wasm.log"
grep '^FROZEN_CONFIG_TRACE=' "$tmp/native.log" > "$tmp/native.json"
grep '^FROZEN_CONFIG_TRACE=' "$tmp/wasm.log" > "$tmp/wasm.json"
test "$(wc -l < "$tmp/native.json")" -eq 1
test "$(wc -l < "$tmp/wasm.json")" -eq 1
diff -u "$tmp/native.json" "$tmp/wasm.json"
echo 'PASS: actual frozen parser/config/raw decoder/refusal tests pass native and Go/WASM; canonical byte traces match'
