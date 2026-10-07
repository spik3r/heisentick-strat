package regime

import (
	"errors"
	"math"
	"testing"

	fp "github.com/spik3r/heisentick-strat/internal/float64contract"
)

func TestPortableIndicatorsHandDerivedBits(t *testing.T) {
	bars := make([]NativeBar, 50)
	for i := range bars {
		bars[i] = NativeBar{BucketT: int64(i) * M30MS, Open: 100, High: 101, Low: 99, Close: 100, Volume: 60, Count: 6, Complete: true}
	}
	rows, err := ReferenceBarIndicatorsPortableV1(bars)
	if err != nil {
		t.Fatal(err)
	}
	if rows[8].ATR10 != nil || rows[9].ATR10 == nil || rows[12].ATR14 != nil || rows[13].ATR14 == nil || rows[9].HMA9 != nil || rows[10].HMA9 == nil || rows[23].HMA21 != nil || rows[24].HMA21 == nil {
		t.Fatal("warmup changed")
	}
	for _, i := range []int{13, 20, 49} {
		if math.Float64bits(*rows[i].ATR14) != 0x4000000000000000 {
			t.Fatal("ATR expected exact2")
		}
	}
	if *rows[19].VolumeMean != 60 || rows[19].VolumeOK || rows[38].Complete40 || !rows[39].Complete40 {
		t.Fatal("volume/readiness")
	}
	// RMA independent recurrence: SMA(2,2)=2; next (2+4)/2=3.
	r := portableRMA([]float64{2, 2, 4, 1}, 2)
	if !math.IsNaN(r[0]) || math.Float64bits(r[1]) != 0x4000000000000000 || math.Float64bits(r[2]) != 0x4008000000000000 || math.Float64bits(r[3]) != 0x4000000000000000 {
		t.Fatal("RMA recurrence")
	}
	w := portableWMA([]float64{2, 4}, 2, 0)
	if math.Float64bits(w[1]) != 0x400aaaaaaaaaaaaa {
		t.Fatalf("WMA bits %016x", math.Float64bits(w[1]))
	}
}
func TestPortableInitializationOverflowCannotHideInWarmup(t *testing.T) {
	bars := []NativeBar{{Open: math.MaxFloat64, High: math.MaxFloat64, Low: 1, Close: math.MaxFloat64, Volume: 1}, {Open: math.MaxFloat64, High: math.MaxFloat64, Low: 1, Close: math.MaxFloat64, Volume: 1}}
	rows, err := ReferenceBarIndicatorsPortableV1(bars)
	var arithmetic *fp.Error
	if rows != nil || !errors.As(err, &arithmetic) {
		t.Fatalf("overflow hidden by unready output: rows%d err%v", len(rows), err)
	}
}
func TestPortableBarrierChronologyAndEvaluatedOperations(t *testing.T) {
	b := NativeBar{Open: 100, High: 102, Low: 98, Close: 100, FirstObservedT: 5, CloseT: 30}
	for _, d := range []int{1, -1} {
		sl, tp := 98., 102.
		if d == -1 {
			sl, tp = 102, 98
		}
		price, reason, when, err := ReferenceBarrierPortableV1(b, sl, tp, d, 0)
		if err != nil || reason != "stop_ambiguous" && d == 1 || reason != "target_ambiguous" && d == -1 || when != 30 || price != 98 {
			t.Fatalf("tie %v %v %v %v", price, reason, when, err)
		}
	}
	b.High = math.Nextafter(102, 100)
	if _, reason, _, err := ReferenceBarrierPortableV1(b, 98, 101, 1, 0); err != nil || reason != "target_ambiguous" {
		t.Fatalf("adjacent nearer-high %s %v", reason, err)
	}
	b.Open = 98
	if price, reason, when, err := ReferenceBarrierPortableV1(b, 98, math.NaN(), 1, 0); err != nil || price != 98 || reason != "stop_gap" || when != 5 {
		t.Fatal("inactive target branch evaluated", err)
	}
	b.Open = math.MaxFloat64
	if _, _, _, err := ReferenceBarrierPortableV1(b, -math.MaxFloat64, math.MaxFloat64, 1, 0); err == nil {
		t.Fatal("directional gap overflow hidden")
	}
}

func TestPortableHMARMAIndependentBits(t *testing.T) {
	// Independently evaluated exact rational operations rounded to binary64 after
	// every operation. Inputs repeat thirteen dyadic values, constructed exactly.
	values := make([]float64, 40)
	for i := range values {
		values[i] = float64(800+(i*7)%13) / 8
	}
	for _, test := range []struct {
		n, index int
		bits     uint64
	}{
		{9, 10, 0x40593f7f0d4629b8}, {9, 11, 0x40594880f2b9d64a}, {9, 39, 0x4059314dbf86a316},
		{21, 24, 0x405938284f51c1e8}, {21, 25, 0x40593cb3627bf934}, {21, 39, 0x40593aa71ecf0436},
		{25, 28, 0x4059324c00e90453}, {25, 29, 0x40592fbad2b768e6}, {25, 39, 0x405937d621391dcf},
	} {
		got := portableHMA(values, test.n)
		if math.Float64bits(got[test.index]) != test.bits {
			t.Fatalf("HMA%d[%d]=%016x want%016x", test.n, test.index, math.Float64bits(got[test.index]), test.bits)
		}
	}
	for _, test := range []struct {
		n          int
		init, next uint64
	}{{10, 0x40592c0000000000, 0x40592b999999999a}, {14, 0x40592c9249249249, 0x40592d6343eb1a1f}} {
		got := portableRMA(values, test.n)
		if math.Float64bits(got[test.n-1]) != test.init || math.Float64bits(got[test.n]) != test.next {
			t.Fatalf("RMA%d init/recurrence bits", test.n)
		}
	}
}

func TestPortableSupertrendEqualityAndAdjacentTransitions(t *testing.T) {
	for _, delta := range []int{-1, 0, 1} {
		bars := make([]NativeBar, 11)
		for i := range bars {
			bars[i] = NativeBar{Open: 100, High: 101, Low: 99, Close: 100, Volume: 1, Complete: true}
		}
		close := 100.
		if delta == 0 {
			close = 106
		} else if delta < 0 {
			close = math.Nextafter(106, 0)
		} else {
			close = math.Nextafter(106, math.Inf(1))
		}
		bars[10].High = close
		bars[10].Low = 100
		bars[10].Close = close
		rows, err := ReferenceBarIndicatorsPortableV1(bars)
		if err != nil {
			t.Fatal(err)
		}
		if rows[9].Supertrend == nil || *rows[9].Supertrend != 106 {
			t.Fatal("independent initial ST106")
		}
		want := 1
		if delta > 0 {
			want = -1
		}
		if rows[10].Regime != want {
			t.Fatalf("ST strict equality delta%d", delta)
		}
	}
	// HMA cross uses strict current sign and inclusive previous zero.
	tiny := math.Float64frombits(1)
	for _, c := range []struct {
		now, previous float64
		want          int
	}{{tiny, 0, 1}, {-tiny, 0, -1}, {0, tiny, 0}, {0, -tiny, 0}, {tiny, tiny, 0}, {-tiny, -tiny, 0}} {
		if got := crossDirection(c.now, c.previous); got != c.want {
			t.Fatalf("cross %+v got%d", c, got)
		}
	}
}

func TestPortableInitializationAndMaskedBandOverflow(t *testing.T) {
	// RMA may not skip a nonfinite ready operand, even before publication.
	for _, values := range [][]float64{{1, math.Inf(1)}, {1, math.NaN()}, {math.MaxFloat64, math.MaxFloat64}} {
		var err error
		func() { defer fp.Recover(&err); portableRMA(values, 10) }()
		if err == nil {
			t.Fatal("RMA initialization masked failure")
		}
	}
	// Prior finite ST does not permit a later raw band/indicator overflow to be
	// hidden by carry-forward. This uses finite bars; rejection precedes JSON.
	bars := make([]NativeBar, 12)
	for i := range bars {
		bars[i] = NativeBar{Open: 100, High: 101, Low: 99, Close: 100, Volume: 1}
	}
	bars[11].High = math.MaxFloat64
	bars[11].Low = math.MaxFloat64 / 2
	bars[11].Open = math.MaxFloat64
	bars[11].Close = math.MaxFloat64
	rows, err := ReferenceBarIndicatorsPortableV1(bars)
	if err == nil || rows != nil {
		t.Fatal("later band/intermediate overflow was masked")
	}
}

func TestPortableHMAHalfProductCheckedBeforeWholeReady(t *testing.T) {
	// Four bars make WMA4 ready inside HMA9; WMA9 is still an intentional NaN.
	// The finite half value's 2*half operation must fail before that sentinel
	// could mask it. All OHLC are coherent and finite, with zero true range.
	for _, n := range []int{4, 5, 8} {
		bars := make([]NativeBar, n)
		for i := range bars {
			bars[i] = NativeBar{Open: 1e308, High: 1e308, Low: 1e308, Close: 1e308, Volume: 1}
		}
		rows, err := ReferenceBarIndicatorsPortableV1(bars)
		var arithmetic *fp.Error
		if !errors.As(err, &arithmetic) || rows != nil {
			t.Fatalf("HMA half product hidden at%d: %v", n, err)
		}
	}
}

func TestPortableCrossCurrentCheckedBeforePreviousReady(t *testing.T) {
	h9, h21 := nanSlice(26), nanSlice(26)
	if portableCrossAt(h9, h21, 23) != 0 {
		t.Fatal("warmup cross")
	}
	h9[24], h21[24] = 1, 0
	if portableCrossAt(h9, h21, 24) != 0 {
		t.Fatal("cross before previous HMA ready")
	}
	h9[24], h21[24] = math.MaxFloat64, -math.MaxFloat64
	var err error
	func() { defer fp.Recover(&err); portableCrossAt(h9, h21, 24) }()
	if err == nil {
		t.Fatal("finite current difference hidden by previous warmup sentinel")
	}
	h9[24], h21[24] = 0, 0
	h9[25], h21[25] = math.Float64frombits(1), 0
	if portableCrossAt(h9, h21, 25) != 1 {
		t.Fatal("first ready cross")
	}
}
