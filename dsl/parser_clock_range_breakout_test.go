package dsl

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Blocks of the spec example, kept separate so tests can reorder them.
var clockExampleBlocks = map[string]string{
	"strategy":   "strategy \"Clock Range Breakout - XAUUSD\" {\n  description \"synthetic example\"\n}\n",
	"market":     "market conditions {\n  slices(XAUUSD 5m)\n  clock UTC+10\n}\n",
	"setup":      "setup {\n  type: clock range breakout\n  range 11:05 to 14:05\n  orders expire 03:00\n  buffer 0 pips\n}\n",
	"risk":       "risk {\n  stop 1 percent\n}\n",
	"management": "management {\n  close positions at 03:00\n}\n",
	"execution":  "execution {\n  risk: 200 USD\n}\n",
}

var clockBlockOrder = []string{"strategy", "market", "setup", "risk", "management", "execution"}

func clockProgram(order []string, mutate func(name, block string) string) string {
	var b strings.Builder
	b.WriteString("dsl v7\n")
	for _, name := range order {
		block := clockExampleBlocks[name]
		if mutate != nil {
			block = mutate(name, block)
		}
		b.WriteString(block)
	}
	return b.String()
}

func clockExample(mutate func(name, block string) string) string {
	return clockProgram(clockBlockOrder, mutate)
}

func replaceInBlock(target, old, new string) func(name, block string) string {
	return func(name, block string) string {
		if name != target {
			return block
		}
		if !strings.Contains(block, old) {
			panic("block " + name + " lacks " + old)
		}
		return strings.Replace(block, old, new, 1)
	}
}

func parseClock(t *testing.T, source string) ParseResult {
	t.Helper()
	result, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func hasError(result ParseResult, want string) bool {
	for _, message := range result.Errors {
		if strings.Contains(message, want) {
			return true
		}
	}
	return false
}

// The spec example compiles to exactly this family configuration. Minutes are
// minutes after midnight on the declared clock: 11:05 = 665, 14:05 = 845,
// 03:00 = 180. The expiry defaults to the close.
func TestClockRangeBreakoutExampleConfig(t *testing.T) {
	result := parseClock(t, clockExample(nil))
	if len(result.Errors) != 0 || len(result.Warnings) != 0 {
		t.Fatalf("errors=%v warnings=%v", result.Errors, result.Warnings)
	}
	cfg := result.Config
	want := map[string]any{
		"setupType":       "clockRangeBreakout",
		"clock":           map[string]any{"utcOffsetMinutes": 600},
		"allowLong":       1,
		"allowShort":      1,
		"cooldownCandles": 0,
		"maxHoldCandles":  0,
		"tradeWindowMode": "unrestricted",
		"riskUsd":         200,
		"target":          map[string]any{},
		"breakeven":       map[string]any{"atR": nil, "offsetAtr": 0},
		"clockRangeBreakout": map[string]any{
			"rangeStartMinute": 665, "rangeEndMinute": 845, "expireMinute": 180, "closeMinute": 180, "bufferPips": 0.0,
		},
		"stop":     map[string]any{"type": "percent", "percent": 1.0, "extremeCandles": 0, "maxAtr": nil, "minAtr": 0, "paddingAtr": 0},
		"sessions": map[string]any{"asia": 1, "london": 1, "mid": 1, "ny": 1},
		"slices":   []any{map[string]any{"symbol": "XAUUSD", "tf": "5m"}},
	}
	for key, value := range want {
		got, _ := json.Marshal(cfg[key])
		expected, _ := json.Marshal(value)
		if string(got) != string(expected) {
			t.Errorf("cfg[%s] = %s, want %s", key, got, expected)
		}
	}
	spec, err := DecodeClockRangeBreakout(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if spec.ResolvedExpiryMinute() != 1620 || spec.ResolvedCloseMinute() != 1620 || spec.UTCOffsetMinutes != 600 {
		t.Errorf("spec = %+v", spec)
	}
}

func TestClockRangeBreakoutAcceptedSpellings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string, string) string
		check  func(t *testing.T, cfg Config)
	}{
		{"UTC+5:30", replaceInBlock("market", "UTC+10", "UTC+5:30"), func(t *testing.T, cfg Config) {
			if cfg["clock"].(map[string]any)["utcOffsetMinutes"] != 330 {
				t.Errorf("clock = %v", cfg["clock"])
			}
		}},
		{"UTC-3:30", replaceInBlock("market", "UTC+10", "UTC-3:30"), func(t *testing.T, cfg Config) {
			if cfg["clock"].(map[string]any)["utcOffsetMinutes"] != -210 {
				t.Errorf("clock = %v", cfg["clock"])
			}
		}},
		{"UTC+14", replaceInBlock("market", "UTC+10", "UTC+14"), nil},
		{"UTC-12", replaceInBlock("market", "UTC+10", "UTC-12"), nil},
		{"lower case clock", replaceInBlock("market", "clock UTC+10", "CLOCK utc+10"), nil},
		{"no expiry line", replaceInBlock("setup", "  orders expire 03:00\n", ""), func(t *testing.T, cfg Config) {
			family := cfg["clockRangeBreakout"].(map[string]any)
			if family["expireMinute"] != 180 {
				t.Errorf("expiry did not default to close: %v", family)
			}
		}},
		{"no buffer line", replaceInBlock("setup", "  buffer 0 pips\n", ""), func(t *testing.T, cfg Config) {
			if cfg["clockRangeBreakout"].(map[string]any)["bufferPips"] != 0.0 {
				t.Errorf("buffer = %v", cfg["clockRangeBreakout"])
			}
		}},
		{"fractional buffer", replaceInBlock("setup", "buffer 0 pips", "buffer 2.5 pips"), func(t *testing.T, cfg Config) {
			if cfg["clockRangeBreakout"].(map[string]any)["bufferPips"] != 2.5 {
				t.Errorf("buffer = %v", cfg["clockRangeBreakout"])
			}
		}},
		{"expire before close on the next day", replaceInBlock("setup", "orders expire 03:00", "orders expire 15:00"), nil},
		{"stop 100 percent", replaceInBlock("risk", "stop 1 percent", "stop 100 percent"), nil},
		{"fractional percent", replaceInBlock("risk", "stop 1 percent", "stop 0.25 percent"), nil},
		{"long only", func(name, block string) string {
			if name == "setup" {
				return block + "filters {\n  side long only\n}\n"
			}
			return block
		}, func(t *testing.T, cfg Config) {
			if cfg["allowLong"] != 1 || cfg["allowShort"] != 0 {
				t.Errorf("sides = %v/%v", cfg["allowLong"], cfg["allowShort"])
			}
		}},
		{"short only", func(name, block string) string {
			if name == "setup" {
				return block + "filters {\n  side short only\n}\n"
			}
			return block
		}, func(t *testing.T, cfg Config) {
			if cfg["allowLong"] != 0 || cfg["allowShort"] != 1 {
				t.Errorf("sides = %v/%v", cfg["allowLong"], cfg["allowShort"])
			}
		}},
		{"explicit zero risk", replaceInBlock("execution", "200 USD", "0 USD"), func(t *testing.T, cfg Config) {
			if cfg["riskUsd"] != 0.0 {
				t.Errorf("riskUsd = %v (%T)", cfg["riskUsd"], cfg["riskUsd"])
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := parseClock(t, clockExample(tc.mutate))
			if len(result.Errors) != 0 {
				t.Fatalf("errors = %v", result.Errors)
			}
			if tc.check != nil {
				tc.check(t, result.Config)
			}
		})
	}
}

// Group 2: required phrases, malformed or out-of-range tokens and values.
func TestClockRangeBreakoutCompileRejections(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string, string) string
		want   string
	}{
		{"missing clock", replaceInBlock("market", "  clock UTC+10\n", ""), "requires clock UTC"},
		{"missing range", replaceInBlock("setup", "  range 11:05 to 14:05\n", ""), "requires range <HH:MM> to <HH:MM>"},
		{"missing percent stop", replaceInBlock("risk", "  stop 1 percent\n", ""), "requires stop <N> percent"},
		{"missing close", replaceInBlock("management", "  close positions at 03:00\n", ""), "requires close positions at"},
		{"offset beyond +14:00", replaceInBlock("market", "UTC+10", "UTC+14:01"), "within UTC-12:00 and UTC+14:00"},
		{"offset beyond +14 hours", replaceInBlock("market", "UTC+10", "UTC+15"), "within UTC-12:00 and UTC+14:00"},
		{"offset beyond -12:00", replaceInBlock("market", "UTC+10", "UTC-12:01"), "within UTC-12:00 and UTC+14:00"},
		{"offset minutes 60", replaceInBlock("market", "UTC+10", "UTC+2:60"), "minutes must be 00-59"},
		{"offset without sign", replaceInBlock("market", "UTC+10", "UTC10"), "clock must be written"},
		{"offset without hours", replaceInBlock("market", "UTC+10", "UTC+"), "clock must be written"},
		{"fractional hours", replaceInBlock("market", "UTC+10", "UTC+1.5"), "clock must be written"},
		{"zero padded hours", replaceInBlock("market", "UTC+10", "UTC+010"), "clock must be written"},
		{"one digit minutes", replaceInBlock("market", "UTC+10", "UTC+5:3"), "clock must be written"},
		{"space after UTC", replaceInBlock("market", "UTC+10", "UTC +10"), "clock must be written"},
		{"GMT spelling", replaceInBlock("market", "UTC+10", "GMT+10"), "clock must be written"},
		{"named zone", replaceInBlock("market", "UTC+10", "Australia/Sydney"), "clock must be written"},
		{"duplicate clock", replaceInBlock("market", "  clock UTC+10\n", "  clock UTC+10\n  clock UTC+2\n"), "duplicate clock"},
		{"24:00 range end", replaceInBlock("setup", "to 14:05", "to 24:00"), `range time "24:00"`},
		{"25:00 range start", replaceInBlock("setup", "range 11:05", "range 25:00"), `range time "25:00"`},
		{"one digit minute", replaceInBlock("setup", "range 11:05", "range 11:5"), `range time "11:5"`},
		{"no colon", replaceInBlock("setup", "range 11:05", "range 1105"), `range time "1105"`},
		{"seconds", replaceInBlock("setup", "to 14:05", "to 14:05:00"), `range time "14:05:00"`},
		{"missing to", replaceInBlock("setup", "11:05 to 14:05", "11:05 14:05"), "range must be written"},
		{"start after end", replaceInBlock("setup", "11:05 to 14:05", "14:05 to 11:05"), "range start must be before range end"},
		{"start equals end", replaceInBlock("setup", "11:05 to 14:05", "14:05 to 14:05"), "range start must be before range end"},
		{"duplicate range", replaceInBlock("setup", "  range 11:05 to 14:05\n", "  range 11:05 to 14:05\n  range 11:10 to 14:05\n"), "duplicate range"},
		{"expiry equals range end", replaceInBlock("setup", "orders expire 03:00", "orders expire 14:05"), "must differ from range end"},
		{"close equals range end", replaceInBlock("management", "03:00", "14:05"), "must differ from range end"},
		{"expiry after close", replaceInBlock("setup", "orders expire 03:00", "orders expire 10:00"), "must not be after close"},
		{"malformed expiry", replaceInBlock("setup", "orders expire 03:00", "orders expire 3:00"), `orders expire time "3:00"`},
		{"malformed close", replaceInBlock("management", "03:00", "24:00"), `close positions at time "24:00"`},
		{"negative buffer", replaceInBlock("setup", "buffer 0 pips", "buffer -1 pips"), "buffer must be nonnegative"},
		{"NaN buffer", replaceInBlock("setup", "buffer 0 pips", "buffer NaN pips"), "buffer must be finite"},
		{"infinite buffer", replaceInBlock("setup", "buffer 0 pips", "buffer Inf pips"), "buffer must be finite"},
		{"overflowing buffer", replaceInBlock("setup", "buffer 0 pips", "buffer 1"+strings.Repeat("0", 400)+" pips"), "buffer must be finite"},
		{"text buffer", replaceInBlock("setup", "buffer 0 pips", "buffer wide pips"), "is not a number"},
		{"buffer in points", replaceInBlock("setup", "buffer 0 pips", "buffer 0 points"), "buffer must be written"},
		{"zero percent", replaceInBlock("risk", "stop 1 percent", "stop 0 percent"), "greater than 0 and at most 100"},
		{"negative percent", replaceInBlock("risk", "stop 1 percent", "stop -1 percent"), "greater than 0 and at most 100"},
		{"percent above 100", replaceInBlock("risk", "stop 1 percent", "stop 100.01 percent"), "greater than 0 and at most 100"},
		{"NaN percent", replaceInBlock("risk", "stop 1 percent", "stop NaN percent"), "stop percent must be finite"},
		{"infinite percent", replaceInBlock("risk", "stop 1 percent", "stop Inf percent"), "stop percent must be finite"},
		{"text percent", replaceInBlock("risk", "stop 1 percent", "stop big percent"), "is not a number"},
		{"duplicate stop", replaceInBlock("risk", "stop 1 percent", "stop 1 percent\n  stop 2 percent"), "duplicate stop"},
		{"ATR stop", replaceInBlock("risk", "stop 1 percent", "stop 1 ATR"), "does not support stop"},
		{"structure stop", replaceInBlock("risk", "stop 1 percent", "stop beyond last 3 candle extreme by 0.2 ATR"), "does not support stop"},
		{"pip stop", replaceInBlock("risk", "stop 1 percent", "stop 20 pips"), "does not support stop"},
		{"negative risk", replaceInBlock("execution", "200 USD", "-5 USD"), "does not support risk"},
		{"nonnumeric risk", replaceInBlock("execution", "200 USD", "lots USD"), "does not support risk"},
		{"unknown pip route", replaceInBlock("market", "XAUUSD 5m", "FOOUSD 5m"), "no reviewed pip size for FOOUSD"},
		{"wrong family spelling", replaceInBlock("setup", "type: clock range breakout", "type: clock range breakout opening"), "require type: clock range breakout"},
		{"two types", replaceInBlock("setup", "  type: clock range breakout\n", "  type: clock range breakout\n  type: opening range breakout\n"), "require type: clock range breakout"},
		{"clock type declared twice", replaceInBlock("setup", "  type: clock range breakout\n", "  type: opening range breakout\n  type: clock range breakout\n"), "does not support setup type"},
		{"odd slices", replaceInBlock("market", "slices(XAUUSD 5m)", "slices(XAUUSD)"), "slices that are not symbol timeframe pairs"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := parseClock(t, clockExample(tc.mutate))
			if !hasError(result, tc.want) {
				t.Fatalf("errors = %q, want one containing %q", result.Errors, tc.want)
			}
			for _, diagnostic := range result.Diagnostics {
				if diagnostic.Severity != DiagnosticError && strings.Contains(diagnostic.Message, tc.want) {
					t.Errorf("%q downgraded to %s", tc.want, diagnostic.Severity)
				}
			}
		})
	}
}

func TestClockRangeBreakoutFamilyDirectivesRequireTheFamily(t *testing.T) {
	source := clockExample(replaceInBlock("setup", "type: clock range breakout", "type: opening range breakout"))
	result := parseClock(t, source)
	if !hasError(result, "require type: clock range breakout") {
		t.Fatalf("errors = %q", result.Errors)
	}
	if hasError(parseClock(t, "dsl v7\nstrategy \"orb\" {\n}\nsetup {\n  type: opening range breakout\n}\n"), "clock range breakout") {
		t.Error("an ordinary opening range breakout mentions the new family")
	}
}

// forbiddenClockDirectives are directives outside the v1 surface. Several
// request an apparently neutral value; all must fail in every section order.
var forbiddenClockDirectives = []string{
	"sessions london",
	"sessions asia london mid ny",
	"trade window unrestricted",
	"trade window minutes 0 to 90",
	"trade window UTC hours 8 to 21",
	"local hour not in (18)",
	"local weekday in (mon)",
	"weekday in (mon)",
	"new york hour in (9 10)",
	"session phase in (open)",
	"day type in ranging",
	"prior day type in trend",
	"maxMovementEr 1",
	"movement er below 0.5",
	"trendiness 0.3",
	"priority(PDH PDL)",
	"near key level 1 ATR",
	"range method zone",
	"channel active within 10",
	"higher timeframe 1h",
	"trigger pin",
	"candle body at least 0.5",
	"tail 0.5",
	"close 0.7",
	"entry limit",
	"when price closes above range then enter long",
	"stop beyond last 3 candle extreme by 0.2 ATR",
	"stop 2 ATR",
	"target 2R",
	"target 0R",
	"take profit at range edge",
	"minimum 0.6",
	"fallback 1R",
	"move stop to breakeven at 0.75R",
	"breakeven 0.5R",
	"partial 50% at 1R",
	"trail 1 ATR",
	"wait 3 candles",
	"maxHoldCandles 0",
	"maxHoldBars 10",
	"cooldown 0",
	"context 1",
	"location 1",
	"require 2",
	"size 2 for grade A",
	"rmv below 30",
	"seasonality hour in (3)",
	"micro spread below 2",
	"open location in (above)",
	"approach 3 candles",
	"direction long",
	"side long",
	"side long short",
	"risk usd",
	"entryTf 1m",
	"source timeframe 1h",
}

func TestClockRangeBreakoutRejectsForbiddenDirectivesInEveryOrder(t *testing.T) {
	orders := permutations(clockBlockOrder[1:])
	for _, directive := range forbiddenClockDirectives {
		for _, section := range clockBlockOrder[1:] {
			for _, order := range orders {
				source := clockProgram(append([]string{"strategy"}, order...), func(name, block string) string {
					if name != section {
						return block
					}
					return strings.Replace(block, "{\n", "{\n  "+directive+"\n", 1)
				})
				result := parseClock(t, source)
				if !hasError(result, "clock range breakout does not support") {
					t.Fatalf("%q in %s with order %v: errors = %q", directive, section, order, result.Errors)
				}
				for _, diagnostic := range result.Diagnostics {
					if diagnostic.Severity == DiagnosticWarning && strings.Contains(diagnostic.Message, "does not support") {
						t.Fatalf("%q downgraded to a warning", directive)
					}
				}
			}
		}
	}
}

func TestClockRangeBreakoutRejectsForbiddenDirectiveBeforeAnySection(t *testing.T) {
	for _, directive := range forbiddenClockDirectives {
		source := "dsl v7\n" + directive + "\n" + clockExample(nil)[len("dsl v7\n"):]
		if result := parseClock(t, source); len(result.Errors) == 0 {
			t.Errorf("%q before any section was accepted", directive)
		}
	}
}

// Valid programs are order independent: any section order and any order of
// the setup phrases (including `type:` last) gives the same configuration.
func TestClockRangeBreakoutParseIsOrderIndependent(t *testing.T) {
	want, _ := json.Marshal(parseClock(t, clockExample(nil)).Config)
	setupLines := []string{"type: clock range breakout", "range 11:05 to 14:05", "orders expire 03:00", "buffer 0 pips"}
	for _, order := range permutations(clockBlockOrder) {
		for _, lines := range permutations(setupLines) {
			source := clockProgram(order, func(name, block string) string {
				if name == "setup" {
					return "setup {\n  " + strings.Join(lines, "\n  ") + "\n}\n"
				}
				return block
			})
			result := parseClock(t, source)
			if len(result.Errors) != 0 {
				t.Fatalf("order %v lines %v: %v", order, lines, result.Errors)
			}
			if got, _ := json.Marshal(result.Config); string(got) != string(want) {
				t.Fatalf("order %v lines %v: config differs", order, lines)
			}
		}
	}
}

func TestClockRangeBreakoutInlineBlocks(t *testing.T) {
	source := `dsl v7
strategy "inline" { description "synthetic" }
market conditions { slices(XAUUSD 5m) clock UTC+10 }
setup { type: clock range breakout range 11:05 to 14:05 orders expire 03:00 buffer 0 pips }
risk { stop 1 percent }
management { close positions at 03:00 }
execution { risk: 200 USD }
`
	result := parseClock(t, source)
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v", result.Errors)
	}
	want, _ := json.Marshal(parseClock(t, clockExample(nil)).Config)
	got, _ := json.Marshal(result.Config)
	// The synthetic description differs; compare the family-relevant keys.
	var a, b map[string]any
	_ = json.Unmarshal(got, &a)
	_ = json.Unmarshal(want, &b)
	for _, key := range []string{"clock", "clockRangeBreakout", "stop", "setupType", "slices", "riskUsd"} {
		if !reflect.DeepEqual(a[key], b[key]) {
			t.Errorf("inline cfg[%s] = %v, want %v", key, a[key], b[key])
		}
	}
}

// Group 3: all four boundaries must sit on every effective route's UTC grid.
// With UTC+10 the example's 11:05 / 14:05 / 03:00 are 01:05 / 04:05 / 17:00
// UTC: aligned for 1m and 5m, not for 15m (01:05 is not a quarter hour).
func TestClockRangeBreakoutRouteAlignment(t *testing.T) {
	routes := func(route string) func(string, string) string {
		return replaceInBlock("market", "slices(XAUUSD 5m)", route)
	}
	tests := []struct {
		name   string
		mutate func(string, string) string
		want   string // empty = accepted
	}{
		{"explicit 5m", routes("slices(XAUUSD 5m)"), ""},
		{"explicit 1m", routes("slices(XAUUSD 1m)"), ""},
		{"explicit 15m", routes("slices(XAUUSD 15m)"), "route XAUUSD 15m: range start 11:05"},
		{"one bad slice among good ones", routes("slices(XAUUSD 5m XAUUSD 15m)"), "route XAUUSD 15m"},
		{"symbols and timeframes 1m 5m", routes("symbols XAUUSD\n  timeframes 5m 1m"), ""},
		{"symbols and timeframes with 15m", routes("symbols XAUUSD\n  timeframes 5m 15m"), "route XAUUSD 15m"},
		{"timeframes only with 15m", routes("timeframes 15m"), "route 15m"},
		{"no route declared uses the shared 5m 15m default", routes("symbols XAUUSD"), "route XAUUSD 15m"},
		{"unsupported weekly timeframe", routes("slices(XAUUSD 1w)"), "unsupported timeframe"},
		{"unsupported odd minute timeframe", routes("slices(XAUUSD 7m)"), "unsupported timeframe"},
		{"range start off grid", replaceInBlock("setup", "range 11:05", "range 11:02"), "range start 11:02 (UTC+10) is not aligned to the 5m"},
		{"range end off grid", replaceInBlock("setup", "to 14:05", "to 14:02"), "range end 14:02 (UTC+10) is not aligned to the 5m"},
		{"expiry off grid", replaceInBlock("setup", "orders expire 03:00", "orders expire 02:58"), "orders expire 02:58 (UTC+10) is not aligned to the 5m"},
		{"close off grid", replaceInBlock("management", "03:00", "03:02"), "close positions 03:02 (UTC+10) is not aligned to the 5m"},
		{"expiry off grid defaulting from close", replaceInBlock("management", "03:00", "03:02"), "is not aligned"},
		{"fractional offset keeps 5m grid", func(name, block string) string {
			return replaceInBlock("market", "UTC+10", "UTC+5:30")(name, replaceInBlock("setup", "range 11:05 to 14:05", "range 08:05 to 11:05")(name, replaceInBlock("setup", "orders expire 03:00", "orders expire 22:00")(name, replaceInBlock("management", "03:00", "22:00")(name, block))))
		}, ""},
		{"fractional offset on a 15m grid", func(name, block string) string {
			return replaceInBlock("market", "slices(XAUUSD 5m)\n  clock UTC+10", "slices(XAUUSD 15m)\n  clock UTC+5:30")(name, replaceInBlock("setup", "range 11:05 to 14:05", "range 08:00 to 11:00")(name, replaceInBlock("setup", "orders expire 03:00", "orders expire 22:00")(name, replaceInBlock("management", "03:00", "22:00")(name, block))))
		}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := parseClock(t, clockExample(tc.mutate))
			if tc.want == "" {
				if len(result.Errors) != 0 {
					t.Fatalf("errors = %v", result.Errors)
				}
				return
			}
			if !hasError(result, tc.want) {
				t.Fatalf("errors = %q, want one containing %q", result.Errors, tc.want)
			}
		})
	}
}

// Existing families keep their handling of the heads this family shares.
func TestClockRangeBreakoutLeavesOtherFamiliesUnchanged(t *testing.T) {
	source := "dsl v7\nstrategy \"orb\" {\n}\nsetup {\n  type: opening range breakout\n}\nrisk {\n  stop 150 percent\n}\nfilters {\n  close 0.7\n  range method zone\n}\n"
	result := parseClock(t, source)
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v", result.Errors)
	}
	stop := result.Config["stop"].(map[string]any)
	if stop["type"] != "fixedAtr" || stop["atr"] != 150.0 || result.Config["closeLocationMin"] != 0.7 {
		t.Errorf("legacy stop/close handling changed: stop=%v close=%v", stop, result.Config["closeLocationMin"])
	}
	if _, present := result.Config["clock"]; present {
		t.Error("clock key leaked into another family")
	}
}

func TestClockRangeSpecValidate(t *testing.T) {
	base := ClockRangeSpec{UTCOffsetMinutes: 600, RangeStartMinute: 665, RangeEndMinute: 845, ExpireMinute: 180, CloseMinute: 180, StopPercent: 1, AllowLong: true, AllowShort: true}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*ClockRangeSpec){
		"offset":       func(s *ClockRangeSpec) { s.UTCOffsetMinutes = 841 },
		"negative":     func(s *ClockRangeSpec) { s.UTCOffsetMinutes = -721 },
		"minute":       func(s *ClockRangeSpec) { s.CloseMinute = 1440 },
		"start":        func(s *ClockRangeSpec) { s.RangeStartMinute = 845 },
		"expiry":       func(s *ClockRangeSpec) { s.ExpireMinute = 845 },
		"expiry>close": func(s *ClockRangeSpec) { s.ExpireMinute = 600 },
		"buffer":       func(s *ClockRangeSpec) { s.BufferPips = -0.5 },
		"percent":      func(s *ClockRangeSpec) { s.StopPercent = 101 },
		"sides":        func(s *ClockRangeSpec) { s.AllowLong, s.AllowShort = false, false },
	}
	for name, mutate := range mutations {
		spec := base
		mutate(&spec)
		if err := spec.Validate(); err == nil {
			t.Errorf("%s: invalid spec accepted", name)
		}
	}
}

func permutations(items []string) [][]string {
	if len(items) <= 1 {
		return [][]string{append([]string(nil), items...)}
	}
	var out [][]string
	for i := range items {
		rest := append(append([]string(nil), items[:i]...), items[i+1:]...)
		for _, tail := range permutations(rest) {
			out = append(out, append([]string{items[i]}, tail...))
		}
	}
	return out
}

var _ = fmt.Sprintf

// A forbidden directive written inline, before, between or after the allowed
// ones, must fail. The ordinary inline splitter drops text before the first
// recognized directive and glues the rest onto its neighbor, so these cases
// exercise the family's audit of the original text.
func TestClockRangeBreakoutRejectsForbiddenDirectivesInsideInlineBlocks(t *testing.T) {
	bodies := []struct {
		section string
		header  string
		parts   []string
	}{
		{"market", "market conditions", []string{"slices(XAUUSD 5m)", "clock UTC+10"}},
		{"setup", "setup", []string{"type: clock range breakout", "range 11:05 to 14:05", "orders expire 03:00", "buffer 0 pips"}},
		{"risk", "risk", []string{"stop 1 percent"}},
		{"management", "management", []string{"close positions at 03:00"}},
		{"execution", "execution", []string{"risk: 200 USD"}},
	}
	injections := []string{"local hour in (1)", "trade window unrestricted", "sessions london", "maxHoldCandles 0", "target 2R", "frobnicate 3", "weekday in (mon)"}
	for _, injection := range injections {
		for _, target := range bodies {
			positions := map[string]func() string{
				"leading":  func() string { return injection + " " + strings.Join(target.parts, " ") },
				"trailing": func() string { return strings.Join(target.parts, " ") + " " + injection },
			}
			if len(target.parts) > 1 {
				positions["middle"] = func() string {
					return target.parts[0] + " " + injection + " " + strings.Join(target.parts[1:], " ")
				}
			}
			for position, body := range positions {
				source := clockExample(func(name, block string) string {
					if name != target.section {
						return block
					}
					return target.header + " { " + body() + " }\n"
				})
				result := parseClock(t, source)
				if len(result.Errors) == 0 {
					t.Errorf("%s inline %q in %s was accepted silently", position, injection, target.section)
				}
			}
		}
	}
}

func TestClockRangeBreakoutAcceptsInlineBlocksAndQuotedMetadata(t *testing.T) {
	source := clockExample(func(name, block string) string {
		switch name {
		case "strategy":
			return "strategy \"Mentions clock UTC+10 and buffer 1 pips\" { description \"orders expire at close; range 11:05 to 14:05; local hour is not used\" }\n"
		case "market":
			return "market conditions { slices(XAUUSD 5m) clock UTC+10 }\n"
		case "setup":
			return "setup { type: clock range breakout range 11:05 to 14:05 orders expire 03:00 buffer 0 pips }\n"
		}
		return block
	})
	if result := parseClock(t, source); len(result.Errors) != 0 {
		t.Fatalf("errors = %v", result.Errors)
	}
}

// Words that happen to be family directives, inside quoted strategy metadata
// of another family, are text. The base parser keeps them; so must this one.
func TestQuotedMetadataKeepsDirectiveWordsInOtherFamilies(t *testing.T) {
	tests := map[string]string{
		"dsl v7\nstrategy \"Old family\" { description \"Runs by clock UTC+10\" }\nsetup { type: opening range breakout }\n":              "Runs by clock UTC+10",
		"dsl v7\nstrategy \"Old family\" { description \"orders expire soon, buffer 2 pips\" }\nsetup { type: opening range breakout }\n": "orders expire soon buffer 2 pips",
	}
	for source, want := range tests {
		result := parseClock(t, source)
		if len(result.Errors) != 0 || result.Config["description"] != want {
			t.Errorf("errors=%v description=%q, want %q", result.Errors, result.Config["description"], want)
		}
	}
}
