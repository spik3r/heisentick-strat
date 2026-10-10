# Synthetic Sequential accounting expectations: public projection v1.1

These invented OHLC/accounting expectations were independently authored by Claude.
Astra prepared this allowlisted public projection. It is deliberately not a
byte-identical copy of the authored source. Private discussion/provenance and
authoring-process text are omitted; no economic input or expected value changed.
The machine-checked projection digest and transformation rules are in IMPORT.json.

## Authority and qualification

SOURCES.json pins the public Go contract, schema, source freeze and exact
arithmetic schedules at merged b574bfc903e3c6c8303c58ef34a0587df6e54968.
Prices, volumes, fees, start equity and risks here are synthetic. There are
30 cases:29 binding,1 provisional, each with long/short variants. Eight
injected-only items are descriptions for separately owned unit-test work.
Seasonal legacy, P1, real calendars, historical outcomes and profitability are
not qualified by these files.

The one provisional case is leg.same_bar_exit_reentry. Its expected numbers
remain preserved for diagnostic use; it must not be silently counted as binding.
Nonfinite overflow asserts typed refusal with no partial result. Exact error
code/field/precedence are advisory wherever the expected record says so.

A scratch reconciliation ran all60 variants against actual Go1.22.12 native
amd64 and Node-hosted Go/WASM from the pinned tree, including repeat checks.
That does not establish this corpus's native ARM64 or eight injection tests.
The separate producer88-case arithmetic qualification is not a substitute for
those missing corpus gates. Permanent adapter/import qualification is pending.

## Numeric representation

All decimal and rational expectation strings are copied unchanged. Exact
rational expectations remain distinct from operation-ordered binary64 checks.
For return, win rate and drawdown percentages, the accepted schedule rounds
division before multiplication by100. Eight drawdown variants distinguish that
order from rounding the final rational once. Do not replace either representation
or introduce an epsilon to make them agree. Use the pinned schedule and source
identities; unexplained numeric or categorical differences fail qualification.

Raw run.trades[].pnl is exit credit and excludes entry fees. All-fee
tradeAccounting[].netPnl and account headline.net are different Go quantities.
Marks use original bar-open timestamps and post-execution close-bar MTM values.
Dollar and percentage drawdown maxima are independent. Defined zero and null
with reason remain distinct; no JavaScript financial projection is authorized.

DERIVATIONS.md describes the unchanged expectation layout. Coverage lists the
author's expectation-level mutation observations, not Go mutation qualification.
This packet contains no execution implementation or generated engine goldens.
