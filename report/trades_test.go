package report

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestReportStrategyTypeTagsMirrorDSLConfigCounts(t *testing.T) {
	tags := strategyTypeTags(dsl.Config{
		"sessionBiasFilters": []any{map[string]any{}, map[string]any{}},
		"seasonalityFilters": []any{map[string]any{}},
		"vp":                 []any{map[string]any{}, map[string]any{}, map[string]any{}},
	})
	if tags.SessionBiasFilterCount != 2 || tags.SeasonalityFilterCount != 1 || tags.VPAFilterCount != 3 {
		t.Fatalf("strategy type tags = %#v", tags)
	}
}

func TestReportTradeJSONFieldFidelity(t *testing.T) {
	trades := []Trade{{Trade: engine.Trade{
		Entry:      1901.25,
		EntryIndex: 7,
		EntryT:     1704067200000,
		Exit:       1902.5,
		ExitIndex:  11,
		ExitT:      1704068400000,
		InitialSL:  1899.5,
		InitialTP:  1904,
		Meta:       engine.TradeMeta{"setup": "range-break-fake", "gradeScore": 4.5, "vwapDistanceAtr": -1.25, "channelDirection": "ascending", "channelWidthAtr": 2.5},
		Partial:    true,
		PnL:        125,
		Points:     1.25,
		Reason:     "tp",
		Side:       "long",
		Size:       100,
		SL:         1901.25,
		Tag:        "range-break-fake",
		TP:         1904,
	}, ReportSymbol: "XAUUSD", ReportTF: "1h", ReportRange: "zone", ReportSlice: "XAUUSD 1h", ReportSessionPhase: "open", ReportPreviousOutcome: "first trade", ReportOpenLocation: "nearPDH", ReportPriorDayType: "trend"}}
	payload := Document{
		TradeSchema: TradesSchema,
		Slices:      []Slice{{Trades: &trades}},
	}
	var out bytes.Buffer
	if err := WriteJSON(&out, payload); err != nil {
		t.Fatalf("write report JSON: %v", err)
	}
	var raw struct {
		TradeSchema string `json:"tradeSchema"`
		Slices      []struct {
			Trades []map[string]any `json:"trades"`
		} `json:"slices"`
	}
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatalf("decode trade JSON: %v", err)
	}
	want := map[string]any{
		"entry":      1901.25,
		"entryIndex": float64(7),
		"entryT":     float64(1704067200000),
		"exit":       1902.5,
		"exitIndex":  float64(11),
		"exitT":      float64(1704068400000),
		"initialSl":  1899.5,
		"initialTp":  float64(1904),
		"meta": map[string]any{
			"setup":            "range-break-fake",
			"gradeScore":       4.5,
			"vwapDistanceAtr":  -1.25,
			"channelDirection": "ascending",
			"channelWidthAtr":  2.5,
		},
		"partial":               true,
		"pnl":                   float64(125),
		"points":                1.25,
		"reason":                "tp",
		"reportSymbol":          "XAUUSD",
		"reportTf":              "1h",
		"reportRange":           "zone",
		"reportSlice":           "XAUUSD 1h",
		"reportPreviousOutcome": "first trade",
		"reportOpenLocation":    "nearPDH",
		"reportPriorDayType":    "trend",
		"reportLocalHour":       float64(0),
		"reportLocalWeekday":    "",
		"reportEntryMonth":      "",
		"reportSessionPhase":    "open",
		"side":                  "long",
		"size":                  float64(100),
		"sl":                    1901.25,
		"tag":                   "range-break-fake",
		"tp":                    float64(1904),
	}
	if raw.TradeSchema != TradesSchema || len(raw.Slices) != 1 || len(raw.Slices[0].Trades) != 1 {
		t.Fatalf("unexpected trade envelope: %+v", raw)
	}
	if got := raw.Slices[0].Trades[0]; !reflect.DeepEqual(got, want) {
		t.Fatalf("trade JSON = %#v, want %#v", got, want)
	}
}

func TestReportTradeNoTargetNullDoesNotChangeLegacyZero(t *testing.T) {
	for name, trade := range map[string]Trade{
		"no-target":   {Trade: engine.Trade{NoTarget: true}},
		"legacy-zero": {Trade: engine.Trade{}},
	} {
		raw, err := json.Marshal(trade)
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		if name == "no-target" {
			if decoded["tp"] != nil || decoded["initialTp"] != nil {
				t.Fatalf("%s = %#v", name, decoded)
			}
		} else if decoded["tp"] != float64(0) || decoded["initialTp"] != float64(0) {
			t.Fatalf("%s = %#v", name, decoded)
		}
	}
}

func TestReportTradePreviousOutcomeUsesChronologicalOrder(t *testing.T) {
	trades := mustAnnotateReportTrades(t, []engine.Trade{
		{EntryT: 300, PnL: -1},
		{EntryT: 100, PnL: 2},
		{EntryT: 200, PnL: 0},
	}, marketdata.NewSeries(0), nil, reportTradeRoute{})
	if got, want := trades[1].ReportPreviousOutcome, "first trade"; got != want {
		t.Fatalf("earliest trade outcome = %q, want %q", got, want)
	}
	if got, want := trades[2].ReportPreviousOutcome, "after win"; got != want {
		t.Fatalf("middle trade outcome = %q, want %q", got, want)
	}
	if got, want := trades[0].ReportPreviousOutcome, "after loss"; got != want {
		t.Fatalf("latest trade outcome = %q, want %q", got, want)
	}
}

func TestReportTradeExcursionCoversLongShortAndInvalidBounds(t *testing.T) {
	series := marketdata.SeriesFromBars([]marketdata.Bar{
		{H: 102, L: 99}, {H: 105, L: 98}, {H: 104, L: 97}, {H: 108, L: 96},
	})
	long := reportTradeExcursion(engine.Trade{Entry: 100, Exit: 100, InitialSL: 98, EntryIndex: 0, ExitIndex: 2, Side: "long"}, series)
	if long == nil || long.mfeR != 2.5 || long.maeR != 1.5 || long.timeToMfeBars != 1 {
		t.Fatalf("long excursion = %#v, want mfe=2.5 mae=1.5 at=1", long)
	}
	if long.postExitMfeR == nil || long.postExitMaeR == nil || *long.postExitMfeR != 4 || *long.postExitMaeR != 2 {
		t.Fatalf("long post-exit excursion = %#v, want mfe=4 mae=2", long)
	}
	short := reportTradeExcursion(engine.Trade{Entry: 100, InitialSL: 102, EntryIndex: 0, ExitIndex: 2, Side: "short"}, series)
	if short == nil || short.mfeR != 1.5 || short.maeR != 2.5 || short.timeToMfeBars != 2 {
		t.Fatalf("short excursion = %#v, want mfe=1.5 mae=2.5 at=2", short)
	}
	for _, trade := range []engine.Trade{
		{Entry: 100, InitialSL: 100, SL: 100, EntryIndex: 0, ExitIndex: 1, Side: "long"},
		{Entry: 100, InitialSL: 98, EntryIndex: -1, ExitIndex: 1, Side: "long"},
		{Entry: 100, InitialSL: 98, EntryIndex: 2, ExitIndex: 1, Side: "long"},
		{Entry: 100, InitialSL: 98, EntryIndex: 4, ExitIndex: 4, Side: "long"},
	} {
		if got := reportTradeExcursion(trade, series); got != nil {
			t.Fatalf("invalid trade excursion = %#v, want nil", got)
		}
	}
}

func TestReportTradeExcursionClampsPostExitWindowToFinalBar(t *testing.T) {
	tests := []struct {
		name  string
		trade engine.Trade
		bars  []marketdata.Bar
	}{
		{
			name:  "long",
			trade: engine.Trade{Entry: 100, Exit: 100, InitialSL: 98, EntryIndex: 0, ExitIndex: 1, Side: "long", Reason: "eod"},
			bars:  []marketdata.Bar{{H: 101, L: 99}, {H: 104, L: 99}},
		},
		{
			name:  "short",
			trade: engine.Trade{Entry: 100, Exit: 100, InitialSL: 102, EntryIndex: 0, ExitIndex: 1, Side: "short", Reason: "eod"},
			bars:  []marketdata.Bar{{H: 101, L: 99}, {H: 101, L: 96}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := reportTradeExcursion(tt.trade, marketdata.SeriesFromBars(tt.bars))
			if summary == nil || summary.postExitMfeR == nil || summary.postExitMaeR == nil {
				t.Fatalf("final-bar excursion = %#v, want finite post-exit values", summary)
			}
			if *summary.postExitMfeR != 2 || *summary.postExitMaeR != 0.5 {
				t.Fatalf("final-bar post-exit excursion = %v/%v, want 2/0.5", *summary.postExitMfeR, *summary.postExitMaeR)
			}
		})
	}
}

func TestReportTradeExcursionFieldsSerializeWhenAvailable(t *testing.T) {
	series := marketdata.SeriesFromBars([]marketdata.Bar{{H: 102, L: 99}, {H: 105, L: 98}})
	trades := mustAnnotateReportTrades(t, []engine.Trade{{Entry: 100, Exit: 100, InitialSL: 98, EntryIndex: 0, ExitIndex: 1, Side: "long", Reason: "eod"}}, series, nil, reportTradeRoute{})
	payload := Document{TradeSchema: TradesSchema, Slices: []Slice{{Trades: &trades}}}
	var out bytes.Buffer
	if err := WriteJSON(&out, payload); err != nil {
		t.Fatalf("write report JSON: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatalf("decode report JSON: %v", err)
	}
	trade := raw["slices"].([]any)[0].(map[string]any)["trades"].([]any)[0].(map[string]any)
	if trade["reportMfeR"] != 2.5 || trade["reportMaeR"] != 1.0 || trade["reportTimeToMfeBars"] != float64(1) {
		t.Fatalf("serialized excursion fields = %#v", trade)
	}
	if trade["reportPostExitMfeR"] != 2.5 || trade["reportPostExitMaeR"] != 1.0 {
		t.Fatalf("serialized final-bar post-exit fields = %#v, want 2.5/1", trade)
	}
}

func TestReportTradeExcursionRejectsNonFiniteRatiosInStableOrder(t *testing.T) {
	tinyRiskTrade := engine.Trade{
		Entry: 0, Exit: 0, InitialSL: -math.SmallestNonzeroFloat64,
		EntryIndex: 0, ExitIndex: 0, Side: "long",
	}
	tests := []struct {
		name   string
		trades []engine.Trade
		bars   []marketdata.Bar
		want   string
	}{
		{
			name:   "primary MFE precedes primary MAE",
			trades: []engine.Trade{tinyRiskTrade},
			bars:   []marketdata.Bar{{H: 1, L: -1}},
			want:   "report trade 0 reportMfeR contains non-finite value",
		},
		{
			name:   "primary MAE",
			trades: []engine.Trade{tinyRiskTrade},
			bars:   []marketdata.Bar{{H: 0, L: -1}},
			want:   "report trade 0 reportMaeR contains non-finite value",
		},
		{
			name:   "post-exit MFE precedes post-exit MAE",
			trades: []engine.Trade{tinyRiskTrade},
			bars:   []marketdata.Bar{{H: 0, L: 0}, {H: 1, L: -1}},
			want:   "report trade 0 reportPostExitMfeR contains non-finite value",
		},
		{
			name:   "post-exit MAE",
			trades: []engine.Trade{tinyRiskTrade},
			bars:   []marketdata.Bar{{H: 0, L: 0}, {H: 0, L: -1}},
			want:   "report trade 0 reportPostExitMaeR contains non-finite value",
		},
		{
			name: "input trade index",
			trades: []engine.Trade{
				{EntryIndex: 0, ExitIndex: 0, Side: "long"},
				tinyRiskTrade,
			},
			bars: []marketdata.Bar{{H: 1, L: 0}},
			want: "report trade 1 reportMfeR contains non-finite value",
		},
		{
			name:   "negative infinity",
			trades: []engine.Trade{tinyRiskTrade},
			bars:   []marketdata.Bar{{H: -1, L: 0}},
			want:   "report trade 0 reportMfeR contains non-finite value",
		},
		{
			name: "NaN from finite differences",
			trades: []engine.Trade{{
				Entry: -math.MaxFloat64, Exit: -math.MaxFloat64, InitialSL: math.MaxFloat64,
				EntryIndex: 0, ExitIndex: 0, Side: "long",
			}},
			bars: []marketdata.Bar{{H: math.MaxFloat64, L: -math.MaxFloat64}},
			want: "report trade 0 reportMfeR contains non-finite value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trades, err := annotateReportTrades(tt.trades, marketdata.SeriesFromBars(tt.bars), nil, reportTradeRoute{})
			if err == nil || err.Error() != tt.want {
				t.Fatalf("annotateReportTrades error = %v, want %q", err, tt.want)
			}
			if trades != nil {
				t.Fatalf("annotateReportTrades returned partial trades on error: %#v", trades)
			}
		})
	}
}

func TestReportTradeAnnotationKeepsZeroRiskExcursionOmitted(t *testing.T) {
	trades := mustAnnotateReportTrades(t, []engine.Trade{{
		Entry: 100, Exit: 100, InitialSL: 100, SL: 100,
		EntryIndex: 0, ExitIndex: 0, Side: "long",
	}}, marketdata.SeriesFromBars([]marketdata.Bar{{H: 101, L: 99}}), nil, reportTradeRoute{})
	if len(trades) != 1 {
		t.Fatalf("annotated trades = %d, want 1", len(trades))
	}
	trade := trades[0]
	if trade.ReportMfeR != nil || trade.ReportMaeR != nil || trade.ReportTimeToMfeBars != nil || trade.ReportPostExitMfeR != nil || trade.ReportPostExitMaeR != nil {
		t.Fatalf("zero-risk trade has excursion annotations: %#v", trade)
	}
}

func TestReportTradeLocalWeekdayUsesUTCPlusTenAndPinsNegativeFallback(t *testing.T) {
	trades := mustAnnotateReportTrades(t, []engine.Trade{
		{EntryT: 1704121200000}, // 2024-01-01 15:00 UTC -> Tuesday at UTC+10.
		{EntryT: -1},
	}, marketdata.NewSeries(0), nil, reportTradeRoute{})
	if got, want := trades[0].ReportLocalWeekday, "Tue"; got != want {
		t.Fatalf("UTC+10 weekday = %q, want %q", got, want)
	}
	if got, want := trades[0].ReportEntryMonth, "2024-01"; got != want {
		t.Fatalf("UTC entry month = %q, want %q", got, want)
	}
	if got, want := trades[1].ReportLocalWeekday, "Sun"; got != want {
		t.Fatalf("negative EntryT weekday fallback = %q, want %q", got, want)
	}
	if got := trades[1].ReportEntryMonth; got != "" {
		t.Fatalf("negative EntryT month fallback = %q, want empty", got)
	}
}

func mustAnnotateReportTrades(t *testing.T, trades []engine.Trade, series marketdata.Series, prepared *engine.PreparedRun, route reportTradeRoute) []Trade {
	t.Helper()
	annotated, err := annotateReportTrades(trades, series, prepared, route)
	if err != nil {
		t.Fatalf("annotateReportTrades: %v", err)
	}
	return annotated
}

func TestDefaultCostModesUseInstrumentCostChecks(t *testing.T) {
	cases := []struct {
		name   string
		symbol string
		checks [3]float64
	}{
		{name: "gold", symbol: "XAUUSD", checks: [3]float64{0, 0.06, 0.12}},
		{name: "euro dollar", symbol: "EURUSD", checks: [3]float64{0, 0.00005, 0.0001}},
		{name: "sterling dollar", symbol: "GBPUSD", checks: [3]float64{0, 0.00005, 0.0001}},
		{name: "sterling yen", symbol: "GBPJPY", checks: [3]float64{0, 0.005, 0.01}},
		{name: "aussie dollar", symbol: "AUDUSD", checks: [3]float64{0, 0.00005, 0.0001}},
		{name: "euro sterling", symbol: "EURGBP", checks: [3]float64{0, 0.00005, 0.0001}},
		{name: "dollar yen", symbol: "USDJPY", checks: [3]float64{0, 0.005, 0.01}},
		{name: "light crude", symbol: "LIGHTCMDUSD", checks: [3]float64{0, 0.02, 0.04}},
		{name: "brent crude", symbol: "BRENTCMDUSD", checks: [3]float64{0, 0.02, 0.04}},
		{name: "natural gas", symbol: "GASCMDUSD", checks: [3]float64{0, 0.0025, 0.005}},
		{name: "australia 200", symbol: "AUS200", checks: [3]float64{0, 0.5, 1}},
		{name: "unknown fallback", symbol: "US30", checks: [3]float64{0, 0.00005, 0.0001}},
		{name: "normalized symbol", symbol: "  xauusd  ", checks: [3]float64{0, 0.06, 0.12}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			modes, primary := CostModes(tc.symbol, nil)
			assertCostModes(t, modes, []CostMode{
				{Label: "raw", Slip: tc.checks[0]},
				{Label: "realistic", Slip: tc.checks[1], Primary: true},
				{Label: "harsh", Slip: tc.checks[2]},
			})
			if primary.Index != 1 || primary.Label != "realistic" {
				t.Fatalf("primary = %+v, want realistic index 1", primary)
			}
		})
	}

	slip := 0.2
	modes, primary := CostModes("XAUUSD", &slip)
	assertCostModes(t, modes, []CostMode{{Label: "slip 0.2", Slip: 0.2, Primary: true}})
	if primary.Index != 0 || primary.Label != "slip 0.2" {
		t.Fatalf("primary = %+v, want explicit slip", primary)
	}
}

func TestBTCUSDTDefaultCostModesUseBasisPoints(t *testing.T) {
	modes, primary := CostModes("btcusdt", nil)
	want := []CostMode{
		{Label: "raw", Bps: 0},
		{Label: "realistic", Bps: 5, Primary: true},
		{Label: "harsh", Bps: 10},
	}
	assertCostModes(t, modes, want)
	if primary.Index != 1 || primary.Label != "realistic" {
		t.Fatalf("primary = %+v, want realistic index 1", primary)
	}
}

func assertCostModes(t *testing.T, got []CostMode, want []CostMode) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("cost modes = %+v, want %+v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("cost mode %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
