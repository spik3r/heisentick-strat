# Go source inspection, v1

This additive inspector is producer-owned metadata, not execution admission.
The canonical work is [HT228](https://github.com/spik3r/heisentick-backlog/pull/490).
Its future release is independent of the unchanged v0.30 adoption.

Native library: `engine.InspectSequentialSource(source)`.
Native command: `heisentick inspect-sequential-source --dsl-file=path`.
Engine WASM: `engineInspectSequentialSource(source)`, exactly one string argument,
returning JSON. No request flags can assert a capability, route or release.

The result schema is `strat-sequential-source-capability-v1`. `ok` means the
inspection/parser invocation succeeded, matching the existing parser envelope.
Ordinary DSL errors still appear in the unchanged `result` object, containing
`cfg`, `errors`, `warnings` and `diagnostics`. They are not an executable config.
Hard transport/parser failures contain only stable bounded error information
and envelope/build metadata, without a partial result or capability claim.

Source identity binds exact valid UTF-8 bytes and their byte count. Native
invalid UTF-8 and original WASM strings containing unpaired UTF-16 surrogates
are rejected before replacement encoding; sources must be 1 byte to 8 MiB.
The config identity is SHA256 of `encoding/json.Marshal(parsed.Config)` from Go,
identified as `go-encoding-json-config-v1`. It includes name/description and all
other Go cfg metadata, not merely economic semantics. No JavaScript serializer
may manufacture this digest. A source-only whitespace/comment change can leave
the cfg digest unchanged while changing the source digest. Renaming changes
the whole-cfg digest. A digest never establishes supported execution.

The producer reports actual Go version/target/module metadata and embedded
VCS revision/modified status when available. Missing VCS fields are null and a
missing module version is `unknown`; `(devel)` is not a release tag. Tests and
builds with VCS stamping disabled may legitimately lack those fields. Consumers
must verify actual downloaded assets against their process-owned release pin.
The inspector cannot attest the digest of its own external binary/WASM file and
accepts no caller-supplied release or asset identity.

## Scope and capability facts

Every fact has `status: supported|unsupported|unknown`, a stable code and an
explicit constraints array. Only the closed full E1/E2 config is classified in
this first inspection version. Classification uses the existing Go decoder,
never source substring heuristics, a storage ID or a research-approval list.
Valid legacy and other Go families remain parsed, with execution facts unknown
and `OUTSIDE_INSPECTOR_SCOPE`. This does not say those families cannot execute.

For valid full E1/E2:

- `engineSupported` describes an existing rule/config implementation.
- `genericSourceExecution` describes the existing generic native/config and
  fixture-WASM surfaces. All runtime input restrictions still apply: ordered
  contiguous valid OHLC, exact declared route, no calendar/source/HTF/window or
  force-route, and the existing cost contract. A terminal held position rejects
  the full run. A terminal pending decision becomes `no_next_bar`.
- `sourceAccounting` is unsupported. No general source/config accounting
  export is introduced here.
- `fixtureAccounting` is unknown with `EXACT_FROZEN_FIXTURE_REQUIRED`. The
  original v1 accounting companion admits four exact source/fixture identities.
  This inspector neither duplicates its allowlist nor evaluates a fixture.
- Calendar execution, preserve-open endpoints, public execution prefix and
  checkpoint resume are unsupported. Core causality/checkpoint tests are not
  full-policy public prefix or broker recovery support.
- Recovery remains unqualified/unknown. Data/provider/calendar readiness and
  consumer integration are unknown because inspection has no such inputs.

A successful inspection cannot make an editor or paper session executable.
The consumer must independently bind its actual adapter/export, pinned assets,
source, complete route/options, cost/unit/accounting model, data and endpoint.
Paper admission additionally requires genuine prefix and durable recovery
support. Source accounting, real-provider calendars and preserved endpoints
belong to later separately approved/qualified surfaces.

## Retained generic cost facts

Fill mode accepts exactly `open` or `nextOpen`. Fee per unit, absolute slippage,
slippage bps and raw start equity must be finite and nonnegative. Raw start
equity zero becomes 10000 under the existing generic normalization; this is a
description, not a new default for future real-market source execution.

Adverse offset is absolute slippage plus abs(reference price) times bps/10000.
Both components can coexist and apply at entry and every exit, including target
exits. Per-unit fees debit realized at both entry and exit. Existing `trade.pnl`
is exit credit and excludes the earlier entry fee; it is not all-fee trade net.
The existing model uses abstract fractional units, without an instrument
multiplier, FX conversion, margin or funding. Explicit authored positive risk
and absolute notional cap govern sizing. None of these facts validates actual
cost options or establishes real-market readiness.

## Verification and compatibility

The schema, native tests and actual Node-hosted engine-WASM parity harness cover
the complete current parser corpus, authored E1/E2 sources, invalid Unicode,
source/config/grammar identity, unknown families and closed failures. The
existing parser result, run surfaces, broker/accounting arithmetic and old
goldens remain unchanged. Strict compiler source/closure admission changes only
for the enumerated inspection files; all 21 instruction pins stay unchanged.
Native ARM execution, independent review, exact-head CI and later release/asset
verification are distinct gates. Node-WASM is not a browser or market claim.
