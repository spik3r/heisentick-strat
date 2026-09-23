# Engine WASM v0.6.0 WebKit full-data OOM

Date: 2026-09-23

## Finding

One capped WebKit run reproduced the v0.5.0 full-data bridge failure with the
published v0.6.0 artifacts. The bridge returned `undefined` after 327.590
seconds because the Go runtime exhausted WASM linear memory and exited with
code 2. This was not a WebAssembly trap, a rejected bridge promise or a WebKit
process crash.

The runtime reported:

- linear memory grew from 27,787,264 to 4,293,394,432 bytes;
- 2,256,273,408 bytes were in use;
- allocation of another 4 MiB block failed; and
- the fatal stack was in opening-range-breakout `sameWindowBars`, through
  `utcSlotProgress` and `fmt.Sprintf`.

At the first bar of a UTC slot, `sameWindowBars` scanned every older bar when
the immediately preceding bar had a different slot key. UTC slots are
contiguous, so an older matching slot cannot exist after that first mismatch.
This made slot-boundary work grow with the entire history and repeatedly
formatted keys while scanning it.

## Fix and regression

The fix stops the reverse scan at the first UTC-slot mismatch. It does not
change the selected bars: the old scan also returned no bars at that boundary.

The regression constructs 50,000 older bars and asks for the prior bars at a
new UTC-slot boundary. It measures 99,964 allocations against the v0.6.0
implementation and at most 10 after the fix.

A single capped WebKit run of a local fixed candidate completed successfully in
5.468 seconds. It returned 132,301 packed trade values and 5,780,867 bytes of
trade strings without a Go exit, promise rejection, trap or page crash. Linear
memory ended at 1,530,462,208 bytes. This candidate was built locally with the
host Go toolchain and is diagnostic evidence, not a published release result or
a D-11 qualification.

## Provenance

Published v0.6.0:

- release commit `38ca44c52723f62ad7cc264143b2500cd79dcc08`;
- `enginewasm.wasm` SHA-256
  `61ff907ae801cda62ec7cf64554b1efd3acce9181888f0cafd76823e27361bfb`;
- `wasm_exec.js` SHA-256
  `45ce9dfe7211247544ab6f4268eb8cb5b6f3d5ae602dc3b51447b7eada99c229`;
- real XAUUSD 5m input: 386,207 bars, SHA-256
  `3074ac6b47d7ade4104cc8325298f3953ab84b4a61f759031de265e3429c3e04`.

Raw evidence:

- `scripts/spikes/enginewasm/evidence/v0.6.0-webkit-full-diagnostic.json`
- `scripts/spikes/enginewasm/evidence/v0.6.0-webkit-full-fixed-candidate.json`

The current v0.6.0 release remains unqualified. A new release would need the
normal producer review and publication flow, followed by bounded final-artifact
measurements and D-11 reliability evidence.
