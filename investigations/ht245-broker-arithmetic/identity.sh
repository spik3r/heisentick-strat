#!/usr/bin/env bash
# Print source, compiler, input and binary identities as JSON for the report.
#   identity.sh [bin-dir]
set -uo pipefail
root=$(git rev-parse --show-toplevel)
bins=${1:-${HT245_WORK:-/tmp/ht245}/bin}
cd "$root"
export GOPROXY=off GOFLAGS=
python3 - "$root" "$bins" <<'PY'
import json, subprocess, sys, hashlib, glob, os
root, bins = sys.argv[1:3]
def sh(*a): return subprocess.run(a, capture_output=True, text=True, cwd=root).stdout.strip()
def sha(p): return hashlib.sha256(open(p, 'rb').read()).hexdigest()
out = {
  "strat_base_commit": sh("git", "merge-base", "HEAD", "origin/main"),
  "strat_head": sh("git", "rev-parse", "HEAD"),
  "conformance_run_tree": sh("git", "rev-parse", "HEAD:conformance/run"),
  "engine_tree": sh("git", "rev-parse", "HEAD:engine"),
  "contextcols_tree": sh("git", "rev-parse", "HEAD:contextcols"),
  "broker_go_blob": sh("git", "rev-parse", "HEAD:engine/broker.go"),
  "host": sh("uname", "-sm"), "go_host": sh("go", "version"),
  "toolchains": {},
  "manifests": {os.path.basename(p): sha(p) for p in sorted(glob.glob(root + "/investigations/ht245-broker-arithmetic/manifests/*.json"))},
  "binaries": {os.path.basename(p): sha(p) for p in sorted(glob.glob(bins + "/harness-*"))},
}
for tc in ("go1.23.8",):
    env = dict(os.environ, GOTOOLCHAIN=tc)
    out["toolchains"][tc] = subprocess.run(["go", "version"], capture_output=True, text=True, env=env).stdout.strip()
print(json.dumps(out, indent=1))
PY
