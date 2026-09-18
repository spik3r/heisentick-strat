package engine

import (
	"math"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestValidateRunRequestRejectsMalformedSeriesColumns(t *testing.T) {
	valid := validationSeries()
	tests := []struct {
		name    string
		mutate  func(*RunRequest)
		wantErr string
	}{
		{
			name:    "market timestamp shorter than core columns",
			mutate:  func(request *RunRequest) { request.Series.T = request.Series.T[:1] },
			wantErr: "market series open column length 2 does not match timestamp length 1",
		},
		{
			name:    "market missing timestamp with populated core columns",
			mutate:  func(request *RunRequest) { request.Series.T = nil },
			wantErr: "market series is empty",
		},
		{
			name:    "market missing open",
			mutate:  func(request *RunRequest) { request.Series.O = nil },
			wantErr: "market series open column length 0 does not match timestamp length 2",
		},
		{
			name:    "market short high",
			mutate:  func(request *RunRequest) { request.Series.H = request.Series.H[:1] },
			wantErr: "market series high column length 1 does not match timestamp length 2",
		},
		{
			name:    "market short low",
			mutate:  func(request *RunRequest) { request.Series.L = request.Series.L[:1] },
			wantErr: "market series low column length 1 does not match timestamp length 2",
		},
		{
			name:    "market short close",
			mutate:  func(request *RunRequest) { request.Series.C = request.Series.C[:1] },
			wantErr: "market series close column length 1 does not match timestamp length 2",
		},
		{
			name:    "market short volume",
			mutate:  func(request *RunRequest) { request.Series.V = request.Series.V[:1] },
			wantErr: "market series volume column length 1 must be zero or match timestamp length 2",
		},
		{
			name:    "higher-timeframe timestamp shorter than core columns",
			mutate:  func(request *RunRequest) { request.HTFSeries.T = request.HTFSeries.T[:1] },
			wantErr: "higher-timeframe series open column length 2 does not match timestamp length 1",
		},
		{
			name:    "higher-timeframe missing timestamp with populated core columns",
			mutate:  func(request *RunRequest) { request.HTFSeries.T = nil },
			wantErr: "higher-timeframe series open column length 2 does not match timestamp length 0",
		},
		{
			name:    "higher-timeframe missing open",
			mutate:  func(request *RunRequest) { request.HTFSeries.O = nil },
			wantErr: "higher-timeframe series open column length 0 does not match timestamp length 2",
		},
		{
			name:    "higher-timeframe short high",
			mutate:  func(request *RunRequest) { request.HTFSeries.H = request.HTFSeries.H[:1] },
			wantErr: "higher-timeframe series high column length 1 does not match timestamp length 2",
		},
		{
			name:    "higher-timeframe short low",
			mutate:  func(request *RunRequest) { request.HTFSeries.L = request.HTFSeries.L[:1] },
			wantErr: "higher-timeframe series low column length 1 does not match timestamp length 2",
		},
		{
			name:    "higher-timeframe short close",
			mutate:  func(request *RunRequest) { request.HTFSeries.C = request.HTFSeries.C[:1] },
			wantErr: "higher-timeframe series close column length 1 does not match timestamp length 2",
		},
		{
			name:    "higher-timeframe short volume",
			mutate:  func(request *RunRequest) { request.HTFSeries.V = request.HTFSeries.V[:1] },
			wantErr: "higher-timeframe series volume column length 1 must be zero or match timestamp length 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := validationRequest(valid, valid)
			tt.mutate(&request)
			if err := validateRunRequest(request); err == nil || err.Error() != tt.wantErr {
				t.Fatalf("validateRunRequest error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestPublicRunFlowsRejectMalformedSeriesWithoutPanicking(t *testing.T) {
	valid := validationSeries()
	tests := []struct {
		name    string
		mutate  func(*RunRequest)
		wantErr string
	}{
		{
			name:    "market close",
			mutate:  func(request *RunRequest) { request.Series.C = request.Series.C[:1] },
			wantErr: "market series close column length 1 does not match timestamp length 2",
		},
		{
			name:    "higher-timeframe high",
			mutate:  func(request *RunRequest) { request.HTFSeries.H = request.HTFSeries.H[:1] },
			wantErr: "higher-timeframe series high column length 1 does not match timestamp length 2",
		},
	}
	flows := []struct {
		name string
		run  func(RunRequest) error
	}{
		{name: "SharedContextKey", run: func(request RunRequest) error { _, err := SharedContextKey(request); return err }},
		{name: "PrepareSharedRunContext", run: func(request RunRequest) error { _, err := PrepareSharedRunContext(request); return err }},
		{name: "PrepareRun", run: func(request RunRequest) error { _, err := PrepareRun(request); return err }},
		{name: "Run", run: func(request RunRequest) error { _, err := Run(request); return err }},
	}

	for _, tt := range tests {
		for _, flow := range flows {
			t.Run(tt.name+"/"+flow.name, func(t *testing.T) {
				request := validationRequest(valid, valid)
				tt.mutate(&request)
				err := runWithoutPanic(t, func() error { return flow.run(request) })
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("%s error = %v, want %q", flow.name, err, tt.wantErr)
				}
			})
		}
	}
}

func TestPublicRunFlowsRejectNonFiniteCoreSeriesValues(t *testing.T) {
	forms := []struct {
		name  string
		value float64
	}{
		{name: "nan", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
	}
	columns := []struct {
		name   string
		values func(*marketdata.Series) []float64
	}{
		{name: "timestamp", values: func(series *marketdata.Series) []float64 { return series.T }},
		{name: "open", values: func(series *marketdata.Series) []float64 { return series.O }},
		{name: "high", values: func(series *marketdata.Series) []float64 { return series.H }},
		{name: "low", values: func(series *marketdata.Series) []float64 { return series.L }},
		{name: "close", values: func(series *marketdata.Series) []float64 { return series.C }},
	}
	seriesCases := []struct {
		name   string
		label  string
		series func(*RunRequest) *marketdata.Series
	}{
		{name: "market", label: "market", series: func(request *RunRequest) *marketdata.Series { return &request.Series }},
		{name: "higher-timeframe", label: "higher-timeframe", series: func(request *RunRequest) *marketdata.Series { return &request.HTFSeries }},
	}

	for _, seriesCase := range seriesCases {
		for _, column := range columns {
			for _, form := range forms {
				for _, flow := range validationPublicFlows() {
					name := seriesCase.name + "/" + column.name + "/" + form.name + "/" + flow.name
					t.Run(name, func(t *testing.T) {
						request := validationRequest(validationSeries(), validationSeries())
						column.values(seriesCase.series(&request))[1] = form.value
						wantErr := seriesCase.label + " series " + column.name + " column contains non-finite value at index 1"
						err := runWithoutPanic(t, func() error { return flow.run(request) })
						if err == nil || err.Error() != wantErr {
							t.Fatalf("%s error = %v, want %q", flow.name, err, wantErr)
						}
					})
				}
			}
		}
	}
}

func TestRunRequestValidationPreservesVolumeAndTimestampPolicies(t *testing.T) {
	for _, form := range []struct {
		name  string
		value float64
	}{
		{name: "nan", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
	} {
		for _, seriesCase := range []struct {
			name   string
			volume func(*RunRequest) []float64
		}{
			{name: "market", volume: func(request *RunRequest) []float64 { return request.Series.V }},
			{name: "higher-timeframe", volume: func(request *RunRequest) []float64 { return request.HTFSeries.V }},
		} {
			for _, flow := range validationPublicFlows() {
				name := "nonfinite volume/" + seriesCase.name + "/" + form.name + "/" + flow.name
				t.Run(name, func(t *testing.T) {
					request := validationRequest(validationSeries(), validationSeries())
					seriesCase.volume(&request)[1] = form.value
					if err := runWithoutPanic(t, func() error { return flow.run(request) }); err != nil {
						t.Fatalf("%s rejected nonfinite volume: %v", flow.name, err)
					}
				})
			}
		}
	}

	for _, timestampCase := range []struct {
		name   string
		mutate func(*RunRequest)
	}{
		{name: "duplicate market", mutate: func(request *RunRequest) { request.Series.T[1] = request.Series.T[0] }},
		{name: "descending market", mutate: func(request *RunRequest) {
			request.Series.T[0], request.Series.T[1] = request.Series.T[1], request.Series.T[0]
		}},
		{name: "duplicate higher-timeframe", mutate: func(request *RunRequest) { request.HTFSeries.T[1] = request.HTFSeries.T[0] }},
		{name: "descending higher-timeframe", mutate: func(request *RunRequest) {
			request.HTFSeries.T[0], request.HTFSeries.T[1] = request.HTFSeries.T[1], request.HTFSeries.T[0]
		}},
	} {
		for _, flow := range validationPublicFlows() {
			t.Run("finite timestamps/"+timestampCase.name+"/"+flow.name, func(t *testing.T) {
				request := validationRequest(validationSeries(), validationSeries())
				timestampCase.mutate(&request)
				if err := runWithoutPanic(t, func() error { return flow.run(request) }); err != nil {
					t.Fatalf("%s rejected finite timestamp policy input: %v", flow.name, err)
				}
			})
		}
	}
}

func TestRunRequestAllowsOptionalVolumeAndEmptyHigherTimeframe(t *testing.T) {
	t.Run("missing market and higher-timeframe volume use NaN without mutating caller", func(t *testing.T) {
		market := validationSeries()
		htf := validationSeries()
		market.V = nil
		htf.V = nil
		originalMarket := market
		originalHTF := htf
		request := validationRequest(market, htf)

		shared, err := PrepareSharedRunContext(request)
		if err != nil {
			t.Fatalf("PrepareSharedRunContext: %v", err)
		}
		if len(shared.series.V) != shared.series.Len() {
			t.Fatalf("normalized market volume length = %d, want %d", len(shared.series.V), shared.series.Len())
		}
		for i, volume := range shared.series.V {
			if !math.IsNaN(volume) {
				t.Fatalf("normalized market volume[%d] = %v, want NaN", i, volume)
			}
		}
		if request.Series.V != nil || request.HTFSeries.V != nil {
			t.Fatalf("caller volume mutated: market=%v htf=%v", request.Series.V, request.HTFSeries.V)
		}
		if !reflect.DeepEqual(request.Series, originalMarket) || !reflect.DeepEqual(request.HTFSeries, originalHTF) {
			t.Fatal("normalization mutated caller series columns")
		}
		if &shared.series.C[0] != &request.Series.C[0] {
			t.Fatal("normalization copied required market columns instead of preserving the shallow series view")
		}
		if _, err := Run(request); err != nil {
			t.Fatalf("Run with missing optional volume: %v", err)
		}
	})

	t.Run("full volume is preserved", func(t *testing.T) {
		market := validationSeries()
		request := validationRequest(market, marketdata.Series{})
		shared, err := PrepareSharedRunContext(request)
		if err != nil {
			t.Fatalf("PrepareSharedRunContext: %v", err)
		}
		if &shared.series.V[0] != &request.Series.V[0] || !reflect.DeepEqual(shared.series.V, request.Series.V) {
			t.Fatalf("full volume replaced: got %v want shared %v", shared.series.V, request.Series.V)
		}
	})

	t.Run("entirely empty higher timeframe is valid", func(t *testing.T) {
		request := validationRequest(validationSeries(), marketdata.Series{})
		for _, flow := range []struct {
			name string
			run  func() error
		}{
			{name: "SharedContextKey", run: func() error { _, err := SharedContextKey(request); return err }},
			{name: "PrepareSharedRunContext", run: func() error { _, err := PrepareSharedRunContext(request); return err }},
			{name: "PrepareRun", run: func() error { _, err := PrepareRun(request); return err }},
			{name: "Run", run: func() error { _, err := Run(request); return err }},
		} {
			t.Run(flow.name, func(t *testing.T) {
				if err := runWithoutPanic(t, flow.run); err != nil {
					t.Fatalf("%s with empty higher timeframe: %v", flow.name, err)
				}
			})
		}
	})
}

func TestValidateRunRequestKeepsExistingErrorPrecedence(t *testing.T) {
	valid := validationSeries()
	tests := []struct {
		name    string
		request RunRequest
		wantErr string
	}{
		{
			name:    "nil config precedes empty market",
			request: RunRequest{},
			wantErr: "engine config is required",
		},
		{
			name: "empty market timestamps precede populated column shapes",
			request: validationRequest(marketdata.Series{
				O: []float64{100}, H: []float64{101}, L: []float64{99}, C: []float64{100},
			}, marketdata.Series{}),
			wantErr: "market series is empty",
		},
		{
			name: "unsupported family precedes malformed market shape",
			request: func() RunRequest {
				request := validationRequest(valid, valid)
				request.Config = dsl.Config{"setupType": "unsupported"}
				request.Series.C = request.Series.C[:1]
				return request
			}(),
			wantErr: `setup family "unsupported" is not implemented`,
		},
		{
			name: "unsupported family precedes malformed higher-timeframe shape",
			request: func() RunRequest {
				request := validationRequest(valid, valid)
				request.Config = dsl.Config{"setupType": "unsupported"}
				request.HTFSeries.H = request.HTFSeries.H[:1]
				return request
			}(),
			wantErr: `setup family "unsupported" is not implemented`,
		},
		{
			name: "unsupported family precedes nonfinite market value",
			request: func() RunRequest {
				request := validationRequest(validationSeries(), validationSeries())
				request.Config = dsl.Config{"setupType": "unsupported"}
				request.Series.C[0] = math.NaN()
				return request
			}(),
			wantErr: `setup family "unsupported" is not implemented`,
		},
		{
			name: "market shape precedes nonfinite values",
			request: func() RunRequest {
				request := validationRequest(validationSeries(), validationSeries())
				request.Series.C = request.Series.C[:1]
				request.Series.T[0] = math.NaN()
				return request
			}(),
			wantErr: "market series close column length 1 does not match timestamp length 2",
		},
		{
			name: "higher-timeframe shape precedes market nonfinite values",
			request: func() RunRequest {
				request := validationRequest(validationSeries(), validationSeries())
				request.HTFSeries.H = request.HTFSeries.H[:1]
				request.Series.T[0] = math.NaN()
				return request
			}(),
			wantErr: "higher-timeframe series high column length 1 does not match timestamp length 2",
		},
		{
			name: "market nonfinite value precedes higher-timeframe nonfinite value",
			request: func() RunRequest {
				request := validationRequest(validationSeries(), validationSeries())
				request.Series.C[1] = math.NaN()
				request.HTFSeries.T[0] = math.Inf(1)
				return request
			}(),
			wantErr: "market series close column contains non-finite value at index 1",
		},
		{
			name: "column order precedes cross-column index order",
			request: func() RunRequest {
				request := validationRequest(validationSeries(), validationSeries())
				request.Series.T[1] = math.Inf(-1)
				request.Series.O[0] = math.NaN()
				return request
			}(),
			wantErr: "market series timestamp column contains non-finite value at index 1",
		},
		{
			name: "ascending index order within a column",
			request: func() RunRequest {
				request := validationRequest(validationSeries(), validationSeries())
				request.Series.C[0] = math.Inf(1)
				request.Series.C[1] = math.NaN()
				return request
			}(),
			wantErr: "market series close column contains non-finite value at index 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateRunRequest(tt.request); err == nil || err.Error() != tt.wantErr {
				t.Fatalf("validateRunRequest error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunRequestValidationAcceptsReviewedFixture(t *testing.T) {
	fixture, cfg := loadEntryAttemptCase(t, "family-break-retest")
	request := RunRequest{
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
	for _, flow := range validationPublicFlows() {
		t.Run(flow.name, func(t *testing.T) {
			if err := runWithoutPanic(t, func() error { return flow.run(request) }); err != nil {
				t.Fatalf("%s rejected reviewed fixture: %v", flow.name, err)
			}
		})
	}
}

type validationPublicFlow struct {
	name string
	run  func(RunRequest) error
}

func validationPublicFlows() []validationPublicFlow {
	return []validationPublicFlow{
		{name: "SharedContextKey", run: func(request RunRequest) error { _, err := SharedContextKey(request); return err }},
		{name: "PrepareSharedRunContext", run: func(request RunRequest) error { _, err := PrepareSharedRunContext(request); return err }},
		{name: "PrepareRun", run: func(request RunRequest) error { _, err := PrepareRun(request); return err }},
		{name: "Run", run: func(request RunRequest) error { _, err := Run(request); return err }},
	}
}

func validationSeries() marketdata.Series {
	return marketdata.Series{
		T: []float64{0, 3_600_000},
		O: []float64{100, 101},
		H: []float64{102, 103},
		L: []float64{99, 100},
		C: []float64{101, 102},
		V: []float64{10, 11},
	}
}

func validationRequest(series, htfSeries marketdata.Series) RunRequest {
	return RunRequest{
		Config:    dsl.Config{"setupType": "rangeBreakFake"},
		Series:    series,
		HTFSeries: htfSeries,
	}
}

func runWithoutPanic(t *testing.T, run func() error) (err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("public run flow panicked: %v", recovered)
		}
	}()
	return run()
}
