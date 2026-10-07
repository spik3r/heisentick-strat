package master

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"

	fp "github.com/spik3r/heisentick-strat/internal/float64contract"
)

func TestPortableMasterFixedPoliciesAndLegacyIdentity(t *testing.T) {
	for _, mode := range []string{SourceMode, ProtectedMode} {
		for _, spread := range []float64{0, 1} {
			r := Request{Config: config(t, mode), M5: waveM5(500), WarmupFromT: 0, TradeFromT: 40 * M30MS, TradeToT: 500 * M30MS, Costs: costs(spread)}
			before, err := Run(r)
			if err != nil {
				t.Fatal(err)
			}
			portable, err := RunPortableV1(r)
			if err != nil {
				t.Fatal(err)
			}
			after, err := Run(r)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("portable call altered legacy defaults/state", err)
			}
			if before.ArithmeticContract != "" || before.Schema != "strat-master-structural-report-v1" {
				t.Fatal("legacy provenance changed")
			}
			if portable.ArithmeticContract != PortableArithmeticContract || portable.Schema != "strat-master-structural-portable-report-v1" || len(portable.Trades) == 0 {
				t.Fatal("portable provenance/coverage")
			}
			if _, err = json.Marshal(portable); err != nil {
				t.Fatal(err)
			}
		}
	}
}
func TestPortableMasterUnexpectedNonfiniteReturnsNoPartialResult(t *testing.T) {
	r := request(t, 50, SourceMode)
	for i := range r.M5.T {
		r.M5.O[i] = math.MaxFloat64
		r.M5.H[i] = math.MaxFloat64
		r.M5.L[i] = 1
		r.M5.C[i] = math.MaxFloat64
	}
	result, err := RunPortableV1(r)
	var arithmetic *fp.Error
	if !errors.As(err, &arithmetic) || !reflect.DeepEqual(result, Result{}) {
		t.Fatalf("nonfinite partial result: %+v %v", result, err)
	}
}
func TestPortableMasterQuantityAndActivationBoundaries(t *testing.T) {
	for _, direction := range []int{1, -1} {
		s := signal(direction, 100, 2, 90, 101, 99)
		if direction == -1 {
			s.Supertrend = 110
		}
		p, cash, err := openPositionArithmetic(s, NativeBar{Open: 100}, ProtectedMode, costs(0), 10000, true)
		if err != nil || math.Float64bits(p.Quantity) != math.Float64bits(10) || cash != 9995 {
			t.Fatalf("quantity/fee %v %v", cash, err)
		}
		threshold := 104.
		if direction == -1 {
			threshold = 96
		}
		row := row(2, 100, threshold, 99, 100, 2, 90, -direction, 0)
		if direction == -1 {
			row.High = 101
			row.Low = threshold
			row.Supertrend = pointer(110)
		}
		_, err = manageArithmetic(p, row, -direction, ProtectedMode, 0, true)
		if err != nil || !p.Locked {
			t.Fatalf("activation equality d%d %v", direction, err)
		}
	}
	// Preserve the explicit epsilon/floor: a below-step quantity is refused.
	s := signal(1, 20000, 2, 19990, 20001, 19999)
	if _, _, err := openPositionArithmetic(s, NativeBar{Open: 20000}, ProtectedMode, costs(0), 10000, true); err == nil {
		t.Fatal("below-step quantity admitted")
	}
	pct := .08
	if gatedCandidate(1, &pct, &H4Snapshot{Regime: -1}) != 1 {
		t.Fatal("ATR equality refused")
	}
	pct = math.Nextafter(.08, 0)
	if gatedCandidate(1, &pct, &H4Snapshot{Regime: -1}) != 0 {
		t.Fatal("below ATR boundary admitted")
	}
}
func TestPortableMaskedProtectionOverflowIsRejected(t *testing.T) {
	var err error
	func() {
		defer fp.Recover(&err)
		s := signal(1, 100, math.MaxFloat64, 99, 101, 99)
		_, _, _ = openPositionArithmetic(s, NativeBar{Open: 100}, ProtectedMode, costs(0), 10000, true)
	}()
	var arithmetic *fp.Error
	if !errors.As(err, &arithmetic) {
		t.Fatal("min structural distance masked overflowing 2ATR", err)
	}
}

func TestPortableMasterActivationAdjacentAndMarketability(t *testing.T) {
	for _, mode := range []string{SourceMode, ProtectedMode} {
		for _, d := range []int{1, -1} {
			for _, spread := range []float64{0, 1} {
				for _, side := range []int{-1, 0, 1} {
					s := signal(d, 100, 2, 90, 101, 99)
					if d < 0 {
						s.Supertrend = 110
					}
					p, _, err := openPositionArithmetic(s, NativeBar{Open: 100}, mode, costs(spread), 10000, true)
					if err != nil {
						t.Fatal(err)
					}
					anchor := s.Anchor
					if mode == ProtectedMode {
						anchor = p.Entry
					}
					trigger := anchor + float64(d)*4
					extent := trigger
					if side < 0 {
						extent = math.Nextafter(trigger, trigger-float64(d))
					} else if side > 0 {
						extent = math.Nextafter(trigger, trigger+float64(d))
					}
					r := row(2, 100, extent, 99, 100, 2, s.Supertrend, -d, 0)
					if d < 0 {
						r.High = 101
						r.Low = extent
					}
					edit, err := manageArithmetic(p, r, -d, mode, spread, true)
					if err != nil || edit.Locked != (side >= 0) {
						t.Fatalf("activation mode%s d%d spread%g side%d %+v %v", mode, d, spread, side, edit, err)
					}
				}
			}
		}
	}
	// A target exactly at liquidation-reference close is newly marketable; one
	// representable price short of it is not. Both policy modes use the same gate.
	for _, mode := range []string{SourceMode, ProtectedMode} {
		for _, d := range []int{1, -1} {
			for _, side := range []int{-1, 0, 1} {
				s := signal(d, 100, 2, 90, 101, 99)
				if d < 0 {
					s.Supertrend = 110
				}
				p, _, err := openPositionArithmetic(s, NativeBar{Open: 100}, mode, costs(0), 10000, true)
				if err != nil {
					t.Fatal(err)
				}
				target := 100 + float64(d)*8
				close := target
				if side < 0 {
					close = math.Nextafter(target, target-float64(d))
				} else if side > 0 {
					close = math.Nextafter(target, target+float64(d))
				}
				r := row(2, 100, math.Max(101, close), math.Min(99, close), close, 2, s.Supertrend, -d, 0)
				edit, err := manageArithmetic(p, r, -d, mode, 0, true)
				if err != nil || edit.TargetMarketable != (side >= 0) {
					t.Fatalf("marketability%s d%d side%d %v", mode, d, side, err)
				}
			}
		}
	}
}

func TestPortableMasterQuantityEpsilonAndFeeBits(t *testing.T) {
	// equity*0.1/entry is 0.3 minus the specified epsilon at this input; adjacent
	// entry bits straddle the floor threshold. Expected bits are literal values,
	// not computed by calling the implementation under test.
	for _, test := range []struct{ entryBits, quantityBits, feeBits uint64 }{
		{0x40aa0aaaaaab0a1b, 0x3fd3333333333334, 0x3fc3333333333334}, // epsilon boundary, lower adjacent entry
		{0x40aa0aaaaaab0a1c, 0x3fc999999999999a, 0x3fb999999999999a}, // boundary entry
		{0x40aa0aaaaaab0a1d, 0x3fc999999999999a, 0x3fb999999999999a}, // upper adjacent entry
		{0x40aa0aaaaaaaaaaa, 0x3fd3333333333334, 0x3fc3333333333334}, // 3333.333333333333
		{0x40b3880000000000, 0x3fc999999999999a, 0x3fb999999999999a}, // 5000 => .2 units
		{0x40c3880000000000, 0x3fb999999999999a, 0x3fa999999999999a}, // 10000 => .1 units
	} {
		entry := math.Float64frombits(test.entryBits)
		s := signal(1, entry, 2, entry-1, entry+1, entry-1)
		p, _, err := openPositionArithmetic(s, NativeBar{Open: entry}, SourceMode, costs(0), 10000, true)
		if err != nil || math.Float64bits(p.Quantity) != test.quantityBits || math.Float64bits(p.EntryFee) != test.feeBits {
			t.Fatalf("quantity/fee bits entry%x %+v %v", test.entryBits, p, err)
		}
	}
}

func TestPortableSummaryOverflowCannotBeHiddenByMax(t *testing.T) {
	for _, run := range []func(){
		func() {
			out := Result{Costs: costs(0), Trades: []Trade{{Net: math.MaxFloat64}, {Net: math.MaxFloat64}}}
			summarizeArithmetic(&out, 10000, true)
		},
		func() { monthlyArithmetic([]Trade{{Net: math.MaxFloat64}, {Net: math.MaxFloat64}}, true) },
	} {
		var err error
		func() { defer fp.Recover(&err); run() }()
		if err == nil {
			t.Fatal("summary accumulation overflow hidden")
		}
	}
}

func TestPortableDiagnosticThresholdEqualityAndAdjacent(t *testing.T) {
	for _, d := range []int{1, -1} {
		for _, side := range []int{-1, 0, 1} {
			delta := 1e-9
			if side < 0 {
				delta = math.Nextafter(delta, 0)
			} else if side > 0 {
				delta = math.Nextafter(delta, math.Inf(1))
			}
			s := signal(d, 0, 0, 0, 1, 1)
			p, _, err := openPositionArithmetic(s, NativeBar{Open: 1}, SourceMode, costs(0), 10000, true)
			if err != nil {
				t.Fatal(err)
			}
			p.SL = pointer(float64(d) * delta)
			p.TP = pointer(delta)
			r := row(2, 1, 1, 1, 1, 0, 0, -d, 0)
			edit, err := manageArithmetic(p, r, -d, SourceMode, 0, true)
			if err != nil || edit.StopWidened != (side > 0) || edit.TargetChanged != (side > 0) {
				t.Fatalf("diagnostic1e-9 d%d side%d %+v %v", d, side, edit, err)
			}
		}
	}
}

func TestPortableMasterHalfProductWarmupRefusal(t *testing.T) {
	r := request(t, 4, SourceMode)
	r.TradeFromT = M30MS
	for i := range r.M5.T {
		r.M5.O[i] = 1e308
		r.M5.H[i] = 1e308
		r.M5.L[i] = 1e308
		r.M5.C[i] = 1e308
	}
	out, err := RunPortableV1(r)
	var arithmetic *fp.Error
	if !errors.As(err, &arithmetic) || !reflect.DeepEqual(out, Result{}) {
		t.Fatalf("Master warmup masked half product: %v", err)
	}
}
