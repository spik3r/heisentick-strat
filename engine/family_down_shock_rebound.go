package engine

import (
	"math"
	"sort"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// The fixed normalizers reproduce HT-086's portable rule. They are not fitted
// on the backtest period and do not implement the Isolation Forest detector.
const shockMinute = 60_000.0
const shockSourceDuration = 15 * shockMinute
const shockGap = 120 * shockMinute

type downShockReboundParams struct {
	Entry, TargetMode string
	TargetValue       float64
}

func downShockReboundParamsFromConfig(cfg dsl.Config) downShockReboundParams {
	c := mapValue(cfg, "downShockRebound")
	return downShockReboundParams{stringValue(c, "entry", "immediate"), stringValue(c, "targetMode", "off"), numberValue(c, "targetValue", 0)}
}

type downShockSignal struct {
	KnownAt, ATR, RZ, RangeRatio float64
	Row                          int
}

func downShockSignals(source marketdata.Series) []downShockSignal {
	var out []downShockSignal
	var previous, variance float64
	count := 0
	ranges := map[int][]float64{}
	tr := []float64{}
	const alpha = 2.0 / 461.0
	for i, t := range source.T {
		if !isWeekdayTimestamp(t) {
			continue
		}
		c, h, l := source.C[i], source.H[i], source.L[i]
		if c <= 0 || h <= 0 || l <= 0 || !isFinite(c+h+l) {
			continue
		}
		lr := math.Log(h / l)
		slot := int(math.Mod(t, dayMS) / shockSourceDuration)
		history := ranges[slot]
		trueRange := h - l
		if count > 0 {
			trueRange = math.Max(trueRange, math.Max(math.Abs(h-previous), math.Abs(l-previous)))
		}
		tr = append(tr, trueRange)
		if len(tr) > 14 {
			tr = tr[1:]
		}
		if count >= 460 && len(history) >= 10 && len(tr) == 14 {
			sorted := append([]float64(nil), history...)
			sort.Float64s(sorted)
			median := sorted[len(sorted)/2]
			if len(sorted)%2 == 0 {
				median = (median + sorted[len(sorted)/2-1]) / 2
			}
			sigma := math.Max(math.Sqrt(variance), 1e-6)
			rz := math.Log(c/previous) / sigma
			relative := math.Max(-5, math.Min(5, math.Log((lr+1e-6)/(median+1e-6))))
			if rz <= -3 && relative >= math.Log(2) {
				atr := 0.0
				for _, v := range tr {
					atr += v
				}
				atr /= 14
				if atr > 0 {
					out = append(out, downShockSignal{t + shockSourceDuration, atr, rz, math.Exp(relative), i})
				}
			}
		}
		history = append(history, lr)
		if len(history) > 20 {
			history = history[1:]
		}
		ranges[slot] = history
		park := lr * lr / (4 * math.Log(2))
		if count == 0 {
			variance = park
		} else {
			variance += alpha * (park - variance)
		}
		count++
		previous = c
	}
	return out
}

// This family owns its source-close boundary explicitly. The generic source
// scheduler waits for a later chart close, which would delay immediate entries.
func (b *broker) runDownShockRebound() []Trade {
	signals := downShockSignals(marketdata.SeriesFromBars(b.fixture.SourceBars))
	pointer := 0
	var waiting *downShockSignal
	var queued *downShockSignal
	end := b.executionEnd()
	if end >= b.series.Len() {
		end = b.series.Len() - 1
	}
	start := b.executionStart()
	busyUntil := -math.MaxFloat64
	for i := 0; i <= end; i++ {
		t := b.series.T[i]
		closeT := t + shockMinute
		if i < start {
			waiting = nil
			queued = nil
			continue
		}
		// Consume only source bars already closed at this minute's open.
		for pointer < len(signals) && signals[pointer].KnownAt <= t {
			signal := signals[pointer]
			pointer++
			if signal.KnownAt < b.series.T[start] || signal.KnownAt < busyUntil || t-signal.KnownAt > shockGap || b.hasPosition || queued != nil {
				continue
			}
			waiting = &signal
		}
		if waiting != nil && b.params.DownShockRebound.Entry == "immediate" {
			queued = waiting
			waiting = nil
		}
		if queued != nil {
			contiguous := b.params.DownShockRebound.Entry != "reversal" || i > 0 && t-b.series.T[i-1] == shockMinute
			if contiguous && t-queued.KnownAt <= shockGap && b.executionIndexAllowed(i) && b.params.AllowLong {
				risk := 2 * queued.ATR
				tp := 0.0
				p := b.params.DownShockRebound
				if p.TargetMode == "atr" {
					tp = b.series.O[i] + p.TargetValue*queued.ATR
				}
				if p.TargetMode == "bp" {
					tp = b.series.O[i] * (1 + p.TargetValue/10_000)
				}
				ord := order{Side: sideLong, SL: b.series.O[i] - risk, TP: tp, NoTarget: p.TargetMode == "off", Size: b.params.RiskUSD / risk, HasSize: true, GapAwareStop: true, Tag: "DSL-DSR", Meta: TradeMeta{"setup": "downShockRebound", "sourceRow": float64(queued.Row), "sourceCloseT": queued.KnownAt, "sourceATR": queued.ATR, "returnZ": queued.RZ, "rangeRatio": queued.RangeRatio, "entryMode": p.Entry, "targetMode": p.TargetMode}}
				b.openPosition(sideLong, b.series.O[i], ord, i)
			}
			queued = nil
		}
		if b.hasPosition && (t-b.position.EntryT >= 240*shockMinute || i > 0 && t-b.series.T[i-1] > shockGap) {
			rule := "down-shock-time"
			if i > 0 && t-b.series.T[i-1] > shockGap {
				rule = "down-shock-gap"
			}
			b.closePosition(b.series.O[i], i, ReasonRule, rule)
			busyUntil = closeT
		}
		wasOpen := b.hasPosition
		b.resolveIntrabarExit(i)
		if wasOpen && !b.hasPosition {
			busyUntil = closeT
		}
		if b.hasPosition && closeT-b.position.EntryT >= 240*shockMinute {
			b.closePosition(b.series.C[i], i, ReasonRule, "down-shock-time")
			busyUntil = closeT
		}
		if waiting != nil && !b.hasPosition && queued == nil {
			p := b.params.DownShockRebound
			ready := p.Entry == "immediate"
			if p.Entry == "reversal" {
				if closeT > waiting.KnownAt+15*shockMinute {
					waiting = nil
					continue
				}
				ready = t >= waiting.KnownAt && i > 0 && t-b.series.T[i-1] == shockMinute && b.series.C[i] > b.series.O[i] && b.series.C[i] > b.series.H[i-1]
			}
			if ready && b.params.AllowLong {
				queued = waiting
				waiting = nil
			}
		}
	}
	if b.hasPosition && end >= 0 {
		b.closePosition(b.series.C[end], end, ReasonEndOfTest, "")
	}
	return b.trades
}
