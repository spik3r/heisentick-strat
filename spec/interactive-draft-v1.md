# Local mutable DSL interactive v1

This unreleased opt-in contract admits mutable SMA Golden Cross, Dual EMA
Resumption and a fixed Failed Breakout construct with mutable risk, without a catalog source-hash whitelist. Other families and
constructs require further qualification. It does not change historical
`dsl.Parse`, the frozen `dslEditorStrategy` source or conformance goldens.

`InspectInteractiveSource` / native `--interactive-source-profile` / WASM
`engineInspectInteractiveSource` consume `{schema,symbol,timeframe}` and optional
`calculationSource` (`raw` or `heikinAshi`, default `raw`)
with `schema: dsl-interactive-source-profile-v1`, and the source as a separate
string. Nonempty symbol/timeframe are required. The result contains `parse`
(Go config and diagnostics), `profile` (family, symbol, timeframe, rangeMethod,
maxBars 30000, selected calculationSource) and SHA256 of exact request/source bytes.
The host verifies the executable identity and binds it across preflight and run.
Preflight validates executor config support using a dummy bar; it performs no
strategy execution or market-data inference.

Source uses draft-specific `dsl.ParseStrict`: exactly one `dsl v7`, setup type
and nonempty complete `slices` declaration; exact long-only side syntax; finite
decimal period/risk tokens without ignored suffixes; no unresolved parser
errors or warnings. Family-specific audits own the admitted phrases.
Unknown setup types and multiple types fail instead of retaining a default.
HTF, source/entry timeframe, microstructure/morphology, approach, grade and
other unaudited directives are refused. This narrower source profile does not
claim support for all ordinary-family directives or rewrite older syntax.
All draft families additionally audit original physical section lines: no
non-comment content after an unquoted closing brace, and no discarded prefix
before the first inline directive. Quoted braces are metadata; `#` starts a
comment. The historical parser and strict parser remain unchanged.
An opening brace whose close is on a later line must have only whitespace or a
comment after it; opening-line bodies otherwise disappear in historical lexing.

## Fixed Failed Breakout source subset

Failed Breakout requires exactly one of each execution declaration below. Only
USD risk is editable; `range method zone` asserts the qualified range algorithm.
Strategy name and description are optional metadata. Use braced sections with
one complete phrase per physical line, or one phrase in a complete inline
section. Blank lines and `#` comments are allowed. Duplicate sections or
assignments, empty/unknown sections, nested/unclosed braces, discarded inline
prefixes and trailing content are refused. `riskUsd N` is an alias for the same
risk assignment; it cannot appear together with `risk N USD`. N must be a finite
positive decimal without suffixes or exponent notation.

```dsl
dsl v7
strategy "Fixed Failed Breakout" {
  description "Existing Go setup; mutable USD risk only."
}
market conditions { slices(XAUUSD 5m) }
setup { type: failed breakout }
filters {
  side long only
  range method zone
}
execution { risk 200 USD }
```

This subset retains the existing Go defaults: range active within eight bars,
PDH/PDL/WH/WL priority with 1.5 ATR proximity, range-low sweep by 0.1 ATR and
reclaim within three bars, three-bar approach, existing bullish pin/engulf/outside
trigger, recent three-bar extreme stop plus 0.25 ATR with maximum 1.5 ATR,
opposite range-edge target/minimum 0.6R/fallback 1R, breakeven at 0.75R plus
0.02 ATR, three-bar cooldown and daily edge-price deduplication. Asia/London/NY
session gates, ranging/choppy classification with the existing ER <= 0.55
escape, and maximum movement ER 0.8 remain active. These are fixed Go behaviors,
not newly configurable phrases. Custom sweep/approach/trigger/levels/day gates,
stops/targets/management/grades/partials/limit entries and auxiliary timeframes
are all refused. Historical ordinary sources are unchanged.

Failed Breakout accepts **raw calculation only**, enforced in inspection and
execution. SMA/Dual HA qualification does not authorize this family's range
and level dependencies. This is additive capability in unreleased local v1;
release adoption still follows the producer semantic-version/review policy.

`RunInteractiveDraftFixture` / native `--interactive-draft` / WASM
`engineRunInteractiveDraftFixture` consume only `schema`, `case`, `strategyId`,
`symbol`, `timeframe`, `rangeMethod`, `costs`, `bars`, optional `calculationSource`
(`raw` or `heikinAshi`, default `raw`), plus separate source.
Schema is `dsl-conformance-run-fixture-v1`; strategyId is `dslDraftStrategy`.
All strings and all five cost fields are explicit. Source is reparsed and route
admission rechecked at execution. Range method must match the Go profile.
Unknown/case aliases, duplicate JSON keys and trailing JSON fail. No parameters,
context, auxiliary rows, other transformations or execution windows are accepted.

Explicit Heikin-Ashi uses the existing `strategy-calculation-source-v1` causal
recursive transform for strategy calculations only. Execution fills, stop/target
touches and equity marks use the supplied raw OHLCV, including raw next-open
prices. The first raw bar seeds the transform; missing periods do not reseed it.
No HA plus source/HTF composition is admitted by this chart-only draft contract.
Absent calculationSource preserves the previous raw request/output meaning.
Consumers bind the selected calculation source across preflight and execution;
the exact request and execution fixture digests include any explicit selection.

Input has 1–30000 six-cell finite raw OHLCV rows with safe nonnegative integer
UTC timestamps, strictly increasing and on the declared fixed timeframe grid;
OHLC bounds and nonnegative volume must be valid. Gaps of whole periods are
allowed. For multiple rows, at least one adjacent declared-period pair is
required to verify cadence; an entirely gapped series is refused. Supported
durations are 1m/5m/15m/30m/1h/4h/1d. The caller supplies closed bars; this
contract does not infer closure from a provider clock.

Success uses `dsl-interactive-draft-v1` and the existing interactive accounting,
curves, trade outcomes, stats, skip schema, final liquidation and exact-byte
provenance from `interactive-run-v1.md`. Both targets reuse the same Go engine
and return structured `{schema,error:{code,message}}` on failure. The reserved
draft identity is rejected by ordinary interactive/conformance/direct run
entrypoints. Draft preflight is not an authorization token for another source.

Consumers must preserve Go accounting, label legacy idle JS diagnostics as
advisory in Go mode, reject unsupported requests visibly without fallback,
and prevent stale source/route/build results from being published.

## Optional registered source assertions

Inspection, finalized draft execution and raw draft-prefix execution additionally
accept `sourceAssertions`, using `dsl-source-assertions-v1`. It checks a caller's
registry snapshot for source consistency, never overrides Go's parsed config.
The outer execution strategy ID remains `dslDraftStrategy`. Its six explicit
fields are `schema`, `strategyId`, `params`, `contextOptions`,
`contextRequirements`, and `preferredRangeMethod`. Unknown, duplicate, missing,
case-aliased, null, or incorrectly typed fields fail closed.

This checkpoint qualifies only `dslDualEmaResumptionXauusdDaily` on XAUUSD 1d
and `dslDualEmaResumptionXauusdFourHour` on XAUUSD 4h, with the strict Dual family.
`params` requires exactly nine finite JSON numbers: `fastEmaLen`, `slowEmaLen`,
`slowRiseBars`, `atrLen`, `stopAtr`, `trailAtr`, `allowLong`, `allowShort`, and
`riskUsd`. Each must equal its effective Go-parsed source value. Emptying params
or substituting a registry default cannot bypass this check.

`contextOptions` is exactly `{}` and `contextRequirements` exactly `["sessions"]`.
This preserves the wrapper's inherited capability declaration; it does not add
a session trading filter to Dual's family execution. `preferredRangeMethod` is
exactly `"zone"`, preserving registry metadata. The actual source profile remains
`pivot`; execution with `rangeMethod: "zone"` still fails. Metadata assertions
never change trading behavior or source admission.

Every operation reparses source and rechecks assertions independently, then echoes
the canonical object as `sourceAssertions` in success. Exact request bytes,
including assertions, remain in fixture provenance. Omitted assertions preserve
the prior request/result shape. Other interactive operations reject this field.
Finalized raw/Heikin-Ashi qualification and raw-only prefix restrictions remain.

This does not authenticate registry source or authorize a new caller. Consumers
must snapshot their actual definition, bind its ID and exact canonical source,
verify Go's echo and executable identity across asynchronous boundaries, and
fail visibly on unsupported options. Daily stays archived. This producer-only
checkpoint does not adopt a registered CLI or alter lifecycle. Source-consistent
edits can pass this contract; the consumer's canonical source binding remains a
separate mandatory boundary.
