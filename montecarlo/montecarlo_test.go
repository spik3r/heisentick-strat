package montecarlo

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type fixture struct {
	Input struct {
		PnLs        []float64 `json:"pnls"`
		StartEquity float64   `json:"startEquity"`
		Iters       int       `json:"iters"`
		Seed        uint32    `json:"seed"`
	} `json:"input"`
	Expected Result `json:"expected"`
}

func loadFixtures(t *testing.T) map[string]fixture {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no fixtures under testdata/")
	}
	out := make(map[string]fixture, len(paths))
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var f fixture
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		out[filepath.Base(p)] = f
	}
	return out
}

// Every fixture's expected block was written by the JS oracle
// (testdata/gen/oracle.mjs). The comparison is exact: each statistic is a
// left-fold sum, a ratio or a selected element, computed in the same order
// in both languages on IEEE doubles, so there is no summation-order slack
// to allow for.
func TestRunMatchesJSOracleExactly(t *testing.T) {
	for name, f := range loadFixtures(t) {
		t.Run(name, func(t *testing.T) {
			got, err := Run(Input{
				PnL:         f.Input.PnLs,
				StartEquity: f.Input.StartEquity,
				Method:      Both,
				Iterations:  f.Input.Iters,
				Seed:        f.Input.Seed,
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if !reflect.DeepEqual(got, f.Expected) {
				gotJSON, _ := json.MarshalIndent(got, "", "  ")
				wantJSON, _ := json.MarshalIndent(f.Expected, "", "  ")
				t.Fatalf("result differs from oracle\n got: %s\nwant: %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestSingleMethodReturnsTheSameNumbersAsBoth(t *testing.T) {
	f := loadFixtures(t)["handcrafted-12.json"]
	base := Input{PnL: f.Input.PnLs, StartEquity: f.Input.StartEquity, Iterations: f.Input.Iters, Seed: f.Input.Seed}

	both, err := Run(withMethod(base, Both))
	if err != nil {
		t.Fatal(err)
	}
	perm, err := Run(withMethod(base, Permutation))
	if err != nil {
		t.Fatal(err)
	}
	boot, err := Run(withMethod(base, Bootstrap))
	if err != nil {
		t.Fatal(err)
	}

	if perm.BootstrapMaxDD != nil || perm.BootstrapNet != nil {
		t.Fatalf("permutation returned bootstrap sections: %+v", perm)
	}
	if boot.OrderShuffleMaxDD != nil {
		t.Fatalf("bootstrap returned the permutation section: %+v", boot)
	}
	if !reflect.DeepEqual(perm.OrderShuffleMaxDD, both.OrderShuffleMaxDD) {
		t.Fatalf("permutation section differs: %+v vs %+v", perm.OrderShuffleMaxDD, both.OrderShuffleMaxDD)
	}
	if !reflect.DeepEqual(boot.BootstrapMaxDD, both.BootstrapMaxDD) || !reflect.DeepEqual(boot.BootstrapNet, both.BootstrapNet) {
		t.Fatalf("bootstrap sections differ: %+v %+v vs %+v %+v", boot.BootstrapMaxDD, boot.BootstrapNet, both.BootstrapMaxDD, both.BootstrapNet)
	}
	if !reflect.DeepEqual(perm.Realized, both.Realized) || !reflect.DeepEqual(boot.Realized, both.Realized) {
		t.Fatalf("realized differs across methods")
	}
}

func withMethod(in Input, m Method) Input {
	in.Method = m
	return in
}

func TestEmptyStreamReportsNoTradesWithoutNaN(t *testing.T) {
	got, err := Run(Input{PnL: nil, StartEquity: 10000, Method: Both, Iterations: 10, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	want := Result{Trades: 0, Warning: WarningNoTrades}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"trades":0,"warning":"no trades"}` {
		t.Fatalf("json = %s", raw)
	}
}

func TestRunRejectsBadInput(t *testing.T) {
	cases := map[string]Input{
		"zero iterations":           {PnL: []float64{1}, StartEquity: 10000, Method: Both, Iterations: 0, Seed: 1},
		"negative iterations":       {PnL: []float64{1}, StartEquity: 10000, Method: Both, Iterations: -1, Seed: 1},
		"zero iterations empty pnl": {PnL: nil, StartEquity: 10000, Method: Both, Iterations: 0, Seed: 1},
		"missing method":            {PnL: []float64{1}, StartEquity: 10000, Iterations: 1, Seed: 1},
		"unknown method":            {PnL: []float64{1}, StartEquity: 10000, Method: "shuffle", Iterations: 1, Seed: 1},
		"nan pnl":                   {PnL: []float64{1, math.NaN()}, StartEquity: 10000, Method: Both, Iterations: 1, Seed: 1},
		"infinite pnl":              {PnL: []float64{math.Inf(1)}, StartEquity: 10000, Method: Both, Iterations: 1, Seed: 1},
		"non-finite start equity":   {PnL: []float64{1}, StartEquity: math.NaN(), Method: Both, Iterations: 1, Seed: 1},
		"infinite start equity":     {PnL: []float64{1}, StartEquity: math.Inf(1), Method: Both, Iterations: 1, Seed: 1},
		"zero start equity":         {PnL: []float64{1}, StartEquity: 0, Method: Both, Iterations: 1, Seed: 1},
		"negative start equity":     {PnL: []float64{1}, StartEquity: -1, Method: Both, Iterations: 1, Seed: 1},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Run(in); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	if _, err := Run(cases["zero iterations"]); !errors.Is(err, ErrNoIterations) {
		t.Fatalf("zero iterations: err = %v, want ErrNoIterations", err)
	}
}

// Reference values printed by the script's mulberry32 under Node 26 for
// the first five draws. Seed 4294967295 is the largest value ">>> 0" can
// produce; seed 0 checks the constant-only first step.
func TestMulberry32MatchesJS(t *testing.T) {
	cases := map[uint32][]float64{
		0:          {0.26642920868471265, 0.0003297457005828619, 0.2232720274478197, 0.1462021479383111, 0.46732782293111086},
		12345:      {0.9797282677609473, 0.3067522644996643, 0.484205421525985, 0.817934412509203, 0.5094283693470061},
		4294967295: {0.8964226141106337, 0.189478256739676, 0.7156526781618595, 0.9440599093213677, 0.8452364315744489},
	}
	for seed, want := range cases {
		r := newMulberry32(seed)
		for i, w := range want {
			if got := r.next(); got != w {
				t.Fatalf("seed %d draw %d = %.17g, want %.17g", seed, i, got, w)
			}
		}
	}
}

func TestMaxDrawdownFraction(t *testing.T) {
	cases := []struct {
		name  string
		pnls  []float64
		start float64
		want  float64
	}{
		{"no trades", nil, 10000, 0},
		{"only gains", []float64{10, 20}, 10000, 0},
		{"single loss from start", []float64{-100}, 10000, 0.01},
		{"peak then loss", []float64{1000, -550}, 10000, 550.0 / 11000},
		{"recovery does not erase the worst", []float64{-500, 2000, -100}, 10000, 0.05},
		// Degenerate starts behave as the script does: 0/0 is NaN, which
		// never beats worst; a loss from a zero peak divides by zero.
		{"zero start, flat trade: 0/0 is NaN and is dropped", []float64{0}, 0, 0},
		{"zero start, loss: 5/0 is +Inf as in JS", []float64{-5}, 0, math.Inf(1)},
	}
	for _, c := range cases {
		if got := MaxDrawdownFraction(c.pnls, c.start); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestPctlUsesTheScriptsNearestRank(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	cases := []struct{ q, want float64 }{
		{0, 1}, {.05, 1}, {.5, 6}, {.95, 10}, {.99, 10}, {1, 10},
	}
	for _, c := range cases {
		if got := pctl(sorted, c.q); got != c.want {
			t.Errorf("pctl(q=%v) = %v want %v", c.q, got, c.want)
		}
	}
	if got := pctl([]float64{42}, .5); got != 42 {
		t.Errorf("single element: %v", got)
	}
	if got := pctl(nil, .5); !math.IsNaN(got) {
		t.Errorf("empty: %v, want NaN", got)
	}
}
