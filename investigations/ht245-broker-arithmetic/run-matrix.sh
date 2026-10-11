#!/usr/bin/env bash
# Build the harness for each target and run one manifest. Isolated caches under
# $WORK; nothing global is cleared. Targets that cannot run here are skipped
# and printed as "not-run <label> <reason>"; a skip is never a pass.
#
#   run-matrix.sh <manifest.json> <out-dir> [label ...]
set -uo pipefail
manifest=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
out=$2; shift 2
root=$(git rev-parse --show-toplevel)
work=${HT245_WORK:-${TMPDIR:-/tmp}/ht245}
mkdir -p "$out" "$work/bin"
export GOPROXY=off GOFLAGS= GOTELEMETRY=off CGO_ENABLED=0
want=("$@")
selected() { [ ${#want[@]} -eq 0 ] && return 0; for w in "${want[@]}"; do [ "$w" = "$1" ] && return 0; done; return 1; }

# label|toolchain|GOOS|GOARCH|extra-env|runner
targets=(
 "arm64-go1.27.0|local|darwin|arm64||"
 "arm64-go1.23.8|go1.23.8|darwin|arm64||"
 "amd64v1-go1.27.0|local|darwin|amd64|GOAMD64=v1|arch -x86_64"
 "amd64v3-go1.27.0|local|darwin|amd64|GOAMD64=v3|arch -x86_64"

 "arm64nofma-diag-go1.27.0|local|darwin|arm64|GOFLAGS=-gcflags=all=-d=fmahash=n|"
 "wasmjs-go1.27.0|local|js|wasm||NODE"
 "wasip1-go1.27.0|local|wasip1|wasm||WASMTIME"
)
for t in "${targets[@]}"; do
  IFS='|' read -r label tc goos goarch extra runner <<<"$t"
  selected "$label" || continue
  export GOCACHE="$work/gocache-$label"
  [ "$tc" = local ] && export GOTOOLCHAIN=local || export GOTOOLCHAIN=$tc
  bin="$work/bin/harness-$label"; [ "$goos" = js ] && bin="$bin.wasm"; [ "$goos" = wasip1 ] && bin="$bin.wasm"
  env $extra GOOS=$goos GOARCH=$goarch go build -trimpath -o "$bin" "$root/investigations/ht245-broker-arithmetic/harness" 2> "$work/build-$label.err" || { echo "not-run $label build-failed: $(head -c 200 "$work/build-$label.err")"; continue; }
  case "$runner" in
    NODE) goroot=$(GOTOOLCHAIN=$tc go env GOROOT); wx="$goroot/lib/wasm/go_js_wasm_exec"; [ -x "$wx" ] || wx="$goroot/misc/wasm/go_js_wasm_exec"
          cmd=("$wx" "$bin") ;;
    WASMTIME) cmd=(wasmtime run --dir="$root"::/w "$bin"); wroot=/w; wmanifest=/w/${manifest#"$root"/} ;;
    "") cmd=("$bin") ;;
    *) cmd=($runner "$bin") ;;
  esac
  rootarg=${wroot:-$root}; manarg=${wmanifest:-$manifest}; unset wroot wmanifest
  if "${cmd[@]}" -root "$rootarg" -manifest "$manarg" -label "$label" > "$out/$label.jsonl" 2> "$work/run-$label.err"; then
    echo "ran $label $(wc -l < "$out/$label.jsonl") receipts"
  else
    echo "not-run $label run-failed: $(head -c 300 "$work/run-$label.err")"; rm -f "$out/$label.jsonl"
  fi
done
