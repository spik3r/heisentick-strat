# Local raw mutable draft prefix v1

This distinct unreleased contract preserves open exposure for the strict source
subset in `interactive-draft-v1.md`. Native `--interactive-draft-prefix` and WASM
`engineRunInteractiveDraftPrefixFixture` consume the same exact run fixture and
separate source, and return `dsl-interactive-draft-prefix-v1`. Finalized draft
execution and historical `RunPrefix` contracts remain unchanged.

Go revalidates strict source/route/range, all five explicit costs, 1–30000 raw
bars, safe ordered timestamp grid/gaps and OHLCV bounds. Calculation is raw only;
fillOn is close or nextOpen. Positive starting equity, nonnegative finite costs,
exact keys and no params/auxiliary rows/windows/finalization switches are required.
SMA retains its established next-open family behavior under global close costs.
Optional `sourceAssertions` is admitted only under the exact consistency contract
in `interactive-draft-v1.md`, rechecked against source on every prefix call and
echoed canonically in success. It does not supply execution overrides, replace
the raw-only restriction, or change position IDs.

Success contains `schema`, exact-byte source/fixture `provenance`, `costs`,
`calculationSource: raw`, `strategyVersion`, `result` and `tradeNetPnl`.
`result` is Go's preserved `PrefixResult`: closed records with `positionId` and
original `trade`, open snapshots with Go stop/target/size/metadata, and a full
`checkpointDigest`. No synthetic final-data close is introduced. An unfilled
final next-open entry is pending and absent from both lists until an extension
fills it; this response is replay evidence, not a serializable pending-order
checkpoint. Replay the complete prefix to extend it.

Go computes `tradeNetPnl` in the same order as `result.trades`, subtracting the
entry fee from original exit-fee-inclusive trade P&L. Current strict families
forbid partial exits; any partial result is refused. Open snapshots do not
contain charged entry fees, marked equity or portfolio net/statistics. Consumers
must not invent those summaries from the snapshots.

The unchanged core `fp_` position-ID algorithm is namespaced by `strategyVersion`,
a Go canonical hash of exact source SHA, normalized costs and the first raw bar.
It distinguishes source/cost/history-origin edits and remains stable when the
same source/cost/history origin is extended. Runtime/build scope is the verified
artifact manifest; IDs are scoped to that source/runtime/cost-bound run. Require
unchanged history origin and append-only history. The full checkpoint digest
binds the replay input and is not resumable state or an append-only proof.

Failures use this schema plus structured `error.code/message`. Native/WASM share
the Go implementation. Consumers validate schema, exact input/artifact identity,
raw mode/costs, finite records, unique position IDs and aligned net values. Keep
original Go records separate from reporting projections.

PR #299's frozen JS manifest/snapshot remains unchanged and retains its legacy
engine identity. This transport uses independent synthetic qualification only;
it neither collects prospective evidence nor relabels old runs as Go. Future
prospective adoption requires a separately decided source/runtime-bound manifest.
