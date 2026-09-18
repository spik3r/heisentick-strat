package dsl

import "testing"

func TestSMAGoldenCrossParsesCausalLongOnlySurface(t *testing.T) {
	result, err := Parse(`dsl v7
strategy "SMA Golden Cross" { description "Long-only SMA cross" }
market conditions { slices(XAUUSD 1d) }
setup {
 type: sma golden cross
 sma fast 50
 sma slow 200
}
filters { side long only }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected parse errors: %v", result.Errors)
	}
	if result.Config["setupType"] != string(FamilySMAGoldenCross) {
		t.Fatalf("setupType = %v", result.Config["setupType"])
	}
	params := result.Config["smaGoldenCross"].(map[string]any)
	if params["fastSmaLen"] != 50.0 || params["slowSmaLen"] != 200.0 {
		t.Fatalf("sma params = %#v", params)
	}
	if result.Config["allowLong"] != 1 || result.Config["allowShort"] != 0 {
		t.Fatalf("side config = long %v short %v", result.Config["allowLong"], result.Config["allowShort"])
	}
	if _, ok := result.Config["smaGoldenCross"].(map[string]any)["atrLen"]; ok {
		t.Fatal("exact SMA surface unexpectedly authored protected ATR settings")
	}
}

func TestSMAGoldenCrossParsesProtectedExecutionBundle(t *testing.T) {
	result, err := Parse(`dsl v7
strategy "SMA Golden Cross protected" { description "Protected SMA cross" }
market conditions { slices(XAUUSD 1d) }
setup {
 type: sma golden cross
 sma atr 14
 sma stop 2 ATR
 sma target 3 ATR
}
filters { side long only }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected parse errors: %v", result.Errors)
	}
	params := result.Config["smaGoldenCross"].(map[string]any)
	if params["atrLen"] != 14.0 || params["stopAtr"] != 2.0 || params["targetAtr"] != 3.0 {
		t.Fatalf("protected SMA params = %#v", params)
	}
}

func TestSMAGoldenCrossRejectsPartialProtectedExecutionBundle(t *testing.T) {
	for _, body := range []string{
		"sma atr 14",
		"sma stop 2 ATR",
		"sma target 3 ATR",
		"sma atr 14\nsma stop 2 ATR",
	} {
		result, err := Parse("dsl v7\nsetup {\n type: sma golden cross\n" + body + "\n}")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, message := range result.Errors {
			if message == "sma golden cross protected execution requires sma atr, sma stop, and sma target together." {
				found = true
			}
		}
		if !found {
			t.Fatalf("body %q errors = %v", body, result.Errors)
		}
	}
}

func TestSMAGoldenCrossRejectsMalformedAndMisplacedProtectedDirectives(t *testing.T) {
	for _, body := range []string{
		"sma atr 14 candles",
		"sma atr 0x10",
		"sma atr 1_000",
		"sma atr 0",
		"sma stop 2",
		"sma stop 2 R",
		"sma stop Infinity ATR",
		"sma stop 0x2 ATR",
		"sma stop 2_0 ATR",
		"sma target 0 ATR",
		"sma target 3 ATR extra",
	} {
		result, err := Parse("dsl v7\nsetup {\n type: sma golden cross\n" + body + "\n}")
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Errors) == 0 {
			t.Fatalf("body %q unexpectedly parsed", body)
		}
	}

	result, err := Parse(`dsl v7
setup { type: sma golden cross }
execution { sma atr 14 }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("misplaced SMA directive unexpectedly parsed")
	}
}

func TestSMAGoldenCrossRejectsUnsupportedSurface(t *testing.T) {
	result, err := Parse(`dsl v7
setup { type: sma golden cross }
target { target 1R }`)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, message := range result.Errors {
		if message == "sma golden cross does not support authored directive: target:target." {
			found = true
		}
	}
	if !found {
		t.Fatalf("errors = %v", result.Errors)
	}
}
