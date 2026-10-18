package engine

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestPriceMomentumStopConfigRejectsInvalidExplicitFixedATR(t *testing.T) {
	base := func(stop map[string]any) dsl.Config {
		return dsl.Config{"setupType": string(dsl.FamilyPriceMomentum), "stop": stop}
	}
	for name, value := range map[string]any{
		"zero":     0.0,
		"negative": -1.0,
		"NaN":      math.NaN(),
		"+Inf":     math.Inf(1),
		"-Inf":     math.Inf(-1),
		"string":   "2",
		"missing":  nil,
	} {
		t.Run(name, func(t *testing.T) {
			stop := map[string]any{"type": "fixedAtr", "minAtr": 0.2, "maxAtr": 3.0}
			if value != nil {
				stop["atr"] = value
			}
			if err := validatePriceMomentumStopConfig(base(stop)); err == nil {
				t.Fatalf("explicit fixed ATR stop %v was accepted", value)
			}
		})
	}
	for name, cfg := range map[string]dsl.Config{
		"valid fixed ATR":     base(map[string]any{"type": "fixedAtr", "atr": 2.0}),
		"valid integer value": base(map[string]any{"type": "fixedAtr", "atr": 2}),
		"absent fixed stop":   base(map[string]any{"type": "structure", "extremeCandles": 3.0}),
		"no stop map":         {"setupType": string(dsl.FamilyPriceMomentum)},
		// Scoped to price momentum: other families keep their own contracts.
		"other family": {"setupType": string(dsl.FamilyOpeningRangeBreakout), "stop": map[string]any{"type": "fixedAtr", "atr": 0.0}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validatePriceMomentumStopConfig(cfg); err != nil {
				t.Fatalf("unexpected rejection: %v", err)
			}
		})
	}
}

func TestPublicRunRejectsInvalidPriceMomentumFixedATRStop(t *testing.T) {
	fixture := loadPriceMomentumATRStopFixture(t)
	for name, value := range map[string]any{"zero": 0.0, "negative": -1.0, "+Inf": math.Inf(1)} {
		t.Run(name, func(t *testing.T) {
			cfg := parsePriceMomentumATRStopCase(t)
			stop := mapValue(cfg, "stop")
			stop["atr"] = value
			cfg["stop"] = stop
			_, err := Run(RunRequest{
				Config:          cfg,
				Series:          marketdata.SeriesFromBars(fixture.Bars),
				HTFSeries:       marketdata.SeriesFromBars(fixture.HTFBars),
				StrategyID:      fixture.StrategyID,
				Symbol:          fixture.Symbol,
				Timeframe:       fixture.Timeframe,
				HigherTimeframe: fixture.HigherTimeframe,
				RangeMethod:     fixture.RangeMethod,
				Costs:           fixture.Costs,
			})
			if err == nil {
				t.Fatalf("run accepted fixed ATR stop %v instead of refusing it", value)
			}
		})
	}
}

func TestParsePriceMomentumRejectsInvalidExplicitFixedATRStop(t *testing.T) {
	for _, multiple := range []string{"0", "-1", "-0", "NaN", "Inf", "-Inf"} {
		t.Run(multiple, func(t *testing.T) {
			parsed, err := dsl.Parse(priceMomentumSource("stop " + multiple + " ATR"))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(parsed.Errors) == 0 {
				t.Fatalf("stop %s ATR parsed without a diagnostic; config stop = %v", multiple, parsed.Config["stop"])
			}
		})
	}
	for _, line := range []string{"stop 2 ATR", "stop 0.25 ATR", "stop beyond last 3 candle extreme by 0.25 ATR"} {
		t.Run(line, func(t *testing.T) {
			parsed, err := dsl.Parse(priceMomentumSource(line))
			if err != nil || len(parsed.Errors) != 0 {
				t.Fatalf("%q rejected: err=%v diagnostics=%v", line, err, parsed.Errors)
			}
		})
	}
}

func priceMomentumSource(stopLine string) string {
	return `dsl v7
strategy "PM stop" {
  description "Fixed ATR stop validation."
}

market conditions {
  slices(XAUUSD 1h)
}

setup {
  type: price momentum
  lookback 20 candles
  neutral zone 0.5 percent
}

risk {
  ` + stopLine + `
}

execution {
  risk 200 USD
}
`
}

const priceMomentumATRStopCase = "family-price-momentum-atr-stop"

func loadPriceMomentumATRStopFixture(t *testing.T) RunFixture {
	t.Helper()
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), priceMomentumATRStopCase+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	return fixture
}

// parsePriceMomentumATRStopCase parses the committed conformance source afresh,
// so each subtest mutates its own config.
func parsePriceMomentumATRStopCase(t *testing.T) dsl.Config {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), priceMomentumATRStopCase+".strat"))
	if err != nil {
		t.Fatalf("read DSL source: %v", err)
	}
	parsed, err := dsl.Parse(string(source))
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("parse DSL: err=%v diagnostics=%v", err, parsed.Errors)
	}
	return parsed.Config
}
