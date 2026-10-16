package dsl

import (
	"strings"
	"testing"
)

const validDonchianBreakoutSource = `dsl v7
strategy "Donchian Breakout Parser" {
  description "Exercise the donchian breakout family."
}
market conditions {
  slices(XAUUSD 4h)
}
setup {
  type: donchian breakout
  channel 3 candles
  trend ema 5
}
filters {
  side: long only
}
risk {
  stop 2 ATR
}
management {
  trail 3 ATR after 0R
}
execution {
  risk 200 USD
}
`

func TestDonchianBreakoutParserDefaults(t *testing.T) {
	result, err := Parse(validDonchianBreakoutSource)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v", result.Errors)
	}
	cfg := result.Config
	if cfg["setupType"] != string(FamilyDonchianBreakout) {
		t.Fatalf("setupType = %v", cfg["setupType"])
	}
	family := copyMap(cfg["donchianBreakout"])
	if family["channelBars"] != float64(3) || family["trendEmaLen"] != float64(5) {
		t.Fatalf("donchianBreakout = %#v", family)
	}
	stop := copyMap(cfg["stop"])
	if stop["type"] != "atr" || stop["atrMult"] != float64(2) {
		t.Fatalf("stop = %#v", stop)
	}
	if be := copyMap(cfg["breakeven"]); be["atR"] != 0 {
		t.Fatalf("breakeven = %#v", be)
	}
	if trail := copyMap(cfg["trail"]); trail["atr"] != float64(3) || trail["triggerR"] != float64(0) {
		t.Fatalf("trail = %#v", trail)
	}
	if cfg["cooldownCandles"] != 0 || cfg["tradeWindowMode"] != "unrestricted" || cfg["maxMovementEr"] != 1 {
		t.Fatalf("cooldown/window/er = %v %v %v", cfg["cooldownCandles"], cfg["tradeWindowMode"], cfg["maxMovementEr"])
	}
	if got := cfg["dayTypes"].([]any); len(got) != 3 {
		t.Fatalf("dayTypes = %v", got)
	}
	if _, ok := copyMap(cfg["target"])["dcbR"]; ok {
		t.Fatalf("target has dcbR without an authored target: %#v", cfg["target"])
	}
}

func TestDonchianBreakoutParserTargetKey(t *testing.T) {
	source := strings.Replace(validDonchianBreakoutSource, "execution {", "target {\n  target 50R\n}\nexecution {", 1)
	result, err := Parse(source)
	if err != nil || len(result.Errors) != 0 {
		t.Fatalf("Parse: %v %v", err, result.Errors)
	}
	if got := copyMap(result.Config["target"])["dcbR"]; got != float64(50) {
		t.Fatalf("dcbR = %v", got)
	}
}

func TestDonchianBreakoutParserRefusals(t *testing.T) {
	tests := []struct {
		name    string
		from    string
		to      string
		message string
	}{
		{"zero channel", "channel 3 candles", "channel 0 candles", "positive integer"},
		{"channel bars", "channel 3 candles", "channel 3 bars", "channel N candles"},
		{"fractional ema", "trend ema 5", "trend ema 2.5", "positive integer"},
		{"sma trend", "trend ema 5", "trend sma 5", "trend ema N"},
		{"extreme stop", "stop 2 ATR", "stop beyond last 3 candle extreme by 0.25 ATR", "stop N ATR"},
		{"stop size", "stop 2 ATR", "stop 2 ATR\n  stop size min 0.5 max 3", "stop N ATR"},
		{"other target", "execution {", "target {\n  minimum reward 2\n}\nexecution {", "only target NR"},
		{"limit entry", "execution {", "entry {\n  enter with limit at swept edge within 3 candles\n}\nexecution {", "enter phrases"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := Parse(strings.Replace(validDonchianBreakoutSource, test.from, test.to, 1))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if !containsSubstring(result.Errors, test.message) {
				t.Fatalf("errors = %v, want %q", result.Errors, test.message)
			}
		})
	}
	result, err := Parse("dsl v7\nsetup {\n  type: donchian breakout\n}\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !containsSubstring(result.Errors, "channel N candles") {
		t.Fatalf("errors = %v", result.Errors)
	}
}

func TestDonchianBreakoutParserKeepsAuthoredGates(t *testing.T) {
	source := strings.Replace(validDonchianBreakoutSource, "  slices(XAUUSD 4h)\n",
		"  slices(XAUUSD 4h)\n  sessions(london, ny)\n  day type in (trending)\n  movement below 0.7\n", 1)
	result, err := Parse(source)
	if err != nil || len(result.Errors) != 0 {
		t.Fatalf("Parse: %v %v", err, result.Errors)
	}
	cfg := result.Config
	if _, ok := cfg["tradeWindowMode"]; ok {
		t.Fatalf("tradeWindowMode = %v, want unset", cfg["tradeWindowMode"])
	}
	if got := cfg["dayTypes"].([]any); len(got) != 1 || got[0] != "trending" {
		t.Fatalf("dayTypes = %v", got)
	}
	if cfg["maxMovementEr"] != 0.7 {
		t.Fatalf("maxMovementEr = %v", cfg["maxMovementEr"])
	}
}

func containsSubstring(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}
