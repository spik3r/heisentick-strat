package dsl

import (
	"reflect"
	"strings"
	"testing"
)

// clockGateProgram is the Asia range colour breakout shape: UTC clock, range
// 00:00-07:00, orders until 13:00, close 20:45, both gates, range-edge stop and
// a 4R target.
func clockGateProgram(mutate func(string) string) string {
	source := `dsl v7
strategy "Clock Range Gates" {
  description "synthetic gate example"
}
market conditions {
  slices(XAUUSD 15m)
  clock UTC+0
}
setup {
  type: clock range breakout
  range 00:00 to 07:00
  orders expire 13:00
  range width at least 0.42 daily ATR 14
  trade with prior day candle
  cancel when opposite side triggers first
}
risk {
  stop beyond opposite range edge
}
target {
  target 4R
}
management {
  close positions at 20:45
}
execution {
  risk: 200 USD
}
`
	if mutate != nil {
		source = mutate(source)
	}
	return source
}

func replaceText(old, new string) func(string) string {
	return func(source string) string {
		if !strings.Contains(source, old) {
			panic("source lacks " + old)
		}
		return strings.Replace(source, old, new, 1)
	}
}

// Minutes after midnight: 07:00 = 420, 13:00 = 780, 20:45 = 1245.
func TestClockRangeGatesCompileToFamilyConfig(t *testing.T) {
	result := parseClock(t, clockGateProgram(nil))
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %q", result.Errors)
	}
	family := result.Config["clockRangeBreakout"].(map[string]any)
	want := map[string]any{
		"rangeStartMinute": 0, "rangeEndMinute": 420, "expireMinute": 780, "closeMinute": 1245, "bufferPips": float64(0),
		"widthMinAtr": 0.42, "widthAtrDays": 14, "priorDayCandle": 1, "cancelOnOpposite": 1, "targetR": float64(4),
	}
	if !reflect.DeepEqual(family, want) {
		t.Errorf("family = %#v\nwant %#v", family, want)
	}
	stop := result.Config["stop"].(map[string]any)
	if stop["type"] != ClockStopOppositeRangeEdge {
		t.Errorf("stop = %#v", stop)
	}
	if _, present := stop["percent"]; present {
		t.Errorf("range-edge stop carries a percent: %#v", stop)
	}
	spec, gates, err := DecodeClockRangeBreakoutGates(result.Config)
	if err != nil {
		t.Fatal(err)
	}
	wantSpec := ClockRangeSpec{RangeEndMinute: 420, ExpireMinute: 780, CloseMinute: 1245, AllowLong: true, AllowShort: true}
	if spec != wantSpec {
		t.Errorf("spec = %+v\nwant %+v", spec, wantSpec)
	}
	wantGates := ClockRangeGates{StopMode: ClockStopOppositeRangeEdge, WidthMinATR: 0.42, WidthATRDays: 14, PriorDayCandle: true, CancelOnOpposite: true, TargetR: 4}
	if gates != wantGates {
		t.Errorf("gates = %+v\nwant %+v", gates, wantGates)
	}
}

// A v1 program decodes with every gate off, so it keeps the v1 behaviour.
func TestClockRangeV1ProgramHasNoGates(t *testing.T) {
	result := parseClock(t, clockExample(nil))
	family := result.Config["clockRangeBreakout"].(map[string]any)
	for _, key := range []string{"widthMinAtr", "widthAtrDays", "priorDayCandle", "cancelOnOpposite", "targetR"} {
		if _, present := family[key]; present {
			t.Errorf("v1 config gained %s", key)
		}
	}
	_, gates, err := DecodeClockRangeBreakoutGates(result.Config)
	if err != nil {
		t.Fatal(err)
	}
	if gates != (ClockRangeGates{StopMode: ClockStopPercent}) {
		t.Errorf("v1 gates = %+v", gates)
	}
}

func TestClockRangeRejectsMixedStopsInDirectConfig(t *testing.T) {
	result := parseClock(t, clockGateProgram(nil))
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %q", result.Errors)
	}
	stop := result.Config["stop"].(map[string]any)
	stop["percent"] = 1.0
	if _, _, err := DecodeClockRangeBreakoutGates(result.Config); err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("mixed stops error = %v", err)
	}
}

func TestClockRangeGatesAreIndependent(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string) string
	}{
		{"width only", func(s string) string {
			s = strings.Replace(s, "  trade with prior day candle\n", "", 1)
			return strings.Replace(s, "  cancel when opposite side triggers first\n", "", 1)
		}},
		{"prior day without cancel", replaceText("  cancel when opposite side triggers first\n", "")},
		{"no target", replaceText("  target 4R\n", "")},
		{"percent stop with gates and no target", func(s string) string {
			s = strings.Replace(s, "  target 4R\n", "", 1)
			return strings.Replace(s, "stop beyond opposite range edge", "stop 0.5 percent", 1)
		}},
		{"fractional target", replaceText("target 4R", "target 2.5R")},
		{"lower case atr", replaceText("daily ATR 14", "daily atr 14")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := parseClock(t, clockGateProgram(test.mutate))
			if len(result.Errors) != 0 {
				t.Fatalf("errors = %q", result.Errors)
			}
			if _, err := DecodeClockRangeBreakout(result.Config); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClockRangeGateRejections(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string) string
		want   string
	}{
		{"cancel without prior day", replaceText("  trade with prior day candle\n", ""), "requires trade with prior day candle"},
		{"prior day with static side", replaceText("  stop beyond opposite range edge\n", "  stop beyond opposite range edge\n}\nfilters {\n  side long only\n"), "remove side long only"},
		{"both stops", replaceText("  stop beyond opposite range edge\n", "  stop beyond opposite range edge\n  stop 1 percent\n"), "not both"},
		{"duplicate edge stop", replaceText("  stop beyond opposite range edge\n", "  stop beyond opposite range edge\n  stop beyond opposite range edge\n"), "duplicate stop beyond opposite range edge"},
		{"width without days", replaceText("daily ATR 14", "daily ATR"), "range width must be written"},
		{"width misspelt", replaceText("at least 0.42", "above 0.42"), "range width must be written"},
		{"zero width multiple", replaceText("at least 0.42", "at least 0"), "must be a positive number"},
		{"negative width multiple", replaceText("at least 0.42", "at least -1"), "must be a positive number"},
		{"fractional ATR days", replaceText("daily ATR 14", "daily ATR 1.5"), "whole number from 1 to 1000"},
		{"zero ATR days", replaceText("daily ATR 14", "daily ATR 0"), "whole number from 1 to 1000"},
		{"too many ATR days", replaceText("daily ATR 14", "daily ATR 1001"), "whole number from 1 to 1000"},
		{"duplicate width", replaceText("  range width at least 0.42 daily ATR 14\n", "  range width at least 0.42 daily ATR 14\n  range width at least 0.5 daily ATR 14\n"), "duplicate range width"},
		{"prior day misspelt", replaceText("trade with prior day candle", "trade with prior candle"), "write exactly trade with prior day candle"},
		{"cancel misspelt", replaceText("cancel when opposite side triggers first", "cancel when opposite triggers"), "write exactly cancel when opposite side triggers first"},
		{"zero target", replaceText("target 4R", "target 0R"), "positive multiple of R"},
		{"trade window still forbidden", replaceText("  trade with prior day candle\n", "  trade with prior day candle\n  trade window unrestricted\n"), `does not support directive "trade"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := parseClock(t, clockGateProgram(test.mutate))
			if !hasError(result, test.want) {
				t.Fatalf("errors = %q, want %q", result.Errors, test.want)
			}
		})
	}
}

// With the v1 percent stop a target stays outside the family, with the v1
// message and no target config.
func TestClockRangeTargetNeedsRangeEdgeStop(t *testing.T) {
	result := parseClock(t, clockExample(replaceInBlock("risk", "  stop 1 percent\n", "  stop 1 percent\n  target 2R\n")))
	if !hasError(result, `does not support directive "target"`) {
		t.Fatalf("errors = %q", result.Errors)
	}
	if _, present := result.Config["clockRangeBreakout"].(map[string]any)["targetR"]; present {
		t.Error("rejected target reached the config")
	}
}

func TestClockRangeGateDirectivesRequireTheFamily(t *testing.T) {
	for _, line := range []string{"range width at least 0.42 daily ATR 14", "trade with prior day candle", "cancel when opposite side triggers first"} {
		source := "dsl v7\nstrategy \"orb\" {\n}\nsetup {\n  type: opening range breakout\n  " + line + "\n}\n"
		if result := parseClock(t, source); !hasError(result, "require type: clock range breakout") {
			t.Errorf("%s: errors = %q", line, result.Errors)
		}
	}
}

// The shared inline splitter breaks "stop beyond opposite range edge" at
// "range", so the inline form fails closed instead of compiling a different
// rule. Write the gate phrases one per line.
func TestClockRangeEdgeStopInlineFailsClosed(t *testing.T) {
	source := clockGateProgram(replaceText("risk {\n  stop beyond opposite range edge\n}\n", "risk { stop beyond opposite range edge }\n"))
	if result := parseClock(t, source); len(result.Errors) == 0 {
		t.Fatalf("inline range-edge stop compiled: %#v", result.Config["stop"])
	}
}

func TestClockRangeSpecValidateGates(t *testing.T) {
	spec := ClockRangeSpec{RangeEndMinute: 420, ExpireMinute: 780, CloseMinute: 1245, AllowLong: true, AllowShort: true}
	base := ClockRangeGates{StopMode: ClockStopOppositeRangeEdge}
	if err := ValidateClockRange(spec, base); err != nil {
		t.Fatal(err)
	}
	if err := spec.Validate(); err == nil {
		t.Error("v1 Validate accepted a spec without a stop percent")
	}
	tests := []struct {
		name   string
		mutate func(*ClockRangeSpec, *ClockRangeGates)
		want   string
	}{
		{"unknown stop mode", func(_ *ClockRangeSpec, g *ClockRangeGates) { g.StopMode = "atr" }, "stop must be"},
		{"edge stop with percent", func(s *ClockRangeSpec, _ *ClockRangeGates) { s.StopPercent = 1 }, "not both"},
		{"percent stop with target", func(s *ClockRangeSpec, g *ClockRangeGates) {
			g.StopMode, s.StopPercent, g.TargetR = ClockStopPercent, 1, 2
		}, "requires stop beyond opposite range edge"},
		{"percent stop without percent", func(_ *ClockRangeSpec, g *ClockRangeGates) { g.StopMode = ClockStopPercent }, "stop percent"},
		{"negative target", func(_ *ClockRangeSpec, g *ClockRangeGates) { g.TargetR = -1 }, "target R"},
		{"width days without multiple", func(_ *ClockRangeSpec, g *ClockRangeGates) { g.WidthATRDays = 14 }, "needs both"},
		{"width multiple without days", func(_ *ClockRangeSpec, g *ClockRangeGates) { g.WidthMinATR = 0.4 }, "needs both"},
		{"cancel without prior day", func(_ *ClockRangeSpec, g *ClockRangeGates) { g.CancelOnOpposite = true }, "requires trade with prior day candle"},
		{"prior day with one side", func(s *ClockRangeSpec, g *ClockRangeGates) { g.PriorDayCandle, s.AllowShort = true, false }, "chooses the side itself"},
	}
	for _, test := range tests {
		s, g := spec, base
		test.mutate(&s, &g)
		if err := ValidateClockRange(s, g); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%s: err = %v, want %q", test.name, err, test.want)
		}
	}
}
