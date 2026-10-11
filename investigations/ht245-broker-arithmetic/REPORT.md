# HT-245 investigation: broker arithmetic across paths and strategies

Status: investigation only. No engine, golden, fingerprint, release or pin file is
changed. Task: [HT-245](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-245-broker-arm64-fused-arithmetic.md).
Claim: backlog PR 482. Written 2026-10-11.

## Answer

1. **Fusion changes more than reported P&L.** On ARM64 it also changes trade
   **size**, **stop/target levels** and **exit prices** by a few ulps. Across the
   scaled sets, 20 distinct frozen cases (spanning dual EMA, flag, supply/demand,
   double top/bottom, Elder, fib, Keltner, price momentum, SMA cross, trend
   pullback and others) showed such differences (the union of the two scaled sets
   is 21 cases once channel break hold is added). In the 4,234 ARM64-vs-amd64 runs
   without bar edits (109 frozen, 231 with an injected fee, 654 + 3,240 scaled)
   no run differed in trade count, side, entry bar, exit bar or exit reason.
2. **A decision (exit bar) did change, and fusion alone causes it.** In
   `channel break hold`, a one-bar edit moves the exit from bar 962 (amd64, both
   WASM) to bar 963 (ARM64) in 3 of 6 constructed cases. This flip appears with
   FMA on and the ARM64 constants removed (scratch copy), and disappears when FMA
   is also off. It also appears with the constants kept and FMA off. Production
   ARM64 hides most of this because two hard-coded ARM64-only constants
   (`engine/family_channel_break_hold.go:147-158`: rail `+9.4e-12`, width
   `-1.565e-12`) cancel the fusion error at the frozen price scale only. Neither
   the constants nor this behaviour is described in the spec, task or docs.
3. **Sizing can flip by expression, not yet by engine run.** `engine/master` and
   `engine/regime` default (non-portable) paths feed fused equity into
   `floor(.../0.1)*0.1` sizing. On invented inputs the next quantity is 0.4 on
   ARM64 and 0.5 elsewhere (`results/micro`). I did not build an engine-level
   Master run that hits the boundary.
4. **Frozen results are blind to this.** All 109 frozen run fixtures (1,035
   trades) are bit-identical on native ARM64 (Go 1.27.0 and 1.23.8), amd64
   (Rosetta) and both WASM targets. Only 1 of 109 has a non-zero `feePerUnit`.
5. **Recommendation: a correction is justified, but as three separate pieces,
   and none should start before the owner decides which arithmetic is canonical.**
   See [Recommendation](#recommendation).

## Facts, assumptions, opinions

Facts are measured here (receipts, assembly, hashes). Assumptions are marked
*Assumption*. Opinions are in Recommendation only.

## Already proven, not repeated

From HT-228/HT-229 (strat PRs 112-116, `docs/sequential-release-qualification.md`,
`docs/sequential-metrics-contract.md`, `engine/legacy_setup9_oracle_test.go`): ARM64
fuses the entry debit and exit credit in `broker.go`; Legacy Setup-9 P&L stays within
an operand-ULP bound; an exact per-target replay explains every differing leaf of
the Sequential accounting corpus. This report adds what that work did not cover:
other broker paths, other families, indicator/context code, and decision impact.

## Identities

`results/identity.json` records: strat base `4ab0fde8dfcfd3f2b33e4ea0af6186474c2b6ffa`
(= origin/main when run; tree of `engine` `a55b0162…`, `conformance/run` `7003c33d…`,
`broker.go` blob `9869eddc…`), host `Darwin arm64`, SHA-256 of every manifest and
harness binary. Compilers: Go 1.27.0 (host) and Go 1.23.8 (module cache, offline).
CI for strat uses `1.22.x`; **Go 1.22 was not available offline and is not-run.**

## Platform matrix

| Target | Result | Notes |
|---|---|---|
| darwin/arm64, Go 1.27.0 | ran | native |
| darwin/arm64, Go 1.23.8 | ran | identical to 1.27.0 on every case |
| darwin/amd64 GOAMD64=v1, Go 1.27.0 | ran | under Rosetta 2 (translation, not a native Intel CPU) |
| darwin/amd64 GOAMD64=v3 | **not-run** | Rosetta refuses v3 binaries. Cross-compiled assembly only |
| js/wasm (Node 26 `go_js_wasm_exec`), Go 1.27.0 | ran | the shipped WASM target |
| wasip1/wasm (wasmtime 48), Go 1.27.0 | ran | agrees with js/wasm on every case |
| js/wasm, Go 1.23.8 | **not-run** | toolchain in the module cache has no `misc/wasm` glue |
| linux/arm64 and linux/amd64 | **not-run** | Docker daemon was not running; assembly cross-compiled only (`linux/arm64`, Go 1.27.0) |
| ARM64, FMA contraction off (`-gcflags=all=-d=fmahash=n`) | ran, **diagnostic only** | compiler debug flag; objdump confirms no FMA in repo code. Never a build setting |

Compilation is not runtime equivalence: assembly files record what was generated;
only receipts record what executed.

## Instruction evidence (compilation)

`results/asm/*.txt`, from `go build -gcflags=-S ./engine`. On ARM64 (Go 1.27.0, 1.23.8,
linux/arm64 identical):

| Source line | Emitted |
|---|---|
| `broker.go:536` `px += sign*slippageAt` | FMADDD |
| `broker.go:539` `SL = px - sign*StopDistance` | FMSUBD |
| `broker.go:542` `TP = px + sign*TargetDistance` | FMADDD |
| `broker.go:577` `realized -= fee*size` | FMSUBD |
| `broker.go:589` `px := exitPrice - sign*slippageAt` | FMSUBD |
| `broker.go:591` `pnl := points*size - fee*size` | FMULD + FMSUBD (one product rounded, one fused) |
| `flag.go:323`, `flag.go:345`, `family_trend_pullback.go:131,160`, `family_channel_break_hold.go:143`, `contextcols/channels.go:351-363` | FMSUBD/FMADDD |

amd64 v1 and WASM emit separate multiply and add/sub (MULSD+SUBSD, F64Mul+F64Sub)
for all of these lines. **amd64 v3 is not fully unfused**: it emits `VFMADD231SD` at
`broker.go:536` and `:542`, `contextcols/channels.go:351`, `:363` (9 of 15 sites)
and `family_channel_break_hold.go:143`; it keeps `broker.go:577` and `:591`
separate. Nothing in the repo sets `GOAMD64`, so v1 is what was run; v3 cannot run
under Rosetta, so a v3 build's channel rails are untested (*assumption*: they could
diverge from v1 with no constant to compensate). Lines 536/539/542/589 multiply by
`sign = ±1`, so fusion cannot change their result; the non-exact fused products are
577, 591 and the family and context-column lines.

A static inventory by a Haiku subagent (spot-checked by me against the arm64
listing) found 210 repo source lines with fused instructions on ARM64, in the
broker, `flag.go`, ~25 family files, `contextcols`, `master`, `regime`,
`range_reversion_reference.go`, `gold_flag_reference.go`. It found **no** kill switch,
loss streak, daily-loss limit, drawdown stop or P&L-based risk scaling anywhere.
Realized P&L is accumulated and read only for accounting checks and reports.
Sizing is `RiskUSD / |entry - SL|` (constant risk) except `master` and `regime`,
which size from running equity. The inventory table is in the PR description, not
checked in, because I did not verify every row.

## Measurements

All counts compare each target with amd64 v1 (reference) bit for bit; "path" means
an entry, exit, stop, target, size or points bit differs while count, side, bars
and reasons are identical; "decision" means trade count, side, index or reason
differs. `results/summaries/*.compare.txt`.

| Set | Cases | ARM64 vs amd64 | WASM (js, wasip1) vs amd64 | ARM64 no-FMA vs amd64 |
|---|---|---|---|---|
| Frozen run corpus, unchanged | 109 (1,035 trades) | 109 identical | 109 identical | 108 identical, 1 path (channel break hold) |
| Fee injected (cancellation): fee = best trade's `points`, ±1 ulp | 231 (3,051 trades) | 199 pnl-only, 32 identical | 231 identical | 231 identical |
| Frozen bars × 6 seeded factors | 654 (6,174 trades) | 629 identical, 25 path | 654 identical | 648 identical, 6 path (all channel break hold) |
| Frozen bars × 30 seeded factors (search; **channel break hold excluded**, so 108 base cases) | 3,240 (30,848 trades) | 3,153 identical, 83 path, 4 pnl-only | not run | 3,240 identical |
| One-bar boundary edits (channel break hold) | 6 | 3 path, **3 decision** | 6 identical | 3 path, **3 decision** (constants still present) |

Reading the table:

- **Fusion-only differences are real but small and did not move a decision in
  unedited runs.** Outside channel break hold, the no-FMA ARM64 build equals
  amd64 on all 3,240 + 654 scaled cases, so FMA contraction explains them; amd64
  equals both WASM runtimes everywhere.
- **Fee injection** (the cancellation case). 269 of 3,051 trades differ; largest
  gap `1.0e-11` currency. In 9 trades one target reports exactly `0` and the other a
  non-zero residual; **in 3 trades the win/loss sign (`pnl > 0`) differs**. Example
  from `micro`: `points=1.1, size=3.3, fee=1.1` gives `0` unfused and `-2.04e-16`
  fused. This affects win rate and profit-factor style metrics, not trading.
  `maxUlps=4.4e18` in the summaries is the distance between a zero and a non-zero,
  not a ulp count.
- **Sizing and levels.** Of the 87 differing cases in the 3,240-case search set,
  `size` differs in 43, `points` in 58, `pnl` in 73, a final stop in 5 and a
  final target in 2 (counts are cases with at least one such trade). In the
  654-case set, `size` differs in 17 of 25 (e.g. flag continuation 27.002894123384
  vs 27.0028941233857). Channel break hold accounts for all 6 exit and most
  stop differences in that set and has its own cause (next section).
- **Boundary flips outside channel break hold: none constructed.** The search set
  excludes channel break hold and gave no case where the exit-deciding stop or
  target differed and never moved after entry (the generator's condition), so
  no one-bar edit could be built there. Two earlier attempts on trailing
  dual-EMA stops did not flip: the edited bar's close also moves the trail
  (no artifact kept). This is a limit of the search, not evidence none exists.

### Channel break hold constants (finding)

`normalizedChannelRail` adds `9.4e-12` and `normalizedChannelWidthATR` subtracts
`1.565e-12` when `runtime.GOARCH == "arm64"`, present since the extraction commit
`eea01f8`. Evidence (`ablate-arch-constants.sh`, `results/ablate-arch-constants.txt`,
scratch copy; repo tree untouched):

| Variant (boundary-patches.json) | vs amd64 |
|---|---|
| ARM64, FMA on, constants on (production) | 3 path, 3 decision |
| ARM64, FMA on, constants removed | 3 path, 3 decision |
| ARM64, FMA off, constants removed | 6 identical |
| ARM64, FMA off, constants on | 3 path, 3 decision |

With FMA on and the constants removed, `TestRunConformance` **fails** on
`family-channel-break-hold` (`initialSl` `…318` vs golden `…319`). So the golden
encodes unfused (amd64/WASM) arithmetic, and the constants force fused ARM64 to
match it. The frozen-corpus ARM64 stop is about 44 ulps from amd64 without them,
close to `9.4e-12` at ~2033. The compensation holds only at the frozen price
scale: on scaled data ARM64 channel `sl`/`exit` differ from amd64 by up to 154
ulps and `size` by up to 156,598 ulps (`maxUlps` 75,084 in the boundary table is
P&L ulps on the edited set). Debug-build evidence (not checked in): no-FMA ARM64
and amd64 agreed on all 5,050 `fitLine` calls, and `rail` differed only through the
constant. Opinion: this is the largest numerical risk found, because it is
deliberate, undocumented and branches on architecture.

### Expression-level sizing flip (Master)

`results/micro/darwin-arm64.txt` line 1: `equity += qty*(gross - fee)` yields
`…b203` fused and `…b204` separately rounded; the next
`floor((equity*.1/entry + 1e-12)/.1)*.1` is 0.4 versus 0.5 (25% size). amd64 and
WASM both give 0.5. If the quantity fell below one step, `master/run.go:117-118`
returns `QuantityBelowStepError`, so a fused equity could turn an entry into a run
error. The inputs were found by search and are invented; the engine
path is `engine/master/execute.go:48` and `run.go:107-112`. The repo already
provides `RunPortableV1` for Master (`docs/master-structural-portable-arithmetic.md`);
the legacy default is what fuses. The legacy path is live: `report/masterstructural/report.go:82` calls
`master.Run` when not bounded, and `cmd/heisentick/regime_report.go:80` calls
`regime.Run`, which has no portable variant.

## Do differences affect thresholds and decisions?

| Quantity | Affected by fusion? | Evidence |
|---|---|---|
| Reported P&L, win/loss label | yes (ulps; zero vs residual) | cancellation set, micro |
| Position size (generic broker) | yes, via fused stop level (ulps) | scaled sets |
| Stop/target level | yes (ulps) | scaled sets: final stop in 5 search cases, final target in 2 |
| Trade count, side, entry/exit bar, reason | yes in `channel break hold` after a one-bar edit; not seen in unedited runs | 0 of 4,234 runs without bar edits; 3 of 6 edited channel cases |
| Subsequent decisions via realized P&L | no path found | no engine code branches on realized/equity except `master`/`regime` entry guards and sizing |
| Master equity-sized quantity | yes in principle | micro; engine-level not built |

## Reproduce

```sh
HT245_WORK=/tmp/ht245 investigations/ht245-broker-arithmetic/run-all.sh
```

Requires Go (host 1.27), optional Go 1.23.8 in the module cache, Node ≥ 22,
wasmtime, Rosetta. Caches are under `$HT245_WORK`; nothing global is cleared. The
two scale manifests were generated once with Python `random.Random(245)` and
`random.Random(2451)`; the checked-in JSON is authoritative. Raw receipts (tens
of MB) are not checked in; the review reproduced the headline counts independently; `results/identity.json` pins inputs and binaries, and a
rerun reproduced the same counts.

Files: `harness/` (public-API runner, bit-exact receipts), `compare.mjs`,
`gen-cancellation-manifest.mjs`, `gen-boundary-manifest.mjs`, `run-matrix.sh`,
`asm-evidence.sh`, `identity.sh`, `micro/` (expression reproducer, not the engine),
`probe/` (context-column hash dump), `manifests/`, `results/`.

## Limits

- Rosetta is a translator, not Intel hardware; WASM ran under Node/V8 and wasmtime.
- Go 1.22 (CI) and Linux targets were not run. The 109-case corpus uses invented
  data; scaled variants are invented too.
- `-d=fmahash=n` is an undocumented compiler debug flag; I verified its effect only
  by disassembly and by equal results, not by its documentation.
- Dedicated routes (`master`, `regime`, gold flag, adaptive flag, range reversion)
  were not executed at engine level here; they are inventory-only.
- Fusion-only decision flips outside channel break hold may exist at inputs this search did not reach.
- `run-all.sh` regenerates `manifests/cancellation-variants.json`, `boundary-patches.json` and `boundary-search.json` from receipts; the scale-manifest generators (Python) are not checked in.
- The boundary edit replaces the whole exit bar (open, high, low, close), not only one extreme; levels are filtered to ones that never move after entry, and every target receives the same edited input.

## Recommendation

Opinion. Leave `broker.go` unchanged for now and open three separate items:

1. **Channel break hold (highest priority).** Replace the GOARCH branches with
   explicit `float64()` splits in `fitLine`, `linePrice` and `engineLinePrice`
   so every target produces the unfused bits without a constant. The golden
   already encodes unfused arithmetic, so this should keep it; that is an
   inference from the global no-FMA run, not a test of local splits. Also check
   a GOAMD64=v3 build, which fuses these sites too. Needs a conformance impact
   statement and independent review.
2. **Documentation (no code).** State in `legacySetup9.md`/specs that fusion also
   changes sizes and stop levels, not only P&L, and that the Master/Regime legacy
   paths fuse equity. Fix the comment noted in the task's addendum.
3. **Broker `float64()` splits** at `broker.go:577, 591` and `flag.go:323` only if the
   owner wants cross-target bit equality. Evidence for doing it: 269/3,051 trades
   differ with a fee; 3 flip win/loss. Evidence against: the frozen corpus has no
   fee, nothing in the engine reads P&L for a decision, the difference is ≤1e-11
   currency, and the change needs an adaptive-flag fingerprint renewal with
   independent review and may alter ARM64 output for every family.

I recommend (1) and (2) now and deferring (3) until a product need for bit-equal
P&L across targets exists. Any correction needs its own reviewed PR; goldens are
not regenerated by this investigation.

## Validation record

See the PR description for pass/fail/skip/not-run of `gofmt -l .`, `go vet ./...`,
`go test ./... -count=1`, `go run ./cmd/conformance check`, `git diff --check`.
