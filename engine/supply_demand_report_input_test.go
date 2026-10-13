package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
)

func sdInputSource(t *testing.T) string {
	t.Helper()
	b, e := os.ReadFile("../conformance/run/family-supply-demand-retest-entry-timeframe.strat")
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func sdInputRequest() map[string]any {
	return map[string]any{
		"schema": SupplyDemandRetestRequestSchema, "case": "synthetic", "strategyId": "test-retest", "symbol": "XAUUSD", "timeframe": "1h", "sourceTimeframe": "4h", "higherTimeframe": "", "rangeMethod": "zone",
		"costs": map[string]any{"feePerUnit": 0, "slippage": 0, "slippageBps": 0, "startEquity": 10000, "fillOn": "close"},
		"bars":  [][]float64{{0, 100, 101, 99, 100, 1}, {3600000, 100, 101, 99, 100, 1}}, "sourceBars": [][]float64{{0, 100, 101, 99, 100, 1}, {14400000, 100, 101, 99, 100, 1}}, "sourceHtfBars": [][]float64{},
	}
}
func sdInputJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func sdInputConfig(t *testing.T) dsl.Config {
	t.Helper()
	p, e := dsl.Parse(sdInputSource(t))
	if e != nil || len(p.Errors) > 0 {
		t.Fatalf("parse %v %v", e, p.Errors)
	}
	return p.Config
}
func sdInputRequireError(t *testing.T, err error, code string) {
	t.Helper()
	var typed *SupplyDemandRetestError
	if !errors.As(err, &typed) || typed.Code != code {
		t.Fatalf("wanted %s, got %v", code, err)
	}
}

func TestSupplyDemandReportInputIdentityAndRoutes(t *testing.T) {
	for _, route := range []struct {
		tf   string
		step float64
	}{{"15m", 900000}, {"30m", 1800000}, {"1h", 3600000}} {
		t.Run(route.tf, func(t *testing.T) {
			req := sdInputRequest()
			req["timeframe"] = route.tf
			req["bars"].([][]float64)[1][0] = route.step
			source := strings.ReplaceAll(sdInputSource(t), "1h", route.tf)
			a, e := prepareSupplyDemandRetest(sdInputJSON(t, req), source)
			if e != nil {
				t.Fatal(e)
			}
			id := a.identity
			if id.Timeframe != route.tf || id.Chart.DurationMS != int64(route.step) || id.Daily.FirstT != nil || id.Daily.LastT != nil || id.Daily.Count != 0 || id.Daily.DurationMS != 0 || id.HTFPolicy != "off" || len(id.RequestSHA256) != 64 || len(id.SourceSHA256) != 64 {
				t.Fatalf("identity %+v", id)
			}
			changed, e := prepareSupplyDemandRetest(append(sdInputJSON(t, req), ' '), source)
			if e != nil {
				t.Fatal(e)
			}
			if changed.identity.RequestSHA256 == id.RequestSHA256 {
				t.Fatal("exact request bytes not bound")
			}
			changed, e = prepareSupplyDemandRetest(sdInputJSON(t, req), source+"\n")
			if e != nil {
				t.Fatal(e)
			}
			if changed.identity.SourceSHA256 == id.SourceSHA256 {
				t.Fatal("exact source bytes not bound")
			}
		})
	}
}
func TestSupplyDemandReportInputClosedAndTyped(t *testing.T) {
	tests := []struct {
		name, code string
		edit       func(map[string]any)
	}{
		{"unknown", "invalid-input", func(r map[string]any) { r["contractVersion"] = 1 }}, {"missing", "invalid-input", func(r map[string]any) { delete(r, "sourceHtfBars") }},
		{"null-string", "invalid-input", func(r map[string]any) { r["higherTimeframe"] = nil }}, {"empty-symbol", "invalid-input", func(r map[string]any) { r["symbol"] = "" }}, {"long-label", "invalid-input", func(r map[string]any) { r["case"] = strings.Repeat("x", 257) }},
		{"control", "invalid-input", func(r map[string]any) { r["strategyId"] = "a\nb" }}, {"off-route", "unsupported-route", func(r map[string]any) { r["symbol"] = "EURUSD" }},
		{"wrong-step", "unsupported-route", func(r map[string]any) { r["timeframe"] = "5m" }}, {"wrong-source", "unsupported-route", func(r map[string]any) { r["sourceTimeframe"] = "1h" }}, {"wrong-range", "unsupported-route", func(r map[string]any) { r["rangeMethod"] = "auto" }},
		{"missing-cost", "invalid-input", func(r map[string]any) { delete(r["costs"].(map[string]any), "slippageBps") }}, {"extra-cost", "invalid-input", func(r map[string]any) { r["costs"].(map[string]any)["spread"] = 0 }},
		{"negative-cost", "invalid-input", func(r map[string]any) { r["costs"].(map[string]any)["feePerUnit"] = -1 }}, {"zero-equity", "invalid-input", func(r map[string]any) { r["costs"].(map[string]any)["startEquity"] = 0 }}, {"cost-string", "invalid-input", func(r map[string]any) { r["costs"].(map[string]any)["slippage"] = "0" }},
		{"fill", "invalid-input", func(r map[string]any) { r["costs"].(map[string]any)["fillOn"] = "intrabar" }}, {"null-bars", "invalid-input", func(r map[string]any) { r["bars"] = nil }}, {"short-series", "invalid-input", func(r map[string]any) { r["sourceBars"] = r["sourceBars"].([][]float64)[:1] }},
		{"short-row", "invalid-input", func(r map[string]any) { r["bars"].([][]float64)[0] = []float64{0, 100, 101, 99, 100} }}, {"long-row", "invalid-input", func(r map[string]any) { r["bars"].([][]float64)[0] = []float64{0, 100, 101, 99, 100, 1, 0} }},
		{"duplicate-time", "invalid-input", func(r map[string]any) { r["bars"].([][]float64)[1][0] = 0 }}, {"phase", "invalid-input", func(r map[string]any) { r["bars"].([][]float64)[0][0] = 1 }}, {"no-declared-interval", "invalid-input", func(r map[string]any) { r["bars"].([][]float64)[1][0] = 7200000 }},
		{"nonmultiple-gap", "invalid-input", func(r map[string]any) { r["sourceBars"].([][]float64)[1][0] = 15000000 }}, {"bad-ohlc", "invalid-input", func(r map[string]any) { r["bars"].([][]float64)[0][2] = 99 }}, {"negative-volume", "invalid-input", func(r map[string]any) { r["bars"].([][]float64)[0][5] = -1 }},
		{"extreme-finite-price", "invalid-input", func(r map[string]any) { r["bars"].([][]float64)[0][2] = 1e308 }}, {"volume-bound", "invalid-input", func(r map[string]any) { r["bars"].([][]float64)[0][5] = 1e12 + 1 }},
		{"off-with-daily", "unsupported-profile", func(r map[string]any) {
			r["sourceHtfBars"] = [][]float64{{0, 100, 101, 99, 100, 1}, {86400000, 100, 101, 99, 100, 1}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := sdInputRequest()
			tt.edit(r)
			_, e := prepareSupplyDemandRetest(sdInputJSON(t, r), sdInputSource(t))
			sdInputRequireError(t, e, tt.code)
		})
	}
	raw := sdInputJSON(t, sdInputRequest())
	for _, r := range [][]byte{append(append([]byte{}, raw...), []byte(" {}")...), []byte(strings.Replace(string(raw), `"case":"synthetic"`, `"case":"x","case":"synthetic"`, 1)), []byte(strings.Replace(string(raw), `"fillOn":"close"`, `"fillOn":"close","fillOn":"close"`, 1)), []byte(strings.Replace(string(raw), `"case":"synthetic"`, `"case":"\ud800"`, 1)), []byte(strings.Replace(string(raw), `"case":"synthetic"`, `"case":"\udc00"`, 1)), []byte(strings.Replace(string(raw), `"case":"synthetic"`, "\"case\":\"\xff\"", 1))} {
		_, e := prepareSupplyDemandRetest(r, sdInputSource(t))
		sdInputRequireError(t, e, "invalid-input")
	}
	for _, fill := range []string{"close", "open", "nextOpen"} {
		r := sdInputRequest()
		r["costs"].(map[string]any)["fillOn"] = fill
		if _, e := prepareSupplyDemandRetest(sdInputJSON(t, r), sdInputSource(t)); e != nil {
			t.Fatal(e)
		}
	}
}
func TestSupplyDemandReportRawDecimalAndUnicode(t *testing.T) {
	for _, ts := range []string{"3600000.00000000000000000001", "3.600000000000000000001e6", "9007199254740992", "-1", "1e9999999999999999999999999"} {
		raw := []byte(fmt.Sprintf("[[0,100,101,99,100,1],[%s,100,101,99,100,1]]", ts))
		if _, e := supplyDemandReadBars(raw, "bars", 3600000, 2, false); e == nil {
			t.Fatalf("accepted raw timestamp %s", ts)
		}
	}
	for _, ts := range []string{"3600000", "3.6e6", "3600000.000", "360000000e-2"} {
		raw := []byte(fmt.Sprintf("[[0,100,101,99,100,1],[%s,100,101,99,100,1]]", ts))
		if _, e := supplyDemandReadBars(raw, "bars", 3600000, 2, false); e != nil {
			t.Fatal(ts, e)
		}
	}
	step := float64(3600000)
	last := math.Floor((float64(1<<53-1)-step)/step) * step
	for _, delta := range []float64{0, step} {
		raw := []byte(fmt.Sprintf("[[%.0f,100,101,99,100,1],[%.0f,100,101,99,100,1]]", last-step+delta, last+delta))
		_, e := supplyDemandReadBars(raw, "bars", 3600000, 2, false)
		if (e == nil) != (delta == 0) {
			t.Fatalf("timestamp close ceiling delta=%g error=%v", delta, e)
		}
	}
	for _, raw := range []string{`{"a":"\ud800\udc00"}`, `{"a":"\\ud800"}`, `{"a":"😀"}`} {
		if !supplyDemandJSONUnicode([]byte(raw)) {
			t.Fatalf("valid Unicode %s", raw)
		}
	}
	for _, raw := range []string{`{"a":"\ud800"}`, `{"a":"\ud800x"}`, `{"a":"\udc00"}`, `{"a":"\ud800\u0041"}`, `{"\ud800":"a"}`} {
		if supplyDemandJSONUnicode([]byte(raw)) {
			t.Fatalf("invalid Unicode %s", raw)
		}
	}
}
func TestSupplyDemandReportDailyAdmission(t *testing.T) {
	source := strings.Replace(sdInputSource(t), "higher timeframe off", "higher timeframe daily must agree", 1)
	r := sdInputRequest()
	r["higherTimeframe"] = "1d"
	r["sourceHtfBars"] = [][]float64{{0, 100, 101, 99, 101, 1}, {86400000, 100, 101, 99, 100, 1}}
	a, e := prepareSupplyDemandRetest(sdInputJSON(t, r), source)
	if e != nil {
		t.Fatal(e)
	}
	if a.identity.HTFMode != "notAgainst" || a.identity.HTFPolicy != SupplyDemandRetestHTFPolicy || a.identity.Daily.DurationMS != 86400000 || a.identity.Daily.Count != 2 {
		t.Fatalf("daily identity %+v", a.identity)
	}
	for _, bars := range []any{[][]float64{}, [][]float64{{0, 100, 101, 99, 101, 1}}, [][]float64{{0, 100, 101, 99, 101, 1}, {172800000, 100, 101, 99, 100, 1}}} {
		r["sourceHtfBars"] = bars
		_, e = prepareSupplyDemandRetest(sdInputJSON(t, r), source)
		sdInputRequireError(t, e, "invalid-input")
	}
	r["sourceHtfBars"] = [][]float64{{0, 100, 101, 99, 101, 1}, {86400000, 100, 101, 99, 100, 1}, {345600000, 100, 101, 99, 100, 1}}
	if _, e = prepareSupplyDemandRetest(sdInputJSON(t, r), source); e != nil {
		t.Fatal("supplied gaps must remain admissible", e)
	}
	r["higherTimeframe"] = "1w"
	_, e = prepareSupplyDemandRetest(sdInputJSON(t, r), source)
	sdInputRequireError(t, e, "unsupported-profile")
}
func TestSupplyDemandReportConfigBoundsBeforeConversion(t *testing.T) {
	tests := []struct {
		name string
		edit func(dsl.Config)
	}{
		{"negative-count", func(c dsl.Config) { mapValue(c, "supplyDemand")["minBaseCandles"] = -1 }}, {"zero-base", func(c dsl.Config) { mapValue(c, "supplyDemand")["minBaseCandles"] = 0 }}, {"fraction", func(c dsl.Config) { mapValue(c, "supplyDemand")["impulseCandles"] = 1.5 }}, {"loop-over", func(c dsl.Config) { mapValue(c, "supplyDemand")["sourceCandles"] = 257 }},
		{"architecture-overflow", func(c dsl.Config) { mapValue(c, "supplyDemand")["maxZoneAgeCandles"] = float64(1 << 40) }}, {"base-order", func(c dsl.Config) { mapValue(c, "supplyDemand")["minBaseCandles"] = 5 }}, {"choch-zero", func(c dsl.Config) {
			mapValue(c, "supplyDemand")["chochCandles"] = 1
			mapValue(c, "supplyDemand")["chochLookbackCandles"] = 0
		}},
		{"nan", func(c dsl.Config) { c["riskUsd"] = math.NaN() }}, {"inf", func(c dsl.Config) { mapValue(c, "stop")["maxAtr"] = math.Inf(1) }}, {"negative-risk", func(c dsl.Config) { c["riskUsd"] = -1 }}, {"coefficient", func(c dsl.Config) { mapValue(c, "supplyDemand")["impulseAtr"] = 1e308 }}, {"risk-bound", func(c dsl.Config) { c["riskUsd"] = 1e12 + 1 }},
		{"negative-distance", func(c dsl.Config) { mapValue(c, "supplyDemand")["retestToleranceAtr"] = -0.1 }}, {"stop-order", func(c dsl.Config) { mapValue(c, "stop")["minAtr"] = 4 }}, {"partial", func(c dsl.Config) { mapValue(c, "partial")["enabled"] = 1 }}, {"channel", func(c dsl.Config) { mapValue(c, "channel")["enabled"] = true }},
		{"channel-target", func(c dsl.Config) { mapValue(c, "target")["edge"] = "channel" }}, {"seasonality", func(c dsl.Config) { c["seasonalityFilters"] = []any{map[string]any{}} }}, {"rmv", func(c dsl.Config) { c["relativeMeasuredVolatility"] = nil }}, {"hour-fraction", func(c dsl.Config) { c["localHours"] = []any{1.5} }},
		{"array", func(c dsl.Config) { c["dayTypes"] = make([]any, 65) }}, {"aggregate-array", func(c dsl.Config) {
			c["dayTypes"] = make([]any, 64)
			c["dayThemes"] = make([]any, 64)
			c["triggerCandles"] = make([]any, 64)
		}}, {"string", func(c dsl.Config) { c["name"] = strings.Repeat("x", 1025) }}, {"map", func(c dsl.Config) {
			m := map[string]any{}
			for i := 0; i < 129; i++ {
				m[fmt.Sprint(i)] = nil
			}
			c["extra"] = m
		}},
		{"depth", func(c dsl.Config) {
			var v any = 0
			for i := 0; i < 17; i++ {
				v = map[string]any{"x": v}
			}
			c["extra"] = v
		}}, {"unknown-type", func(c dsl.Config) { c["extra"] = make(chan int) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := sdInputConfig(t)
			tt.edit(c)
			sdInputRequireError(t, validateSupplyDemandReportConfig(c), "unsupported-profile")
		})
	}
	c := sdInputConfig(t)
	mapValue(c, "supplyDemand")["sourceCandles"] = 0
	delete(mapValue(c, "supplyDemand"), "maxBaseBodyAtr")
	if e := validateSupplyDemandReportConfig(c); e != nil {
		t.Fatal("zero source disabled/absent unlimited must remain", e)
	}
	c = sdInputConfig(t)
	mapValue(c, "supplyDemand")["impulseCandles"] = 256
	mapValue(c, "supplyDemand")["minBaseCandles"] = 1
	mapValue(c, "supplyDemand")["maxBaseCandles"] = 256
	mapValue(c, "supplyDemand")["chochCandles"] = 1
	mapValue(c, "supplyDemand")["chochLookbackCandles"] = 256
	if e := validateSupplyDemandReportConfig(c); e != nil {
		t.Fatal(e)
	}
}
func TestSupplyDemandReportWorkBoundaries(t *testing.T) {
	c := sdInputConfig(t)
	for _, axis := range []string{"chart", "source", "daily"} {
		t.Run(axis, func(t *testing.T) {
			last := 0
			for n := 2; n <= SupplyDemandRetestMaxRows; n++ {
				cc, ss, dd := 2, 2, 0
				switch axis {
				case "chart":
					cc = n
				case "source":
					ss = n
				case "daily":
					dd = n
				}
				_, e := supplyDemandWorkScore(cc, ss, dd, c)
				if e != nil {
					break
				}
				last = n
			}
			if last == 0 {
				t.Fatal("no admitted boundary")
			}
			cc, ss, dd := 2, 2, 0
			switch axis {
			case "chart":
				cc = last + 1
			case "source":
				ss = last + 1
			case "daily":
				dd = last + 1
			}
			_, e := supplyDemandWorkScore(cc, ss, dd, c)
			if last < SupplyDemandRetestMaxRows {
				sdInputRequireError(t, e, "work-limit")
			}
		})
	}
	if _, e := supplyDemandWorkScore(math.MaxInt, math.MaxInt, math.MaxInt, c); e == nil {
		t.Fatal("overflow accepted")
	}
	if _, e := supplyDemandReadBars([]byte(`[[0,1,1,1,1,0],[3600000,1,1,1,1,0]]`), "bars", 3600000, 1, false); e == nil {
		t.Fatal("row budget accepted")
	}
}

func TestSupplyDemandReportSourceNonfiniteSpellings(t *testing.T) {
	for _, token := range []string{"NaN", "Inf", "+Inf", "-Infinity", "1e309", "1e+309", "2.5+Inf", "2.5+1e309", "2.5+1e+309", "2.5+0x1p+1024", "2.5+1e+9999999999", "2.5+-1e+309", "2.5++1e+309", "2.5+-0x1p+1024"} {
		source := strings.Replace(sdInputSource(t), "impulse at least 2.5 ATR", "impulse at least "+token+" ATR", 1)
		_, e := prepareSupplyDemandRetest(sdInputJSON(t, sdInputRequest()), source)
		if e == nil {
			t.Fatalf("nonfinite token %q unexpectedly accepted", token)
		}
		sdInputRequireError(t, e, "unsupported-profile")
	}
	for _, source := range []string{`strategy "Inf" { description "1e309" }`, "description Inf", "description 1e+309", "symbols(INF)", "# Inf 1e309\nrisk 100 USD", "impulse at least 2.5 ATR"} {
		if !supplyDemandFiniteSourceTokens(source) {
			t.Fatalf("valid source tokens refused: %s", source)
		}
	}
}

func TestSupplyDemandReportSharedThresholdAdmission(t *testing.T) {
	for _, directive := range []string{"tail rejection at least -1", "close location at least -1", "prior day range at least -1 ATR"} {
		source := strings.Replace(sdInputSource(t), "filters {", "filters {\n  "+directive, 1)
		parsed, e := dsl.Parse(source)
		if e != nil || len(parsed.Errors) > 0 {
			t.Fatalf("test source must parse: %v %v", e, parsed.Errors)
		}
		_, e = prepareSupplyDemandRetest(sdInputJSON(t, sdInputRequest()), source)
		sdInputRequireError(t, e, "unsupported-profile")
	}
	for _, description := range []string{"Inf", "1e+309"} {
		source := strings.Replace(sdInputSource(t), `description "Synthetic zone retested on the entry candles."`, "description "+description, 1)
		if _, e := prepareSupplyDemandRetest(sdInputJSON(t, sdInputRequest()), source); e != nil {
			t.Fatalf("non-numeric description %s: %v", description, e)
		}
	}
}

func TestSupplyDemandReportJoinedExponentAdmission(t *testing.T) {
	for _, token := range []string{"2.5+1e+3", "2.5+-1e+3", "2.5+0x1p+3", "2.5+there+1e+3", "2.5+1e-9999"} {
		if !supplyDemandFiniteJoinedToken(token) {
			t.Fatalf("finite spelling: %s", token)
		}
	}
	for _, token := range []string{"2.5+1e+309", "2.5+-1e+309", "2.5++1e+309", "2.5+there+1e+309", "2.5+0x1p+1024", "2.5+-0x1p+1024", "2.5+1e+9999999999"} {
		if supplyDemandFiniteJoinedToken(token) {
			t.Fatalf("overflow spelling accepted: %s", token)
		}
	}
}
