#!/usr/bin/env bash
# Historical procedure is retained under results/historical-tools for audit only.
# Its unbound receipts cannot be admitted to the corrected evidence pipeline.
set -euo pipefail
echo 'No current bound constant-ablation run is available. Historical author evidence is preserved under results/; it is not refreshed ARM runtime proof.' >&2
exit 2
