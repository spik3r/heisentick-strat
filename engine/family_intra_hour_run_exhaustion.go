package engine

import (
	"math"
	"strconv"

	"github.com/spik3r/heisentick-strat/marketdata"
)

const intraHourMillis = 60 * 60 * 1000

// runIntraHourRunExhaustion mirrors
// engine/dsl/setups/intraHourRunExhaustion.js: it fades a steady
// same-direction run inside a clock hour when the hour's final candle closes
// back against that run.
func (b *broker) runIntraHourRunExhaustion() []Trade {
	end := b.executionEnd()
	if end >= b.series.Len() {
		end = b.series.Len() - 1
	}
	p := b.params.IntraHourRunExhaustion
	atrLen := p.ATRLength
	if atrLen < 1 {
		atrLen = 14
	}
	maxHold := int(b.params.MaxHoldBars)
	if maxHold < 1 {
		maxHold = 8
	}
	if p.HourCandles < 2 || p.RunCandles < 1 || p.RunCandles >= p.HourCandles {
		return b.trades
	}
	slotMillis := intraHourMillis / p.HourCandles
	if slotMillis*p.HourCandles != intraHourMillis {
		return b.trades
	}
	finalSlotOffset := float64((p.HourCandles - 1) * slotMillis)

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
		if b.hasPosition || atrs[i] == 0 {
			continue
		}
		// Fire only on the final candle of a complete clock hour.
		if math.Mod(b.series.T[i], intraHourMillis) != finalSlotOffset {
			continue
		}
		start := i - p.RunCandles
		if start < 0 {
			continue
		}
		contiguous := true
		for j := start + 1; j <= i; j++ {
			if b.series.T[j]-b.series.T[j-1] != float64(slotMillis) {
				contiguous = false
				break
			}
		}
		if !contiguous {
			continue
		}

		direction := intraHourRunDirection(b.series, start, i)
		if direction == 0 {
			continue
		}
		atr := atrs[i]
		if (b.series.C[i-1]-b.series.O[start])*direction < p.MinRunATR*atr {
			continue
		}
		runExtreme := b.series.H[start]
		if direction < 0 {
			runExtreme = b.series.L[start]
		}
		for j := start + 1; j < i; j++ {
			if direction > 0 {
				runExtreme = math.Max(runExtreme, b.series.H[j])
			} else {
				runExtreme = math.Min(runExtreme, b.series.L[j])
			}
		}
		if p.RequirePoke {
			poked := b.series.H[i] > runExtreme
			if direction < 0 {
				poked = b.series.L[i] < runExtreme
			}
			if !poked {
				continue
			}
		}

		location := 0.5
		if barRange := b.series.H[i] - b.series.L[i]; barRange > 0 {
			location = (b.series.C[i] - b.series.L[i]) / barRange
			if direction < 0 {
				location = 1 - location
			}
		}
		if location > p.ExhaustLocationPct {
			continue
		}

		// Fade the run: an up run is faded short.
		s := sideShort
		if direction < 0 {
			s = sideLong
		}
		if (s == sideShort && !b.params.AllowShort) || (s == sideLong && !b.params.AllowLong) {
			continue
		}
		hourExtreme := math.Max(runExtreme, b.series.H[i])
		if direction < 0 {
			hourExtreme = math.Min(runExtreme, b.series.L[i])
		}
		entry := b.series.C[i]
		sl := hourExtreme + direction*p.StopPadATR*atr
		risk := math.Abs(sl - entry)
		if risk <= 0 {
			continue
		}
		b.openPosition(s, entry, order{
			Side: s, SL: sl, TP: entry - direction*p.TargetR*risk,
			RiskUSD: b.params.RiskUSD, HasRisk: true, Tag: "DSL-IHRE",
			Meta: TradeMeta{
				"setup":                   "intraHourRunExhaustion",
				"entryIndex":              float64(i),
				"runDirection":            direction,
				"runSizeAtr":              diagnosticNumber((b.series.C[i-1] - b.series.O[start]) * direction / atr),
				"exhaustionCloseLocation": diagnosticNumber(location),
				"signalAtr":               diagnosticNumber(atr),
			},
		}, i)
	}
	if b.hasPosition && end >= 0 {
		b.closePosition(b.series.C[end], end, ReasonEndOfTest, "")
	}
	return b.trades
}

// intraHourRunDirection reports +1 when every candle in [start, end) closes
// above its open and above the previous close, -1 for the mirror case, and 0
// when the run is not steady.
func intraHourRunDirection(series marketdata.Series, start, end int) float64 {
	up, down := true, true
	for j := start; j < end; j++ {
		if !(series.C[j] > series.O[j] && (j == start || series.C[j] > series.C[j-1])) {
			up = false
		}
		if !(series.C[j] < series.O[j] && (j == start || series.C[j] < series.C[j-1])) {
			down = false
		}
	}
	if up {
		return 1
	}
	if down {
		return -1
	}
	return 0
}

// diagnosticNumber mirrors the JS runtime's Number(value.toPrecision(12)), so
// trade meta stays byte-identical across the two implementations.
func diagnosticNumber(value float64) float64 {
	rounded, err := strconv.ParseFloat(strconv.FormatFloat(value, 'g', 12, 64), 64)
	if err != nil {
		return value
	}
	return rounded
}
