# Ordinary arithmetic source review: scheduled trade

Reviewed source candidate: `6c7dabf117569a805f8d9d757afdbcd8710c4c13`
(`Add scheduled settlement trade DSL family`), with arithmetic manifest renewal
on its descendant qualification commit.
This is a narrow source-inventory review for the existing ordinary backtest
arithmetic proof. It does not qualify the scheduled-trade family or assert a
new arithmetic result.

The scheduled family adds its own DSL parser and engine route. In `engine.Run`,
only a configuration identified by `isScheduledTrade` takes the new route; all
other configurations continue into the pre-existing `PrepareRun` path. The
scheduled checks in `PrepareRun`, shared-context construction and fixture
handling reject unsupported routes before ordinary execution. The new optional
`RunResult.ScheduledAudit` field is nil for existing families and uses
`omitempty`. No broker open, close, partial-management or ordinary financial
projection implementation changed. In particular, the three ordinary proof
compiler targets remain `engine.(*broker).applyPartialManagement`,
`report.ProjectOrdinaryBacktestFinancials` and
`report.validateOrdinaryFinancials`; their source files are unchanged by this
candidate. The ordinary proof's fixed synthetic source/request identities and
expected arithmetic leaves were not changed.

The exact reviewed source inventory and target dependency closures are recorded
in `scripts/checks/sequential-arithmetic-schedules.json`. They were regenerated
with Go 1.22.12. A Go 1.22.12 cross-compile on Darwin/ARM64 compared the
ordinary function streams at `origin/main` and this candidate for Linux AMD64,
Linux ARM64 and WASM. In all three targets, both ordinary report streams are
byte-identical. `applyPartialManagement` keeps its instruction count and every
instruction except one reference to the generated `partial` string literal:
AMD64 `stmp_205` becomes `stmp_209`, ARM64 `stmp_205` becomes `stmp_209`, and
WASM `stmp_204` becomes `stmp_208`. The updated pins record these exact streams.
No arithmetic operation or allowance changed.

The exact proof entry point requires a Linux host for codegen. Although a
task-local CPython 3.12 runtime is now available, this Darwin host cannot run
that wrapper. The cross-compiled stream comparison uses the exact pinned Go
compiler and target outputs but is not Linux-native execution evidence.
Exact-head Linux CI remains required; if it reports a closure or instruction
difference, review that difference rather than broadening the proof.

The change adds four source files to the source inventory, changes nine
existing source hashes, and adds four files to each engine build closure. The
ordinary broker/projection source hashes remain unchanged. These hashes admit
the reviewed family source; they do not establish ordinary runtime parity,
release readiness, or scheduled-trade arithmetic correctness.

The added `RunResult.ScheduledAudit` field also changes the by-value ABI of
`report.ProjectSequentialMetrics`, which is pinned by the separate sequential
arithmetic schedule. I compared the full Go 1.22.12 compiler streams from
`origin/main` and this candidate on all three targets. The old streams match
the manifest exactly. The new hashes are Linux AMD64
`f7e4458430e9c4a01a6948c05a35fe6d99af878ed942e5953596667a644a6468` (482
instructions), Linux ARM64
`07f567d2d9e8fa7c66c345ec569fd4551d6e2f7bce5ad8645dc5e1f44e7d41de` (567),
and WASM
`722cc14b2ca9f949ec8b9ebb3711569fdce5d1c0f6fa995cb2af264251dc99bd` (2176).
Only those three entries in the sequential schedule were renewed. The full
instruction streams are saved outside the repository under
`/private/tmp/ordinary-proof-asm-39946935449/`.

For this function, the AMD64 change removes three ABI/stack instructions and
moves the `RunResult` operand stack offset by eight bytes. ARM64 removes two
`MOVD` stack-copy instructions. WASM keeps the same opcode sequence and count,
with frame/field offsets shifted. Floating arithmetic mnemonics remain in the
same order on every target (15 AMD64 operations and 16 ARM64/WASM operations);
no arithmetic operation, operand value, or operation order changed. The full
manifest remains a strict instruction-stream pin; this review does not add an
exception or relax a proof check.

Before the header-validation follow-up, the sequential schedule manifest
SHA-256 was
`8055cfadb457e02e9524af46e6d0cc074b15848dd1bf2db45facf3761b3f6514`.
That digest was reported in the ordinary arithmetic proof compiler receipt; it
was not a separately allowlisted output.

## Header validation follow-up

Commit `3699f1b2d80d8f5cb077d5f770c224669f3b9901` changes only the scheduled
DSL validator and its regression test. The validator rejects trailing tokens
after quoted `strategy`/`description` metadata and after `dsl v7`, which the
legacy normalizer otherwise drops. CI correctly refused the stale source
inventory before reaching codegen. The renewed inventory records the exact new
SHA-256 for `dsl/scheduled_trade.go`; each Linux AMD64, Linux ARM64 and WASM
build-input closure changes only that same file hash, with no package or input
count changes.

Using the exact Go 1.22.12 compiler on Darwin/ARM64, I regenerated all three
target instruction streams. Every pinned function stream matches the manifest
byte-for-byte on Linux AMD64, Linux ARM64 and WASM. No instruction pin,
arithmetic operation, allowance, or proof check changes in this follow-up.
This cross-compile comparison is not Linux-native execution evidence; the
refreshed strict proof and downstream parity checks still require exact-head
Linux CI. The updated schedule manifest SHA-256 is
`76962b3c69049162709ed1b471e4ffe0e4230ee9457d103e88efdfb58ea48088`.
