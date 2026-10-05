# Offline quote/window preflight

Bounded Go primitive slice for [HT-128](https://github.com/spik3r/heisentick-backlog/blob/codex/ht128-preflight-claim/tasks/HT-128-gold-close-momentum-validation.md).
The [contract proposal](https://github.com/spik3r/heisentick-backlog/blob/codex/ht128-preflight-claim/strategy-plans/reviews/ht128/implementation-contract-proposal.md)
records proposed numeric settings and the remaining gates. It is not a frozen
outcome-ready protocol.

`marketdata.NewResearchQuotes` copies and validates strictly increasing source
event and availability timestamps and finite positive bid <= ask. It never
sorts, fills, rescales, repairs or silently drops duplicates. Same-millisecond
events need a separate sequence contract and are currently refused.

`Before` requires availability strictly before a decision and bounded age from
the source event. `FirstAfter` requires an event at/after submission and its
availability by an inclusive deadline; late delivery of an older event cannot
serve as a post-submission observation. Stale observations can be skipped only
within that bounded query. Neither function makes a broker-fill claim.

`InspectResearchWindow` requires explicit source/execution session bounds and
ordered decision/entry/exit deadlines. Exit deadlines are strictly before
both closes. It selects the first fresh observation after the prescheduled
exit submission, never the last observed quote before a future closure. A
missing exit retains the entry under `unresolved_exit`; there is no P&L,
return, promotion or outcome-readiness field. Missing input is not a flat trade.

`ResearchLocalMinute` resolves four explicit research clocks (UTC, New York,
London, Tokyo) in 2000–2099 and rejects invalid, nonexistent or ambiguous wall
minutes. It uses Go LoadLocation (including possible ZONEINFO, system, GOROOT or
imported tzdata fallback). The actual resolved source/version/hash must be
pinned before outcomes; a host package version alone is insufficient. It is not a holiday calendar; even weekend wall times resolve
normally. Caller-supplied daily sessions need independently verified historical
product/platform applicability. Latest schedules do not prove past availability.

The tests use invented quotes and interval bounds, including the original
fixed-21:00 overlap as a deliberately conditional clock fixture. They do not
assert that real source and execution calendars match. Each of the seven
planned cells exercises the same primitive code, without implementing its
strategy, rROD/RV, costs, sizing, calendar ingestion or statistics.

This slice changes no parser/DSL/engine semantics, conformance goldens, runner,
strategy registry, release pin, data acquisition or broker behavior. Existing
consumers do not call the new functions. No external Go dependency is added.

Validation from the repository root:

```sh
gofmt -l .
go vet ./...
go test ./...
go run ./cmd/conformance check
go test -count=1 -race ./marketdata
```

Passing these tests demonstrates synthetic mechanics only. Actual quote
lineage, full eight-year coverage, continuous-window integrity, source-to-broker
execution and numeric cost/financing contracts remain unverified, so real
outcome measurement stays blocked.
