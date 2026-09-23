# Engine WASM v0.5.0 real-data qualification

Date: 2026-09-23

This report measures the published `v0.5.0` artifacts. `v0.6.0` was published
after the run started and is not qualified by these results.

## Result

`v0.5.0` does not pass T-F0 or D-11. The 50,000-bar WebKit cell has exact
native/WASM/JavaScript trade equality, but its median WASM run is 1.504 times
the JavaScript path. The provisional budget is at most 1.5 times JavaScript.
Firefox did not return the aggregate matrix within 45 minutes. The bounded
WebKit 200,000-bar cell timed out after 15 minutes, and the 386,207-bar bridge
call returned no result. The minimum 20 comparisons per cell and a
crash-plus-timeout rate below 0.5% are therefore unmet.

The published WASM is 1,127,743 bytes after local Brotli compression, below the
provisional 8 MB asset budget.

## Inputs

| Input | Identity |
| --- | --- |
| XAUUSD 5m BBT1 | 386,207 bars, 18,537,952 bytes, SHA-256 `3074ac6b47d7ade4104cc8325298f3953ab84b4a61f759031de265e3429c3e04` |
| WASM | 5,827,444 bytes, SHA-256 `3f0f2b9efabcfc785e0ea6c550113a7840a20ac8c31f2cd5085fe5371e3740b9` |
| `wasm_exec.js` | SHA-256 `45ce9dfe7211247544ab6f4268eb8cb5b6f3d5ae602dc3b51447b7eada99c229` |
| Native Darwin arm64 CLI | SHA-256 `495ea1d15a1350d0e52617204e970a584d7da830bb4681a23870847642b05a3c` |
| Producer revision | `ff2964f3bdaa1230d6d0ed956a494cd43e6b049a` |
| JavaScript comparison | app revision `0b30bec37d51487570deecd81616ae0d6bb7ed28`, with `v0.5.0` release semantics applied |

The published native binary reports Go 1.22.12 and the expected producer
revision. Its embedded build metadata also reports `vcs.modified=true`; the
hash above identifies the measured release asset without hiding that flag.
The available real file has 386,207 bars, so the full cell uses that count
rather than padding it to 400,000.

Host: Apple M4, 10 logical CPUs, 16 GiB memory, macOS 25.5.0, Node v26.0.0,
Playwright 1.57.0.

## Measurements

Each completed cell has three sequential repeats. Times are wall-clock
medians. The bridge timings come from the WASM response. Native timings include
CLI process startup and JSON output. JavaScript totals include context creation
and the engine run.

| Browser | Bars | Native | WASM | JavaScript | WASM / JS | Parity | Result |
| --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| WebKit | 50,000 | 2.273 s | 10.530 s | 7.003 s | 1.504 | exact, 1,297 trades | measured |
| WebKit | 200,000 | 32.599 s | — | — | — | not established | timed out at 15 min |
| WebKit | 386,207 | 119.880 s | — | — | — | not established | failed: bridge returned no result |
| Firefox | matrix | — | — | — | — | not established | aggregate attempt stopped at 45 min |

For WebKit 50,000, the three WASM runs were 10.300, 10.545 and 10.530
seconds. The matching JavaScript totals were 6.017, 7.003 and 8.165 seconds.
The WASM engine medians were about 10.495 seconds; median bridge work was about
2 ms copy-in, 6 ms adapter, 24 ms pack and 1 ms copy-out. Initial asset fetch,
compile, instantiate and runtime-ready times were 14, 24, 2 and 11 ms.

The normalized native, WASM and JavaScript trade digest for WebKit 50,000 is
`a416f690384b35d2bb566d5cc271773fa5f594f81201974a89665c26df672c60`.
The earlier JavaScript exit-reason mismatch came from running the benchmark
with legacy defaults. Applying the pinned release semantics fixed the harness;
it did not require an engine change.

## Reliability and memory limits

The original aggregate Chromium matrix completed in process, but its evidence
was lost when the following Firefox phase exceeded 45 minutes because the
first harness wrote only at the end. Those Chromium numbers are excluded. The
revised harness launches and records each browser/size cell separately and
uses `--cell-timeout-ms` so later failures cannot erase earlier cells.

The WebKit full-data failure surfaced as `TypeError: undefined is not an object
(evaluating 'wasm.ok')`. The browser did not provide a bridge response, so the
run cannot distinguish a WASM trap, process memory failure or another browser
runtime failure. It remains an unresolved reliability failure.

WebKit does not expose `performance.memory`, and Playwright does not expose a
comparable WebKit or Firefox process RSS metric. During the stopped Firefox
attempt, the content process remained CPU-active and was observed around
1.2 GiB RSS, but that operating-system sample is diagnostic only and is not a
portable memory budget measurement.

Three local repeats cannot establish the D-11 requirement of at least 20
comparisons per cell or a timeout rate below 0.5%. Production shadow telemetry
is still required. The data and artifacts are real; synthetic browser fixtures
are not counted as qualification evidence.

## Reproduction

The harness requires explicit release identity so a later `v0.6.0` run can use
the same code without changing source:

```sh
ENGINEWASM_PLAYWRIGHT_PACKAGE=/path/to/playwright node \
  scripts/spikes/enginewasm/real-data-browser-benchmark.mjs \
  --release=v0.5.0 \
  --release-commit=ff2964f3bdaa1230d6d0ed956a494cd43e6b049a \
  --wasm=/path/to/enginewasm.wasm \
  --wasm-exec=/path/to/wasm_exec.js \
  --native=/path/to/heisentick-darwin-arm64 \
  --data=/path/to/XAUUSD/5m.bin \
  --app=/path/to/heisentick \
  --output=/path/to/evidence.json \
  --repeats=3 \
  --browsers=chromium,firefox,webkit \
  --cell-timeout-ms=900000
```

The JSON evidence is
`scripts/spikes/enginewasm/evidence/v0.5.0-real-data-webkit.json`.
