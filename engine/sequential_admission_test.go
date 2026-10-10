package engine

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func seqFullAdmissionSource() string {
	return `dsl v7
strategy "Sequential synthetic E1" {}
market { sequential symbol SYNTH sequential timeframe 5m }
setup { type: sequential full sequential profile seq.full.public_approx.v1 sequential policy E1 }
risk { sequential riskUsd 100 sequential maxNotionalUsd 100000 }`
}

func seqFullAdmissionRequest(t *testing.T) RunRequest {
	t.Helper()
	parsed, err := dsl.Parse(seqFullAdmissionSource())
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("parse: %v %v", err, parsed.Errors)
	}
	bars := make([]marketdata.Bar, 19)
	for i := range bars {
		close := 100.0
		if i == 4 {
			close = 101
		} else if i >= 5 && i <= 13 {
			close = float64(104 - i)
		}
		bars[i] = marketdata.Bar{T: float64(i * 300000), O: close, H: close + 1, L: close - 1, C: close, V: 1}
		if i >= 14 {
			bars[i].H, bars[i].L = 100.5, 99.5
		}
	}
	return RunRequest{Config: parsed.Config, Series: marketdata.SeriesFromBars(bars), Symbol: "SYNTH", Timeframe: "5m", StrategyID: "synthetic-e1", Costs: Costs{FillOn: "open"}}
}

func TestSequentialFullPublicRunAndPreparedIsolation(t *testing.T) {
	request := seqFullAdmissionRequest(t)
	want, err := Run(request)
	if err != nil || len(want.Trades) != 1 || want.SequentialFull == nil {
		t.Fatalf("meaningful run: %+v %v", want, err)
	}
	if want.Trades[0].EntryIndex != 14 || want.Trades[0].ExitIndex != 18 || want.Trades[0].Reason != "time" {
		t.Fatalf("entry/exit: %+v", want.Trades[0])
	}
	prepared, err := PrepareRun(request)
	if err != nil {
		t.Fatal(err)
	}
	// Bind values, not caller maps or column backing arrays.
	request.Config["sequentialFull"].(map[string]any)["policy"] = "P1"
	request.Series.C[0] = math.NaN()
	first, err := prepared.RunChecked(request.Costs)
	if err != nil || !reflect.DeepEqual(want, first) {
		t.Fatalf("prepared binding: %v", err)
	}
	first.Trades[0].Meta["policy"] = "caller mutation"
	first.SequentialFull.Opportunities[0].ID = "caller mutation"
	*first.SequentialFull.Opportunities[0].Fill = -123
	other := request.Costs
	other.Slippage = .25
	if _, err := prepared.RunChecked(other); err != nil {
		t.Fatal(err)
	}
	last, err := prepared.RunChecked(request.Costs)
	if err != nil || !reflect.DeepEqual(want, last) {
		t.Fatalf("rerun isolation: %v", err)
	}
	badCosts := request.Costs
	badCosts.FillOn = "close"
	if _, err := prepared.RunChecked(badCosts); err == nil {
		t.Fatal("prepared accepted close fill")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("unchecked run hid invalid costs")
			}
		}()
		prepared.Run(badCosts)
	}()
}

func TestSequentialFullAdmissionRefusalsBeforeEmptySuccess(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RunRequest)
	}{
		{"wrong symbol", func(r *RunRequest) { r.Symbol = "OTHER" }},
		{"wrong timeframe", func(r *RunRequest) { r.Timeframe = "1m" }},
		{"source timeframe", func(r *RunRequest) { r.SourceTimeframe = "5m" }},
		{"higher timeframe", func(r *RunRequest) { r.HigherTimeframe = "1h" }},
		{"source columns without times", func(r *RunRequest) { r.SourceSeries.O = []float64{1} }},
		{"HTF columns", func(r *RunRequest) { r.HTFSeries.O = []float64{1} }},
		{"source HTF columns", func(r *RunRequest) { r.SourceHTFSeries.O = []float64{1} }},
		{"force route", func(r *RunRequest) { r.ForceRoute = true }},
		{"execution window", func(r *RunRequest) { r.ExecutionWindow = &ExecutionWindow{} }},
		{"calendar", func(r *RunRequest) { r.TimedCalendar = &TimedReturnCalendar{} }},
		{"blank fill", func(r *RunRequest) { r.Costs.FillOn = "" }},
		{"close fill", func(r *RunRequest) { r.Costs.FillOn = "close" }},
		{"negative fee", func(r *RunRequest) { r.Costs.FeePerUnit = -1 }},
		{"NaN slippage", func(r *RunRequest) { r.Costs.Slippage = math.NaN() }},
		{"infinite bps", func(r *RunRequest) { r.Costs.SlippageBps = math.Inf(1) }},
		{"negative equity", func(r *RunRequest) { r.Costs.StartEquity = -1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := seqFullAdmissionRequest(t)
			r.Series = marketdata.Series{}
			test.mutate(&r)
			result, err := Run(r)
			var typed *SequentialFullExecutionError
			if !errors.As(err, &typed) || result.SequentialFull != nil || result.Trades != nil {
				t.Fatalf("not typed fail closed: %+v %T %v", result, err, err)
			}
		})
	}
	r := seqFullAdmissionRequest(t)
	r.Config["setupType"] = "legacySetup9"
	r.Series = marketdata.Series{}
	_, err := Run(r)
	var cfgErr *dsl.SequentialFullConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("relabeled full config: %T %v", err, err)
	}
}

func TestSequentialFullInvalidSeriesAndGapRefusals(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*marketdata.Series)
	}{
		{"shape", func(s *marketdata.Series) { s.O = s.O[:2] }},
		{"NaN time", func(s *marketdata.Series) { s.T[0] = math.NaN() }},
		{"fractional time", func(s *marketdata.Series) { s.T[0] = .5 }},
		{"unsafe time", func(s *marketdata.Series) { s.T[0] = float64(1 << 53) }},
		{"negative time", func(s *marketdata.Series) { s.T[0] = -1 }},
		{"duplicate", func(s *marketdata.Series) { s.T[1] = s.T[0] }},
		{"unordered", func(s *marketdata.Series) { s.T[2] = s.T[0] }},
		{"gap", func(s *marketdata.Series) {
			for i := 10; i < len(s.T); i++ {
				s.T[i] += 300000
			}
		}},
		{"short interval", func(s *marketdata.Series) { s.T[1] -= 1 }},
		{"nonfinite price", func(s *marketdata.Series) { s.O[0] = math.Inf(1) }},
		{"bad envelope", func(s *marketdata.Series) { s.L[0] = s.H[0] + 1 }},
		{"bad close", func(s *marketdata.Series) { s.C[0] = s.H[0] + 1 }},
		{"bad volume", func(s *marketdata.Series) { s.V[0] = math.NaN() }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := seqFullAdmissionRequest(t)
			test.mutate(&r.Series)
			_, err := PrepareRun(r)
			var typed *SequentialFullExecutionError
			if !errors.As(err, &typed) {
				t.Fatalf("%T %v", err, err)
			}
		})
	}
}

func TestSequentialFullExcludedExecutionSurfaces(t *testing.T) {
	r := seqFullAdmissionRequest(t)
	if _, err := SharedContextKey(r); err == nil {
		t.Error("shared key accepted")
	}
	if _, err := PrepareSharedRunContext(r); err == nil {
		t.Error("shared context accepted")
	}
	if _, err := (&SharedRunContext{}).PrepareVariant(r.Config); err == nil {
		t.Error("shared variant accepted")
	}
	if _, err := RunPrefix(r); err == nil {
		t.Error("prefix accepted")
	}
	if _, _, err := RunPrefixResumable(r, nil); err == nil {
		t.Error("checkpoint accepted")
	}
	defer func() {
		if recover() == nil {
			t.Error("internal generic PreparedRunner accepted")
		}
	}()
	newPreparedRunner(RunFixture{}, r.Config)
}

func TestSequentialFullFixtureAndTerminalRefusal(t *testing.T) {
	r := seqFullAdmissionRequest(t)
	bars := make([]marketdata.Bar, r.Series.Len())
	for i := range bars {
		bars[i] = marketdata.Bar{T: r.Series.T[i], O: r.Series.O[i], H: r.Series.H[i], L: r.Series.L[i], C: r.Series.C[i], V: r.Series.V[i]}
	}
	f := RunFixture{Case: "synthetic-e1", StrategyID: r.StrategyID, Symbol: r.Symbol, Timeframe: r.Timeframe, Costs: r.Costs, Bars: bars}
	result, err := RunFixtureCase(f, seqFullAdmissionSource())
	if err != nil || result.TradeCount != 1 || result.SequentialFull == nil {
		t.Fatalf("fixture: %+v %v", result, err)
	}
	raw, err := json.Marshal(result)
	if err != nil || !strings.Contains(string(raw), `"sequentialFull"`) {
		t.Fatalf("audit not serialized: %s %v", raw, err)
	}
	f.Bars = f.Bars[:15]
	result, err = RunFixtureCase(f, seqFullAdmissionSource())
	var typed *SequentialFullExecutionError
	if !errors.As(err, &typed) || typed.Kind != "unsupported-incomplete-terminal-run" || result.Trades != nil {
		t.Fatalf("terminal: %+v %T %v", result, err, err)
	}
	f.Bars = f.Bars[:14]
	result, err = RunFixtureCase(f, seqFullAdmissionSource())
	if err != nil || result.TradeCount != 0 || len(result.SequentialFull.Opportunities) != 1 || result.SequentialFull.Opportunities[0].Reason != SequentialFullNoNextBar {
		t.Fatalf("pending terminal: %+v %v", result, err)
	}
	f.RawBars = [][]float64{{0, 1, 2}}
	if _, err = RunFixtureCase(f, seqFullAdmissionSource()); err == nil {
		t.Fatal("malformed raw rows accepted")
	}
}

func TestSequentialFullExactRawTimestampAdmission(t *testing.T) {
	for _, raw := range []string{"0", "-0", "-0.0", "0.0", "0e999999999999999999999999", "1000000000000", "1000000000000.0", "1e12", "100000000000000e-2", "9007199254740991"} {
		if err := ValidateSequentialFullJSONEnvelope([]byte(`{"bars":[[`+raw+`,1,2,0,1,1]]}`), false); err != nil {
			t.Errorf("valid timestamp %s: %v", raw, err)
		}
	}
	for _, raw := range []string{"1000000000000.00001", "0.0000000000000000001", "100000000000000001e-5", "1e-999999999999999999999999", "9007199254740992", "-1", "null", `"0"`, `{}`, `[]`} {
		if err := ValidateSequentialFullJSONEnvelope([]byte(`{"bars":[[`+raw+`,1,2,0,1,1]]}`), false); err == nil {
			t.Errorf("lossy/invalid timestamp accepted %s", raw)
		}
	}
}
