# Authored Day Session ORB v1 producer port

`authoredorb.Run` executes the archived `daySessionOrbStrategy` and its active
`daySessionOrbOriginalTvParity` variant as complete authored strategies. The
former advertises XAUUSD 5m/15m; the latter advertises XAUUSD 30m. Both use the
same JS `onBar` algorithm. The Go port does not reinterpret either as a Strat
DSL setup family. Routes outside those preferred slices require explicit
`forceRoute`.

Input and output use the `authored-orb-run-v1` schema. Native callers can call
`authoredorb.Run` or `enginewasm --authored-orb request.json`. The full engine
WASM exports `engineRunAuthoredOrb(requestJSON)` and returns result JSON (or an
`error` object). Both adapters call the same Go function. The request supplies
OHLCV bars, optional numeric strategy params, execution costs, and optionally
one context VWAP value per bar. Without a context column, the producer computes
the ordinary JS context's UTC-day close-volume VWAP. Where that VWAP is not
finite, the authored strategy's AEST-day HLC3 mean applies. No server data or
external state is required.
An explicit `null` context VWAP value also selects that fallback.
Context VWAP advances on every input bar, including held bars, while the
authored `onBar` state remains paused for held positions. Requests reject
malformed OHLCV, negative execution costs, unknown params, and trailing JSON.

The frozen fixtures under `authoredorb/testdata/` cover both IDs, route
metadata, default short, long next-open target, trend-pullback gap stop,
session boundary, one-trade-per-day retry gating, engine-computed VWAP, and
HLC3 fallback including the source's first-bar double count. They were generated
from the app's JS broker at `a7bc97689838d7cc5e2d7fdf462b9ca2f5dbfd28`.
The only intentional compatibility quirk is named in that fixture README:
the JS `defineStrategy` wrapper pauses this strategy's `onBar` while a
position is open, leaving its session-close rule unreachable. This port
preserves that behavior under schema v1. A correction needs a new version and
reviewed fixture set. The port applies the JS default of one open position,
stop-first ambiguous OHLC exits, and no lower-timeframe magnifier input.
