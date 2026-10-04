package dsl

import (
	"os"
	"strings"
	"testing"
)

func timedSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../engine/testdata/timed-return/valid.strat")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTimedReturnStrictGrammar(t *testing.T) {
	source := timedSource(t)
	r, err := Parse(source)
	if err != nil || len(r.Errors) > 0 {
		t.Fatalf("%v %v", err, r.Errors)
	}
	spec, err := DecodeTimedReturn(r.Config)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Start.Price != "open" || spec.End.Price != "close" || spec.Quantity != 1 {
		t.Fatalf("%+v", spec)
	}
	for _, change := range []struct{ old, new string }{
		{"dsl v7", "dsl v6"}, {"timed timezone UTC", "timed timezone Europe/London"},
		{"timed entry 10:00", "timed entry 10:00 extra"}, {"timed exit 11:00", "timed exit 09:00"},
		{"timed quantity 1 unit", "timed quantity 2 unit"}, {"timed execution delayed-open", "timed execution close"},
		{"timed calendar required", "timed calendar optional"}, {"current clock 10:00 completed-close", "current clock 10:00"},
		{"current session open", "current session close open"}, {"current clock 10:00 completed-close", "previous clock 10:00 completed-close"},
		{"timed direction with return", "timed direction fixed long"}, {"timed entry 10:00", "timed entry 10:00\n timed entry 10:01"},
		{"slices(SYNTHUSD 1m)", "slices(SYNTHUSD 1m OTHERUSD 1m)"},
		{"timed entry 10:00", "timed entry 25:00"}, {"timed calendar required\n", ""},
	} {
		p, _ := Parse(strings.Replace(source, change.old, change.new, 1))
		if len(p.Errors) == 0 {
			t.Errorf("accepted %q", change.new)
		}
	}
}

func TestTimedReturnRejectsAuthoredAndRuntimeFallbacks(t *testing.T) {
	source := timedSource(t)
	for _, extra := range []string{"setup {\n type: price momentum\n}\n", "entry at 10:00", "stop 2 ATR", "risk 200 USD", "new york hour in (10)", "rebalance monthly", "source timeframe 4h", "sessions(ny)", "maxHoldCandles 30", "trade window unrestricted"} {
		r, _ := Parse(source + extra + "\n")
		if len(r.Errors) == 0 {
			t.Errorf("accepted unsupported %s", extra)
		}
	}
	for _, side := range []string{"long only", "short only", "both"} {
		r, _ := Parse(source + "filters {\n side " + side + "\n}\n")
		if len(r.Errors) > 0 {
			t.Fatalf("side %s: %v", side, r.Errors)
		}
	}
	for _, side := range []string{"long perhaps", "banana", "both extra"} {
		r, _ := Parse(source + "filters {\n side " + side + "\n}\n")
		if len(r.Errors) == 0 {
			t.Fatalf("accepted side %s", side)
		}
	}
	for key, value := range map[string]any{"riskUsd": 3, "sourceTimeframe": "1h", "allowLong": "1", "allowShort": true, "injected": true} {
		r, _ := Parse(source)
		r.Config[key] = value
		if _, err := DecodeTimedReturn(r.Config); err == nil {
			t.Errorf("accepted hand-built %s", key)
		}
	}
}

func TestTimedReturnConsumesRawPhysicalSource(t *testing.T) {
	source := timedSource(t)
	for _, suffix := range []string{
		"filters { side both } risk 999 USD",
		"filters { side both } timed quantity 2 unit",
		"filters { ignored condition side both }",
		"filters { side both }",
		"filters {\n side both\n} risk 999 USD",
		"filters {\n side both\n",
		"}",
		"strategy \"ignored trailing condition\" risk 999 USD",
		"strategy {\n description \"note\" risk 999 USD\n}",
		"filters {\n filters {\n side both\n}\n}",
	} {
		r, _ := Parse(source + suffix + "\n")
		if len(r.Errors) == 0 {
			t.Errorf("silently accepted raw suffix %q", suffix)
		}
	}
	for _, replacement := range []string{"dsl v7 risk 999 USD", "dsl v7 timed quantity 2 unit", "dsl v7 ignored"} {
		r, _ := Parse(strings.Replace(source, "dsl v7", replacement, 1))
		if len(r.Errors) == 0 {
			t.Errorf("version header ignored %q", replacement)
		}
	}
	for _, replacement := range []string{"type: timed return risk 999 USD", "type: timed return timed quantity 2 unit"} {
		r, _ := Parse(strings.Replace(source, "type: timed return", replacement, 1))
		if len(r.Errors) == 0 {
			t.Errorf("type header ignored %q", replacement)
		}
	}
	// Normal multiline syntax and actual comment suffixes remain accepted.
	r, _ := Parse(source + "filters { # comment\n side both # comment\n} # comment\n")
	if len(r.Errors) != 0 {
		t.Fatalf("valid multiline source: %v", r.Errors)
	}
}
