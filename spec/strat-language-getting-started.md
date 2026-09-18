# Getting to Know the Strat Language

A hands-on tour of the Strat strategy language: what it is, how to write a
strategy from nothing, and how to grow it feature by feature while testing each
step. Every fenced `dsl` block below compiles as-is.

- Normative reference: [`dsl-spec.md`](dsl-spec.md) (v7). This guide teaches; the
  spec is the source of truth.
- Per-family phrases: [`dsl-spec-families/`](dsl-spec-families/).
- Quick phrase lookup: [`dsl-phrase-reference.md`](dsl-phrase-reference.md).
- Available context data (ATR, EMA, sessions, VP…):
  [`strategy-dsl-context.md`](../../docs/strategy-dsl-context.md).
- Testing commands: `skills/strategy-testing/SKILL.md`.

## 1. The mental model

A strategy is a plain-text document read **line by line**. There is no
free-form expression grammar — every meaningful line is a **directive phrase**:
a head word followed by tokens in a fixed shape, written in trading language.

```
sessions(london, ny)
day type in (trending) or movement below 0.6
stop beyond last 3 candle extreme by 0.25 ATR
```

Directives are grouped into **sections** (`market conditions`, `setup`,
`levels`, `filters`, `trigger`, `entry`, `risk`, `target`, `management`,
`grade`, `execution`). Sections are mostly documentary — a directive means the
same thing wherever it sits — but grouping keeps a strategy readable, and a few
directives are section-scoped (noted in the spec).

Two rules shape everything:

- **Trading language, not code.** `type: flag continuation`, not a function
  call. Commas, colons and parentheses are optional sugar:
  `sessions(asia, london)`, `sessions: asia london`, and `sessions asia london`
  are identical.
- **Strictly causal.** Nothing may read the current bar's future or a
  still-forming higher-timeframe candle. Levels, ranges, biases, seasonality and
  volume-profile context are computed from completed data only. If an idea can't
  respect that, it belongs in a chart overlay, not a strategy.

## 2. Your first strategy

The smallest thing that compiles and runs: pick a route and a setup family.

```dsl
dsl v7
strategy "Starter" {
  description "Minimal strategy: fade a failed breakout of prior-day levels."
}

market conditions {
  slices(XAUUSD 1h)
}

setup {
  type: failed breakout
}
```

- `dsl v7` names the language version (optional but recommended).
- `slices(XAUUSD 1h)` routes it to one symbol/timeframe pair.
- `type: failed breakout` selects the setup family (the built-in entry/exit
  logic). It's also the default, but always declare it.

Everything else (levels, stop, target, sizing) falls back to sensible defaults
from `strat/implementations/browser-runtime/compiler/defaultConfig.js`.

## 3. The edit → embed → test loop

Strategies live as `.strat` files in `strategies/source/`. The `.strat` file
is the source of truth; a build step embeds it into a registered JS module.

1. **Write** `strategies/source/dslMyIdea.strat`.
2. **Embed** it into the registry: `pnpm run dsl:embed` (generates the matching
   `dsl*.js`; never hand-edit that generated file). `pnpm run dsl:embed:check`
   verifies they're in sync.
3. **Lint** without running: `pnpm run dsl:lint`.
4. **Test** a route:
   ```bash
   pnpm run report -- --strategy=dslMyIdea --symbol=XAUUSD --tf=1h --range=preferred
   ```

You can also author interactively in the **DSL Editor** tab in the app: it gives
line-anchored diagnostics, phrase help, and templates, and lets you run a
strategy before you commit it to a file. For a fast throwaway loop before
registering anything, point the funnel/report tools at a scratch file:

```bash
pnpm run funnel -- --dsl-file=./scratch.strat --dsl-id=scratch --preferred=1 --range=preferred
```

## 4. Growing the strategy, section by section

Start from the minimal strategy and add one concern at a time, re-testing after
each addition.

### 4.1 Where and when — `market conditions`

Restrict routing and time-of-day, and gate on the shape of the day.

```dsl
dsl v7
strategy "Where and when" {
  description "Trade only liquid sessions on trending days."
}

market conditions {
  slices(XAUUSD 1h)
  sessions(london, ny)
  day type in (trending) or movement below 0.6
}

setup {
  type: failed breakout
}
```

- `sessions(london, ny)` — only London and NY hours.
- `day type in (trending) or movement below 0.6` — trending days, but still
  allow a non-trending day when it's quiet (movement efficiency ≤ 0.6). Add
  `strictly` to remove the escape hatch.
- More gates in the spec: `trade window`, `local weekday/hour`,
  `session phase`, `prior day type`, `open location`, `session bias`,
  `seasonality`, and range-usage/exhaustion filters.

### 4.2 The level universe — `levels`

Setups anchor to levels. Declare which ones matter and how close price must be.

```dsl
dsl v7
strategy "Levels" {
  description "Anchor entries to prior-day high/low within 1.5 ATR."
}

market conditions {
  slices(XAUUSD 1h)
  sessions(london, ny)
}

levels {
  priority(PDH, PDL)
  near within 1.5
}

setup {
  type: failed breakout
}
```

- `priority(PDH, PDL)` — the ordered level list (prior-day high/low here). Many
  names are built in (PDO, WH/WL, VWAP, POC/VAH/VAL, round numbers `RN…`, …).
- `near within 1.5` — a signal must be within 1.5 ATR of a priority level.
- You can also define custom levels (`support`/`resistance`, `trendline`,
  `fib`) and require `level must align with another level within X ATR`.

### 4.3 Quality gates — `filters`, `trigger`

Filters must hold for any entry; the trigger constrains the signal candle.

```dsl
dsl v7
strategy "Filters and trigger" {
  description "Only with the higher timeframe, on clean rejection candles."
}

market conditions {
  slices(XAUUSD 1h)
  sessions(london, ny)
}

levels {
  priority(PDH, PDL)
  near within 1.5
}

filters {
  higher timeframe must agree
  side both
}

trigger {
  candle in (pin, engulf)
}

setup {
  type: failed breakout
}
```

- `higher timeframe must agree` — require HTF alignment (or the softer
  `higher timeframe must not oppose entry`).
- `side both` — allow longs and shorts (or `side long only` / `side short only`).
- `candle in (pin, engulf)` — accept only pin bars and engulfing signals.

### 4.4 Risk, target, management, sizing

How the trade is protected, where it aims, how it's managed, and how big it is.

```dsl
dsl v7
strategy "Full loop" {
  description "Stop beyond structure, edge target, breakeven + cooldown."
}

market conditions {
  slices(XAUUSD 1h)
  sessions(london, ny)
  day type in (trending) or movement below 0.6
}

levels {
  priority(PDH, PDL)
  near within 1.5
}

filters {
  higher timeframe must agree
  side both
}

trigger {
  candle in (pin, engulf)
}

setup {
  type: failed breakout
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

- `stop beyond last 3 candle extreme by 0.25 ATR` + `stop size min/max` — a
  structure stop, clamped to a sane ATR range.
- `take profit opposite range edge` with `minimum reward` / `fallback` — aim at
  the far edge, but skip the trade if that edge pays less than 0.8R, and fall
  back to 1R when there's no edge. A fixed target is simply `target 2R`.
- `move stop to breakeven after 0.75 R plus 0.05 ATR` and
  `wait 4 candles after trade` — management and a re-entry cooldown.
- `risk 200 USD` — position size = risk ÷ stop distance.

### 4.5 Optional: grade the signal — `grade`

Score signals on context/location/trigger/reward and reject weak ones (or size
by score).

```dsl
dsl v7
strategy "Graded" {
  description "Sweep-and-reclaim, only above a minimum quality score."
}

market conditions {
  slices(XAUUSD 1h)
  sessions(london, ny)
}

levels {
  priority(PDH, PDL)
  near within 1.2
}

entry {
  when price sweeps range.high by 0.3 within 3 then signal short
  enter at market
}

risk {
  stop beyond last 3 candle extreme by 0.25 ATR
}

target {
  target 2R
}

grade {
  context 2
  location 2
  trigger 1
  risk reward 1
  require >= 4
}

execution {
  risk 200 USD
}
```

`require >= 4` rejects any signal scoring under 4; `size by score …` can scale
risk by quality instead. Graded trades carry `gradeScore` and per-component
detail that the report tools bucket for you.

## 5. Reading the feedback

- **Compile diagnostics** — errors/warnings are prefixed with the source line;
  an unknown head suggests the closest known directive. Fix these first (the DSL
  Editor shows them inline).
- **Few or no trades?** Run the funnel to see where bars drop out across market
  gates, level proximity and setup candidates:
  ```bash
  pnpm run funnel -- --strategy=dslMyIdea --preferred=1 --range=preferred
  ```
- **Is the edge real?** Prefer `validate-strategy` — it bundles the report,
  robustness grid, Monte Carlo distribution and anti-overfit warnings:
  ```bash
  pnpm run --silent validate-strategy -- --strategy=dslMyIdea --iters=3000 --json-only=1
  ```
  Judge profit factor, expectancy, drawdown, sample size and yearly consistency
  over net P&L — and always check realistic vs harsh costs. A tiny high-PF
  sample is a lead, not a strategy.

## 6. How a signal is admitted (evaluation order)

Knowing the order helps you reason about which gate is filtering trades. The
first failing gate rejects the signal:

1. market gates (sessions, windows, weekday/hour/phase, day/prior-day type,
   open location, movement, range-stat and bias filters, routing);
2. seasonality (side-aware);
3. signal-candle quality;
4. day-theme gate (side-aware);
5. entry-distance cap;
6. setup-age cap;
7. grade requirement, then grade-based sizing;
8. broker admission (cooldown, one position at a time, side allowances).

## 7. Where to go next

- Skim the **worked example** and section reference in
  [`dsl-spec.md`](dsl-spec.md).
- Pick a **setup family** that matches your idea and read its file in
  [`dsl-spec-families/`](dsl-spec-families/) for the phrases it adds (flag
  depth, channel width, retrace ratio, VWAP extension, …).
- Copy a real strategy from `strategies/source/*.strat` and change one thing
  at a time.
- Promote only with evidence: follow `skills/strategy-testing/SKILL.md` and the
  bar in [`strategy-library.md`](../../docs/strategy-library.md).
