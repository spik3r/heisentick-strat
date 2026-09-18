package dsl

import "testing"

const validKeltnerReversionSource = `dsl v7
strategy "Keltner reversion parser"
setup {
  type: keltner reversion
  ema length 20
  distance 2.0 ATR
  pierce at least 0.25 ATR
  reclaim within 3 candles
  target midline else 1R
}
`

func TestKeltnerParserTypePhrasesAndDefaults(t *testing.T) {
	for _, tc := range []struct {
		phrase string
		family FamilyID
	}{
		{"keltner reversion", FamilyKeltnerReversion},
		{"keltnerReversion", FamilyKeltnerReversion},
		{"keltner expansion", FamilyKeltnerExpansion},
		{"keltnerExpansion", FamilyKeltnerExpansion},
	} {
		source := "dsl v7\nsetup { type: " + tc.phrase + " }\n"
		result, err := Parse(source)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.phrase, err)
		}
		if len(result.Errors) != 0 {
			t.Fatalf("Parse(%q) errors = %v", tc.phrase, result.Errors)
		}
		if got := result.Config["setupType"]; got != string(tc.family) {
			t.Fatalf("Parse(%q) setupType = %#v, want %v", tc.phrase, got, tc.family)
		}
		if got := result.Config["timeframes"]; !equalStringSlice(got, []string{"15m"}) {
			t.Fatalf("Parse(%q) timeframes = %#v, want [15m]", tc.phrase, got)
		}
		stop := copyMap(result.Config["stop"])
		if stop["extremeCandles"] != 0 || stop["paddingAtr"] != 0.25 || stop["minAtr"] != 0.4 || stop["maxAtr"] != 2 {
			t.Fatalf("Parse(%q) stop = %#v", tc.phrase, stop)
		}
		breakeven := copyMap(result.Config["breakeven"])
		if breakeven["atR"] != 0.5 || breakeven["offsetAtr"] != 0.05 {
			t.Fatalf("Parse(%q) breakeven = %#v", tc.phrase, breakeven)
		}
		if got := result.Config["cooldownCandles"]; got != 12 {
			t.Fatalf("Parse(%q) cooldownCandles = %#v", tc.phrase, got)
		}
		if got := result.Config["maxHoldCandles"]; got != 24 {
			t.Fatalf("Parse(%q) maxHoldCandles = %#v", tc.phrase, got)
		}
		target := copyMap(result.Config["target"])
		if !numEquals(target["krR"], 1) || !numEquals(target["minR"], 0.5) {
			t.Fatalf("Parse(%q) target = %#v", tc.phrase, target)
		}
		// Both families initialize (and share) the "keltnerReversion" key;
		// there must be no separate "keltnerExpansion" config key.
		if _, ok := result.Config["keltnerReversion"]; !ok {
			t.Fatalf("Parse(%q) missing keltnerReversion config key", tc.phrase)
		}
		if _, ok := result.Config["keltnerExpansion"]; ok {
			t.Fatalf("Parse(%q) unexpectedly has a keltnerExpansion config key", tc.phrase)
		}
	}
}

func TestKeltnerParserDoesNotOverrideExplicitTimeframes(t *testing.T) {
	source := `dsl v7
timeframes 1h
setup {
  type: keltner reversion
}
`
	result, err := Parse(source)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v", result.Errors)
	}
	if got := result.Config["timeframes"]; !equalStringSlice(got, []string{"1h"}) {
		t.Fatalf("timeframes = %#v, want [1h] (explicit timeframes directive preserved)", got)
	}
}

func TestKeltnerParserPhraseForms(t *testing.T) {
	result, err := Parse(validKeltnerReversionSource)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v", result.Errors)
	}
	keltner := copyMap(result.Config["keltnerReversion"])
	if keltner["emaLen"] != float64(20) {
		t.Fatalf("emaLen = %#v", keltner["emaLen"])
	}
	if keltner["bandAtr"] != 2.0 {
		t.Fatalf("bandAtr = %#v", keltner["bandAtr"])
	}
	if keltner["minPierceAtr"] != 0.25 {
		t.Fatalf("minPierceAtr = %#v", keltner["minPierceAtr"])
	}
	if keltner["reclaimCandles"] != float64(3) {
		t.Fatalf("reclaimCandles = %#v", keltner["reclaimCandles"])
	}
	if keltner["targetMode"] != "midlineElseFixedR" || keltner["targetR"] != float64(1) {
		t.Fatalf("target = mode=%#v r=%#v", keltner["targetMode"], keltner["targetR"])
	}
}

func TestKeltnerParserTargetModes(t *testing.T) {
	for _, tc := range []struct {
		phrase   string
		wantMode string
	}{
		{"target midline else 1R", "midlineElseFixedR"},
		{"target 1R", "fixedR"},
		{"target 0.5R", "fixedR"},
	} {
		source := "dsl v7\nsetup { type: keltner reversion " + tc.phrase + " }\n"
		result, err := Parse(source)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.phrase, err)
		}
		if len(result.Errors) != 0 {
			t.Fatalf("Parse(%q) errors = %v", tc.phrase, result.Errors)
		}
		keltner := copyMap(result.Config["keltnerReversion"])
		if keltner["targetMode"] != tc.wantMode {
			t.Fatalf("Parse(%q) targetMode = %#v, want %v", tc.phrase, keltner["targetMode"], tc.wantMode)
		}
	}
}

func TestKeltnerExpansionSharesConfigAndPhrases(t *testing.T) {
	source := `dsl v7
setup {
  type: keltner expansion
  ema length 34
  distance 1.5 ATR
  pierce at least 0.1 ATR
  reclaim within 4 candles
  target 2R
}
`
	result, err := Parse(source)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v", result.Errors)
	}
	if got := result.Config["setupType"]; got != string(FamilyKeltnerExpansion) {
		t.Fatalf("setupType = %#v", got)
	}
	keltner := copyMap(result.Config["keltnerReversion"])
	if keltner["emaLen"] != float64(34) || keltner["bandAtr"] != 1.5 || keltner["minPierceAtr"] != 0.1 ||
		keltner["reclaimCandles"] != float64(4) || keltner["targetMode"] != "fixedR" || keltner["targetR"] != float64(2) {
		t.Fatalf("keltnerReversion = %#v", keltner)
	}
}

// TestKeltnerParserRejectsInvalidPhrases writes each phrase on its own
// physical line (rather than concatenated inline within `setup { ... }`)
// because the inline-body splitter's directive boundary list
// (directiveStartsForSection in parser_lexing.go) does not include
// "distance", "pierce", or "reclaim" as boundary words — a pre-existing gap
// shared by other families (e.g. vwapExtensionFade's "distance" head hits
// the same limitation) and out of scope for this change.
func TestKeltnerParserRejectsInvalidPhrases(t *testing.T) {
	for _, tc := range []struct {
		name   string
		phrase string
	}{
		{"bad ema length not integer", "ema length 1.5"},
		{"bad ema length too small", "ema length 1"},
		{"bad distance unit", "distance 2 percent"},
		{"bad distance value", "distance -1 ATR"},
		{"bad pierce value", "pierce at least -1 ATR"},
		{"bad reclaim not integer", "reclaim within 3.5 candles"},
		{"bad reclaim not positive", "reclaim within 0 candles"},
		{"bad target value", "target 0R"},
		{"bad target missing r token", "target midline else fixed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := "dsl v7\nsetup {\n  type: keltner reversion\n  " + tc.phrase + "\n}\n"
			result, err := Parse(source)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.phrase, err)
			}
			if len(result.Errors) == 0 {
				t.Fatalf("Parse(%q) errors = nil, want an error", tc.phrase)
			}
		})
	}
}

func numEquals(got any, want float64) bool {
	switch typed := got.(type) {
	case int:
		return float64(typed) == want
	case float64:
		return typed == want
	default:
		return false
	}
}

func equalStringSlice(got any, want []string) bool {
	switch typed := got.(type) {
	case []string:
		return equalStrings(typed, want)
	case []any:
		strs := make([]string, 0, len(typed))
		for _, item := range typed {
			s, ok := item.(string)
			if !ok {
				return false
			}
			strs = append(strs, s)
		}
		return equalStrings(strs, want)
	default:
		return false
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
