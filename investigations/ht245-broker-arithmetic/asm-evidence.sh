#!/usr/bin/env bash
# Print the floating-point instructions the compiler emits for the broker's
# money-path source lines, per target. Compilation evidence only: it shows what
# was generated, not what ran. Isolated GOCACHE; nothing global is touched.
#   asm-evidence.sh <out-dir>
set -uo pipefail
out=$1; mkdir -p "$out"
root=$(git rev-parse --show-toplevel)
work=${HT245_WORK:-${TMPDIR:-/tmp}/ht245}
export GOPROXY=off GOFLAGS= GOTELEMETRY=off CGO_ENABLED=0
lines='broker\.go:(536|539|542|577|589|590|591)\)|flag\.go:(323|345)\)|family_trend_pullback\.go:(131|160)\)|family_channel_break_hold\.go:(143)\)|channels\.go:(351|354|357|359|363)\)'
ops='FMADDD|FMSUBD|FNMADDD|FNMSUBD|FMULD|FADDD|FSUBD|MULSD|ADDSD|SUBSD|VFMADD[0-9]*SD|VFNMADD[0-9]*SD|VFMSUB[0-9]*SD|F64Mul|F64Add|F64Sub'
for t in "arm64-go1.27.0|local|darwin|arm64|" "arm64-go1.23.8|go1.23.8|darwin|arm64|" "arm64-linux-go1.27.0|local|linux|arm64|" \
         "amd64v1-go1.27.0|local|darwin|amd64|GOAMD64=v1" "amd64v3-go1.27.0|local|darwin|amd64|GOAMD64=v3" "wasmjs-go1.27.0|local|js|wasm|"; do
  IFS='|' read -r label tc goos goarch extra <<<"$t"
  export GOCACHE="$work/gocache-asm-$label" GOTOOLCHAIN=$tc
  [ "$tc" = local ] && export GOTOOLCHAIN=local
  {
    echo "# $label: $(env $extra GOOS=$goos GOARCH=$goarch go version) GOOS=$goos GOARCH=$goarch${extra:+ $extra}"
    env $extra GOOS=$goos GOARCH=$goarch go build -gcflags=-S -o /dev/null "$root/engine" "$root/contextcols" 2>&1 \
      | grep -E "$lines" | grep -oE "\((/[^)]*/)?(engine|contextcols)/[a-z_]+\.go:[0-9]+\)[[:space:]]+[A-Za-z0-9]+" \
      | sed -E 's#\(/?[^)]*/(engine|contextcols)/#(\1/#; s/\)[[:space:]]+/ /' | grep -E " ($ops)\$" | sort | uniq -c
  } > "$out/$label.txt" 2>&1
  echo "wrote $out/$label.txt ($(wc -l < "$out/$label.txt") lines)"
done
