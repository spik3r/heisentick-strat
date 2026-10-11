#!/usr/bin/env bash
# Reproduce every result in REPORT.md. Needs: Go 1.27 (host) and optionally the
# go1.23.8 toolchain in the module cache (offline), Node >= 22, wasmtime, and on
# Apple Silicon Rosetta for darwin/amd64. Targets that cannot run print "not-run".
# Writes under results/ (summaries) and $HT245_WORK (receipts, binaries, caches).
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
export HT245_WORK=${HT245_WORK:-${TMPDIR:-/tmp}/ht245}
rec=$HT245_WORK/receipts; res=$here/results
mkdir -p "$rec" "$res/summaries" "$res/asm"
cd "$here"
step() { m=$1

  ./run-matrix.sh manifests/$m.json "$rec/$m" | tee "$res/summaries/$m.run.txt"
  node compare.mjs "$rec/$m" --ref amd64v1-go1.27.0 --json "$res/summaries/$m.compare.json" | tee "$res/summaries/$m.compare.txt"
}
step frozen-run-corpus
node gen-cancellation-manifest.mjs "$rec/frozen-run-corpus/arm64-go1.27.0.jsonl" manifests/frozen-run-corpus.json > manifests/cancellation-variants.json
step cancellation-variants
step scaled-prices
node gen-boundary-manifest.mjs "$rec/scaled-prices/arm64-go1.27.0.jsonl" "$rec/scaled-prices/amd64v1-go1.27.0.jsonl" manifests/scaled-prices.json > manifests/boundary-patches.json
step boundary-patches
# Derived manifests: search over 30 more seeded scales, then one-bar boundary edits.
./run-matrix.sh manifests/scaled-search.json "$rec/scaled-search" arm64-go1.27.0 amd64v1-go1.27.0 arm64nofma-diag-go1.27.0 | tee "$res/summaries/scaled-search.run.txt"
node compare.mjs "$rec/scaled-search" --ref amd64v1-go1.27.0 --json "$res/summaries/scaled-search.compare.json" | tee "$res/summaries/scaled-search.compare.txt"
node gen-boundary-manifest.mjs "$rec/scaled-search/arm64-go1.27.0.jsonl" "$rec/scaled-search/amd64v1-go1.27.0.jsonl" manifests/scaled-search.json > manifests/boundary-search.json
./run-matrix.sh manifests/boundary-search.json "$rec/boundary-search" | tee "$res/summaries/boundary-search.run.txt"
node compare.mjs "$rec/boundary-search" --ref amd64v1-go1.27.0 --json "$res/summaries/boundary-search.compare.json" | tee "$res/summaries/boundary-search.compare.txt"
./asm-evidence.sh "$res/asm"
./identity.sh > "$res/identity.json"
