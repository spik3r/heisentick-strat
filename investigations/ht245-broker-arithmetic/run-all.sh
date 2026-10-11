#!/usr/bin/env bash
# Execute the six committed manifests without rewriting any input or tracked output.
# HT245_GO=/exact/go HT245_WORK=/outside/source/cache run-all.sh NEW-output-root [target ...]
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
: "${1:?usage: run-all.sh NEW-output-root [native|amd64v1|amd64v3|arm64|arm64nofma|wasmjs|wasip1 ...]}"
out=$1; shift
if [ -e "$out" ]; then echo 'output root already exists; use a fresh path' >&2; exit 2; fi
mkdir -p "$(dirname "$out")"
mkdir "$out" # Exclusive ownership; never share an invocation output root.
targets=("$@")
if [ ${#targets[@]} -eq 0 ]; then targets=(native wasmjs); fi
for name in frozen-run-corpus cancellation-variants scaled-prices boundary-patches scaled-search boundary-search; do
  "$here/run-matrix.sh" "$here/manifests/$name.json" "$out/$name" "${targets[@]}" | tee "$out/$name.run.txt"
  if [ ${#targets[@]} -gt 1 ]; then
    node "$here/compare.mjs" "$out/$name" --json "$out/$name.compare.json" | tee "$out/$name.compare.txt"
  fi
done
