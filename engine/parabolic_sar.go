package engine

import (
	"fmt"
	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// parabolicSARRuntime is immutable after preparation. It is kept outside the
// historical Columns/flagParams layouts so old broker arithmetic operands do
// not move. Each variant owns its predicate while sharing numeric slices.
type parabolicSARRuntime struct {
	spec    dsl.ParabolicSARSpec
	columns contextcols.ParabolicSARColumns
}

func prepareParabolicSAR(cfg dsl.Config, series marketdata.Series) *parabolicSARRuntime {
	spec, present, err := dsl.DecodeParabolicSAR(cfg)
	if !present || err != nil {
		return nil
	} // Public callers validate first.
	return &parabolicSARRuntime{spec: spec, columns: contextcols.ComputeParabolicSAR(series, *parabolicSAROptions(cfg))}
}

func (r *PreparedRun) resetBroker(trades []Trade) {
	r.broker.reset(r.series, r.cols, r.htfTrend, r.ema, r.emaSlope, r.params, r.fixture, trades)
	r.broker.parabolicSAR = r.parabolicSAR
	r.broker.clock.installClockRangeGates(r.clockGates)
}

func validateParabolicSARConfig(cfg dsl.Config) error {
	_, _, err := dsl.DecodeParabolicSAR(cfg)
	return err
}

func parabolicSAROptions(cfg dsl.Config) *contextcols.ParabolicSAROptions {
	spec, present, err := dsl.DecodeParabolicSAR(cfg)
	if !present || err != nil {
		return nil
	}
	return &contextcols.ParabolicSAROptions{Start: spec.Start, Step: spec.Step, Max: spec.Max}
}

// parabolicSARGateOK is evaluated on the closed signal bar, before capturing
// or queuing an order. Later fills must not re-evaluate it using future OHLC.
func (b *broker) parabolicSARGateOK(i int, s side) bool {
	if b.parabolicSAR == nil {
		return true
	}
	predicate := b.parabolicSAR.spec.Predicate
	if predicate == "" {
		return true
	}
	cols := b.parabolicSAR.columns
	if i < 0 || i >= len(cols.SAR) || i >= len(cols.Trend) || i >= len(cols.Flip) ||
		i >= len(b.series.C) || !isFinite(cols.SAR[i]) || !isFinite(b.series.C[i]) || cols.Trend[i] == 0 {
		return false
	}
	switch predicate {
	case "bullish":
		return cols.Trend[i] == 1
	case "bearish":
		return cols.Trend[i] == -1
	case "closeAbove":
		return b.series.C[i] > cols.SAR[i]
	case "closeBelow":
		return b.series.C[i] < cols.SAR[i]
	case "flipBullish":
		return cols.Flip[i] == 1
	case "flipBearish":
		return cols.Flip[i] == -1
	case "supportsEntry":
		return (s == sideLong || s == sideShort) && cols.Trend[i] == int8(s)
	default:
		return false
	}
}

// The ordinary engine historically accepts some malformed OHLC geometries.
// SAR opts into a strict input contract without changing existing strategies.
func validateParabolicSARSeries(cfg dsl.Config, series marketdata.Series) error {
	if _, present := cfg["parabolicSAR"]; !present {
		return nil
	}
	if err := validateSeriesShape("SAR", series); err != nil {
		return err
	}
	if err := validateSeriesValues("SAR", series); err != nil {
		return err
	}
	for i := range series.T {
		if series.L[i] > series.H[i] || series.O[i] < series.L[i] || series.O[i] > series.H[i] || series.C[i] < series.L[i] || series.C[i] > series.H[i] {
			return fmt.Errorf("SAR requires valid OHLC geometry at index %d", i)
		}
		if i > 0 && series.T[i] <= series.T[i-1] {
			return fmt.Errorf("SAR timestamps must strictly increase at index %d", i)
		}
	}
	return nil
}

func validateParabolicSARFixture(cfg dsl.Config, fixture RunFixture) error {
	if _, present := cfg["parabolicSAR"]; !present {
		return nil
	}
	if fixture.rawRowDefect != "" {
		return fmt.Errorf("SAR malformed series: %s", fixture.rawRowDefect)
	}
	if fixture.RawBars == nil {
		return nil
	}
	if len(fixture.RawBars) != len(fixture.Bars) {
		return fmt.Errorf("SAR malformed series: raw rows were dropped")
	}
	for i, row := range fixture.RawBars {
		if len(row) != 6 {
			return fmt.Errorf("SAR malformed series: row %d must have six values", i)
		}
	}
	return nil
}

// SourceTimeframe can be supplied independently of the authored config by
// native callers. Do not accept a contradictory route label for chart SAR.
func validateParabolicSARRoute(cfg dsl.Config, chart, source string) error {
	if _, present := cfg["parabolicSAR"]; present && source != "" && source != chart {
		return fmt.Errorf("SAR source timeframe must be empty or equal the chart timeframe")
	}
	return nil
}
