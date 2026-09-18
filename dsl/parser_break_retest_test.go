package dsl

import "testing"

func TestBreakRetestSwingAndEMAParser(t *testing.T) {
	result, err := Parse(`dsl v7
strategy "Break retest parser"
setup {
  type: break retest
  swing levels lookback 60 candles pivot 2 count 4
  ema length 50
  ema slope window 5
  ema confluence within 0.25 ATR
}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v", result.Errors)
	}
	if result.Config["setupType"] != string(FamilyBreakRetest) {
		t.Fatalf("setupType = %#v", result.Config["setupType"])
	}

	breakRetest := copyMap(result.Config["breakRetest"])
	if breakRetest["levelSource"] != "swing" ||
		breakRetest["swingLookbackBars"] != float64(60) ||
		breakRetest["swingPivotK"] != float64(2) ||
		breakRetest["swingLevelCount"] != float64(4) {
		t.Fatalf("swing config = %#v", breakRetest)
	}
	if result.Config["emaLen"] != float64(50) ||
		breakRetest["emaLen"] != float64(50) ||
		breakRetest["emaSlopeLen"] != float64(5) ||
		breakRetest["emaConfluenceAtr"] != 0.25 {
		t.Fatalf("EMA config = %#v root emaLen = %#v", breakRetest, result.Config["emaLen"])
	}
}
