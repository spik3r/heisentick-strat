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
