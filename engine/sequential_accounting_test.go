package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func accountingSource(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "conformance", path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func accountingFixture(t *testing.T, name, id string) RunFixture {
	t.Helper()
	fixture, err := LoadRunFixture(filepath.Join("..", "conformance", "run", name+".fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	fixture.StrategyID, fixture.HigherTimeframe = id, ""
	return fixture
}

func TestSequentialAccountingFrozenSourcesCaptureIdentity(t *testing.T) {
	cases := []struct{ fixture, id, source, profile, policy string }{
		{"family-legacy-setup9", "dslSequentialLegacySetup9", "parse/setup-legacy-setup9.strat", dsl.LegacySetup9Profile, ""},
		{"family-legacy-setup9-fall", "dslSequentialLegacySetup9", "parse/setup-legacy-setup9.strat", dsl.LegacySetup9Profile, ""},
		{"family-legacy-setup9-perf-seasonal-long", "dslSequentialLegacySetup9PerfSeasonal", "parse/setup-legacy-setup9-perf-seasonal.strat", dsl.LegacySetup9PerfSeasonalProfile, ""},
		{"family-legacy-setup9-perf-seasonal-short", "dslSequentialLegacySetup9PerfSeasonal", "parse/setup-legacy-setup9-perf-seasonal.strat", dsl.LegacySetup9PerfSeasonalProfile, ""},
		{"family-sequential-full-e1-long", "dslSequentialFullE1", "run/family-sequential-full-e1-long.strat", dsl.SequentialFullProfileID, "E1"},
		{"family-sequential-full-e1-short", "dslSequentialFullE1", "run/family-sequential-full-e1-long.strat", dsl.SequentialFullProfileID, "E1"},
		{"family-sequential-full-e2-long", "dslSequentialFullE2", "run/family-sequential-full-e2-long.strat", dsl.SequentialFullProfileID, "E2"},
		{"family-sequential-full-e2-short", "dslSequentialFullE2", "run/family-sequential-full-e2-long.strat", dsl.SequentialFullProfileID, "E2"},
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			fixture, source := accountingFixture(t, c.fixture, c.id), accountingSource(t, c.source)
			fixture.Costs.FeePerUnit, fixture.Costs.Slippage, fixture.Costs.SlippageBps = .03125, .0625, 1.25
			off, err := RunFixtureCase(fixture, source)
			if err != nil {
				t.Fatal(err)
			}
			on, accounting, err := RunSequentialFixtureWithAccounting(fixture, source)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(off)
			after, _ := json.Marshal(on)
			if !bytes.Equal(before, after) {
				t.Fatalf("capture changed retained trades/audit: off=%s on=%s", before, after)
			}
			if on.TradeCount == 0 || len(accounting.Trades) != on.TradeCount || len(accounting.Marks) != len(fixture.Bars) || len(accounting.Events) != 2*on.TradeCount {
				t.Fatalf("counts: run=%d accounting=%+v", on.TradeCount, accounting)
			}
			if accounting.Profile != c.profile || accounting.Policy != c.policy {
				t.Fatalf("wrong source identity: %+v", accounting)
			}
			for i, mark := range accounting.Marks {
				if mark.Index != i || mark.T != fixture.Bars[i].T || mark.Equity != float64(float64(accounting.StartEquity+mark.Realized)+mark.Unrealized) {
					t.Fatalf("mark %d: %+v", i, mark)
				}
			}
			for i, trade := range accounting.Trades {
				if trade.TradeIndex != i || trade.EntryIndex != on.Trades[i].EntryIndex || trade.ExitIndex != on.Trades[i].ExitIndex || trade.Side != on.Trades[i].Side || trade.NetPnL != float64(trade.ExitCredit-trade.EntryFee) {
					t.Fatalf("trade %d: %+v", i, trade)
				}
				if numberForJSON(trade.ExitCredit) != on.Trades[i].PnL || numberForJSON(trade.Size) != on.Trades[i].Size {
					t.Fatalf("raw broker trade does not match retained serialization: %+v", trade)
				}
			}
			if accounting.EndEquity != accounting.Marks[len(accounting.Marks)-1].Equity || accounting.EndEquity != float64(accounting.StartEquity+accounting.FinalRealized) {
				t.Fatal("terminal equity differs from last mark")
			}
			if _, err := json.Marshal(accounting); err != nil {
				t.Fatal(err)
			}
			// A later generic run remains independent, including its audit.
			again, err := RunFixtureCase(fixture, source)
			last, _ := json.Marshal(again)
			if err != nil || !bytes.Equal(before, last) {
				t.Fatalf("capture leaked into later generic run: %v", err)
			}
		})
	}
}

func accountingBroker(costs Costs, closes ...float64) (*broker, *sequentialAccountingCollector) {
	bars := make([]marketdata.Bar, len(closes))
	for i, close := range closes {
		bars[i] = marketdata.Bar{T: float64(i), O: close, H: close, L: close, C: close, V: 1}
	}
	b := &broker{}
	b.reset(marketdata.SeriesFromBars(bars), contextcols.Columns{}, nil, nil, nil, flagParams{}, RunFixture{Costs: costs}, nil)
	capture := newSequentialAccountingCollector(b.costs.StartEquity)
	b.sequentialAccounting = capture
	return b, capture
}

func TestSequentialAccountingLongShortFeesAndMarks(t *testing.T) {
	for _, direction := range []side{sideLong, sideShort} {
		t.Run(direction.String(), func(t *testing.T) {
			close := 100 + float64(direction)*2
			b, c := accountingBroker(Costs{StartEquity: 100, FeePerUnit: .25, Slippage: .25}, 100, close)
			b.openPosition(direction, 100, order{Size: 2, HasSize: true}, 0)
			c.mark(b, 0, false)
			if c.data.Marks[0].Realized != -.5 || c.data.Marks[0].Unrealized != -.5 || c.data.Marks[0].Equity != 99 {
				t.Fatalf("slipped entry mark: %+v", c.data.Marks[0])
			}
			b.closePosition(close, 1, "test", "")
			c.mark(b, 1, false)
			got, err := c.finish(2, 1)
			if err != nil || got.FinalRealized != 2 || got.EndEquity != 102 || got.Trades[0].ExitCredit != 2.5 || got.Trades[0].NetPnL != 2 || got.Trades[0].EntryFee != .5 || got.Trades[0].ExitFee != .5 {
				t.Fatalf("accounting: %+v %v", got, err)
			}
			if got.Events[0].Kind != "entry" || got.Events[0].RealizedBefore != 0 || got.Events[0].RealizedAfter != -.5 || got.Events[1].Kind != "exit" || got.Events[1].RealizedBefore != -.5 || got.Events[1].RealizedAfter != 2 {
				t.Fatalf("mutation chain: %+v", got.Events)
			}
		})
	}
}

func TestSequentialAccountingSameBarReentryAndClassifications(t *testing.T) {
	b, c := accountingBroker(Costs{StartEquity: 100, FeePerUnit: 1}, 100)
	// Two closes and a reentry on one original bar; no extra equity samples.
	b.openPosition(sideLong, 100, order{Size: 1, HasSize: true}, 0)
	b.closePosition(101.5, 0, "test", "") // Retained PnL +.5, all-fee net -.5.
	b.openPosition(sideShort, 100, order{Size: 1, HasSize: true}, 0)
	b.closePosition(98, 0, "test", "") // All-fee net exactly zero.
	b.openPosition(sideLong, 100, order{Size: 0, HasSize: true}, 0)
	b.closePosition(100, 0, "test", "") // Explicit zero size is not default size.
	c.mark(b, 0, false)
	got, err := c.finish(1, 3)
	if err != nil || len(got.Events) != 6 || len(got.Marks) != 1 || got.EndEquity != 99.5 || got.Trades[0].ExitCredit != .5 || got.Trades[0].NetPnL != -.5 || got.Trades[1].NetPnL != 0 || got.Trades[2].Size != 0 || got.Trades[2].NetPnL != 0 {
		t.Fatalf("same-bar accounting: %+v %v", got, err)
	}
	for i, trade := range got.Trades {
		if trade.TradeIndex != i || trade.ExitIndex-trade.EntryIndex != 0 || got.Events[2*i].Index != 0 || got.Events[2*i+1].Index != 0 {
			t.Fatalf("same-bar identity: %+v", trade)
		}
	}
}

func TestSequentialAccountingFeeOnlyAndTinyNetAfterLargeHistory(t *testing.T) {
	b, c := accountingBroker(Costs{StartEquity: 100, FeePerUnit: .125}, 10)
	b.openPosition(sideLong, 10, order{}, 0)
	b.closePosition(10, 0, "test", "")
	c.mark(b, 0, false)
	got, err := c.finish(1, 1)
	if err != nil || got.Trades[0].NetPnL != -.25 || got.FinalRealized != -.25 {
		t.Fatalf("fee-only: %+v %v", got, err)
	}
	b, c = accountingBroker(Costs{StartEquity: 100}, 1, 1)
	b.openPosition(sideLong, 0, order{}, 0)
	b.closePosition(1e16, 0, "test", "")
	c.mark(b, 0, false)
	b.openPosition(sideLong, 1, order{}, 1)
	b.closePosition(math.Nextafter(1, 2), 1, "test", "")
	c.mark(b, 1, false)
	got, err = c.finish(2, 2)
	if err != nil || got.Trades[1].NetPnL != math.Nextafter(1, 2)-1 || got.Trades[1].NetPnL <= 0 || got.FinalRealized != 1e16 || got.Events[3].RealizedAfter != got.Events[3].RealizedBefore {
		t.Fatalf("tiny trade must survive rounded-away account mutation: %+v %v", got, err)
	}
}

func TestSequentialAccountingLegacyLastBarEntryReplacesFinalMark(t *testing.T) {
	source := accountingSource(t, "parse/setup-legacy-setup9.strat")
	fixture := RunFixture{Schema: runFixtureSchema, Case: "invented-accounting", RangeMethod: "zone", StrategyID: "dslSequentialLegacySetup9", Symbol: "XAUUSD", Timeframe: "1h", Costs: Costs{StartEquity: 100, FeePerUnit: .125, Slippage: .25}}
	fixture.Bars = rowsToBars(traceBars(ramp(13, 100, -1)))
	result, accounting, err := RunSequentialFixtureWithAccounting(fixture, source)
	if err != nil || result.TradeCount != 1 {
		t.Fatalf("last-bar entry: %+v %v", result, err)
	}
	trade := accounting.Trades[0]
	last := accounting.Marks[len(accounting.Marks)-1]
	if trade.EntryIndex != 12 || trade.ExitIndex != 12 || result.Trades[0].Reason != ReasonEndOfTest || len(accounting.Marks) != 13 || last.Unrealized != 0 || last.Realized != accounting.FinalRealized || last.Equity != accounting.EndEquity {
		t.Fatalf("last mark was not replaced: trade=%+v mark=%+v", trade, last)
	}
}

func TestSequentialAccountingFullRefusalsAndRecovery(t *testing.T) {
	source := accountingSource(t, "run/family-sequential-full-e1-long.strat")
	fixture := accountingFixture(t, "family-sequential-full-e1-long", "dslSequentialFullE1")
	fixture.Bars, fixture.RawBars = fixture.Bars[:15], nil
	result, accounting, err := RunSequentialFixtureWithAccounting(fixture, source)
	var full *SequentialFullExecutionError
	if !errors.As(err, &full) || full.Kind != "unsupported-incomplete-terminal-run" || !reflect.DeepEqual(result, RunResult{}) || !reflect.DeepEqual(accounting, SequentialAccounting{}) {
		t.Fatalf("partial terminal exposure escaped: %+v %+v %v", result, accounting, err)
	}
	fixture.Bars = append([]marketdata.Bar(nil), fixture.Bars...)
	fixture.Bars[10].T += 1
	result, accounting, err = RunSequentialFixtureWithAccounting(fixture, source)
	if !errors.As(err, &full) || full.Kind != "unsupported-sequential-time-gap" || !reflect.DeepEqual(result, RunResult{}) || !reflect.DeepEqual(accounting, SequentialAccounting{}) {
		t.Fatalf("gap accepted: %+v %+v %v", result, accounting, err)
	}
	fixture = accountingFixture(t, "family-sequential-full-e1-long", "dslSequentialFullE1")
	fixture.Bars, fixture.RawBars = fixture.Bars[:14], nil
	result, accounting, err = RunSequentialFixtureWithAccounting(fixture, source)
	if err != nil || result.TradeCount != 0 || len(result.SequentialFull.Opportunities) != 1 || result.SequentialFull.Opportunities[0].Reason != SequentialFullNoNextBar || len(accounting.Marks) != 14 || accounting.EndEquity != accounting.StartEquity {
		t.Fatalf("pending-only recovery: %+v %+v %v", result, accounting, err)
	}
}

func TestSequentialAccountingAdmissionAndBrokerReset(t *testing.T) {
	source := accountingSource(t, "parse/setup-legacy-setup9.strat")
	base := RunFixture{Schema: runFixtureSchema, Case: "invented-accounting", RangeMethod: "zone", StrategyID: "dslSequentialLegacySetup9", Symbol: "XAUUSD", Timeframe: "1h", Bars: []marketdata.Bar{{T: 0, O: 1, H: 1, L: 1, C: 1, V: 1}}}
	for _, c := range []struct {
		name   string
		mutate func(*RunFixture)
	}{
		{"unknown identity", func(f *RunFixture) { f.StrategyID = "adaptiveFlag" }},
		{"cross source", func(f *RunFixture) { f.StrategyID = "dslSequentialFullE1" }},
		{"route", func(f *RunFixture) { f.Symbol = "OTHER" }},
		{"source timeframe", func(f *RunFixture) { f.SourceTimeframe = "1h" }},
		{"higher timeframe", func(f *RunFixture) { f.HigherTimeframe = "1d" }},
		{"negative start", func(f *RunFixture) { f.Costs.StartEquity = -1 }},
		{"nan start", func(f *RunFixture) { f.Costs.StartEquity = math.NaN() }},
		{"infinite start", func(f *RunFixture) { f.Costs.StartEquity = math.Inf(1) }},
		{"infinite fee", func(f *RunFixture) { f.Costs.FeePerUnit = math.Inf(1) }},
		{"malformed bars", func(f *RunFixture) { f.RawBars = [][]float64{{0, 1}} }},
	} {
		t.Run(c.name, func(t *testing.T) {
			fixture := base
			c.mutate(&fixture)
			result, accounting, err := RunSequentialFixtureWithAccounting(fixture, source)
			var typed *SequentialAccountingError
			if !errors.As(err, &typed) || !reflect.DeepEqual(result, RunResult{}) || !reflect.DeepEqual(accounting, SequentialAccounting{}) {
				t.Fatalf("admission: %+v %+v %v", result, accounting, err)
			}
		})
	}
	if _, _, err := RunSequentialFixtureWithAccounting(base, source+"\n"); err == nil {
		t.Fatal("changed source admitted")
	}
	_, accounting, err := RunSequentialFixtureWithAccounting(base, source)
	if err != nil || accounting.StartEquity != 10000 || accounting.EndEquity != 10000 {
		t.Fatalf("zero normalization: %+v %v", accounting, err)
	}
	b, old := accountingBroker(Costs{}, 1)
	b.openPosition(sideLong, 1, order{}, 0)
	b.reset(b.series, contextcols.Columns{}, nil, nil, nil, flagParams{}, RunFixture{}, nil)
	if b.sequentialAccounting != nil || b.realized != 0 {
		t.Fatal("broker reset retained accounting or realized state")
	}
	before := len(old.data.Events)
	b.openPosition(sideShort, 1, order{}, 0)
	b.closePosition(1, 0, "test", "")
	if len(old.data.Events) != before {
		t.Fatal("generic run wrote to stale collector")
	}
}

func TestSequentialAccountingFiniteBounds(t *testing.T) {
	for _, start := range []float64{math.SmallestNonzeroFloat64, math.MaxFloat64} {
		b, c := accountingBroker(Costs{StartEquity: start}, 0)
		c.mark(b, 0, false)
		got, err := c.finish(1, 0)
		if err != nil || got.EndEquity != start {
			t.Fatalf("finite no-trade start %.17g: %+v %v", start, got, err)
		}
	}
	b, c := accountingBroker(Costs{StartEquity: math.SmallestNonzeroFloat64}, 0)
	b.openPosition(sideLong, 0, order{}, 0)
	b.closePosition(math.SmallestNonzeroFloat64, 0, "test", "")
	c.mark(b, 0, false)
	got, err := c.finish(1, 1)
	if err != nil || got.Trades[0].NetPnL != math.SmallestNonzeroFloat64 || got.EndEquity != 2*math.SmallestNonzeroFloat64 {
		t.Fatalf("subnormal lost: %+v %v", got, err)
	}
	for _, mode := range []string{"entry fee", "exit credit", "net", "cash", "unrealized", "cash plus unrealized", "broken chain"} {
		t.Run(mode, func(t *testing.T) {
			b, c := accountingBroker(Costs{StartEquity: math.MaxFloat64}, 0)
			switch mode {
			case "entry fee":
				b.costs.FeePerUnit = math.MaxFloat64
				b.openPosition(sideLong, 0, order{Size: 2, HasSize: true}, 0)
			case "exit credit":
				b.openPosition(sideLong, -math.MaxFloat64, order{}, 0)
				b.closePosition(math.MaxFloat64, 0, "test", "")
			case "net":
				b.costs.FeePerUnit = math.MaxFloat64
				b.openPosition(sideLong, 0, order{}, 0)
				b.closePosition(0, 0, "test", "")
			case "cash":
				b.openPosition(sideLong, 0, order{}, 0)
				b.closePosition(math.MaxFloat64, 0, "test", "")
				c.mark(b, 0, false)
			case "unrealized":
				b.openPosition(sideLong, -math.MaxFloat64, order{Size: 2, HasSize: true}, 0)
				c.mark(b, 0, false)
			case "cash plus unrealized":
				b.openPosition(sideLong, -math.MaxFloat64, order{}, 0)
				c.mark(b, 0, false)
			case "broken chain":
				b.realized = 1
				b.openPosition(sideLong, 0, order{}, 0)
			}
			got, err := c.finish(1, len(b.trades))
			var typed *SequentialAccountingError
			if !errors.As(err, &typed) || !reflect.DeepEqual(got, SequentialAccounting{}) {
				t.Fatalf("nonfinite/invalid accounting escaped: %+v %v", got, err)
			}
		})
	}
}

func TestSequentialAccountingPublicFiniteBounds(t *testing.T) {
	source := accountingSource(t, "parse/setup-legacy-setup9.strat")
	fixture := RunFixture{Schema: runFixtureSchema, Case: "invented-accounting", RangeMethod: "zone", StrategyID: "dslSequentialLegacySetup9", Symbol: "XAUUSD", Timeframe: "1h", Bars: []marketdata.Bar{{T: 0, O: 1, H: 1, L: 1, C: 1, V: 1}}}
	for _, start := range []float64{0, math.SmallestNonzeroFloat64, math.MaxFloat64} {
		fixture.Costs.StartEquity = start
		result, accounting, err := RunSequentialFixtureWithAccounting(fixture, source)
		if err != nil || result.TradeCount != 0 || len(accounting.Marks) != 1 || accounting.FinalRealized != 0 || accounting.EndEquity != fixture.Costs.normalized().StartEquity {
			t.Fatalf("no-trade finite start %.17g: %+v %+v %v", start, result, accounting, err)
		}
	}
	fixture.Bars = rowsToBars(traceBars(ramp(13, 100, -1)))
	fixture.Costs = Costs{FeePerUnit: math.MaxFloat64, StartEquity: 100}
	result, accounting, err := RunSequentialFixtureWithAccounting(fixture, source)
	var typed *SequentialAccountingError
	if !errors.As(err, &typed) || !reflect.DeepEqual(result, RunResult{}) || !reflect.DeepEqual(accounting, SequentialAccounting{}) {
		t.Fatalf("public overflow returned partial data or untyped refusal: %+v %+v %v", result, accounting, err)
	}
	// An ordinary finite loss larger than the account is still valid.
	b, c := accountingBroker(Costs{StartEquity: 1, FeePerUnit: 1}, 0)
	b.openPosition(sideLong, 0, order{}, 0)
	b.closePosition(0, 0, "test", "")
	c.mark(b, 0, false)
	got, err := c.finish(1, 1)
	if err != nil || got.EndEquity != -1 || got.FinalRealized != -2 {
		t.Fatalf("negative finite equity was clamped/refused: %+v %v", got, err)
	}
}

func TestSequentialAccountingNearZeroCancellationSigns(t *testing.T) {
	for _, tc := range []struct {
		name string
		exit float64
		sign int
	}{
		{"negative", math.Nextafter(2, 0), -1},
		{"flat", 2, 0},
		{"positive", math.Nextafter(2, 3), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, c := accountingBroker(Costs{StartEquity: 100}, 0)
			b.openPosition(sideLong, 0, order{}, 0)
			b.closePosition(1e16, 0, "history", "")
			b.costs.FeePerUnit = 1
			b.openPosition(sideLong, 0, order{}, 0)
			b.closePosition(tc.exit, 0, "cancellation", "")
			c.mark(b, 0, false)
			got, err := c.finish(1, 2)
			if err != nil {
				t.Fatal(err)
			}
			net, sign := got.Trades[1].NetPnL, 0
			if net < 0 {
				sign = -1
			} else if net > 0 {
				sign = 1
			}
			if net != tc.exit-2 || sign != tc.sign {
				t.Fatalf("classification changed around zero: net=%.17g sign=%d want=%d", net, sign, tc.sign)
			}
		})
	}
}

func TestSequentialAccountingPublicOriginalRowAdmission(t *testing.T) {
	source := accountingSource(t, "parse/setup-legacy-setup9.strat")
	base := RunFixture{Schema: runFixtureSchema, Case: "original-row-admission", RangeMethod: "zone", StrategyID: "dslSequentialLegacySetup9", Symbol: "XAUUSD", Timeframe: "1h", Bars: []marketdata.Bar{{T: 0, O: 1, H: 1, L: 1, C: 1, V: 1}}}
	for _, tc := range []struct {
		name, field string
		mutate      func(*RunFixture)
	}{
		{"missing schema", "schema", func(f *RunFixture) { f.Schema = "" }},
		{"wrong schema", "schema", func(f *RunFixture) { f.Schema = "other" }},
		{"missing case", "case", func(f *RunFixture) { f.Case = "" }},
		{"blank case", "case", func(f *RunFixture) { f.Case = "  " }},
		{"missing range", "rangeMethod", func(f *RunFixture) { f.RangeMethod = "" }},
		{"unsupported range", "rangeMethod", func(f *RunFixture) { f.RangeMethod = "body" }},
		{"empty bars", "bars.count", func(f *RunFixture) { f.Bars = nil }},
		{"oversized bars", "bars.count", func(f *RunFixture) { f.Bars = make([]marketdata.Bar, sequentialAccountingMaxBars+1) }},
		{"negative timestamp", "bars.timestamp", func(f *RunFixture) { f.Bars[0].T = -1 }},
		{"fractional timestamp", "bars.timestamp", func(f *RunFixture) { f.Bars[0].T = .5 }},
		{"unsafe timestamp", "bars.timestamp", func(f *RunFixture) { f.Bars[0].T = 1 << 53 }},
		{"no step headroom", "bars.timestamp", func(f *RunFixture) { f.Bars[0].T = float64(1<<53-1) - 3600000 + 1 }},
		{"duplicate timestamp", "bars.timestamp", func(f *RunFixture) { f.Bars = append(f.Bars, f.Bars[0]) }},
		{"descending timestamp", "bars.timestamp", func(f *RunFixture) {
			f.Bars[0].T = 1
			f.Bars = append(f.Bars, marketdata.Bar{T: 0, O: 1, H: 1, L: 1, C: 1, V: 1})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := base
			fixture.Bars = append([]marketdata.Bar(nil), base.Bars...)
			tc.mutate(&fixture)
			result, accounting, err := RunSequentialFixtureWithAccounting(fixture, source)
			var typed *SequentialAccountingError
			if !errors.As(err, &typed) || typed.Field != tc.field || !reflect.DeepEqual(result, RunResult{}) || !reflect.DeepEqual(accounting, SequentialAccounting{}) {
				t.Fatalf("original row admission: %+v %+v %v", result, accounting, err)
			}
		})
	}
	// Legacy permits monotonic irregular gaps; no Full calendar rule is added.
	for _, second := range []float64{1, 7200001, float64(1<<53-1) - 3600000} {
		fixture := base
		fixture.Bars = append([]marketdata.Bar(nil), base.Bars...)
		fixture.Bars = append(fixture.Bars, marketdata.Bar{T: second, O: 1, H: 1, L: 1, C: 1, V: 1})
		result, accounting, err := RunSequentialFixtureWithAccounting(fixture, source)
		if err != nil || result.TradeCount != 0 || len(accounting.Marks) != 2 || accounting.Marks[1].T != second {
			t.Fatalf("legacy monotonic gap refused: %+v %+v %v", result, accounting, err)
		}
	}
	// The generic entry point keeps its prior empty/default envelope behavior.
	generic := base
	generic.Schema, generic.Case, generic.RangeMethod, generic.Bars = "", "", "", nil
	if _, err := RunFixtureCase(generic, source); err != nil {
		t.Fatalf("generic fixture admission changed: %v", err)
	}
}
