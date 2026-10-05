# Native timed-return contract

This opt-in offline family is a bar-price execution proxy. It does not establish
executable bid/ask fills, an investable return, a literature replication, a
forward/live runner, or evidence of an edge. Its public examples are invented.
Research definitions and actual strategy parameters belong to consumer repos.

## Native DSL and CLI

The complete syntax is illustrated in
[`family-timed-return.strat`](../conformance/run/family-timed-return.strat).
Every `timed` field is required exactly once. This family requires one directive
per physical line and nonnested blocks: a section opener ends in `{`, and its
closing `}` occupies a separate line. Inline bodies are rejected independently
of legacy line expansion. Version/type headers consume their full line; strategy
names and descriptions are single quoted strings without trailing text, braces
or `#`. The only optional filter is
`side long only`, `side short only`, or `side both` in a filters block. Generic
entry, risk, stop, session, calendar, monthly and event filters fail closed.
Exactly one explicit slice is required, with timeframe 1m, 5m, 15m, 30m or 1h.

```
heisentick timed-report --dsl-file=example.strat --calendar-file=calendar.json \
  --symbol=SYNTHUSD --tf=1m --data-root=./data --slippage=0.1
```

The command loads only the declared local BTB1 file, requires explicit absolute
per-fill slippage, and optionally accepts nonnegative `--slippage-bps`.
Every flag requires an explicit nonempty value (`--flag=value` or `--flag value`);
bare `--slippage` and `--slippage-bps` are errors, never boolean-style defaults.
It emits
`strat-timed-return-report-v1` with SHA-256 identities of the exact DSL, calendar
file and binary snapshots read. Its `netPriceUnits` is the sum for one fixed
unit with adverse entry and exit slippage. Commission is unsupported and the
native API rejects nonzero `FeePerUnit`; `costComplete` is always false. No
currency conversion, risk normalization, financing, fees or portfolio is implied.

The checked `engine.Run` API, fixture path and CLI share the same runner.
A nonnil timed calendar paired with any other family is rejected before generic
execution or preparation, including off-route and otherwise empty results. Generic
report/grid preparation, prefix/resume, source-timeframe transfer, clipping,
HTF and generic report context deliberately refuse this family. No app pin,
WASM/browser runner adoption, default switch or release is included.

## Anchors and availability

`current/previous session open` means O of the bar starting at the reference
open. `current/previous session close` means C of the bar ending at its reference
close. `current clock HH:MM open` and `current clock HH:MM completed-close` name
these price conventions explicitly. A completed boundary close is the preceding
bar's close, never a substituted session opening price.

Bar timestamps denote interval starts. A close at T + timeframe is assumed
available at that boundary under an idealized publication model. The required
ordering is signal start < signal end <= decision (entry boundary) < exit
boundary. Exact endpoint price comparisons select with/against direction; log
returns are audit metadata. Exactly equal endpoint prices yield a dated
no-trade outcome.
Entry and exit use O exactly one timeframe after their respective boundaries,
with no nearest-bar lookup, adjacent-index shortcut or same-timestamp close/open
ordering assumption. Both fills must remain on the reference local date.

## Calendar and complete-dataset admission

`timed-return-calendar-v1` sidecars are represented by `TimedReturnCalendar` in
`engine/timed_return_types.go`; both run fixtures include small complete examples.
Timezone must match the DSL and be UTC or America/New_York. The caller supplies
base64 TZif bytes (1–65536 bytes) and their verified SHA-256. No host timezone
database is used for decisions. The fixtures pin IANA 2026b TZif bytes. Dates
2000–2099 are supported; the exclusive end bound may be 2100-01-01. Gaps and
folds in local clocks are rejected rather than normalized.

Reference-source and execution-source SHA-256 values identify caller-supplied
provenance; the engine cannot authenticate the source or decide whether it is the
correct market calendar. Every civil date in the bounded calendar is explicit
and consecutive. Rows are `full`, `early` or `closed`. The previous session is
the immediately preceding nonclosed row, including early closes. Required
predecessor context must be supplied, with no skipping to a favorable date.
Early/closed current sessions and early predecessor sessions are recorded as
excluded. Full eligible sessions alone create execution obligations.

Reference session anchors and quoted execution availability are separate.
Quoted UTC intervals are sorted, nonoverlapping, half-open [fromT,toT). A fill
at an interval's end is outside it. A fill after reference close is admissible
only when its exact timestamp is inside the separately supplied quote interval.
The engine never moves that fill earlier to fit the reference session.

Before evaluating any endpoint return, the entire batch must have both exact
entry and exit bars for every eligible schedule date, inside the quote intervals,
with positive finite post-slippage prices. Any failure rejects the whole dataset
with the date, role and timestamp. Even a day whose eventual signal is zero or
missing has this obligation. This is retrospective dataset admission, not causal
forward batch acceptance, and cannot justify selecting or dropping days by P&L.
Missing signal endpoints remain dated `unavailable_endpoint` rows. Zero,
side-blocked, excluded and executed days remain in the audit. All supplied bars
must have strictly increasing unique integer timestamps on the declared grid,
positive finite coherent OHLC, and finite nonnegative volume when supplied.
No end-of-test fill substitutes for a scheduled exit.

## Conformance and adoption

The two new run fixtures exercise one-unit, two-fill slippage and previous-session
anchors across a weekend/DST change. Their prices and calendars are invented.
Existing parse/run goldens must remain byte-identical. Native tests additionally
exercise unsupported syntax, typed prices, gap/fold refusal, missing/invalid bars,
whole-batch admission, side routing and CLI output/failure parity.

The manifest classifies this family, aliases and directive as `go-only`.
The existing consumer JS grammar generator currently validates all canonical
examples through its JS parser, including Go-only entries. Consumer adoption
therefore needs a separate reviewed native-only filtering/validation change;
this producer change does not silently teach JS different strategy semantics.
The Go passive grammar table was regenerated using the existing official
`renderGo` projection for non-JS-only entries, with the manifest hash retained.
