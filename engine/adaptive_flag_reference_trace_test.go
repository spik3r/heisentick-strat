package engine

import (
	"crypto/sha256"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
)

// These digests were independently generated from the corrected Python source
// whose exact SHA256 is AdaptiveFlagReferenceSHA256, using only its invented
// fixture and the explicit variants below. Python sys.settrace captured locals
// at each simulate-loop back edge, after that row's close, including continue
// branches. Each line is the format built below. Floats use big-endian binary64
// bits (Python struct.pack('>d', value).hex()), avoiding JSON/display rounding.
// SNAPSHOT_C is dataclasses.replace(TWEAKED, atr_stop_mult=2.0), not a change to B.
// These tests qualify the declared reference, never TradingView or market data.
func TestAdaptiveReferencePinnedPythonCloseStateTraces(t *testing.T) {
	cases := []struct{ name, digest string }{
		{"source", "94382005c258386b09e646cd82a911f117ebd18f10ad80784869a9fbe691ccaf"},
		{"latch", "3f9ee2c618e823b5588948f9838b66907ddf91ea2e3cc747fdd090a397991435"},
		{"expiry", "089ca4074f53d7951d52efa2382f122292d6d3439e00a7dbd2dfec87a12ddc59"},
		{"hold", "4c378229be743e93b7d322604c0d129482eea3900cc6a4b6a5d92369e2f3082d"},
		{"no-confirmation", "01dd9fa2422174a11cceafe085e023c4c6d4d2fa7e817411336441ac1e8aca82"},
		{"pending", "190186beac83696346cc9f027114f8495ea70ed617d31cfef0c882095773d788"},
		{"open", "ee7a568544a19eda4fd7248003d0c0e35ecbe8b2e16ab9d5fed6a9b5964b0b8d"},
		{"queued", "f375bc2becb0c17e5ef81fe2cfa32ba53b480cf87febeb7f5b7cc6a1a9394ffc"},
		{"short", "d059397e5bfc8ec5e9287b1e060af32df33f1cbbef56b3ef357589d7a2b1428e"},
		{"tweaked", "10d0f158e4c0e762bc17cdc5f64c156bdcd28bd9bb038d7274a5d2a75db77bb0"},
		{"snapshot-c", "93c82965b7a66a86c03f9ae72e3a3bd8d919c62742c1af8b82235f8219bc91bc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := adaptiveTestRules(t)
			bs := adaptiveTestBreakout()
			bundle := "INITIAL"
			switch tc.name {
			case "latch":
				bs[28] = adaptiveTestBar(28, 107, 109, 100, 101)
				bs[28].Volume = 0
				bs[29] = adaptiveTestBar(29, 106, 109, 105, 108)
			case "expiry":
				bs = bs[:28]
				for i := 28; i <= 41; i++ {
					bs = append(bs, adaptiveTestBar(i, 107, 107.4, 106.4, 107))
				}
			case "hold":
				bs = bs[:29]
				for i := 29; i < 89; i++ {
					bs = append(bs, adaptiveTestBar(i, 110, 111, 109, 110.5))
				}
				bs = append(bs, adaptiveTestBar(89, 123, 124, 122, 123))
			case "no-confirmation":
				bs = bs[:27]
			case "pending":
				bs = bs[:28]
			case "open":
				bs = bs[:29]
			case "queued":
				bs = bs[:29]
				bs[28] = adaptiveTestBar(28, 107, 109, 100, 101)
			case "short":
				bs = adaptiveTestReflect(bs)
				r.MaxFlagBars = 3
				bundle = "CUSTOM"
			case "tweaked":
				r, _ = dsl.AdaptiveFlagPreset("TWEAKED")
				bundle = "TWEAKED"
			case "snapshot-c":
				r, _ = dsl.AdaptiveFlagPreset("SNAPSHOT_C")
				bundle = "SNAPSHOT_C"
			}
			out := adaptiveTestRun(t, bs, bundle, r)
			var trace strings.Builder
			bit := func(v float64) string { return fmt.Sprintf("%016x", math.Float64bits(v)) }
			optional := func(v *float64) string {
				if v == nil {
					return "nil"
				}
				return bit(*v)
			}
			pivot := func(p *AdaptiveFlagPivot) string {
				if p == nil {
					return "nil"
				}
				return fmt.Sprintf("%d,%s", p.Index, bit(p.Price))
			}
			for i, state := range out.States {
				pending, position, queue := "nil", "nil", "nil"
				if state.PendingOrderID != nil {
					o := out.Orders[*state.PendingOrderID]
					pending = fmt.Sprintf("%d,%d,%s,%s,%s,%s", o.SignalIdx, state.PendingAge, o.Side, bit(o.Trigger), bit(o.Stop), bit(o.Target))
				}
				if state.PositionOrderID != nil {
					o := out.Orders[*state.PositionOrderID]
					position = fmt.Sprintf("%d,%d,%s,%s,%s,%s", o.SignalIdx, *o.FillIdx, o.Side, bit(*o.Entry), bit(o.Stop), bit(o.Target))
				}
				if state.QueuedExit != nil {
					queue = fmt.Sprintf("%s,%d", state.QueuedExit.Reason, state.QueuedExit.NextOpenIdx)
				}
				s := out.Snapshots[i]
				fmt.Fprintf(&trace, "%d|%s|%s|%s|%s|%s|%s|%s|%s|%s\n", i, pending, position, queue, pivot(s.LastHigh), pivot(s.LastLow), optional(s.ATR), bit(s.FastEMA), bit(s.SlowEMA), optional(s.VolumeSMA))
			}
			got := fmt.Sprintf("%x", sha256.Sum256([]byte(trace.String())))
			if got != tc.digest {
				t.Fatalf("corrected Python close-state trace differs: got %s want %s\n%s", got, tc.digest, trace.String())
			}
		})
	}
}
