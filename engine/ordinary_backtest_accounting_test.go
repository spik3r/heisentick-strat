package engine

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"runtime"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// These synthetic direct-broker fixtures exercise financial hooks, not an
// admitted ordinary report endpoint or full contemporaneous state coverage.
type ordinaryFinancialPair struct {
	t        *testing.T
	bare     broker
	observed broker
	capture  *ordinaryBacktestFinancialCapture
}

func ordinaryFinancialBars(prices ...float64) []marketdata.Bar {
	bars := make([]marketdata.Bar, len(prices))
	for i, p := range prices {
		bars[i] = marketdata.Bar{T: float64(i * 60000), O: p, H: p, L: p, C: p, V: 1}
	}
	return bars
}

func ordinaryFinancialNewPair(t *testing.T, bars []marketdata.Bar, costs Costs) *ordinaryFinancialPair {
	t.Helper()
	p := &ordinaryFinancialPair{t: t}
	series := marketdata.SeriesFromBars(bars)
	cols := contextcols.Columns{Regime: make([]int8, len(bars))}
	fixture := RunFixture{Case: "synthetic-financial-hooks", StrategyID: "synthetic", Costs: costs}
	for _, b := range []*broker{&p.bare, &p.observed} {
		b.reset(series, cols, nil, nil, nil, flagParams{TradeWindowUnrestricted: true}, fixture, nil)
	}
	var err error
	p.capture, err = attachOrdinaryBacktestFinancialCapture(&p.observed)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (p *ordinaryFinancialPair) step(f func(*broker)) {
	p.t.Helper()
	f(&p.bare)
	f(&p.observed)
	ordinaryFinancialEqualBits(p.t, reflect.ValueOf(p.bare), reflect.ValueOf(p.observed), "broker")
	// Exercise the unchanged native 15-significant-digit envelope and actual
	// JSON marshaler, in addition to comparing every raw broker float bit.
	a, err := json.Marshal(resultEnvelope(p.bare.fixture, p.bare.trades))
	if err != nil {
		p.t.Fatal(err)
	}
	b, err := json.Marshal(resultEnvelope(p.observed.fixture, p.observed.trades))
	if err != nil || !bytes.Equal(a, b) {
		p.t.Fatalf("native serialization changed: %v\n%s\n%s", err, a, b)
	}
}

func ordinaryFinancialEqualBits(t *testing.T, a, b reflect.Value, path string) {
	t.Helper()
	if a.IsValid() != b.IsValid() {
		t.Fatalf("%s validity differs", path)
	}
	if !a.IsValid() {
		return
	}
	if a.Type() != b.Type() {
		t.Fatalf("%s type differs", path)
	}
	switch a.Kind() {
	case reflect.Float64:
		if math.Float64bits(a.Float()) != math.Float64bits(b.Float()) {
			t.Fatalf("%s bits differ: %016x != %016x", path, math.Float64bits(a.Float()), math.Float64bits(b.Float()))
		}
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			name := a.Type().Field(i).Name
			if a.Type() == reflect.TypeOf(broker{}) && name == "paperAccounting" {
				continue
			}
			ordinaryFinancialEqualBits(t, a.Field(i), b.Field(i), path+"."+name)
		}
	case reflect.Slice, reflect.Array:
		if a.Len() != b.Len() || (a.Kind() == reflect.Slice && a.IsNil() != b.IsNil()) {
			t.Fatalf("%s length or nil differs", path)
		}
		for i := 0; i < a.Len(); i++ {
			ordinaryFinancialEqualBits(t, a.Index(i), b.Index(i), path+"[]")
		}
	case reflect.Map:
		if a.IsNil() != b.IsNil() || a.Len() != b.Len() {
			t.Fatalf("%s map differs", path)
		}
		for _, k := range a.MapKeys() {
			ordinaryFinancialEqualBits(t, a.MapIndex(k), b.MapIndex(k), path+"["+k.String()+"]")
		}
	case reflect.Interface, reflect.Pointer:
		if a.IsNil() != b.IsNil() {
			t.Fatalf("%s nil differs", path)
		}
		if !a.IsNil() {
			ordinaryFinancialEqualBits(t, a.Elem(), b.Elem(), path+"*")
		}
	case reflect.String:
		if a.String() != b.String() {
			t.Fatalf("%s string differs", path)
		}
	case reflect.Bool:
		if a.Bool() != b.Bool() {
			t.Fatalf("%s bool differs", path)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if a.Int() != b.Int() {
			t.Fatalf("%s integer differs", path)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if a.Uint() != b.Uint() {
			t.Fatalf("%s integer differs", path)
		}
	default:
		t.Fatalf("unhandled raw comparison %s: %s", path, a.Kind())
	}
}

func ordinaryFinancialMark(b *broker, index int) {
	if b.paperAccounting != nil {
		b.paperAccounting.mark(b, index)
	}
}

func ordinaryFinancialFinish(t *testing.T, p *ordinaryFinancialPair) OrdinaryBacktestFinancials {
	t.Helper()
	got, err := p.capture.finish()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Marks) != p.observed.series.Len() || !paperSame(got.TerminalRealized, p.observed.realized) ||
		!paperSame(got.LastFlatRealized, p.observed.realized) || got.TerminalUnrealized != 0 || got.OpenEntryFee != 0 {
		t.Fatalf("terminal identity: %+v", got)
	}
	return got
}

func ordinaryFinancialRefused(t *testing.T, x *ordinaryBacktestFinancialCapture) {
	t.Helper()
	got, err := x.finish()
	if err == nil || !reflect.DeepEqual(got, OrdinaryBacktestFinancials{}) {
		t.Fatalf("expected all-or-nothing refusal, got %+v, %v", got, err)
	}
}

func TestOrdinaryFinancialMarketAndNextOpenPreserveNative(t *testing.T) {
	for _, direction := range []side{sideLong, sideShort} {
		for _, fill := range []string{"close", "open", "nextOpen"} {
			t.Run(direction.String()+"/"+fill, func(t *testing.T) {
				p := ordinaryFinancialNewPair(t, ordinaryFinancialBars(100, 105, 110), Costs{StartEquity: 1000, FeePerUnit: .1, Slippage: .3, SlippageBps: 1.3, FillOn: fill})
				p.step(func(b *broker) {
					b.dispatchCaptured(0, order{Side: direction, Size: .7, HasSize: true, NoStop: true, NoTarget: true})
				})
				p.step(func(b *broker) { b.run() })
				got := ordinaryFinancialFinish(t, p)
				entryIndex := 0
				if fill != "close" {
					entryIndex = 1
					if got.Marks[0].Equity != 1000 {
						t.Fatal("pending entry charged before fill")
					}
				}
				if len(got.Events) != 2 || got.Events[0].Index != entryIndex || got.Events[1].TradeIndex != 0 || got.Events[1].Index != 2 {
					t.Fatalf("events: %+v", got.Events)
				}
				if !paperSame(got.Events[0].Price, p.observed.trades[0].Entry) || !paperSame(got.Events[1].ExitCredit, p.observed.trades[0].PnL) {
					t.Fatal("raw entry/credit was rounded")
				}
			})
		}
	}
}

func TestOrdinaryFinancialLimitAndGapPreserveNative(t *testing.T) {
	for _, direction := range []side{sideLong, sideShort} {
		for _, gap := range []bool{false, true} {
			name := direction.String() + "/touch"
			if gap {
				name = direction.String() + "/gap"
			}
			t.Run(name, func(t *testing.T) {
				bars := ordinaryFinancialBars(100, 100, 100)
				limit, want := 99.0, 99.0
				bars[1].L = 98
				if direction == sideShort {
					limit, want, bars[1].H = 101, 101, 102
				}
				if gap {
					want = 95
					if direction == sideShort {
						want = 105
					}
					bars[1].O, bars[1].H, bars[1].L, bars[1].C = want, want, want, want
				}
				p := ordinaryFinancialNewPair(t, bars, Costs{StartEquity: 1000, FeePerUnit: .5, Slippage: 1, FillOn: "close"})
				p.step(func(b *broker) {
					b.dispatchCaptured(0, order{Side: direction, Size: 2, HasSize: true, Limit: limit, HasLimit: true, ExpireBars: 3, NoStop: true, NoTarget: true})
				})
				p.step(func(b *broker) { b.run() })
				got := ordinaryFinancialFinish(t, p)
				if len(got.Events) != 2 || got.Events[0].Index != 1 || got.Events[0].Price != want {
					t.Fatalf("limit fill incorrectly slipped/repriced: %+v", got.Events)
				}
			})
		}
	}
}

func TestOrdinaryFinancialSameBarExitReentry(t *testing.T) {
	p := ordinaryFinancialNewPair(t, ordinaryFinancialBars(100, 105, 101), Costs{StartEquity: 1000, FeePerUnit: .5, FillOn: "close"})
	p.step(func(b *broker) {
		b.openPosition(sideLong, 100, order{Size: 2, HasSize: true, NoStop: true, NoTarget: true}, 0)
		ordinaryFinancialMark(b, 0)
	})
	p.step(func(b *broker) {
		b.closePosition(105, 1, ReasonRule, "synthetic")
		b.openPosition(sideShort, 105, order{Size: 3, HasSize: true, NoStop: true, NoTarget: true}, 1)
		ordinaryFinancialMark(b, 1)
	})
	p.step(func(b *broker) { b.runRangeWithFinalization(2, true) })
	got := ordinaryFinancialFinish(t, p)
	if len(got.Events) != 4 || got.Events[1].Kind != "exit" || got.Events[2].Kind != "entry" ||
		got.Events[1].Index != 1 || got.Events[2].Index != 1 || got.Events[1].Ordinal != 0 || got.Events[2].Ordinal != 1 ||
		got.Events[0].PositionID == got.Events[2].PositionID || !paperSame(got.Events[1].RealizedAfter, got.Events[2].RealizedBefore) ||
		got.EntryFees != 2.5 || got.ExitFees != 2.5 || got.TerminalRealized != 17 {
		t.Fatalf("same-bar chain: %+v", got)
	}
}

func TestOrdinaryFinancialPartialThenFinal(t *testing.T) {
	for _, direction := range []side{sideLong, sideShort} {
		t.Run(direction.String(), func(t *testing.T) {
			prices, stop, target := []float64{100, 105, 110}, 90.0, 120.0
			if direction == sideShort {
				prices, stop, target = []float64{100, 95, 90}, 110, 80
			}
			p := ordinaryFinancialNewPair(t, ordinaryFinancialBars(prices...), Costs{StartEquity: 1000, FeePerUnit: .5, Slippage: .25, FillOn: "close"})
			p.step(func(b *broker) {
				b.openPosition(direction, 100, order{Size: 10, HasSize: true, SL: stop, TP: target}, 0)
				ordinaryFinancialMark(b, 0)
			})
			p.step(func(b *broker) {
				b.params.Partial.Enabled = true
				b.params.Partial.Fraction = .5
				b.params.Partial.TriggerR = .25
				b.applyPartialManagement(1)
				ordinaryFinancialMark(b, 1)
			})
			if p.capture.collector.statistics.Count != 0 {
				t.Fatal("partial completed position")
			}
			p.step(func(b *broker) { b.runRangeWithFinalization(2, true) })
			got := ordinaryFinancialFinish(t, p)
			if len(got.Events) != 3 || got.Events[1].Kind != "partial" || got.Events[2].Kind != "exit" || got.Events[2].Size != 5 ||
				got.Marks[1].Realized != 15 || got.Marks[1].Equity != 1038.75 || got.TerminalRealized != 60 || got.Marks[2].Equity != 1060 ||
				got.EntryFees != 5 || got.ExitFees != 5 {
				t.Fatalf("partial/final fees: %+v", got)
			}
		})
	}
}

func TestOrdinaryFinancialActualFinalRemainderWitness(t *testing.T) {
	p := ordinaryFinancialNewPair(t, ordinaryFinancialBars(100, 105, 110), Costs{StartEquity: 1000, FeePerUnit: .1, FillOn: "close"})
	p.step(func(b *broker) {
		b.openPosition(sideLong, 100, order{Size: 10, HasSize: true, SL: 90, TP: 120}, 0)
		ordinaryFinancialMark(b, 0)
	})
	p.step(func(b *broker) {
		b.params.Partial.Enabled = true
		b.params.Partial.Fraction = .7
		b.params.Partial.TriggerR = .25
		b.applyPartialManagement(1)
		ordinaryFinancialMark(b, 1)
	})
	actualBits := math.Float64bits(p.observed.position.Size)
	if runtime.GOARCH == "arm64" && actualBits != 0x4008000000000001 {
		t.Fatalf("ARM64 remainder: %016x", actualBits)
	}
	if (runtime.GOARCH == "amd64" || runtime.GOARCH == "wasm") && actualBits != 0x4008000000000000 {
		t.Fatalf("separate remainder: %016x", actualBits)
	}
	p.step(func(b *broker) { b.runRangeWithFinalization(2, true) })
	got := ordinaryFinancialFinish(t, p)
	if math.Float64bits(got.Events[2].Size) != actualBits {
		t.Fatal("final remainder witness was reconstructed")
	}
	t.Logf("actual target %s/%s, remainder %016x, partial size %016x", runtime.GOOS, runtime.GOARCH, actualBits, math.Float64bits(got.Events[1].Size))
}

func TestOrdinaryFinancialFinalMarkDiscardsProvisionalPeak(t *testing.T) {
	p := ordinaryFinancialNewPair(t, ordinaryFinancialBars(10, 30), Costs{StartEquity: 100, SlippageBps: 1000, FillOn: "close"})
	p.step(func(b *broker) {
		b.openPosition(sideLong, 10, order{Size: 1, HasSize: true, NoStop: true, NoTarget: true}, 0)
	})
	p.step(func(b *broker) { b.run() })
	if p.capture.collector.account.PeakEquity != 119 || p.capture.collector.account.Equity != 119 || p.observed.trades[0].Exit != 27 {
		t.Fatal("stale-peak witness changed")
	}
	got := ordinaryFinancialFinish(t, p)
	if got.Marks[0].Equity != 99 || got.Marks[1].Equity != 116 || got.Marks[1].Unrealized != 0 || got.TerminalRealized != 16 {
		t.Fatalf("finalized curve: %+v", got)
	}
	// The eventual report reduction sees only [99,116]: peak 116 and max DD 1.
	peak, maxDD := got.StartEquity, float64(0)
	for _, m := range got.Marks {
		if m.Equity > peak {
			peak = m.Equity
		}
		dd := float64(peak - m.Equity)
		if dd > maxDD {
			maxDD = dd
		}
	}
	if peak != 116 || maxDD != 1 {
		t.Fatal("provisional peak survived finalization")
	}
}

func TestOrdinaryFinancialNoTradeAndPendingLastBar(t *testing.T) {
	for _, pending := range []string{"none", "market", "limit"} {
		t.Run(pending, func(t *testing.T) {
			p := ordinaryFinancialNewPair(t, ordinaryFinancialBars(10, 11), Costs{StartEquity: 100, FeePerUnit: .5, FillOn: "nextOpen"})
			p.step(func(b *broker) { ordinaryFinancialMark(b, 0) })
			p.step(func(b *broker) {
				if pending != "none" {
					b.dispatchCaptured(1, order{Side: sideLong, Size: 1, HasSize: true, NoStop: true, NoTarget: true, HasLimit: pending == "limit", Limit: 9, ExpireBars: 5})
				}
				b.runRangeWithFinalization(1, true)
			})
			got := ordinaryFinancialFinish(t, p)
			if len(got.Events) != 0 || len(p.observed.trades) != 0 || got.Marks[0].Equity != 100 || got.Marks[1].Equity != 100 || got.EntryFees != 0 || got.ExitFees != 0 {
				t.Fatalf("fabricated pending fill: %+v", got)
			}
			if pending != "none" && len(p.observed.pendingOrders)+len(p.observed.limitOrders) != 1 {
				t.Fatal("native pending order changed")
			}
		})
	}
}

func TestOrdinaryFinancialQuantityBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name           string
		size, fraction float64
		risk           bool
		fail           bool
	}{
		{"zero", 0, .5, false, true}, {"negative", -1, .5, false, true},
		{"risk-underflow", math.SmallestNonzeroFloat64, .5, true, true},
		{"closed-size-underflow", math.SmallestNonzeroFloat64, .5, false, true},
		{"zero-remainder", math.SmallestNonzeroFloat64, .75, false, true},
		{"small-positive", math.SmallestNonzeroFloat64 * 4, .5, false, false},
		{"fraction-zero", 10, 0, false, false}, {"fraction-one", 10, 1, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := ordinaryFinancialNewPair(t, ordinaryFinancialBars(100, 105, 110), Costs{StartEquity: 1000, FillOn: "close"})
			p.step(func(b *broker) {
				ord := order{Size: tc.size, HasSize: true, SL: 90, TP: 120}
				if tc.risk {
					ord = order{RiskUSD: tc.size, HasRisk: true, SL: -1e308, TP: 120}
				}
				b.openPosition(sideLong, 100, ord, 0)
				ordinaryFinancialMark(b, 0)
			})
			p.step(func(b *broker) {
				b.params.Partial.Enabled = true
				b.params.Partial.Fraction = tc.fraction
				b.params.Partial.TriggerR = .25
				b.applyPartialManagement(1)
				ordinaryFinancialMark(b, 1)
			})
			p.step(func(b *broker) { b.runRangeWithFinalization(2, true) })
			if tc.fail {
				ordinaryFinancialRefused(t, p.capture)
				return
			}
			got := ordinaryFinancialFinish(t, p)
			if tc.fraction == 1 && (len(got.Events) != 2 || got.Events[1].Kind != "exit" || p.observed.trades[0].Partial || p.observed.trades[0].Reason != "partial") {
				t.Fatal("fraction=1 must fully flatten")
			}
			if tc.fraction == 0 && len(got.Events) != 2 {
				t.Fatal("fraction=0 fabricated a partial")
			}
		})
	}
}

func TestOrdinaryFinancialCancellationSensitiveFeesAndSignedZero(t *testing.T) {
	for _, fee := range []float64{0, math.Copysign(0, -1), .1, .3333333333333333, 1e12, math.SmallestNonzeroFloat64} {
		p := ordinaryFinancialNewPair(t, ordinaryFinancialBars(1e8, 1e8+.3), Costs{StartEquity: 1e16, FeePerUnit: fee, Slippage: .1, SlippageBps: .001, FillOn: "close"})
		p.step(func(b *broker) {
			b.openPosition(sideLong, 1e8, order{Size: .7, HasSize: true, NoStop: true, NoTarget: true}, 0)
			b.run()
		})
		got := ordinaryFinancialFinish(t, p)
		if !paperSame(got.Events[0].Fee, float64(fee*.7)) {
			t.Fatal("raw fee bits were lost")
		}
		if math.Signbit(fee) && !math.Signbit(got.Events[0].Fee) {
			t.Fatal("signed zero normalized before report")
		}
	}
}

func TestOrdinaryFinancialSARResetReattachAndReuse(t *testing.T) {
	fixture, source := sarNamedLevel(t)
	for _, directive := range []string{"sar trend bearish", "sar trend bullish"} {
		t.Run(directive, func(t *testing.T) {
			cfg := sarParsed(t, source+"\nfilters { "+directive+" }\n")
			req := RunRequest{Config: cfg, Series: marketdata.SeriesFromBars(fixture.Bars), Symbol: fixture.Symbol, Timeframe: fixture.Timeframe, Costs: Costs{StartEquity: 5000, FeePerUnit: .1, Slippage: .3, FillOn: "close"}}
			prepared, err := PrepareRun(req)
			if err != nil {
				t.Fatal(err)
			}
			baselinePrepared, err := PrepareRun(req)
			if err != nil {
				t.Fatal(err)
			}
			var previous *ordinaryBacktestFinancialCapture
			var prior OrdinaryBacktestFinancials
			for repeat := 0; repeat < 3; repeat++ {
				baseline := baselinePrepared.Run(req.Costs)
				bare := baselinePrepared.broker
				prepared.resetBroker(prepared.trades)
				if prepared.broker.paperAccounting != nil || prepared.broker.parabolicSAR != prepared.parabolicSAR {
					t.Fatal("reset/reattach order")
				}
				capture, err := attachOrdinaryBacktestFinancialCapture(&prepared.broker)
				if err != nil {
					t.Fatal(err)
				}
				prepared.trades = prepared.broker.run()
				ordinaryFinancialEqualBits(t, reflect.ValueOf(bare), reflect.ValueOf(prepared.broker), "SAR broker")
				got, err := capture.finish()
				if err != nil {
					t.Fatal(err)
				}
				native := resultEnvelope(prepared.fixture, prepared.trades)
				a, _ := json.Marshal(baseline)
				b, _ := json.Marshal(native)
				if !bytes.Equal(a, b) {
					t.Fatal("SAR native serialization changed")
				}
				if repeat > 0 && !reflect.DeepEqual(got, prior) {
					t.Fatal("reuse changed capture")
				}
				if previous != nil {
					ordinaryFinancialRefused(t, previous)
				}
				previous, prior = capture, got
			}
		})
	}
}

func ordinaryFinancialCompletedFixture(t *testing.T) *ordinaryFinancialPair {
	t.Helper()
	p := ordinaryFinancialNewPair(t, ordinaryFinancialBars(100, 105, 110), Costs{StartEquity: 1000, FeePerUnit: .5, FillOn: "close"})
	p.step(func(b *broker) {
		b.openPosition(sideLong, 100, order{Size: 2, HasSize: true, NoStop: true, NoTarget: true}, 0)
		b.run()
	})
	return p
}

func TestOrdinaryFinancialRejectsCorruptedCapture(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ordinaryFinancialPair)
	}{
		{"missing-mark", func(p *ordinaryFinancialPair) {
			p.capture.collector.events = append(p.capture.collector.events[:2], p.capture.collector.events[3:]...)
		}},
		{"empty-events", func(p *ordinaryFinancialPair) { p.capture.collector.events = nil }},
		{"duplicate-mark", func(p *ordinaryFinancialPair) { c := p.capture.collector; c.events = append(c.events, c.events[1]) }},
		{"wrong-time", func(p *ordinaryFinancialPair) { p.capture.collector.events[1].T++ }},
		{"wrong-index", func(p *ordinaryFinancialPair) { p.capture.collector.events[1].Index++ }},
		{"wrong-ordinal", func(p *ordinaryFinancialPair) { p.capture.collector.events[1].Ordinal++ }},
		{"duplicate-event-id", func(p *ordinaryFinancialPair) {
			p.capture.collector.events[1].EventID = p.capture.collector.events[0].EventID
		}},
		{"wrong-kind", func(p *ordinaryFinancialPair) { p.capture.collector.events[0].Kind = "unknown" }},
		{"broken-realized-chain", func(p *ordinaryFinancialPair) { p.capture.collector.events[1].RealizedBefore++ }},
		{"missing-effect", func(p *ordinaryFinancialPair) { p.capture.collector.events[0].Effect = nil }},
		{"wrong-fee", func(p *ordinaryFinancialPair) { p.capture.collector.events[0].Effect.Fee++ }},
		{"wrong-side", func(p *ordinaryFinancialPair) { p.capture.collector.events[0].Effect.Side = "flat" }},
		{"wrong-size", func(p *ordinaryFinancialPair) { p.capture.collector.events[0].Effect.Size++ }},
		{"bad-stop", func(p *ordinaryFinancialPair) { v := math.NaN(); p.capture.collector.events[0].Effect.Stop = &v }},
		{"wrong-credit", func(p *ordinaryFinancialPair) { *p.capture.collector.events[4].Effect.ExitCredit++ }},
		{"wrong-final-size", func(p *ordinaryFinancialPair) { p.capture.collector.events[4].Effect.Size++ }},
		{"wrong-net", func(p *ordinaryFinancialPair) { *p.capture.collector.events[4].Effect.CompletedNetPnL++ }},
		{"wrong-position-id", func(p *ordinaryFinancialPair) { p.capture.collector.events[4].PositionID = "other" }},
		{"wrong-mark-cash", func(p *ordinaryFinancialPair) { p.capture.collector.events[1].Account.Cash++ }},
		{"wrong-mark-equity", func(p *ordinaryFinancialPair) { p.capture.collector.events[1].Account.Equity++ }},
		{"wrong-mark-unrealized", func(p *ordinaryFinancialPair) { p.capture.collector.events[1].Account.Unrealized++ }},
		{"wrong-mark-fees", func(p *ordinaryFinancialPair) { p.capture.collector.events[1].Account.EntryFees++ }},
		{"nonfinite-mark", func(p *ordinaryFinancialPair) { p.capture.collector.events[1].Account.Equity = math.Inf(1) }},
		{"nonfinite-event", func(p *ordinaryFinancialPair) { p.capture.collector.events[0].RealizedAfter = math.NaN() }},
		{"nonfinite-terminal", func(p *ordinaryFinancialPair) { p.observed.realized = math.Inf(1) }},
		{"nonfinite-terminal-fees", func(p *ordinaryFinancialPair) { p.capture.collector.account.ExitFees = math.Inf(1) }},
		{"nonfinite-completion", func(p *ordinaryFinancialPair) { *p.capture.collector.events[4].Effect.CompletedNetPnL = math.NaN() }},
		{"nonfinite-terminal-statistics", func(p *ordinaryFinancialPair) { p.capture.collector.statistics.GrossWins = math.Inf(1) }},
		{"wrong-terminal-fees", func(p *ordinaryFinancialPair) { p.capture.collector.account.Fees++ }},
		{"missing-native-exit", func(p *ordinaryFinancialPair) { p.observed.trades = nil }},
		{"wrong-native-credit", func(p *ordinaryFinancialPair) { p.observed.trades[0].PnL++ }},
		{"changed-bars", func(p *ordinaryFinancialPair) { p.observed.series.C[1]++ }},
		{"changed-costs", func(p *ordinaryFinancialPair) { p.observed.costs.FeePerUnit++ }},
		{"detached", func(p *ordinaryFinancialPair) { p.observed.paperAccounting = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := ordinaryFinancialCompletedFixture(t)
			tc.mutate(p)
			ordinaryFinancialRefused(t, p.capture)
		})
	}
}

func TestOrdinaryFinancialMissingFinalizationAndUnsupportedMarks(t *testing.T) {
	p := ordinaryFinancialNewPair(t, ordinaryFinancialBars(100, 105), Costs{StartEquity: 1000, FillOn: "close"})
	p.step(func(b *broker) {
		b.openPosition(sideLong, 100, order{Size: 1, HasSize: true, NoStop: true, NoTarget: true}, 0)
		b.runWithFinalization(false)
	})
	ordinaryFinancialRefused(t, p.capture)
	// A path without per-original-row marks cannot be qualified by final state.
	p = ordinaryFinancialNewPair(t, ordinaryFinancialBars(100, 105), Costs{StartEquity: 1000, FillOn: "close"})
	p.step(func(b *broker) { b.runScheduled(nil, nil) })
	ordinaryFinancialRefused(t, p.capture)
}

func TestOrdinaryFinancialCopiedValuesAndNoCollectorDrawdownAuthority(t *testing.T) {
	p := ordinaryFinancialCompletedFixture(t)
	for i := range p.capture.collector.events {
		a := p.capture.collector.events[i].Account
		if a != nil {
			a.PeakEquity = 777
			a.Drawdown = 666
			a.DrawdownFraction = 555
			a.MaxDrawdown = 444
			a.MaxDrawdownFraction = 333
		}
	}
	got := ordinaryFinancialFinish(t, p)
	original := got.Events[0].Price
	p.capture.collector.events[0].Effect.Price = 999
	p.capture.collector.events[1].Account.Equity = 9999
	if got.Events[0].Price != original || got.Marks[0].Equity == 9999 {
		t.Fatal("financial copy aliases collector")
	}
	got.Events[0].Size = 777
	got.Marks[0].Equity = 777
	if p.capture.collector.events[0].Effect.Size == 777 || p.observed.trades[0].Size == 777 {
		t.Fatal("caller mutation aliases execution")
	}
	ordinaryFinancialRefused(t, p.capture)
}

func TestOrdinaryFinancialAttachmentGuards(t *testing.T) {
	if x, err := attachOrdinaryBacktestFinancialCapture(nil); err == nil || x != nil {
		t.Fatal("nil broker accepted")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*broker)
	}{
		{"already-attached", func(b *broker) { b.paperAccounting = newPaperCollector("other", 100) }},
		{"already-trading", func(b *broker) { b.hasPosition = true }},
		{"nonzero-realized", func(b *broker) { b.realized = 1 }},
		{"negative-start", func(b *broker) { b.costs.StartEquity = -1 }},
		{"zero-start", func(b *broker) { b.costs.StartEquity = 0 }},
		{"nonfinite-cost", func(b *broker) { b.costs.FeePerUnit = math.NaN() }},
		{"negative-cost", func(b *broker) { b.costs.Slippage = -1 }},
		{"bad-fill", func(b *broker) { b.costs.FillOn = "unknown" }},
		{"no-bars", func(b *broker) { b.series.T = nil }},
		{"unequal-columns", func(b *broker) { b.series.C = nil }},
		{"nonfinite-close", func(b *broker) { b.series.C[0] = math.NaN() }},
		{"duplicate-time", func(b *broker) { b.series.T[1] = b.series.T[0] }},
		{"windowed", func(b *broker) { b.windowed = true }},
		{"captured-source", func(b *broker) { b.captureEntries = true }},
		{"pending-order", func(b *broker) { b.pendingOrders = []order{{}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b broker
			b.reset(marketdata.SeriesFromBars(ordinaryFinancialBars(10, 11)), contextcols.Columns{}, nil, nil, nil, flagParams{}, RunFixture{Costs: Costs{StartEquity: 100, FillOn: "close"}}, nil)
			tc.mutate(&b)
			before := b.paperAccounting
			if x, err := attachOrdinaryBacktestFinancialCapture(&b); err == nil || x != nil || b.paperAccounting != before {
				t.Fatal("invalid attachment changed state")
			}
		})
	}
}

func TestOrdinaryFinancialNonfiniteArisesOnlyAtFinalLiquidation(t *testing.T) {
	p := ordinaryFinancialNewPair(t, ordinaryFinancialBars(100, 100), Costs{StartEquity: 1000, Slippage: 10, FillOn: "close"})
	p.step(func(b *broker) {
		// A synthetic no-slip entry makes both close marks finite. Only the
		// genuine ordinary final exit's slippage * size overflows the credit.
		b.openPosition(sideLong, 100, order{Size: 1e308, HasSize: true, NoSlip: true, NoStop: true, NoTarget: true}, 0)
		b.run()
	})
	c := p.capture.collector
	if len(c.events) != 3 || c.events[1].Account.Equity != 1000 || c.events[2].Account.Equity != 1000 ||
		!math.IsInf(p.observed.realized, -1) || c.err == nil {
		t.Fatal("expected finite provisional marks followed by nonfinite terminal credit")
	}
	ordinaryFinancialRefused(t, p.capture)
}
