package dsl

import (
	"strings"
	"testing"
)

const validWeekendExtremeFadeSource = `dsl v7
strategy "Weekend Extreme Fade" {
  description "x"
}
market conditions {
  slices(BTCUSDT 4h)
}
setup {
  type: weekend extreme fade
  weekend range max 5 ATR
  weekend close extreme 30 percent
  stop ATR 0.75
  target ATR 2
  hold max 12 candles
}
`

func TestWeekendExtremeFadeParserExactGrammar(t *testing.T) {
	result, _ := Parse(validWeekendExtremeFadeSource)
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v", result.Errors)
	}
	if result.Config["setupType"] != string(FamilyWeekendExtremeFade) {
		t.Fatalf("setupType = %v", result.Config["setupType"])
	}
	family := copyMap(result.Config["weekendExtremeFade"])
	if family["closeExtremePct"] != 0.3 || family["targetAtr"] != 2.0 || result.Config["maxHoldCandles"] != 12.0 {
		t.Fatalf("weekendExtremeFade = %#v, maxHoldCandles = %#v", family, result.Config["maxHoldCandles"])
	}
}

func TestWeekendExtremeFadeParserRejectsTrailingGarbage(t *testing.T) {
	result, _ := Parse(validWeekendExtremeFadeSource + "weekend range max 5 ATR extra\n")
	if len(result.Errors) == 0 {
		t.Fatal("trailing weekend phrase was accepted")
	}
}

func TestWeekendExtremeFadeParserAcceptsDecimalAndWholePercent(t *testing.T) {
	for _, spelling := range []string{"0.3", "30"} {
		source := strings.Replace(validWeekendExtremeFadeSource, "30 percent", spelling+" percent", 1)
		result, _ := Parse(source)
		if len(result.Errors) != 0 {
			t.Fatalf("%s percent errors = %v", spelling, result.Errors)
		}
		family := copyMap(result.Config["weekendExtremeFade"])
		if family["closeExtremePct"] != 0.3 {
			t.Fatalf("%s percent closeExtremePct = %#v, want 0.3", spelling, family["closeExtremePct"])
		}
	}
}
