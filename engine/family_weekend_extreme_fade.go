package engine

import (
	"math"
	"time"
)

const weekendBarMillis = 4 * 60 * 60 * 1000

func isMondayOpenTimestamp(timestamp float64) bool {
	d := time.UnixMilli(int64(timestamp)).UTC()
	return d.Weekday() == time.Monday && d.Hour() == 0 && d.Minute() == 0
}

func (b *broker) runWeekendExtremeFade() []Trade {
	end := b.executionEnd()
	if end >= b.series.Len() {
		end = b.series.Len() - 1
	}
	atrLen := b.params.WeekendExtremeFade.ATRLength
	if atrLen < 1 {
		atrLen = 20
	}
	trueRanges := make([]float64, b.series.Len())
	atrs := make([]float64, b.series.Len())
	for i := 0; i <= end; i++ {
		previousClose := b.series.C[i]
		if i > 0 {
			previousClose = b.series.C[i-1]
		}
		trueRanges[i] = math.Max(b.series.H[i]-b.series.L[i], math.Max(
			math.Abs(b.series.H[i]-previousClose), math.Abs(b.series.L[i]-previousClose),
		))
		if i+1 < atrLen {
			continue
		}
		sum := 0.0
		for j := i + 1 - atrLen; j <= i; j++ {
			sum += trueRanges[j]
		}
		atrs[i] = sum / float64(atrLen)
	}

	maxHold := int(b.params.MaxHoldBars)
	if maxHold < 1 {
		maxHold = 12
	}
	for i := b.executionStart(); i <= end; i++ {
		if b.hasPosition && i > b.position.EntryIndex {
			pos := b.position
			if (pos.Side == sideLong && b.series.L[i] <= pos.SL) || (pos.Side == sideShort && b.series.H[i] >= pos.SL) {
				b.closePosition(pos.SL, i, "sl", "")
			} else if (!pos.NoTarget && pos.Side == sideLong && b.series.H[i] >= pos.TP) || (!pos.NoTarget && pos.Side == sideShort && b.series.L[i] <= pos.TP) {
				b.closePosition(pos.TP, i, "tp", "")
			} else if i-pos.EntryIndex >= maxHold {
				b.closePosition(b.series.C[i], i, "time", "")
			}
		}
		if b.hasPosition || !isMondayOpenTimestamp(b.series.T[i]) || atrs[i] == 0 {
			continue
		}
		start := i - 12
		if start < 0 || !weekendBarsContiguous(b.series.T, start, i) {
			continue
		}
		high, low := b.series.H[start], b.series.L[start]
		for j := start + 1; j < i; j++ {
			high = math.Max(high, b.series.H[j])
			low = math.Min(low, b.series.L[j])
		}
		rangeSize := high - low
		location := 0.5
		if rangeSize > 0 {
			location = (b.series.C[i-1] - low) / rangeSize
		}
		if rangeSize/atrs[i] > b.params.WeekendExtremeFade.MaxWeekendRangeATR {
			continue
		}
		long := b.params.AllowLong && location <= b.params.WeekendExtremeFade.CloseExtremePct && b.series.C[i] > b.series.O[i]
		short := b.params.AllowShort && location >= 1-b.params.WeekendExtremeFade.CloseExtremePct && b.series.C[i] < b.series.O[i]
		if !long && !short {
			continue
		}
		s := sideShort
		if long {
			s = sideLong
		}
		direction := float64(s)
		entry := b.series.C[i]
		atr := atrs[i]
		b.openPosition(s, entry, order{
			Side: s, SL: entry - direction*b.params.WeekendExtremeFade.StopATR*atr,
			TP:      entry + direction*b.params.WeekendExtremeFade.TargetATR*atr,
			RiskUSD: b.params.RiskUSD, HasRisk: true, Tag: "DSL-WEF",
			Meta: TradeMeta{
				"setup": "weekendExtremeFade", "side": s.String(),
				"entryIndex": float64(i), "weekendRangeAtr": rangeSize / atrs[i],
				"weekendCloseLocation": location, "signalAtr": atr,
			},
		}, i)
	}
	if b.hasPosition && end >= 0 {
		b.closePosition(b.series.C[end], end, ReasonEndOfTest, "")
	}
	return b.trades
}

func weekendBarsContiguous(timestamps []float64, start, end int) bool {
	for i := start + 1; i <= end; i++ {
		if timestamps[i]-timestamps[i-1] != weekendBarMillis {
			return false
		}
	}
	return true
}
