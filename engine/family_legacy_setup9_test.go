package engine

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// traceBars builds the contract section 8.1 bars: o = c-0.5, h = c+1, l = c-1.
func traceBars(closes []float64) [][]float64 {
	rows := make([][]float64, len(closes))
	for i, c := range closes {
		rows[i] = []float64{float64(1767571200000 + int64(i)*3600000), c - 0.5, c + 1, c - 1, c, 100}
	}
	return rows
}

func ramp(n int, start, step float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = start + step*float64(i)
	}
	return out
}

func storedInts(stored []int8) []int {
	out := make([]int, len(stored))
	for i, v := range stored {
		out[i] = int(v)
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Contract 8.1 traces A, B and C. These expectations are written from the
// contract text, not read back from the oracle file.
func TestLegacySetup9ContractTracesAToC(t *testing.T) {
	a := storedInts(legacySetup9Counter(ramp(14, 100, 1)))
	if want := []int{0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}; !equalInts(a, want) {
		t.Fatalf("trace A = %v, want %v", a, want)
	}
	closes := ramp(14, 100, 1)
	closes[10] = closes[6]
	if b, want := storedInts(legacySetup9Counter(closes)), []int{0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 0, 1, 2, 3}; !equalInts(b, want) {
		t.Fatalf("trace B = %v, want %v", b, want)
	}
	if c, want := storedInts(legacySetup9Counter([]float64{100, 101, 102, 103, 99, 98, 104, 105})), []int{0, 0, 0, 0, -1, -2, 1, 2}; !equalInts(c, want) {
		t.Fatalf("trace C = %v, want %v (a direct flip with no price-flip gate)", c, want)
	}
	if got := legacySetup9Counter([]float64{1, 2, 3, 4}); len(got) != 4 || got[3] != 0 {
		t.Fatalf("fewer than 5 bars must give zeros, got %v", got)
	}
}

// Contract 8.1 trace E: the stored count wraps, the internal count does not.
func TestLegacySetup9Int8WrapIsPreserved(t *testing.T) {
	rise := legacySetup9Counter(ramp(300, 100, 1))
	for index, want := range map[int]int{12: 9, 129: 126, 130: 127, 131: -128, 132: -127, 250: -9, 257: -2, 258: -1, 259: 0, 260: 1, 268: 9} {
		if int(rise[index]) != want {
			t.Errorf("rising stored[%d] = %d, want %d", index, rise[index], want)
		}
	}
	fall := legacySetup9Counter(ramp(300, 1000, -1))
	for index, want := range map[int]int{12: -9, 130: -127, 131: -128, 132: 127, 250: 9, 258: 1, 259: 0, 260: -1, 268: -9} {
		if int(fall[index]) != want {
			t.Errorf("falling stored[%d] = %d, want %d", index, fall[index], want)
		}
	}
	// Internal count keeps going: the sign flips at 131 are not resets. After
	// the stored value passes through 0 at 259 the run is still the same
	// monotone run (count 256 at index 259), so index 260 continues upward.
	seen := map[int]bool{}
	for i, v := range rise {
		if v == 9 || v == -9 {
			seen[i] = true
		}
	}
	if len(seen) != 3 || !seen[12] || !seen[250] || !seen[268] {
		t.Fatalf("rising |stored| == 9 at %v, want exactly 12, 250, 268", seen)
	}
}

func signalBarsFor(t *testing.T, profile string, rows [][]float64, costs Costs) (*PreparedRun, []int, []string) {
	t.Helper()
	request := legacySetup9Request(t, profile, "1h", oracleSeries(rows), costs)
	prepared, err := PrepareRun(request)
	if err != nil {
		t.Fatal(err)
	}
	prepared.broker.reset(prepared.series, prepared.cols, prepared.htfTrend, prepared.ema, prepared.emaSlope, prepared.params, prepared.fixture, nil)
	var at []int
	var sides []string
	for i := 0; i < prepared.series.Len(); i++ {
		if s, ok := prepared.broker.legacySetup9Signal(i); ok {
			at = append(at, i)
			sides = append(sides, s.String())
		}
	}
	return prepared, at, sides
}

func TestLegacySetup9EmitsPhantomAndRepeatedNines(t *testing.T) {
	_, at, sides := signalBarsFor(t, dsl.LegacySetup9Profile, traceBars(ramp(300, 100, 1)), Costs{})
	if !equalInts(at, []int{12, 250, 268}) || strings.Join(sides, ",") != "short,long,short" {
		t.Fatalf("rising signals %v %v, want short at 12, long at 250, short at 268", at, sides)
	}
	_, at, sides = signalBarsFor(t, dsl.LegacySetup9Profile, traceBars(ramp(300, 1000, -1)), Costs{})
	if !equalInts(at, []int{12, 250, 268}) || strings.Join(sides, ",") != "long,short,long" {
		t.Fatalf("falling signals %v %v, want the mirror", at, sides)
	}
}

func TestLegacySetup9TraceCHasNoFlipGateAndRepeatsNines(t *testing.T) {
	// Six bars after a nine on a 15-bar rise: the stored count is 10 at
	// index 13, not a new nine, so a later reversal needs its own nine.
	_, at, _ := signalBarsFor(t, dsl.LegacySetup9Profile, traceBars(ramp(14, 100, 1)), Costs{})
	if !equalInts(at, []int{12}) {
		t.Fatalf("signals = %v, want only index 12 (count 10 is not a signal)", at)
	}
	// A direct flip from a sell run to a buy count of 9 needs no reset bar.
	closes := append(ramp(10, 100, 1), ramp(14, 109, -1)...)
	stored := storedInts(legacySetup9Counter(closes))
	signalAt := -1
	for i, v := range stored {
		if v == -9 {
			signalAt = i
			break
		}
	}
	if signalAt < 0 {
		t.Fatalf("no buy nine after a direct flip: %v", stored)
	}
	_, at, sides := signalBarsFor(t, dsl.LegacySetup9Profile, traceBars(closes), Costs{})
	foundLong := false
	for k, i := range at {
		if i == signalAt && sides[k] == "long" {
			foundLong = true
		}
	}
	if !foundLong {
		t.Fatalf("expected a long at %d, got %v %v", signalAt, at, sides)
	}
}

// Contract 8.1 D: perfection is inclusive.
func TestLegacySetup9PerfectionIsInclusive(t *testing.T) {
	rows := traceBars(ramp(14, 100, 1))
	rows[9][2], rows[10][2], rows[11][2], rows[12][2] = 114, 115, 115, 115
	high := make([]float64, len(rows))
	low := make([]float64, len(rows))
	for i, r := range rows {
		high[i], low[i] = r[2], r[3]
	}
	if !legacySetup9Perfected(high, low, 12, true) {
		t.Fatal("max(h11,h12) = 115 >= max(h9,h10) = 115 must be perfected for the legacy profile")
	}
	if strictly := math.Max(high[11], high[12]) > math.Max(high[9], high[10]); strictly {
		t.Fatal("test setup: the strict comparison must be false so the pair separates legacy from strict")
	}
	// Buy side mirror: min(l[i-1], l[i]) <= min(l[i-3], l[i-2]).
	lows := []float64{10, 9, 8, 7, 8, 7}
	highs := []float64{20, 20, 20, 20, 20, 20}
	if !legacySetup9Perfected(highs, lows, 5, false) {
		t.Fatal("equal lows must be perfected for a buy")
	}
	lows[5], lows[4] = 8, 8
	if legacySetup9Perfected(highs, lows, 5, false) {
		t.Fatal("lows above both earlier bars must not be perfected")
	}
	for i := 0; i < 3; i++ {
		if legacySetup9Perfected(highs, lows, i, true) || legacySetup9Perfected(highs, lows, i, false) {
			t.Fatalf("bar %d is before index 3 and must not be perfected", i)
		}
	}
}

func TestLegacySetup9SeasonalGateBoundaries(t *testing.T) {
	profile := legacySetup9Profiles[dsl.LegacySetup9PerfSeasonalProfile]
	pct := func(v float64) *float64 { return &v }
	next := func(n int, p *float64) *contextcols.SeasonalityProjection {
		return &contextcols.SeasonalityProjection{Bucket: "09:00", DirectionalCount: n, BullishPercent: p}
	}
	cases := []struct {
		name  string
		next  *contextcols.SeasonalityProjection
		sell  bool
		agree bool
	}{
		{"no next bucket long", nil, false, false},
		{"no next bucket short", nil, true, false},
		{"no directional bars", next(0, nil), false, false},
		{"nil percent with samples", next(12, nil), true, false},
		{"long at 60 with 10", next(10, pct(60)), false, true},
		{"long just below 60", next(10, pct(59.99999999999999)), false, false},
		{"long at 60 with 9", next(9, pct(60)), false, false},
		{"long at 100 with 10", next(10, pct(100)), false, true},
		{"long at 50", next(20, pct(50)), false, false},
		{"short at 40 with 10", next(10, pct(40)), true, true},
		{"short just above 40", next(10, pct(40.00000000000001)), true, false},
		{"short at 40 with 9", next(9, pct(40)), true, false},
		{"short at 0 with 10", next(10, pct(0)), true, true},
		{"short at 60", next(20, pct(60)), true, false},
		{"long at 40", next(20, pct(40)), false, false},
	}
	for _, c := range cases {
		if got := legacySetup9SeasonAgrees(profile, c.next, c.sell); got != c.agree {
			t.Errorf("%s: agrees = %v, want %v", c.name, got, c.agree)
		}
	}
}

func TestLegacySetup9ProfileDefaultsFollowJavaScriptOperators(t *testing.T) {
	zero := legacyNum(0)
	raw := legacySetup9Raw{SetupCount: zero, StopBufferATR: zero, RMultiple: zero, SeasonalityEdge: zero, MinSamples: zero, RequirePerfection: true}
	p := raw.resolve("test", true, "L", "S")
	if p.SetupCount != 9 {
		t.Errorf("setupCount 0 || 9 = %v", p.SetupCount)
	}
	if p.StopBufferATR != 0.5 {
		t.Errorf("stopBufferAtr 0 || 0.5 = %v", p.StopBufferATR)
	}
	if p.RMultiple != 0 {
		t.Errorf("rMultiple has no fallback: 0 stays %v (target = entry)", p.RMultiple)
	}
	if p.SeasonalityEdge != 0 {
		t.Errorf("seasonalityEdge 0 ?? 10 stays 0, got %v", p.SeasonalityEdge)
	}
	if p.MinSamples != 10 {
		t.Errorf("seasonalityMinSamples 0 || 10 = %v", p.MinSamples)
	}
	undefined := legacySetup9Raw{}.resolve("test", true, "L", "S")
	if undefined.SetupCount != 9 || undefined.StopBufferATR != 0.5 || undefined.SeasonalityEdge != 10 || undefined.MinSamples != 10 || !math.IsNaN(undefined.RMultiple) {
		t.Errorf("undefined parameters = %+v", undefined)
	}
	nan := math.NaN()
	if legacyOr(&nan, 7) != 7 {
		t.Error("NaN is falsy for ||")
	}
	// The shipped profiles.
	raws := legacySetup9Profiles[dsl.LegacySetup9Profile]
	if raws.SetupCount != 9 || raws.StopBufferATR != 0.5 || raws.RMultiple != 2 || raws.RequirePerfection || raws.Seasonal {
		t.Errorf("raw profile = %+v", raws)
	}
	seasonal := legacySetup9Profiles[dsl.LegacySetup9PerfSeasonalProfile]
	if seasonal.SetupCount != 9 || seasonal.StopBufferATR != 0.5 || seasonal.RMultiple != 2 || !seasonal.RequirePerfection ||
		!seasonal.Seasonal || seasonal.SeasonalityEdge != 10 || seasonal.MinSamples != 10 {
		t.Errorf("seasonal profile = %+v", seasonal)
	}
}

func TestLegacySetup9RefusesOtherFills(t *testing.T) {
	rows := traceBars(ramp(60, 100, 1))
	for _, fillOn := range []string{"open", "nextOpen", "bogus"} {
		request := legacySetup9Request(t, dsl.LegacySetup9Profile, "1h", oracleSeries(rows), Costs{})
		request.Costs.FillOn = fillOn
		if _, err := Run(request); err == nil || !strings.Contains(err.Error(), "supports only costs.fillOn=close") {
			t.Errorf("fillOn %q: err = %v", fillOn, err)
		}
		if _, err := RunPrefix(request); err == nil {
			t.Errorf("RunPrefix fillOn %q accepted", fillOn)
		}
		if _, err := PrepareSharedRunContext(request); err == nil {
			t.Errorf("shared context fillOn %q accepted", fillOn)
		}
	}
	// A prepared run must not be re-costed to another fill either.
	prepared, err := PrepareRun(legacySetup9Request(t, dsl.LegacySetup9Profile, "1h", oracleSeries(rows), Costs{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.RunChecked(Costs{FillOn: "nextOpen"}); err == nil {
		t.Error("RunChecked accepted nextOpen")
	}
	if _, err := prepared.RunChecked(Costs{FillOn: "close"}); err != nil {
		t.Errorf("RunChecked close: %v", err)
	}
	for _, fillOn := range []string{"open", "nextOpen"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("PreparedRun.Run accepted fillOn %q", fillOn)
				}
			}()
			prepared.Run(Costs{FillOn: fillOn})
		}()
	}
	prepared.Run(Costs{FillOn: "close"})
}

func TestLegacySetup9FixtureRunnerRefusesOtherFills(t *testing.T) {
	fixture := RunFixture{Case: "x", Symbol: "XAUUSD", Timeframe: "1h", RangeMethod: "zone", Costs: Costs{FillOn: "nextOpen"}}
	fixture.Bars = marketdataBars(traceBars(ramp(60, 100, 1)))
	if _, err := RunFixtureCase(fixture, legacySetup9Source(dsl.LegacySetup9Profile, "1h")); err == nil || !strings.Contains(err.Error(), "supports only costs.fillOn=close") {
		t.Fatalf("err = %v", err)
	}
	fixture.Costs.FillOn = "close"
	if _, err := RunFixtureCase(fixture, legacySetup9Source(dsl.LegacySetup9Profile, "1h")); err != nil {
		t.Fatal(err)
	}
}

func marketdataBars(rows [][]float64) []marketdata.Bar {
	bars := make([]marketdata.Bar, len(rows))
	for i, r := range rows {
		bars[i] = marketdata.Bar{T: r[0], O: r[1], H: r[2], L: r[3], C: r[4], V: r[5]}
	}
	return bars
}

func TestLegacySetup9HandBuiltConfigWithoutProfileFailsClearly(t *testing.T) {
	cfg := legacySetup9Config(t, dsl.LegacySetup9Profile, "1h")
	cfg["legacySetup9"] = map[string]any{}
	request := legacySetup9Request(t, dsl.LegacySetup9Profile, "1h", oracleSeries(traceBars(ramp(60, 100, 1))), Costs{})
	request.Config = cfg
	if _, err := Run(request); err == nil || !strings.Contains(err.Error(), "known sequential profile") {
		t.Fatalf("err = %v, want a profile error and not a zero-trade success", err)
	}
}

func TestLegacySetup9ProfilesBindToDifferentDigests(t *testing.T) {
	rows := traceBars(ramp(60, 100, 1))
	raw := legacySetup9Request(t, dsl.LegacySetup9Profile, "1h", oracleSeries(rows), Costs{})
	seasonal := legacySetup9Request(t, dsl.LegacySetup9PerfSeasonalProfile, "1h", oracleSeries(rows), Costs{})
	a, err := prefixCheckpointBindingDigest(raw)
	if err != nil {
		t.Fatal(err)
	}
	b, err := prefixCheckpointBindingDigest(seasonal)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two profiles share a binding digest")
	}
	if legacySetup9Source(dsl.LegacySetup9Profile, "1h") == legacySetup9Source(dsl.LegacySetup9PerfSeasonalProfile, "1h") {
		t.Fatal("two profiles share a source")
	}
}

// On a timeframe of two hours or more with hour-aligned opens the next UTC
// hour has no bucket, so the seasonal profile cannot signal (contract 8.3).
func TestLegacySetup9SeasonalProfileNeverSignalsOnTwoHourBars(t *testing.T) {
	rows := make([][]float64, 0, 2400)
	price := 2000.0
	state := uint32(12345)
	prev := 0.0
	for i := 0; i < 2400; i++ {
		state = state*1664525 + 1013904223
		z := float64(state>>8)/float64(1<<24) - 0.5
		ret := 0.7*prev + z*2
		prev = ret
		o, c := price, price+ret
		rows = append(rows, []float64{float64(1767571200000 + int64(i)*7200000), o, math.Max(o, c) + 0.3, math.Min(o, c) - 0.3, c, 100})
		price = c
	}
	_, rawAt, _ := signalBarsForTimeframe(t, dsl.LegacySetup9Profile, "2h", rows)
	_, seasonalAt, _ := signalBarsForTimeframe(t, dsl.LegacySetup9PerfSeasonalProfile, "2h", rows)
	if len(rawAt) == 0 {
		t.Fatal("test setup: raw profile found no nine")
	}
	if len(seasonalAt) != 0 {
		t.Fatalf("seasonal profile signalled at %v on 2h bars", seasonalAt)
	}
}

func signalBarsForTimeframe(t *testing.T, profile, timeframe string, rows [][]float64) (*PreparedRun, []int, []string) {
	t.Helper()
	request := legacySetup9Request(t, profile, timeframe, oracleSeries(rows), Costs{})
	prepared, err := PrepareRun(request)
	if err != nil {
		t.Fatal(err)
	}
	prepared.broker.reset(prepared.series, prepared.cols, prepared.htfTrend, prepared.ema, prepared.emaSlope, prepared.params, prepared.fixture, nil)
	var at []int
	var sides []string
	for i := 0; i < prepared.series.Len(); i++ {
		if s, ok := prepared.broker.legacySetup9Signal(i); ok {
			at = append(at, i)
			sides = append(sides, s.String())
		}
	}
	return prepared, at, sides
}

func TestLegacySetup9TradeMetaAndTags(t *testing.T) {
	result, err := Run(legacySetup9Request(t, dsl.LegacySetup9Profile, "1h", oracleSeries(traceBars(ramp(14, 100, 1))), Costs{}))
	if err != nil || len(result.Trades) != 1 {
		t.Fatalf("%v %v", err, result.Trades)
	}
	trade := result.Trades[0]
	if trade.Tag != "TD-SELL-9" || trade.Side != "short" || trade.Meta["profile"] != dsl.LegacySetup9Profile || trade.Meta["setup"] != "legacySetup9" {
		t.Fatalf("trade = %+v", trade)
	}
	if trade.EntryIndex != 12 || trade.Entry != 112 {
		t.Fatalf("entry %d at %v, want index 12 at the signal-bar close 112 (no slippage)", trade.EntryIndex, trade.Entry)
	}
}

// Prefix invariance and batch/incremental/checkpoint agreement.
func prefixRequest(t *testing.T, profile string, c oracleCase, count int, slip float64) RunRequest {
	t.Helper()
	series := seriesPrefix(oracleSeries(c.Bars), count)
	return legacySetup9Request(t, profile, c.Timeframe, series, Costs{Slippage: slip, FeePerUnit: 0.02})
}

func findOracleCase(t *testing.T, name string) oracleCase {
	t.Helper()
	for _, c := range loadLegacySetup9Oracle(t).Cases {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no oracle case %s", name)
	return oracleCase{}
}

func TestLegacySetup9PrefixRunsAreInvariant(t *testing.T) {
	for _, name := range []string{"walk-1h-continuous", "walk-1h-weekends", "trace-e-rise-300"} {
		c := findOracleCase(t, name)
		n := len(c.Bars)
		for _, profile := range dsl.LegacySetup9Profiles {
			full, err := RunPrefix(prefixRequest(t, profile, c, n, 0.07))
			if err != nil {
				t.Fatal(err)
			}
			for k := 5; k <= n; k += 61 {
				got, err := RunPrefix(prefixRequest(t, profile, c, k, 0.07))
				if err != nil {
					t.Fatalf("%s/%s k=%d: %v", name, profile, k, err)
				}
				var wantClosed []PrefixClosedTrade
				for _, trade := range full.Trades {
					if trade.Trade.ExitIndex <= k-1 {
						wantClosed = append(wantClosed, trade)
					}
				}
				requireSameJSON(t, nonNilClosed(got.Trades), nonNilClosed(wantClosed))
				// A position open at k is the full run's trade (or open position) that has entered by then.
				var wantOpen []OpenPositionSnapshot
				for _, trade := range full.Trades {
					if trade.Trade.EntryIndex <= k-1 && trade.Trade.ExitIndex > k-1 {
						wantOpen = append(wantOpen, openSnapshotAt(trade.Trade, trade.PositionID))
					}
				}
				for _, open := range full.OpenPositions {
					if open.EntryIndex <= k-1 {
						wantOpen = append(wantOpen, open)
					}
				}
				if len(got.OpenPositions) != len(wantOpen) {
					t.Fatalf("%s/%s k=%d: %d open, want %d", name, profile, k, len(got.OpenPositions), len(wantOpen))
				}
				for i := range wantOpen {
					g, w := got.OpenPositions[i], wantOpen[i]
					if g.PositionID != w.PositionID || g.EntryIndex != w.EntryIndex || g.Entry != w.Entry || g.InitialSL != w.InitialSL || g.InitialTP != w.InitialTP || g.Size != w.Size || g.Side != w.Side {
						t.Fatalf("%s/%s k=%d: open %+v, want %+v", name, profile, k, g, w)
					}
				}
			}
		}
	}
}

func nonNilClosed(in []PrefixClosedTrade) []PrefixClosedTrade {
	if in == nil {
		return []PrefixClosedTrade{}
	}
	return in
}

func openSnapshotAt(trade Trade, id string) OpenPositionSnapshot {
	return OpenPositionSnapshot{PositionID: id, Side: trade.Side, Entry: trade.Entry, EntryIndex: trade.EntryIndex, InitialSL: trade.InitialSL, InitialTP: trade.InitialTP, Size: trade.Size}
}

func TestLegacySetup9BatchEqualsIncrementalPrefixPlusLiquidation(t *testing.T) {
	for _, name := range []string{"walk-1h-continuous", "walk-15m-weekends"} {
		c := findOracleCase(t, name)
		n := len(c.Bars)
		for _, profile := range dsl.LegacySetup9Profiles {
			batch, err := Run(prefixRequest(t, profile, c, n, 0.07))
			if err != nil {
				t.Fatal(err)
			}
			prefix, err := RunPrefix(prefixRequest(t, profile, c, n, 0.07))
			if err != nil {
				t.Fatal(err)
			}
			want := len(prefix.Trades) + len(prefix.OpenPositions)
			if len(batch.Trades) != want {
				t.Fatalf("%s/%s: batch %d trades, prefix %d closed + %d open", name, profile, len(batch.Trades), len(prefix.Trades), len(prefix.OpenPositions))
			}
			for i, closed := range prefix.Trades {
				b := batch.Trades[i]
				if b.EntryIndex != closed.Trade.EntryIndex || b.ExitIndex != closed.Trade.ExitIndex || b.Side != closed.Trade.Side || b.Reason != closed.Trade.Reason {
					t.Fatalf("%s/%s trade %d differs: %+v vs %+v", name, profile, i, b, closed.Trade)
				}
			}
			if len(prefix.OpenPositions) == 1 {
				last := batch.Trades[len(batch.Trades)-1]
				if last.Reason != ReasonEndOfTest || last.EntryIndex != prefix.OpenPositions[0].EntryIndex {
					t.Fatalf("%s/%s: final liquidation %+v vs open %+v", name, profile, last, prefix.OpenPositions[0])
				}
			}
		}
	}
}

func TestLegacySetup9CheckpointResumeMatchesFullReplay(t *testing.T) {
	for _, name := range []string{"walk-1h-continuous", "walk-1h-weekends", "trace-e-fall-300"} {
		c := findOracleCase(t, name)
		n := len(c.Bars)
		for _, profile := range dsl.LegacySetup9Profiles {
			var checkpoint []byte
			var accumulated []PrefixClosedTrade
			steps := []int{40, 130, 131, 133, 400, 900, 901, 1700, 2400, n}
			for _, k := range steps {
				if k > n {
					k = n
				}
				result, next, err := RunPrefixResumable(prefixRequest(t, profile, c, k, 0.07), checkpoint)
				if err != nil {
					t.Fatalf("%s/%s k=%d: %v", name, profile, k, err)
				}
				accumulated = append(accumulated, result.Trades...)
				full, err := RunPrefix(prefixRequest(t, profile, c, k, 0.07))
				if err != nil {
					t.Fatal(err)
				}
				requireSameJSON(t, nonNilClosed(accumulated), nonNilClosed(full.Trades))
				requireSameJSON(t, result.OpenPositions, full.OpenPositions)
				if result.CheckpointDigest != full.CheckpointDigest {
					t.Fatalf("%s/%s k=%d: digest differs", name, profile, k)
				}
				checkpoint = next
			}
		}
	}
}

func TestLegacySetup9CheckpointSupportDoesNotWidenOtherFamilies(t *testing.T) {
	for _, family := range []dsl.FamilyID{
		dsl.FamilySMAGoldenCross, dsl.FamilyFailedBreakout, dsl.FamilyNamedLevelFlag, dsl.FamilyPriceMomentum,
		dsl.FamilyKeltnerReversion, dsl.FamilyWeekendExtremeFade,
	} {
		request := openingRangeCheckpointRequest(t, 36, "close")
		request.Config["setupType"] = string(family)
		_, _, err := RunPrefixResumable(request, nil)
		var unsupported *PrefixCheckpointUnsupportedError
		if !errors.As(err, &unsupported) {
			t.Errorf("%s: err = %v, want checkpoint unsupported", family, err)
		}
	}
	// The family accepts a checkpoint, and refuses the other unsupported inputs
	// exactly as the opening-range family does.
	c := findOracleCase(t, "trace-e-rise-300")
	request := prefixRequest(t, dsl.LegacySetup9Profile, c, 100, 0)
	request.ExecutionWindow = &ExecutionWindow{}
	if _, _, err := RunPrefixResumable(request, nil); err == nil {
		t.Error("legacy setup 9 accepted a bounded execution window")
	}
	request = prefixRequest(t, dsl.LegacySetup9Profile, c, 100, 0)
	request.SourceSeries = request.Series
	if _, _, err := RunPrefixResumable(request, nil); err == nil {
		t.Error("legacy setup 9 accepted source-series replay")
	}
}
