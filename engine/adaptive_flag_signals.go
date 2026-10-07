package engine

import (
	"fmt"
	"math"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func adaptiveFlagPointer[T any](v T) *T { return &v }

// Truncate before computing indicators or managing orders. Inspect timestamps
// only until the first supplied open at/after the exclusive end; arbitrary
// future values cannot change the admitted prefix. Shape is still mandatory.
func adaptiveFlagWindowPrefix(s marketdata.Series, timeframe string, requested *AdaptiveFlagExecutionWindow) (marketdata.Series, *AdaptiveFlagExecutionWindow, error) {
	if requested == nil {
		return s, nil, nil
	}
	window := *requested
	step := int64(1800000)
	if timeframe == "H1" {
		step = 3600000
	}
	if window.TradeFromMS < 0 || window.TradeToMS > 9007199254740991 || window.TradeFromMS >= window.TradeToMS || window.TradeFromMS%step != 0 || window.TradeToMS%step != 0 {
		return marketdata.Series{}, nil, fmt.Errorf("adaptive evaluation window requires exact nonnegative %s UTC-grid start < end within the safe millisecond range", timeframe)
	}
	n := s.Len()
	if len(s.O) != n || len(s.H) != n || len(s.L) != n || len(s.C) != n || len(s.V) != n {
		return marketdata.Series{}, nil, fmt.Errorf("adaptive evaluation window requires equal-length six-column OHLCV")
	}
	end := n
	for i, t := range s.T {
		if t >= float64(window.TradeToMS) {
			end = i
			break
		}
		if !isFinite(t) {
			return marketdata.Series{}, nil, fmt.Errorf("adaptive evaluation window encountered an invalid timestamp before its end at row %d", i)
		}
	}
	return marketdata.Series{T: s.T[:end], O: s.O[:end], H: s.H[:end], L: s.L[:end], C: s.C[:end], V: s.V[:end]}, &window, nil
}

func adaptiveFlagSource(s marketdata.Series, timeframe string) ([]AdaptiveFlagBar, int64, error) {
	var step int64
	switch timeframe {
	case "M30":
		step = 1800000
	case "H1":
		step = 3600000
	default:
		return nil, 0, fmt.Errorf("adaptive volume flag requires M30 or H1 supplied bars")
	}
	n := s.Len()
	if n == 0 || len(s.O) != n || len(s.H) != n || len(s.L) != n || len(s.C) != n || len(s.V) != n {
		return nil, 0, fmt.Errorf("adaptive volume flag requires nonempty equal-length six-column OHLCV")
	}
	bars := make([]AdaptiveFlagBar, n)
	previous := -1.
	for i, timestamp := range s.T {
		// Close-phase availability must also remain in the exact-safe range.
		if !isFinite(timestamp) || timestamp < 0 || timestamp > float64(int64(9007199254740991)-step) || math.Trunc(timestamp) != timestamp || int64(timestamp)%step != 0 || timestamp <= previous {
			return nil, 0, fmt.Errorf("adaptive volume flag invalid, unordered, or off-grid %s timestamp at row %d", timeframe, i)
		}
		previous = timestamp
		o, h, l, c, v := s.O[i], s.H[i], s.L[i], s.C[i], s.V[i]
		if !isFinite(o) || !isFinite(h) || !isFinite(l) || !isFinite(c) || !isFinite(v) || l <= 0 || o < l || o > h || c < l || c > h || v < 0 {
			return nil, 0, fmt.Errorf("adaptive volume flag invalid OHLCV at row %d", i)
		}
		bars[i] = AdaptiveFlagBar{Index: i, OpenT: int64(timestamp), CloseT: int64(timestamp) + step, Open: o, High: h, Low: l, Close: c, Volume: v}
	}
	return bars, step, nil
}

// Every arithmetic operation in this family is explicitly converted to float64.
// These conversions are rounding barriers under the Go specification. The fixed
// BINARY64_ORDERED_V1 contract uses a sequential ATR seed and written operation
// order; version-specific Python compensated sum is deliberately not emulated.
// They are not a claim of completed cross-architecture qualification.
func adaptiveFlagEMA(bars []AdaptiveFlagBar, length int) []float64 {
	out := make([]float64, len(bars))
	alpha := float64(2.0 / float64(float64(length)+1.0))
	weight := float64(1.0 - alpha)
	for i, b := range bars {
		if i == 0 {
			out[i] = b.Close
			continue
		}
		out[i] = float64(float64(alpha*b.Close) + float64(weight*out[i-1]))
	}
	return out
}

func adaptiveFlagATR(bars []AdaptiveFlagBar, length int) ([]float64, []*float64) {
	tr := make([]float64, len(bars))
	out := make([]*float64, len(bars))
	value := 0.
	for i, b := range bars {
		tr[i] = float64(b.High - b.Low)
		if i > 0 {
			tr[i] = math.Max(tr[i], math.Max(math.Abs(float64(b.High-bars[i-1].Close)), math.Abs(float64(b.Low-bars[i-1].Close))))
		}
		if i < length {
			value = float64(value + tr[i])
			if i == length-1 {
				value = float64(value / float64(length))
				out[i] = adaptiveFlagPointer(value)
			}
		} else {
			value = float64(float64(float64(value*float64(length-1))+tr[i]) / float64(length))
			out[i] = adaptiveFlagPointer(value)
		}
	}
	return tr, out
}

func adaptiveFlagVolumeSMA(bars []AdaptiveFlagBar, length int) []*float64 {
	out := make([]*float64, len(bars))
	total := 0.
	for i, b := range bars {
		total = float64(total + b.Volume)
		if i >= length {
			total = float64(total - bars[i-length].Volume)
		}
		if i >= length-1 {
			out[i] = adaptiveFlagPointer(float64(total / float64(length)))
		}
	}
	return out
}

func adaptiveFlagConfirmedPivot(bars []AdaptiveFlagBar, current, sensitivity int, high bool) *AdaptiveFlagPivot {
	idx := current - sensitivity
	if idx < sensitivity {
		return nil
	}
	value := bars[idx].Low
	if high {
		value = bars[idx].High
	}
	for j := idx - sensitivity; j <= current; j++ {
		if j == idx {
			continue
		}
		if (high && bars[j].High >= value) || (!high && bars[j].Low <= value) {
			return nil
		}
	}
	return &AdaptiveFlagPivot{Index: idx, ConfirmationIdx: current, Price: value}
}

func adaptiveFlagPoleRecent(age int, r dsl.AdaptiveFlagRules) bool {
	// Equivalent to age <= max_flag_bars + sensitivity + 6, without overflow.
	return age <= r.PivotSensitivity || age-r.PivotSensitivity <= r.MaxFlagBars || age-r.PivotSensitivity-r.MaxFlagBars <= 6
}

func adaptiveFlagCandidate(s AdaptiveFlagSnapshot, r dsl.AdaptiveFlagRules) *AdaptiveFlagSignal {
	if s.ATR == nil || (!s.BullValid && !s.BearValid) {
		return nil
	}
	buffer := float64(r.EntryBufferATR * *s.ATR)
	stopBuffer := float64(r.ATRStopMult * *s.ATR)
	out := AdaptiveFlagSignal{SignalIdx: s.Index, Side: "long"}
	if s.BullValid {
		out.Trigger = float64(s.FlagHigh + buffer)
		out.Stop = float64(s.FlagLow - stopBuffer)
		out.Target = float64(out.Trigger + float64(float64(out.Trigger-out.Stop)*r.TargetR))
	} else {
		out.Side = "short"
		out.Trigger = float64(s.FlagLow - buffer)
		out.Stop = float64(s.FlagHigh + stopBuffer)
		out.Target = float64(out.Trigger - float64(float64(out.Stop-out.Trigger)*r.TargetR))
	}
	out.PlannedTriggerToStopDistance = math.Abs(float64(out.Trigger - out.Stop))
	return &out
}

func adaptiveFlagSnapshots(bars []AdaptiveFlagBar, r dsl.AdaptiveFlagRules) []AdaptiveFlagSnapshot {
	tr, atr := adaptiveFlagATR(bars, r.ATRLen)
	fast, slow := adaptiveFlagEMA(bars, r.FastEMALen), adaptiveFlagEMA(bars, r.SlowEMALen)
	vol := adaptiveFlagVolumeSMA(bars, r.VolumeSMALen)
	rows := make([]AdaptiveFlagSnapshot, len(bars))
	var lastHi, lastLo *AdaptiveFlagPivot
	for i, b := range bars {
		hi, lo := adaptiveFlagConfirmedPivot(bars, i, r.PivotSensitivity, true), adaptiveFlagConfirmedPivot(bars, i, r.PivotSensitivity, false)
		if hi != nil {
			lastHi = hi
		}
		if lo != nil {
			lastLo = lo
		}
		s := AdaptiveFlagSnapshot{AdaptiveFlagBar: b, TrueRange: tr[i], ATR: atr[i], FastEMA: fast[i], SlowEMA: slow[i], VolumeSMA: vol[i], ConfirmedHigh: hi, ConfirmedLow: lo, LastHigh: lastHi, LastLow: lastLo, PoleEndpointIdx: i}
		if i >= r.PivotSensitivity {
			s.PoleEndpointIdx = i - r.PivotSensitivity
		}
		s.PoleHigh, s.PoleLow = bars[s.PoleEndpointIdx].High, bars[s.PoleEndpointIdx].Low
		if lastLo != nil && adaptiveFlagPoleRecent(i-lastLo.Index, r) {
			s.BullHeight = float64(s.PoleHigh - lastLo.Price)
			s.BullImpulse = atr[i] != nil && s.BullHeight >= float64(r.MinPoleATR**atr[i]) && (lastHi == nil || lastHi.Index > lastLo.Index)
		}
		if lastHi != nil && adaptiveFlagPoleRecent(i-lastHi.Index, r) {
			s.BearHeight = float64(lastHi.Price - s.PoleLow)
			s.BearImpulse = atr[i] != nil && s.BearHeight >= float64(r.MinPoleATR**atr[i]) && (lastLo == nil || lastLo.Index > lastHi.Index)
		}
		if lastHi != nil {
			s.SinceHigh = i - lastHi.Index
		}
		s.FlagBars = min(max(r.MinFlagBars, s.SinceHigh), r.MaxFlagBars)
		s.FlagStartIdx = max(0, i-s.FlagBars+1)
		s.FlagHigh, s.FlagLow = bars[s.FlagStartIdx].High, bars[s.FlagStartIdx].Low
		for j := s.FlagStartIdx + 1; j <= i; j++ {
			s.FlagHigh = math.Max(s.FlagHigh, bars[j].High)
			s.FlagLow = math.Min(s.FlagLow, bars[j].Low)
		}
		s.FlagWidth = float64(s.FlagHigh - s.FlagLow)
		s.TrendBull = !r.UseEMATrend || fast[i] > slow[i] || b.Close > fast[i]
		s.TrendBear = !r.UseEMATrend || fast[i] < slow[i] || b.Close < fast[i]
		s.VolumeOK = !r.UseVolumeFilter || (vol[i] != nil && b.Volume > float64(*vol[i]*r.VolumeSMAMult))
		if lastHi != nil {
			s.BullRetrace = adaptiveFlagPointer(float64(lastHi.Price - s.FlagLow))
		}
		if lastLo != nil {
			s.BearRetrace = adaptiveFlagPointer(float64(s.FlagHigh - lastLo.Price))
		}
		s.BullValid = s.BullImpulse && s.BullRetrace != nil && *s.BullRetrace <= float64(s.BullHeight*r.MaxFlagRetrace) && s.FlagWidth <= float64(s.BullHeight*r.FlagWidthPoleMult) && s.TrendBull && s.VolumeOK
		s.BearValid = s.BearImpulse && s.BearRetrace != nil && *s.BearRetrace <= float64(s.BearHeight*r.MaxFlagRetrace) && s.FlagWidth <= float64(s.BearHeight*r.FlagWidthPoleMult) && s.TrendBear && s.VolumeOK
		s.Candidate = adaptiveFlagCandidate(s, r)
		rows[i] = s
	}
	return rows
}
