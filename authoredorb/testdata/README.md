# Authored ORB frozen JS fixtures

These cases were produced by `generate.mjs` against the read-only app source
at commit `a7bc97689838d7cc5e2d7fdf462b9ca2f5dbfd28` (the active authored
IDs and source files were also verified unchanged at main `2df6693b`).
Run `node generate.mjs /path/to/heisentick-app` from any directory only when
intentionally reviewing a source change. The Go tests compare complete trade
fields and the per-bar equity curve against the frozen JS results.

`authored-orb-run-v1` retains an important JS behavior: `defineStrategy`
returns before calling the strategy's `onBar` while a position is open. The
strategy's in-bar `session close` exit is therefore unreachable and its
EMA, ATR, ORB, and day state pause while a position remains open. The
`parity-held-through-session-close` fixture makes this visible as `eod`.
The first bar of a run is also counted twice in the strategy's HLC3 fallback
VWAP because initialization and the ordinary same-day branch both add it;
`parity-first-bar-hlc3-double-count` freezes that edge behavior.
Fixing that source bug would require a separately named schema version and
new reviewed fixtures, not a silent change to this port.
