package engine

import (
	"math"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const doubleTopPivotValidationError = "engine config doubleTopBottom.pivotWindow must be non-negative"

func TestDoubleTopPivotValidationRejectsNegativeValuesAcrossPublicBoundaries(t *testing.T) {
	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-double-top-bottom")

	for _, tt := range []struct {
		name  string
		value any
	}{
		{name: "negative one", value: float64(-1)},
		{name: "minimum integer numeric", value: int64(math.MinInt64)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, setupType := range []struct {
				name  string
				value any
			}{
				{name: "string family", value: string(dsl.FamilyDoubleTopBottom)},
				{name: "named family", value: dsl.FamilyDoubleTopBottom},
			} {
				t.Run(setupType.name, func(t *testing.T) {
					cfg := cloneTestConfig(t, reviewedConfig)
					cfg["setupType"] = setupType.value
					mapValue(cfg, "doubleTopBottom")["pivotWindow"] = tt.value
					request := doubleTopPivotRequest(fixture, cfg)

					for _, flow := range validationPublicFlows() {
						t.Run(flow.name, func(t *testing.T) {
							err := runWithoutPanic(t, func() error { return flow.run(request) })
							if err == nil || err.Error() != doubleTopPivotValidationError {
								t.Fatalf("%s error = %v, want %q", flow.name, err, doubleTopPivotValidationError)
							}
						})
					}
				})
			}
		})
	}
}

func TestDoubleTopPivotValidationRejectsNegativeSharedVariant(t *testing.T) {
	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-double-top-bottom")
	shared, err := PrepareSharedRunContext(doubleTopPivotRequest(fixture, reviewedConfig))
	if err != nil {
		t.Fatalf("prepare reviewed shared context: %v", err)
	}

	for _, value := range []any{float64(-1), int64(math.MinInt64)} {
		cfg := cloneTestConfig(t, reviewedConfig)
		mapValue(cfg, "doubleTopBottom")["pivotWindow"] = value
		if _, err := shared.PrepareVariant(cfg); err == nil || err.Error() != doubleTopPivotValidationError {
			t.Fatalf("PrepareVariant pivotWindow %v error = %v, want %q", value, err, doubleTopPivotValidationError)
		}
	}
}

func TestDoubleTopPivotValidationAcceptsAbsentZeroAndPositiveValues(t *testing.T) {
	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-double-top-bottom")

	for _, tt := range []struct {
		name   string
		mutate func(dsl.Config)
	}{
		{
			name: "absent uses default",
			mutate: func(cfg dsl.Config) {
				delete(mapValue(cfg, "doubleTopBottom"), "pivotWindow")
			},
		},
		{
			name: "zero",
			mutate: func(cfg dsl.Config) {
				mapValue(cfg, "doubleTopBottom")["pivotWindow"] = float64(0)
			},
		},
		{
			name: "positive",
			mutate: func(cfg dsl.Config) {
				mapValue(cfg, "doubleTopBottom")["pivotWindow"] = float64(2)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cloneTestConfig(t, reviewedConfig)
			tt.mutate(cfg)
			request := doubleTopPivotRequest(fixture, cfg)
			for _, flow := range validationPublicFlows() {
				t.Run(flow.name, func(t *testing.T) {
					if err := runWithoutPanic(t, func() error { return flow.run(request) }); err != nil {
						t.Fatalf("%s rejected accepted pivot window: %v", flow.name, err)
					}
				})
			}
		})
	}
}

func TestDoubleTopPivotValidationIgnoresInactiveSection(t *testing.T) {
	request := validationRequest(validationSeries(), marketdata.Series{})
	request.Config["doubleTopBottom"] = dsl.Config{"pivotWindow": float64(-1)}

	for _, flow := range validationPublicFlows() {
		t.Run(flow.name, func(t *testing.T) {
			if err := runWithoutPanic(t, func() error { return flow.run(request) }); err != nil {
				t.Fatalf("%s rejected inactive DoubleTopBottom section: %v", flow.name, err)
			}
		})
	}
}

func TestDoubleTopPivotValidationKeepsExistingPrecedence(t *testing.T) {
	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-double-top-bottom")
	if _, err := SharedContextKey(RunRequest{}); err == nil || err.Error() != "engine config is required" {
		t.Fatalf("nil request config error = %v, want engine config precedence", err)
	}
	unsupported := cloneTestConfig(t, reviewedConfig)
	unsupported["setupType"] = "unsupported"
	mapValue(unsupported, "doubleTopBottom")["pivotWindow"] = float64(-1)
	if _, err := SharedContextKey(doubleTopPivotRequest(fixture, unsupported)); err == nil || err.Error() != `setup family "unsupported" is not implemented` {
		t.Fatalf("unsupported request error = %v, want setup family precedence", err)
	}

	shared, err := PrepareSharedRunContext(doubleTopPivotRequest(fixture, reviewedConfig))
	if err != nil {
		t.Fatalf("prepare reviewed shared context: %v", err)
	}

	if _, err := shared.PrepareVariant(nil); err == nil || err.Error() != "engine config is required" {
		t.Fatalf("nil variant error = %v, want engine config precedence", err)
	}
	if _, err := shared.PrepareVariant(unsupported); err == nil || err.Error() != `setup family "unsupported" is not implemented` {
		t.Fatalf("unsupported variant error = %v, want setup family precedence", err)
	}
}

func doubleTopPivotRequest(fixture RunFixture, cfg dsl.Config) RunRequest {
	return RunRequest{
		Config:          cfg,
		Series:          marketdata.SeriesFromBars(fixture.Bars),
		HTFSeries:       marketdata.SeriesFromBars(fixture.HTFBars),
		StrategyID:      fixture.StrategyID,
		Symbol:          fixture.Symbol,
		Timeframe:       fixture.Timeframe,
		HigherTimeframe: fixture.HigherTimeframe,
		RangeMethod:     fixture.RangeMethod,
		Costs:           fixture.Costs,
	}
}
