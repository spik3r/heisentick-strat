# Semantic fixture set A (draft)

Small, hand-derived cases for the Strat language's execution rules. Each
case is the smallest strategy and bar series that exercises one rule, with
the expected trades worked out by a person from the specification
(`strat/docs/dsl-spec.md`, `strat/docs/dsl-spec-families/*.md`,
`strat/README.md`). This is layer A of the correctness plan in
`backlog/plans/2026-09-18-one-engine-migration.md` ("Correctness"): Go is
checked against these fixtures; JS and WASM are then checked against Go.

## The rule

**Expectations are hand-derived. Regenerating `expected.json` from any
engine (JS, Go, WASM) is forbidden.** An engine-produced expectation would
only prove that the engine agrees with itself. When an engine disagrees with
a case, the outcome is one of: a bug in the engine, a mistake in the
derivation (fix the derivation and say why in `RATIONALE.md`), or a gap in
the spec (mark the case `spec-gap`, list it below, and get the spec changed
first). `scripts/checks/semanticFixturesCheck.mjs --compare=js` prints the JS
engine's result per case as triage information; a mismatch never fails the
check.

## Layout

```
strat/conformance/semantic/
├── README.md                       this file
└── <case-id>/
    ├── strategy.strat              smallest strategy exercising the rule
    ├── fixture.json                dsl-conformance-run-fixture-v1: symbol, timeframe,
    │                               costs (fillOn, slippage 0), hand-written bars
    │                               [t,o,h,l,c,v] on exact bar boundaries, htfBars when
    │                               needed, optional "extends": "<case-id>" for a
    │                               prefix-extension pair
    ├── expected.json               dsl-conformance-semantic-expected-v1: trades + notes
    └── RATIONALE.md                bar-by-bar derivation citing the spec sections
```

`expected.json` trades list only the fields that matter to the rule and can
be derived by hand: `side`, `entryIndex`, `entryT`, `entry`, `exitIndex`,
`exitT`, `exit`, `reason`, and `initialSl` / `initialTp` when a stop and
target exist. `pnl`, `size` and `points` are omitted unless a case is about
sizing or costs. A no-trade case has `trades: []` and says why in `notes`.

Exit-reason vocabulary: `tp`, `sl`, `eod` (an end-of-day flatten), `time`
(`maxHoldCandles`), `end-of-test` (position still open on the last bar) and
`rule` (a family close rule such as the SMA bearish cross). A `rule` trade also
carries its specific identity in the optional `rule` field, such as
`sma-bearish-cross`; other reason categories omit that field. `end-of-test` is
never spelled `eod`, `tp` or `sl`.

Conventions: XAUUSD 1h, T0 = 2026-01-05T00:00Z, bar `k` opens at T0 + k h,
round prices near 2000, every bar has a 10-point true range and opens at the
prior close unless a gap is the point of the case (so ATR = 10 under any
definition), slippage 0, `fillOn: close` unless the case is about fills.

Run:

```bash
pnpm run dsl:semantic:check                    # shape validation (in `ci`)
pnpm run dsl:semantic:check -- --compare=js    # + JS engine result per case
pnpm run dsl:semantic:check -- --json-only=1
node --test test/semanticFixtures.test.mjs
```

## Cases

Status: `derived` = every expected value rests on a spec statement;
`spec-gap` = at least one expected value rests on a conservative reading
where the spec is silent (listed under "Spec gaps found"). `js-result` is
the JS engine's output on 2026-09-18 with a one-line classification guess;
it is information, not a verdict.

| case | semantic under test | spec section cited | status | js-result |
| --- | --- | --- | --- | --- |
| `sma-cross-next-open-entry-exit` | Order timing: signal at bar close, entry and rule exit at the next bar open | `dsl-spec-families/smaGoldenCross.md` opening paragraph | derived | match |
| `sma-warm-up-no-signal` | Warm-up: SMA undefined before N closes → no signal; partial windows forbidden | `smaGoldenCross.md`; `dsl-spec.md` §10 causality | derived | match |
| `sma-end-of-test-open-position` | Position open at the last bar reported with reason `end-of-test`, never tp/sl/rule | `smaGoldenCross.md`; plan "Correctness" layer A | spec-gap | mismatch: JS reason `eod` for end of data (`spec-gap`: reason vocabulary undefined; JS conflates end-of-data with `eod`) |
| `sma-prefix-extension-a` | Append invariance, base series | `dsl-spec.md` §10 causality | derived | match |
| `sma-prefix-extension-b` | Append invariance: strict prefix extension of `-a`; trade 1 identical, new trade 2 open at end | `dsl-spec.md` §10 causality; `smaGoldenCross.md` | spec-gap | mismatch: trade 2 reason `eod` vs `end-of-test` (same naming gap; trade 1 identical to `-a`) |
| `pm-signal-close-entry` | `enter at market` fills at the signal close with `fillOn: close`; extreme stop, 1R target | `dsl-spec.md` §6 entry, §7 risk/target; `priceMomentum.md` Purpose + shared phrases | derived | match |
| `pm-fill-on-open-entry` | `fillOn: open` defers the market fill to the next bar open | `dsl-spec.md` §6–7; `conformance/README.md` run fixtures | derived | mismatch: JS entryIndex 31 (consumer adoption pending) |
| `pm-same-bar-stop-and-target` | Same bar touches stop and target: deterministic rule, conservative = stop wins | `dsl-spec.md` §7; `priceMomentum.md` shared phrases | spec-gap | match |
| `pm-gap-through-stop` | Gap through the stop fills at the worse open, not the level | `dsl-spec-families/dailyFlushFailure.md` Phrases | spec-gap | mismatch: JS exit 1992.5 (`js-bug?`: fills at the stop level on a bar that opened at 1980, against the only written gap rule) |
| `pm-gap-through-target` | Gap through the target credited at the level, never better | `dsl-spec.md` §7; `dailyFlushFailure.md` (stop rule only) | spec-gap | match |
| `pm-tick-rounding` | Stop/target between ticks: unrounded values until the spec defines tick size and rounding | `dsl-spec.md` §2, §7; plan "Correctness" determinism | spec-gap | match (JS does not round either) |
| `pm-missing-candle` | Missing candle inside a session: `lookback N candles` counts bars, no synthetic fill | `priceMomentum.md` Purpose ("exactly N bars earlier"); `dsl-spec.md` §2 | derived | match |
| `pm-session-no-entry-outside-window` | Signal outside every enabled session is rejected; next in-window signal trades | `dsl-spec.md` §4 sessions, §10 gate order | spec-gap | match |
| `pm-position-crosses-session-end` | Session end does not close an open position; target hit after the window | `dsl-spec.md` §4, §7 (`maxHoldCandles` is the only time exit), §10 | spec-gap | match |
| `pm-htf-unclosed-bar-no-lookahead` | HTF guard reads only completed 4h candles; signal inside a forming candle judged by the last closed one | `dsl-spec.md` §10 causality, §6 HTF gate; `priceMomentum.md` HTF guard | derived | mismatch: JS 0 trades pending the D19 consumer adoption; Go uses the specified last-completed-candle close/open direction |

Semantics required by the ticket and where they are covered: order timing
(`sma-cross-next-open-entry-exit`, `pm-signal-close-entry`,
`pm-fill-on-open-entry`); same-bar stop and target
(`pm-same-bar-stop-and-target`); gap through a level (`pm-gap-through-stop`,
`pm-gap-through-target`); tick rounding (`pm-tick-rounding`); missing candle
(`pm-missing-candle`); HTF availability (`pm-htf-unclosed-bar-no-lookahead`);
session boundary (`pm-session-no-entry-outside-window`,
`pm-position-crosses-session-end`); warm-up (`sma-warm-up-no-signal`);
end-of-test (`sma-end-of-test-open-position`, `sma-prefix-extension-b`);
append invariance (`sma-prefix-extension-a` / `-b`).

## Spec gaps found

Each item is a decision the spec must make before the case can move to
`derived`. The fixture takes the most conservative reading (no trade, no
look-ahead, worst fill) in the meantime.

1. **Same-bar stop and target.** No ambiguity rule. `pm-same-bar-stop-and-target`.
2. **Gap fills.** The "worse open" rule exists only in
   `dailyFlushFailure.md`; nothing for shared stops or for targets.
   `pm-gap-through-stop`, `pm-gap-through-target`.
3. **Tick size and rounding.** No per-instrument tick, no rounding rule
   (the plan asks for a banker's-rounding-free rule). `pm-tick-rounding`.
4. **Session hours.** `sessions(...)` names four sessions but their hours and
   the UTC+10 anchor are only in engine code. Also unstated: sessions gate
   entries only. `pm-session-no-entry-outside-window`,
   `pm-position-crosses-session-end`.
5. **End-of-test liquidation.** No reason name and no liquidation price.
   `sma-end-of-test-open-position`, `sma-prefix-extension-b`.
6. **Exit-reason vocabulary.** No canonical set; this README defines one for
   the fixtures.
7. **ATR definition for `by X ATR`** (length, smoothing) and whether "last N
   candles" includes the signal candle. Every `pm-*` case neutralises both
   by construction (constant true range; shared 3-candle low).
8. **Default gates on families.** `dsl-spec.md` §10 applies day-type and
    movement-efficiency gates to every signal, while `smaGoldenCross.md`
    says the family has "no … session … semantics". Day-type and movement
    efficiency are themselves undefined. The `pm-*` cases neutralise them
    with `day type in (trending, ranging, choppy)` and `movement below 1.1`;
    the `sma-*` cases rely on the family statement.

## Not derived by hand

Nothing in `expected.json` came from an engine. Two inputs were taken from
engine code because the spec does not carry them, and are flagged in the
RATIONALE files: the session window table (gap 5) and the day-type value
set, which `docs/strategy-dsl-context.md` lists as exactly `ranging`,
`choppy`, `trending`.
