#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
runner="$(go env GOROOT)/misc/wasm/go_js_wasm_exec"
if [[ ! -f "$runner" ]]; then runner="$(go env GOROOT)/lib/wasm/go_js_wasm_exec"; fi
[[ -f "$runner" ]] || { echo 'Go WASM runner missing' >&2; exit 1; }
go test ./dsl ./engine/frozeninput -count=1 -run '^TestFrozenInput' -v > "$tmp/native.log"
GOOS=js GOARCH=wasm go test -exec "$runner" ./dsl ./engine/frozeninput -count=1 -run '^TestFrozenInput' -v > "$tmp/wasm.log"
grep '^FROZEN_INPUT_' "$tmp/native.log" > "$tmp/native.json"
grep '^FROZEN_INPUT_' "$tmp/wasm.log" > "$tmp/wasm.json"
test "$(wc -l < "$tmp/native.json")" -eq 4
test "$(wc -l < "$tmp/wasm.json")" -eq 4
diff -u "$tmp/native.json" "$tmp/wasm.json"
echo 'PASS: original-byte admission, exact decimal oracle, nested rejection cases and copied paired projection agree in native and Go/WASM'
