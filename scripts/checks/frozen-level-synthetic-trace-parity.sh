#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
runner="$(go env GOROOT)/misc/wasm/go_js_wasm_exec"
if [[ ! -f "$runner" ]]; then runner="$(go env GOROOT)/lib/wasm/go_js_wasm_exec"; fi
[[ -f "$runner" ]] || { echo 'Go WASM runner missing' >&2; exit 1; }
go test ./dsl ./engine/frozencontrol -count=1 -run '^TestFrozen(Synthetic|TraceCosts)' -v > "$tmp/native.log"
GOOS=js GOARCH=wasm go test -exec "$runner" ./dsl ./engine/frozencontrol -count=1 -run '^TestFrozen(Synthetic|TraceCosts)' -v > "$tmp/wasm.log"
grep '^SYNTHETIC_' "$tmp/native.log" > "$tmp/native.json"
grep '^SYNTHETIC_' "$tmp/wasm.log" > "$tmp/wasm.json"
test "$(wc -l < "$tmp/native.json")" -eq 15
test "$(wc -l < "$tmp/wasm.json")" -eq 15
diff -u "$tmp/native.json" "$tmp/wasm.json"
echo 'PASS: 14 complete synthetic event/fill/fee traces and strict cost controls agree byte-for-byte in native and Go/WASM'
