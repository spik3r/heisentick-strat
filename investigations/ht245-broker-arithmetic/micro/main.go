// Command micro is the minimal reproducer for HT-245. It evaluates, on invented
// inputs, copies of the engine's money-path expressions exactly as written
// (so the compiler may fuse them), the same expressions with explicit float64()
// splits, and math.FMA references for both fusion orientations. Output is one
// line per case with raw bit patterns; diff the output across targets.
//
// The copies are labelled with their engine source (strat main 4ab0fde). This
// program does NOT call the engine; it shows what the compiler does with those
// expression shapes. Engine-level evidence is the harness.
package main

import (
	"fmt"
	"math"
)

type in struct {
	name                     string
	points, size, fee, realz float64
}

var table = []in{
	{"breakeven", 1.1, 3.3, 1.1, 10000},
	{"near", 1234.5678, 987.654321, 1234.5679, 10000},
	{"large", 98765.4321, 12345.6789, 98765.4320, 1e6},
	{"small", 0.00123, 456.789, 0.00124, 10000},
	{"exact-products", 2, 4, 1, 10000},
}

// closePnl is engine/broker.go closePosition: pnl := points*pos.Size - b.costs.FeePerUnit*pos.Size
//
//go:noinline
func closePnl(points, size, fee float64) float64 { return points*size - fee*size }

//go:noinline
func closePnlSplit(points, size, fee float64) float64 {
	return float64(points*size) - float64(fee*size)
}

// entryDebit is engine/broker.go openPosition: b.realized -= b.costs.FeePerUnit * size
//
//go:noinline
func entryDebit(realized, fee, size float64) float64 {
	realized -= fee * size
	return realized
}

//go:noinline
func entryDebitSplit(realized, fee, size float64) float64 {
	realized -= float64(fee * size)
	return realized
}

// target is engine/family_trend_pullback.go: Target: entry + float64(s)*risk*p.TPBTargetR
//
//go:noinline
func target(entry, s, risk, r float64) float64 { return entry + s*risk*r }

//go:noinline
func targetSplit(entry, s, risk, r float64) float64 { return entry + float64(float64(s*risk)*r) }

// masterExitEquity is engine/master/execute.go (non-portable branch):
// equity += p.Quantity * (gross - r.Costs.FeePerUnitSide)
//
//go:noinline
func masterExitEquity(equity, qty, gross, fee float64) float64 {
	equity += qty * (gross - fee)
	return equity
}

//go:noinline
func masterExitEquitySplit(equity, qty, gross, fee float64) float64 {
	equity += float64(qty * (gross - fee))
	return equity
}

// masterQty is engine/master/run.go (non-portable branch) with the 10% sizing:
// raw := equity * .1 / entry; qty = math.Floor((raw+1e-12)/.1) * .1
//
//go:noinline
func masterQty(equity, entry float64) float64 {
	raw := equity * .1 / entry
	return math.Floor((raw+1e-12)/.1) * .1
}

func b(x float64) string { return fmt.Sprintf("%016x", math.Float64bits(x)) }

func main() {
	// Invented: after this exit, the next entry's quantity depends on one equity ulp.
	const eq, qty, gross, fee, entry = 9573.02, 33.1, -1.4875365455523637, 0.0641, 1904.332166072252
	w, sp := masterExitEquity(eq, qty, gross, fee), masterExitEquitySplit(eq, qty, gross, fee)
	fmt.Printf("master-equity  equity written=%s split=%s  nextQty written=%v split=%v\n", b(w), b(sp), masterQty(w, entry), masterQty(sp, entry))
	for _, c := range table {
		fmt.Printf("%-14s closePnl written=%s split=%s fmaA=%s fmaB=%s\n", c.name,
			b(closePnl(c.points, c.size, c.fee)), b(closePnlSplit(c.points, c.size, c.fee)),
			b(math.FMA(c.points, c.size, -(c.fee*c.size))), b(math.FMA(-c.fee, c.size, c.points*c.size)))
		fmt.Printf("%-14s entryDebit written=%s split=%s fma=%s\n", c.name,
			b(entryDebit(c.realz, c.fee, c.size)), b(entryDebitSplit(c.realz, c.fee, c.size)),
			b(math.FMA(-c.fee, c.size, c.realz)))
		fmt.Printf("%-14s target written=%s split=%s\n", c.name,
			b(target(c.points*1000, 1, c.size, c.fee)), b(targetSplit(c.points*1000, 1, c.size, c.fee)))
	}
}
