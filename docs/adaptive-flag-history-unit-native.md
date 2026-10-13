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

## Distinct native evidence envelope

The final schema is `strat-adaptive-volume-flag-unit-history-v1`. It contains:

- `profile`, always `adaptive-history-v1`;
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
