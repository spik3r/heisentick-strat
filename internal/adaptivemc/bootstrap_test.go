package adaptivemc

import (
	"crypto/sha256"
	"encoding/json"
	"math"
	"math/big"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func near(t *testing.T, a, b float64) {
	t.Helper()
	if !finite(a) || !finite(b) || math.Abs(a-b) > 1e-11 {
		t.Fatalf("got%.17g want%.17g", a, b)
	}
}
func utc(day, hour int) int64 { return time.Date(2021, 1, day, hour, 0, 0, 0, time.UTC).UnixMilli() }
func TestIndependentPositivePathRecurrence(t *testing.T) {
	marks := []Mark{{T: utc(1, 1), Equity: 11000}, {T: utc(1, 2), Equity: 9000}, {T: utc(1, 3), Equity: 10500}, {T: utc(2, 1), Equity: 8400}, {T: utc(2, 2), Equity: 12600}, {T: utc(2, 3), Equity: 11550}}
	days, e := Group(marks)
	if e != nil {
		t.Fatal(e)
	}
	o, e := Aggregate(days, []int{0, 1})
	if e != nil {
		t.Fatal(e)
	}
	near(t, o.Return, .155)
	near(t, o.DD, 13.0/55)
	if o.CapitalLoss {
		t.Fatal("positive path flagged")
	}
}
func TestRationalOracleManyPositivePaths(t *testing.T) {
	rng := rand.New(rand.NewSource(17))
	for trial := 0; trial < 128; trial++ {
		marks := []Mark{}
		peak := big.NewRat(10000, 1)
		dd := new(big.Rat)
		var last *big.Rat
		for day := 1; day <= 12; day++ {
			for h := 1; h <= 4; h++ {
				equity := int64(4000 + rng.Intn(12001))
				x := big.NewRat(equity, 1)
				if x.Cmp(peak) > 0 {
					peak = new(big.Rat).Set(x)
				}
				draw := new(big.Rat).Quo(new(big.Rat).Sub(peak, x), peak)
				if draw.Cmp(dd) > 0 {
					dd = draw
				}
				last = x
				marks = append(marks, Mark{T: utc(day, h), Equity: float64(equity)})
			}
		}
		days, e := Group(marks)
		if e != nil {
			t.Fatal(e)
		}
		indices := []int{}
		for i := range days {
			indices = append(indices, i)
		}
		o, e := Aggregate(days, indices)
		if e != nil {
			t.Fatal(e)
		}
		wantDD, _ := dd.Float64()
		wantReturn, _ := new(big.Rat).Sub(new(big.Rat).Quo(last, big.NewRat(10000, 1)), big.NewRat(1, 1)).Float64()
		near(t, o.DD, wantDD)
		near(t, o.Return, wantReturn)
	}
}
func TestActualUTCDatesFlatGapsAndLoneMidnight(t *testing.T) {
	m := []Mark{{T: utc(1, 23), Equity: 10000}, {T: utc(2, 0), Equity: 10000}, {T: utc(4, 1), Equity: 10100, Gap: true}}
	d, e := Group(m)
	if e != nil || len(d) != 3 {
		t.Fatal(d, e)
	}
	if d[1].Date != "2021-01-02" || d[1].Marks != 1 || d[2].Date != "2021-01-04" || !d[2].Gap {
		t.Fatal("invented/dropped date")
	}
	near(t, d[1].End, 1)
	near(t, d[1].DD, 0)
	near(t, d[2].End, 1.01)
}
func TestCapitalLossGlobalPeakAndUnclippedDebt(t *testing.T) {
	days := []Day{{End: 1, Max: 2, Min: 1, DD: .5}, {End: -.1, Max: 1.2, Min: -.1, DD: 13.0 / 12, CapitalLoss: true, UnclippedLossFactor: -.1}, {End: 100, Max: 100, Min: 1}}
	o, e := Aggregate(days, []int{0, 1, 2})
	if e != nil {
		t.Fatal(e)
	}
	near(t, o.Return, -1.1)
	near(t, o.DD, 1.05)
	near(t, o.UnclippedLossFactor, -.1)
	if !o.CapitalLoss || o.UsedDays != 2 {
		t.Fatal("loss dropped/clipped/recovery simulated")
	}
	d, e := Group([]Mark{{T: utc(1, 1), Equity: 12000}, {T: utc(1, 2), Equity: -1000}, {T: utc(1, 3), Equity: -2000}})
	if e != nil {
		t.Fatal(e)
	}
	near(t, d[0].End, -.1)
	near(t, d[0].ActualEnd, -.2)
	if !d[0].CapitalLoss {
		t.Fatal("source lossflag missing")
	}
	o, e = Aggregate(d, []int{0})
	if e != nil {
		t.Fatal(e)
	}
	near(t, o.DD, 13.0/12)
}
func TestPinnedRNGOracleAndCompleteNonCircularBlocks(t *testing.T) {
	m, e := Generate(6, 2, 3, Seed)
	if e != nil {
		t.Fatal(e)
	}
	want := [][]int{{3, 4, 2, 3, 1, 2}, {1, 2, 3, 4, 3, 4}, {4, 5, 0, 1, 3, 4}}
	if !reflect.DeepEqual(m.Indices, want) {
		t.Fatal(m.Indices)
	}
	for _, block := range []int{10, 20, 40} {
		m, e = Generate(103, block, 20, Seed)
		if e != nil {
			t.Fatal(e)
		}
		for _, row := range m.Indices {
			if len(row) != 103 {
				t.Fatal("length")
			}
			for i, v := range row {
				if v < 0 || v >= 103 {
					t.Fatal("outside")
				}
				if i%block == 0 {
					if v > 103-block {
						t.Fatal("incomplete source block")
					}
				} else if v != row[i-1]+1 {
					t.Fatal("noncontiguous/circular")
				}
			}
		}
		_, h, e := MatrixBytes(m)
		if e != nil {
			t.Fatal(e)
		}
		t.Logf("BLOCK%d_SHA256=%s", block, h)
	}
}
func TestEmpiricalQuantilesAndWilson(t *testing.T) {
	v := make([]float64, 2000)
	for i := range v {
		v[i] = float64(1999 - i)
	}
	q := qs(v)
	if q.P05 != 99 || q.P50 != 999 || q.P95 != 1899 {
		t.Fatal(q)
	}
	f := frequency(1000, 2000)
	near(t, f.Low, .4781079507511692)
	near(t, f.High, .5218920492488308)
	f = frequency(0, 2000)
	near(t, f.Low, 0)
	near(t, f.High, .0019170472812529335)
	f = frequency(2000, 2000)
	near(t, f.Low, .998082952718747)
	near(t, f.High, 1)
}
func TestRefuseNonfiniteAndMalformedPaths(t *testing.T) {
	if _, e := Group([]Mark{{T: utc(1, 1), Equity: math.NaN()}}); e == nil {
		t.Fatal("NaN")
	}
	if _, e := Group([]Mark{{T: utc(1, 1), Equity: 1}, {T: utc(1, 1), Equity: 1}}); e == nil {
		t.Fatal("duplicate time")
	}
	if _, e := Aggregate([]Day{{End: 1, Max: 1, Min: math.Inf(-1)}}, []int{0}); e == nil {
		t.Fatal("Inf")
	}
	if _, e := Generate(10, 20, 2000, Seed); e == nil {
		t.Fatal("incomplete source blocks")
	}
}
func TestInsufficientHistoryNoReplicateOutputs(t *testing.T) {
	streams := []Stream{}
	for _, c := range []string{"B0", "B1", "B2"} {
		for _, cost := range []string{"GROSS", "BASIC", "HARSH", "SOURCE_FEE"} {
			streams = append(streams, Stream{ID: c + ":" + cost, Days: []Day{{Date: "2021-01-01", End: 1, Max: 1, Min: 1}}})
		}
	}
	r, m, e := Analyze(streams)
	if e != nil || r.Status != "insufficient-history" || len(m) != 0 || len(r.Blocks) != 0 {
		t.Fatal("insufficient history admitted", e)
	}
}

func TestSharedMatricesAndPairedIdenticalSyntheticStreams(t *testing.T) {
	days := make([]Day, 100)
	for i := range days {
		end := 1.01
		if i%2 == 0 {
			end = .99
		}
		days[i] = Day{Date: time.Unix(int64(i)*86400, 0).UTC().Format("2006-01-02"), End: end, Max: 1.01, Min: .99, DD: 2.0 / 101}
	}
	streams := []Stream{}
	for _, c := range []string{"B0", "B1", "B2"} {
		for _, cost := range []string{"GROSS", "BASIC", "HARSH", "SOURCE_FEE"} {
			streams = append(streams, Stream{ID: c + ":" + cost, Days: days})
		}
	}
	r, m, e := Analyze(streams)
	if e != nil || len(m) != 3 || r.Replicates != 2000 {
		t.Fatal(e)
	}
	raw, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("SYNTHETIC_REPORT_SHA256=%x", sha256.Sum256(raw))
	for n, b := range r.Blocks {
		_, h, e := MatrixBytes(m[n])
		if e != nil || h != b.IndexSHA256 {
			t.Fatal("matrix hash")
		}
		for _, p := range b.Paired {
			if p.ReturnDifference != (Quantiles{}) || p.DDDifference != (Quantiles{}) {
				t.Fatal("paired indices differ")
			}
		}
	}
}

func TestNonzeroNormalizationAndDebtUnderflowRefused(t *testing.T) {
	for _, equity := range []float64{math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64} {
		if _, e := Group([]Mark{{T: utc(1, 1), Equity: equity}}); e == nil {
			t.Fatal("normalization underflow accepted")
		}
	}
	for _, end := range []float64{math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64} {
		days := []Day{{End: .5, Max: 1, Min: .5, DD: .5}, {End: end, Max: 1, Min: end, DD: 1, CapitalLoss: end < 0}}
		if _, e := Aggregate(days, []int{0, 1}); e == nil {
			t.Fatal("capital underflow accepted")
		}
	}
	days, e := Group([]Mark{{T: utc(1, 1), Equity: 0}})
	if e != nil {
		t.Fatal(e)
	}
	o, e := Aggregate(days, []int{0})
	if e != nil || !o.CapitalLoss || o.Return != -1 || o.UnclippedLossFactor != 0 {
		t.Fatalf("genuine zero absorption: %+v %v", o, e)
	}
}
