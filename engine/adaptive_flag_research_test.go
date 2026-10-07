package engine

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
)

func researchTestRequest(t *testing.T, bars []AdaptiveFlagBar, mode AdaptiveFlagResearchAblation) AdaptiveFlagRequest {
	t.Helper()
	r, err := dsl.AdaptiveFlagPreset("SNAPSHOT_C")
	if err != nil {
		t.Fatal(err)
	}
	return AdaptiveFlagRequest{Config: dsl.AdaptiveFlagConfig("Invented caps", "Synthetic mechanics only", "M30", "SNAPSHOT_C", r), Series: adaptiveTestSeries(bars), ResearchAblation: mode}
}
func researchTestModes() []AdaptiveFlagResearchAblation {
	return []AdaptiveFlagResearchAblation{AdaptiveFlagResearchRetraceCapOff, AdaptiveFlagResearchWidthCapOff, AdaptiveFlagResearchBothCapsOff}
}
func researchTestRun(t *testing.T, req AdaptiveFlagRequest) AdaptiveFlagResult {
	t.Helper()
	out, err := RunAdaptiveVolumeFlag(req)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// A high pivot and subsequent tight lower shelf isolate retrace failure while
// preserving the original moving endpoint, pivot ordering, and both EMA/volume.
func researchTestRetraceBars() []AdaptiveFlagBar {
	bs := adaptiveTestBars(20)
	for i := range bs {
		bs[i] = adaptiveTestBar(i, 100, 101, 99, 100)
	}
	for _, v := range [][4]float64{{100, 101, 90, 95}, {95, 108, 94, 107}, {107, 118, 106, 117}, {117, 126, 116, 125}, {125, 130, 124, 129}, {114, 116, 110, 115}, {114, 116, 110, 115}, {114, 116, 110, 115}, {114, 116, 110, 115}, {114, 116, 110, 115}, {114, 116, 110, 115}, {116, 118, 113, 117}, {117, 119, 113, 118}, {118, 121, 116, 120}} {
		bs = append(bs, adaptiveTestBar(len(bs), v[0], v[1], v[2], v[3]))
	}
	return bs
}
func TestAdaptiveResearchSourceBackedCapAdmissions(t *testing.T) {
	width := adaptiveTestBreakout()
	width[28] = adaptiveTestBar(28, 107, 120, 106, 119)
	fixtures := []struct {
		name     string
		bars     []AdaptiveFlagBar
		start    int
		admitted [3]bool
	}{
		{"retrace-only-fails", researchTestRetraceBars(), 30, [3]bool{true, false, true}},
		{"width-only-fails", width, 28, [3]bool{false, true, true}},
		{"both-fail", adaptiveTestBreakout(), 28, [3]bool{false, false, true}},
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			req := researchTestRequest(t, f.bars, "")
			req.Window = &AdaptiveFlagExecutionWindow{TradeFromMS: int64(f.start) * 1800000, TradeToMS: int64(len(f.bars)) * 1800000}
			base := researchTestRun(t, req)
			if len(base.Orders) != 0 {
				t.Fatal("baseline must reject synthetic cap failures")
			}
			for k, mode := range researchTestModes() {
				req.ResearchAblation = mode
				out := researchTestRun(t, req)
				if (len(out.Orders) > 0) != f.admitted[k] || (out.Snapshots[f.start].Candidate != nil) != f.admitted[k] {
					t.Fatalf("cap admission %s orders%+v", mode, out.Orders)
				}
				if f.admitted[k] && (out.Orders[0].SignalIdx != f.start || out.Events[0].State != "pending" || out.Orders[0].FillOpportunities == 0) {
					t.Fatal("missing source-backed order lifecycle")
				}
			}
		})
	}
}

func TestAdaptiveResearchClosedAdmissionAndImmutableMetadata(t *testing.T) {
	bs := adaptiveTestBreakout()
	base := researchTestRun(t, researchTestRequest(t, bs, ""))
	b, _ := json.Marshal(base)
	if strings.Contains(string(b), "research") {
		t.Fatal("legacy metadata leaked")
	}
	for _, mode := range researchTestModes() {
		req := researchTestRequest(t, bs, mode)
		out := researchTestRun(t, req)
		if out.Schema != AdaptiveFlagResearchSchema || out.ConfigSHA256 != base.ConfigSHA256 || !reflect.DeepEqual(out.EffectiveConfig, base.EffectiveConfig) {
			t.Fatal("schema or base identity changed")
		}
		hash, err := adaptiveFlagHash(*out.ResearchPolicy)
		if err != nil || hash != out.ResearchPolicySHA256 {
			t.Fatal("policy hash")
		}
		original := *out.ResearchPolicy
		out.ResearchPolicy.CandidateID = "mutated"
		out.ResearchProducer.Contract = "mutated"
		req.ResearchAblation = "unknown"
		req.Config["adaptiveVolumeFlag"].(map[string]any)["bundle"] = "CUSTOM"
		again := researchTestRun(t, researchTestRequest(t, bs, mode))
		if *again.ResearchPolicy != original || again.ResearchProducer.Contract != AdaptiveFlagResearchProducerContract {
			t.Fatal("metadata alias")
		}
	}
	for _, mode := range []AdaptiveFlagResearchAblation{"G1", "C_RETRACE_CAP_OFF_CORE_V1 ", "C_EMA_OFF", "C_RETRACE_CAP_OFF_CORE_V1,C_WIDTH_CAP_OFF_CORE_V1"} {
		req := researchTestRequest(t, bs, mode)
		if _, err := RunAdaptiveVolumeFlag(req); err == nil {
			t.Fatal("unknown accepted", mode)
		}
	}
	for _, bundle := range []string{"CUSTOM", "INITIAL", "TWEAKED"} {
		r, _ := dsl.AdaptiveFlagPreset("SNAPSHOT_C")
		if bundle != "CUSTOM" {
			r, _ = dsl.AdaptiveFlagPreset(bundle)
		}
		for _, mode := range researchTestModes() {
			req := researchTestRequest(t, bs, mode)
			req.Config = dsl.AdaptiveFlagConfig("Synthetic", "Only", "M30", bundle, r)
			if _, err := RunAdaptiveVolumeFlag(req); err == nil {
				t.Fatal("wrong bundle", bundle)
			}
		}
	}
	for _, mode := range researchTestModes() {
		for _, gate := range []string{"useEMATrend", "useVolumeFilter"} {
			req := researchTestRequest(t, bs, mode)
			req.Config["adaptiveVolumeFlag"].(map[string]any)["rules"].(map[string]any)[gate] = false
			if _, err := RunAdaptiveVolumeFlag(req); err == nil {
				t.Fatal("hybrid accepted")
			}
		}
	}
}

func TestAdaptiveResearchNumericTruthTableAndBoundaries(t *testing.T) {
	r, _ := dsl.AdaptiveFlagPreset("SNAPSHOT_C")
	for _, mode := range researchTestModes() {
		req := researchTestRequest(t, adaptiveTestBars(1), mode)
		spec, _ := dsl.DecodeAdaptiveVolumeFlag(req.Config)
		p, err := adaptiveFlagResearchPolicy(mode, spec)
		if err != nil {
			t.Fatal(err)
		}
		for _, side := range []string{"long", "short"} {
			for _, retrace := range []float64{-1, 5, math.Nextafter(5, math.Inf(1))} {
				for _, width := range []float64{5.5, math.Nextafter(5.5, math.Inf(1))} {
					s := AdaptiveFlagSnapshot{AdaptiveFlagBar: adaptiveTestBar(0, 100, 101, 99, 100), ATR: adaptiveFlagPointer(1.), BullHeight: 10, BearHeight: 10, BullRetrace: adaptiveFlagPointer(retrace), BearRetrace: adaptiveFlagPointer(retrace), FlagWidth: width, FlagHigh: 101, FlagLow: 99, TrendBull: true, TrendBear: true, VolumeOK: true}
					if side == "long" {
						s.BullImpulse = true
					} else {
						s.BearImpulse = true
					}
					rows := []AdaptiveFlagSnapshot{s}
					adaptiveFlagResearchOverlay(rows, r, *p)
					want := (!p.RetraceCapEnabled || retrace <= 5) && (!p.WidthCapEnabled || width <= 5.5)
					if (rows[0].Candidate != nil) != want {
						t.Fatalf("%s %s retrace%g width%g", mode, side, retrace, width)
					}
					for _, missing := range []string{"impulse", "opposite-pivot", "trend", "volume"} {
						bad := s
						switch missing {
						case "impulse":
							bad.BullImpulse = false
							bad.BearImpulse = false
						case "opposite-pivot":
							bad.BullRetrace = nil
							bad.BearRetrace = nil
						case "trend":
							bad.TrendBull = false
							bad.TrendBear = false
						case "volume":
							bad.VolumeOK = false
						}
						rows[0] = bad
						adaptiveFlagResearchOverlay(rows, r, *p)
						if rows[0].Candidate != nil {
							t.Fatal("missing core/gate admitted", missing)
						}
					}
				}
			}
		}
	}
}

func TestAdaptiveResearchSourceFeatureInvarianceAndCausalPrefixes(t *testing.T) {
	bs := researchTestRetraceBars()
	base := researchTestRun(t, researchTestRequest(t, bs, ""))
	for _, mode := range researchTestModes() {
		out := researchTestRun(t, researchTestRequest(t, bs, mode))
		for i, s := range out.Snapshots {
			old := base.Snapshots[i]
			candidate := s.Candidate
			s.BullValid, s.BearValid, s.Candidate = old.BullValid, old.BearValid, old.Candidate
			if !reflect.DeepEqual(s, old) {
				t.Fatalf("raw feature changed %s row%d", mode, i)
			}
			if old.Candidate != nil && !reflect.DeepEqual(candidate, old.Candidate) {
				t.Fatal("common candidate geometry changed")
			}
			prefix := researchTestRun(t, researchTestRequest(t, bs[:i+1], mode))
			if !reflect.DeepEqual(prefix.Snapshots, out.Snapshots[:i+1]) || !reflect.DeepEqual(prefix.States, out.States[:i+1]) {
				t.Fatal("causal prefix changed", mode, i)
			}
		}
	}
}

func TestAdaptiveResearchWindowAndNonfiniteRefusal(t *testing.T) {
	for _, mode := range researchTestModes() {
		bs := researchTestRetraceBars()
		req := researchTestRequest(t, bs, mode)
		req.Window = &AdaptiveFlagExecutionWindow{TradeFromMS: 30 * 1800000, TradeToMS: 32 * 1800000}
		out := researchTestRun(t, req)
		if out.UsedSourceRows != 32 || out.PreTradeRows != 30 || out.IgnoredSuffixRows != 2 {
			t.Fatal("window counts")
		}
		for _, s := range out.States[:30] {
			if s.Status != "flat" {
				t.Fatal("pre-window occupancy")
			}
		}
		req.Series.H[32] = math.NaN()
		again := researchTestRun(t, req)
		if !reflect.DeepEqual(out, again) {
			t.Fatal("ignored suffix changed result")
		}
		req.Window.TradeFromMS = 0
		if out.ExecutionWindow.TradeFromMS != 30*1800000 {
			t.Fatal("window alias")
		}
		req = researchTestRequest(t, bs, mode)
		req.Series.H[25] = math.Inf(1)
		if _, err := RunAdaptiveVolumeFlag(req); err == nil {
			t.Fatal("nonfinite accepted")
		}
	}
}

func TestAdaptiveResearchSourceCoreExclusionsBothSides(t *testing.T) {
	stale := adaptiveTestBreakout()[:28]
	for len(stale) < 53 {
		stale = append(stale, adaptiveTestBar(len(stale), 107, 107.4, 106.4, 107))
	}
	small := adaptiveTestBars(20)
	for i := range small {
		small[i] = adaptiveTestBar(i, 100, 101, 99, 100)
	}
	for _, v := range [][4]float64{{100, 101, 98, 100}, {100, 101.1, 99, 100}, {100, 101.2, 99, 100}, {100, 101.3, 99, 100}, {100, 101.4, 99, 100}, {100, 101.3, 99, 100}, {100, 101.2, 99, 100}, {100, 101.1, 99, 100}} {
		small = append(small, adaptiveTestBar(len(small), v[0], v[1], v[2], v[3]))
	}
	cases := []struct {
		name string
		bars []AdaptiveFlagBar
		idx  int
	}{
		{"missing-ATR", adaptiveTestBars(5), 4}, {"missing-origin", adaptiveTestBars(20), 19},
		{"missing-opposite-with-impulse", adaptiveTestBreakout()[:27], 26},
		{"stale-origin", stale, 52}, {"wrong-order", adaptiveTestReflect(adaptiveTestBreakout()), 27}, {"below-minimum-pole", small, 27},
	}
	for _, f := range cases {
		for _, side := range []string{"long", "short"} {
			t.Run(f.name+"/"+side, func(t *testing.T) {
				bs := f.bars
				if side == "short" {
					bs = adaptiveTestReflect(bs)
				}
				base := researchTestRun(t, researchTestRequest(t, bs, ""))
				s := base.Snapshots[f.idx]
				origin, opposite, impulse, height, retrace := s.LastLow, s.LastHigh, s.BullImpulse, s.BullHeight, s.BullRetrace
				if side == "short" {
					origin, opposite, impulse, height, retrace = s.LastHigh, s.LastLow, s.BearImpulse, s.BearHeight, s.BearRetrace
				}
				switch f.name {
				case "missing-ATR":
					if s.ATR != nil {
						t.Fatal("fixture ATR")
					}
				case "missing-origin":
					if origin != nil {
						t.Fatal("fixture origin")
					}
				case "missing-opposite-with-impulse":
					if opposite != nil || !impulse || retrace != nil {
						t.Fatal("fixture source impulse/availability")
					}
				case "stale-origin":
					if origin == nil || f.idx-origin.Index <= 30 || impulse {
						t.Fatal("fixture recency")
					}
				case "wrong-order":
					if origin == nil || opposite == nil || opposite.Index > origin.Index || impulse {
						t.Fatal("fixture order")
					}
				case "below-minimum-pole":
					if origin == nil || opposite == nil || s.ATR == nil || height >= 1.8**s.ATR || impulse {
						t.Fatal("fixture pole")
					}
				}
				for _, mode := range researchTestModes() {
					out := researchTestRun(t, researchTestRequest(t, bs, mode))
					row := out.Snapshots[f.idx]
					valid := row.BullValid
					if side == "short" {
						valid = row.BearValid
					}
					if valid {
						t.Fatal("core admitted", mode)
					}
					if !reflect.DeepEqual(row.BullRetrace, s.BullRetrace) || !reflect.DeepEqual(row.BearRetrace, s.BearRetrace) {
						t.Fatal("raw availability changed")
					}
				}
			})
		}
	}
}

// The added source-backed G2 order remains occupied when C later obtains a
// valid quote. A post-filter of C orders cannot reproduce this state divergence.
func TestAdaptiveResearchSourceOccupancyDivergence(t *testing.T) {
	bs := adaptiveTestBreakout()[:29]
	bs[28] = adaptiveTestBar(28, 107, 120, 106, 119)
	bs = append(bs, adaptiveTestBar(29, 121, 125, 119, 124))
	for len(bs) < 37 {
		bs = append(bs, adaptiveTestBar(len(bs), 122, 123, 121, 122))
	}
	req := researchTestRequest(t, bs, "")
	req.Window = &AdaptiveFlagExecutionWindow{TradeFromMS: 28 * 1800000, TradeToMS: int64(len(bs)) * 1800000}
	base := researchTestRun(t, req)
	req.ResearchAblation = AdaptiveFlagResearchWidthCapOff
	out := researchTestRun(t, req)
	if len(base.Orders) == 0 || len(out.Orders) != 1 || out.Orders[0].SignalIdx != 28 || *out.Orders[0].FillIdx != 29 {
		t.Fatalf("occupancy fixture orders base%+v research%+v", base.Orders, out.Orders)
	}
	later := base.Orders[0].SignalIdx
	if later <= 29 || out.Snapshots[later].Candidate == nil || out.States[later].Status != "open" {
		t.Fatal("added order did not block later opportunity")
	}
	if !reflect.DeepEqual(base.Snapshots[later].Candidate, out.Snapshots[later].Candidate) {
		t.Fatal("common geometry changed")
	}
	if out.Orders[0].Entry == nil || *out.Orders[0].Entry != 121 || !out.Orders[0].EntryAtOpen || out.Orders[0].Trigger != out.Snapshots[28].Candidate.Trigger {
		t.Fatal("frozen levels or opening gap changed")
	}
	if out.Orders[0].Target != out.Snapshots[28].Candidate.Target || *out.Orders[0].BracketCreationIdx != 29 || out.Terminal.Status != "open" {
		t.Fatal("bracket/terminal contract")
	}
}

func TestAdaptiveResearchSourceDelayedActivationAndTerminalPrefixes(t *testing.T) {
	for _, mode := range researchTestModes() {
		t.Run(string(mode), func(t *testing.T) {
			bs := adaptiveTestBreakout()[:29]
			start := 28
			if mode == AdaptiveFlagResearchWidthCapOff {
				bs[28] = adaptiveTestBar(28, 107, 120, 106, 119)
			}
			if mode == AdaptiveFlagResearchRetraceCapOff {
				bs = researchTestRetraceBars()[:31]
				start = 30
			}
			req := researchTestRequest(t, bs, mode)
			req.Window = &AdaptiveFlagExecutionWindow{TradeFromMS: int64(start) * 1800000, TradeToMS: int64(start+1) * 1800000}
			pending := researchTestRun(t, req)
			if len(pending.Orders) != 1 || pending.Terminal.Status != "pending" {
				t.Fatal("terminal source pending")
			}
			sig := pending.Orders[0].AdaptiveFlagSignal
			bs = append(bs, adaptiveTestBar(len(bs), sig.Trigger-1, sig.Trigger+1, sig.Stop-2, sig.Stop-1))
			req = researchTestRequest(t, bs, mode)
			req.Window = &AdaptiveFlagExecutionWindow{TradeFromMS: int64(start) * 1800000, TradeToMS: int64(len(bs)) * 1800000}
			queued := researchTestRun(t, req)
			if queued.Terminal.Status != "queued-exit" || queued.Terminal.QueuedExit.Reason != "stop-activated-at-close" || queued.Orders[0].Exit != nil || *queued.Orders[0].BracketCreationIdx != start+1 {
				t.Fatalf("delayed activated stop %+v", queued.Terminal)
			}
			recovered := sig.Stop + 2
			bar := adaptiveTestBar(len(bs), recovered, recovered+1, recovered-1, recovered)
			bar.OpenT += 5 * 1800000
			bar.CloseT += 5 * 1800000
			bs = append(bs, bar)
			req = researchTestRequest(t, bs, mode)
			req.Window = &AdaptiveFlagExecutionWindow{TradeFromMS: int64(start) * 1800000, TradeToMS: bar.CloseT}
			out := researchTestRun(t, req)
			if *out.Orders[0].Exit != recovered || out.Orders[0].Reason != "stop-activated-at-close" || *out.Orders[0].ExitIdx != start+2 || len(out.Gaps) != 1 {
				t.Fatal("recovery/gap canceled queued stop")
			}
			if out.Orders[0].Trigger != sig.Trigger || out.Orders[0].Stop != sig.Stop || out.Orders[0].Target != sig.Target {
				t.Fatal("pending geometry mutated")
			}
		})
	}
}

func TestAdaptiveResearchSourceExpiryAndSameCloseRearm(t *testing.T) {
	for _, mode := range researchTestModes() {
		bs := adaptiveTestBreakout()[:29]
		start := 28
		if mode == AdaptiveFlagResearchWidthCapOff {
			bs[28] = adaptiveTestBar(28, 107, 120, 106, 119)
		}
		if mode == AdaptiveFlagResearchRetraceCapOff {
			bs = researchTestRetraceBars()[:31]
			start = 30
		}
		for len(bs) <= start+21 {
			if mode == AdaptiveFlagResearchRetraceCapOff {
				bs = append(bs, adaptiveTestBar(len(bs), 115, 116, 110, 115))
			} else {
				high := 119.8
				if len(bs) == 35 {
					high = 119.9
				}
				bs = append(bs, adaptiveTestBar(len(bs), 119, high, 118, 119))
			}
		}
		req := researchTestRequest(t, bs, mode)
		req.Window = &AdaptiveFlagExecutionWindow{TradeFromMS: int64(start) * 1800000, TradeToMS: int64(len(bs)) * 1800000}
		out := researchTestRun(t, req)
		if len(out.Orders) == 0 || out.Orders[0].Status != "expired" || out.Orders[0].FillOpportunities != 21 || out.Orders[0].PendingAge != 21 {
			t.Fatalf("expiry %s %+v", mode, out.Orders)
		}
		if mode != AdaptiveFlagResearchRetraceCapOff && (len(out.Orders) != 2 || out.Orders[1].SignalIdx != start+21 || out.Terminal.Status != "pending") {
			t.Fatalf("same-close rearm %s %+v", mode, out.Orders)
		}
	}
}

func TestAdaptiveResearchSourceHoldCountsObservedBars(t *testing.T) {
	bs := researchTestRetraceBars()[:32]
	for len(bs) <= 92 {
		bar := adaptiveTestBar(len(bs), 117, 118, 116, 117)
		if len(bs) >= 50 {
			bar.OpenT += 7 * 1800000
			bar.CloseT += 7 * 1800000
		}
		bs = append(bs, bar)
	}
	for _, mode := range []AdaptiveFlagResearchAblation{AdaptiveFlagResearchRetraceCapOff, AdaptiveFlagResearchBothCapsOff} {
		req := researchTestRequest(t, bs, mode)
		req.Window = &AdaptiveFlagExecutionWindow{TradeFromMS: 30 * 1800000, TradeToMS: bs[len(bs)-1].CloseT}
		out := researchTestRun(t, req)
		if len(out.Orders) == 0 || *out.Orders[0].FillIdx != 31 || *out.Orders[0].ExitIdx != 92 || out.Orders[0].Reason != "max-hold" || len(out.Gaps) != 1 {
			t.Fatal("hold observed-bar clock changed")
		}
		req.Series = adaptiveTestSeries(bs[:92])
		req.Window.TradeToMS = bs[91].CloseT
		queued := researchTestRun(t, req)
		if queued.Terminal.Status != "queued-exit" || queued.Terminal.QueuedExit.CreationIdx != 91 {
			t.Fatal("terminal hold queue")
		}
	}
}

func TestAdaptiveResearchPinnedCanonicalPolicyHashes(t *testing.T) {
	expected := []string{"18b228b6424a901cabbc416091bb519ff170eea3b61aacf633858b85443614ff", "7a15d81aed107c354caa8f92a1329abcf4a454a29f22dda263249937e4ff296d", "3da4755e85245bc3b6091d3c8df1d20fbfc4014dec33180e2457f1d03a0b61e3"}
	for i, mode := range researchTestModes() {
		out := researchTestRun(t, researchTestRequest(t, adaptiveTestBars(1), mode))
		if out.ResearchPolicySHA256 != expected[i] || out.ConfigSHA256 != "5d467c58497b6115961f0db0267f135a2ba8be764bfa8a0a2d6bf193619735ee" {
			t.Fatal("frozen compact policy/config encoding changed", mode, out.ResearchPolicySHA256)
		}
	}
}
