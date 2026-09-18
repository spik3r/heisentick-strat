// Package marketdata contains compact OHLCV data structures shared by the Go
// research engine packages.
package marketdata

// Bar is one UTC OHLCV candle. T is Unix milliseconds.
type Bar struct {
	T float64
	O float64
	H float64
	L float64
	C float64
	V float64
}

// Series stores candles in columnar form for cache-friendly scans.
type Series struct {
	T []float64
	O []float64
	H []float64
	L []float64
	C []float64
	V []float64
}

// NewSeries allocates all OHLCV columns with length n.
func NewSeries(n int) Series {
	return Series{
		T: make([]float64, n),
		O: make([]float64, n),
		H: make([]float64, n),
		L: make([]float64, n),
		C: make([]float64, n),
		V: make([]float64, n),
	}
}

// SeriesFromBars converts row bars to columnar storage.
func SeriesFromBars(bars []Bar) Series {
	series := NewSeries(len(bars))
	for i, bar := range bars {
		series.T[i] = bar.T
		series.O[i] = bar.O
		series.H[i] = bar.H
		series.L[i] = bar.L
		series.C[i] = bar.C
		series.V[i] = bar.V
	}
	return series
}

// Len returns the number of candles in the series.
func (s Series) Len() int {
	return len(s.T)
}

// Bar returns the row candle at i.
func (s Series) Bar(i int) Bar {
	return Bar{
		T: s.T[i],
		O: s.O[i],
		H: s.H[i],
		L: s.L[i],
		C: s.C[i],
		V: s.V[i],
	}
}

// Bars returns a row-copy of the series.
func (s Series) Bars() []Bar {
	bars := make([]Bar, s.Len())
	for i := range bars {
		bars[i] = s.Bar(i)
	}
	return bars
}
