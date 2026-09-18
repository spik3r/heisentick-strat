package dsl

import (
	"fmt"
	"testing"
)

const validFairValueGapSource = `dsl v7
strategy "Fair value gap parser"
setup {
  type: fair value gap
  gap minimum 0.1 ATR
  displacement minimum 0.8 ATR
  retest within 8 candles
  entry at midpoint
}
`

func TestFairValueGapParserCanonicalAndAlias(t *testing.T) {
	for _, setupType := range []string{"fair value gap", "fvg"} {
		source := validFairValueGapSource
		if setupType == "fvg" {
			source = `dsl v7
strategy "FVG alias parser"
setup { type: fvg gap minimum 0.2 ATR displacement minimum 1.1 ATR retest within 5 candles entry at proximal edge }
`
		}
		result, err := Parse(source)
		if err != nil {
			t.Fatalf("Parse(%q): %v", setupType, err)
		}
		if len(result.Errors) != 0 {
			t.Fatalf("Parse(%q) errors = %v", setupType, result.Errors)
		}
		if got := result.Config["setupType"]; got != string(FamilyFairValueGap) {
			t.Fatalf("Parse(%q) setupType = %#v", setupType, got)
		}
		fvg := copyMap(result.Config["fairValueGap"])
		wantGap, wantDisplacement, wantRetest, wantReference := 0.1, 0.8, float64(8), "midpoint"
		if setupType == "fvg" {
			wantGap, wantDisplacement, wantRetest, wantReference = 0.2, 1.1, 5, "proximalEdge"
		}
		if fvg["minGapAtr"] != wantGap || fvg["minDisplacementAtr"] != wantDisplacement || fvg["retestCandles"] != wantRetest || fvg["entryReference"] != wantReference {
			t.Fatalf("Parse(%q) fairValueGap = %#v", setupType, fvg)
		}
	}
}

func TestFairValueGapEntryWindow(t *testing.T) {
	result, err := Parse("dsl v7\nsetup { type: fair value gap entry at midpoint within 12 candles }")
	if err != nil || len(result.Errors) != 0 {
		t.Fatalf("Parse entry window: %v %v", result.Errors, err)
	}
	if got := result.Config["fairValueGap"].(map[string]any)["entryExpireCandles"]; got != float64(12) {
		t.Fatalf("entryExpireCandles = %#v, want 12", got)
	}
}

func TestFairValueGapParserSupportsFib618Entry(t *testing.T) {
	result, err := Parse("dsl v7\nsetup { type: fair value gap entry at 0.618 within 12 candles }")
	if err != nil || len(result.Errors) != 0 {
		t.Fatalf("Parse fib 0.618 entry: %v %v", result.Errors, err)
	}
	fvg := result.Config["fairValueGap"].(map[string]any)
	if got := fvg["entryReference"]; got != "fib618" {
		t.Fatalf("entryReference = %#v, want fib618", got)
	}
}

func TestFairValueGapParserSupportsUnrestrictedTradeWindow(t *testing.T) {
	source := `dsl v7
market conditions {
  slices(EURUSD 15m)
  trade window unrestricted
}
setup { type fair value gap }
`
	result, err := Parse(source)
	if err != nil || len(result.Errors) != 0 {
		t.Fatalf("Parse(%q) errors=%v err=%v", source, result.Errors, err)
	}
	if got := result.Config["tradeWindowMode"]; got != "unrestricted" {
		t.Fatalf("tradeWindowMode = %#v", got)
	}
}

func TestParserSupportsNewYorkHourFilter(t *testing.T) {
	source := `dsl v7
market conditions {
  new york hour in (10)
  new york hour not in (9)
}
`
	result, err := Parse(source)
	if err != nil || len(result.Errors) != 0 {
		t.Fatalf("Parse(%q) errors=%v err=%v", source, result.Errors, err)
	}
	if got := result.Config["newYorkHours"]; fmt.Sprint(got) != "[10]" {
		t.Fatalf("newYorkHours = %#v", got)
	}
	if got := result.Config["blockedNewYorkHours"]; fmt.Sprint(got) != "[9]" {
		t.Fatalf("blockedNewYorkHours = %#v", got)
	}
}

func TestFairValueGapParserSweepPhrases(t *testing.T) {
	result, err := Parse(`dsl v7
setup {
  type: fair value gap
  sweep required within 12 candles
  sweep pivot 3 candles
}
`)
	if err != nil || len(result.Errors) != 0 {
		t.Fatalf("Parse errors=%v err=%v", result.Errors, err)
	}
	fvg := copyMap(result.Config["fairValueGap"])
	if fvg["sweepRequired"] != 1.0 || fvg["sweepLookbackCandles"] != 12.0 || fvg["sweepPivotWidth"] != 3.0 {
		t.Fatalf("fairValueGap = %#v", fvg)
	}

	off, err := Parse("dsl v7\nsetup {\n type fair value gap\n sweep required within 9 candles\n sweep off\n}\n")
	if err != nil || len(off.Errors) != 0 {
		t.Fatalf("Parse sweep off: %v %v", off.Errors, err)
	}
	if offFvg := copyMap(off.Config["fairValueGap"]); offFvg["sweepRequired"] != 0.0 || offFvg["sweepLookbackCandles"] != 9.0 {
		t.Fatalf("sweep off fairValueGap = %#v", offFvg)
	}

	for _, phrase := range []string{
		"sweep required within 0 candles",
		"sweep pivot 0 candles",
		"sweep pivot nah candles",
		"sweep bogus",
	} {
		result, err := Parse("dsl v7\nsetup {\n type fair value gap\n " + phrase + "\n}\n")
		if err != nil {
			t.Fatalf("Parse(%q): %v", phrase, err)
		}
		if len(result.Errors) == 0 {
			t.Fatalf("Parse(%q) errors = nil, want invalid sweep phrase error", phrase)
		}
	}
}

func TestFairValueGapParserRejectsInvalidThresholds(t *testing.T) {
	for _, phrase := range []string{
		"gap minimum 0 ATR",
		"gap minimum nope ATR",
		"displacement minimum -1 ATR",
	} {
		result, err := Parse("dsl v7\nsetup { type: fair value gap " + phrase + " }\n")
		if err != nil {
			t.Fatalf("Parse(%q): %v", phrase, err)
		}
		if len(result.Errors) == 0 {
			t.Fatalf("Parse(%q) errors = nil, want invalid FVG parameter error", phrase)
		}
	}
}
