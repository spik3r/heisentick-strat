# Frozen invented unit qualification evidence

This bundle makes the unit parity qualification reproducible from an ordinary
producer checkout. It contains only retained evidence for 96 invented source
cases under three cost policies. Materialization performs no accounting and
never calls an oracle. This is not historical-market, browser, capacity, ARM64
or Backtester acceptance evidence.

## Contents and provenance

`manifest.json` lists every output path, exact byte count, SHA256 and provenance.
Its digest is pinned in `scripts/checks/adaptive-flag-unit-evidence.mjs`.
The compressed pack's digest is pinned both there and in the manifest.

There are 964 materialized files:

- `core/`: 96 raw outputs, 288 core projections and the original core receipt.
  Their source names are relative to `evidence/b1-2/native/`.
- `oracle/`: the 288 original references and 288 original comparisons for the
  source-backed cases, plus the unchanged original `ARTIFACTS.json`.
  Their source names are relative to `oracle-results-v1/`.
- `INVOCATION_PLAN.json`: unchanged original bytes from
  `oracle-protocol-v2/INVOCATION_PLAN.json`.
- `oracle/ATTESTATION.json`: a separately identified sanitized derivative of
  selected facts in the approved original oracle checkpoint.

All 963 files marked `byte-exact-original` are literal copies. The original
306-case invocation plan and 611-entry artifact manifest retain their complete
original bytes, including historical status labels and hashes for the additional
hand cases. Only the 288 source-backed reference/comparison pairs are packaged.
The plan's relative input names describe its original invocation, not paths to
open during fresh qualification. The manifest maps the retained source bytes to
their new materialized locations.

The original checkpoint contains machine-specific absolute module paths. Its
bytes are deliberately absent. `ATTESTATION.json` explicitly says
`sanitized-derivative-not-original-checkpoint` and binds the original checkpoint
digest, original artifact-manifest digest, original plan digest, original core
receipt digest, original execution counts and the selected subset. It makes no
claim that its own bytes equal the original checkpoint. The original evidence
was not edited. No Python source or historical research data is in this pack.

Original evidence identities:

| Artifact | SHA256 |
| --- | --- |
| Oracle checkpoint (not included) | `7b8096c10736d9f14df725ed0040484fd3a41063e0857e2e4c92fa5ba711e926` |
| Oracle artifact manifest | `5f35153b406f7c36aed07398ee733c5d2ef328f9c843aff5a03919a2df6d261d` |
| Invocation plan | `9d141c25ead79077685781fec2cd52c0166a09e5ccde07b3f4953cc72c75a161` |
| Core receipt | `57be2690e82ddffbeda6251030a5286575b3413aeef138f1ff368f57c436d10f` |

The new manifest, pack and attestation have distinct identities recorded in
`EVIDENCE_IDENTITY`. Unit parity receipts using `--bundle` have schema
`adaptive-flag-unit-public-parity-v2` and include those identities in
`evidenceBundle`. Their existing `retainedEvidence` fields continue to name the
original evidence. Legacy explicit retained-directory invocation still emits v1.

## Materialization

From the repository root:

```sh
node scripts/checks/adaptive-flag-unit-evidence.mjs /new/evidence-directory
node scripts/checks/adaptive-flag-unit-parity.mjs \
  /native /engine.wasm /wasm_exec.js /go \
  --bundle /new/evidence-directory /new/report-directory /receipt.json
node --test scripts/checks/adaptive-flag-unit-evidence.test.mjs
```

Both output directories must be new. The loader verifies the pinned manifest and
compressed pack before expanding anything. It parses and hashes all entries
before creating the output directory, then verifies the complete materialized
tree. Parity verifies that tree again before using its contents. Missing, extra,
modified, linked or undeclared files fail closed.

## Pack format and bounds

The deterministic gzip pack is 1,677,307 bytes. Decompression produces 21,789,874
bytes, including framing; the actual 964 file payloads total 21,724,634 bytes.
Limits are 4 MiB compressed, 32 MiB expanded, 1,024 files and a 512-byte per-file
header. The manifest is limited to 512 KiB. Limits are enforced before allocation
where possible, and decompression has a hard output limit.

The uncompressed stream begins with the ASCII bytes `HT195_EVIDENCE_V1\n`.
In manifest path order, each file then has a four-byte unsigned big-endian header
length, a canonical compact UTF-8 JSON header with exactly the ordered fields
`path`, `type`, `bytes`, followed by exactly that many literal payload bytes.
`type` must equal `file`. There are no directory or link entries and no trailing
records. Gzip was created with level 9 and a zero timestamp; no names or source
paths are stored in gzip metadata.

The extractor rejects absolute paths, traversal, path aliases, duplicate paths,
file/directory collisions, extra metadata, all non-regular entry types and
undeclared entries. It refuses existing output directories, directory symlinks,
file symlinks and hardlinks. File creation is exclusive with `O_NOFOLLOW`.
Focused tests include a gzip expansion exceeding the limit, corrupt digests,
truncation, duplicate JSON keys, missing/extra files and linked destinations.
