// Package montecarlo estimates how bad a strategy's drawdown could get
// from ordering luck and from sampling luck, given its realised per-trade
// P&L stream.
//
// It mirrors heisentick scripts/strategy/monteCarlo.mjs: the "--order"
// method permutes the realised trades (same set, shuffled order) and the
// "--bootstrap" method samples trades with replacement (same count). Both
// build an equity path from a starting equity and record the maximum
// drawdown as a fraction of the running peak; bootstrap also records the
// terminal net P&L. The result carries the realised net and drawdown, the
// p50/p95/p99/worst drawdown of each method and the p5/p50/p95 bootstrap
// net with the probability that net is at or below zero.
//
// Reproducibility is bit for bit with the script: the PRNG is mulberry32
// with the same uint32 arithmetic, the shuffle is the same Fisher-Yates
// walk with the same index truncation, and the two methods draw from one
// stream per iteration in the script's order (shuffle, then bootstrap).
// Percentiles use the script's nearest-rank rule (round(q*(n-1))). Every
// sum is a left fold in stream order, so IEEE doubles give the same bits.
//
// The JSON fixtures under testdata/ are the contract; they are written by
// testdata/gen/oracle.mjs, which holds the script's functions verbatim.
package montecarlo

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// Method selects which resampling statistics Run returns. The PRNG stream
// is consumed identically whichever method is chosen, so the numbers for a
// method never depend on whether the other was requested.
type Method string

const (
	// Permutation shuffles the realised trades (the script's --order).
	Permutation Method = "permutation"
	// Bootstrap samples trades with replacement (the script's --bootstrap).
	Bootstrap Method = "bootstrap"
	// Both returns the permutation and bootstrap sections, as the script
	// computes them on every run.
	Both Method = "both"
)

// Input is one Monte Carlo request.
type Input struct {
	// PnL is the realised per-trade P&L in chronological (exit) order.
	PnL []float64
	// StartEquity is the equity the paths accumulate from. The script
	// always uses 10000.
	StartEquity float64
	// Method selects the returned sections; it is required.
	Method Method
	// Iterations is the number of resampled paths; it must be positive.
	Iterations int
	// Seed seeds mulberry32. It is required; there is no default and no
	// time-based fallback. The script applies ">>> 0" to its numeric seed,
	// which is ToUint32.
	Seed uint32
}

// Realized is the statistic of the trades in their actual order.
type Realized struct {
	// Net is the sum of the P&L stream.
	Net float64 `json:"net"`
	// MaxDD is the maximum drawdown as a fraction of the running peak.
	MaxDD float64 `json:"maxDD"`
}

// DrawdownQuantiles summarise the sorted per-iteration maximum drawdowns.
type DrawdownQuantiles struct {
	P50   float64 `json:"p50"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
	Worst float64 `json:"worst"`
}

// NetQuantiles summarise the sorted per-iteration bootstrap net P&L.
type NetQuantiles struct {
	P5  float64 `json:"p5"`
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	// ProbNetLeZero is the fraction of iterations whose net was <= 0.
	ProbNetLeZero float64 `json:"probNetLeZero"`
}

// Result is the script's per-strategy JSON block without the strategy id,
// routes and provenance. Sections a Method did not ask for are nil. An
// empty stream yields Trades 0 and Warning "no trades" with every other
// field zero or nil, as the script reports it.
type Result struct {
	Trades            int                `json:"trades"`
	Iterations        int                `json:"iters,omitempty"`
	Seed              uint32             `json:"seed,omitempty"`
	Warning           string             `json:"warning,omitempty"`
	Realized          *Realized          `json:"realized,omitempty"`
	OrderShuffleMaxDD *DrawdownQuantiles `json:"orderShuffleMaxDD,omitempty"`
	BootstrapMaxDD    *DrawdownQuantiles `json:"bootstrapMaxDD,omitempty"`
	BootstrapNet      *NetQuantiles      `json:"bootstrapNet,omitempty"`
}

// WarningNoTrades is Result.Warning for an empty stream.
const WarningNoTrades = "no trades"

// ErrNoIterations is returned when Input.Iterations is not positive.
var ErrNoIterations = errors.New("montecarlo: iterations must be positive")

// Run computes the statistics for in. It returns an error for a
// non-positive iteration count, an unknown method, a start equity that is
// not finite and positive, or a non-finite P&L value. The script never
// sees these (its start equity is the constant 10000) and would carry NaN
// or Inf through into null JSON fields instead.
func Run(in Input) (Result, error) {
	if in.Iterations <= 0 {
		return Result{}, ErrNoIterations
	}
	switch in.Method {
	case Permutation, Bootstrap, Both:
	default:
		return Result{}, fmt.Errorf("montecarlo: unknown method %q", in.Method)
	}
	if !(in.StartEquity > 0) || math.IsInf(in.StartEquity, 0) {
		return Result{}, fmt.Errorf("montecarlo: start equity %v must be finite and positive", in.StartEquity)
	}
	for i, p := range in.PnL {
		if math.IsNaN(p) || math.IsInf(p, 0) {
			return Result{}, fmt.Errorf("montecarlo: pnl[%d] = %v is not finite", i, p)
		}
	}
	n := len(in.PnL)
	if n == 0 {
		return Result{Trades: 0, Warning: WarningNoTrades}, nil
	}

	realizedNet := 0.0
	for _, p := range in.PnL {
		realizedNet += p
	}
	res := Result{
		Trades:     n,
		Iterations: in.Iterations,
		Seed:       in.Seed,
		Realized: &Realized{
			Net:   realizedNet,
			MaxDD: MaxDrawdownFraction(in.PnL, in.StartEquity),
		},
	}

	rnd := newMulberry32(in.Seed)
	buf := make([]float64, n)
	orderDD := make([]float64, 0, in.Iterations)
	bootDD := make([]float64, 0, in.Iterations)
	bootNet := make([]float64, 0, in.Iterations)
	for k := 0; k < in.Iterations; k++ {
		shuffleInto(buf, in.PnL, rnd) // permutation: same trades, new order
		orderDD = append(orderDD, MaxDrawdownFraction(buf, in.StartEquity))
		net := 0.0
		for i := range buf { // bootstrap: sample with replacement
			buf[i] = in.PnL[int(rnd.next()*float64(n))]
			net += buf[i]
		}
		bootDD = append(bootDD, MaxDrawdownFraction(buf, in.StartEquity))
		bootNet = append(bootNet, net)
	}
	sort.Float64s(orderDD)
	sort.Float64s(bootDD)
	sort.Float64s(bootNet)

	if in.Method != Bootstrap {
		res.OrderShuffleMaxDD = drawdownQuantiles(orderDD)
	}
	if in.Method != Permutation {
		res.BootstrapMaxDD = drawdownQuantiles(bootDD)
		leZero := 0
		for _, v := range bootNet {
			if v <= 0 {
				leZero++
			}
		}
		res.BootstrapNet = &NetQuantiles{
			P5:            pctl(bootNet, .05),
			P50:           pctl(bootNet, .5),
			P95:           pctl(bootNet, .95),
			ProbNetLeZero: float64(leZero) / float64(len(bootNet)),
		}
	}
	return res, nil
}

// MaxDrawdownFraction is the maximum drawdown, as a fraction of the running
// peak, of the equity path that accumulates pnls from start. It is the
// script's maxDDfrac: the peak starts at start, and a drawdown counts only
// when it exceeds the worst seen so far, so a path that never falls below
// its peak returns 0.
func MaxDrawdownFraction(pnls []float64, start float64) float64 {
	eq := start
	peak := start
	worst := 0.0
	for _, p := range pnls {
		eq += p
		if eq > peak {
			peak = eq
		}
		dd := (peak - eq) / peak
		if dd > worst {
			worst = dd
		}
	}
	return worst
}

func drawdownQuantiles(sorted []float64) *DrawdownQuantiles {
	return &DrawdownQuantiles{
		P50:   pctl(sorted, .5),
		P95:   pctl(sorted, .95),
		P99:   pctl(sorted, .99),
		Worst: sorted[len(sorted)-1],
	}
}

// shuffleInto copies src into dst and Fisher-Yates shuffles it in place,
// drawing j = trunc(rnd * (i+1)) for i from len-1 down to 1 as the script
// does. It draws len(src)-1 numbers.
func shuffleInto(dst, src []float64, rnd *mulberry32) {
	copy(dst, src)
	for i := len(dst) - 1; i > 0; i-- {
		j := int(rnd.next() * float64(i+1))
		dst[i], dst[j] = dst[j], dst[i]
	}
}

// pctl is the script's nearest-rank percentile: the element at
// round(q*(n-1)) of an ascending slice. math.Round and JS Math.round agree
// for the non-negative arguments used here. It returns NaN for an empty
// slice, as the script does; Run never calls it with one.
func pctl(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	idx := int(math.Round(q * float64(len(sorted)-1)))
	if idx < 0 {
		idx = 0
	}
	if idx > len(sorted)-1 {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// mulberry32 is the script's PRNG. JS runs it on int32 with Math.imul and
// unsigned shifts; every step has the same bit pattern in uint32, and the
// final ">>> 0" then "/ 4294967296" is an exact uint32 to double division.
type mulberry32 struct {
	a uint32
}

func newMulberry32(seed uint32) *mulberry32 {
	return &mulberry32{a: seed}
}

// next returns the next value in [0, 1).
func (r *mulberry32) next() float64 {
	r.a += 0x6D2B79F5
	t := (r.a ^ (r.a >> 15)) * (1 | r.a)
	t = (t + (t^(t>>7))*(61|t)) ^ t
	return float64(t^(t>>14)) / 4294967296
}
