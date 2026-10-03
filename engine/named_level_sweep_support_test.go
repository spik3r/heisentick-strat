package engine

import (
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestNamedLevelSweepUnsupportedParsedLevelFailsClosed(t *testing.T) {
	source := `dsl v7
setup { type: named level sweep }
entry { when price sweeps EMA and closes back below it then signal short }
`
	parsed, err := dsl.Parse(source)
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("valid shared DSL phrase did not parse: %v, %v", err, parsed.Errors)
	}
	rules := mapValue(mapValue(parsed.Config, "namedLevelSweep"), "rules")
	if rules["EMA"] == nil {
		t.Fatalf("parsed EMA rule missing: %v", rules)
	}
	bars := []marketdata.Bar{{T: 0, O: 100, H: 101, L: 99, C: 100, V: 1}}
	request := RunRequest{Config: parsed.Config, Series: marketdata.SeriesFromBars(bars), Symbol: "XAUUSD", Timeframe: "15m"}
	for entry, run := range map[string]func() error{
		"fixture": func() error {
			_, err := RunFixtureCase(RunFixture{Bars: bars, Symbol: "XAUUSD", Timeframe: "15m"}, source)
			return err
		},
		"run":     func() error { _, err := Run(request); return err },
		"shared":  func() error { _, err := PrepareSharedRunContext(request); return err },
		"variant": func() error { _, err := (&SharedRunContext{}).PrepareVariant(parsed.Config); return err },
	} {
		if err := run(); err == nil || !strings.Contains(err.Error(), `named level sweep level "EMA" is not implemented`) {
			t.Errorf("%s error = %v, want unsupported EMA level", entry, err)
		}
	}
}

func TestNamedLevelSweepMalformedDirectConfigFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  dsl.Config
		want string
	}{
		{"scalar config", dsl.Config{"setupType": string(dsl.FamilyNamedLevelSweep), "namedLevelSweep": "bad"}, "config must be an object"},
		{"scalar rules", dsl.Config{"setupType": string(dsl.FamilyNamedLevelSweep), "namedLevelSweep": map[string]any{"rules": "bad"}}, "rules must be an object"},
		{"scalar rule", dsl.Config{"setupType": string(dsl.FamilyNamedLevelSweep), "namedLevelSweep": map[string]any{"rules": map[string]any{"PDH": "bad"}}}, `rule "PDH" must be an object`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateNamedLevelSweepSupport(tc.cfg); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestNamedLevelSweepNestedConfigRulesReachRuntime(t *testing.T) {
	cfg := dsl.Config{
		"setupType": string(dsl.FamilyNamedLevelSweep),
		"namedLevelSweep": dsl.Config{"rules": dsl.Config{
			"PDH": dsl.Config{"mode": "sweep", "side": "short", "reclaimCandles": float64(2)},
		}},
	}
	if err := validateNamedLevelSweepSupport(cfg); err != nil {
		t.Fatalf("valid nested config rejected: %v", err)
	}
	rule := namedLevelSweepParamsFromConfig(cfg).Rules["PDH"]
	if rule.Mode != "sweep" || rule.Side != sideShort || rule.ReclaimCandles != 2 {
		t.Fatalf("nested rule discarded: %+v", rule)
	}
}
