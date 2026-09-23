# Engine WASM v0.6.1 final-artifact browser evidence

Date: 2026-09-23

## Result

Published v0.6.1 fixes the reproduced WebKit full-data WASM OOM. A bounded
WASM-only WebKit call on all 386,207 real XAUUSD 5m bars returned successfully
in 6.691 seconds without a Go exit, trap, rejected promise or page crash.

The combined WASM and JavaScript comparison cells established exact parity at
50,000 bars in WebKit and Chromium. Both full-data combined cells reached the
15-minute cap before returning, so exact full-data native/WASM/JavaScript
parity remains unmeasured. The WASM-only success does not turn those timed-out
comparison cells into parity evidence.

This is not a D-11 pass. Three local repeats do not meet the minimum 20
comparisons per cell or the production reliability requirement. Firefox was
not repeated after its earlier bounded failure and remains unmeasured for
v0.6.1.

## Provenance

| Item | Identity |
| --- | --- |
| Release | `v0.6.1`, commit `27d02fe0929441ea3b99bc67a65fdd0e63f5db24` |
| WASM | 5,838,695 bytes, SHA-256 `82c48fd89fcd63668277e504f175c628b27ad1285027142c4045d5825098d271` |
| `wasm_exec.js` | SHA-256 `45ce9dfe7211247544ab6f4268eb8cb5b6f3d5ae602dc3b51447b7eada99c229` |
| Native Darwin arm64 CLI | SHA-256 `8238a2fb311ec8fae41382ef70c4061cc73a324a128f67b38090f3a47527e2b1` |
| JavaScript comparison | app commit `d224b9a5613ecbdf8226cb2d2391147279295d9a` with v0.6.1 release semantics |
| XAUUSD 5m BBT1 | 386,207 bars, SHA-256 `3074ac6b47d7ade4104cc8325298f3953ab84b4a61f759031de265e3429c3e04` |

The published WASM compresses to 1,129,655 bytes with local Brotli settings,
within the provisional 8 MB asset budget. The native binary reports Go 1.22.12
and the release commit. Its build metadata also reports `vcs.modified=true`,
which is retained in the raw evidence.

## Combined cells

Each completed cell contains three sequential repeats. Times below are medians.

| Browser | Bars | Native | WASM | JavaScript | WASM / JS | Parity | Status |
| --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| WebKit 26.0 | 50,000 | 0.326 s | 0.825 s | 7.632 s | 0.108 | exact, 1,297 trades | measured |
| WebKit 26.0 | 386,207 | 2.550 s | — | — | — | not established | timed out at 15 min |
| Chromium 143.0.7499.4 | 50,000 | 0.326 s | 0.875 s | 4.907 s | 0.178 | exact, 1,297 trades | measured |
| Chromium 143.0.7499.4 | 386,207 | 2.550 s | — | — | — | not established | timed out at 15 min |
| Firefox | — | — | — | — | — | not established | unmeasured |

The normalized native, WASM and JavaScript digest for both 50,000-bar cells is
`a416f690384b35d2bb566d5cc271773fa5f594f81201974a89665c26df672c60`.
WebKit WASM runs were 0.792, 0.825 and 0.881 seconds. Chromium WASM runs were
0.875, 0.762 and 0.889 seconds.

Chromium exposed 842 MB used JavaScript heap and 1.36 GB total JavaScript heap
after the 50,000-bar cell. WebKit exposes no comparable `performance.memory`
or process RSS through Playwright.

## Full-data WASM isolation

The published WebKit WASM-only call returned 132,301 packed numeric trade
values, corresponding to 10,177 trades, and 5,780,867 bytes of trade strings.
The published native run also returned 10,177 trades. This count agreement is
not an exact parity digest.

WASM linear memory grew from 27,787,264 bytes to 2,292,711,424 bytes during the
full call. The call completed, but that memory level remains material evidence
for D-11 budget review. The result isolates the released OOM fix; it does not
explain or waive the combined-cell timeouts.

## Combined timeout isolation

A follow-up WebKit diagnostic applied separate two-minute caps to JavaScript
setup/transfer, context construction, engine execution and normalization. On
app main `dea45b7b`, setup took 83 ms and context construction took 9.687
seconds. The JavaScript engine phase then exceeded its two-minute cap;
normalization never began. Published v0.6.1 WASM had already completed the same
full input in 6.691 seconds, so neither WASM nor transfer/normalization caused
the combined-cell timeout.

The JavaScript opening-range-breakout implementation had the same contiguous
UTC-slot reverse-scan defect fixed in Go. At the first bar of a new slot it
read all 50,000 older bars instead of stopping at the immediately different
slot key. A one-line candidate fix reduced that regression from 50,000 indexed
bar reads to one. Against the same app commit plus that diff, full WebKit setup
took 78 ms, context construction 9.755 seconds, engine execution 1.722 seconds
and normalization 64 ms, returning 10,177 trades.

Those candidate timings prove the timeout cause but do not qualify unmerged
application code or establish exact full-data parity.

## Full-data parity rerun after the scan fix

After the application fix merged as `ad722ca2`, one published-artifact full
cell completed in each of WebKit and Chromium. Native and WASM agreed on 10,177
trades with digest
`fbe2c738c4eac9d048766414746f66d94fee81bb01d8b1d5dc0337c477de86dc`.
JavaScript produced digest
`e46725260609839419cbc00fa88aee60d4fa8fc97b39e27f8e9464478e6ffab7`
in both browsers.

The first JSON-significant difference is trade 5,438: native/WASM record
`exit` and `tp` as `2049.445`; JavaScript records both as
`2049.4449999999997`. The surrounding entry, stop, size, P&L, reason, indices,
timestamps and metadata agree. WebKit took 5.343 seconds for WASM and 14.147
seconds for JavaScript context plus engine work. Chromium took 5.918 seconds
for WASM and 10.568 seconds for JavaScript context plus engine work.

This browser-independent exact price mismatch fails the parity gate. Repeats
and Firefox were not run because the task required exact equality before those
resource-heavy cells. D-11 remains open.

## Evidence files

- `scripts/spikes/enginewasm/evidence/v0.6.1-real-data-webkit-chromium.json`
- `scripts/spikes/enginewasm/evidence/v0.6.1-webkit-full-wasm-only.json`
- `scripts/spikes/enginewasm/evidence/v0.6.1-webkit-full-js-main-baseline.json`
- `scripts/spikes/enginewasm/evidence/v0.6.1-webkit-full-js-fixed-candidate.json`
- `scripts/spikes/enginewasm/evidence/v0.6.1-full-parity-webkit.json`
- `scripts/spikes/enginewasm/evidence/v0.6.1-full-parity-chromium.json`

HT-036 remains open for exact full-data parity, at least 20 comparisons per
required cell, Firefox coverage and production shadow reliability evidence.
