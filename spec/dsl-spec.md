# Strat Language Specification (v7)

Status: normative for the core document model and the shared (non-setup)
directive vocabulary. Setup-family phrase sets are indexed in §9 and specified
in follow-up sections as they are written. Where this document and the JS
implementation (`strat/implementations/browser-runtime/compiler/`) disagree, one of them has a bug — file it
rather than silently following the code. The machine-checked companion to this
document is `strat/conformance/` (see its README): any conforming implementation
must reproduce those parse and trade goldens byte-for-byte.

Scope note: Strat source files conventionally use the `.strat` extension. The
in-file version header remains `dsl v7` because it names the language version,
not the file format. `dsl v6` and `dsl v7` are currently parsed identically;
v7 is the specified language. v6-only spellings that survive for compatibility
are marked *deprecated* and emit warnings.

## 1. Document model

A strategy is a UTF-8 text document processed line by line. There is no
free-form expression grammar; every non-structural line is a **directive
phrase** — a head word followed by tokens in a fixed, per-directive shape.

Processing pipeline (normative order):

1. **Comments** — `#` starts a comment; everything from `#` to end of line is
   removed before any other interpretation.
2. **Inline sections** — a physical line of the form `<section> { <body> }`
   (or `strategy "<name>" { <body> }`) is expanded: the body is split into
   one synthetic line per directive using the section's known directive-start
   words, at parenthesis depth 0 only. Known setup-family `type` values are
   treated as complete values before later setup directives are split, so
   `setup { type: channel break hold hold 2 candles }` expands to the `type`
   directive followed by the `hold` directive. Diagnostics for synthetic lines
   report the original physical line.
3. **`when`-clause joining** — a line starting `when ` that has unbalanced
   parentheses or no `then` yet is joined with following lines until its
   parentheses balance and a `then` appears. Diagnostics report the first
   line of the clause.
4. **Line normalization** — each logical line is then normalized:
   - `name(args)` becomes `name args` (one level; commas inside become
     spaces),
   - `->` becomes ` enter `,
   - `key:` becomes `key ` (a trailing colon after a word is dropped),
   - all of `( ) , { }` become spaces; whitespace collapses to single
     spaces.
   Consequently `sessions(asia, london)`, `sessions: asia london` and
   `sessions asia london` are the same directive.
5. **Tokenization** — the normalized line splits on whitespace. The first
   token is the **head**; dispatch is by head (and sometimes the following
   token), the current section, and the current setup type.

### 1.1 Sections

A section opens with `<name> {` (or `<name>:` on its own line) and closes
with `}`. Section names (case-insensitive, canonical form in parentheses):

| Written | Canonical |
| --- | --- |
| `strategy` | strategy |
| `market conditions`, `market`, `conditions` | market |
| `time`, `context` | (deprecated accepted openers; no own vocabulary; directives inside still use the shared flat namespace) |
| `setup` | setup |
| `levels` | levels |
| `filters` | filters |
| `triggers`, `triggerCandles` | trigger |
| `entry` | entry |
| `risk` | risk |
| `target` | target |
| `management` | management |
| `execution` | execution |
| `grade` | grade |
| `volume profile` | volume profile |

Sections are mostly *documentary*: with the exceptions below, a directive is
recognized anywhere and acts on the same configuration regardless of the
section it appears in. The `time` and `context` section openers are accepted
only for compatibility and emit warnings because they do not own any
directives; prefer the existing sections below. Section placement matters for:
inline-body splitting (step 2), the `grade` vocabulary (§8), setup-phrase
enablement (§9, which also requires the setup `type` to be declared first),
`movement` (market scope), `range edge` (levels scope), and
`minR`/`fallbackR` bare keys (target scope).

### 1.2 Version header

`dsl v6` | `dsl 6` | `dsl v7` | `dsl 7` — optional, conventionally the first
line. Any other version is a compile error ("unsupported DSL version");
implementations must refuse unknown versions rather than guess.

### 1.3 Diagnostics contract

Compilation returns the configuration plus:

- `errors: string[]`, `warnings: string[]` — human-readable, prefixed
  `line N:` where N is the **original source line** (1-based);
- `diagnostics: { severity: 'error'|'warning', message, line, col,
  suggestion? }[]` — the structured mirror; `line: null` for
  whole-spec checks (e.g. unknown level in `priority`). `col` is a
  best-effort 1-based column of the directive head in the source line.
  Unknown heads carry a `suggestion` when an edit-distance-≤2 match exists
  among the known directive heads.

An unknown head is the error `unknown directive "<head>"`; a known head with
a malformed tail is a per-directive error message stating the accepted
shape(s). Compilation continues after errors (all diagnostics are collected
in one pass).

Deprecated spellings marked below remain accepted with compatibility warnings
under `dsl v6`. Under `dsl v7`, those same spellings are compile errors with a
did-you-mean replacement.

## 2. Common token shapes

- **number** — JavaScript-style decimal; a trailing `R` is stripped
  (`1.5R` reads as 1.5). A trailing `%` is stripped where a percentage is
  expected. In `dsl v7`, a token in a numeric position that fails to parse is
  a compile error. In `dsl v6`, malformed numbers keep the historical
  fallback-to-default behavior but emit a compatibility warning.
- **list** — values separated by commas and/or spaces (after normalization
  these are identical).
- **`in (…)` lists** — `X in (a, b)` gates on membership; `X not in (a, b)`
  blocks membership (`not` must precede `in`).
- **timeframes** — `1m 5m 15m 30m 1h 4h 1d` (lowercased).
- **symbols** — uppercased, `/^[A-Z][A-Z0-9]{2,15}$/` (in `slices`).
- **sessions** — exactly `asia`, `mid`, `london`, `ny`; anything else is an
  error with a did-you-mean suggestion.
- **weekdays** — first three letters, case-insensitive (`mon`, `Tue`, …).
- **hours** — integers 0–23; `18:00` is accepted for `18`.
- **dates** — `YYYY-MM-DD` (UTC midnight) or anything `Date.parse` accepts.
- **side** — `long`/`buy` → long, `short`/`sell` → short.
- **candle-count units** — author-facing count phrases use `candle` or
  `candles`; numeric `bar`/`bars` unit aliases remain accepted but are
  *deprecated* and emit warnings.

## 3. `strategy` section

```
strategy "Name" { description "..." }
```

- `strategy "Name"` / `name "Name"` — display name.
- `description "..."` — free text.

Quotes (single or double) are stripped; both directives ignore empty values.

## 4. `market conditions` — where and when the strategy trades

Routing:

- `slices(SYMBOL tf[, SYMBOL tf …])` — symbol/timeframe pairs; must be pairs,
  symbols and timeframes validated as in §2; duplicates dropped. Preferred
  over `symbols`/`timeframes` when both are present.
- `symbols(...)`, `timeframes(...)` — independent lists (defaults:
  no symbol restriction; `5m, 15m`).

Time gates:

- `sessions(asia, london, ny)` — enable trading sessions (default
  asia+london+ny; `mid` exists and defaults off). The compiled session object
  always carries all four keys: `asia`, `mid`, `london`, and `ny`. When a setup
  family has its own session default, that default applies only until the
  author writes an explicit session directive; explicit `sessions(...)` or
  deprecated `windows(...)` values win regardless of whether they appear before
  or after `type:`. `windows(...)` is the *deprecated* spelling.
- `trade window minutes A to B` — minutes since session-window open,
  increasing range required.
- `trade window unrestricted` — research-only override that removes the
  engine's fixed UTC+10 trade-window gate. Other filters, including any
  explicit `local hour` filter, still apply; use it only when the study maps a
  session in another timezone explicitly.
- `trade window in (london.open, ny.open, …)` — window segments
  `<window>.<part>` with window ∈ {asia, london, ny} and part ∈
  {open, middle, close, all} (aliases: first/second/third/full;
  `london_open`/`london-open` normalize to `london.open`).
- `local weekday in (Mon, Tue)` / `local weekday not in (Fri)` — allow/block
  by local weekday (`weekday …` also accepted).
- `local hour in (…)` / `not in (…)` — allow/block local hours 0–23.
- `new york hour in (…)` / `not in (…)` — allow/block America/New_York wall-clock
  hours 0–23 using the IANA timezone database, including EST/EDT transitions.
- `session phase in (…)` / `not in (…)` — named local session phases.

Day-shape gates:

- `day type in (trending, ranging, …)` — allowed day types; optional escape
  `… or movement below X` (a non-matching day still trades when movement
  efficiency ≤ X; default escape 0.55) or `… strictly` (no escape).
  `regime …` is the *deprecated* head for the same gate.
- `prior day type in (…)` / `not in (…)` — allow/block by the prior day's
  type.
- `day theme in (buy_lows, sell_highs, join_momentum, stand_aside)` —
  day-plan theme gate; enforced direction-aware at entry.
- `open location in (nearPDH, nearPDL, nearDO, …)` — where price opened
  relative to key levels (aliases like `near PDH`, `near day open`
  normalize).
- `movement below X` / `maxMovementEr X` / `trendiness below X` — cap on
  movement efficiency ratio (default 0.8).

Range-stat and bias gates (repeatable; each adds one filter):

- `room session at least X ATR` (scope ∈ session/window/day/week).
- `<scope> range used below N%` / `above N%`.
- `<scope> range exhausted up|down [at least N%]` (default 70).
- `session bias up|down|mixed|pause` (scope ∈ session/window).
- `seasonality <dimension> [lookback] in (<classes>) [min samples N]` or
  `seasonality <dimension> [lookback] supports entry` — dimension ∈
  intraday/dayOfWeek/weekOfYear/monthly (aliases: hourly, dow, weekly,
  month…); lookback `all` or `<N>d`; classes from insufficient_data,
  extreme_bullish, bullish, neutral, bearish, extreme_bearish; default
  min samples 5. Causal: computed from prior data only.

Event-data gates (repeatable; each adds the `microstructure` context
requirement):

- `micro spread ticks|bps above|below|at least|at most N`.
- `micro liquidity imbalance above|below|at least|at most N`.
- `micro weighted mid displacement above|below|at least|at most N`.
- `micro quote flow imbalance above|below|at least|at most N over <duration>`.
- `micro quote rate above|below|at least|at most N over <duration>`.
- `micro realised volatility above|below|at least|at most N over <duration>`.

Durations use `ms`, `s`, or `m` (for example `250ms`, `5s`, or `1 minute`).
Compilation records the required capability and window. Before execution the
selected event dataset should satisfy the `microstructure` context requirement;
at runtime a missing or unsupported value records an explicit data skip and
never behaves as zero. `liquidity imbalance` is broker/source liquidity unless
the event manifest separately proves queue semantics. All values are computed
from the current and prior event snapshots only.

Context columns:

- `ema length N` — enables the EMA context column (also makes `EMA` usable
  in `priority(...)`; declaring `priority(EMA)` without a length defaults it
  to 21).

## 5. `levels` — the level universe

- `priority(PDH, PDL, …)` — ordered level priority. Known names: PDH PDL
  PDO PDC DO DH DL WH WL AH AL LH LL NH NL CAM_R3 CAM_R4 CAM_S3 CAM_S4
  channel.high channel.low VWAP EMA POC VAH VAL, `RN<step>` (round numbers),
  plus any custom level keys defined below. Unknown names are a whole-spec
  error. Default when unspecified: PDH PDL WH WL.
- `near <levels…> [within X]` / `nearKeyLevel …` / `withinAtr X` /
  `range edge within X` — level-proximity distance in ATR (default 1.5).
  A leading `any` is skipped; naming levels here also sets the priority
  list.
- `support <price> [to <price>]` / `resistance <price> [to <price>]` —
  custom horizontal level or zone; auto-keyed S1, S2… / R1, R2… and
  auto-appended to the priority list.
- `trendline from <date> <price> to <date> <price>` — custom trendline
  (keys TL1…); end date must be after start.
- `fib from <date> <price> to <date> <price> at <ratio> [<ratio>…]` — fib
  levels from a leg (keys FE1…).
- `level must align with another level within X ATR` — confluence
  requirement (X must be positive).
- `retrace …` — see fib-continuation setup (§9).

## 6. `filters`, `trigger`, `entry`

Structure filters:

- `range method pivot|zone` — how ranges are detected (strategy metadata may
  set a preferred method; reports can force either).
- `range active within N candles` — max age of the last range
  (default 8). *Deprecated spellings:* `activeWithinCandles N`,
  `maxAgeBars N`.
- `channel active within N candles`, `channel width between X and Y ATR` |
  `at least X` | `below X`, `channel direction in (ascending, descending,
  flat)` (`directional` = ascending+descending), `channel lookback N`,
  `channel span at least N`, `channel source close|wick`,
  `channel min width X ATR` — channel gate family; any channel directive
  enables channel detection.
- `higher timeframe must agree` | `higher timeframe must not oppose entry` |
  `higher timeframe off` — HTF gate (mode notAgainst/off; default off).
  Accepted shorthands are `<timeframe> must agree`, `higher timeframe
  <timeframe> must agree`, `htf must agree`, and `mtf must not oppose entry`
  (`higher time frame` with a space is accepted). Supported timeframe tokens
  include `4h`, `daily`, and `weekly`; otherwise the HTF is chosen
  automatically per chart timeframe.
- `source timeframe <tf>` — v7-only, explicit source declaration for setup or
  filter evaluation. It is accepted only in `setup {}` or `filters {}`, where
  `<tf>` is exactly one of `1m 5m 15m 30m 1h 4h 1d`. Omitting it leaves the
  field absent; it does not select a default. Source data acquisition, context
  construction, and cache identity are source-aware. The only lower-timeframe
  execution pair presently admitted is `XAUUSD`, `source timeframe 4h`, and
  `entryTf 15m`; its source setup becomes eligible on the first actual 15m
  decision close strictly after the completed 4h close. All other non-current
  `entryTf` values are rejected.
- `side long only` | `side short only` | `side both` (`direction …` same).
- `approach at least N candles` / `approach distance at least X ATR` —
  sustained approach toward the level before a signal (defaults: 3
  candles, no distance minimum). *Deprecated:* `approachBars`.
- `pullback session VWAP wick|close touch`, `pullback first session VWAP touch
  only`, `reclaim session VWAP on trend side` — the `trend pullback` family's
  opt-in causal session-VWAP location gate; all three phrases are required
  together. See its family specification for touch and reclaim semantics.
- `tail rejection at least X` — minimum rejection-tail fraction.
- `close location at least X` — minimum close position within the bar.
- `entry distance max X ATR` — reject entries further than X ATR from the
  level.

Trigger:

- `trigger candle in (pin, engulf, outside)` / `rejection candle …` /
  `candle in (…)` — accepted signal candles (default pin+engulf+outside;
  `any` disables the check).
- `setup expires after N candles` — max setup age.

Entry:

- `when price sweeps range.high|range.low|channel.high|channel.low by X
  within N then signal long|short [grade …]` — sweep-and-reclaim rule per
  edge (`by` = sweep distance ATR, default 0.1; `within` = reclaim
  candles, default 3; side defaults short at highs, long at lows).
  `sweep high|low …` with key/value tokens (`sweepDistanceAtr`,
  `reclaimCandles`, `enter`) is the *deprecated* form (`sweepAtr` likewise).
- `enter at market` — enter on signal close (default).
- `enter with limit at swept edge within N candles` — rest a limit at the
  swept edge; unfilled orders expire after N candles (default 5).
  `->` reads as `enter`.

## 7. `risk`, `target`, `management`, `execution`

Risk (stop):

- `stop beyond last N candle extreme by X ATR` — stop beyond the recent
  extreme (defaults: 3 candles, 0.25 ATR padding).
- `stop beyond flag|zone|pattern|pullback edge by X ATR [min A] [max B]` —
  setup-structure stops (which one applies depends on the setup family).
- `stop beyond opposite range edge [by X] [min A] [max B]`.
- `stop <M> ATR` — fixed ATR-multiple stop.
- `stop beyond <ratio> retrace [by X]` — fib-retrace stop.
- `stop extreme N [+ X]`, `stop recentExtreme key/value…` — *deprecated*
  spellings of the extreme stop.
- `stop size min A max B` — clamp the stop distance in ATR
  (defaults min 0, max 1.5; bare `extremeCandles/paddingAtr/minAtr/maxAtr N`
  are accepted inside `risk`).

Stop fills: a bar whose range reaches the stop fills at the stop level. On a
bar after the entry bar, a bar that opens beyond the stop fills at the open
instead (`min(stop, open)` for longs, `max(stop, open)` for shorts), because
the stop level never traded. Slippage applies on top of either fill. A bar
that opens beyond the target fills at the target level, never better.

Target:

- `target <N>R` — R-multiple target (routed to the active setup family).
- `take profit [at] [the] opposite range edge` |
  `[at] [the] opposite channel edge` — edge target with `minimum reward X`
  (reject if the edge pays < X R; default 0.6) and `fallback X` (R used when
  no edge target exists; default 1).
- `target fib extension <ratio>` — measured-move target (break-retest and
  fib-continuation).
- `target N range` — opening-range multiple (ORB only).
- `target vwap` — VWAP target for VWAP-aware setup families. For
  range-break-fake, the current runtime falls back to the range midpoint when
  VWAP is unavailable and emits a compile warning; future v7 semantics may
  reject missing VWAP instead.
- `target oppositeRangeEdge minR X fallbackR Y` — *deprecated* spelling.

Management:

- `move stop to breakeven after X R [plus Y ATR]` — breakeven move
  (defaults 0.75R, +0.02 ATR). `breakeven off` disables;
  `breakeven atR X offsetAtr Y` is the *deprecated* spelling.
- `partial 50% at 1R [move stop to breakeven]` — one partial exit; size as
  percent, fraction, `half`, or `quarter`; mentioning `breakeven` in the
  phrase also moves the stop.
- `trail <N> ATR [after <R>R]` — ratcheting ATR trail once the trigger R is
  reached (default trigger 1R); `trail off` disables. Elder Triple Screen
  uses its family defaults when omitted and honors explicit shared trail
  values.
- `wait N candles after trade` — re-entry cooldown (default 3).
  *Deprecated:* `cooldownCandles`, `cooldownBars`.
- `maxHoldCandles N` — time stop (0 = off). *Deprecated:* `maxHoldBars`.

Execution:

- `risk 200 USD` / `riskUsd 200` — fixed risk per trade in account currency
  (default 200). Position size = risk / stop distance.

## 8. `grade` — signal quality scoring

Inside the `grade` section only:

- `context N`, `location N`, `trigger N`, `risk reward N` (or
  `risk_reward N`) — component weights (0 disables a component).
- `require >= N` (`above`/`minimum`/`min` also accepted) — reject signals
  whose total grade score is below N.
- `size by score <threshold> <multiplier> [<threshold> <multiplier> …]` —
  grade-based risk sizing tiers, evaluated highest threshold first; the
  matched multiplier scales the base risk.

Graded trades carry `gradeScore`, per-component metadata, and the required
total in `trade.meta` for report diagnostics.

## 9. `setup` — families (indexed here, specified separately)

`type: <family>` selects the setup family and must appear before that
family's phrases; family phrases outside their family (or before `type`)
are errors. The `type` value itself is written in trading language
(`type: flag continuation`). Families and their canonical ids:

| `type:` phrase | id |
| --- | --- |
| `failed breakout` | failedBreakout (default) |
| `flag continuation` | flagContinuation |
| `break retest` | breakRetest |
| `opening range breakout` | openingRangeBreakout |
| `session break hold` | sessionBreakHold |
| `channel break hold` | channelBreakHold |
| `inside day expansion` | insideDayExpansion |
| `day open reclaim` | dayOpenReclaim |
| `supply demand` | supplyDemand |
| `fair value gap` | fairValueGap |
| `double top bottom` | doubleTopBottom |
| `range break fake` | rangeBreakFake |
| `trend pullback` | trendPullback |
| `sma golden cross` | smaGoldenCross |
| `fib continuation` | fibContinuation |
| `triple push exhaustion` | triplePushExhaustion |
| `vwap extension fade` | vwapExtensionFade |
| `volume anomaly exhaustion` | volumeAnomalyExhaustion |
| `elder triple screen` | elderTripleScreen |
| `price momentum` | priceMomentum |
| `daily flush failure` | dailyFlushFailure |
| `keltner reversion` | keltnerReversion |
| `keltner expansion` | keltnerExpansion |
| `intra hour run exhaustion` | intraHourRunExhaustion |

Per-family phrase sets live in one standalone file per family under
[`strat/docs/dsl-spec-families/`](dsl-spec-families/breakRetest.md) (named by the
canonical id above); authoring rules and status are tracked in
`plans/COMPLETED.md` (spec-families closeout), and `test/dslSpecFamilies.test.mjs`
enforces that specified files carry compiling examples. Until a family file
leaves TODO status, its normative sources are
`strat/implementations/browser-runtime/compiler/parseSetups.js` + `engine/dsl/setups/*` and the family's
case in `strat/conformance/parse/`.

The `volume profile` section configures the VP context (scope
session/day/week/rolling/fixed — `visible` is rejected as
viewport-dependent; state, offset, layout, row size, value-area percent,
anchors, lookback) used by POC/VAH/VAL levels and VP-aware setups.

## 10. Evaluation order and causality

Signal admission happens in this normative order; the first failing gate
rejects the signal (skips are recorded for funnel diagnostics):

1. market gates (sessions, windows, weekday/hour/phase, day type + escape,
   prior day type, open location, movement efficiency, range-stat and bias
   filters, slice routing);
2. seasonality filters (side-aware);
3. candle-quality check for the signal candle;
4. day-theme gate (side-aware);
5. entry-distance cap (`entry distance max`);
6. setup-age cap (`setup expires after`);
7. grade requirement, then grade-based sizing;
8. broker admission (cooldown, one-position-at-a-time, side allowances).

Causality rules (normative):

- No strategy-visible value may depend on the current bar's future or on
  later bars: levels, ranges, channels, day types, biases, seasonality and
  VP context are computed from completed data only.
- Higher-timeframe values must come from *completed* HTF candles (no
  forward-fill of a forming candle).
- Session/window statistics ("used", "exhausted", bias) describe the
  session so far, never the finished session.

Any extension that cannot satisfy these rules belongs in chart overlays,
not in the strategy language.

The `fair value gap` family has an additional lifecycle rule: a gap is created
only after the completed three-bar formation is known, and its retest state is
advanced one completed bar at a time. The formation bar cannot also be the
retest bar. Its full causal contract, including invalidation and entry
reference semantics, is specified in
[`dsl-spec-families/fairValueGap.md`](dsl-spec-families/fairValueGap.md).

## 11. Defaults

The complete default configuration is `DEFAULT_CFG` in
`strat/implementations/browser-runtime/compiler/defaultConfig.js`; the values cited throughout this
document are normative snapshots of it. A conforming implementation must
produce identical compiled output for an empty-but-valid document (see
`strat/conformance/parse/`).

## 12. Worked example

```dsl
dsl v7
strategy "Spec Worked Example" {
  description "Documentation example: sweep-and-reclaim at prior-day levels."
}

market conditions {
  slices(XAUUSD 1h)
  sessions(london, ny)
  day type in (trending) or movement below 0.6
}

levels {
  priority(PDH, PDL)
  near within 1.2
}

filters {
  range active within 10 candles
  higher timeframe must not oppose entry
  side both
}

trigger {
  candle in (pin, engulf)
}

entry {
  when price sweeps range.high by 0.3 within 3 then signal short
  enter at market
}

risk {
  stop beyond last 3 candle extreme by 0.25 ATR
  stop size min 0.2 max 1.4
}

target {
  take profit opposite range edge
  minimum reward 0.8
  fallback 1
}

management {
  move stop to breakeven after 0.75 R plus 0.05 ATR
  wait 4 candles after trade
}

execution {
  risk 200 USD
}
```

Every fenced `dsl` block in this document must compile without errors;
`test/dslSpecExamples.test.mjs` enforces that in CI.
