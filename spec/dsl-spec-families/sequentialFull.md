# Setup family: `sequential full` (`sequentialFull`)

Status: bounded synthetic Go execution (HT-228). This is the accepted public-rule
approximation `seq.full.public_approx.v1`, using the existing `engine/seqcore`
lifecycle and existing Go broker. It is not proprietary DeMARK parity, a
historical study, a released consumer integration, or evidence of an edge.
The archived `legacy setup 9` family and its two profile IDs remain unchanged.

```dsl
dsl v7
strategy "Sequential synthetic E1" {
  description "Explicit synthetic execution inputs."
}
market {
  sequential symbol SYNTH
  sequential timeframe 1m
}
setup {
  type: sequential full
  sequential profile seq.full.public_approx.v1
  sequential policy E1
}
risk {
  sequential riskUsd 25
  sequential maxNotionalUsd 10000
}
```

Replace `E1` with `E2` for the Countdown policy. The risk and cap numbers above
are illustrative synthetic inputs, not defaults or recommended study values.
Both are required finite positive values. The cap is an absolute notional
amount, not leverage or a fraction of equity. No study budget is implied.

## Closed source and config

Only the four blocks above are accepted. Each block and directive appears once;
`description` is optional. Within `setup`, `type` precedes the profile and policy.
The strategy name is nonempty. All directives are
consumed, so unknown, duplicated, misplaced or trailing text is rejected. Keywords
are case-insensitive; profile/policy values, uppercase symbol identifiers and
lowercase timeframe labels are canonical and case-sensitive. Numeric source
inputs are positive decimal literals without signs or exponent notation.

The symbol is an explicit uppercase identifier of 1–32 characters, beginning
with a letter and containing letters, digits, underscore, dot or hyphen. Supported
timeframe literals are `1m`, `5m`, `15m`, `30m`, `1h`, `4h`, and `1d`. These are
fixed-duration synthetic capabilities, not exchange-calendar qualification.
The runtime request must match the single authored symbol/timeframe exactly.

The complete config contains only `dslVersion`, `name`, `description`,
`setupType`, and `sequentialFull`. Its family object contains only:

- `contractVersion`: `sequential-full-config-v1`
- `profile`: `seq.full.public_approx.v1`
- `policy`: `E1` or `E2`
- `symbol`, `timeframe`, `riskUsd`, `maxNotionalUsd`: the explicit inputs

No generic defaults, filters, stops, targets, management rules or route lists
are inherited. The parser and `DecodeSequentialFullConfig` validate the closed
projection independently. `DecodeSequentialFullConfigJSON` also rejects duplicate
decoded keys, malformed Unicode and trailing JSON documents. A reserved family
object or full-profile value under a relabeled family cannot fall back to legacy
execution. Invalid source returns empty config plus `unsupported_config`
diagnostics; native config admission returns `SequentialFullConfigError`.

P1, nondefault core counts, optional qualifiers, Intersection, Combo, Risk Level,
TDST export/cancellation, custom buffers, arbitrary holding periods and additional
filters are unsupported. Their omission does not silently enable anything.

## Signal and execution rules

All decisions occur after the bar closes. The core's strict Setup/perfection,
inclusive Countdown/terminal qualification, flip gate, deferral, cancellation,
overlap and recycle semantics remain those of `seq-core-freeze.v1`.

- E1 is countertrend on the first immediate or delayed perfection event of a
  completed Setup episode. Delayed age is decision index minus Setup9 index:
  ages 1–4 are eligible; age 5 or later is `signal_expired`. A decision on the
  last eligible bar may fill at the following open, outside that response window.
- E2 is countertrend on a qualified `countdown_complete` event. Unqualified
  terminal attempts do not signal. Completion followed by recycle on the same
  bar still retains its eligible completion event, even though the snapshot ends
  idle. The twelve-later-bar response window does not delay this immediate E2
  decision or create retries.
- E1's anchor is the lowest low for a buy or highest high for a sell over Setup
  bars 1–9. E2's range starts at Setup1 of the episode that originally admitted
  its Countdown and includes every bar through qualified CD13. Later same-side
  Setups never replace that active Countdown's anchor.
- ATR is the simple mean of true range over 14 bars, with at least 14 closed
  bars required. It is frozen on the decision bar, including a delayed-perfection
  decision. The structural stop is anchor minus 0.10 ATR for a long, plus 0.10 ATR
  for a short; it does not move to follow the entry fill.
- Costs must explicitly select next-open fills, using existing `open` or
  `nextOpen` spelling. Empty or `close` fills are refused. At actual slipped fill
  F, reject a long if stop >= F or a short if stop <= F, without opening a trade.
  Otherwise target is F plus/minus twice the fill-to-stop distance. Risk sizing
  uses that distance and the explicit absolute notional cap.
- Each policy is a separate one-position book. Signals while occupied are
  dropped with `blocked_in_position`; they are never queued for a later
  opportunity. Simultaneously executable buy/sell candidates skip both with
  `simultaneous_signal`.
- Stops and targets are checked on the entry bar. A bar touching both is
  stop-first. Carried stop gaps fill at the worse open, using the existing broker.
  The entry bar e is held bar 1. E1 exits at open e+4; E2 exits at open e+12,
  before that bar's stop/target checks. Exits precede new closed-bar decisions.

Ordinary Run/PrepareRun, fixture and WASM transports execute real broker trades.
The optional execution record explains opportunities and refusals; it is not a
separate diagnostic product or a substitute for trades. Repeated prepared runs
must start from fresh policy and broker state.

## Temporary synthetic capability boundary

This slice admits only contiguous, strictly increasing synthetic timestamps.
Missing intervals and calendar declarations are refused; it does not import
weekend/calendar assumptions. Source/HTF/C5, execution windows, transfer,
shared/grid, full-policy prefix and checkpoint/resume paths are unsupported and
must fail explicitly. The independent core's checkpoint support does not imply
execution-checkpoint support.

A signal at the terminal decision bar records `no_next_bar`. If a position is
still held at dataset end before its natural stop, target or scheduled time exit,
the whole run returns a typed incomplete-terminal refusal. It must not force a
last-close liquidation or return an empty successful result. Supply enough
contiguous synthetic bars to observe the policy's natural exit.

These temporary admissions defer unresolved gap-execution and terminal-truncation
semantics. They do not change the core lifecycle, authorize real-market data,
qualify a historical calendar, or choose any HT-231 study numbers. Producer
regression tests are separate from the independently hand-derived acceptance
corpus owned in the central backlog.
