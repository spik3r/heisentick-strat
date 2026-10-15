# Bounded native Adaptive history accounting

`adaptive-flag-history-unit-report` is an additive native-only research route.
It uses the existing exact INITIAL, TWEAKED and SNAPSHOT_C M30/H1 sources,
raw state machine and ordered Go unit projector. The explicit resource profile
is operational admission, not study authorization or evidence of profitability.

```
heisentick adaptive-flag-history-unit-report \
  --profile=adaptive-history-v1 \
  --request-file=raw-request.json --projection-file=unit-request.json \
  --dsl-file=baseline.strat --bars-file=original-six-column.btb1
```

All five flags are mandatory and nonrepeatable; unknown flags refuse. The raw
and projection request schemas and source admission are unchanged. Costs are
exactly `RAW`, `RAZOR_PROXY_BASIC` or `RAZOR_PROXY_HARSH_AGGREGATE`; there are no
aliases, overrides, account sizing, equity defaults or custom baselines.

## Continuous state and bounds

The original `adaptive-history-v1` profile retains the fixed limits below.

| Resource | Fixed maximum |
| --- | ---: |
| Supplied BTB1 rows, including all warmup and any ignored suffix | 70,000 |
| Retained rows, including all warmup | 70,000 |
| Exact six-column BTB1 bytes | 3,360,016 |
| UTC trading-window date labels | 2,200 |
| Definite plus possible midnight elements, globally across exposures | 30,000 |
| Copied gap objects, globally across exposures | 70,000 |
| Raw compact or final encoded bytes, including LF | 256 MiB |
| Supervised child wall time | 240 seconds |
| Supervised child observed peak RSS | 2.5 GiB |

Metadata/source are strictly admitted before opening the bars file. The header,
version, supplied row count, six columns and exact expected size are checked
before the full bounded read; trailing bytes and short/changed streams refuse.
Retained rows keep their original binary64 values. A future suffix stops at its
first open at/after the exclusive endpoint before suffix value validation,
matching the existing route. Ignored suffix values are not certified.

The complete supplied prefix warms the indicators/pivots continuously, with a
flat pre-window broker. The trading interval remains bar-open `[start,end)`.
There is no implicit tail, chunk, restart, interpolation, calendar widening,
aggregation, forced close or hypothetical trade at the endpoint. A final open,
pending or queued state is retained. Both drawdown concepts and unknown funding
remain explicit in the unchanged unit projection.

The fixed cap covers the outcome-blind original S1 run list: eighteen A/B/C
M30/H1 runs repeat `[2021-04-01,2025-01-01)`, `[2025-01-01,2026-01-01)` and
`[2026-01-02,2026-10-01)` (1,371, 365 and 272 UTC date labels). The original
full supplied inputs have 33,947 H1 / 67,941 M30 rows; even the prefix from
2021-01-03 through exclusive 2026-10-01 spans 2,097 date labels. These dates
justify a resource ceiling; they do not authorize executing those histories.

Before entering the shared strategy runner, a closed type-derived bound covers
its internal compact snapshot finite-check serialization and six-number-row/config
identity serialization. New snapshot collections, string roles or custom
marshalers fail closed. The raw allocation is row/input bounded. Its complete-result
serializer has a separate guard on the actual produced snapshot/state/order/event/
gap/assumption/event-link collections and conservative value byte bound, immediately
before JSON allocation. The original runner passes a nil guard and preserves its
behavior. Guard failure returns no partial result. Before large raw-envelope
serialization
or unit projection allocation, the owned typed result receives the existing
semantic/numerical validator plus native cardinality and global exposure caps.
The projection allocation bound reuses reviewed full per-type allowances and
actual order/row/day/exposure counts. A separate bounded value walk reserves
maximum JSON string escaping and numeric widths before raw/final serialization.
Unhandled types/custom JSON marshalers refuse. A dense case may hit the 256 MiB
conservative bound below 70,000 rows; row count alone never guarantees admission.

Encoded-byte bounds are not peak-memory or runtime promises. The qualified
command requires an external supervisor. The included Linux qualification
harness monitors its own fresh child using `/proc`, bounded pipe capture and
`wait4`, kills that child on observed RSS/time/output breaches or cancellation,
and refuses output if terminal peak-RSS evidence exceeded the cap. Sampling does
not guarantee observing every transient peak before it happens. Complete output
is accepted only after exit zero, strict single-document validation and all
resource/identity checks; failed partial bytes remain separately labeled.

## Fixed long H1 profile

`adaptive-history-long-h1-v1` is a second closed native profile for H1 source
and requests only. It uses the same six admitted original source/preset
combinations, existing cost selectors, prefix/warmup rules and semantic
validation. It does not accept caller-supplied limits. Its fixed maxima are
150,000 supplied/retained rows, 9,000 UTC trading-window date labels, 120,000
global exposure midnights and 150,000 global copied gaps. Its internal JSON,
raw output and final output caps are 512 MiB; the original profile retains its
256 MiB caps.

The 150,000-row ceiling does not promise that every input at that size runs.
The closed type-derived internal JSON bound is 284,914,957 bytes at 141,397
rows and 302,250,002 bytes at 150,000 rows. Both fit the long profile's
512 MiB cap and exceed the original profile's 256 MiB cap. Separate raw and
projection bounds use actual result cardinalities and can refuse a dense result
before serialization when its conservative bound exceeds 512 MiB. Row, day,
midnight and gap maxima are individual fixed ceilings, not a joint allowance.
The external supervisor remains required.

## Distinct native evidence envelope

The final schema is `strat-adaptive-volume-flag-unit-history-v1`. It contains:

- `profile`, the selected fixed native profile;
- `rawEnvelopeSchema`, always `strat-adaptive-volume-flag-history-raw-v1`;
- `rawEnvelopeSha256` and `rawEnvelopeBytes`, binding the freshly produced
  compact native raw document plus its trailing LF;
- `rawRequest` and `rawRequestSha256`, the admitted request and exact input hash;
- `projectionRequest` and `projectionRequestSha256`, preserving the exact input;
- `projection`, the unchanged `adaptive-unit-projection-go-v1` value contract.

There is no duplicated full `raw` object in the final envelope and no decode of
serialized raw state back into a second ledger. The owned typed raw state is
projected directly. Its separate raw schema/hash differs from portable Stage A;
the projection manifest names that exact new hash. All other accounting and
source/input/config/economic/numerical identities retain their existing meaning.
Declared provider/quote-side identity is not authenticated by a bar hash.

This schema is not admitted by the existing browser consumer. Original native
and WASM wrappers retain their 1,024-row/366-day unit profile, error documents,
wire bytes and exports. Native-tagged registration leaves no history command
or excluded-handler reference in a js/wasm build. Published releases are not
modified by adding this source.

## Qualification and study gates

```
python scripts/checks/adaptive-flag-history-capacity.py \
  --native=/absolute/path/to/heisentick --output-dir=/new/evidence/directory
```

The harness uses only invented bars and the frozen public invented source corpus.
It checks six exact routes by three cost selectors at 70,000 continuous rows,
20,000 warmup rows and nonempty long/short lifecycles, plus resource refusals,
calendar boundaries and supervisor failure paths. Record the exact source,
toolchain, native artifact, harness and all returned-byte hashes alongside actual
latency/RSS/output measurements. Dirty-source exploratory measurements do not
qualify a later frozen head.

Independent exact-source/capacity review and a separate coordinator release are
required before replaying the already frozen S1 inputs. Such a later comparison
must distinguish unchanged raw execution from Python-versus-Go unit accounting,
preserve all mismatches and original goldens, and identify inspected history as
inspected. HT210 archived-draw reproduction and volume variants remain separate.
No new holdout, parameter search, forward deployment or market order is enabled.


## Source-pin qualification after PR137 review (2026-10-15)

The bounded [HT210 review claim](https://github.com/spik3r/heisentick-backlog/blob/1978d2bcf4d237832e24b997dae4b3ae099d26ef/tasks/HT-210-adaptive-flag-trade-path-exit-audit.md#L486-L497)
reviewed the complete production-source inventory at
`d7ef1d06221d9bc79534a502a1acec7b1c8e7e9b` against base
`08ee262ce2f653aff7c4f5f7579097e3d5435379`. Exactly four of 319 compiler-input
source hashes changed: the native history CLI and three native history report
files. The two native dependency closures each changed only those three report
hashes; package/file membership and the WASM closure were unchanged. No engine,
Sequential accounting or metric arithmetic source changed.

Before renewing any source hash, compile-only observations with Go 1.22.12,
CPython 3.12.14 and the original pinned flags compared all seven existing
instruction streams on each of `linux-amd64`, `linux-arm64` and `js-wasm`.
All 21 streams matched their original hashes and instruction counts exactly.
ARM64 was cross-compiled on Linux AMD64, not executed on native ARM64. Independent
review accepted only four global source-hash updates and three dependency-hash
updates for each Linux target. Instruction hashes, accounting schedules,
`sourceParents`, membership, the verifier and compiler flags remain unchanged.
The renewed manifest SHA256 is
`c160c67e4858e589c7df9e51fd98c8dcee43c65d8dc9c945319ebfe82c36ac8f`.

After renewal, the unchanged official commands passed:

```sh
python -B scripts/checks/sequential-accounting-proof.py codegen "$PWD" "$PINNED_GO_1_22_12" "$EVIDENCE_DIR"
python -B scripts/checks/sequential-accounting-proof-test.py
```

The official codegen check covers the actual Linux AMD64 host and WASM; all
19 exact-arithmetic self-tests passed. The separate 21-stream independent
acceptance receipt has SHA256
`9c6cdc8713ae6e863f7678e4a205d4f72c10a558a97a8591e02f531aedb184f6`.

Resource observations are retained honestly: the initial combined observation
stopped its WASM compile at the 300 MiB working-storage budget. Its failure
receipt and complete emitted listing were preserved; verified lossless listing
compaction enabled a separately bounded WASM-only continuation with the same
cap and 512 MiB free-space reserve. The official codegen then passed. The first
self-test admission was held while compiler intermediates occupied headroom;
the same unchanged tests passed after those task-owned intermediates were
released. Neither event changed a qualification or CI guard.

This repair qualifies reviewed source pins and unchanged compiler schedules.
It does not independently re-accept the earlier Darwin capacity/historical
receipts, establish native ARM64 execution, or rerun market history. Exact-head
hosted checks and the separate private historical-evidence handoff remain
independent review requirements.
