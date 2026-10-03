package engine

import (
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestBreakRetestUnsupportedParsedFeaturesFailClosedAcrossGoEntryPoints(t *testing.T) {
	bars := []marketdata.Bar{
		{T: 0, O: 100, H: 101, L: 99, C: 100, V: 1},
		{T: htfQuarter, O: 100, H: 105, L: 99, C: 104, V: 1},
		{T: 2 * htfQuarter, O: 104, H: 104, L: 101, C: 102, V: 1},
	}
	for _, tc := range []struct {
		name, phrase, configKey, want string
	}{
		{"swing levels", "swing levels lookback 60 candles pivot 1 count 4", "levelSource", "level source \"swing\" is not implemented"},
		{"EMA confluence", "ema confluence within 0.25 ATR", "emaConfluenceAtr", "EMA confluence is not implemented"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := "dsl v7\nsetup {\n  type: break retest\n  " + tc.phrase + "\n}"
			parsed, err := dsl.Parse(source)
			if err != nil || len(parsed.Errors) != 0 {
				t.Fatalf("valid v7 phrase did not parse: %v, %v", err, parsed.Errors)
			}
			if mapValue(parsed.Config, "breakRetest")[tc.configKey] == nil {
				t.Fatalf("parsed break-retest config did not retain %s", tc.configKey)
			}
			request := RunRequest{
				Config: parsed.Config, Series: marketdata.SeriesFromBars(bars),
				Symbol: "XAUUSD", Timeframe: "15m",
			}
			assertUnsupported := func(entry string, err error) {
				t.Helper()
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Errorf("%s error = %v, want %q", entry, err, tc.want)
				}
			}
			_, err = RunFixtureCase(RunFixture{Bars: bars, Symbol: "XAUUSD", Timeframe: "15m"}, source)
			assertUnsupported("fixture", err)
			_, err = Run(request)
			assertUnsupported("run", err)
			_, err = PrepareRun(request)
			assertUnsupported("prepared", err)
			_, err = PrepareSharedRunContext(request)
			assertUnsupported("shared", err)
			_, err = (&SharedRunContext{}).PrepareVariant(parsed.Config)
			assertUnsupported("variant", err)
		})
	}
}

func TestBreakRetestKeyLevelsAndInactiveEMAConfluenceRemainSupported(t *testing.T) {
	for _, cfg := range []dsl.Config{
		{"setupType": string(dsl.FamilyBreakRetest), "breakRetest": map[string]any{"levelSource": "key", "emaConfluenceAtr": float64(0)}},
		{"setupType": string(dsl.FamilyBreakRetest)},
		{"setupType": string(dsl.FamilyFailedBreakout), "breakRetest": map[string]any{"levelSource": "swing"}},
	} {
		if err := validateBreakRetestSupport(cfg); err != nil {
			t.Errorf("unaffected config %v rejected: %v", cfg, err)
		}
	}
}

func TestBreakRetestMalformedDirectConfigDoesNotFallBackToKeyLevels(t *testing.T) {
	for _, value := range []any{nil, 123, []string{"swing"}} {
		cfg := dsl.Config{"setupType": string(dsl.FamilyBreakRetest), "breakRetest": map[string]any{"levelSource": value}}
		if err := validateBreakRetestSupport(cfg); err == nil || !strings.Contains(err.Error(), "must be a string") {
			t.Errorf("levelSource %v error = %v, want type diagnostic", value, err)
		}
	}
	cfg := dsl.Config{"setupType": string(dsl.FamilyBreakRetest), "breakRetest": "swing"}
	if err := validateBreakRetestSupport(cfg); err == nil || !strings.Contains(err.Error(), "must be an object") {
		t.Errorf("scalar breakRetest error = %v, want object diagnostic", err)
	}
}
