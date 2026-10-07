package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

type RangeReversionWindow struct {
	TradeFromMS int64 `json:"tradeFromMs"`
	TradeToMS   int64 `json:"tradeToMs"`
}
type RangeReversionRequest struct {
	Config                    dsl.Config
	EntrySeries, SourceSeries marketdata.Series
	Window                    RangeReversionWindow
	Execution                 RangeReversionExecution
}

type RangeReversionExecution struct {
	SlippagePerFill       float64 `json:"slippagePerFill"`
	CommissionPerUnitSide float64 `json:"commissionPerUnitSide"`
	Units                 float64 `json:"units"`
}

// The following named types keep the offline research contract independent of
// the ordinary route report's implicit aggregation and cost defaults.
type RangeReversionSignal struct {
	SignalIndex   int     `json:"signalIndex"`
	SignalCloseMS int64   `json:"signalCloseMs"`
	Side          string  `json:"side"`
	Bound         float64 `json:"bound"`
	Stop          float64 `json:"stop"`
	Target        float64 `json:"target"`
	PlannedRisk   float64 `json:"plannedRisk"`
}
type RangeReversionTrade struct {
	RangeReversionSignal
	EntryIndex      int     `json:"entryIndex"`
	EntryMS         int64   `json:"entryMs"`
	Entry           float64 `json:"entry"`
	RawEntry        float64 `json:"rawEntry"`
	ExitIndex       int     `json:"exitIndex"`
	ExitMS          int64   `json:"exitMs"`
	Exit            float64 `json:"exit"`
	RawExit         float64 `json:"rawExit"`
	ExitReason      string  `json:"exitReason"`
	GrossPnL        float64 `json:"grossPnl"`
	GrossR          float64 `json:"grossR"`
	CommissionPrice float64 `json:"commissionPrice"`
	NetPnL          float64 `json:"netPnl"`
	NetR            float64 `json:"netR"`
}
type RangeReversionResult struct {
	Schema              string                  `json:"schema"`
	Policy              string                  `json:"policy"`
	ConfigSHA256        string                  `json:"configSha256"`
	EntrySHA256         string                  `json:"entrySha256"`
	SourceSHA256        string                  `json:"sourceSha256"`
	EffectiveConfig     dsl.RangeReversionSpec  `json:"effectiveConfig"`
	Execution           RangeReversionExecution `json:"execution"`
	Window              RangeReversionWindow    `json:"window"`
	InputEntryBars      int                     `json:"inputEntryBars"`
	InputSourceBars     int                     `json:"inputSourceBars"`
	UsedEntryBars       int                     `json:"usedEntryBars"`
	UsedSourceBars      int                     `json:"usedSourceBars"`
	Signals             []RangeReversionSignal  `json:"signals"`
	Trades              []RangeReversionTrade   `json:"trades"`
	CensoredOpen        bool                    `json:"censoredOpen"`
	PendingAtWindowEnd  bool                    `json:"pendingAtWindowEnd"`
	CanceledAtEntryRisk int                     `json:"canceledAtEntryNonpositiveRisk"`
	Summary             map[string]any          `json:"summary"`
	Assumptions         []string                `json:"assumptions"`
}

type rrBar struct {
	t, closeT     int64
	o, h, l, c, v float64
}
type rrPosition struct {
	signal                    RangeReversionSignal
	entryIndex                int
	entryT                    int64
	entry                     float64
	rawEntry                  float64
	stop, target, initialRisk float64
	active                    bool
	queuedStop                bool
	beSet                     bool
	beQueued                  float64
}

func rrSeries(s marketdata.Series, tf string, beforeMS int64) ([]rrBar, int64, error) {
	step := int64(1800000)
	if tf == "H1" {
		step = 3600000
	}
	if s.Len() == 0 || len(s.O) != s.Len() || len(s.H) != s.Len() || len(s.L) != s.Len() || len(s.C) != s.Len() || len(s.V) != s.Len() {
		return nil, 0, fmt.Errorf("range reversion requires nonempty equal six-column series")
	}
	out := make([]rrBar, 0, s.Len())
	prev := int64(-1)
	for i := 0; i < s.Len(); i++ {
		t := s.T[i]
		if isFinite(t) && t >= float64(beforeMS) {
			break
		}
		if !isFinite(t) || t < 0 || math.Trunc(t) != t || t > float64(9007199254740991-step) || int64(t)%step != 0 || int64(t) <= prev {
			return nil, 0, fmt.Errorf("invalid or off-grid %s timestamp at row %d", tf, i)
		}
		prev = int64(t)
		o, h, l, c, v := s.O[i], s.H[i], s.L[i], s.C[i], s.V[i]
		if !isFinite(o) || !isFinite(h) || !isFinite(l) || !isFinite(c) || !isFinite(v) || l <= 0 || o < l || o > h || c < l || c > h || v < 0 {
			return nil, 0, fmt.Errorf("invalid OHLCV at row %d", i)
		}
		out = append(out, rrBar{int64(t), int64(t) + step, o, h, l, c, v})
	}
	return out, step, nil
}

func rrSource(s marketdata.Series, beforeMS int64) ([]rrBar, error) {
	if s.Len() == 0 || len(s.O) != s.Len() || len(s.H) != s.Len() || len(s.L) != s.Len() || len(s.C) != s.Len() || len(s.V) != s.Len() {
		return nil, fmt.Errorf("range reversion requires nonempty equal six-column source series")
	}
	const step = int64(4 * 3600000)
	out := make([]rrBar, 0, s.Len())
	prev := int64(-1)
	for i := 0; i < s.Len(); i++ {
		t := s.T[i]
		if isFinite(t) && t >= float64(beforeMS) {
			break
		}
		if !isFinite(t) || t < 0 || math.Trunc(t) != t || t > float64(9007199254740991-step) || int64(t)%step != 0 || int64(t) <= prev {
			return nil, fmt.Errorf("invalid or off-grid H4 timestamp at row %d", i)
		}
		prev = int64(t)
		o, h, l, c, v := s.O[i], s.H[i], s.L[i], s.C[i], s.V[i]
		if !isFinite(o) || !isFinite(h) || !isFinite(l) || !isFinite(c) || !isFinite(v) || l <= 0 || o < l || o > h || c < l || c > h || v < 0 {
			return nil, fmt.Errorf("invalid H4 OHLCV at row %d", i)
		}
		out = append(out, rrBar{int64(t), int64(t) + step, o, h, l, c, v})
	}
	return out, nil
}

func rrEMA(v []rrBar, n int) []float64 {
	out := make([]float64, len(v))
	if len(v) == 0 {
		return out
	}
	a := 2.0 / float64(n+1)
	out[0] = v[0].c
	for i := 1; i < len(v); i++ {
		out[i] = a*v[i].c + (1-a)*out[i-1]
	}
	return out
}
func rrRMA(v []float64, valid []bool, n int) []float64 {
	out := make([]float64, len(v))
	have := make([]bool, len(v))
	seed := 0.
	count := 0
	state := 0.
	active := false
	for i, x := range v {
		if !valid[i] {
			continue
		}
		if !active {
			seed += x
			count++
			if count == n {
				state = seed / float64(n)
				active = true
				out[i] = state
				have[i] = true
			}
		} else {
			state = (state*float64(n-1) + x) / float64(n)
			out[i] = state
			have[i] = true
		}
	}
	_ = have
	return out
}
func rrIndicators(b []rrBar, r dsl.RangeReversionRules) (atr, adx, chop []float64, atrOK, adxOK, chopOK []bool) {
	n := len(b)
	atr = make([]float64, n)
	adx = make([]float64, n)
	chop = make([]float64, n)
	atrOK = make([]bool, n)
	adxOK = make([]bool, n)
	chopOK = make([]bool, n)
	tr := make([]float64, n)
	trOK := make([]bool, n)
	plus := make([]float64, n)
	minus := make([]float64, n)
	pOK := make([]bool, n)
	mOK := make([]bool, n)
	for i, x := range b {
		tr[i] = x.h - x.l
		trOK[i] = true
		pOK[i] = true
		mOK[i] = true
		if i > 0 {
			prev := b[i-1]
			tr[i] = math.Max(x.h-x.l, math.Max(math.Abs(x.h-prev.c), math.Abs(x.l-prev.c)))
			up := x.h - prev.h
			down := prev.l - x.l
			if up > down && up > 0 {
				plus[i] = up
			}
			if down > up && down > 0 {
				minus[i] = down
			}
		} else {
			trOK[i] = false
		}
	}
	state := 0.
	for i := range b {
		if i < r.ATRLength {
			state += tr[i]
			if i == r.ATRLength-1 {
				state /= float64(r.ATRLength)
				atr[i] = state
				atrOK[i] = true
			}
		} else {
			state = (state*float64(r.ATRLength-1) + tr[i]) / float64(r.ATRLength)
			atr[i] = state
			atrOK[i] = true
		}
	}
	dx := make([]float64, n)
	dxOK := make([]bool, n)
	for i := range b {
		st, ok1 := rrFullSum(tr, trOK, i, r.VectorLength)
		sp, ok2 := rrFullSum(plus, pOK, i, r.VectorLength)
		sm, ok3 := rrFullSum(minus, mOK, i, r.VectorLength)
		if !ok1 || !ok2 || !ok3 {
			continue
		}
		pdi, mdi := 0., 0.
		if st != 0 {
			pdi = sp / st * 100
			mdi = sm / st * 100
		}
		den := pdi + mdi
		if den == 0 {
			dx[i] = 0
		} else {
			dx[i] = math.Abs(pdi-mdi) / den * 100
		}
		dxOK[i] = true
	}
	adx = rrRMA(dx, dxOK, r.VectorLength)
	for i := range b {
		_, ok := rrFullSum(dx, dxOK, i, r.VectorLength)
		if ok {
			adxOK[i] = true
		}
	}
	for i := range b {
		sum, ok := rrFullSum(tr, trOK, i, r.VectorLength)
		if !ok {
			continue
		}
		start := i - r.VectorLength + 1
		hi, lo := b[start].h, b[start].l
		for j := start + 1; j <= i; j++ {
			hi = math.Max(hi, b[j].h)
			lo = math.Min(lo, b[j].l)
		}
		width := hi - lo
		if width == 0 {
			chop[i] = 50
		} else if sum > 0 {
			chop[i] = 100 * math.Log10(sum/width) / math.Log10(float64(r.VectorLength))
		}
		chopOK[i] = true
	}
	return
}
func rrFullSum(v []float64, valid []bool, end, n int) (float64, bool) {
	if end+1 < n {
		return 0, false
	}
	s := 0.
	for i := end - n + 1; i <= end; i++ {
		if !valid[i] {
			return 0, false
		}
		s += v[i]
	}
	return s, true
}

func rrSourceAvailability(b []rrBar) []int64 {
	out := make([]int64, len(b))
	for i := 0; i < len(b); i++ {
		out[i] = math.MaxInt64
		if i+1 < len(b) {
			out[i] = b[i+1].t
		}
	}
	return out
}
func rrBounds(b []rrBar, i, n int) (float64, float64, bool) {
	if i < n {
		return 0, 0, false
	}
	hi, lo := b[i-n].h, b[i-n].l
	for j := i - n + 1; j < i; j++ {
		hi = math.Max(hi, b[j].h)
		lo = math.Min(lo, b[j].l)
	}
	return hi, lo, true
}
func rrExit(p *rrPosition, b rrBar, policy string) (string, float64, bool) {
	long := p.signal.Side == "long"
	if long && b.o <= p.stop {
		return "stop-gap", b.o, true
	}
	if !long && b.o >= p.stop {
		return "stop-gap", b.o, true
	}
	if long && b.o >= p.target {
		return "target-gap", b.o, true
	}
	if !long && b.o <= p.target {
		return "target-gap", b.o, true
	}
	if policy == dsl.RangeReversionImmediatePolicy {
		if long && b.l <= p.stop && b.h >= p.target {
			return "stop-same-bar", p.stop, true
		}
		if !long && b.h >= p.stop && b.l <= p.target {
			return "stop-same-bar", p.stop, true
		}
	}
	path := [4]float64{b.o, b.h, b.l, b.c}
	if policy == dsl.RangeReversionDelayedPinePolicy && math.Abs(b.o-b.h) > math.Abs(b.o-b.l) {
		path = [4]float64{b.o, b.l, b.h, b.c}
	}
	for k := 0; k < 3; k++ {
		a, z := path[k], path[k+1]
		if long {
			if z < a && z <= p.stop && p.stop <= a {
				return "stop", p.stop, true
			}
			if z > a && a <= p.target && p.target <= z {
				return "target", p.target, true
			}
		} else {
			if z > a && a <= p.stop && p.stop <= z {
				return "stop", p.stop, true
			}
			if z < a && z <= p.target && p.target <= a {
				return "target", p.target, true
			}
		}
	}
	return "", 0, false
}

func RunRangeReversion(request RangeReversionRequest) (RangeReversionResult, error) {
	spec, err := dsl.DecodeRangeReversion(request.Config)
	if err != nil {
		return RangeReversionResult{}, err
	}
	r := spec.Rules
	if request.Window.TradeFromMS < 0 || request.Window.TradeToMS <= request.Window.TradeFromMS {
		return RangeReversionResult{}, fmt.Errorf("range-reversion window requires nonnegative start < end")
	}
	stepMS := int64(1800000)
	if r.Timeframe == "H1" {
		stepMS = 3600000
	}
	if request.Window.TradeFromMS > 9007199254740991 || request.Window.TradeToMS > 9007199254740991 || request.Window.TradeFromMS%stepMS != 0 || request.Window.TradeToMS%stepMS != 0 {
		return RangeReversionResult{}, fmt.Errorf("range-reversion window must use exact UTC %s boundaries", r.Timeframe)
	}
	if !isFinite(request.Execution.SlippagePerFill) || request.Execution.SlippagePerFill < 0 || !isFinite(request.Execution.CommissionPerUnitSide) || request.Execution.CommissionPerUnitSide < 0 || !isFinite(request.Execution.Units) || request.Execution.Units <= 0 {
		return RangeReversionResult{}, fmt.Errorf("range-reversion execution requires finite nonnegative slippage/commission and positive fixed units")
	}
	entry, _, err := rrSeries(request.EntrySeries, r.Timeframe, request.Window.TradeToMS)
	if err != nil {
		return RangeReversionResult{}, err
	}
	source, err := rrSource(request.SourceSeries, request.Window.TradeToMS)
	if err != nil {
		return RangeReversionResult{}, err
	}
	if len(entry) == 0 || len(source) < 2 {
		return RangeReversionResult{}, fmt.Errorf("range reversion requires entry history and at least two source rows before trade end")
	}
	atr, adx, chop, atrOK, adxOK, chopOK := rrIndicators(entry, r)
	htfEMA := rrEMA(source, r.HTFEMALength)
	sourceAvail := rrSourceAvailability(source)
	var trades []RangeReversionTrade
	var signals []RangeReversionSignal
	var pending *RangeReversionSignal
	var pos *rrPosition
	lastExit := -1
	censored := false
	canceledAtEntryRisk := 0
	srcIdx := -1
	for i, b := range entry {
		for srcIdx+1 < len(sourceAvail) && sourceAvail[srcIdx+1] <= b.t {
			srcIdx++
		}
		justEntered := false
		rejectedAtEntry := false
		exited := false
		if pending != nil && i > pending.SignalIndex {
			p := *pending
			pending = nil
			rawEntry := b.o
			entryPx := rawEntry
			if p.Side == "long" {
				entryPx += request.Execution.SlippagePerFill
			} else {
				entryPx -= request.Execution.SlippagePerFill
			}
			initialRisk := p.Stop - entryPx
			if p.Side == "long" {
				initialRisk = entryPx - p.Stop
			}
			if !isFinite(initialRisk) {
				return RangeReversionResult{}, fmt.Errorf("nonfinite fill risk at entry row %d", i)
			}
			if r.Policy == dsl.RangeReversionImmediatePolicy {
				// The immediate contract measures risk as the absolute distance to
				// the stop. A fill through the stop remains a real fill and is handled
				// by the entry-bar bracket below; only a zero-distance fill is invalid.
				initialRisk = math.Abs(initialRisk)
			}
			if r.Policy == dsl.RangeReversionImmediatePolicy && initialRisk <= 1e-12 {
				// Keep the historical counter name for report compatibility; these
				// immediate-policy cancellations are zero or near-zero risk fills.
				canceledAtEntryRisk++
				rejectedAtEntry = true
			} else {
				pos = &rrPosition{signal: p, entryIndex: i, entryT: b.t, entry: entryPx, rawEntry: rawEntry, stop: p.Stop, target: p.Target, initialRisk: initialRisk}
				justEntered = true
			}
			// The immediate policy submits its bracket with the entry. If adverse
			// slippage places the fill beyond either bracket level, the order is
			// flattened at the raw open with both sides of execution cost charged.
			// A zero-distance fill is rejected because its fill-relative stop risk
			// is undefined; positive-distance fills through either bracket flatten.
			if pos != nil && r.Policy == dsl.RangeReversionImmediatePolicy {
				pastStop := p.Side == "long" && entryPx <= pos.stop || p.Side == "short" && entryPx >= pos.stop
				pastTarget := p.Side == "long" && entryPx >= pos.target || p.Side == "short" && entryPx <= pos.target
				if pastStop || pastTarget {
					exitReason := "target-gap"
					if pastStop {
						// Stop-first is also the classification when a fill crosses
						// both barriers at once.
						exitReason = "stop-gap"
					}
					exitFill := rawEntry
					if p.Side == "long" {
						exitFill -= request.Execution.SlippagePerFill
					} else {
						exitFill += request.Execution.SlippagePerFill
					}
					gross := (exitFill - entryPx) * request.Execution.Units
					if p.Side == "short" {
						gross = -gross
					}
					commission := request.Execution.CommissionPerUnitSide * request.Execution.Units * 2
					net := gross - commission
					if !isFinite(exitFill) || !isFinite(gross) || !isFinite(net) {
						return RangeReversionResult{}, fmt.Errorf("nonfinite entry-through-bracket result at row %d", i)
					}
					trades = append(trades, RangeReversionTrade{RangeReversionSignal: p, EntryIndex: i, EntryMS: b.t, Entry: entryPx, RawEntry: rawEntry, ExitIndex: i, ExitMS: b.t, Exit: exitFill, RawExit: rawEntry, ExitReason: exitReason, GrossPnL: gross, GrossR: gross / (p.PlannedRisk * request.Execution.Units), CommissionPrice: commission, NetPnL: net, NetR: net / (p.PlannedRisk * request.Execution.Units)})
					pos = nil
					lastExit = i
					exited = true
				}
			}
		}
		if pos != nil {
			if pos.beQueued != 0 {
				pos.stop = pos.beQueued
				pos.beQueued = 0
			}
			reason, price, hit := "", 0., false
			if pos.queuedStop {
				reason, price, hit = "close-latched-stop", b.o, true
			} else if r.Policy == dsl.RangeReversionDelayedPinePolicy && !pos.active {
			} else {
				reason, price, hit = rrExit(pos, b, r.Policy)
			}
			if hit {
				rawExit := price
				exitFill := rawExit
				if pos.signal.Side == "long" {
					exitFill -= request.Execution.SlippagePerFill
				} else {
					exitFill += request.Execution.SlippagePerFill
				}
				gross := (exitFill - pos.entry) * request.Execution.Units
				if pos.signal.Side == "short" {
					gross = -gross
				}
				commission := request.Execution.CommissionPerUnitSide * request.Execution.Units * 2
				net := gross - commission
				if !isFinite(exitFill) || !isFinite(gross) || !isFinite(net) || !(pos.signal.PlannedRisk > 0) || !isFinite(pos.signal.PlannedRisk) {
					return RangeReversionResult{}, fmt.Errorf("nonfinite or invalid trade result at entry row %d", pos.entryIndex)
				}
				trades = append(trades, RangeReversionTrade{RangeReversionSignal: pos.signal, EntryIndex: pos.entryIndex, EntryMS: pos.entryT, Entry: pos.entry, RawEntry: pos.rawEntry, ExitIndex: i, ExitMS: b.t, Exit: exitFill, RawExit: rawExit, ExitReason: reason, GrossPnL: gross, GrossR: gross / (pos.signal.PlannedRisk * request.Execution.Units), CommissionPrice: commission, NetPnL: net, NetR: net / (pos.signal.PlannedRisk * request.Execution.Units)})
				pos = nil
				lastExit = i
				exited = true
			}
			if pos != nil {
				pos.active = true
				if r.BreakEvenEnabled && !pos.beSet {
					favour := b.c - pos.entry
					if pos.signal.Side == "short" {
						favour = pos.entry - b.c
					}
					if favour >= pos.initialRisk*r.BreakEvenTriggerR {
						offset := r.TickSize * float64(r.BreakEvenOffsetTicks)
						if pos.signal.Side == "long" {
							pos.beQueued = pos.entry + offset
						} else {
							pos.beQueued = pos.entry - offset
						}
						pos.beSet = true
					}
				}
				if justEntered && r.Policy == dsl.RangeReversionDelayedPinePolicy && ((pos.signal.Side == "long" && b.c <= pos.stop) || (pos.signal.Side == "short" && b.c >= pos.stop)) {
					pos.queuedStop = true
				}
			}
		}
		if pos != nil || pending != nil || exited || rejectedAtEntry || b.closeT < request.Window.TradeFromMS || b.closeT >= request.Window.TradeToMS || lastExit >= 0 && i-lastExit < r.CooldownBars {
			continue
		}
		var hi, lo float64
		var ok bool
		if r.Bounds == "CHART" {
			hi, lo, ok = rrBounds(entry, i, r.BoundsLookback)
		} else if srcIdx+1 >= r.BoundsLookback {
			hi, lo, ok = rrBounds(source, srcIdx+1, r.BoundsLookback)
		}
		if !ok {
			continue
		}
		// The conceptual policy discards a candle that swept and reclaimed both
		// bounds before applying candle colour or trend gates. Filtering first
		// can turn a true two-sided sweep into an invented one-sided signal.
		if r.Policy == dsl.RangeReversionImmediatePolicy {
			rawShort := b.h > hi && b.c < hi
			rawLong := b.l < lo && b.c > lo
			if rawShort && rawLong {
				continue
			}
		}
		if !atrOK[i] || r.UseVectorGates && (!adxOK[i] || !chopOK[i]) {
			continue
		}
		if r.UseVectorGates && !(adx[i] < r.MaxADX && chop[i] > r.MinCHOP) {
			continue
		}
		if r.UseRangeExpansion && b.h-b.l < atr[i]*r.RangeATRMultiple {
			continue
		}
		var emaValue float64
		if srcIdx >= 0 {
			emaValue = htfEMA[srcIdx]
		}
		if r.UseHTFEMA && srcIdx < 0 {
			continue
		}
		shortSignal := b.h > hi && b.c < hi && (!r.RequireCandleColor || b.c < b.o) && (!r.UseHTFEMA || b.c < emaValue)
		longSignal := b.l < lo && b.c > lo && (!r.RequireCandleColor || b.c > b.o) && (!r.UseHTFEMA || b.c > emaValue)
		if r.Policy == dsl.RangeReversionImmediatePolicy && shortSignal && longSignal {
			continue
		}
		side := ""
		if shortSignal {
			side = "short"
		} else if longSignal {
			side = "long"
		}
		if side == "" {
			continue
		}
		stop := b.h + atr[i]*r.StopATRMultiple
		if side == "long" {
			stop = b.l - atr[i]*r.StopATRMultiple
		}
		risk := math.Abs(b.c - stop)
		target := b.c - risk*r.TargetR
		if side == "long" {
			target = b.c + risk*r.TargetR
		}
		if !(risk > 0 && isFinite(risk) && isFinite(stop) && isFinite(target)) {
			return RangeReversionResult{}, fmt.Errorf("nonfinite or nonpositive planned risk at signal row %d", i)
		}
		sig := RangeReversionSignal{SignalIndex: i, SignalCloseMS: b.closeT, Side: side, Bound: map[bool]float64{true: hi, false: lo}[side == "short"], Stop: stop, Target: target, PlannedRisk: risk}
		signals = append(signals, sig)
		pending = &sig
	}
	if pos != nil {
		censored = true
	}
	winTrades := make([]RangeReversionTrade, 0, len(trades))
	for _, trade := range trades {
		if trade.SignalCloseMS >= request.Window.TradeFromMS && trade.SignalCloseMS < request.Window.TradeToMS {
			winTrades = append(winTrades, trade)
		}
	}
	wins, losses := 0., 0.
	positive, negative := 0., 0.
	for _, t := range winTrades {
		if t.GrossPnL > 0 {
			wins += t.GrossPnL
			positive += t.GrossR
		} else if t.GrossPnL < 0 {
			losses -= t.GrossPnL
			negative -= t.GrossR
		}
	}
	configBytes, _ := json.Marshal(request.Config)
	ch := sha256.Sum256(configBytes)
	hashSeries := func(bs []rrBar) string {
		rows := make([][6]float64, len(bs))
		for i, b := range bs {
			rows[i] = [6]float64{float64(b.t), b.o, b.h, b.l, b.c, b.v}
		}
		sum := sha256.Sum256(mustJSON(rows))
		return hex.EncodeToString(sum[:])
	}
	netWins, netLosses := 0.0, 0.0
	for _, t := range winTrades {
		if t.NetPnL > 0 {
			netWins += t.NetPnL
		} else if t.NetPnL < 0 {
			netLosses -= t.NetPnL
		}
	}
	netPositiveR, netNegativeR := 0.0, 0.0
	for _, t := range winTrades {
		if t.NetR > 0 {
			netPositiveR += t.NetR
		} else if t.NetR < 0 {
			netNegativeR -= t.NetR
		}
	}
	commissionTotal := 0.0
	for _, t := range winTrades {
		commissionTotal += t.CommissionPrice
	}
	summary := map[string]any{"trades": len(winTrades), "grossProfitFactorPrice": ratio(wins, losses), "netProfitFactorPrice": ratio(netWins, netLosses), "grossProfitFactorR": ratio(positive, negative), "netProfitFactorR": ratio(netPositiveR, netNegativeR), "grossPnlPrice": sumTradePnL(winTrades), "netPnlPrice": sumNetPnL(winTrades), "commissionPrice": commissionTotal, "censoredOpen": censored, "canceledAtEntryNonpositiveRisk": canceledAtEntryRisk}
	out := RangeReversionResult{Schema: "strat-range-reversion-reference-v1", Policy: r.Policy, ConfigSHA256: hex.EncodeToString(ch[:]), EntrySHA256: hashSeries(entry), SourceSHA256: hashSeries(source), EffectiveConfig: spec, Execution: request.Execution, Window: request.Window, InputEntryBars: request.EntrySeries.Len(), InputSourceBars: request.SourceSeries.Len(), UsedEntryBars: len(entry), UsedSourceBars: len(source), Signals: signals, Trades: winTrades, CensoredOpen: censored, PendingAtWindowEnd: pending != nil, CanceledAtEntryRisk: canceledAtEntryRisk, Summary: summary, Assumptions: []string{"Fixed-unit model-price P&L; slippage is applied adversely to entry and exit fills and changes fill-relative breakeven; commission is an explicit per-unit, per-side projection. Point value, account sizing and financing are not inferred.", "The source series value is available from the next native 4h row timestamp; the final source row is unavailable without a successor.", "The Pine policy delays bracket activation until after the entry bar and uses nearer-extreme OHLC path; immediate policy activates entry-bar brackets, measures actual risk as the absolute fill-to-stop distance, cancels zero or near-zero distances (at most 1e-12), and resolves simultaneous touches stop-first.", "Only the supplied bars before tradeTo are evaluated; input suffix rows do not enter indicators or execution."}}
	return out, nil
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func ratio(a, b float64) any {
	if b == 0 {
		return nil
	}
	return a / b
}
func sumTradePnL(ts []RangeReversionTrade) float64 {
	x := 0.
	for _, t := range ts {
		x += t.GrossPnL
	}
	return x
}
func sumNetPnL(ts []RangeReversionTrade) float64 {
	x := 0.
	for _, t := range ts {
		x += t.NetPnL
	}
	return x
}
