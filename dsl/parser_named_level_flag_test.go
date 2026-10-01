package dsl

import "testing"

func TestNamedLevelFlagTypeAndBodyFilter(t *testing.T) {
	result, err := Parse(`dsl v7
strategy "Named level flag"
levels { priority(PDH, PDL, AH, AL) }
setup { type: named level flag }
filters {
  breakout body at least 0.5 ATR
}
`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors: %v", result.Errors)
	}
	if result.Config["setupType"] != string(FamilyNamedLevelFlag) {
		t.Fatalf("setupType = %#v", result.Config["setupType"])
	}
	p := result.Config["namedLevelFlag"].(map[string]any)
	if p["breakoutBodyAtr"] != 0.5 || p["maxBars"] != 10 || p["impulseAtr"] != 0.8 || p["minStopPoints"] != 0 {
		t.Fatalf("namedLevelFlag config = %#v", p)
	}
}

func TestNamedLevelFlagSignalRiskFilter(t *testing.T) {
	result, err := Parse(`dsl v7
setup { type: named level flag }
filters {
  signal risk at least 1.2 points
}
`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors: %v", result.Errors)
	}
	p := result.Config["namedLevelFlag"].(map[string]any)
	if p["minStopPoints"] != 1.2 {
		t.Fatalf("minStopPoints = %#v, want 1.2; errors=%v; cfg=%#v", p["minStopPoints"], result.Errors, result.Config)
	}
}

func TestNamedLevelFlagUsesGenericStopOverrides(t *testing.T) {
	result, err := Parse(`dsl v7
setup { type: named level flag }
risk {
  stop size between 0.6 and 1.8 ATR
  stop below the flag extreme by 0.3 ATR
}
`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors: %v", result.Errors)
	}
	stop := copyMap(result.Config["stop"])
	if stop["minAtr"] != 0.6 || stop["maxAtr"] != 1.8 || stop["paddingAtr"] != 0.3 {
		t.Fatalf("stop config = %#v, want min=.6 max=1.8 padding=.3", stop)
	}
}

func TestNamedLevelFlagCandidatesCanNeutralizeGenericMarketGates(t *testing.T) {
	result, err := Parse(`dsl v7
setup { type: named level flag }
market conditions {
  day type in (ranging, choppy, trending)
  movement below 1.5
}
`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors: %v", result.Errors)
	}
	got, ok := result.Config["dayTypes"].([]any)
	if !ok || len(got) != 3 || result.Config["maxMovementEr"] != 1.5 {
		t.Fatalf("market gates = %#v / %#v; want all regimes and movement threshold 1.5", result.Config["dayTypes"], result.Config["maxMovementEr"])
	}
}

func TestNamedLevelFlagBodyFilterRequiresPositiveATR(t *testing.T) {
	result, err := Parse(`dsl v7
setup { type: named level flag }
filters {
  breakout body at least 0 ATR
}
`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("errors = %#v, want one positive-threshold error", result.Errors)
	}
}
