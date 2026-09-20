package engine

import (
	"math"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

type position struct {
	Side         side
	Entry        float64
	Size         float64
	SL           float64
	TP           float64
	InitialSL    float64
	InitialTP    float64
	Meta         TradeMeta
	EntryIndex   int
	EntryT       float64
	Tag          string
	PartialTaken bool
	GapAwareStop bool
	NoTarget     bool
	NoStop       bool
}

type order struct {
	Side              side
	SL                float64
	TP                float64
	Size              float64
	HasSize           bool
	RiskUSD           float64
	HasRisk           bool
	Tag               string
	Meta              TradeMeta
	Index             int
	Limit             float64
	HasLimit          bool
	NoSlip            bool
	PlacedAt          int
	ExpireAt          int
	ExpireBars        int
	StopDistance      float64
	HasStopDistance   bool
	TargetDistance    float64
	HasTargetDistance bool
	GapAwareStop      bool
	NoTarget          bool
	NoStop            bool
}

type pendingExit struct {
	PositionEntryIndex, Index int
	Reason                    string
}

type broker struct {
	series                 marketdata.Series
	cols                   contextcols.Columns
	htfTrend               []int8
	ema                    []float64
	emaSlope               []float64
	costs                  Costs
	params                 flagParams
	fixture                RunFixture
	position               position
	hasPosition            bool
	pendingOrders          []order
	pendingExits           []pendingExit
	limitOrders            []order
	trades                 []Trade
	realized               float64
	flagLastEntry          int
	hasFlagEntry           bool
	rbfLastEntry           int
	hasRBFEntry            bool
	orbLastEntry           int
	hasORBEntry            bool
	ideLastEntry           int
	hasIDEEntry            bool
	dorLastEntry           int
	hasDOREntry            bool
	brLastEntry            int
	hasBREntry             bool
	sbhLastEntry           int
	hasSBHEntry            bool
	tpbLastEntry           int
	hasTPBEntry            bool
	tpbAttemptsLong        int
	tpbAttemptsShort       int
	tpbLastAttemptLong     int
	tpbLastAttemptShort    int
	hasTPBLastAttemptLong  bool
	hasTPBLastAttemptShort bool
	tpbVWAPSessionKey      string
	tpbVWAPTouchCount      int
	tpbVWAPFirstTouch      int
	sdZones                []sdZone
	dtbHighs               []dtbPivot
	dtbLows                []dtbPivot
	dtbSeen                []string
	dtbCooldown            int
	hasDTBCool             bool
	fibLastEntry           int
	hasFibEntry            bool
	cbhLast                int
	hasCBHLast             bool
	lsLastEntry            int
	hasLSEntry             bool
	tpeHighs               []tpePivot
	tpeLows                []tpePivot
	tpeLastConfirmed       int
	tpeLastEntry           int
	hasTPEEntry            bool
	vefLast                int
	hasVEFLast             bool
	vaeLast                int
	hasVAELast             bool
	elderLastEntry         int
	hasElderEntry          bool
	priceMomentumLastEntry int
	hasPriceMomentumEntry  bool
	fvgLastEntry           int
	hasFVGEntry            bool
	fvgZones               []fvgZone
	keltner                keltnerState
	seen                   dailySeenSets
	captureEntries         bool
	capturedEntries        []order
	lastExitIndex          int
	dualFast               float64
	dualSlow               float64
	dualATR                float64
	dualPreviousClose      float64
	dualPreviousFast       float64
	dualInitialized        bool
	dualSlowHistory        []float64
	dualPositionEntry      int
	dualHasPositionEntry   bool
	dualBestClose          float64
	dualExitPending        bool
	sma                    smaGoldenCrossState
	execution              ExecutionBounds
	windowed               bool
}

func (b *broker) reset(series marketdata.Series, cols contextcols.Columns, htfTrend []int8, ema []float64, emaSlope []float64, params flagParams, fixture RunFixture, trades []Trade) {
	b.series = series
	b.cols = cols
	b.htfTrend = htfTrend
	b.ema = ema
	b.emaSlope = emaSlope
	b.costs = fixture.Costs.normalized()
	b.params = params
	b.fixture = fixture
	b.hasPosition = false
	b.pendingOrders = b.pendingOrders[:0]
	b.pendingExits = b.pendingExits[:0]
	b.limitOrders = b.limitOrders[:0]
	b.trades = trades[:0]
	b.realized = 0
	b.flagLastEntry = 0
	b.hasFlagEntry = false
	b.rbfLastEntry = 0
	b.hasRBFEntry = false
	b.orbLastEntry = 0
	b.hasORBEntry = false
	b.ideLastEntry = 0
	b.hasIDEEntry = false
	b.dorLastEntry = 0
	b.hasDOREntry = false
	b.brLastEntry = 0
	b.hasBREntry = false
	b.sbhLastEntry = 0
	b.hasSBHEntry = false
	b.tpbLastEntry = 0
	b.hasTPBEntry = false
	b.tpbAttemptsLong = 0
	b.tpbAttemptsShort = 0
	b.tpbLastAttemptLong = 0
	b.tpbLastAttemptShort = 0
	b.hasTPBLastAttemptLong = false
	b.hasTPBLastAttemptShort = false
	b.tpbVWAPSessionKey = ""
	b.tpbVWAPTouchCount = 0
	b.tpbVWAPFirstTouch = -1
	b.sdZones = b.sdZones[:0]
	b.dtbHighs = b.dtbHighs[:0]
	b.dtbLows = b.dtbLows[:0]
	b.dtbSeen = b.dtbSeen[:0]
	b.dtbCooldown = 0
	b.hasDTBCool = false
	b.fibLastEntry = 0
	b.hasFibEntry = false
	b.cbhLast = 0
	b.hasCBHLast = false
	b.lsLastEntry = 0
	b.hasLSEntry = false
	b.tpeHighs = b.tpeHighs[:0]
	b.tpeLows = b.tpeLows[:0]
	b.tpeLastConfirmed = -1
	b.tpeLastEntry = 0
	b.hasTPEEntry = false
	b.vefLast = 0
	b.hasVEFLast = false
	b.vaeLast = 0
	b.hasVAELast = false
	b.elderLastEntry = 0
	b.hasElderEntry = false
	b.priceMomentumLastEntry = 0
	b.hasPriceMomentumEntry = false
	b.fvgLastEntry = 0
	b.hasFVGEntry = false
	b.fvgZones = b.fvgZones[:0]
	b.keltner.reset()
	b.seen.reset()
	b.captureEntries = false
	b.capturedEntries = b.capturedEntries[:0]
	b.lastExitIndex = -1
	b.dualInitialized = false
	b.dualSlowHistory = b.dualSlowHistory[:0]
	b.dualHasPositionEntry = false
	b.dualExitPending = false
	b.sma = smaGoldenCrossState{}
	b.windowed = false
	b.execution = ExecutionBounds{}
}

func (b *broker) setExecutionWindow(window ExecutionBounds) {
	b.execution = window
	b.windowed = true
}

func (b *broker) executionStart() int {
	if !b.windowed {
		return 0
	}
	return b.execution.TradeStart
}

func (b *broker) executionEnd() int {
	if !b.windowed {
		return b.series.Len() - 1
	}
	return b.execution.TradeEnd
}

func (b *broker) executionIndexAllowed(index int) bool {
	return index >= b.executionStart() && index <= b.executionEnd()
}

func (b *broker) clearExecutionOrders() {
	b.pendingOrders = b.pendingOrders[:0]
	b.pendingExits = b.pendingExits[:0]
	b.limitOrders = b.limitOrders[:0]
}

func (b *broker) run() []Trade {
	if trades, handled := b.runSpecialSetup(); handled {
		return trades
	}
	n := b.series.Len()
	end := b.executionEnd()
	if end >= n {
		end = n - 1
	}
	for i := 0; i <= end; i++ {
		if b.windowed && i < b.executionStart() {
			b.clearExecutionOrders()
		}
		b.fillPendingExits(i)
		b.fillPending(i)
		b.fillLimits(i)
		b.closeExpiredWindowPosition(i)
		b.resolveIntrabarExit(i)
		b.onBar(i)
		if b.windowed && i < b.executionStart() {
			b.clearExecutionOrders()
		}
	}
	if n > 0 && end >= 0 && b.hasPosition {
		b.closePosition(b.series.C[end], end, "eod")
	}
	return b.trades
}

// runCapturedSource evaluates the ordinary parsed setup on source candles but
// captures its admitted orders instead of opening source-timeframe positions.
// C5 later dispatches those exact orders through the chart broker.
func (b *broker) runCapturedSource() []order {
	b.captureEntries = true
	for i := 0; i < b.series.Len(); i++ {
		b.onBar(i)
	}
	b.captureEntries = false
	return append([]order(nil), b.capturedEntries...)
}

func (b *broker) runScheduled(entries []ScheduledEntry, orders []order) []Trade {
	byChart := make(map[int][]order, len(entries))
	for index, entry := range entries {
		if index < len(orders) {
			byChart[entry.ChartIndex] = append(byChart[entry.ChartIndex], orders[index])
		}
	}
	end := b.executionEnd()
	if end >= b.series.Len() {
		end = b.series.Len() - 1
	}
	for i := 0; i <= end; i++ {
		if b.windowed && i < b.executionStart() {
			b.clearExecutionOrders()
		}
		b.fillPending(i)
		b.fillLimits(i)
		b.closeExpiredWindowPosition(i)
		b.resolveIntrabarExit(i)
		for _, captured := range byChart[i] {
			b.dispatchCaptured(i, captured)
		}
		if b.windowed && i < b.executionStart() {
			b.clearExecutionOrders()
		}
	}
	if end >= 0 && b.hasPosition {
		b.closePosition(b.series.C[end], end, "eod")
	}
	return b.trades
}

func (b *broker) fillPending(i int) {
	for k := 0; k < len(b.pendingOrders); {
		ord := b.pendingOrders[k]
		if ord.Index > i {
			k++
			continue
		}
		b.pendingOrders = append(b.pendingOrders[:k], b.pendingOrders[k+1:]...)
		b.openPosition(ord.Side, b.series.O[i], ord, i)
	}
}

func (b *broker) fillLimits(i int) {
	for k := len(b.limitOrders) - 1; k >= 0; k-- {
		ord := b.limitOrders[k]
		if i <= ord.PlacedAt {
			continue
		}
		fillable := (ord.Side == sideLong && b.series.L[i] <= ord.Limit) ||
			(ord.Side == sideShort && b.series.H[i] >= ord.Limit)
		if fillable {
			gapped := (ord.Side == sideLong && b.series.O[i] < ord.Limit) ||
				(ord.Side == sideShort && b.series.O[i] > ord.Limit)
			fill := ord.Limit
			if gapped {
				fill = b.series.O[i]
			}
			b.limitOrders = append(b.limitOrders[:k], b.limitOrders[k+1:]...)
			ord.NoSlip = true
			b.openPosition(ord.Side, fill, ord, i)
			continue
		}
		if i >= ord.ExpireAt {
			b.limitOrders = append(b.limitOrders[:k], b.limitOrders[k+1:]...)
		}
	}
}

func (b *broker) enter(i int, side side, setup flagSetup) {
	if b.hasPosition || len(b.pendingOrders) > 0 || len(b.limitOrders) > 0 {
		return
	}
	if b.windowed && !b.executionIndexAllowed(i) {
		return
	}
	if !b.guardedEntryAllowed(i) || !b.marketNonSessionGatesOK(i) || !b.guardedCandleQualityOK(i, side) {
		return
	}
	theme, ok := b.dayThemeAdmission(i, side)
	if !ok {
		return
	}
	if !b.typedEntryDistanceOK(i, setup.Meta) {
		return
	}
	setup.Meta = annotateDayTheme(setup.Meta, theme)
	ord := order{
		Side:       side,
		SL:         setup.Stop,
		TP:         setup.Target,
		RiskUSD:    b.params.RiskUSD,
		HasRisk:    true,
		Tag:        "DSL-FLAG",
		Meta:       setup.Meta,
		Index:      i,
		ExpireBars: 5,
	}
	if b.captureEntries {
		b.capturedEntries = append(b.capturedEntries, ord)
		return
	}
	if b.costs.FillOn == "nextOpen" {
		ord.Index = i + 1
		b.pendingOrders = append(b.pendingOrders, ord)
		return
	}
	b.openPosition(side, b.series.C[i], ord, i)
}

func (b *broker) dispatchCaptured(i int, captured order) {
	if b.hasPosition || len(b.pendingOrders) > 0 || len(b.limitOrders) > 0 {
		return
	}
	captured.Index = i
	if captured.HasLimit {
		// The source timeframe identifies the area. The lower-timeframe broker
		// owns the pending order, so its placement and expiry are counted from
		// this chart bar rather than from the source-bar index.
		captured.PlacedAt = i
		captured.ExpireAt = i + captured.ExpireBars
		b.limitOrders = append(b.limitOrders, captured)
		return
	}
	if b.costs.FillOn == "nextOpen" {
		captured.Index = i + 1
		b.pendingOrders = append(b.pendingOrders, captured)
		return
	}
	b.openPosition(captured.Side, b.series.C[i], captured, i)
}

func (b *broker) enterLimit(i int, limit float64, setup setupPlan, expireBars int) bool {
	if b.hasPosition || len(b.pendingOrders) > 0 || len(b.limitOrders) > 0 {
		return false
	}
	if b.windowed && !b.executionIndexAllowed(i) {
		return false
	}
	if !b.guardedEntryAllowed(i) || !b.guardedCandleQualityOK(i, setup.Side) {
		return false
	}
	theme, ok := b.dayThemeAdmission(i, setup.Side)
	if !ok {
		return false
	}
	setup.Meta = annotateDayTheme(setup.Meta, theme)
	ord := order{
		Side:       setup.Side,
		SL:         setup.Stop,
		TP:         setup.Target,
		RiskUSD:    b.params.RiskUSD,
		HasRisk:    true,
		Tag:        setup.Tag,
		Meta:       setup.Meta,
		Index:      i,
		Limit:      limit,
		HasLimit:   true,
		PlacedAt:   i,
		ExpireAt:   i + expireBars,
		ExpireBars: expireBars,
	}
	if b.captureEntries {
		b.capturedEntries = append(b.capturedEntries, ord)
		return true
	}
	b.limitOrders = append(b.limitOrders, ord)
	return true
}

func (b *broker) guardedEntryAllowed(i int) bool {
	return inAdmittedTradeWindow(b.series.T[i], b.params, 0)
}

func (b *broker) openPosition(s side, fillPrice float64, ord order, index int) {
	if b.windowed && !b.executionIndexAllowed(index) {
		return
	}
	sign := float64(s)
	px := fillPrice
	if !ord.NoSlip {
		px += sign * b.slippageAt(fillPrice)
	}
	if ord.HasStopDistance {
		ord.SL = px - sign*ord.StopDistance
	}
	if ord.HasTargetDistance {
		ord.TP = px + sign*ord.TargetDistance
	}
	size := ord.Size
	hasSize := ord.HasSize || size != 0
	hasRisk := ord.HasRisk || ord.RiskUSD != 0
	if !hasSize && hasRisk {
		dist := math.Abs(px - ord.SL)
		if dist > 0 {
			size = ord.RiskUSD / dist
		}
	}
	if !hasSize && !hasRisk {
		size = 1
	}
	b.position = position{
		Side:         s,
		Entry:        px,
		Size:         size,
		SL:           ord.SL,
		TP:           ord.TP,
		InitialSL:    ord.SL,
		InitialTP:    ord.TP,
		Meta:         ord.Meta,
		EntryIndex:   index,
		EntryT:       b.series.T[index],
		Tag:          ord.Tag,
		GapAwareStop: ord.GapAwareStop,
		NoTarget:     ord.NoTarget,
		NoStop:       ord.NoStop,
	}
	b.hasPosition = true
	b.realized -= b.costs.FeePerUnit * size
}

func (b *broker) closePosition(exitPrice float64, index int, reason string) {
	pos := b.position
	sign := float64(pos.Side)
	px := exitPrice - sign*b.slippageAt(exitPrice)
	points := (px - pos.Entry) * sign
	pnl := points*pos.Size - b.costs.FeePerUnit*pos.Size
	b.realized += pnl
	b.trades = append(b.trades, Trade{
		Side:       pos.Side.String(),
		Entry:      pos.Entry,
		Exit:       px,
		SL:         pos.SL,
		TP:         pos.TP,
		InitialSL:  pos.InitialSL,
		InitialTP:  pos.InitialTP,
		Meta:       pos.Meta,
		Size:       pos.Size,
		EntryIndex: pos.EntryIndex,
		ExitIndex:  index,
		EntryT:     pos.EntryT,
		ExitT:      b.series.T[index],
		Points:     points,
		PnL:        pnl,
		Reason:     reason,
		Tag:        pos.Tag,
		NoTarget:   pos.NoTarget,
		NoStop:     pos.NoStop,
	})
	for k := len(b.pendingExits) - 1; k >= 0; k-- {
		if b.pendingExits[k].PositionEntryIndex == pos.EntryIndex {
			b.pendingExits = append(b.pendingExits[:k], b.pendingExits[k+1:]...)
		}
	}
	b.lastExitIndex = index
	b.hasPosition = false
}

func (b *broker) tightenStop(price float64) {
	if !b.hasPosition {
		return
	}
	if b.position.Side == sideLong {
		if price > b.position.SL {
			b.position.SL = price
		}
		return
	}
	if price < b.position.SL {
		b.position.SL = price
	}
}
