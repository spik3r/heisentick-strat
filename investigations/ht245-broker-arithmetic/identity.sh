#!/usr/bin/env bash
# Validate a completed run, then show its source and evidence binding.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
: "${1:?usage: identity.sh completed-run-directory}"
node --input-type=module - "$here" "$1" <<'JS'
const {loadRun}=await import(process.argv[2]+'/evidence.mjs');
const b=loadRun(process.argv[3]);
console.log(JSON.stringify({runSha256:b.runSha256,run:b.run,identity:b.identity},null,2));
JS
