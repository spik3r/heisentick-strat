#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
# Tests compile the SAME Go primitives for native and WASM. This does not claim
# that the production enginewasm bridge has exposed a registered quote family.
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
runner="$(go env GOROOT)/misc/wasm/go_js_wasm_exec"
if [[ ! -f "$runner" ]]; then
  runner="$(go env GOROOT)/lib/wasm/go_js_wasm_exec"
fi
[[ -f "$runner" ]] || { echo "Go WASM runner missing" >&2; exit 1; }
go test ./engine/frozenlevels -count=1 -run '^TestPrimitiveParityTrace$' -v > "$tmp/native.log"
GOOS=js GOARCH=wasm go test -exec "$runner" ./engine/frozenlevels -count=1 -v > "$tmp/wasm.log"
grep '^PRIMITIVE_TRACE=' "$tmp/native.log" > "$tmp/native.json"
grep '^PRIMITIVE_TRACE=' "$tmp/wasm.log" > "$tmp/wasm.json"
test "$(wc -l < "$tmp/native.json")" -eq 1
test "$(wc -l < "$tmp/wasm.json")" -eq 1
diff -u "$tmp/native.json" "$tmp/wasm.json"
echo 'PASS: identical native/Go-WASM primitive trace; all focused WASM tests passed'
