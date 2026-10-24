# Ordinary arithmetic source review: 2027 settlement calendar

Reviewed candidate: `eac7a7d6a99a320264adf02f39d75b998084c42e`
(`Extend scheduled settlement calendar through official 2027 holidays`),
based on `f2c1aa3ba27902f9f2662b31ef4139ae6c0ab997`.

The calendar extension changes three Go source files in the global source
inventory: `engine/conformance.go`, `engine/family_scheduled_trade.go` and
`engine/scheduled_trade_calendar.go`. The engine build dependency closures
also include the updated embedded calendar JSON. The package sets are
unchanged: 14 packages and 268 files for Linux AMD64 and ARM64, and 15
packages and 277 files for WASM. The only dependency-file hash changes are
those three Go files and `engine/japan-settlement-holidays.json`.

| Build input | Old SHA-256 | Candidate SHA-256 |
| --- | --- | --- |
| `engine/conformance.go` | `df9953a55878380cfb11620a8f82dafc08d633a94ccc6f089fe2e528ad2d220a` | `65829598910bb0463ff9502672ca90f845b31dc99c3418304bb886791c0eb9c8` |
| `engine/family_scheduled_trade.go` | `e31ea8915044a198368bd41f9399932567b2e51786a0356269bd2ab5b36178e2` | `ab8f83e38c30e08a977e3532aa76c3f3973d8717e60a90042c78044f485d3713` |
| `engine/scheduled_trade_calendar.go` | `b69a6e7b7377c998176734a3c472f16f71ccf6a597f4a74a0b313d8cc13007cf` | `209ce0b463848316f45a34d5cfc3888a3fc687c7ea395078e7010f8fe33310ab` |
| `engine/japan-settlement-holidays.json` | `e24f88cd62388e8fa8ce0b4ef55d573af9f372a570a7b29fd74926103450d0d6` | `ab0bd1f693e129aa0bef7a0db0f6f7ab75f8b4ce44b8e5594c7a43ca0e54cc21` |

I regenerated those source hashes and target dependency closures in
`scripts/checks/sequential-arithmetic-schedules.json`, and added the candidate
commit to `sourceParents`. The strict compiler instruction pins remain
unchanged. No fixed arithmetic inputs, expected leaves, schedules, proof
flags, verifier code or instruction allowances changed.

Using Go 1.22.12 on Darwin/ARM64, I compared complete compiler instruction
streams from the old main commit and this candidate for all seven sequential
accounting functions and the three ordinary arithmetic functions. The
Linux AMD64, Linux ARM64 and WASM streams are byte-for-byte identical. Captured
streams and the comparison receipt are in
`/private/tmp/ht323-arithmetic-calendar-2027/streams/`; the updated manifest
SHA-256 is `e8a746b0110d76038cd7d8baa7854696df5c57ff89a44dcba1d6237043936785`.
The dependency closure digests are `e73458bf85303e63f67f4abd4795b9eb7bdb91241cea92c05e8ce100431cc504`
for both Linux targets and
`c18cd15a3bbdcc6a06d24db8ee6ca739d0221edcfdb2ba3ddac05de6ec9a095d` for WASM.

This is cross-compilation evidence from a Darwin host, not Linux-native
execution. The strict proof still needs to pass on the exact candidate in
Linux CI. The inventory renewal admits the reviewed 2027 calendar source; it
does not qualify ordinary arithmetic or the scheduled-trade family.
