package adaptivestudy

import (
	"crypto/sha256"
	"encoding/json"
	"github.com/spik3r/heisentick-strat/engine/regimeadaptive"
	"math"
	"reflect"
	"testing"
)

func TestPortableManualDigest(t *testing.T) {
	bars := []Bar{b(0, 100, 101, 99, 100), b(1, 100, 104, 98, 100)}
	rows := []regimeadaptive.Row{row("TL", 1, 100), row("", 1, 100)}
	payload := []Result{}
	for _, cost := range []string{"GROSS", "BASIC", "HARSH", "SOURCE_FEE"} {
		for _, stress := range []string{"none", "one-tick", "target-first", "skip-entry-gap"} {
			payload = append(payload, fixture(t, bars, rows, cost, stress))
		}
	}
	raw, e := json.Marshal(payload)
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("PORTABLE_SHA256=%x", sha256.Sum256(raw))
}

func TestGapCapitalLossPreservesDebtAndStopsNewSizing(t *testing.T) {
	bars := []Bar{b(0, 100, 101, 99, 100), b(1, 100, 100.1, 99.9, 100), b(4, 1000, 1001, 999, 1000)}
	rows := []regimeadaptive.Row{row("TS", .1, 100), row("", .1, 100), row("TL", 1, 1000)}
	r := fixture(t, bars, rows, "GROSS", "none")
	near(t, r.Terminal.Equity, -80000)
	near(t, r.Metrics.NetReturn, -9)
	near(t, r.Metrics.MaxDrawdown, 9)
	if r.Terminal.Pending != nil || r.Terminal.Open != nil || r.Diagnostics.RejectedQty != 1 || r.Diagnostics.GapStops != 1 {
		t.Fatal("capital loss clipped or new risk admitted")
	}
}
func TestSubstepQuantityAndTrendOnlyRangeRefusal(t *testing.T) {
	bars := []Bar{b(0, 100, 101, 99, 100)}
	rows := []regimeadaptive.Row{row("TL", 50000, 100)}
	r := fixture(t, bars, rows, "GROSS", "none")
	if r.Diagnostics.RejectedQty != 1 || r.Terminal.Pending != nil {
		t.Fatal("substep qty admitted")
	}
	rows[0] = row("RL", 1, 101)
	r, e := runRows(bars, rows, 0, Config{Candidate: "B1", Cost: "GROSS", Stress: "none", StartT: 0, EndT: Duration})
	if e != nil || r.Terminal.Pending != nil {
		t.Fatal("B1 range admitted", e)
	}
}

func b(n int, o, h, l, c float64) Bar {
	return Bar{T: int64(n) * Duration, CloseT: int64(n+1) * Duration, Open: o, High: h, Low: l, Close: c}
}
func row(branch string, atr, basis float64) regimeadaptive.Row {
	s := regimeadaptive.Signals{FlatEligible: true}
	switch branch {
	case "TL":
		s.InitialLong = true
	case "TS":
		s.InitialShort = true
	case "RL":
		s.RangeLong = true
	case "RS":
		s.RangeShort = true
	case "both":
		s.RangeLong = true
		s.RangeShort = true
		s.BothBands = true
	}
	return regimeadaptive.Row{ATR: &atr, BBBasis: &basis, Signals: s}
}
func fixture(t *testing.T, bars []Bar, rows []regimeadaptive.Row, cost, stress string) Result {
	t.Helper()
	r, e := runRows(bars, rows, 0, Config{Candidate: "B0", Cost: cost, Stress: stress, StartT: 0, EndT: bars[len(bars)-1].CloseT})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func near(t *testing.T, a, z float64) {
	t.Helper()
	if math.Abs(a-z) > 1e-8 {
		t.Fatalf("got%.12g want%.12g", a, z)
	}
}
func TestNextOpenFixedSizingAndTerminalCosts(t *testing.T) {
	bars := []Bar{b(0, 100, 101, 99, 100), b(1, 103, 104, 102, 103.2)}
	rows := []regimeadaptive.Row{row("TL", 1, 100), row("", 1, 100)}
	for _, c := range []struct {
		cost string
		fee  float64
	}{{"GROSS", 0}, {"SOURCE_FEE", .25}, {"BASIC", 27.777 * .095}, {"HARSH", 27.777 * .155}} {
		t.Run(c.cost, func(t *testing.T) {
			r := fixture(t, bars, rows, c.cost, "none")
			p := r.Terminal.Open
			if p == nil {
				t.Fatal("missing open")
			}
			near(t, p.Entry, 103)
			near(t, p.Order.Qty, 27.777)
			near(t, p.Stop, 101.2)
			near(t, p.Target, 106.2)
			near(t, p.Order.PlannedRisk, 49.9986)
			near(t, r.Terminal.Balance, 10000-c.fee)
			near(t, r.Terminal.Unrealized, 5.5554)
			near(t, r.Terminal.Equity, 10005.5554-c.fee)
			near(t, r.Terminal.LiquidationEquivalentEquity, r.Terminal.Equity-c.fee)
			if len(r.Trades) != 0 || r.Terminal.Pending != nil {
				t.Fatal("terminal invented close/pending")
			}
		})
	}
}
func TestBothHitEntryBarStopFirstAndNamedSensitivity(t *testing.T) {
	bars := []Bar{b(0, 100, 101, 99, 100), b(1, 100, 104, 98, 100)}
	rows := []regimeadaptive.Row{row("TL", 1, 100), row("", 1, 100)}
	a := fixture(t, bars, rows, "BASIC", "none")
	z := fixture(t, bars, rows, "BASIC", "target-first")
	if len(a.Trades) != 1 || len(z.Trades) != 1 || a.Diagnostics.Collisions != 1 || a.Diagnostics.EntryBarExits != 1 {
		t.Fatal("collision accounting")
	}
	near(t, a.Trades[0].Exit, 98.2)
	near(t, a.Trades[0].Gross, -49.9986)
	near(t, a.Trades[0].Fees, 5.27763)
	near(t, a.Trades[0].Net, -55.27623)
	near(t, a.Terminal.Equity, 9944.72377)
	near(t, z.Trades[0].Gross, 88.8864)
	if a.Trades[0].IntrabarT != nil || a.Trades[0].ExitStage != "intrabar-exit" {
		t.Fatal("fabricated intrabar time")
	}
}
func TestNoAdditionsNoMovingBracket(t *testing.T) {
	bars := []Bar{b(0, 100, 101, 99, 100), b(1, 100, 100.5, 99.5, 100), b(2, 100, 100.5, 99.5, 100), b(3, 100, 101, 98.1, 99)}
	rows := []regimeadaptive.Row{row("TL", 1, 100), row("TL", 10, 100), row("TS", 10, 100), row("", 1, 100)}
	r := fixture(t, bars, rows, "SOURCE_FEE", "none")
	if len(r.Trades) != 1 || r.Diagnostics.Occupied != 2 {
		t.Fatal("added/reversed or moved brackets")
	}
	near(t, r.Trades[0].Position.Stop, 98.2)
	near(t, r.Trades[0].Position.Target, 103.2)
	near(t, r.Trades[0].Net, -50.4986)
	near(t, r.Terminal.Balance, 9949.5014)
}
func TestCarriedGapStopAndOpeningTargetPrecedence(t *testing.T) {
	rows := []regimeadaptive.Row{row("TL", 1, 100), row("", 1, 100), row("", 1, 100)}
	bars := []Bar{b(0, 100, 101, 99, 100), b(1, 100, 101, 99, 100), b(4, 95, 96, 94, 95.5)}
	r := fixture(t, bars, rows, "SOURCE_FEE", "none")
	near(t, r.Trades[0].Exit, 95)
	near(t, r.Trades[0].Net, -139.385)
	if r.Diagnostics.GapStops != 1 || r.Diagnostics.HeldGaps != 1 || r.Diagnostics.MissingIntervals != 2 || r.Diagnostics.HeldGapMS != 2*Duration {
		t.Fatal(r.Diagnostics)
	}
	bars[2] = b(4, 110, 111, 95, 100)
	r = fixture(t, bars, rows, "GROSS", "none")
	near(t, r.Trades[0].Exit, 103.2)
	if r.Trades[0].Reason != "target" || r.Trades[0].ExitStage != "open-gap-exit" {
		t.Fatal("known opening target lost to later unordered stop")
	}
}
func TestProspectiveEntryGapThreshold(t *testing.T) {
	rows := []regimeadaptive.Row{row("TL", 1, 100), row("", 1, 100)}
	for _, n := range []int{2, 3} {
		bars := []Bar{b(0, 100, 101, 99, 100), b(n, 100, 101, 99, 100)}
		r := fixture(t, bars, rows, "GROSS", "skip-entry-gap")
		if r.Diagnostics.PendingEntryGaps != 1 {
			t.Fatal("gap not counted")
		}
		if n == 2 {
			if r.Terminal.Open == nil || r.Diagnostics.SkippedEntryGaps != 0 {
				t.Fatal("30m close-to-fill wrongly skipped")
			}
		} else if r.Terminal.Open != nil || r.Diagnostics.SkippedEntryGaps != 1 {
			t.Fatal("prospective gap filter")
		}
	}
}
func TestOpeningCapAndFeeReservationNoFutureResize(t *testing.T) {
	rows := []regimeadaptive.Row{row("TL", 1, 100), row("", 1, 100)}
	r := fixture(t, []Bar{b(0, 100, 101, 99, 100), b(1, 1000, 1001, 999, 1000)}, rows, "GROSS", "none")
	if r.Diagnostics.RejectedCap != 1 || r.Terminal.Fees != 0 || r.Terminal.Open != nil {
		t.Fatal("future price resized qty")
	}
	rows[0] = row("TL", .1, 100)
	bars := []Bar{b(0, 100, 101, 99, 100), b(1, 100, 100.1, 99.9, 100)}
	r = fixture(t, bars, rows, "BASIC", "none")
	if r.Diagnostics.RejectedCost != 1 || r.Terminal.Fees != 0 {
		t.Fatal("cost reservation")
	}
	r = fixture(t, bars, rows, "GROSS", "none")
	if r.Terminal.Open == nil {
		t.Fatal("gross equality cap refused")
	}
	near(t, r.Terminal.Open.Order.Qty, 100)
}
func TestRangeTargetWrongSideAndBothBand(t *testing.T) {
	bars := []Bar{b(0, 100, 101, 99, 100), b(1, 103, 104, 102, 103)}
	rows := []regimeadaptive.Row{row("RL", 1, 102), row("", 1, 100)}
	r := fixture(t, bars, rows, "SOURCE_FEE", "none")
	if r.Diagnostics.RejectedTarget != 1 || r.Terminal.Fees != 0 {
		t.Fatal("wrong-side range target admitted")
	}
	rows[0] = row("both", 1, 105)
	r = fixture(t, bars, rows, "GROSS", "none")
	if r.Diagnostics.BothBand != 1 || r.Terminal.Open != nil || r.Terminal.Pending != nil {
		t.Fatal("both-band not skipped")
	}
}
func TestShortSymmetryAndSeparateSlippage(t *testing.T) {
	bars := []Bar{b(0, 100, 101, 99, 100), b(1, 100, 102, 96, 100)}
	rows := []regimeadaptive.Row{row("TS", 1, 100), row("", 1, 100)}
	r := fixture(t, bars, rows, "SOURCE_FEE", "none")
	near(t, r.Trades[0].Exit, 101.8)
	near(t, r.Trades[0].Net, -50.4986)
	s := fixture(t, bars, rows, "SOURCE_FEE", "one-tick")
	near(t, s.Trades[0].Position.Entry, 99.99)
	near(t, s.Trades[0].Exit, 101.80)
	near(t, s.Trades[0].Fees, .5)
}
func TestLastClosePendingAndPrefixDeterminism(t *testing.T) {
	bars := []Bar{b(0, 100, 101, 99, 100)}
	rows := []regimeadaptive.Row{row("TL", 1, 100)}
	r := fixture(t, bars, rows, "SOURCE_FEE", "none")
	if r.Terminal.Pending == nil || r.Terminal.Open != nil || r.Terminal.Fees != 0 || len(r.Trades) != 0 {
		t.Fatal("last-close fabricated fill")
	}
	near(t, r.Terminal.Equity, 10000)
	full := fixture(t, append(bars, b(1, 100, 101, 99, 100)), append(rows, row("", 1, 100)), "SOURCE_FEE", "none")
	if !reflect.DeepEqual(r.Equity[0], full.Equity[0]) || !reflect.DeepEqual(r.Events[0], full.Events[0]) {
		t.Fatal("future changed prefix")
	}
	a, _ := json.Marshal(full)
	z, _ := json.Marshal(fixture(t, append(bars, b(1, 100, 101, 99, 100)), append(rows, row("", 1, 100)), "SOURCE_FEE", "none"))
	if string(a) != string(z) {
		t.Fatal("nondeterministic")
	}
}
func TestSelectorEarlierYearsEligibilityAndTie(t *testing.T) {
	end := date(2024, 1, 1)
	base := Result{Schema: "adaptive-study-result-v1", Status: "retrospective-assumed-model-not-Pine-parity", Profile: Profile, DataHash: M30Hash, Config: Config{StartT: date(2021, 4, 1), EndT: end, Cost: "BASIC", Stress: "none"}, Metrics: Metrics{TradeCount: 30, NetReturn: .1, MaxDrawdown: .1}}
	m := map[string]Result{}
	for _, id := range []string{"B0", "B1", "B2"} {
		r := base
		r.Config.Candidate = id
		m[id] = r
	}
	id, e := Select(m, end)
	if e != nil || id != "B0" {
		t.Fatal("tie", id, e)
	}
	r := m["B1"]
	r.Metrics.NetReturn = .2
	r.Metrics.MaxDrawdown = .11
	m["B1"] = r
	id, e = Select(m, end)
	if e != nil || id != "B0" {
		t.Fatal("DD guard")
	}
	r.Metrics.MaxDrawdown = .09
	m["B1"] = r
	id, e = Select(m, end)
	if e != nil || id != "B1" {
		t.Fatal("selection", id, e)
	}
	r.Config.EndT = end + Duration
	m["B1"] = r
	if _, e = Select(m, end); e == nil {
		t.Fatal("future admitted")
	}
}
func TestInputGuardsNoWarmupNoWrongData(t *testing.T) {
	if _, e := decodeM30([]byte("bad")); e == nil {
		t.Fatal("unverified data admitted")
	}
	if _, e := Run(VerifiedM30{bars: []Bar{b(0, 100, 101, 99, 100)}, hash: M30Hash}, Config{Candidate: "B0", Cost: "BASIC", Stress: "none", StartT: 0, EndT: Duration}); e == nil {
		t.Fatal("missing warmup admitted")
	}
}
