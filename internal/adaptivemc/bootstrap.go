package adaptivemc

import (
	"fmt"
	fp "github.com/spik3r/heisentick-strat/internal/float64contract"
	"math"
	"math/rand"
	"time"
)

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// Group uses ACTUAL mark timestamps. A lone midnight mark defines an observed
// UTC date, never a fabricated session. Daystart is the prior observed equity.
// On first nonpositive equity, preserve that signed factor and stop its tuple;
// actual dayend remains visible. No later recovery silently erases capital loss.
func Group(marks []Mark) (days []Day, err error) {
	defer fp.Recover(&err)
	if len(marks) == 0 {
		return nil, fmt.Errorf("empty marked path")
	}
	prior := 10000.
	lastT := int64(-1)
	for n := 0; n < len(marks); {
		date := time.UnixMilli(marks[n].T).UTC().Format("2006-01-02")
		end := n
		for end < len(marks) && time.UnixMilli(marks[end].T).UTC().Format("2006-01-02") == date {
			end++
		}
		if prior <= 0 {
			return nil, fmt.Errorf("source path already capital-lost; subsequent day normalization unsupported")
		}
		d := Day{Date: date, End: 1, Max: 1, Min: 1}
		peak := 1.
		for i := n; i < end; i++ {
			m := marks[i]
			if m.T < 0 || m.T <= lastT || !finite(m.Equity) {
				return nil, fmt.Errorf("invalid/nonmonotone marked path")
			}
			lastT = m.T
			d.Marks++
			d.Gap = d.Gap || m.Gap
			factor := fp.Div(m.Equity, prior)
			if factor == 0 && m.Equity != 0 {
				return nil, fmt.Errorf("nonzero normalization underflow")
			}
			d.ActualEnd = factor
			if d.CapitalLoss {
				continue
			}
			d.End = factor
			d.Max = math.Max(d.Max, factor)
			d.Min = math.Min(d.Min, factor)
			peak = math.Max(peak, factor)
			d.DD = math.Max(d.DD, fp.Sub(1, fp.Div(factor, peak)))
			if factor <= 0 {
				d.CapitalLoss = true
				d.UnclippedLossFactor = factor
			}
		}
		days = append(days, d)
		prior = marks[end-1].Equity
		n = end
	}
	return days, nil
}

// Generate uses non-circular COMPLETE contiguous blocks. Each sensitivity starts
// its own pinned NewSource(seed); all streams share exactly the same matrix.
func Generate(n, block, reps int, seed int64) (Matrix, error) {
	if n < block || block < 1 || reps < 1 || n > 5000 || reps > Replicates {
		return Matrix{}, fmt.Errorf("invalid bootstrap dimensions")
	}
	rng := rand.New(rand.NewSource(seed))
	m := Matrix{Schema: "adaptive-bootstrap-index-matrix-v1", Days: n, Block: block, Replicates: reps, Seed: seed, Indices: make([][]int, reps)}
	for r := range m.Indices {
		row := make([]int, 0, n)
		for len(row) < n {
			start := rng.Intn(n - block + 1)
			for k := 0; k < block && len(row) < n; k++ {
				row = append(row, start+k)
			}
		}
		m.Indices[r] = row
	}
	return m, nil
}
func Aggregate(days []Day, indices []int) (o Outcome, err error) {
	defer fp.Recover(&err)
	s, p, d := 1., 1., 0.
	for _, i := range indices {
		if i < 0 || i >= len(days) {
			return o, fmt.Errorf("invalid sampled index")
		}
		x := days[i]
		if !finite(x.End) || !finite(x.Max) || !finite(x.Min) || !finite(x.DD) || x.Max < 1 || x.Min > 1 || x.DD < 0 {
			return o, fmt.Errorf("invalid day tuple")
		}
		if x.End > x.Max || x.End < x.Min || x.CapitalLoss && x.End > 0 || x.Min <= 0 && x.End > 0 {
			return o, fmt.Errorf("inconsistent loss/day tuple")
		}
		o.UsedDays++
		if x.CapitalLoss || x.End <= 0 {
			// At first loss use the GLOBAL prior/prefix peak. Negative local DD cannot
			// be merged with max(): a smaller local peak overstates global debt DD.
			p = math.Max(p, fp.Mul(s, x.Max))
			s = fp.Mul(s, x.End)
			if s == 0 && x.End != 0 {
				return o, fmt.Errorf("signed capital underflow; debt must not be erased")
			}
			d = math.Max(d, fp.Sub(1, fp.Div(s, p)))
			o.CapitalLoss = true
			o.UnclippedLossFactor = s
			break
		}
		// Positive-equity recurrence: drawdown BEFORE peak/capital updates.
		low := fp.Mul(s, x.Min)
		if low == 0 && x.Min != 0 {
			return o, fmt.Errorf("nonzero minimum capital underflow")
		}
		d = math.Max(d, math.Max(fp.Sub(1, fp.Div(low, p)), x.DD))
		p = math.Max(p, fp.Mul(s, x.Max))
		s = fp.Mul(s, x.End)
		if s == 0 {
			return o, fmt.Errorf("positive compounding underflow; no capital-loss inference")
		}
		if s < 0 {
			o.CapitalLoss = true
			o.UnclippedLossFactor = s
			break
		}
	}
	o.Return = fp.Sub(s, 1)
	o.DD = d
	return o, nil
}
