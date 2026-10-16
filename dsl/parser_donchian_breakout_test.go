package dsl

import (
	"encoding/json"
	"math"
	"strconv"
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

func TestDonchianBreakoutParserCheckedCounts(t *testing.T) {
	upper := math.Ldexp(1, strconv.IntSize-1)
	below := math.Floor(math.Nextafter(upper, 0))
	tests := []struct {
		name  string
		token string
		want  float64
		valid bool
	}{
		{"integer", "3", 3, true},
		{"decimal integer", "3.0", 3, true},
		{"exponent integer", "3e0", 3, true},
		{"largest representable integral count", strconv.FormatFloat(below, 'f', 0, 64), below, true},
		{"exclusive int bound", strconv.FormatFloat(upper, 'f', 0, 64), 0, false},
		{"beyond int range", "1e20", 0, false},
		{"zero", "0", 0, false},
		{"negative", "-3", 0, false},
		{"fraction", "3.5", 0, false},
		{"fractional exponent", "3e-1", 0, false},
		{"R suffix", "3R", 0, false},
		{"percent suffix", "3%", 0, false},
		{"nan", "NaN", 0, false},
		{"infinity", "Inf", 0, false},
		{"overflow", "1e309", 0, false},
		{"underflow", "1e-400", 0, false},
		{"malformed", "3..0", 0, false},
	}
	for _, field := range []struct {
		name, from, prefix, suffix, key, message string
	}{
		{"channel", "channel 3 candles", "channel ", " candles", "channelBars", "donchian-breakout channel must be exactly"},
		{"trend", "trend ema 5", "trend ema ", "", "trendEmaLen", "donchian-breakout trend gate must be exactly"},
	} {
		for _, test := range tests {
			t.Run(field.name+"/"+test.name, func(t *testing.T) {
				source := strings.Replace(validDonchianBreakoutSource, field.from, field.prefix+test.token+field.suffix, 1)
				result, err := Parse(source)
				if err != nil {
					t.Fatalf("Parse: %v", err)
				}
				if test.valid {
					if len(result.Errors) != 0 {
						t.Fatalf("errors = %v", result.Errors)
					}
					if got := copyMap(result.Config["donchianBreakout"])[field.key]; got != test.want {
						t.Fatalf("%s = %v, want %v", field.key, got, test.want)
					}
				} else {
					if !containsSubstring(result.Errors, field.message) {
						t.Fatalf("errors = %v, want %q", result.Errors, field.message)
					}
					if _, exists := copyMap(result.Config["donchianBreakout"])[field.key]; exists {
						t.Fatalf("invalid count stored in config: %#v", result.Config["donchianBreakout"])
					}
				}
			})
		}
	}
}

func TestDonchianBreakoutParserTargetNumbers(t *testing.T) {
	tests := []struct {
		name, token string
		want        float64
		valid       bool
	}{
		{"positive integer", "1R", 1, true},
		{"trailing dot", "1.R", 1, true},
		{"leading dot", ".5R", 0.5, true},
		{"lowercase suffix", "2.5r", 2.5, true},
		{"zero", "0R", 0, true},
		{"decimal zero", "00.00R", 0, true},
		{"leading dot zero", ".0R", 0, true},
		{"trailing dot zero", "0.R", 0, true},
		{"positive subnormal", "0." + strings.Repeat("0", 323) + "5R", math.SmallestNonzeroFloat64, true},
		{"dot only", ".R", 0, false},
		{"multiple dots", "1..2R", 0, false},
		{"negative", "-1R", 0, false},
		{"positive sign", "+1R", 0, false},
		{"exponent", "1e3R", 0, false},
		{"nan", "NaNR", 0, false},
		{"infinity", "InfR", 0, false},
		{"overflow", strings.Repeat("9", 309) + "R", 0, false},
		{"nonzero decimal underflow", "0." + strings.Repeat("0", 400) + "1R", 0, false},
		{"missing suffix", "1", 0, false},
		{"extra token", "1R extra", 0, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := donchianParserInsertBlock(validDonchianBreakoutSource, "target", "target "+test.token, false)
			result, err := Parse(source)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if test.valid {
				if len(result.Errors) != 0 {
					t.Fatalf("errors = %v", result.Errors)
				}
				if got := copyMap(result.Config["target"])["dcbR"]; got != test.want {
					t.Fatalf("dcbR = %v, want %v", got, test.want)
				}
			} else if !containsSubstring(result.Errors, "only target NR") {
				t.Fatalf("errors = %v, want target refusal", result.Errors)
			}
		})
	}
}

func TestDonchianBreakoutParserInvalidStopsStayFinite(t *testing.T) {
	for _, token := range []string{"0", "-1", "NaN", "Inf", "-Inf", "+Inf", "Infinity", "1e309", "1e-400", "2R", "2%", "bogus"} {
		for _, before := range []bool{false, true} {
			for _, followedByValid := range []bool{false, true} {
				t.Run(token+"/before="+strconv.FormatBool(before)+"/thenValid="+strconv.FormatBool(followedByValid), func(t *testing.T) {
					source := strings.Replace(validDonchianBreakoutSource, "  stop 2 ATR\n", "", 1)
					source = donchianParserInsertBlock(source, "risk", "stop "+token+" ATR", before)
					if followedByValid {
						source = donchianParserInsertBlock(source, "risk", "stop 3 ATR", false)
					}
					result, err := Parse(source)
					if err != nil {
						t.Fatalf("Parse: %v", err)
					}
					if !containsSubstring(result.Errors, "only a fixed ATR stop") {
						t.Fatalf("errors = %v, want stop refusal", result.Errors)
					}
					if _, err := json.Marshal(result.Config); err != nil {
						t.Fatalf("refused config must stay JSON serializable: %v; stop = %#v", err, result.Config["stop"])
					}
					if followedByValid && copyMap(result.Config["stop"])["atrMult"] != float64(3) {
						t.Fatalf("final valid stop was not retained: %#v", result.Config["stop"])
					}
				})
			}
		}
	}
}

func TestDonchianBreakoutParserFiniteStops(t *testing.T) {
	for _, test := range []struct {
		token string
		want  float64
	}{
		{"2", 2}, {"2.", 2}, {".5", 0.5}, {"1e2", 100}, {"1e-100", 1e-100},
	} {
		t.Run(test.token, func(t *testing.T) {
			result, err := Parse(strings.Replace(validDonchianBreakoutSource, "stop 2 ATR", "stop "+test.token+" ATR", 1))
			if err != nil || len(result.Errors) != 0 {
				t.Fatalf("Parse: %v %v", err, result.Errors)
			}
			if got := copyMap(result.Config["stop"])["atrMult"]; got != test.want {
				t.Fatalf("atrMult = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDonchianBreakoutParserStickyAuthoredRefusals(t *testing.T) {
	tests := []struct {
		name, section, body, message string
	}{
		{"market entry", "entry", "enter at market", "enter phrases"},
		{"close entry", "entry", "enter at close", "enter phrases"},
		{"next open entry", "entry", "enter at next open", "enter phrases"},
		{"empty entry", "entry", "enter", "enter phrases"},
		{"malformed entry", "entry", "enter nonsense", "enter phrases"},
		{"limit entry", "entry", "enter with limit at swept edge within 3 candles", "enter phrases"},
		{"limit then market", "entry", "enter with limit at swept edge within 3 candles\n  enter at market", "enter phrases"},
		{"invalid then valid target", "target", "target .R\n  target 0R", "only target NR"},
		{"underflow then valid target", "target", "target 0." + strings.Repeat("0", 400) + "1R\n  target 0R", "only target NR"},
		{"target outside target section", "risk", "target 1..2R", "only target NR"},
	}
	for _, test := range tests {
		for _, before := range []bool{false, true} {
			t.Run(test.name+"/before="+strconv.FormatBool(before), func(t *testing.T) {
				source := donchianParserInsertBlock(validDonchianBreakoutSource, test.section, test.body, before)
				result, err := Parse(source)
				if err != nil {
					t.Fatalf("Parse: %v", err)
				}
				if !containsSubstring(result.Errors, test.message) {
					t.Fatalf("errors = %v, want %q", result.Errors, test.message)
				}
			})
		}
	}
}

func TestDonchianBreakoutParserCountRefusalsSurviveValidLines(t *testing.T) {
	for _, test := range []struct {
		from, to, message string
	}{
		{"channel 3 candles", "channel 1e20 candles\n  channel 3 candles", "donchian-breakout channel must be exactly"},
		{"trend ema 5", "trend ema 1e20\n  trend ema 5", "donchian-breakout trend gate must be exactly"},
	} {
		result, err := Parse(strings.Replace(validDonchianBreakoutSource, test.from, test.to, 1))
		if err != nil || !containsSubstring(result.Errors, test.message) {
			t.Fatalf("Parse: %v %v, want %q", err, result.Errors, test.message)
		}
	}
}

func donchianParserInsertBlock(source, section, body string, beforeFamily bool) string {
	anchor := "execution {"
	if beforeFamily {
		anchor = "setup {"
	}
	return strings.Replace(source, anchor, section+" {\n  "+body+"\n}\n"+anchor, 1)
}
