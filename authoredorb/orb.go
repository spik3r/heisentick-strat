// Package authoredorb ports the authored JavaScript Day Session ORB strategy.
// Version 1 implements both the archived base ID and its active 30m parity ID.
// It is independent of the Strat DSL family runner.
package authoredorb

import (
	"errors"
	"fmt"
	"math"

	"github.com/spik3r/heisentick-strat/marketdata"
)

const (
	Schema           = "authored-orb-run-v1"
	OriginalTVParity = "daySessionOrbOriginalTvParity"
	ArchivedBase     = "daySessionOrbStrategy"
	dayMS            = 86400000.0
	localOffsetMS    = 36000000.0
)

type Session struct {
	Key        string `json:"key"`
	Start, End int
}

var Sessions = [...]Session{
	{"Early 11-15 AEST", 660, 900}, {"Mid 12-16 AEST", 720, 960},
	{"Late 13-17 AEST", 780, 1020}, {"After 14-18 AEST", 840, 1080},
	{"London 16-19 AEST", 960, 1140}, {"London 17-21 AEST", 1020, 1260},
	{"London 18-22 AEST", 1080, 1320}, {"London-NY 20-24 AEST", 1200, 1440},
	{"NY 21-23 AEST", 1260, 1380}, {"NY 22-24 AEST", 1320, 1440},
	{"PM 16-21 AEST", 960, 1260}, {"Day 09-21 AEST", 540, 1260},
}

var styleLabels = [...]string{"ORB", "VWAP Reclaim", "Trend Pullback"}

// Params matches the numeric JavaScript params object. Omitted entries use
// the authored defaults; explicit zero values remain available through map.
type Params map[string]float64

func defaults() Params {
	return Params{
		"sessionPreset": 1, "strategyStyle": 0, "direction": 0, "orbMinutes": 60,
		"emaFastLen": 20, "emaSlowLen": 100, "useVwap": 1, "atrLen": 14,
		"atrMult": 1.5, "rr": 2, "oneTradePerDay": 1, "riskUsd": 200,
	}
}

type Costs struct {
	Slippage    float64  `json:"slippage"`
	SlippageBps float64  `json:"slippageBps"`
	FeePerUnit  float64  `json:"feePerUnit"`
	FillOn      string   `json:"fillOn"`
	StartEquity *float64 `json:"startEquity,omitempty"`
}

type Request struct {
	Schema     string           `json:"schema"`
	StrategyID string           `json:"strategyId"`
	Symbol     string           `json:"symbol"`
	Timeframe  string           `json:"timeframe"`
	Params     Params           `json:"params,omitempty"`
	Costs      Costs            `json:"costs"`
	Bars       []marketdata.Bar `json:"bars"`
	// ContextVWAP, when supplied, represents api.ctx.vwap. A null or
	// non-finite value selects the authored HLC3 fallback. Without a column,
	// Run computes the ordinary JS context's UTC-day close-volume VWAP.
	ContextVWAP []*float64 `json:"contextVwap,omitempty"`
	ForceRoute  bool       `json:"forceRoute,omitempty"`
	CloseAtEnd  *bool      `json:"closeAtEnd,omitempty"`
}

type Trade struct {
	PositionID string            `json:"positionId"`
	Side       string            `json:"side"`
	Entry      float64           `json:"entry"`
	Exit       float64           `json:"exit"`
	SL         float64           `json:"sl"`
	TP         float64           `json:"tp"`
	InitialSL  float64           `json:"initialSl"`
	InitialTP  float64           `json:"initialTp"`
	Size       float64           `json:"size"`
	EntryIndex int               `json:"entryIndex"`
	ExitIndex  int               `json:"exitIndex"`
	EntryT     float64           `json:"entryT"`
	ExitT      float64           `json:"exitT"`
	Points     float64           `json:"points"`
	PnL        float64           `json:"pnl"`
	Reason     string            `json:"reason"`
	Tag        string            `json:"tag"`
	Meta       map[string]string `json:"meta"`
}

type OpenPosition struct {
	PositionID string            `json:"positionId"`
	Side       string            `json:"side"`
	Entry      float64           `json:"entry"`
	SL         float64           `json:"sl"`
	TP         float64           `json:"tp"`
	Size       float64           `json:"size"`
	EntryIndex int               `json:"entryIndex"`
	EntryT     float64           `json:"entryT"`
	Tag        string            `json:"tag"`
	Meta       map[string]string `json:"meta"`
}

type Result struct {
	Schema        string         `json:"schema"`
	StrategyID    string         `json:"strategyId"`
	Symbol        string         `json:"symbol"`
	Timeframe     string         `json:"timeframe"`
	Trades        []Trade        `json:"trades"`
	EquityCurve   []float64      `json:"equityCurve"`
	OpenPositions []OpenPosition `json:"openPositions,omitempty"`
	// Interactive accounting is available to Go adapters without changing the
	// historical authored-orb-run-v1 JSON envelope or its parity fixtures.
	ClosedEquity  []float64 `json:"-"`
	CashEndEquity float64   `json:"-"`
}

type position struct {
	OpenPosition
	initialSL, initialTP float64
}
type intent struct {
	side         string
	sl, tp, risk float64
	tag          string
	meta         map[string]string
}
type runner struct {
	req                                             Request
	params                                          Params
	result                                          Result
	pos                                             *position
	pending                                         *intent
	nextPositionID                                  int
	realized                                        float64
	dayKey, utcDayKey                               float64
	dayVwapSum, dayVwapBars, utcVwapNum, utcVwapDen float64
	emaFast, emaSlow, atr, atrSeedSum, lastClose    float64
	atrCount                                        int
	orbHigh, orbLow                                 float64
	tradedToday, inSession                          bool
}

func routeAllowed(id, symbol, tf string) bool {
	if symbol != "XAUUSD" {
		return false
	}
	if id == OriginalTVParity {
		return tf == "30m"
	}
	return tf == "5m" || tf == "15m"
}

// PreferredRoute exposes authored route metadata for catalog adapters.
func PreferredRoute(id string) (string, []string, error) {
	switch id {
	case OriginalTVParity:
		return "XAUUSD", []string{"30m"}, nil
	case ArchivedBase:
		return "XAUUSD", []string{"5m", "15m"}, nil
	default:
		return "", nil, fmt.Errorf("unknown authored strategy %q", id)
	}
}

func Run(req Request) (Result, error) {
	if req.Schema != Schema {
		return Result{}, fmt.Errorf("schema %q, want %q", req.Schema, Schema)
	}
	if _, _, err := PreferredRoute(req.StrategyID); err != nil {
		return Result{}, err
	}
	if !routeAllowed(req.StrategyID, req.Symbol, req.Timeframe) && !req.ForceRoute {
		return Result{}, fmt.Errorf("route %s %s is outside %s preferred route", req.Symbol, req.Timeframe, req.StrategyID)
	}
	if len(req.Bars) == 0 {
		return Result{}, errors.New("bars are empty")
	}
	if req.ContextVWAP != nil && len(req.ContextVWAP) != len(req.Bars) {
		return Result{}, errors.New("contextVwap length must match bars")
	}
	if req.Costs.FillOn != "" && req.Costs.FillOn != "close" && req.Costs.FillOn != "nextOpen" {
		return Result{}, fmt.Errorf("unsupported fillOn %q", req.Costs.FillOn)
	}
	for name, value := range map[string]float64{"slippage": req.Costs.Slippage, "slippageBps": req.Costs.SlippageBps, "feePerUnit": req.Costs.FeePerUnit} {
		if !finite(value) || value < 0 {
			return Result{}, fmt.Errorf("costs.%s must be finite and nonnegative", name)
		}
	}
	startEquity := 10000.0
	if req.Costs.StartEquity != nil {
		startEquity = *req.Costs.StartEquity
	}
	if !finite(startEquity) || startEquity < 0 {
		return Result{}, errors.New("costs.startEquity must be finite and nonnegative")
	}
	for i, b := range req.Bars {
		if !finite(b.T) || !finite(b.O) || !finite(b.H) || !finite(b.L) || !finite(b.C) ||
			!finite(b.V) || b.V < 0 || b.H < b.L || b.O > b.H || b.O < b.L || b.C > b.H || b.C < b.L ||
			(i > 0 && b.T <= req.Bars[i-1].T) {
			return Result{}, fmt.Errorf("bars[%d] has invalid OHLCV/time ordering", i)
		}
	}
	p := defaults()
	for k, v := range req.Params {
		if _, ok := p[k]; !ok {
			return Result{}, fmt.Errorf("unknown authored ORB param %q", k)
		}
		if !finite(v) {
			return Result{}, fmt.Errorf("param %q must be finite", k)
		}
		p[k] = v
	}
	if p["riskUsd"] < 0 {
		return Result{}, errors.New("param riskUsd must be nonnegative")
	}
	if !finite(p["atrLen"]) || p["atrLen"] < 1 {
		p["atrLen"] = 14
	}
	r := runner{req: req, params: p, result: Result{Schema: Schema, StrategyID: req.StrategyID, Symbol: req.Symbol, Timeframe: req.Timeframe, Trades: make([]Trade, 0), EquityCurve: make([]float64, len(req.Bars)), ClosedEquity: make([]float64, len(req.Bars))}, orbHigh: math.NaN(), orbLow: math.NaN(), atr: math.NaN()}
	for i, b := range req.Bars {
		// buildContext computes UTC-day VWAP for every bar, even while the
		// defineStrategy wrapper pauses this strategy's onBar state.
		r.updateContextVWAP(i, b)
		if r.pending != nil {
			next := r.pending
			r.pending = nil
			r.open(i, b.O, *next)
		}
		r.checkBracket(i, b)
		// defineStrategy does not call the authored onBar while a position
		// survives bracket processing. Its session-close exit is therefore
		// unreachable for an open position in this v1 compatibility port.
		if r.pos == nil {
			r.onBar(i, b)
		}
		unrealized := 0.0
		if r.pos != nil {
			unrealized = (b.C - r.pos.Entry) * sign(r.pos.Side) * r.pos.Size
		}
		r.result.EquityCurve[i] = startEquity + r.realized + unrealized
		r.result.ClosedEquity[i] = startEquity + r.realized
	}
	if req.CloseAtEnd == nil || *req.CloseAtEnd {
		i := len(req.Bars) - 1
		r.close(i, req.Bars[i].C, "eod")
	} else if r.pos != nil {
		r.result.OpenPositions = []OpenPosition{r.pos.OpenPosition}
	}
	r.result.CashEndEquity = startEquity + r.realized
	return r.result, nil
}

func (r *runner) updateContextVWAP(i int, b marketdata.Bar) {
	utcDay := math.Floor(b.T / dayMS)
	if i == 0 || utcDay != r.utcDayKey {
		r.utcDayKey = utcDay
		r.utcVwapNum = 0
		r.utcVwapDen = 0
	}
	r.utcVwapNum += b.C * b.V
	r.utcVwapDen += b.V
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func sign(side string) float64 {
	if side == "long" {
		return 1
	}
	return -1
}
func truth(v float64) bool { return v != 0 && !math.IsNaN(v) }
func numOr(v, fallback float64) float64 {
	if !finite(v) || v == 0 {
		return fallback
	}
	return v
}
func round(v float64) float64                 { return math.Floor(v + 0.5) } // Math.round for finite JavaScript numbers
func clamp(v, lo, hi float64) float64         { return math.Max(lo, math.Min(hi, v)) }
func ema(prev, value, length float64) float64 { return prev + (2/(math.Max(1, length)+1))*(value-prev) }

func (r *runner) slip(price float64) float64 {
	return r.req.Costs.Slippage + math.Abs(price)*r.req.Costs.SlippageBps/10000
}
func (r *runner) open(i int, price float64, o intent) {
	px := price + sign(o.side)*r.slip(price)
	size := 0.0
	dist := math.Abs(px - o.sl)
	if dist > 0 {
		size = o.risk / dist
	}
	r.pos = &position{OpenPosition: OpenPosition{PositionID: fmt.Sprintf("position-%d-%d", i, r.nextPositionID), Side: o.side, Entry: px, SL: o.sl, TP: o.tp, Size: size, EntryIndex: i, EntryT: r.req.Bars[i].T, Tag: o.tag, Meta: o.meta}, initialSL: o.sl, initialTP: o.tp}
	r.nextPositionID++
	r.realized -= r.req.Costs.FeePerUnit * size
}
func (r *runner) enter(i int, o intent) {
	if r.pos != nil || r.pending != nil {
		return
	}
	if r.req.Costs.FillOn == "nextOpen" {
		r.pending = &o
		return
	}
	r.open(i, r.req.Bars[i].C, o)
}
func (r *runner) close(i int, price float64, reason string) {
	if r.pos == nil {
		return
	}
	p := r.pos
	px := price - sign(p.Side)*r.slip(price)
	points := (px - p.Entry) * sign(p.Side)
	pnl := points*p.Size - r.req.Costs.FeePerUnit*p.Size
	r.realized += pnl
	r.result.Trades = append(r.result.Trades, Trade{PositionID: p.PositionID, Side: p.Side, Entry: p.Entry, Exit: px, SL: p.SL, TP: p.TP, InitialSL: p.initialSL, InitialTP: p.initialTP, Size: p.Size, EntryIndex: p.EntryIndex, ExitIndex: i, EntryT: p.EntryT, ExitT: r.req.Bars[i].T, Points: points, PnL: pnl, Reason: reason, Tag: p.Tag, Meta: p.Meta})
	r.pos = nil
}
func (r *runner) checkBracket(i int, b marketdata.Bar) {
	if r.pos == nil {
		return
	}
	p := r.pos
	s := sign(p.Side)
	hitSL := (s > 0 && b.L <= p.SL) || (s < 0 && b.H >= p.SL)
	hitTP := (s > 0 && b.H >= p.TP) || (s < 0 && b.L <= p.TP)
	carried := p.EntryIndex < i
	if carried && ((s > 0 && b.O <= p.SL) || (s < 0 && b.O >= p.SL)) {
		r.close(i, b.O, "sl")
		return
	}
	if hitSL {
		price := p.SL
		if carried {
			if s > 0 {
				price = math.Min(b.O, p.SL)
			} else {
				price = math.Max(b.O, p.SL)
			}
		}
		r.close(i, price, "sl")
		return
	}
	if hitTP {
		r.close(i, p.TP, "tp")
	}
}

func (r *runner) onBar(i int, b marketdata.Bar) {
	p := r.params
	localDay := math.Floor((b.T + localOffsetMS) / dayMS)
	if i == 0 {
		r.dayKey = localDay
		r.emaFast = b.C
		r.emaSlow = b.C
		r.lastClose = b.C
	}
	if i == 0 || localDay != r.dayKey {
		r.dayKey = localDay
		r.dayVwapSum = (b.H + b.L + b.C) / 3
		r.dayVwapBars = 1
		if i == 0 {
			// The JS initializer seeds this value, then its same-day branch
			// adds the first bar again before any signal is evaluated.
			r.dayVwapSum *= 2
			r.dayVwapBars = 2
		}
		r.orbHigh = math.NaN()
		r.orbLow = math.NaN()
		r.tradedToday = false
	} else {
		r.dayVwapSum += (b.H + b.L + b.C) / 3
		r.dayVwapBars++
	}
	minute := int(math.Floor(math.Mod((b.T+localOffsetMS)/60000, 1440)))
	preset := 1
	if v := p["sessionPreset"]; finite(v) && v == math.Trunc(v) && v >= 0 && v < float64(len(Sessions)) {
		preset = int(v)
	}
	session := Sessions[preset]
	orbMinutes := int(clamp(round(numOr(p["orbMinutes"], 60)), 15, 120))
	atrLen := int(math.Max(1, round(numOr(p["atrLen"], 14))))
	rr := numOr(p["rr"], 0)
	atrMult := numOr(p["atrMult"], 0)
	inSession := minute >= session.Start && minute < session.End
	justClosed := r.inSession && !inSession
	orbReady := inSession && minute >= session.Start+orbMinutes
	inBuild := inSession && minute < session.Start+orbMinutes
	if justClosed {
		r.close(i, b.C, "session close")
	}
	r.emaFast = ema(r.emaFast, b.C, p["emaFastLen"])
	r.emaSlow = ema(r.emaSlow, b.C, p["emaSlowLen"])
	tr := math.Max(b.H-b.L, math.Max(math.Abs(b.H-r.lastClose), math.Abs(b.L-r.lastClose)))
	r.lastClose = b.C
	r.atrCount++
	if !finite(r.atr) {
		r.atrSeedSum += tr
		r.atr = r.atrSeedSum / math.Max(1, math.Min(float64(r.atrCount), float64(atrLen)))
	} else {
		r.atr = (r.atr*float64(atrLen-1) + tr) / float64(atrLen)
	}
	if inBuild {
		if !finite(r.orbHigh) {
			r.orbHigh = b.H
		}
		if !finite(r.orbLow) {
			r.orbLow = b.L
		}
		r.orbHigh = math.Max(r.orbHigh, b.H)
		r.orbLow = math.Min(r.orbLow, b.L)
	}
	r.inSession = inSession
	if r.pos != nil || !inSession || r.dayVwapBars == 0 {
		return
	}
	if truth(p["oneTradePerDay"]) && r.tradedToday {
		return
	}
	if !finite(r.emaFast) || !finite(r.emaSlow) || !finite(r.orbHigh) || !finite(r.orbLow) || !orbReady || r.atr <= 0 || atrMult <= 0 || rr <= 0 {
		return
	}
	vwap := r.dayVwapSum / r.dayVwapBars
	if r.req.ContextVWAP != nil {
		if value := r.req.ContextVWAP[i]; value != nil && finite(*value) {
			vwap = *value
		}
	} else if r.utcVwapDen > 0 {
		vwap = r.utcVwapNum / r.utcVwapDen
	}
	trendUp := r.emaFast > r.emaSlow && (!truth(p["useVwap"]) || b.C > vwap)
	trendDown := r.emaFast < r.emaSlow && (!truth(p["useVwap"]) || b.C < vwap)
	style := int(clamp(round(numOr(p["strategyStyle"], 0)), 0, 2))
	allowLong, allowShort := true, true
	if direction := p["direction"]; finite(direction) && direction == math.Trunc(direction) {
		switch direction {
		case 0:
			allowLong = false
		case 1:
			allowShort = false
		}
	}
	longSignal, shortSignal := false, false
	switch style {
	case 0:
		longSignal = b.C > r.orbHigh && trendUp
		shortSignal = b.C < r.orbLow && trendDown
	case 1:
		longSignal = b.C > vwap && b.C > r.emaFast && trendUp
		shortSignal = b.C < vwap && b.C < r.emaFast && trendDown
	case 2:
		longSignal = b.L <= r.emaFast && b.C > r.emaFast && trendUp
		shortSignal = b.H >= r.emaFast && b.C < r.emaFast && trendDown
	}
	side := ""
	if allowLong && longSignal {
		side = "long"
	} else if allowShort && shortSignal {
		side = "short"
	}
	if side == "" {
		return
	}
	r.tradedToday = true
	stop, target := b.C-r.atr*atrMult, b.C+r.atr*atrMult*rr
	if side == "short" {
		stop = b.C + r.atr*atrMult
		target = b.C - r.atr*atrMult*rr
	}
	label := styleLabels[style]
	directionLabel := "LONG"
	if side == "short" {
		directionLabel = "SHORT"
	}
	r.enter(i, intent{side: side, sl: stop, tp: target, risk: p["riskUsd"], tag: fmt.Sprintf("DaySessionOrb:%s:%s:%s", session.Key, directionLabel, label), meta: map[string]string{"strategy": "daySessionOrb", "style": label, "session": session.Key, "setup": "ORB", "entryStyle": "signal"}})
}
