# Strict original-payload window admission v1

`enginewasm --market-data-window request.json original-payload` and WASM
`engineReadMarketDataWindow(requestJSON, payloadBase64)` share one Go decoder.
The request has exactly `schema: market-data-window-request-v1`, `format`
(`json` or `btb1`), and nullable integer `fromMs`/`toMs`. Bounds are inclusive.

The complete original payload is admitted before filtering. JSON envelopes
require a `bars` array with exactly six finite numeric OHLCV values per row.
Null, missing, string and extra values are rejected. Envelope metadata is
opaque, never interpreted, and retained in the original payload hash. Duplicate
top-level fields and trailing JSON are rejected. BTB1 requires exactly six
columns and exact byte length; five-column zero padding, extra columns and
trailing bytes are not admitted. Existing permissive legacy decoders are not
changed. Invalid chosen data must never fall back to another format.

All rows, including those outside the requested window, require ordered unique
nonnegative safe-integer timestamps, finite prices, nonnegative volume and
consistent OHLC bounds. Strategy-specific route/cadence admission still belongs
to the execution operation. Maximum original payload size is 32 MiB; maximum
selected length is 30,000 bars. Empty selections are returned explicitly.

The result binds the exact request bytes and exact original payload bytes with
separate SHA-256 values. Selected rows have another digest and the declared
`strict-six-number-ohlcv-window-v1` normalization identity. Whitespace or opaque
metadata changes therefore alter original identity even if selected rows match.
No repair, fill, sort, deduplication or source-format fallback is performed.

Reproduce generic invented native/WASM cases with
`node scripts/checks/window-payload-parity.mjs <native> <wasm> <wasm_exec.js>`.
This opt-in contract does not collect data, identify its provider or certify
coverage/quality beyond the stated admission checks.
