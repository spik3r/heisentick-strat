package engine

import (
	"math"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func namedLevelFlagFixture() ([]marketdata.Bar, contextcols.Columns) {
	start := time.Date(2026, 9, 29, 1, 0, 0, 0, time.UTC).UnixMilli()
	bars := []marketdata.Bar{
		{O: 100, H: 101, L: 99.8, C: 100.5},
		{O: 100.7, H: 102.2, L: 100.5, C: 102},    // impulse through the stable PDH
		{O: 102, H: 102.1, L: 101.7, C: 101.8},    // first, opposite-color flag bar
		{O: 101.8, H: 102, L: 101.6, C: 101.9},    // second completed flag bar
		{O: 102.55, H: 102.8, L: 101.7, C: 102.3}, // opposite-color close beyond the prior flag high
		{O: 102.2, H: 102.5, L: 101.9, C: 102.1},
	}
	for i := range bars {
		bars[i].T = float64(start + int64(i)*5*time.Minute.Milliseconds())
		bars[i].V = 1
	}
	series := marketdata.SeriesFromBars(bars)
	cols := contextcols.Build(series, contextcols.Options{})
	cols.ATR = make([]float64, len(bars))
	cols.PriorDayH = make([]float64, len(bars))
	for i := range bars {
		cols.ATR[i] = 1
		cols.PriorDayH[i] = 101
	}
	cols.ATR[4] = 0.5 // breakout signal ATR differs from the impulse ATR
	return bars, cols
}

func TestNamedLevelFlagUsesNextOpenAndSignalAnchoredTwoR(t *testing.T) {
	bars, cols := namedLevelFlagFixture()
	series := marketdata.SeriesFromBars(bars)
	params := flagParams{
		SetupType: string(dsl.FamilyNamedLevelFlag), TradeWindowUnrestricted: true,
		AllowLong: true, AllowShort: true, RiskUSD: 200, MaxHoldBars: 24,
		NamedLevelFlag: namedLevelFlagParams{LevelPriority: []string{"PDH"}, ImpulseATR: .8, BreakDistanceATR: .1, MaxBars: 10, MinBars: 2, StopPaddingATR: .2, MinStopATR: .4, MaxStopATR: 2, TargetMode: "fixed2R", BodyATR: .5, TargetR: 2},
	}
	var b broker
	b.reset(series, cols, nil, nil, nil, params, RunFixture{Costs: Costs{FillOn: "nextOpen", StartEquity: 10000}}, nil)
	trades := b.run()
	if len(trades) != 1 {
		t.Fatalf("trades = %#v, want one named-level flag trade", trades)
	}
	trade := trades[0]
	if trade.EntryIndex != 5 {
		t.Fatalf("entry index = %d, want next-open index 5", trade.EntryIndex)
	}
	if trade.Meta["setup"] != "namedLevelFlag" || trade.Meta["levelKey"] != "PDH" {
		t.Fatalf("trade metadata = %#v", trade.Meta)
	}
	if math.Abs(trade.TP-103.9) > 1e-9 {
		t.Fatalf("target = %v, want 103.9 from breakout close 102.3 + 2R using signal ATR", trade.TP)
	}
}

func TestNamedLevelFlagShortUsesSignalATRStopAndNextOpen(t *testing.T) {
	bars, _ := namedLevelFlagFixture()
	// Mirror the long fixture around 101: the impulse breaches PDL, the first
	// two flag bars include a green pullback, and the final red close breaks
	// below the completed flag low.
	for i := range bars {
		bars[i].O, bars[i].H, bars[i].L, bars[i].C = 202-bars[i].O, 202-bars[i].L, 202-bars[i].H, 202-bars[i].C
	}
	series := marketdata.SeriesFromBars(bars)
	cols := contextcols.Build(series, contextcols.Options{})
	cols.ATR = make([]float64, len(bars))
	cols.PriorDayL = make([]float64, len(bars))
	for i := range bars {
		cols.ATR[i], cols.PriorDayL[i] = 1, 101
	}
	cols.ATR[4] = .5 // stop padding must use breakout signal ATR, not impulse ATR
	params := flagParams{SetupType: string(dsl.FamilyNamedLevelFlag), TradeWindowUnrestricted: true,
		AllowLong: true, AllowShort: true, RiskUSD: 200, MaxHoldBars: 24,
		NamedLevelFlag: namedLevelFlagParams{LevelPriority: []string{"PDL"}, ImpulseATR: .8, BreakDistanceATR: .1, MaxBars: 10, MinBars: 2, StopPaddingATR: .2, MinStopATR: .4, MaxStopATR: 2, TargetMode: "fixed2R", BodyATR: .5, TargetR: 2}}
	var b broker
	b.reset(series, cols, nil, nil, nil, params, RunFixture{Costs: Costs{FillOn: "nextOpen", StartEquity: 10000}}, nil)
	trades := b.run()
	if len(trades) != 1 {
		t.Fatalf("trades = %#v, want one short named-level flag trade", trades)
	}
	trade := trades[0]
	if trade.EntryIndex != 5 || trade.Side != "short" {
		t.Fatalf("entry=%d side=%v, want next-open index 5 and short", trade.EntryIndex, trade.Side)
	}
	wantStop := 202 - 101.6 + .2*.5
	if math.Abs(trade.SL-wantStop) > 1e-9 {
		t.Fatalf("stop = %v, want %v using breakout signal ATR", trade.SL, wantStop)
	}
}

func TestNamedLevelFlagHoldsFor24ObservedBars(t *testing.T) {
	bars, cols := namedLevelFlagFixture()
	for len(bars) < 30 {
		last := bars[len(bars)-1]
		last.T += float64(5 * time.Minute.Milliseconds())
		last.O, last.H, last.L, last.C = 102.2, 102.5, 101.9, 102.1
		bars = append(bars, last)
		cols.ATR = append(cols.ATR, 1)
		cols.PriorDayH = append(cols.PriorDayH, 101)
	}
	params := flagParams{SetupType: string(dsl.FamilyNamedLevelFlag), TradeWindowUnrestricted: true, AllowLong: true, AllowShort: true, RiskUSD: 200, MaxHoldBars: 24,
		NamedLevelFlag: namedLevelFlagParams{LevelPriority: []string{"PDH"}, ImpulseATR: .8, BreakDistanceATR: .1, MaxBars: 10, MinBars: 2, StopPaddingATR: .2, MinStopATR: .4, MaxStopATR: 2, TargetMode: "fixed2R", BodyATR: .5, TargetR: 2}}
	var b broker
	b.reset(marketdata.SeriesFromBars(bars), cols, nil, nil, nil, params, RunFixture{Costs: Costs{FillOn: "nextOpen", StartEquity: 10000}}, nil)
	trades := b.run()
	if len(trades) != 1 {
		t.Fatalf("trades = %#v, want one trade", trades)
	}
	if trades[0].EntryIndex != 5 || trades[0].ExitIndex != 29 || trades[0].Reason != "time" {
		t.Fatalf("entry=%d exit=%d reason=%q, want next-open index5 and time close after 24 observed bars at index29", trades[0].EntryIndex, trades[0].ExitIndex, trades[0].Reason)
	}
}

func TestNamedLevelFlagSignalRiskFilterConsumesArmedAttempt(t *testing.T) {
	bars, cols := namedLevelFlagFixture()
	series := marketdata.SeriesFromBars(bars)
	params := flagParams{
		SetupType: string(dsl.FamilyNamedLevelFlag), TradeWindowUnrestricted: true, AllowLong: true, AllowShort: true,
		RiskUSD: 200, MaxHoldBars: 24,
		NamedLevelFlag: namedLevelFlagParams{LevelPriority: []string{"PDH"}, ImpulseATR: .8, BreakDistanceATR: .1, MaxBars: 10, MinBars: 2, StopPaddingATR: .2, MinStopATR: .4, MaxStopATR: 2, TargetMode: "fixed2R", MinStopPoints: .81, TargetR: 2},
	}
	var b broker
	b.reset(series, cols, nil, nil, nil, params, RunFixture{Costs: Costs{FillOn: "nextOpen", StartEquity: 10000}}, nil)
	if trades := b.run(); len(trades) != 0 {
		t.Fatalf("trades = %#v, want the 0.8-point signal risk rejected by the 0.81-point filter", trades)
	}
	if b.nlfState != nil {
		t.Fatalf("armed state survived risk-filter rejection: %#v", b.nlfState)
	}
	day := floorDivInt64(int64(bars[4].T), 24*60*60*1000)
	if !b.seen.nlf.seen(day, "PDH") {
		t.Fatal("risk-floor rejection did not consume the armed level attempt")
	}
}

func TestNamedLevelFlagRequiresTwoCompletedBarsAndStableLevel(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func([]marketdata.Bar, *contextcols.Columns)
	}{
		{"one completed bar is insufficient", func(bars []marketdata.Bar, _ *contextcols.Columns) {
			bars[3].O = 101.9
			bars[3].H = 103
			bars[3].C = 102.5
		}},
		{"adverse midpoint wick invalidates", func(bars []marketdata.Bar, _ *contextcols.Columns) { bars[2].L = 101.2 }},
		{"changed named level invalidates", func(_ []marketdata.Bar, cols *contextcols.Columns) { cols.PriorDayH[3] = 101.2 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bars, cols := namedLevelFlagFixture()
			tc.mutate(bars, &cols)
			params := flagParams{SetupType: string(dsl.FamilyNamedLevelFlag), TradeWindowUnrestricted: true, AllowLong: true, AllowShort: true, RiskUSD: 200, MaxHoldBars: 24,
				NamedLevelFlag: namedLevelFlagParams{LevelPriority: []string{"PDH"}, ImpulseATR: .8, BreakDistanceATR: .1, MaxBars: 10, MinBars: 2, StopPaddingATR: .2, MinStopATR: .4, MaxStopATR: 2, TargetMode: "fixed2R", BodyATR: .5, TargetR: 2}}
			var b broker
			b.reset(marketdata.SeriesFromBars(bars), cols, nil, nil, nil, params, RunFixture{Costs: Costs{FillOn: "nextOpen", StartEquity: 10000}}, nil)
			if trades := b.run(); len(trades) != 0 {
				t.Fatalf("trades = %#v, want no entry", trades)
			}
		})
	}
}

func TestNamedLevelFlagKnownRoomDoesNotReplaceTwoRTarget(t *testing.T) {
	bars, cols := namedLevelFlagFixture()
	cols.PriorDayL = make([]float64, len(bars))
	for i := range cols.PriorDayL {
		cols.PriorDayL[i] = 97
	}
	cols.PriorSession.Asia.H = make([]float64, len(bars))
	for i := range cols.PriorSession.Asia.H {
		cols.PriorSession.Asia.H[i] = 104
	}
	params := flagParams{SetupType: string(dsl.FamilyNamedLevelFlag), TradeWindowUnrestricted: true, AllowLong: true, AllowShort: true, RiskUSD: 200, MaxHoldBars: 24,
		NamedLevelFlag: namedLevelFlagParams{LevelPriority: []string{"PDH", "PDL", "AH"}, ImpulseATR: .8, BreakDistanceATR: .1, MaxBars: 10, MinBars: 2, StopPaddingATR: .2, MinStopATR: .4, MaxStopATR: 2, TargetMode: "knownLevel2R", BodyATR: .5, TargetR: 2}}
	var b broker
	b.reset(marketdata.SeriesFromBars(bars), cols, nil, nil, nil, params, RunFixture{Costs: Costs{FillOn: "nextOpen", StartEquity: 10000}}, nil)
	trades := b.run()
	if len(trades) != 1 {
		t.Fatalf("trades = %#v, want stable known room ahead", trades)
	}
	if trades[0].Meta["targetLevelKey"] != "AH" || trades[0].Meta["targetLevelPrice"] != 104.0 {
		t.Fatalf("target metadata = %#v", trades[0].Meta)
	}
	if math.Abs(trades[0].TP-103.9) > 1e-9 {
		t.Fatalf("TP = %v, want signal-anchored 2R 103.9", trades[0].TP)
	}
}

func TestNamedLevelFlagKnownRoomRequiresAheadLevel(t *testing.T) {
	bars, cols := namedLevelFlagFixture()
	cols.PriorDayL = make([]float64, len(bars))
	for i := range cols.PriorDayL {
		cols.PriorDayL[i] = 97
	}
	params := flagParams{SetupType: string(dsl.FamilyNamedLevelFlag), TradeWindowUnrestricted: true, AllowLong: true, AllowShort: true, RiskUSD: 200, MaxHoldBars: 24,
		NamedLevelFlag: namedLevelFlagParams{LevelPriority: []string{"PDH", "PDL"}, ImpulseATR: .8, BreakDistanceATR: .1, MaxBars: 10, MinBars: 2, StopPaddingATR: .2, MinStopATR: .4, MaxStopATR: 2, TargetMode: "knownLevel2R", BodyATR: .5, TargetR: 2}}
	var b broker
	b.reset(marketdata.SeriesFromBars(bars), cols, nil, nil, nil, params, RunFixture{Costs: Costs{FillOn: "nextOpen", StartEquity: 10000}}, nil)
	if trades := b.run(); len(trades) != 0 {
		t.Fatalf("trades = %#v, want no trade without a stable known level ahead", trades)
	}
}

func TestNamedLevelFlagRejectsBreakoutAfterTenObservedBars(t *testing.T) {
	bars, _ := namedLevelFlagFixture()
	bars = append(bars, make([]marketdata.Bar, 6)...)
	for i := 4; i < 12; i++ {
		bars[i].O, bars[i].H, bars[i].L, bars[i].C = 101.9, 102.05, 101.65, 101.95
	}
	bars = append(bars, marketdata.Bar{O: 101.95, H: 102.8, L: 101.7, C: 102.3}, marketdata.Bar{O: 102.2, H: 102.5, L: 101.9, C: 102.1})
	for i := range bars {
		bars[i].T = float64(time.Date(2026, 9, 29, 1, 0, 0, 0, time.UTC).UnixMilli() + int64(i)*5*time.Minute.Milliseconds())
		bars[i].V = 1
	}
	cols := contextcols.Build(marketdata.SeriesFromBars(bars), contextcols.Options{})
	cols.ATR = make([]float64, len(bars))
	cols.PriorDayH = make([]float64, len(bars))
	for i := range bars {
		cols.ATR[i], cols.PriorDayH[i] = 1, 101
	}
	cols.ATR[12] = .5
	params := flagParams{SetupType: string(dsl.FamilyNamedLevelFlag), TradeWindowUnrestricted: true, AllowLong: true, AllowShort: true, RiskUSD: 200, MaxHoldBars: 24,
		NamedLevelFlag: namedLevelFlagParams{LevelPriority: []string{"PDH"}, ImpulseATR: .8, BreakDistanceATR: .1, MaxBars: 10, MinBars: 2, StopPaddingATR: .2, MinStopATR: .4, MaxStopATR: 2, TargetMode: "fixed2R", BodyATR: .5, TargetR: 2}}
	var b broker
	b.reset(marketdata.SeriesFromBars(bars), cols, nil, nil, nil, params, RunFixture{Costs: Costs{FillOn: "nextOpen", StartEquity: 10000}}, nil)
	if trades := b.run(); len(trades) != 0 {
		t.Fatalf("trades = %#v, want flag expired before late breakout", trades)
	}
}

func TestNamedLevelFlagUTCWindowKeepsOriginal18Boundary(t *testing.T) {
	for _, tc := range []struct {
		name      string
		shift     time.Duration
		wantTrade bool
	}{
		{"07:55 UTC signal fills at 08:00", 6*time.Hour + 35*time.Minute, true},
		{"08:00 UTC signal is outside the window", 6*time.Hour + 40*time.Minute, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bars, cols := namedLevelFlagFixture()
			for i := range bars {
				bars[i].T += float64(tc.shift.Milliseconds())
			}
			p := flagParams{SetupType: string(dsl.FamilyNamedLevelFlag), TradeWindowUTCHourFrom: 0, TradeWindowUTCHourTo: 8, TradeWindowUTCHourRangeSet: true,
				AllowLong: true, AllowShort: true, RiskUSD: 200, MaxHoldBars: 24,
				NamedLevelFlag: namedLevelFlagParams{LevelPriority: []string{"PDH"}, ImpulseATR: .8, BreakDistanceATR: .1, MaxBars: 10, MinBars: 2, StopPaddingATR: .2, MinStopATR: .4, MaxStopATR: 2, TargetMode: "fixed2R", BodyATR: .5, TargetR: 2}}
			var b broker
			b.reset(marketdata.SeriesFromBars(bars), cols, nil, nil, nil, p, RunFixture{Costs: Costs{FillOn: "nextOpen", StartEquity: 10000}}, nil)
			trades := b.run()
			if (len(trades) == 1) != tc.wantTrade {
				t.Fatalf("trades = %#v, wantTrade=%v", trades, tc.wantTrade)
			}
			if tc.wantTrade && trades[0].EntryIndex != 5 {
				t.Fatalf("entry index = %d, want next-open 08:00 bar", trades[0].EntryIndex)
			}
		})
	}
}

func TestNamedLevelFlagPositionDoesNotRefreshLevelCache(t *testing.T) {
	b := broker{hasPosition: true, nlfPrices: map[string]float64{"PDH": 101}, params: flagParams{NamedLevelFlag: namedLevelFlagParams{LevelPriority: []string{"PDH"}}}, series: marketdata.SeriesFromBars([]marketdata.Bar{{T: 1, O: 100, H: 101, L: 99, C: 100}})}
	b.onNamedLevelFlagBar(0)
	if b.nlfPrices["PDH"] != 101 {
		t.Fatalf("cached PDH = %v, want stale flat-bar value 101", b.nlfPrices["PDH"])
	}
}

func TestNamedLevelFlagBrokerResetClearsLevelCacheAndState(t *testing.T) {
	bars, cols := namedLevelFlagFixture()
	series := marketdata.SeriesFromBars(bars)
	b := broker{
		nlfPrices: map[string]float64{"PDH": 101},
		nlfState:  &namedLevelFlagState{Key: "PDH", Level: 101},
	}
	params := flagParams{SetupType: string(dsl.FamilyNamedLevelFlag), TradeWindowUnrestricted: true,
		NamedLevelFlag: namedLevelFlagParams{LevelPriority: []string{"PDH"}, MaxBars: 10}}
	b.reset(series, cols, nil, nil, nil, params, RunFixture{}, nil)
	b.setExecutionWindow(ExecutionBounds{TradeStart: 2, TradeEnd: series.Len() - 1})
	if b.nlfPrices != nil || b.nlfState != nil {
		t.Fatalf("reset retained named-level flag state: prices=%v state=%+v", b.nlfPrices, b.nlfState)
	}
	if trades := b.run(); len(trades) != 0 {
		t.Fatalf("windowed run trades = %#v, want no trade from pre-window bars", trades)
	}
	if b.nlfState != nil || b.nlfPrices["PDH"] != 101 {
		t.Fatalf("windowed run did not rebuild cache from its own bars: prices=%v state=%+v", b.nlfPrices, b.nlfState)
	}
}
