package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
)

func orbAdmissionSource(extra string) string {
	return "dsl v7\nstrategy \"Admission source\" {}\nsetup {\n type: opening range breakout\n}\n" + extra + "\n"
}
func orbParseAdmission(t *testing.T, source string) dsl.Config {
	t.Helper()
	parsed, err := dsl.Parse(source)
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("parse: %v / %v", err, parsed.Errors)
	}
	return parsed.Config
}
func orbAdmission(t *testing.T, source string) (paperORBAuthoredFeatures, error) {
	t.Helper()
	cfg := orbParseAdmission(t, source)
	route := PaperRoute{"synthetic", "XAUUSD", "5m", "zone"}
	if slices, ok := cfg["slices"].([]any); ok && len(slices) > 0 {
		row := slices[0].(map[string]any)
		route.Symbol = row["symbol"].(string)
		route.Timeframe = row["tf"].(string)
	}
	return validatePaperORB(source, cfg, route)
}

func TestPaperORBAuthoredPositiveForms(t *testing.T) {
	for _, source := range []string{
		orbAdmissionSource(""),
		strings.ReplaceAll(strings.ToUpper(orbAdmissionSource("# Retest off is a comment\nmanagement { trail off }")), "\n", "\r\n"),
		orbAdmissionSource("setup {\n hold 2147483647 candles\n hold 1 candle\n}"),
		orbAdmissionSource("filters { side only long }\nfilters { direction only short }"),
		strings.Replace(orbAdmissionSource(""), "v7", "v6", 1),
		orbAdmissionSource("# comment contains target opposite range edge\nfilters { side both }\nmanagement { trail off }"),
		orbAdmissionSource("strategy \"Metadata target opposite range edge\" {\n description \"This source has retest words.\"\n}"),
		orbAdmissionSource("market conditions {\n slices(SYNTH 5m, XAUUSD 15m)\n symbols(SYNTH XAUUSD)\n timeframes(5m 15m)\n sessions(asia, mid, london, ny)\n}"),
		orbAdmissionSource("setup {\n opening range first 3 candles in (asia)\n opening range first 30 minutes\n break beyond range edge by 0.1 ATR\n hold 2 candles\n}"),
		orbAdmissionSource("setup {\n opening range every 3 hours UTC\n opening range first 3 bars\n}"),
		"dsl v7\nmarket conditions { sessions(asia) }\nsetup { type: opening range breakout }\n",
		orbAdmissionSource("market conditions {\n trade window in (asia.first, mid.full, london_middle, ny-close)\n trade window minutes 5 to 120\n}"),
		orbAdmissionSource("market conditions {\n trade window UTC hours 1.5 to 23\n trade window unrestricted\n}"),
		orbAdmissionSource("market conditions {\n session phase in (open middle lunch close overnight other)\n local weekday in (Monday Tue)\n weekday not in (Fri)\n local hour in (9 10)\n hour not in (23)\n open location in (nearPDH near DO other)\n prior day type in (range trend wide narrow outside)\n prior day type not in (narrow)\n prior day range at least 0.5 ATR\n day type in (trending ranging choppy) strictly\n day theme in (join_momentum buy_lows sell_highs)\n movement below 999\n}"),
		orbAdmissionSource("market conditions {\n day type in (trending ranging) or movement below 0.3\n trendiness below 0.8\n}"),
		orbAdmissionSource("filters {\n tail rejection at least 0.3\n close location at least 0.7\n rmv ATR period 3\n rmv lookback 5\n rmv at least 0\n}"),
		orbAdmissionSource("market conditions {\n seasonality intraday all in (bullish extreme_bullish) min samples 1\n seasonality day of week 30d supports entry\n seasonality week of year lookback all in (neutral)\n seasonality monthly history aligns entry\n}"),
		orbAdmissionSource("risk {\n stop beyond opposite range edge by 0.2 ATR min 0.1 ATR max 3 ATR\n stop size between 0.2 and 4 ATR\n stop size min 0.1 max 5\n stop 1.25 ATR\n stop 10 pips\n}\ntarget {\n target 1.5R\n target 2 range\n target 20 pips\n}\nexecution { risk 50 USD }"),
		orbAdmissionSource("management {\n move stop to breakeven after 0.5R plus 0.1 ATR\n partial half at 1 R move to breakeven\n trail 2 ATR after 1.5R\n wait 3.5 candles after trade\n maxHoldCandles 1.4\n}"),
		orbAdmissionSource("management {\n partial quarter at 0.5R\n partial 25% at 1R\n partial 1.2 at 2R\n breakeven off\n trail off\n maxHoldCandles 0\n}\nfilters { higher timeframe off }\nfilters { entryTf current }"),
	} {
		t.Run(source, func(t *testing.T) {
			if _, err := orbAdmission(t, source); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPaperORBAuthoredRefusesIgnoredAndErasedIntent(t *testing.T) {
	for _, extra := range []string{
		"market conditions { local weekday in (Mondayevil) }",
		"market conditions { local weekday in (Mon123) }",
		"setup {\n opening range first 2147483648 candles\n opening range first 3 candles\n}",
		"setup {\n opening range first 2147483648 minutes\n type: opening range breakout\n}",
		"target { take profit at opposite range edge }",
		"strategy \"metadata\" target \"ignored\"",
		"target {\n target oppositeRangeEdge\n}",
		"target { minimum reward 0.6R }", "target { fallback 1R }",
		"setup { retest within 6 candles tolerance 0.2 ATR }", "setup { retest off }", "setup { break buffer 0.2 ATR }",
		"market conditions { new york hour in (1) }", "filters { range method zone }",
		"filters {\n approach distance 0 ATR\n}", "filters { channel width below 0 ATR }",
		"filters {\n context 0\n}", "filters {\n require range day\n}",
		"risk {\n stop beyond last 0 candle extreme by 0.25 ATR\n}",
		"filters { entry distance at most 0 ATR }", "filters { trigger any }", "filters { no confirmation candle }",
		"execution {\n enter limit at swept edge\n enter market\n}",
		"filters {\n higher timeframe must agree\n higher timeframe off\n}",
		"setup {\n type: vwap extension fade\n require range day\n type: opening range breakout\n}",
		"setup { type: unknown ignored family }",
		"management { breakeven off } partial 0.5 at 1R",
		"filters { approach 0 ATR side both }",
		"management { breakeven after 1R trail off }",
		"filters { side both } unexpected discarded text",
		"filters { side both { } }", "filters { side both extra }",
		"management {\n trail 1e999 ATR after 1R\n trail off\n}",
		"execution {\n risk 0 USD\n risk 50 USD\n}",
		"management {\n partial 0 at 1R\n partial half at 1R\n}",
	} {
		t.Run(extra, func(t *testing.T) {
			source := orbAdmissionSource(extra)
			parsed, err := dsl.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			if len(parsed.Errors) > 0 {
				return
			} // Existing parser refusal is sufficient.
			if _, err = validatePaperORB(source, parsed.Config, PaperRoute{"synthetic", "XAUUSD", "5m", "zone"}); err == nil || err.Error() != "PAPER_PREFIX_ADMISSION" {
				t.Fatalf("ignored intent admitted: %v", err)
			}
		})
	}
}

func TestPaperORBOccurrenceReceiptAndOrder(t *testing.T) {
	source := orbAdmissionSource("management {\n partial half at 1R\n trail 1 ATR after 1R\n trail off\n breakeven off\n}\nsetup { type: opening range breakout }\nfilters {\n rmv lookback 3\n}")
	features, err := orbAdmission(t, source)
	if err != nil {
		t.Fatal(err)
	}
	if !features.Partial || !features.Trailing || !features.Breakeven || features.Contexts&paperORBRMV == 0 {
		t.Fatalf("authored intent erased: %+v", features)
	}
	cfg := orbParseAdmission(t, source)
	if mapValue(cfg, "trail")["atr"] != 0 {
		t.Fatal("ordinary off semantics changed")
	}
	if got := numberValue(mapValue(cfg, "breakeven"), "atR", 0); got != .5 {
		t.Fatalf("ordinary repeated-type reset changed: %v", got)
	}
	pre := strings.Replace(source, "setup {\n type: opening range breakout\n}\n", "management { trail 1 ATR }\nsetup { type: opening range breakout }\n", 1)
	if _, err := orbAdmission(t, pre); err == nil {
		t.Fatal("pre-family management reset intent admitted")
	}
}

func TestPaperORBConfigLeavesFailClosed(t *testing.T) {
	source := orbAdmissionSource("")
	baseline := orbParseAdmission(t, source)
	for _, mutate := range []func(dsl.Config){
		func(c dsl.Config) { c["unknown"] = 0 },
		func(c dsl.Config) { mapValue(c, "openingRangeBreakout")["unknown"] = 0 },
		func(c dsl.Config) { mapValue(c, "stop")["unknown"] = 0 },
		func(c dsl.Config) { mapValue(c, "target")["unknown"] = 0 },
		func(c dsl.Config) { mapValue(c, "partial")["unknown"] = 0 },
		func(c dsl.Config) { mapValue(c, "breakeven")["unknown"] = 0 },
		func(c dsl.Config) { mapValue(c, "trail")["unknown"] = 0 },
		func(c dsl.Config) { c["riskUsd"] = map[string]any{"value": 200} },
		func(c dsl.Config) { c["sessions"] = []any{"asia"} },
		func(c dsl.Config) {
			c["slices"] = []any{map[string]any{"symbol": "XAUUSD", "tf": "5m", "future": true}}
		},
		func(c dsl.Config) { c["dayTypes"] = []any{map[string]any{"name": "trending"}} },
		func(c dsl.Config) {
			c["relativeMeasuredVolatility"] = map[string]any{"atrPeriod": 14, "lookback": 100, "unknown": 0}
		},
		func(c dsl.Config) {
			c["seasonalityFilters"] = []any{map[string]any{"dimension": "intraday", "lookback": "all", "lookbackDays": nil, "minSamples": 1, "mode": "supportsEntry", "unknown": 0}}
		},
		func(c dsl.Config) { c["relativeMeasuredVolatility"] = map[string]any{} },
		func(c dsl.Config) { c["relativeMeasuredVolatility"] = map[string]any{"atrPeriod": 3} },
		func(c dsl.Config) { c["relativeMeasuredVolatility"] = map[string]any{"lookback": 3} },
		func(c dsl.Config) { c["relativeMeasuredVolatility"] = map[string]any{"op": "above", "value": 3} },
		func(c dsl.Config) {
			c["seasonalityFilters"] = []any{map[string]any{"dimension": "intraday", "lookback": "all", "minSamples": 1, "mode": "supportsEntry", "unknown": 0}}
		},
		func(c dsl.Config) {
			c["seasonalityFilters"] = []any{map[string]any{"dimension": "intraday", "lookback": "all", "minSamples": 1, "mode": "classification", "classifications": []any{"bullish"}, "unknown": 0}}
		},
		func(c dsl.Config) { delete(c, "riskUsd") },
		func(c dsl.Config) { c["openingRangeBreakout"] = map[string]any{"firstMinutes": 0, "requireRetest": 0} },
		func(c dsl.Config) { c["newYorkHours"] = []any{1} },
	} {
		c := orbParseAdmission(t, source)
		mutate(c)
		if _, err := validatePaperORB(source, c, PaperRoute{"synthetic", "XAUUSD", "5m", "zone"}); err == nil {
			raw, _ := json.Marshal(c)
			t.Fatalf("config mutation admitted: %s", raw)
		}
	}
	if !reflect.DeepEqual(baseline, orbParseAdmission(t, source)) {
		t.Fatal("mutated shared baseline")
	}
}

func TestPaperORBFixedPipsRequireInstrumentUnits(t *testing.T) {
	r := paperORBRequest(t, "close")
	if _, err := validatePaperORB(strings.ReplaceAll(r.Source, "XAUUSD", "SYNTH"), orbParseAdmission(t, strings.ReplaceAll(r.Source, "XAUUSD", "SYNTH")), PaperRoute{"free-identity", "SYNTH", "5m", "zone"}); err == nil {
		t.Fatal("unmapped pip route admitted")
	}
	source := strings.ReplaceAll(strings.Replace(strings.Replace(r.Source, "stop 10 pips", "stop 1 ATR", 1), "target 20 pips", "target 2R", 1), "XAUUSD", "SYNTH")
	if _, err := validatePaperORB(source, orbParseAdmission(t, source), PaperRoute{"arbitrary", "SYNTH", "5m", "zone"}); err != nil {
		t.Fatal("ATR strategy research-gated", err)
	}
}

// Cases are shared with the actual native CLI/public WASM transport proof.
type paperORBCase struct {
	Name     string             `json:"name"`
	Request  PaperPrefixRequest `json:"request"`
	Error    string             `json:"error,omitempty"`
	Expected map[string]any     `json:"expected,omitempty"`
}

func paperORBCases(t *testing.T) []paperORBCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "paper_orb", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []paperORBCase
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

func paperORBCheckExpected(t *testing.T, result PaperPrefixResult, expected map[string]any) {
	t.Helper()
	entries, exits, partials := []PaperEvent{}, []PaperEvent{}, 0
	for _, event := range result.Events {
		switch event.Kind {
		case "entry":
			entries = append(entries, event)
		case "exit":
			exits = append(exits, event)
		case "partial":
			partials++
		}
	}
	// Compare projected values only for hand expectations; parity and replay below
	// independently compare every public field, not this selected assertion view.
	raw, _ := json.Marshal(result)
	var obj map[string]any
	json.Unmarshal(raw, &obj)
	account := obj["account"].(map[string]any)
	completed := obj["completedTrades"].(map[string]any)
	observed := map[string]any{"realizedNet": account["realizedNet"], "equity": account["equity"], "fees": account["fees"], "completedTrades": completed["count"], "netPnl": completed["netPnl"], "openCount": float64(len(result.OpenPositions)), "partialEvents": float64(partials)}
	if len(entries) > 0 {
		var event map[string]any
		raw, _ := json.Marshal(entries[0])
		json.Unmarshal(raw, &event)
		effect := event["effect"].(map[string]any)
		observed["entryIndex"] = event["index"]
		observed["entrySide"] = effect["side"]
		observed["entrySize"] = effect["size"]
	}
	if len(exits) > 0 {
		var event map[string]any
		raw, _ := json.Marshal(exits[len(exits)-1])
		json.Unmarshal(raw, &event)
		observed["exitIndex"] = event["index"]
		observed["exitReason"] = event["effect"].(map[string]any)["reason"]
	}
	if len(result.OpenPositions) > 0 {
		var pos map[string]any
		raw, _ := json.Marshal(result.OpenPositions[0])
		json.Unmarshal(raw, &pos)
		observed["stop"] = pos["sl"]
	}
	for key, want := range expected {
		if !reflect.DeepEqual(observed[key], want) {
			t.Errorf("%s = %#v, want %#v", key, observed[key], want)
		}
	}
}
func TestPaperORBCorpus(t *testing.T) {
	for _, tc := range paperORBCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			cfg := orbParseAdmission(t, tc.Request.Source)
			_, admission := validatePaperORB(tc.Request.Source, cfg, tc.Request.Route)
			if tc.Error != "" {
				if admission == nil || admission.Error() != tc.Error {
					t.Fatalf("admission = %v, want %s", admission, tc.Error)
				}
				result, err := RunPaperPrefix(tc.Request)
				if err == nil || err.Error() != tc.Error || !reflect.DeepEqual(result, PaperPrefixResult{}) {
					t.Fatalf("public refusal = %+v / %v", result, err)
				}
				return
			}
			if admission != nil {
				t.Fatal(admission)
			}
			full, err := RunPaperPrefix(tc.Request)
			if err != nil {
				t.Fatal(err)
			}
			paperORBCheckExpected(t, full, tc.Expected)
			for n := 1; n <= len(tc.Request.Bars); n++ {
				prefix, err := RunPaperPrefix(paperPrefix(tc.Request, n))
				if err != nil {
					t.Fatal(err)
				}
				next := tc.Request
				next.Checkpoint = prefix.Checkpoint
				resumed, err := RunPaperPrefix(next)
				if err != nil {
					t.Fatal(err)
				}
				combined := append(append([]PaperEvent{}, prefix.Events...), resumed.Events...)
				if !reflect.DeepEqual(combined, full.Events) || !reflect.DeepEqual(resumed.Account, full.Account) || !reflect.DeepEqual(resumed.CompletedTrades, full.CompletedTrades) || !reflect.DeepEqual(resumed.OpenPositions, full.OpenPositions) || !reflect.DeepEqual(resumed.Pending, full.Pending) || !reflect.DeepEqual(resumed.Checkpoint, full.Checkpoint) {
					t.Fatalf("split %d changed complete replay", n)
				}
				next.Checkpoint = resumed.Checkpoint
				retry, err := RunPaperPrefix(next)
				if err != nil || len(retry.Events) != 0 || !reflect.DeepEqual(retry.Checkpoint, full.Checkpoint) {
					t.Fatalf("split %d retry: %v", n, err)
				}
			}
		})
	}
}
