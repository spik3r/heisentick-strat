package dsl

import (
	"strings"
	"testing"
)

func legacySetup9Source(setup string, extra string) string {
	return "dsl v7\nstrategy \"legacy setup 9\" { description \"controls\" }\nmarket conditions { slices(XAUUSD 1h) }\nsetup {\n type: legacy setup 9\n" + setup + "\n}\n" + extra
}

func parseLegacy(t *testing.T, source string) ParseResult {
	t.Helper()
	result, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestLegacySetup9ParsesBothProfiles(t *testing.T) {
	for _, profile := range LegacySetup9Profiles {
		result := parseLegacy(t, legacySetup9Source(" sequential profile "+profile, ""))
		if len(result.Errors) != 0 {
			t.Fatalf("%s: %v", profile, result.Errors)
		}
		if result.Config["setupType"] != string(FamilyLegacySetup9) {
			t.Fatalf("setupType = %v", result.Config["setupType"])
		}
		if got := result.Config["legacySetup9"].(map[string]any)["profile"]; got != profile {
			t.Fatalf("profile = %v, want %s", got, profile)
		}
		if result.Config["cooldownCandles"] != 0 || result.Config["maxHoldCandles"] != 0 {
			t.Fatalf("legacy controls must have no cooldown or time exit: %v %v", result.Config["cooldownCandles"], result.Config["maxHoldCandles"])
		}
		if result.Config["allowLong"] != 1 || result.Config["allowShort"] != 1 {
			t.Fatal("both sides must stay enabled")
		}
	}
}

func TestLegacySetup9ProfileIDIsCaseInsensitiveButStoredCanonical(t *testing.T) {
	result := parseLegacy(t, legacySetup9Source(" sequential profile SEQ.LEGACY.SETUP9.V1", ""))
	if len(result.Errors) != 0 || result.Config["legacySetup9"].(map[string]any)["profile"] != LegacySetup9Profile {
		t.Fatalf("errors %v cfg %v", result.Errors, result.Config["legacySetup9"])
	}
}

func TestLegacySetup9RejectsBadProfileUse(t *testing.T) {
	cases := map[string]struct{ source, want string }{
		"missing profile":      {legacySetup9Source("", ""), "requires `sequential profile"},
		"unknown profile":      {legacySetup9Source(" sequential profile seq.full.public_approx.v1", ""), "unknown sequential profile"},
		"future v2 profile":    {legacySetup9Source(" sequential profile seq.legacy.setup9.v2_i8fix", ""), "unknown sequential profile"},
		"duplicate profile":    {legacySetup9Source(" sequential profile seq.legacy.setup9.v1\n sequential profile seq.legacy.setup9.v1", ""), "only once"},
		"extra tokens":         {legacySetup9Source(" sequential profile seq.legacy.setup9.v1 now", ""), "unrecognized sequential line"},
		"other sequential key": {legacySetup9Source(" sequential profile seq.legacy.setup9.v1\n sequential countdown on", ""), "unrecognized sequential line"},
		"before type": {"dsl v7\nsetup {\n sequential profile seq.legacy.setup9.v1\n type: legacy setup 9\n}\n",
			"requires `type: legacy setup 9` before it"},
		"other family": {"dsl v7\nsetup {\n type: sma golden cross\n sequential profile seq.legacy.setup9.v1\n}\n",
			"requires `type: legacy setup 9` before it"},
	}
	for name, c := range cases {
		result := parseLegacy(t, c.source)
		if !strings.Contains(strings.Join(result.Errors, "\n"), c.want) {
			t.Errorf("%s: errors %v, want substring %q", name, result.Errors, c.want)
		}
	}
}

func TestLegacySetup9RejectsEveryUnsupportedKnob(t *testing.T) {
	profile := " sequential profile seq.legacy.setup9_perf_seasonal.v1"
	for _, extra := range []struct{ name, setup, filters string }{
		{"side", "", "filters { side long only }"},
		{"stop", " stop beyond last 3 candle extreme by 0.2 ATR", ""},
		{"target", " target 3R", ""},
		{"trade window", "", "filters { trade window unrestricted }"},
		{"seasonality filter", "", "filters { seasonality intraday 90d supports entry }"},
		{"cooldown", " wait 12 candles after trade", ""},
		{"max hold", " maxHoldCandles 10", ""},
		{"breakeven", " breakeven off", ""},
		{"movement", "", "filters { movement below 1.1 }"},
		{"sma", " sma fast 50", ""},
		{"unknown directive", " countdown 13", ""},
	} {
		result := parseLegacy(t, legacySetup9Source(profile+"\n"+extra.setup, extra.filters))
		if len(result.Errors) == 0 {
			t.Errorf("%s: parsed without error; a knob the profile does not own was accepted", extra.name)
		}
	}
}

func TestLegacySetup9RiskUsdIsValidated(t *testing.T) {
	good := parseLegacy(t, legacySetup9Source(" sequential profile seq.legacy.setup9.v1", "risk { riskUsd 150 }"))
	if len(good.Errors) != 0 || good.Config["riskUsd"] != 150.0 {
		t.Fatalf("errors %v riskUsd %v", good.Errors, good.Config["riskUsd"])
	}
	for _, bad := range []string{"riskUsd abc", "riskUsd 0", "riskUsd -5", "riskUsd 100 200", "riskUsd"} {
		result := parseLegacy(t, legacySetup9Source(" sequential profile seq.legacy.setup9.v1", "risk { "+bad+" }"))
		if !strings.Contains(strings.Join(result.Errors, "\n"), "riskUsd must be one finite positive number") {
			t.Errorf("%q: errors %v", bad, result.Errors)
		}
	}
}

func TestSequentialDirectiveIsUnknownOutsideLegacySetup9(t *testing.T) {
	result := parseLegacy(t, "dsl v7\nsetup {\n type: opening range breakout\n sequential profile seq.legacy.setup9.v1\n}\n")
	if len(result.Errors) == 0 {
		t.Fatal("sequential directive accepted by another family")
	}
}

func TestLegacySetup9DoesNotChangeOtherFamilyConfig(t *testing.T) {
	// A new family must not add a default key that every other family's
	// parse golden would then carry.
	result := parseLegacy(t, "dsl v7\nsetup {\n type: opening range breakout\n}\n")
	if _, present := result.Config["legacySetup9"]; present {
		t.Fatal("legacySetup9 key leaked into another family's config")
	}
}

func TestLegacySetup9ManifestBindings(t *testing.T) {
	if generatedCanonicalSetupFamilies["legacy setup 9"] != string(FamilyLegacySetup9) {
		t.Fatal("generated family table lacks legacy setup 9")
	}
	if parserDirectiveHandlerBindings["directive.sequential"] == "" || parserFamilyHandlerBindings["legacySetup9"] == "" {
		t.Fatal("handler bindings missing")
	}
}
