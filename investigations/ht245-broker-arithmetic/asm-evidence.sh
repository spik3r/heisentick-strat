#!/usr/bin/env bash
# Fresh full assembly, explicitly compilation-only, with actual compiler identity.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)
: "${1:?usage: asm-evidence.sh NEW-output-directory}"
out=$1
if [ -e "$out" ]; then echo 'output directory already exists' >&2; exit 2; fi
mkdir -p "$out"
go=${HT245_GO:-go}
work=${HT245_WORK:-${TMPDIR:-/tmp}/ht245-assembly-$$}
mkdir -p "$work/cache" "$work/tmp"
export GOCACHE="$work/cache" GOTMPDIR="$work/tmp" GOTOOLCHAIN=local GOPROXY=off GOFLAGS= GOEXPERIMENT= GOCOMPILEDEBUG= CGO_ENABLED=0 GOWORK=off
cd "$root"
"$go" version > "$out/compiler.txt"
for target in linux-amd64 linux-arm64 js-wasm; do
  goos=${target%-*}; goarch=${target#*-}
  GOOS=$goos GOARCH=$goarch GOAMD64=v1 "$go" build -trimpath -gcflags=-S -o /dev/null ./engine ./contextcols 2> "$out/$target.txt"
done
