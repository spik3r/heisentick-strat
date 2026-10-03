# Local Forward prefix fixture v1

`engine.RunForwardPrefixFixture`, native `enginewasm --forward-prefix
fixture.json source.strat`, and WASM `engineRunForwardPrefixFixture` use the
same Go adapter. This is an unreleased local qualification contract.

Input uses `dsl-conformance-run-fixture-v1`. The only accepted keys are
`schema`, `case`, `strategyId`, `symbol`, `timeframe`, `rangeMethod`, `costs`
and `bars`. All are required; strings are nonempty, `rangeMethod` is `zone`,
and 1–30,000 raw OHLCV rows have six finite cells. Costs explicitly supply
`fillOn`, `feePerUnit`, `slippage`, `slippageBps` and positive `startEquity`.
Ordinary DSL families with HTF mode off and no source timeframe are admitted
only on their declared route. Additional context, authored wrappers,
calculation-source changes and runtime parameter overrides are rejected.

Bar timestamps are nonnegative safe integers in strictly increasing order;
OHLC bounds and nonnegative volume are validated before execution. Supplied
bars are assumed explicitly closed by the caller; this transport has no clock
or provider feed from which to infer closure.

Success has schema `dsl-forward-prefix-v1`, `provenance.fixtureSha256` and
`provenance.sourceSha256` (SHA256 of the exact respective input bytes), and
`result` with the existing `PrefixResult` contract. It preserves open exposure,
emits stable `fp_` position identities, and never synthesizes a final-bar close.
Its `checkpointDigest` binds complete replay input; it is not resumable engine
state. No chart statistics, equity or skip counts are asserted by this contract.

Native and WASM failures share the schema and an `error` object with `code`
(`invalid-input` or `unsupported-route`) and `message`. Consumers must refuse
failure envelopes and never fall back to JavaScript or report liquidation.
App Forward replay adds native binary/source identity to its own checkpoint
and durable runtime identity; a published release/pin remains a separate gate.
