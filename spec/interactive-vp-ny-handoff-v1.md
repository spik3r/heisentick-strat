# VP NY handoff interactive composition v1

`dslSessionBreakHoldNyFocusBalancedFifteenVpVeto` is an app JavaScript wrapper
over archived Session Break Hold DSL. Its `dslText` is exactly the unwrapped
base, so the ordinary executor would otherwise discard the VP filter.

Use `engine.RunInteractiveVPNYHandoffVetoFixture(raw, source)` or the equivalent
`enginewasm --interactive-vp-ny-handoff fixture.json source.strat` and WASM
`engineRunInteractiveVPNYHandoffVetoFixture(rawJSON, source)`. The result schema
is `dsl-interactive-vp-ny-handoff-v1`; the ordinary interactive result remains
`dsl-interactive-run-v1`. Generic `RunInteractiveFixture` and `RunFixtureCase`
reject the wrapper strategy ID. A failed composition call returns an error,
never an unwrapped result.

The composition binds strategy ID
`dslSessionBreakHoldNyFocusBalancedFifteenVpVeto`, the archived base DSL SHA-256
`c3fa897885f84b61d718b364e78448457016814efd101125333a87df7878a405`,
and an XAUUSD 15m chart/1h HTF `zone` route. It requires the ordinary
interactive cost contract, causal fixed-grid HTF validation, and finite,
positive-volume, positive-price, internally consistent chart OHLCV. Missing or
unrepresentable VP inputs return `ErrInteractiveUnsupported`. The caller must
select this composition explicitly and attach the verified build digest when
presenting a result. A future DSL edit or different wrapper options require a
new reviewed composition version.

At each UTC day, the Go broker builds a volume profile from available 07:00
through 11:59 chart bars (minimum four). It distributes volume into 24
tick-quantized price rows, calculates a 70% value area, and compares the first
12:00 through 15:59 destination open to VAH/VAL. A destination open outside
the value area activates the filter only when the source edge is within 0.25
source ranges. Until a strict high/low sweep resolves that edge, it vetoes a
Session Break Hold entry opposite the predicted sweep. The veto leaves that
setup's daily-seen and cooldown state untouched. Accepted entries in the
active destination window carry `meta.vpNyHandoff`. The same broker still owns
fills, costs, fees, per-bar marked and closed equity, and final cash.
The POC near-tie rule retains the maximum-volume reference when it selects a
slightly lower-volume higher row, then starts the value-area expansion with
the selected POC row's actual volume.

The composition reports `skipReasonSchema: "dsl-skip-reasons-v2"`. It retains
the v1 Go market-gate reason priority and adds
`filter.vp_ny_unresolved_raid` when an otherwise admissible setup reaches a
blocked entry. The ordinary route keeps the v1 skip schema.

`engine/testdata/vp-ny-handoff-js-oracle.json` freezes the JS base and wrapped
whole-strategy outputs at app commit `22de07ba`. Its 4,300-bar real-data
conformance fixture has the 2019-03-20 12:00 open/low changed to 1301.7 and
1301.65, making the wrapped strategy veto an actual 13:00 long. The JS base
has six trades; the wrapper has five and one VP skip. A second destination
open of 1305.4 verifies accepted-entry VP metadata. A five-price near-tie
case pins JS VAL/VAH at 100/100.3 and a short veto at a 100.39 destination
open. Tests compare all common Go/JS trade fields, every JS marked-equity
point, JS-derived closed equity, net P&L, skip count, prefix curves, HTF
behavior and fee-inclusive accounting to this frozen evidence. The native/WASM
parity matrix executes both the veto and allowed-entry composition routes,
including their complete trade and equity responses. This oracle is a semantic test, not a
strategy promotion or a conformance golden regeneration.
