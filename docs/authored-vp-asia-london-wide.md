# Authored VP Asia-London wide port

`RunAuthoredVPAsiaLondonWide` executes the active JavaScript
`vpAsiaLondonSweepContinuationFiveMinuteWideAsia` strategy as
`vpAsiaLondonSweepContinuationFiveMinuteWideAsia/go-v1`. It admits XAUUSD 5m
bars with close fills. `RunAuthoredVPAsiaLondonWideInteractive` returns the
versioned `authored-vp-asia-london-wide-interactive-v1` result with per-bar
marked and closed equity, fee-inclusive statistics and final cash. The
implementation is a dedicated Go producer API; it
does not claim that the Strat language can parse or express this setup.

The active variant inherits `onBar` and the default risk, profile, session,
stop and target parameters from the archived
`vpAsiaLondonSweepContinuation.js` base. It overrides the Asia range threshold
to the floor-indexed 67th percentile of up to 120 earlier qualifying ranges,
requires 30 earlier ranges, and limits the London open's distance to the
expected Asia edge to 0.25 of the completed range. The Go runner uses the
existing causal `AsiaLondonSweepState` and `ComputeVolumeProfile` primitives,
then sends its entries to the common Go broker. It processes stops and targets
before each bar's new signal, and closes an open trade at the first bar stamped
at or after 16:00 UTC if the bracket has not closed it.

`engine/testdata/asia-london-wide-whole-strategy.json` freezes the authored
JavaScript run over 37 synthetic UTC sessions, including 30 range-history
sessions, long and short trades, target and stop exits, a London-close exit,
and blocked distance, narrow-range and ambiguous/opposing first-raids. It
contains source SHA-256 values for the active variant, inherited base and JS
broker engine. Regenerate from an app checkout with:

```sh
node engine/testdata/asia-london-wide-oracle.mjs /absolute/path/to/heisentick-one-engine
```

The raw, 0.06 point slippage and 0.25-per-unit fee oracles are compared
against all three whole trades and their entry metadata. The fee oracle also
freezes every marked and closed equity point. The native entry point is:

```sh
go run ./cmd/enginewasm --authored-vp-asia-london-wide engine/testdata/asia-london-wide-interactive.fixture.json
```

The same exact active ID is dispatched by the WASM
`engineRunInteractiveFixture(fixtureJSON, "")` export. Both transports use the
same Go result; `node scripts/checks/authored-vp-native-wasm-parity.mjs` builds
both artifacts and compares their results. The Go result encodes the app's legacy
`london-close` exit as `reason: rule, rule: london-close`, per the Go engine's
versioned exit taxonomy. This is an intentional output encoding correction,
not a trading-rule change.

The dedicated API rejects unsupported routes and fill modes, incomplete or
invalid cost objects (including explicit zero start equity), malformed OHLCV,
and zero-trade runs instead of returning an apparently successful empty parity
result. It does not admit custom strategy parameters, magnifier execution, or
another instrument. It has no effect on ordinary DSL runs or conformance
goldens, and is not a performance or promotion claim.
