package engine

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const entryRetestSource = `dsl v7
strategy "Retest on entry timeframe" { description "Synthetic zone retested on the entry candles." }
market conditions {
  slices(XAUUSD 1h)
  sessions(london, ny)
  day type in (trending, ranging, choppy)
  movement below 1.0
}
setup {
  type: supply demand
  source timeframe 4h
  retest on entry timeframe
  patterns(DBR, RBR, RBD, DBD)
  base 1 to 4 candles
  impulse at least 2.5 ATR within 4 candles
  zone width max 1.0 ATR
  retest within 96 candles
  retest first touch only
  retest must reject zone
  wait 0 candles after zone
}
filters {
  higher timeframe off
  side both
}
risk {
  stop: beyond zone + 0.3 ATR min 0.4 ATR max 3 ATR
}
target {
  target 2R
}
management {
  breakeven off
  wait 6 candles after trade
}
entryTf 1h
`

const (
	hourMS     = 3600e3
	retestBar  = 157 // 13:00 UTC, inside the second source candle after the zone
	zoneSource = 34  // source index at which the departure completes
)

// entryRetestBars builds a 1h chart and its 4h source: a calm stretch, a one-bar
// base at source 30, a rally through source 34, a plateau, then one 1h bar at
// retestBar that dips into the zone and closes back above it.
func entryRetestBars(mutateAfterSource int) (chart []marketdata.Bar, source []marketdata.Bar) {
	n := 4 * 50
	chart = make([]marketdata.Bar, n)
	for i := 0; i < n; i++ {
		o, c := 100.0, 100.0
		h, l := 100.5, 99.5
		switch {
		case i >= 120 && i < 124: // base candle(s)
			h, l = 100.2, 99.8
		case i >= 124 && i < 140: // rally, +0.5 a bar
			o = 100 + 0.5*float64(i-124)
			c = o + 0.5
			h, l = c+0.1, o-0.1
		case i >= 140:
			o, c = 108, 108
			h, l = 108.5, 107.5
		}
		if i == retestBar {
			o, h, l, c = 108, 108.2, 100.1, 101
		}
		chart[i] = marketdata.Bar{T: float64(i) * hourMS, O: o, H: h, L: l, C: c, V: 1}
	}
	if mutateAfterSource >= 0 {
		for i := 4 * mutateAfterSource; i < n; i++ {
			chart[i].H += 40
			chart[i].L -= 40
			chart[i].C += 20
		}
	}
	source = make([]marketdata.Bar, 0, n/4)
	for s := 0; s < n/4; s++ {
		bar := marketdata.Bar{T: chart[4*s].T, O: chart[4*s].O, H: math.Inf(-1), L: math.Inf(1), C: chart[4*s+3].C, V: 4}
		for j := 4 * s; j < 4*s+4; j++ {
			bar.H = math.Max(bar.H, chart[j].H)
			bar.L = math.Min(bar.L, chart[j].L)
		}
		source = append(source, bar)
	}
	return chart, source
}

func runEntryRetest(t *testing.T, chart, source []marketdata.Bar) RunResult {
	t.Helper()
	fixture := RunFixture{
		Schema: runFixtureSchema, Case: "entry-retest", StrategyID: "entry-retest", Symbol: "XAUUSD", Timeframe: "1h",
		SourceTimeframe: "4h", RangeMethod: "zone", Bars: chart, SourceBars: source,
	}
	result, err := RunFixtureCase(fixture, entryRetestSource)
	if err != nil {
		t.Fatalf("RunFixtureCase: %v", err)
	}
	return result
}

func TestRetestOnEntryTimeframeEntersOnTheEntryCandle(t *testing.T) {
	chart, source := entryRetestBars(-1)
	result := runEntryRetest(t, chart, source)
	if len(result.Trades) != 1 {
		t.Fatalf("trades = %d, want 1: %+v", len(result.Trades), result.Trades)
	}
	trade := result.Trades[0]
	if trade.EntryIndex != retestBar || trade.Side != "long" {
		t.Fatalf("entry index %d side %s, want %d long (the 1h rejection candle, not a 4h close)", trade.EntryIndex, trade.Side, retestBar)
	}
	if retestBar%4 == 3 {
		t.Fatal("test bar must not sit on a 4h boundary")
	}
	meta := trade.Meta
	if meta["zoneType"] != "demand" || int(meta["baseEnd"].(int)) != 30 {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestRetestOnEntryTimeframeIgnoresSourceCandlesThatHaveNotClosed(t *testing.T) {
	chart, source := entryRetestBars(-1)
	base := runEntryRetest(t, chart, source)
	// Wreck every source candle from index 40 on: the entry at chart bar 157 closes
	// inside source candle 39, so nothing decided up to there may change.
	mutatedChart, mutatedSource := entryRetestBars(40)
	changed := runEntryRetest(t, mutatedChart, mutatedSource)
	if len(base.Trades) != 1 || len(changed.Trades) == 0 {
		t.Fatalf("base %d, changed %d trades", len(base.Trades), len(changed.Trades))
	}
	got, want := changed.Trades[0], base.Trades[0]
	if got.EntryIndex != want.EntryIndex || got.Entry != want.Entry || got.InitialSL != want.InitialSL || got.InitialTP != want.InitialTP {
		t.Fatalf("future source candles changed the entry: got %+v want %+v", got, want)
	}
}

func TestRetestOnEntryTimeframeNeedsAFullyClosedZoneCandle(t *testing.T) {
	chart, source := entryRetestBars(-1)
	// A touch inside the very source candle that completes the departure must not trade.
	chart[4*zoneSource+1] = marketdata.Bar{T: chart[4*zoneSource+1].T, O: 107, H: 107.2, L: 100.1, C: 101, V: 1}
	for s := range source {
		if s == zoneSource {
			source[s].L = math.Min(source[s].L, 100.1)
		}
	}
	result := runEntryRetest(t, chart, source)
	for _, trade := range result.Trades {
		if trade.EntryIndex < 4*(zoneSource+1) {
			t.Fatalf("entered on bar %d before source candle %d closed", trade.EntryIndex, zoneSource)
		}
	}
}

func retestBoundarySource(chart []marketdata.Bar) []marketdata.Bar {
	source := make([]marketdata.Bar, len(chart)/4)
	for s := range source {
		b := marketdata.Bar{T: chart[4*s].T, O: chart[4*s].O, H: math.Inf(-1), L: math.Inf(1), C: chart[4*s+3].C, V: 4}
		for j := 4 * s; j < 4*s+4; j++ {
			b.H = math.Max(b.H, chart[j].H)
			b.L = math.Min(b.L, chart[j].L)
		}
		source[s] = b
	}
	return source
}

func retestBoundaryFixture(t *testing.T, flip, mirror, laterTouch bool, entryTimeframe ...string) (RunResult, int) {
	t.Helper()
	chart, _ := entryRetestBars(-1)
	// Keep the formation and flip boundary in existing London/NY windows.
	for j := range chart {
		chart[j].T += 12 * hourMS
	}
	boundary := 4*zoneSource + 3
	// A larger final departure isolates this formation from earlier candidate zones.
	chart[boundary].H, chart[boundary].C = 116.1, 116
	if flip {
		boundary += 4
		chart[boundary].L, chart[boundary].C = 94.9, 95
		if laterTouch {
			chart[boundary+1].O, chart[boundary+1].H, chart[boundary+1].L, chart[boundary+1].C = 95, 100.1, 94.9, 99
		}
	} else {
		chart[boundary].L = 100.1
		if laterTouch {
			chart[boundary+1].O, chart[boundary+1].H, chart[boundary+1].L, chart[boundary+1].C = 108, 108.2, 100.1, 101
		}
	}
	if mirror {
		for j := range chart {
			b := chart[j]
			chart[j].O, chart[j].H, chart[j].L, chart[j].C = 200-b.O, 200-b.L, 200-b.H, 200-b.C
		}
	}
	source := retestBoundarySource(chart)
	tf, parts := "1h", 1
	if len(entryTimeframe) > 0 {
		tf = entryTimeframe[0]
	}
	switch tf {
	case "30m":
		parts = 2
	case "15m":
		parts = 4
	}
	if parts > 1 {
		fine := make([]marketdata.Bar, 0, len(chart)*parts)
		for hour, b := range chart {
			active := parts - 1
			if laterTouch && hour == boundary+1 {
				active = 0
			}
			for j := 0; j < parts; j++ {
				px := b.O
				if j > active {
					px = b.C
				}
				v := marketdata.Bar{T: b.T + float64(j)*hourMS/float64(parts), O: px, H: px, L: px, C: px, V: 1 / float64(parts)}
				if j == active {
					v.O, v.H, v.L, v.C = b.O, b.H, b.L, b.C
				}
				fine = append(fine, v)
			}
		}
		chart = fine
		boundary = (boundary+1)*parts - 1
	}
	cfg := strings.Replace(entryRetestSource, "min 0.4 ATR max 3 ATR", "min 0.4 ATR max 30 ATR", 1)
	cfg = strings.Replace(cfg, "impulse at least 2.5 ATR", "impulse at least 6 ATR", 1)
	if flip {
		cfg = strings.Replace(cfg, "  wait 0 candles after zone", "  zone flip\n  wait 0 candles after zone", 1)
	}
	cfg = strings.ReplaceAll(cfg, "1h", tf)
	fixture := RunFixture{Schema: runFixtureSchema, Case: "retest-boundary", StrategyID: "retest-boundary", Symbol: "XAUUSD", Timeframe: tf, SourceTimeframe: "4h", RangeMethod: "zone", Bars: chart, SourceBars: source}
	result, err := RunFixtureCase(fixture, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return result, boundary
}

func TestRetestEntryBarMustBeginAfterZoneFormationOrFlip(t *testing.T) {
	for _, flip := range []bool{false, true} {
		for _, mirror := range []bool{false, true} {
			name := "formation-long"
			if flip {
				name = "flip-short"
			}
			if mirror {
				name += "-mirrored"
			}
			t.Run(name, func(t *testing.T) {
				result, boundary := retestBoundaryFixture(t, flip, mirror, false)
				for _, tr := range result.Trades {
					t.Logf("entry=%d created=%v base=%v flipped=%v side=%v", tr.EntryIndex, tr.Meta["zoneCreatedAt"], tr.Meta["baseEnd"], tr.Meta["flipped"], tr.Side)
					if tr.EntryIndex == boundary && tr.Meta["zoneCreatedAt"] == boundary/4 {
						t.Fatalf("zone formation/flip and entry share close %v; touch uses the pre-formation bar range", float64(boundary+1+12)*hourMS)
					}
				}
			})
		}
	}
}

func TestRetestEntryBarOpenAtFormationOrFlipCloseIsAllowed(t *testing.T) {
	for _, flip := range []bool{false, true} {
		for _, mirror := range []bool{false, true} {
			name := "formation-long"
			if flip {
				name = "flip-short"
			}
			if mirror {
				name += "-mirrored"
			}
			t.Run(name, func(t *testing.T) {
				result, boundary := retestBoundaryFixture(t, flip, mirror, true)
				want := boundary + 1
				found := false
				for _, tr := range result.Trades {
					if tr.EntryIndex == want {
						found = true
						if tr.Meta["zoneCreatedAt"] != boundary/4 {
							t.Fatalf("created=%v, want %d", tr.Meta["zoneCreatedAt"], boundary/4)
						}
					}
					if tr.EntryIndex == boundary {
						t.Fatalf("same-close entry %d", boundary)
					}
				}
				if !found {
					t.Fatalf("entry candle opening at formation/flip close must be eligible; want entry %d, got %+v", want, result.Trades)
				}
			})
		}
	}
}

func TestRetestEntryBoundaryOnOtherAdmittedTimeframes(t *testing.T) {
	for _, tf := range []string{"30m", "15m"} {
		for _, flip := range []bool{false, true} {
			for _, mirror := range []bool{false, true} {
				for _, later := range []bool{false, true} {
					name := tf + "-formation"
					if flip {
						name = tf + "-flip"
					}
					if mirror {
						name += "-mirrored"
					}
					if later {
						name += "-next-open-equality"
					}
					t.Run(name, func(t *testing.T) {
						result, boundary := retestBoundaryFixture(t, flip, mirror, later, tf)
						created := zoneSource
						if flip {
							created++
						}
						found := false
						for _, tr := range result.Trades {
							if tr.Meta["zoneCreatedAt"] == created && tr.EntryIndex == boundary {
								t.Fatalf("formation/flip bar admitted its pre-formation touch: %+v", tr)
							}
							if tr.Meta["zoneCreatedAt"] == created && tr.EntryIndex == boundary+1 {
								found = true
							}
						}
						if later && !found {
							t.Fatalf("entry open equal to source close not admitted: boundary=%d result=%+v", boundary, result.Trades)
						}
					})
				}
			}
		}
	}
}

// Positive waits retain their decision-close meaning: unlike formation, the
// zone already exists throughout the final waiting candle's touch interval.
func TestRetestPositiveWaitIsCheckedAtDecisionClose(t *testing.T) {
	for _, mirror := range []bool{false, true} {
		name := "long"
		if mirror {
			name = "short"
		}
		t.Run(name, func(t *testing.T) {
			chart, _ := entryRetestBars(-1)
			for i := range chart {
				chart[i].T += 8 * hourMS
			}
			formation := 4*zoneSource + 3
			chart[formation].H, chart[formation].C = 116.1, 116
			beforeMaturity, atMaturity := formation+3, formation+4
			for _, i := range []int{beforeMaturity, atMaturity} {
				chart[i].O, chart[i].H, chart[i].L, chart[i].C = 108, 108.2, 100.1, 101
			}
			if mirror {
				for i, b := range chart {
					chart[i].O, chart[i].H, chart[i].L, chart[i].C = 200-b.O, 200-b.L, 200-b.H, 200-b.C
				}
			}
			source := retestBoundarySource(chart)
			maturityClose := source[zoneSource+1].T + 4*hourMS
			if chart[atMaturity].T >= maturityClose || chart[atMaturity].T+hourMS != maturityClose {
				t.Fatal("fixture must touch during the last waiting candle and decide at its close")
			}
			cfg := strings.Replace(entryRetestSource, "min 0.4 ATR max 3 ATR", "min 0.4 ATR max 30 ATR", 1)
			cfg = strings.Replace(cfg, "impulse at least 2.5 ATR", "impulse at least 6 ATR", 1)
			fixture := RunFixture{Schema: runFixtureSchema, Case: "retest-positive-wait", StrategyID: "retest-positive-wait", Symbol: "XAUUSD", Timeframe: "1h", SourceTimeframe: "4h", RangeMethod: "zone", Bars: chart, SourceBars: source}
			for _, wait := range []struct {
				phrase string
				want   int
			}{
				{"wait 0 candles after zone", beforeMaturity},
				{"wait 1 candle after zone", atMaturity},
			} {
				result, err := RunFixtureCase(fixture, strings.Replace(cfg, "wait 0 candles after zone", wait.phrase, 1))
				if err != nil {
					t.Fatal(err)
				}
				if len(result.Trades) == 0 || result.Trades[0].EntryIndex != wait.want || result.Trades[0].Meta["zoneCreatedAt"] != zoneSource {
					t.Fatalf("%s: want first entry %d from zone %d, got %+v", wait.phrase, wait.want, zoneSource, result.Trades)
				}
			}
		})
	}
}

func entryRetestRequest(t *testing.T) RunRequest {
	t.Helper()
	parsed, err := dsl.Parse(entryRetestSource)
	if err != nil || len(parsed.Errors) > 0 {
		t.Fatalf("parse err=%v errors=%v", err, parsed.Errors)
	}
	chart, source := entryRetestBars(-1)
	return RunRequest{Config: parsed.Config, Series: marketdata.SeriesFromBars(chart), SourceSeries: marketdata.SeriesFromBars(source), Symbol: "XAUUSD", Timeframe: "1h", SourceTimeframe: "4h", StrategyID: "entry-retest-admission"}
}
func TestRetestJSONRoundTripPreservesExecution(t *testing.T) {
	req := entryRetestRequest(t)
	want, err := Run(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(want.Trades) != 1 {
		t.Fatalf("control=%+v", want.Trades)
	}
	raw, err := json.Marshal(req.Config)
	if err != nil {
		t.Fatal(err)
	}
	var decoded dsl.Config
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	req.Config = decoded
	got, err := Run(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("native trades=%d entry=%d; JSON trades=%d; retest type=%T; sourceEntryRequest=%v", len(want.Trades), want.Trades[0].EntryIndex, len(got.Trades), mapValue(decoded, "supplyDemand")["retestOnEntryTimeframe"], sourceEntryRequest(req))
	if !reflect.DeepEqual(want.Trades, got.Trades) {
		t.Fatalf("JSON round-trip changed trades: want=%+v got=%+v", want.Trades, got.Trades)
	}
}
func TestRetestRejectsUnsupportedDirectRoutes(t *testing.T) {
	for _, route := range []struct{ source, entry string }{{"1h", "5m"}, {"15m", "1m"}, {"1h", "15m"}, {"4h", "current"}} {
		t.Run(route.source+"-"+route.entry, func(t *testing.T) {
			req := entryRetestRequest(t)
			req.Config["sourceTimeframe"] = route.source
			req.SourceTimeframe = route.source
			req.Config["entryTf"] = route.entry
			req.Timeframe = route.entry
			req.Config["slices"] = []any{map[string]any{"symbol": "XAUUSD", "tf": route.entry}}
			prepared, err := PrepareRun(req)
			if err == nil {
				t.Fatalf("unsupported retest route admitted: c5=%v retest=%v", prepared.c5, prepared.params.SupplyDemand.RetestOnEntryTimeframe)
			}
		})
	}
}
func TestRetestRejectsCrossFamilyConfig(t *testing.T) {
	req := entryRetestRequest(t)
	req.Config["setupType"] = "breakRetest"
	got, err := Run(req)
	if err == nil {
		t.Fatalf("non-supplyDemand config admitted retest mode, trades=%+v", got.Trades)
	}
}

func TestRetestExecutionWindowControls(t *testing.T) {
	for _, tc := range []struct {
		name, fill                    string
		from, to, wantEntry, wantExit int
		wantReason                    string
		wantTrades                    int
	}{
		{"zone-warms-before-start", "close", 157, 159, 157, 158, "tp", 1},
		{"end-at-entry-close", "close", 157, 158, 157, 157, ReasonEndOfTest, 1},
		{"excluded-signal", "close", 158, 160, 0, 0, "", 0},
		{"next-open", "open", 157, 160, 158, 158, "tp", 1},
		{"drop-prewindow-pending", "open", 158, 160, 0, 0, "", 0},
		{"no-next-open-outside-end", "open", 157, 158, 0, 0, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := entryRetestRequest(t)
			from, to := int64(req.Series.T[tc.from]), int64(req.Series.T[tc.to])
			req.ExecutionWindow = &ExecutionWindow{TradeFromT: &from, TradeToT: &to}
			req.Costs.FillOn = tc.fill
			result, err := Run(req)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Trades) != tc.wantTrades {
				t.Fatalf("trades=%+v", result.Trades)
			}
			if tc.wantTrades == 1 {
				tr := result.Trades[0]
				if tr.EntryIndex != tc.wantEntry || tr.ExitIndex != tc.wantExit || tr.Reason != tc.wantReason {
					t.Fatalf("trade=%+v", tr)
				}
			}
		})
	}
}
func TestRetestPreparedReplayControls(t *testing.T) {
	req := entryRetestRequest(t)
	prepared, err := PrepareRun(req)
	if err != nil {
		t.Fatal(err)
	}
	var first []byte
	for i := 0; i < 3; i++ {
		result, err := prepared.RunChecked(req.Costs)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(result)
		if i == 0 {
			first = raw
		} else if string(first) != string(raw) {
			t.Fatalf("replay %d changed output", i)
		}
	}
}
func TestRetestFormingSourceAndGapControls(t *testing.T) {
	for _, gap := range []bool{false, true} {
		req := entryRetestRequest(t)
		if gap {
			s := req.SourceSeries
			s.T = append(s.T[:38:38], s.T[39:]...)
			s.O = append(s.O[:38:38], s.O[39:]...)
			s.H = append(s.H[:38:38], s.H[39:]...)
			s.L = append(s.L[:38:38], s.L[39:]...)
			s.C = append(s.C[:38:38], s.C[39:]...)
			s.V = append(s.V[:38:38], s.V[39:]...)
			req.SourceSeries = s
		} else {
			req.SourceSeries.H[39] += 40
			req.SourceSeries.L[39] -= 40
			req.SourceSeries.C[39] += 20
		}
		result, err := Run(req)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, tr := range result.Trades {
			if tr.EntryIndex == 157 {
				found = true
			}
		}
		if found == gap {
			t.Fatalf("gap=%v trade157=%v trades=%+v", gap, found, result.Trades)
		}
	}
}
func TestRetestSharedVariantMustFailClosed(t *testing.T) {
	req := entryRetestRequest(t)
	valid := req.Config
	base := entryRetestRequest(t).Config
	base["entryTf"] = "current"
	delete(mapValue(base, "supplyDemand"), "retestOnEntryTimeframe")
	req.Config = base
	shared, err := PrepareSharedRunContext(req)
	if err != nil {
		t.Fatal(err)
	}
	variant, err := shared.PrepareVariant(valid)
	if err == nil {
		result, runErr := variant.RunChecked(req.Costs)
		t.Fatalf("retest admitted into unsupported shared variant: c5=%v trades=%d runErr=%v", variant.c5, len(result.Trades), runErr)
	}
}
func TestRetestDuplicateTypeMustFailClosed(t *testing.T) {
	source := strings.Replace(entryRetestSource, "  retest on entry timeframe", "  retest on entry timeframe\n  type: break retest", 1)
	parsed, err := dsl.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Errors) == 0 {
		req := entryRetestRequest(t)
		req.Config = parsed.Config
		result, runErr := Run(req)
		t.Fatalf("duplicate family DSL accepted: setupType=%v retest=%v trades=%+v err=%v", parsed.Config["setupType"], mapValue(parsed.Config, "supplyDemand")["retestOnEntryTimeframe"], result.Trades, runErr)
	}
}

func TestRetestManagementOrderingMatchesOrdinaryBroker(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		trail, partial, breakeven, stopFirst bool
	}{
		{name: "intrabar-stop-before-management", trail: true, partial: true, breakeven: true, stopFirst: true},
		{name: "trail-before-partial", trail: true, partial: true, breakeven: true},
		{name: "partial-before-breakeven-and-time", partial: true, breakeven: true},
		{name: "max-hold", breakeven: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := entryRetestRequest(t)
			p := paramsFromConfig(req.Config)
			p.MaxHoldBars = 1
			p.BreakevenR = 0
			p.Partial = partialParams{}
			p.Trail = trailParams{}
			if tc.trail {
				p.Trail = trailParams{ATR: 2, TriggerR: 0.1}
			}
			if tc.partial {
				p.Partial = partialParams{Enabled: true, Fraction: 0.5, TriggerR: 0.25, MoveBreakeven: true}
			}
			if tc.breakeven {
				p.BreakevenR = 0.3
				p.BreakevenOffsetATR = 0
			}
			bars := []marketdata.Bar{{T: 0, O: 100, H: 104.5, L: 99, C: 104, V: 1}, {T: hourMS, O: 104, H: 105, L: 103, C: 104.5, V: 1}, {T: 2 * hourMS, O: 104.5, H: 106, L: 103, C: 105, V: 1}}
			if tc.stopFirst {
				bars[0].L = 89
			}
			series := marketdata.SeriesFromBars(bars)
			makeBroker := func() *broker {
				cols := contextcols.Build(series, contextcols.Options{})
				for i := range cols.ATR {
					cols.ATR[i] = 1
				}
				b := new(broker)
				b.reset(series, cols, nil, nil, nil, p, RunFixture{}, nil)
				b.position = position{Side: sideLong, Entry: 100, Size: 10, SL: 90, TP: 120, InitialSL: 90, InitialTP: 120, EntryIndex: 0, EntryT: 0, Meta: TradeMeta{"setup": "supplyDemand"}}
				b.hasPosition = true
				return b
			}
			ordinary, custom := makeBroker(), makeBroker()
			want := ordinary.run()
			r := sdEntryRetestRun{src: &broker{}, idx: []int{-1, -1, -1}, done: -1}
			got := r.run(custom)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("custom=%+v ordinary=%+v", got, want)
			}
			if len(got) == 0 {
				t.Fatal("control must close at least one trade")
			}
			if tc.stopFirst && (len(got) != 1 || got[0].Reason != "sl") {
				t.Fatalf("stop must precede all management: %+v", got)
			}
		})
	}
}

func TestRetestFlagRepresentationsAndMalformedValues(t *testing.T) {
	for _, value := range []any{true, 1, int64(1), float64(1), -2, int64(2), float64(-0.5)} {
		req := entryRetestRequest(t)
		mapValue(req.Config, "supplyDemand")["retestOnEntryTimeframe"] = value
		result, err := Run(req)
		if err != nil || len(result.Trades) != 1 || result.Trades[0].EntryIndex != retestBar {
			t.Fatalf("enabled %T(%v): trades=%+v err=%v", value, value, result.Trades, err)
		}
	}
	for _, value := range []any{"1", "false", nil, []any{1}, map[string]any{}, math.NaN(), math.Inf(1), math.Inf(-1)} {
		req := entryRetestRequest(t)
		mapValue(req.Config, "supplyDemand")["retestOnEntryTimeframe"] = value
		if _, err := Run(req); err == nil {
			t.Fatalf("malformed flag %T(%v) admitted", value, value)
		}
		if _, err := SharedContextKey(req); err == nil {
			t.Fatalf("malformed shared key %T(%v) admitted", value, value)
		}
		if _, err := PrepareSharedRunContext(req); err == nil {
			t.Fatalf("malformed shared context %T(%v) admitted", value, value)
		}
	}
}

func TestRetestDeclaredAndActualRouteMustAgree(t *testing.T) {
	for _, forced := range []bool{false, true} {
		req := entryRetestRequest(t)
		req.Timeframe, req.ForceRoute = "30m", forced
		if _, err := Run(req); err == nil {
			t.Fatalf("mismatched chart admitted, ForceRoute=%v", forced)
		}
		fixture := RunFixture{Schema: runFixtureSchema, Case: "retest-mismatched-chart", Symbol: req.Symbol, Timeframe: req.Timeframe, SourceTimeframe: req.SourceTimeframe, Bars: barsFromSeriesForRetest(req.Series), SourceBars: barsFromSeriesForRetest(req.SourceSeries)}
		if _, err := RunFixtureCase(fixture, entryRetestSource); err == nil {
			t.Fatal("mismatched fixture chart admitted")
		}
	}
}

func TestRetestDisabledPreservesLegacyRouteAdmission(t *testing.T) {
	for _, route := range supportedSourceEntryRoutes {
		for _, value := range []any{false, 0, int64(0), float64(0)} {
			req := entryRetestRequest(t)
			mapValue(req.Config, "supplyDemand")["retestOnEntryTimeframe"] = value
			req.Config["sourceTimeframe"], req.Config["entryTf"] = route.source, route.entry
			req.SourceTimeframe, req.Timeframe = route.source, route.entry
			req.Config["slices"] = []any{map[string]any{"symbol": "XAUUSD", "tf": route.entry}}
			prepared, err := PrepareRun(req)
			if err != nil || !prepared.c5 || prepared.params.SupplyDemand.RetestOnEntryTimeframe {
				t.Fatalf("disabled legacy %s -> %s, %T: prepared=%+v err=%v", route.source, route.entry, value, prepared, err)
			}
		}
	}
}

func barsFromSeriesForRetest(s marketdata.Series) []marketdata.Bar {
	bars := make([]marketdata.Bar, s.Len())
	for i := range bars {
		bars[i] = marketdata.Bar{T: s.T[i], O: s.O[i], H: s.H[i], L: s.L[i], C: s.C[i], V: s.V[i]}
	}
	return bars
}

func TestRetestContradictionPreservesClosedFamilyRefusals(t *testing.T) {
	sequential := seqFullAdmissionRequest(t)
	sequential.Config["supplyDemand"] = map[string]any{"retestOnEntryTimeframe": 1}
	if _, err := Run(sequential); err == nil {
		t.Fatal("Sequential executed with a contradictory retest flag")
	} else {
		var typed *dsl.SequentialFullConfigError
		if !errors.As(err, &typed) {
			t.Fatalf("Sequential typed refusal lost: %T %v", err, err)
		}
	}
	_, _, timed := timedFixture(t, false)
	timed.Config["supplyDemand"] = map[string]any{"retestOnEntryTimeframe": 1}
	if _, err := Run(timed); err == nil {
		t.Fatal("timed return executed with a contradictory retest flag")
	}
}
