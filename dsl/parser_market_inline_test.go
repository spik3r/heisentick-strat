package dsl

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestMarketInlineResearchFullConfig(t *testing.T) {
	source := marketInlineFixture(t, "testdata/market-inline-research.strat", "7f016469b99922bd79445839572157aa0004371b5c9b47ba9bad01105b4be88e")
	expected := marketInlineFixture(t, "testdata/market-inline-research.cfg.json", "472ef1847c06513c0702941a4997f4baec6d26970a3492bcfcb8aa9f11c43b91")
	var want map[string]any
	if err := json.Unmarshal(expected, &want); err != nil {
		t.Fatal(err)
	}
	if len(want) != 75 {
		t.Fatalf("frozen expected config has %d keys, want 75", len(want))
	}
	multiline := string(source)
	start := strings.Index(multiline, "market conditions {")
	end := start + strings.Index(multiline[start:], "}") + 1
	compact := multiline[:start] + strings.ReplaceAll(multiline[start:end], "\n", " ") + multiline[end:]
	for _, source := range []string{multiline, compact} {
		result := marketInlineParse(t, source)
		marketInlineClean(t, result)
		encoded, err := json.Marshal(result.Config)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("full research config drift: %s", encoded)
		}
		if got["dayTypeErEscape"] != float64(2) || got["maxMovementEr"] != float64(2) {
			t.Fatalf("escape/cap drift: %v / %v", got["dayTypeErEscape"], got["maxMovementEr"])
		}
	}
}

func TestMarketInlineEscapeEquivalent(t *testing.T) {
	tests := []struct {
		name        string
		lines       []string
		escape, cap float64
	}{
		{"escape only", []string{"day type in (trending, ranging) or movement below 0.23"}, 0.23, 0.8},
		{"cap after", []string{"day type in (trending) or movement below 0.23", "movement below 0.91"}, 0.23, 0.91},
		{"cap before", []string{"movement below 0.91", "day type in (trending) or movement below 0.23"}, 0.23, 0.91},
		{"strict", []string{"day type in (trending) strictly", "movement below 0.91"}, -1, 0.91},
		{"default", []string{"day type in (trending)", "movement below 0.91"}, 0.55, 0.91},
		{"zero escape", []string{"day type in (trending) or movement below 0", "movement below 0.91"}, 0, 0.91},
		{"signed escape", []string{"day type in (trending) or movement below +0.23", "movement below 0.91"}, 0.23, 0.91},
		{"negative escape", []string{"day type in (trending) or movement below -0.23", "movement below 0.91"}, -0.23, 0.91},
		{"exponent escape", []string{"day type in (trending) or movement below 23e-2", "movement below 0.91"}, 0.23, 0.91},
		{"positive exponent", []string{"day type in (trending) or movement below 2e+0", "movement below 0.91"}, 2, 0.91},
		{"signed positive exponent", []string{"day type in (trending) or movement below +2e+0", "movement below 0.91"}, 2, 0.91},
		{"negative positive exponent", []string{"day type in (trending) or movement below -2E+0", "movement below 0.91"}, -2, 0.91},
		{"signed negative exponent", []string{"day type in (trending) or movement below +23e-2", "movement below 0.91"}, 0.23, 0.91},
		{"exponent with suffix", []string{"day type in (trending) or movement below 2e+0R", "movement below 0.91"}, 2, 0.91},
		{"exponent with percent", []string{"day type in (trending) or movement below 2e+0%", "movement below 0.91"}, 2, 0.91},
		{"suffix escape", []string{"day type in (trending) or movement below 0.23R", "movement below 0.91"}, 0.23, 0.91},
		{"percent spelling", []string{"day type in (trending) or movement below 0.23%", "movement below 0.91"}, 0.23, 0.91},
		{"colon cue", []string{"day type in (trending) or movement below:2", "movement below 0.91"}, 2, 0.91},
		{"spaced colon cue", []string{"day type in (trending) or movement below :2", "movement below 0.91"}, 2, 0.91},
		{"spaced colon signed exponent", []string{"day type in (trending) or movement below :+2e+0", "movement below 0.91"}, 2, 0.91},
		{"spaced colon exponent suffix", []string{"day type in (trending) or movement below :2e+0R", "movement below 0.91"}, 2, 0.91},
		{"spaced colon exponent percent", []string{"day type in (trending) or movement below :2e+0%", "movement below 0.91"}, 2, 0.91},
		{"spaced attached cue", []string{"day type in (trending) or movement :below:2", "movement below 0.91"}, 2, 0.91},
		{"spaced colon words", []string{"day type in (trending) or movement :below :2", "movement below 0.91"}, 2, 0.91},
		{"separate colon cue", []string{"day type in (trending) or movement below : 2", "movement below 0.91"}, 2, 0.91},
		{"completed colon cue", []string{"day type in (trending) or movement below: 2", "movement below 0.91"}, 2, 0.91},
		{"colon words", []string{"day type in (trending) or movement:below:2", "movement below 0.91"}, 2, 0.91},
		{"colon signed exponent", []string{"day type in (trending) or:movement:below:+2e+0", "movement below 0.91"}, 2, 0.91},
		{"case and tabs", []string{"DAY\tTYPE in (trending) OR\tMOVEMENT below 0.23", "MOVEMENT below 0.91"}, 0.23, 0.91},
		{"duplicate cap", []string{"movement below 0.71", "day type in (trending) or movement below 0.23", "movement below 0.91"}, 0.23, 0.91},
		{"duplicate escape", []string{"day type in (trending) or movement below 0.23", "movement below 0.91", "day type in (ranging) or movement below 0.37"}, 0.37, 0.91},
		{"omitted escape retained", []string{"day type in (trending) or movement below 0.23", "day type in (ranging)", "movement below 0.91"}, 0.23, 0.91},
		{"explicit then strict", []string{"day type in (trending) or movement below 0.23", "day type in (ranging) strictly", "movement below 0.91"}, -1, 0.91},
		{"strict then explicit", []string{"day type in (trending) strictly", "day type in (ranging) or movement below 0.23", "movement below 0.91"}, 0.23, 0.91},
	}
	for _, version := range []string{"v6", "v7"} {
		for _, section := range []string{"market", "market conditions", "conditions"} {
			for _, test := range tests {
				t.Run(version+"/"+section+"/"+test.name, func(t *testing.T) {
					lines := append([]string{"slices(XAUUSD 15m, EURUSD 15m)", "sessions(london, ny)"}, test.lines...)
					multiline := marketInlineParse(t, "dsl "+version+"\n"+section+" {\n  "+strings.Join(lines, "\n  ")+"\n}\n")
					compact := marketInlineParse(t, "dsl "+version+"\n"+section+" {  "+strings.Join(lines, " \t ")+"  } # trailing comment\n")
					marketInlineClean(t, multiline)
					marketInlineClean(t, compact)
					if !reflect.DeepEqual(compact.Config, multiline.Config) {
						t.Fatalf("compact/multiline config mismatch:\n%#v\n%#v", compact.Config, multiline.Config)
					}
					if fmt.Sprint(compact.Config["dayTypeErEscape"]) != fmt.Sprint(test.escape) || fmt.Sprint(compact.Config["maxMovementEr"]) != fmt.Sprint(test.cap) {
						t.Fatalf("escape/cap = %v / %v, want %v / %v", compact.Config["dayTypeErEscape"], compact.Config["maxMovementEr"], test.escape, test.cap)
					}
				})
			}
		}
	}
}

func TestMarketInlineEscapeMalformedStructure(t *testing.T) {
	for _, suffix := range []string{
		"or", "or movement", "or movement below", "or below 0.23", "or movement above 0.23",
		"or movement below :", "or movement :below :",
		"or movement below :2e+ 0", "or movement below :+ 2",
		"or movement below 0.23 or movement below 0.37", "or movement below 0.23 strictly",
		"strictly or movement below 0.23", "strictly strictly", "strictly extra", "or movement below 0.23 extra",
		"or movement below 2e + 0", "or movement below 2e+ 0", "or movement below + 0.23", "or movement below + 2e+0", "or movement below 2 e+0",
	} {
		for _, version := range []string{"v6", "v7"} {
			t.Run(version+"/"+suffix, func(t *testing.T) {
				marketInlineDiagnosticPair(t, version, "day type in (trending) "+suffix, "error", "day type escape must use exactly one")
			})
		}
	}
}

func TestMarketInlineEscapeMalformedNumberCompatibility(t *testing.T) {
	for _, token := range []string{"nope", "0.23oops", "NaN", "Inf", "+Inf", "-Inf", "Infinity", "1e999", "1e+999", "2e++0", "2e+", "--2", "2e--0", "1:2", "1:", "below:2", "foo:2"} {
		for _, version := range []string{"v6", "v7"} {
			t.Run(version+"/"+token, func(t *testing.T) {
				severity := "error"
				if version == "v6" {
					severity = "warning"
				}
				marketInlineDiagnosticPair(t, version, "day type in (trending) or movement below "+token, severity, "malformed number")
			})
		}
	}
}

func TestMarketInlineUnicodeWhitespace(t *testing.T) {
	spaces := []rune{}
	for r := rune(0); r <= utf8.MaxRune; r++ {
		if unicode.IsSpace(r) {
			spaces = append(spaces, r)
		}
	}
	for _, r := range spaces {
		t.Run(fmt.Sprintf("U+%04X", r), func(t *testing.T) {
			space := string(r)
			if !inlineMarketBoundary("day"+space+"type", 3, true) {
				t.Fatal("market boundary did not recognize Unicode whitespace")
			}
			if r == '\n' {
				return // LF starts a physical directive, rather than separating its words.
			}
			for _, version := range []string{"v6", "v7"} {
				for _, section := range []string{"market", "market conditions", "conditions"} {
					lines := []string{
						strings.ReplaceAll("day type in (trending, ranging) or movement below +2e+0", " ", space),
						strings.ReplaceAll("movement below .91", " ", space),
						"sessions(london)",
					}
					multiline := marketInlineParse(t, "dsl "+version+"\n"+section+" {\n"+strings.Join(lines, "\n")+"\n}\n")
					compact := marketInlineParse(t, "dsl "+version+"\n"+section+" { "+strings.Join(lines, space)+" }\n")
					marketInlineClean(t, multiline)
					marketInlineClean(t, compact)
					if !reflect.DeepEqual(multiline.Config, compact.Config) || compact.Config["dayTypeErEscape"] != float64(2) || compact.Config["maxMovementEr"] != 0.91 {
						t.Fatalf("%s/%s whitespace changed the config: %#v", version, section, compact.Config)
					}
				}
			}
		})
	}
}

func TestMarketInlineBoundaryIsolation(t *testing.T) {
	for _, test := range []struct {
		name, section, body string
		want                []string
	}{
		{"unicode clauses", "market", "sessions(london)\u2003movement\u2003below .91", []string{"sessions(london)\u2003", "movement\u2003below .91"}},
		{"parentheses", "market", "day type in (trending,\u2003movement\u2003below) or movement below 2\u2003sessions(london)", []string{"day type in (trending,\u2003movement\u2003below) or movement below 2\u2003", "sessions(london)"}},
		{"quoted market", "market", "description \"plain\u2003movement\u2003below\u20030.2\"\u2003movement below .91", []string{"description \"plain\u2003movement\u2003below\u20030.2\"\u2003", "movement below .91"}},
		{"quoted strategy", "strategy", "description \"plain\u2003name\u2003metadata\"", []string{"description \"plain\u2003name\u2003metadata\""}},
		{"single quoted market", "market", "description 'plain\u2003movement\u2003below'\u2003sessions(london)", []string{"description 'plain\u2003movement\u2003below'\u2003", "sessions(london)"}},
		{"unicode is not quote byte", "market", "description plain\u2027\u2003movement below .91", []string{"description plain\u2027\u2003", "movement below .91"}},
		{"adjacent suffix", "market", "description plain\u2003movements below .91", []string{"description plain\u2003movements below .91"}},
		{"adjacent prefix", "market", "description plain\u2003xmovement below .91", []string{"description plain\u2003xmovement below .91"}},
		{"punctuation", "market", "sessions(london) movement: .91", []string{"sessions(london) ", "movement: .91"}},
		{"nonboundary punctuation", "market", "description plain\u2003movement,.91", []string{"description plain\u2003movement,.91"}},
		{"setup", "setup", "type:\u2003channel\u2003break\u2003hold", []string{"type:\u2003channel\u2003break\u2003hold"}},
		{"risk", "risk", "stop size min 1 max 2 target 3R", []string{"stop size min 1 max 2 ", "target 3R"}},
		{"filters", "filters", "source timeframe 15m sessions(london)", []string{"source timeframe 15m ", "sessions(london)"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := splitInlineBody(test.body, directiveStartsForSection(test.section), test.section)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parts = %#v, want %#v", got, test.want)
			}
		})
	}
	for _, source := range []string{
		"dsl v7\nstrategy \"sample\" { description \"plain\u2003name\u2003metadata\" }\n",
		"dsl v7\nmarket { description \"plain\u2003movement\u2003below\u20030.2\" day type in (trending) or movement below 2 movement below .91 }\n",
		"dsl v7\nsetup { type:\u2003channel\u2003break\u2003hold }\n",
	} {
		result := marketInlineParse(t, source)
		marketInlineClean(t, result)
		if strings.Contains(source, "strategy") && result.Config["description"] != "plain name metadata" ||
			strings.Contains(source, "market") && result.Config["description"] != "plain movement below 0.2" ||
			strings.Contains(source, "setup") && result.Config["setupType"] != "channelBreakHold" {
			t.Fatalf("accepted inline metadata/setup changed: %#v", result.Config)
		}
	}
	for _, suffix := range []string{"", "(", ":", " ", "\t", "x", ",", ")", "-"} {
		if got, want := inlineMarketBoundary("day"+suffix, 3, true), isBoundary("day"+suffix, 3); got != want {
			t.Fatalf("ASCII boundary %q changed: %v versus %v", suffix, got, want)
		}
	}
	for _, invalid := range []string{"\xff", "\xc2", "\xe2\x80", "\x80\x85"} {
		for _, section := range []string{"market", "strategy", "setup"} {
			body := "description \"plain" + invalid + "text\" " + invalid + " movement below .91"
			_ = splitInlineBody(body, directiveStartsForSection(section), section)
			_ = marketInlineParse(t, "dsl v7\n"+section+" { "+body+" }\n")
		}
	}
}

func TestMarketInlineColonCueMalformedNumber(t *testing.T) {
	for _, atom := range []string{"1:2", "1:", "below:2", "foo:2", ":", ":2", ":below:2", "1below:2"} {
		for _, cue := range []string{
			"or movement below:", "or movement below :", "or movement:below:",
			"or movement :below:", "or movement :below :", "or:movement:below:",
			"or movement below: ", "or movement below:\u2003",
		} {
			for _, version := range []string{"v6", "v7"} {
				t.Run(version+"/"+cue+atom, func(t *testing.T) {
					severity := "error"
					if version == "v6" {
						severity = "warning"
					}
					marketInlineDiagnosticPair(t, version, "day type in (trending) "+cue+atom, severity, fmt.Sprintf("malformed number %q", atom))
				})
			}
		}
	}
}

func marketInlineDiagnosticPair(t *testing.T, version, directive, severity, message string) {
	t.Helper()
	var previous string
	var previousConfig Config
	for _, layout := range []struct {
		source string
		line   int
	}{
		{"dsl " + version + "\nmarket {\n  " + directive + "\n  sessions(london)\n}\n", 3},
		{"dsl " + version + "\nmarket { " + directive + " sessions(london) }\n", 2},
	} {
		result := marketInlineParse(t, layout.source)
		if len(result.Diagnostics) != 1 {
			t.Fatalf("diagnostics = %#v, want exactly one", result.Diagnostics)
		}
		diagnostic := result.Diagnostics[0]
		if string(diagnostic.Severity) != severity || !strings.Contains(diagnostic.Message, message) || diagnostic.Line == nil || *diagnostic.Line != layout.line {
			t.Fatalf("diagnostic = %#v, want %s %q on line %d", diagnostic, severity, message, layout.line)
		}
		if previous != "" && diagnostic.Message != previous {
			t.Fatalf("layout-dependent diagnostic: %q versus %q", diagnostic.Message, previous)
		}
		previous = diagnostic.Message
		if previousConfig != nil && !reflect.DeepEqual(previousConfig, result.Config) {
			t.Fatal("malformed escape produced layout-dependent configuration")
		}
		previousConfig = result.Config
		if severity == "warning" && (len(result.Errors) != 0 || len(result.Warnings) != 1) || severity == "error" && (len(result.Errors) != 1 || len(result.Warnings) != 0) {
			t.Fatalf("errors/warnings = %v / %v", result.Errors, result.Warnings)
		}
		if result.Config["dayTypeErEscape"] != 0.55 || result.Config["maxMovementEr"] != 0.8 {
			t.Fatalf("malformed escape changed thresholds: %#v", result.Config)
		}
	}
}

func marketInlineFixture(t *testing.T, path, wantSHA string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != wantSHA {
		t.Fatalf("frozen input changed: %s", path)
	}
	return data
}

func marketInlineParse(t *testing.T, source string) ParseResult {
	t.Helper()
	result, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func marketInlineClean(t *testing.T, result ParseResult) {
	t.Helper()
	if len(result.Errors)+len(result.Warnings)+len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: errors=%v warnings=%v diagnostics=%v", result.Errors, result.Warnings, result.Diagnostics)
	}
}
