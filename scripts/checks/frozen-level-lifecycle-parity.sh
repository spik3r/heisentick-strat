#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
runner="$(go env GOROOT)/misc/wasm/go_js_wasm_exec"
if [[ ! -f "$runner" ]]; then runner="$(go env GOROOT)/lib/wasm/go_js_wasm_exec"; fi
[[ -f "$runner" ]] || { echo 'Go WASM runner missing' >&2; exit 1; }
go test ./engine/frozenlevels -count=1 -run '^Test(Level|Lifecycle|IndependentLifecycle|IndependentOverflow)' -v > "$tmp/native.log"
GOOS=js GOARCH=wasm go test -exec "$runner" ./engine/frozenlevels -count=1 -run '^Test(Level|Lifecycle|IndependentLifecycle|IndependentOverflow)' -v > "$tmp/wasm.log"
grep -E '^(INDEPENDENT_)?LIFECYCLE_TRACE=' "$tmp/native.log" > "$tmp/native.json"
grep -E '^(INDEPENDENT_)?LIFECYCLE_TRACE=' "$tmp/wasm.log" > "$tmp/wasm.json"
test "$(wc -l < "$tmp/native.json")" -eq 2
test "$(wc -l < "$tmp/wasm.json")" -eq 2
diff -u "$tmp/native.json" "$tmp/wasm.json"
echo 'PASS: all lifecycle tests and exact event trace agree in native and Go/WASM'
