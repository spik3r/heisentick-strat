package adaptivemc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	fp "github.com/spik3r/heisentick-strat/internal/float64contract"
	"math"
	"runtime"
	"sort"
)

func MatrixBytes(m Matrix) ([]byte, string, error) {
	b, e := json.Marshal(m)
	if e != nil {
		return nil, "", e
	}
	b = append(b, '\n')
	h := sha256.Sum256(b)
	return b, hex.EncodeToString(h[:]), nil
}
func quantile(x []float64, p float64) float64 {
	v := append([]float64{}, x...)
	sort.Float64s(v)
	return v[int(math.Ceil(p*float64(len(v))))-1]
}
func qs(x []float64) Quantiles { return Quantiles{quantile(x, .05), quantile(x, .5), quantile(x, .95)} }
func frequency(count, n int) Frequency {
	p := fp.Div(float64(count), float64(n))
	z := 1.959963984540054
	z2 := fp.Mul(z, z)
	N := float64(n)
	den := fp.Add(1, fp.Div(z2, N))
	center := fp.Div(fp.Add(p, fp.Div(z2, fp.Mul(2, N))), den)
	variance := fp.Add(fp.Div(fp.Mul(p, fp.Sub(1, p)), N), fp.Div(z2, fp.Mul(4, fp.Mul(N, N))))
	half := fp.Mul(fp.Div(z, den), math.Sqrt(variance))
	return Frequency{Count: count, N: n, Fraction: p, Low: math.Max(0, fp.Sub(center, half)), High: math.Min(1, fp.Add(center, half)), Meaning: "Wilson95% Monte Carlo sampling error only; not future outcome probability or model uncertainty"}
}

func summarize(out []Outcome) Stats {
	r, d := []float64{}, []float64{}
	loss, dd10, dd20, caploss := 0, 0, 0, 0
	for _, o := range out {
		r = append(r, o.Return)
		d = append(d, o.DD)
		if o.Return < 0 {
			loss++
		}
		if o.DD > .1 {
			dd10++
		}
		if o.DD > .2 {
			dd20++
		}
		if o.CapitalLoss {
			caploss++
		}
	}
	return Stats{Return: qs(r), DD: qs(d), Loss: frequency(loss, len(out)), DD10: frequency(dd10, len(out)), DD20: frequency(dd20, len(out)), CapitalLoss: frequency(caploss, len(out))}
}

// Analyze freezes20/10/40,2000,2872026. It reads no market data and never changes
// costs/selection. Each sensitivity's matrix is shared by every candidate/cost.
func Analyze(streams []Stream) (report Report, matrices []Matrix, err error) {
	defer fp.Recover(&err)
	report = Report{Schema: "adaptive-bootstrap-report-v1", Status: "descriptive-retrospective-not-forecast", StudyHead: StudyHead, GoVersion: runtime.Version(), Replicates: Replicates, Seed: Seed, Original: map[string]Outcome{}, Streams: streams, Blocks: []BlockReport{}, Limitations: []string{"conditional descriptive resampling of exposed assumed-model development equity paths; no forecasts/selection/retuning", "UTC dates are observed mark dates, not verified trading sessions; no fictional missing dates", "M30 closed-mark drawdown only, no tick/intrabar risk; funding/provider/unobserved extremes outside model", "Wilson intervals only Monte Carlo sampling error; not strategy/model/population uncertainty", "cost scenarios independently rerun equity-dependent study, never posthoc cost subtraction; no annualization of irregular horizon"}}
	if len(streams) != 12 {
		return report, nil, fmt.Errorf("exact12stream set required")
	}
	n := len(streams[0].Days)
	report.Days = n
	seen := map[string]bool{}
	for _, s := range streams {
		if seen[s.ID] || len(s.Days) != n {
			return report, nil, fmt.Errorf("different/duplicate day membership")
		}
		seen[s.ID] = true
		for i, d := range s.Days {
			if d.Date != streams[0].Days[i].Date {
				return report, nil, fmt.Errorf("different observed dates")
			}
		}
	}
	for _, c := range []string{"B0", "B1", "B2"} {
		for _, cost := range []string{"GROSS", "BASIC", "HARSH", "SOURCE_FEE"} {
			if !seen[c+":"+cost] {
				return report, nil, fmt.Errorf("missing candidate/cost")
			}
		}
	}
	if n < 100 {
		report.Status = "insufficient-history"
		return report, nil, nil
	}
	if n > 5000 {
		return report, nil, fmt.Errorf("too many observed dates")
	}
	original := make([]int, n)
	for i := range original {
		original[i] = i
	}
	for _, s := range streams {
		o, e := Aggregate(s.Days, original)
		if e != nil {
			return report, nil, e
		}
		report.Original[s.ID] = o
	}
	for _, block := range []int{20, 10, 40} {
		m, e := Generate(n, block, Replicates, Seed)
		if e != nil {
			return report, nil, e
		}
		_, hash, e := MatrixBytes(m)
		if e != nil {
			return report, nil, e
		}
		matrices = append(matrices, m)
		br := BlockReport{Block: block, IndexSHA256: hash, Stats: map[string]Stats{}, Paired: []Paired{}}
		all := map[string][]Outcome{}
		for _, s := range streams {
			out := make([]Outcome, Replicates)
			for i, indices := range m.Indices {
				o, e := Aggregate(s.Days, indices)
				if e != nil {
					return report, nil, e
				}
				out[i] = o
			}
			all[s.ID] = out
			br.Stats[s.ID] = summarize(out)
		}
		for _, cost := range []string{"GROSS", "BASIC", "HARSH", "SOURCE_FEE"} {
			base := all["B0:"+cost]
			for _, c := range []string{"B1", "B2"} {
				r, d := make([]float64, Replicates), make([]float64, Replicates)
				for i, o := range all[c+":"+cost] {
					r[i] = fp.Sub(o.Return, base[i].Return)
					d[i] = fp.Sub(o.DD, base[i].DD)
				}
				br.Paired = append(br.Paired, Paired{ID: c + "-B0:" + cost, ReturnDifference: qs(r), DDDifference: qs(d)})
			}
		}
		report.Blocks = append(report.Blocks, br)
	}
	return report, matrices, nil
}
