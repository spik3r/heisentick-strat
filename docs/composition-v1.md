# Authored wrapper composition v1

The Go engine owns the route and execution adapter for five active authored
JavaScript wrappers. `engine/composition_manifest_v1.json` pins the app source
revision, all 14 symbol/timeframe routes, the order of children on each route,
the ten child `.strat` path/digest pairs, and each child's generated numeric
runtime params. Archived children remain required dependencies when an active
wrapper references them.

Call `engine.RunBuiltInComposition(request, sources)` with the wrapper ID in
`RunRequest.StrategyID`. `sources` maps each child ID to the raw text at its
manifest path. A caller can obtain paths from
`engine.BuiltInCompositionManifest()` and load the app's pinned source tree.
No JavaScript runtime, wrapper registry, or parent params are used. The Go DSL
parser compiles each child and the Go broker executes it. `RunComposition` is
also available for an explicitly supplied v1 manifest.

Every child referenced by the selected wrapper is required and checked by
SHA-256 before route selection. Missing, changed, unparsable, off-route, or
unsupported child semantics return an error, including dependencies used only
on a different route. Generated child params must match their DSL source; the
London Quality displacement parser gap is declared explicitly. An unlisted
route returns an empty standard result only after all its dependencies and
market input have validated.
Single-child routes run the child directly with the wrapper strategy ID in the
result. The balanced XAUUSD 15m route runs the long quality child before the
LL-short child on each bar. Its broker shares position, pending orders, fills,
P&L, session-break-hold cooldown, and daily seen state across both children.
The short child is called only when no position exists after the long child,
as in the authored wrapper.

The native/WASM bridge accepts `(fixtureJSON, childSourcesJSON)` as
`engineRunCompositionFixture` in WASM or `enginewasm --composition fixture.json
child-sources.json` natively. `childSourcesJSON` is an object keyed by child
ID. Errors return `{"error":"..."}` from the WASM function and a nonzero exit
from the native command. The app still owns source distribution and its own JS
registration until its consumer release adopts a pinned Go build. The manifest
is not a new home for strategy sources.

Tests include a full six-trade conformance fixture, an ordered two-child
fixture with both sides and no overlapping positions, shared cooldown and
rejected-entry retry fixtures frozen from the app's SBH runtime, fail-closed
dependency checks, and a route table lock. Frozen whole-wrapper JS trade
oracles cover all five active IDs with positive trades: balanced 15m, London
4h, continuation 1h, high-sample 4h, and trend-pullback 4h. The balanced and
London fixtures are targeted historical XAUUSD slices; the continuation
fixtures are existing run-conformance slices. When the app checkout is
available, run `HEISENTICK_APP_SOURCE_ROOT=/path/to/app go test ./engine
-run 'TestBuiltInComposition' -v` to verify exact source digests and replay
the oracles. The checked-in slices do not establish full-history strategy
profitability.
