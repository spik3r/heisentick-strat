package contextcols

import (
	"math"
	"math/big"

	"github.com/spik3r/heisentick-strat/marketdata"
)

// ComputeATR matches engine/indicators.js: a simple moving average of true
// range, seeded with the partial average before len bars are available.
func ComputeATR(series marketdata.Series, length int) []float64 {
	if length <= 0 {
		length = 14
	}
	n := series.Len()
	tr := make([]float64, n)
	atr := make([]float64, n)
	var sum float64
	for i := 0; i < n; i++ {
		prevC := series.C[i]
		if i > 0 {
			prevC = series.C[i-1]
		}
		value := max(series.H[i]-series.L[i], math.Abs(series.H[i]-prevC), math.Abs(series.L[i]-prevC))
		tr[i] = value
		sum += value
		if i >= length {
			sum -= tr[i-length]
		}
		if i >= length-1 {
			atr[i] = sum / float64(length)
		} else {
			atr[i] = sum / float64(i+1)
		}
	}
	return atr
}

// ComputeER matches engine/engine.js Kaufman efficiency ratio.
func ComputeER(series marketdata.Series, length int) []float64 {
	if length <= 0 {
		length = 10
	}
	n := series.Len()
	er := make([]float64, n)
	for i := length; i < n; i++ {
		net := math.Abs(series.C[i] - series.C[i-length])
		var vol float64
		for j := i - length + 1; j <= i; j++ {
			vol += math.Abs(series.C[j] - series.C[j-1])
		}
		if vol > 0 {
			er[i] = net / vol
		}
	}
	return er
}

// ComputeTrueRangeER uses true range instead of close-to-close movement in
// the efficiency denominator. ComputeER remains the default compatibility
// path for existing strategies.
func ComputeTrueRangeER(series marketdata.Series, length int) []float64 {
	if length <= 0 {
		length = 10
	}
	n := series.Len()
	er := make([]float64, n)
	for i := length; i < n; i++ {
		net := math.Abs(series.C[i] - series.C[i-length])
		var movement float64
		for j := i - length + 1; j <= i; j++ {
			prevClose := series.C[j-1]
			movement += max(series.H[j]-series.L[j], math.Abs(series.H[j]-prevClose), math.Abs(series.L[j]-prevClose))
		}
		if movement > 0 {
			er[i] = net / movement
		}
	}
	return er
}

// ComputeEMA matches engine/indicators.js: close EMA seeded with close[0].
func ComputeEMA(series marketdata.Series, length int) []float64 {
	if length <= 0 {
		length = 1
	}
	n := series.Len()
	out := make([]float64, n)
	if n == 0 {
		return out
	}
	k := 2 / (float64(length) + 1)
	ema := series.C[0]
	out[0] = ema
	for i := 1; i < n; i++ {
		ema += k * (series.C[i] - ema)
		out[i] = ema
	}
	return out
}

// ComputeSlopeFromSeries subtracts the value length bars ago.
func ComputeSlopeFromSeries(values []float64, length int) []float64 {
	if length <= 0 {
		length = 1
	}
	out := nanSlice(len(values))
	for i := length; i < len(values); i++ {
		if isFinite(values[i]) && isFinite(values[i-length]) {
			out[i] = values[i] - values[i-length]
		}
	}
	return out
}

type VolumeAnomalyStats struct {
	SMA20             []float64
	Ratio20           []float64
	Z50               []float64
	SpreadATR         []float64
	BodyATR           []float64
	EffortResultRatio []float64
}

// ComputeVolumeAnomalyStats matches engine/engine.js computeVolumeAnomalyStats.
func ComputeVolumeAnomalyStats(series marketdata.Series, atr []float64, smaLen int, zLen int) VolumeAnomalyStats {
	if smaLen <= 0 {
		smaLen = 20
	}
	if zLen <= 0 {
		zLen = 50
	}
	n := series.Len()
	stats := VolumeAnomalyStats{
		SMA20:             nanSlice(n),
		Ratio20:           nanSlice(n),
		Z50:               nanSlice(n),
		SpreadATR:         nanSlice(n),
		BodyATR:           nanSlice(n),
		EffortResultRatio: nanSlice(n),
	}
	volWindow := make([]float64, 0, smaLen+1)
	zWindow := make([]float64, 0, zLen+1)
	var volSum, zSum float64
	zSumSqJS := new(big.Float).SetPrec(53).SetMode(big.ToNearestEven)
	for i := 0; i < n; i++ {
		if i < len(atr) && isFinite(atr[i]) && atr[i] > 0 {
			stats.SpreadATR[i] = (series.H[i] - series.L[i]) / atr[i]
			stats.BodyATR[i] = math.Abs(series.C[i]-series.O[i]) / atr[i]
		}
		v := series.V[i]
		if !isFinite(v) || v < 0 {
			continue
		}
		if len(volWindow) > 0 {
			sma := volSum / float64(len(volWindow))
			stats.SMA20[i] = sma
			if sma > 0 {
				stats.Ratio20[i] = v / sma
			}
		}
		if len(zWindow) >= 2 {
			mean := zSum / float64(len(zWindow))
			zSumSqRounded, _ := zSumSqJS.Float64()
			variance := math.Max(0, zSumSqRounded/float64(len(zWindow))-mean*mean)
			sd := math.Sqrt(variance)
			if sd > 0 {
				stats.Z50[i] = (v - mean) / sd
			} else {
				stats.Z50[i] = 0
			}
		}
		if isFinite(stats.Ratio20[i]) && isFinite(stats.SpreadATR[i]) {
			stats.EffortResultRatio[i] = stats.Ratio20[i] / math.Max(stats.SpreadATR[i], 0.1)
		}

		volWindow = append(volWindow, v)
		volSum += v
		if len(volWindow) > smaLen {
			volSum -= volWindow[0]
			volWindow = volWindow[1:]
		}

		zWindow = append(zWindow, v)
		zSum += v
		vSq := v * v
		zSumSqJS.Add(zSumSqJS, new(big.Float).SetPrec(53).SetMode(big.ToNearestEven).SetFloat64(vSq))
		if len(zWindow) > zLen {
			old := zWindow[0]
			zWindow = zWindow[1:]
			zSum -= old
			oldSq := old * old
			zSumSqJS.Sub(zSumSqJS, new(big.Float).SetPrec(53).SetMode(big.ToNearestEven).SetFloat64(oldSq))
		}
	}
	return stats
}

func nanSlice(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	return out
}

func nan() float64 {
	return math.NaN()
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
