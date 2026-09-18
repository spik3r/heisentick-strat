package dsl

import (
	"strings"
	"testing"
)

const validDailyFlushFailureSource = `dsl v7
strategy "Daily Flush Failure Parser"
setup {
  type: daily flush failure
  flush range at least 1.25 ATR
  flush close in bottom 25 percent
}
management { maxHoldCandles 10 }
`

func TestDailyFlushFailureParserExactGrammar(t *testing.T) {
	result, err := Parse(validDailyFlushFailureSource)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v", result.Errors)
	}
	if result.Config["setupType"] != string(FamilyDailyFlushFailure) {
		t.Fatalf("setupType = %#v", result.Config["setupType"])
	}
	family := copyMap(result.Config["dailyFlushFailure"])
	if family["minFlushRangeAtr"] != 1.25 || family["maxFlushCloseLocation"] != 0.25 {
		t.Fatalf("dailyFlushFailure = %#v", family)
	}
}

func TestDailyFlushFailureParserRejectsMissingThreshold(t *testing.T) {
	result, err := Parse(strings.Replace(validDailyFlushFailureSource, "  flush close in bottom 25 percent\n", "", 1))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !containsPriceMomentumError(result.Errors, "flush close in bottom X percent") {
		t.Fatalf("errors = %v", result.Errors)
	}
}
