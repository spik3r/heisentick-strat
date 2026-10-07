package dsl

import (
	"reflect"
	"strings"
	"testing"
)

func TestAdaptiveFlagFullSourceAndEquivalentSyntax(t *testing.T) {
	want := adaptiveFlagParseOK(t, adaptiveFlagTestSource)
	for _, s := range []string{strings.ReplaceAll(adaptiveFlagTestSource, "\n", " "), "# type: anything\n" + adaptiveFlagTestSource, strings.ReplaceAll(strings.ReplaceAll(adaptiveFlagTestSource, "adaptiveflag ", "ADAPTIVEFLAG "), "type:", "TYPE:"), strings.ReplaceAll(adaptiveFlagTestSource, "0.05", ".05")} {
		if got := adaptiveFlagParseOK(t, s); !reflect.DeepEqual(want, got) {
			t.Fatal("equivalent syntax changed config")
		}
	}
	for _, old := range []string{"useVolumeFilter true", "useEMATrend true"} {
		adaptiveFlagReject(t, strings.Replace(adaptiveFlagTestSource, old, strings.Replace(old, "true", "false", 1), 1))
	}
}

func TestAdaptiveFlagStrictSourceRejections(t *testing.T) {
	replacements := [][2]string{
		{"dsl v7", "dsl v6"}, {"dsl v7", ""}, {"type: adaptive volume flag", "type: adaptive volume flag type: flag continuation"},
		{"type: adaptive volume flag", "type: flag continuation type: adaptive volume flag"}, {"type: adaptive volume flag", ""},
		{"adaptiveflag bundle INITIAL", ""}, {"adaptiveflag policy DELAYED_OHLC_REFERENCE_V1", ""}, {"adaptiveflag timeframe M30", ""},
		{"adaptiveflag timeframe M30", "adaptiveflag timeframe M30 from M5"}, {"adaptiveflag bundle INITIAL", "adaptiveflag bundle initial"},
		{"adaptiveflag minPoleATR 1.8", "adaptiveflag minPoleATR 1.800000000000000001"},
		{"adaptiveflag pivotSensitivity 3", "adaptiveflag pivotSensitivity 3.0"}, {"adaptiveflag targetR 2.5", "adaptiveflag targetR NaN"},
		{"adaptiveflag targetR 2.5", "adaptiveflag targetR -2.5"}, {"adaptiveflag useEMATrend true", "adaptiveflag useEMATrend 1"},
		{"adaptiveflag atrStopMult 1.2", "adaptiveflag atrStopMult 1.2 adaptiveflag atrStopMult 1.2"},
		{"adaptiveflag targetR 2.5", "adaptiveflag targetR 2.5 adaptiveflag riskUSD 100"},
	}
	for _, r := range replacements {
		adaptiveFlagReject(t, strings.Replace(adaptiveFlagTestSource, r[0], r[1], 1))
	}
	for field := range adaptiveFlagRuleTypes {
		lines := strings.Split(adaptiveFlagTestSource, "\n")
		out := []string{}
		for _, line := range lines {
			if !strings.Contains(line, "adaptiveflag "+field+" ") {
				out = append(out, line)
			}
		}
		adaptiveFlagReject(t, strings.Join(out, "\n"))
	}
	for _, tail := range []string{" ignored", "}", " risk { risk 100 USD }", " setup {}"} {
		adaptiveFlagReject(t, adaptiveFlagTestSource+tail)
	}
}

func TestAdaptiveFlagMalformedReservedIntentCannotFallback(t *testing.T) {
	for _, clause := range []string{
		"type: adaptive volume flag", "type: adaptiveVolumeFlag", "type: ADAPTIVE_VOLUME_FLAG", "type: \"adaptive volume flag\"", "type==adaptive-volume-flag",
		"type: adaptive volume flag type: flag continuation", "type: flag continuation type: adaptive volume flag", "unknown adaptiveVolumeFlag null", "stop 2 ATR adaptiveflag targetR 2.5", "risk 100 USD adaptiveflag policy broken", "type: flag continuation adaptiveflag\npolicy bad", "type\u2003:\u2003adaptive\u2003volume\u2003flag",
	} {
		adaptiveFlagReject(t, "dsl v7\nstrategy \"Adversarial\"\nsetup { "+clause+" }")
	}
	// Genuine metadata/list data must remain opaque and preserve legacy behavior.
	for _, source := range []string{
		"dsl v7\nstrategy \"adaptiveVolumeFlag\"\nsetup { type: flag continuation }",
		"dsl v7\nstrategy \"Legacy\"\n description \"type: adaptive volume flag\"\nsetup { type: flag continuation }",
		"dsl v7\nstrategy \"Legacy\"\nmarket { symbols (adaptiveVolumeFlag) }\nsetup { type: flag continuation }",
	} {
		if adaptiveFlagSourceCandidate(source) {
			t.Fatalf("metadata manufactured reserved code: %s", source)
		}
	}
}

func TestAdaptiveFlagPunctuationTailsCannotBecomeLegacyStrategy(t *testing.T) {
	for _, head := range []string{"adaptive:flag", "adaptive.flag", "adaptive/flag", "adaptive | flag", "adaptive : volume : flag", "ADAPTIVE+FLAG", "adaptive‧flag"} {
		for _, source := range []string{
			"dsl v7\nstrategy \"Review\"\nsetup { type: flag continuation }\nrisk { stop 2 ATR " + head + " targetR 2.5 }",
			"dsl v7\nstrategy \"Review\"\nsetup { type: flag continuation }\nrisk 100 USD " + head + " targetR 2.5",
		} {
			adaptiveFlagReject(t, source)
		}
	}
	for _, key := range []string{"adaptive:volume:flag", "adaptive.flag", "adaptive/flag", "adaptive | flag"} {
		if !IsAdaptiveVolumeFlagReserved(Config{key: nil, "setupType": "flagContinuation"}) || !IsAdaptiveVolumeFlagReserved(Config{"setupType": key}) {
			t.Fatal("punctuation key/family bypass", key)
		}
	}
}
