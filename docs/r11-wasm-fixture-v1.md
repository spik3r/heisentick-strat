# R11 WASM fixture contract v1

This is a dedicated browser bridge to the existing Go range-reversion
reference. It does not add range reversion to the generic fixture engine or
JavaScript runtime. The bridge admits only the two fixed, tested R11 rules:

- `r11-management-target-3r-30m`: XAUUSD M30, chart bounds 20, vector gates
  ADX <30 and CHOP >48, target 3R.
- `r11-entry-chop-45-1h`: XAUUSD H1, chart bounds 20, vector gates ADX <30
  and CHOP >45, target 2R.

The accepted DSL is byte-pinned as well as config-checked:

| Strategy ID | Required DSL SHA-256 |
|---|---|
| `r11-management-target-3r-30m` | `c99f0695cc47fb263a93e4d9bc58970d089cb62e23ffb714482368318cb9ef5a` |
| `r11-entry-chop-45-1h` | `5b0572545543910ea69abc04037d9b5675baca69be44a8ad034dea6efe7c9fd8` |

The 2.5R M30 and CHOP >46 H1 comparisons remain research evidence and are not
admitted by this browser contract. The rules retain delayed Pine OHLC policy,
completed H4 EMA200 with next-native-row availability, candle colour, 1.1 ATR
range expansion, ATR14, 0.6 ATR stop, one-R close-triggered break-even with a
10-tick offset, two-bar cooldown, and 0.01 tick size. This bridge does not
change the known synthetic entry-bar close-latch difference between the
Python reference and native engine.

## Call

The WASM module exports `engineRunR11Fixture(fixtureJSON, dslText)`. Both
arguments are strings. The fixture is one JSON object with exactly these
fields:

```json
{
  "schema": "strat-r11-fixture-v1",
  "contractVersion": 1,
  "strategyId": "r11-management-target-3r-30m",
  "symbol": "XAUUSD",
  "timeframe": "30m",
  "sourceTimeframe": "4h",
  "tradeFromMs": 1577836800000,
  "tradeToMs": 1609459200000,
  "execution": {
    "slippagePerFill": 0.06,
    "commissionPerUnitSide": 0.5,
    "units": 1
  },
  "entryBars": [[1577836800000, 1515.0, 1516.0, 1514.0, 1515.5, 100.0]],
  "sourceBars": [
    [1577836800000, 1510.0, 1518.0, 1509.0, 1516.0, 400.0],
    [1577851200000, 1516.0, 1520.0, 1512.0, 1518.0, 420.0]
  ]
}
```

`tradeFromMs` is inclusive and `tradeToMs` is exclusive. Both are UTC Unix
milliseconds on the selected entry-timeframe grid. Bar rows are original
binary64 JSON numbers in `[timestampMs, open, high, low, close, volume]` order;
the bridge does not infer, resample, trim, or synthesize data. Entry rows must
be strictly increasing and on the 30-minute or one-hour UTC grid. Source rows
must be strictly increasing and on the four-hour UTC grid. Prices must be
finite and positive, OHLC bounds valid, and volume finite and nonnegative.

The app fills `sourceBars` from the original H4 context rows, alongside the
original chart-timeframe `entryBars`; neither stream is projected into the
other. The app must confirm its loaded higher-timeframe identity is H4 before
calling this bridge.

The bridge rejects unknown or missing fields at every object depth, duplicate
JSON object keys, trailing JSON, nonfinite values, malformed rows, timestamp
or route mismatch, and a strategy ID whose source DSL, timeframe, or rule
parameters differ from the fixed definition above. Exact source DSL hashes
are checked before parsing. The decoder also checks every effective config
field against the strategy allowlist, including `DELAYED_PINE_OHLC_V1`, H4
`NEXT_NATIVE_ROW`, chart lookback 20, candle colour, H4 EMA200, vector length
14, ADX 30, route-specific CHOP minimum, range expansion 1.1, ATR14, stop
0.6 ATR, route-specific target R, cooldown 2, BE enabled at 1R with 10 ticks,
and tick size 0.01. H4 bars are passed directly to the native engine. A
completed H4 row becomes available at the timestamp of its next native H4 row;
the final supplied H4 row has no successor and is unavailable. A fixture must
include at least two valid H4 rows before `tradeToMs`, as required by the
native runner.

Resource limits are 8 MiB of DSL text, 64 MiB of fixture JSON, 500,000 entry
bars, 100,000 source bars, 64 MiB of serialized output, 500,000 signals and
250,000 trades. The schema-aware JSON token walk rejects nested values outside
the fixed shallow fixture shape and caps input at five million tokens. Before
encoding, the bridge rejects a conservative output budget
above 64 MiB, estimated at 512 bytes per signal, 1,024 bytes per trade and
64 KiB of fixed envelope data. The fixture byte and row caps bound
input-derived allocations; they are not a promise about exact peak WASM heap
usage. Execution costs must be present as JSON numbers, finite and
nonnegative; units must equal exactly one. Percentage sizing, basis-point
slippage, arbitrary parameters, transformed bars, and fallback execution are
unsupported.

## Result

Success returns this envelope:

```json
{
  "schema": "strat-r11-wasm-result-v1",
  "contractVersion": 1,
  "identity": {
    "fixtureSha256": "...",
    "dslSha256": "...",
    "strategyId": "r11-management-target-3r-30m",
    "symbol": "XAUUSD",
    "timeframe": "30m",
    "sourceTimeframe": "4h",
    "tradeFromMs": 1577836800000,
    "tradeToMs": 1609459200000,
    "execution": {
      "slippagePerFill": 0.06,
      "commissionPerUnitSide": 0.5,
      "units": 1
    },
    "effectiveConfig": { "rules": {} }
  },
  "run": { "schema": "strat-range-reversion-reference-v1" }
}
```

The two SHA-256 values cover the exact UTF-8 fixture and DSL argument bytes.
`run` is the existing `engine.RangeReversionResult` without rewriting its
ledger. It carries the native configuration and input-series hashes, signals,
closed trades, summary, assumptions, `censoredOpen` and
`pendingAtWindowEnd`. Consumers must verify every echoed identity field and
must not recompute the native summary or reinterpret missing/censored state as
a closed result. The Go summary owns one-unit price P&L and PF, planned-risk R
P&L and PF, commission and censored-state accounting. UI must label these
metrics as one-unit model-price research; it must not convert them into
account equity, percent return, or Pine-sized USD results.

Errors return the same result schema with an `error` object containing a stable
`code` and a human-readable `message`. Codes are `R11_FIXTURE_INVALID`,
`R11_DSL_INVALID`, `R11_IDENTITY_MISMATCH`, and `R11_ENGINE_REJECTED`.

The bridge call runs synchronously within its worker. Cancellation terminates
the dedicated worker; no cooperative Go cancellation function is exported.
