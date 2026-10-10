package engine

import (
	"math"

	"github.com/spik3r/heisentick-strat/contextcols"
)

// Legacy Setup-9 controls (HT-227): faithful ports of the archived
// dslTDSequential and dslTDSeasonalReversal strategies, pinned at heisentick
// 48a1761867342494c69c77e7ecce325e10d5d935. The two profiles reproduce the
// archived behaviour on purpose, including its defects:
//
//   - the counter has no price-flip precondition and is index based, so it
//     compares across gaps;
//   - the strategy reads an Int8Array element, so the stored count wraps
//     modulo 256 while the internal count keeps growing. A long monotone run
//     therefore emits phantom or repeated nines. A corrected variant must be a
//     new profile, not an edit of these.
//
// The counter is a pure function of the closes up to each bar. It is computed
// once per run, so full runs, prefix replays and checkpoint resumes agree.

type legacySetup9State struct {
	stored []int8
	ready  bool
}

// legacySetup9Counter mirrors computeTDSetup. The returned slice holds the
// stored (wrapped) value; the internal count is a plain int that never wraps.
func legacySetup9Counter(close []float64) []int8 {
	n := len(close)
	out := make([]int8, n)
	count := 0
	for i := 4; i < n; i++ {
		switch {
		case close[i] > close[i-4]:
			if count < 0 {
				count = 1
			} else {
				count++
			}
		case close[i] < close[i-4]:
			if count > 0 {
				count = -1
			} else {
				count--
			}
		default:
			count = 0
		}
		out[i] = int8(count) // wraps modulo 256 like Int8Array
	}
	return out
}

// legacySetup9Perfected mirrors tdPerfected: fixed offsets i-3..i, inclusive.
func legacySetup9Perfected(high, low []float64, i int, sell bool) bool {
	if i < 3 {
		return false
	}
	if sell {
		return math.Max(high[i-1], high[i]) >= math.Max(high[i-3], high[i-2])
	}
	return math.Min(low[i-1], low[i]) <= math.Min(low[i-3], low[i-2])
}

// legacySetup9SeasonAgrees mirrors seasonalAgrees. next may be nil (bucket
// never created) and bullishPercent may be nil (no directional bars).
func legacySetup9SeasonAgrees(p legacySetup9Profile, next *contextcols.SeasonalityProjection, sell bool) bool {
	if next == nil || next.BullishPercent == nil {
		return false
	}
	if float64(next.DirectionalCount) < p.MinSamples {
		return false
	}
	if sell {
		return *next.BullishPercent <= 50-p.SeasonalityEdge
	}
	return *next.BullishPercent >= 50+p.SeasonalityEdge
}

func (b *broker) legacySetup9Counter() []int8 {
	if !b.legacy9.ready || len(b.legacy9.stored) != b.series.Len() {
		b.legacy9.stored = legacySetup9Counter(b.series.C)
		b.legacy9.ready = true
	}
	return b.legacy9.stored
}

// legacySetup9Signal evaluates the strategy rules at bar i without looking at
// the position state. The long rule is tried first and the first true rule wins.
func (b *broker) legacySetup9Signal(i int) (side, bool) {
	p := b.params.LegacySetup9
	if !p.Enabled {
		return 0, false
	}
	profile := p.Profile
	stored := int(b.legacySetup9Counter()[i])
	buy, sell := 0, 0
	if stored < 0 {
		buy = -stored
	} else if stored > 0 {
		sell = stored
	}
	var next *contextcols.SeasonalityProjection
	if profile.Seasonal {
		key := contextcols.SeasonalityKey{Dimension: "intraday", Lookback: legacySetup9SeasonKey, MinSamples: legacySetup9SeasonCtxMinSamp}
		if rows := b.cols.Seasonality[key]; i < len(rows) {
			next = rows[i].Next
		}
	}
	ok := func(isSell bool) bool {
		if profile.RequirePerfection && !legacySetup9Perfected(b.series.H, b.series.L, i, isSell) {
			return false
		}
		return !profile.Seasonal || legacySetup9SeasonAgrees(profile, next, isSell)
	}
	if float64(buy) == profile.SetupCount && ok(false) {
		return sideLong, true
	}
	if float64(sell) == profile.SetupCount && ok(true) {
		return sideShort, true
	}
	return 0, false
}

func (b *broker) onLegacySetup9Bar(i int) {
	if b.hasPosition || len(b.pendingOrders) > 0 || len(b.limitOrders) > 0 {
		return
	}
	if b.windowed && !b.executionIndexAllowed(i) {
		return
	}
	s, ok := b.legacySetup9Signal(i)
	if !ok {
		return
	}
	profile := b.params.LegacySetup9.Profile
	atr := b.cols.ATR[i]
	entry := b.series.C[i]
	sign := float64(s)
	// Same operation order as the archived strategy: low - atr*buffer, and
	// entry + side*|entry - sl|*R. The float64 conversions forbid fused
	// multiply-add, which arm64 would otherwise use and JavaScript never does.
	var sl float64
	tag := profile.LongTag
	if s == sideLong {
		sl = b.series.L[i] - float64(atr*profile.StopBufferATR)
	} else {
		sl = b.series.H[i] + float64(atr*profile.StopBufferATR)
		tag = profile.ShortTag
	}
	tp := entry + float64(float64(sign*math.Abs(entry-sl))*profile.RMultiple)
	b.openPosition(s, entry, order{
		Side: s, SL: sl, TP: tp, RiskUSD: b.params.RiskUSD, HasRisk: true, Tag: tag, Index: i,
		Meta: TradeMeta{
			"setup":       "legacySetup9",
			"profile":     profile.ID,
			"signalIndex": float64(i),
			"signalAtr":   atr,
		},
	}, i)
}
