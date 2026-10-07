# Adaptive flag unit reporting

The dedicated unit route runs the same Go adaptive flag engine used by the raw
report, then applies the qualified fixed unit accounting core to its freshly
owned result. Named INITIAL, TWEAKED and SNAPSHOT_C baselines are supported.
The generic engine routes keep their existing family refusals.

Native command:

```
heisentick adaptive-flag-unit-report \
  --request-file=raw-request.json \
  --projection-file=unit-request.json \
  --dsl-file=baseline.dsl \
  --bars-file=bars.btb1
```

The Go/WASM export accepts exactly four primitive arguments:

```
engineRunAdaptiveFlagUnitReport(rawMetadataJSON, projectionMetadataJSON,
                               sourceText, ordinaryUint8ArrayBTB1)
```

It returns a complete JSON string synchronously. It requires the original matching
Go shim and intact runtime imports at initialization. There is no caller-supplied
raw-ledger input and no separate JavaScript accounting implementation.

Raw metadata uses the unchanged adaptive-flag-runtime-request-v1 schema. Projection
metadata uses exactly:

```json
{
  "schema": "adaptive-flag-unit-request-v1",
  "scenario": "UNIT_POINT_VALUE_1",
  "numericalPolicy": "BINARY64_ORDERED_V1",
  "costPolicy": "RAW",
  "dataSource": {
    "id": "INVENTED_EXAMPLE_REVISION_1",
    "sourceSha256": "0000000000000000000000000000000000000000000000000000000000000000"
  }
}
```

The example identity is a placeholder. Supply the actual immutable dataset/revision
identity and original source digest. Data source identity is declared by the caller;
it is not a certified provider identity. Revising prior candles requires revising
that identity. Extending the delivered prefix of the same underlying revision
preserves causal candidate IDs.

Cost profiles are RAW, RAZOR_PROXY_BASIC and RAZOR_PROXY_HARSH_AGGREGATE. Quantity is
fixed at creation at 1/abs(raw trigger minus frozen stop), without compounding,
lot rounding or resizing at a gap fill. A loss may exceed 1R. These outputs describe
UNIT_POINT_VALUE_1 research units, not account dollars or percentage returns.
Funding is unknown and null. An open position stays open; hypothetical liquidation
is a separate diagnostic and does not create a closing fill or booked exit cost.
No capital, leverage, fee override, funding assumption or account options are
accepted. CUSTOM and G research overlays remain outside this public unit route.

Both metadata strings and source are admitted before bars are accessed. Raw request
and source limits remain 4,096 UTF-8 bytes each; unit metadata is limited to 1,024.
BTB1 retains the Stage A six-column, 16,384 supplied-row profile. Unit reporting
adds a 1,024-row retained-prefix limit including every supplied warmup row, with no
trimming. Windows span at most 366 UTC date labels, within years 1970 through 9999.
Aggregate exposure lists are bounded at 732 midnight elements and 1,024 copied
quote gaps. Timestamp/count/calendar and conservative output admission precede
raw engine allocation; actual exposure counts precede economic array allocation.
Ignored future suffixes keep Stage A's cutoff-before-validation behavior.

Success schema is strat-adaptive-volume-flag-unit-runtime-v1. Fixed outer order is
schema, projectionRequest, projectionRequestSha256, rawEnvelopeSha256, raw,
projection. The exact original Stage A bytes, including indentation and trailing
newline, are embedded after the raw key without reserialization. The raw object's
rawOnly and unavailable-stage-a economics labels remain its own unchanged labels;
the separate projection sibling contains the unit accounting. Request and raw
hashes bind their exact byte domains. Build identity reports the actual Go target
and available VCS stamp, including unverified status when a stamp is absent.

Errors use adaptive-flag-unit-error-v1 with closed code/phase/location fields.
WASM returns the error JSON string; native emits the same JSON plus newline and
exits nonzero. A failed stdout write is a failed operation, and any bytes from a
nonzero process exit must be discarded. Existing raw routes keep their old output
and refusal behavior.

The initial profile is intentionally bounded. Encoded output is capped at 64 MiB,
with a conservative type-derived proof before serialization. This is not a peak
memory, mobile capacity, real-browser latency or long-history qualification.
Actual ARM64 unit execution, browser qualification and Backtester adoption remain
separate gates. The fixed original invented corpus and independently retained
Python outputs are used for local public-route comparison; this route does not
run the Python reference.
