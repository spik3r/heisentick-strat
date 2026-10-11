#!/usr/bin/env bash
# Scratch-copy ablation: remove the two GOARCH==arm64 constants in
# engine/family_channel_break_hold.go (the repo tree is not touched), then
# compare ARM64 with FMA, ARM64 without FMA (diagnostic flag) and the amd64
# receipts from run-all.sh on boundary-patches.json. Also runs the engine's
# TestRunConformance in the copy.
#   ablate-arch-constants.sh <receipts-dir-from-run-all>
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd); root=$(git -C "$here" rev-parse --show-toplevel)
rec=$1; work=${HT245_WORK:-/tmp/ht245}/noconst
rm -rf "$work"; mkdir -p "$work/out"
rsync -a --exclude .git --exclude '*.wasm' "$root"/ "$work/src/"
cd "$work/src"
python3 - <<'PY'
p='engine/family_channel_break_hold.go'
s=open(p).read()
a=s.replace('return rail + 9.4e-12','return rail').replace('return widthATR - 1.565e-12','return widthATR')
assert a!=s
open(p,'w').write(a)
PY
export GOCACHE="$work/gocache" GOPROXY=off GOFLAGS= CGO_ENABLED=0
go build -o "$work/h-fma" ./investigations/ht245-broker-arithmetic/harness
GOFLAGS=-gcflags=all=-d=fmahash=n go build -o "$work/h-nofma" ./investigations/ht245-broker-arithmetic/harness 2>/dev/null
m=investigations/ht245-broker-arithmetic/manifests/boundary-patches.json
"$work/h-fma" -root . -manifest $m -label arm64-noconst-fma > "$work/out/arm64-noconst-fma.jsonl"
"$work/h-nofma" -root . -manifest $m -label arm64-noconst-nofma > "$work/out/arm64-noconst-nofma.jsonl"
cp "$rec/boundary-patches/amd64v1-go1.27.0.jsonl" "$work/out/"
node "$here/compare.mjs" "$work/out" --ref amd64v1-go1.27.0 | grep -E "^arm"
echo "--- TestRunConformance in the copy, FMA on, constants removed:"
go test ./engine -run TestRunConformance -count=1 2>&1 | grep -E "^(--- FAIL|FAIL|ok)|mismatch|initialSl" | cut -c1-200 | head -6
