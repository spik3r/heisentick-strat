package dsl

import "testing"

func TestParseDualEMAResumption(t *testing.T) {
	source := `dsl v7
strategy "x" { description "x" }
setup {
 type: dual ema resumption
 ema fast 20
 ema slow 80
 ema slow rise 12
 ema wilder ATR 20
 stop initial 2.5 ATR
 trail close 3 ATR
 fallback below slow ema
}
filters { side long only }`
	result, err := Parse(source)
	if err != nil || len(result.Errors) != 0 {
		t.Fatalf("parse: %v %#v", err, result.Errors)
	}
	if got := result.Config["setupType"]; got != string(FamilyDualEMAResumption) {
		t.Fatalf("setupType=%#v", got)
	}
	params := result.Config["dualEmaResumption"].(map[string]any)
	if params["fastEmaLen"] != float64(20) || params["trailAtr"] != 3.0 {
		t.Fatalf("params=%#v", params)
	}
}

func TestDualEMAResumptionRejectsOrderDependentSideAndSource(t *testing.T) {
	for _, body := range []string{
		`setup { side short only type: dual ema resumption }`,
		`setup { type: dual ema resumption side short only }`,
		`setup { source timeframe 4h type: dual ema resumption }`,
		`setup { type: dual ema resumption source timeframe 4h }`,
	} {
		result, _ := Parse("dsl v7\nstrategy \"x\" { description \"x\" }\n" + body)
		if len(result.Errors) == 0 {
			t.Fatalf("expected error: %s", body)
		}
	}
}

func TestDualEMAResumptionRejectsGenericDirectivesAtDefaultValues(t *testing.T) {
	for _, line := range []string{
		"management { wait 3 candles after trade }",
		"target { target 1R }",
		"market conditions { sessions(asia, london, ny) }",
		"filters { day type in (ranging, choppy) }",
	} {
		result, _ := Parse("dsl v7\nstrategy \"x\" { description \"x\" }\nsetup { type: dual ema resumption }\n" + line)
		if len(result.Errors) == 0 {
			t.Fatalf("expected error: %s", line)
		}
	}
}

func TestDualEMAResumptionRejectsFamilyPhrasesBeforeType(t *testing.T) {
	for _, line := range []string{
		"ema fast 20",
		"stop initial 2.5 ATR",
		"trail nonsense",
		"fallback nonsense",
	} {
		result, _ := Parse("dsl v7\nstrategy \"x\" { description \"x\" }\nsetup {\n" + line + "\ntype: dual ema resumption\n}")
		if len(result.Errors) == 0 {
			t.Fatalf("expected order error: %s", line)
		}
	}
}

func TestDualEMAResumptionRejectsInvalidRisk(t *testing.T) {
	for _, line := range []string{"risk: 0 USD", "risk: -1 USD", "risk: nope USD", "risk: Inf USD", "risk: 200 nope"} {
		result, _ := Parse("dsl v7\nstrategy \"x\" { description \"x\" }\nsetup { type: dual ema resumption }\nexecution { " + line + " }")
		if len(result.Errors) == 0 {
			t.Fatalf("expected risk error: %s", line)
		}
	}
}
