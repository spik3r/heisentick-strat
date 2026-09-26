package engine

import (
	"math"
	"strings"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

type setupPlan struct {
	Side   side
	Stop   float64
	Target float64
	Tag    string
	Meta   TradeMeta
}

type dailySeenSet struct {
	day  int64
	keys []string
}

type dailySeenSets struct {
	rbf dailySeenSet
	orb dailySeenSet
	ide dailySeenSet
	dor dailySeenSet
	br  dailySeenSet
	sbh dailySeenSet
	cbh dailySeenSet
	ls  dailySeenSet
	vef dailySeenSet
	vae dailySeenSet
	nls dailySeenSet
}

func (s *dailySeenSets) reset() {
	*s = dailySeenSets{
		rbf: dailySeenSet{day: math.MinInt64},
		orb: dailySeenSet{day: math.MinInt64},
		ide: dailySeenSet{day: math.MinInt64},
		dor: dailySeenSet{day: math.MinInt64},
		br:  dailySeenSet{day: math.MinInt64},
		sbh: dailySeenSet{day: math.MinInt64},
		cbh: dailySeenSet{day: math.MinInt64},
		ls:  dailySeenSet{day: math.MinInt64},
		vef: dailySeenSet{day: math.MinInt64},
		vae: dailySeenSet{day: math.MinInt64},
		nls: dailySeenSet{day: math.MinInt64},
	}
}

func (s *dailySeenSet) seen(day int64, key string) bool {
	if s.day != day {
		s.day = day
		s.keys = s.keys[:0]
	}
	for _, seen := range s.keys {
		if seen == key {
			return true
		}
	}
	return false
}

func (s *dailySeenSet) add(day int64, key string) {
	if s.day != day {
		s.day = day
		s.keys = s.keys[:0]
	}
	s.keys = append(s.keys, key)
}

type tradeWindow struct {
	Key   string
	Start float64
	End   float64
}

var tradeWindows = [...]tradeWindow{
	{Key: "asia", Start: 9, End: 12},
	{Key: "mid", Start: 12, End: 16},
	{Key: "london", Start: 16, End: 19},
	{Key: "ny", Start: 21, End: 24},
}

type windowProgress struct {
	Key              string
	MinutesFromStart int
	utcSlot          bool
	slotMinutes      int
	SlotStart        int64
}

func tradeWindowProgress(t float64, allowed map[string]bool) (windowProgress, bool) {
	h := contextcols.LocalHour(int64(t))
	for _, w := range tradeWindows {
		if !allowed[w.Key] {
			continue
		}
		if h >= w.Start && h < w.End {
			return windowProgress{
				Key:              w.Key,
				MinutesFromStart: int(math.Floor((h-w.Start)*60 + 1e-6)),
			}, true
		}
	}
	return windowProgress{}, false
}

func tradeWindowSegmentMatches(part string, minutesFromStart int) bool {
	switch part {
	case "", "all", "full":
		return true
	case "open", "first":
		return minutesFromStart >= 0 && minutesFromStart < 60
	case "middle", "second":
		return minutesFromStart >= 60 && minutesFromStart < 120
	case "close", "third":
		return minutesFromStart >= 120 && minutesFromStart < 180
	default:
		return false
	}
}

func inSegmentedTradeWindow(t float64, p flagParams, allowed map[string]bool) bool {
	if len(p.TradeWindowSegments) == 0 && !p.TradeWindowMinuteRangeSet {
		return true
	}
	progress, ok := tradeWindowProgress(t, allowed)
	if !ok {
		return false
	}
	if p.TradeWindowMinuteRangeSet && (float64(progress.MinutesFromStart) < p.TradeWindowMinuteFrom || float64(progress.MinutesFromStart) >= p.TradeWindowMinuteTo) {
		return false
	}
	if len(p.TradeWindowSegments) == 0 {
		return true
	}
	for _, raw := range p.TradeWindowSegments {
		parts := strings.SplitN(strings.ToLower(raw), ".", 2)
		part := "all"
		if len(parts) == 2 {
			part = parts[1]
		}
		if parts[0] == progress.Key && tradeWindowSegmentMatches(part, progress.MinutesFromStart) {
			return true
		}
	}
	return false
}

func allowedWindows(p flagParams) map[string]bool {
	return map[string]bool{
		"asia":   p.UseAsiaWindow,
		"mid":    p.UseMidWindow,
		"london": p.UseLondonWindow,
		"ny":     p.UseNYWindow,
	}
}

func inSetupTradeWindow(t float64, p flagParams, minMinutesLeft float64) bool {
	if len(p.NewYorkHours) > 0 || len(p.BlockedNewYorkHours) > 0 {
		hourNY := contextcols.NewYorkHour(int64(t))
		if len(p.NewYorkHours) > 0 && !intContains(p.NewYorkHours, hourNY) {
			return false
		}
		if len(p.BlockedNewYorkHours) > 0 && intContains(p.BlockedNewYorkHours, hourNY) {
			return false
		}
	}
	if p.TradeWindowUnrestricted {
		return true
	}
	h := contextcols.LocalHour(int64(t))
	inside := p.UseAsiaWindow && h >= 9 && h < 12 && (12-h)*60 >= minMinutesLeft ||
		p.UseMidWindow && h >= 12 && h < 16 && (16-h)*60 >= minMinutesLeft ||
		p.UseLondonWindow && h >= 16 && h < 19 && (19-h)*60 >= minMinutesLeft ||
		p.UseNYWindow && h >= 21 && h < 24 && (24-h)*60 >= minMinutesLeft
	return inside && inSegmentedTradeWindow(t, p, allowedWindows(p))
}

func openingAllowedWindows(p flagParams) map[string]bool {
	allowed := allowedWindows(p)
	if p.ORBOpeningSessionsOverride {
		for key := range allowed {
			allowed[key] = containsString(p.ORBOpeningSessions, key)
		}
	}
	return allowed
}

func (b *broker) marketGatesOK(i int) bool {
	if !inFlagTradeWindow(b.series.T[i], b.params, 0) {
		return false
	}
	return b.marketNonSessionGatesOK(i)
}

func (b *broker) marketNonSessionGatesOK(i int) bool {
	if len(b.params.SessionPhases) > 0 {
		phase := ""
		if i >= 0 && i < len(b.cols.SessionPhase) {
			phase = sessionPhaseName(b.cols.SessionPhase[i])
		}
		if !containsString(b.params.SessionPhases, phase) {
			return false
		}
	}
	priorDayType := ""
	if i >= 0 && i < len(b.cols.PriorDayType) {
		priorDayType = priorDayTypeName(b.cols.PriorDayType[i])
	}
	if len(b.params.PriorDayTypes) > 0 && !containsString(b.params.PriorDayTypes, priorDayType) {
		return false
	}
	if len(b.params.BlockedPriorDayTypes) > 0 && containsString(b.params.BlockedPriorDayTypes, priorDayType) {
		return false
	}
	if len(b.params.DayTypes) > 0 && !regimeAllowed(b.cols.Regime[i], b.params.DayTypes) && finiteOrZero(b.cols.ER[i]) > b.params.DayTypeEREscape {
		return false
	}
	if finiteOrZero(b.cols.ER[i]) > b.params.MaxMovementER {
		return false
	}
	if b.params.PriorDayMinRangeATR > 0 {
		atr := finiteOrZero(b.cols.ATR[i])
		if atr == 0 || !isFinite(b.cols.PriorDayH[i]) || !isFinite(b.cols.PriorDayL[i]) {
			return false
		}
		if b.cols.PriorDayH[i]-b.cols.PriorDayL[i] < atr*b.params.PriorDayMinRangeATR {
			return false
		}
	}
	return true
}

func priorDayTypeName(code int8) string {
	switch code {
	case 1:
		return "range"
	case 2:
		return "trend"
	case 3:
		return "wide"
	case 4:
		return "narrow"
	case 5:
		return "outside"
	default:
		return ""
	}
}

func sessionPhaseName(code int8) string {
	switch code {
	case 1:
		return "open"
	case 2:
		return "middle"
	case 3:
		return "lunch"
	case 4:
		return "close"
	case 5:
		return "overnight"
	case 6:
		return "other"
	default:
		return ""
	}
}

func openLocationName(code int8) string {
	switch code {
	case 1:
		return "nearPDH"
	case 2:
		return "nearPDL"
	case 3:
		return "nearPDO"
	case 4:
		return "nearPDC"
	case 5:
		return "nearDayOpen"
	case 6:
		return "nearRangeMid"
	case 7:
		return "nearAH"
	case 8:
		return "nearAL"
	case 9:
		return "nearLH"
	case 10:
		return "nearLL"
	case 11:
		return "nearOvernightHigh"
	case 12:
		return "nearOvernightLow"
	case 15:
		return "nearCurrentOpen"
	default:
		return "other"
	}
}

func regimeName(code int8) string {
	switch code {
	case 1:
		return "trending"
	case 2:
		return "ranging"
	case 3:
		return "choppy"
	default:
		return ""
	}
}

func regimeAllowed(regime int8, allowed []string) bool {
	name := ""
	switch regime {
	case 1:
		name = "trending"
	case 2:
		name = "ranging"
	case 3:
		name = "choppy"
	}
	return containsString(allowed, name)
}

func (b *broker) htfAllows(i int, s side) bool {
	if !b.params.UseHTFBias {
		return true
	}
	// "Must not oppose": fail closed when HTF bias is requested but no completed
	// bar is available (absent series or a gap). A genuine flat bar opposes
	// neither side and is allowed.
	if i < 0 || i >= len(b.htfTrend) {
		return false
	}
	htfTrend := b.htfTrend[i]
	if htfTrend == htfUnavailable {
		return false
	}
	if htfTrend == trendFlat {
		return true
	}
	if s == sideLong {
		return htfTrend == trendUp
	}
	return htfTrend == trendDown
}

func recentExtreme(series marketdata.Series, i int, lookback int, s side) float64 {
	if lookback <= 0 {
		lookback = 1
	}
	start := maxInt(0, i-lookback+1)
	if s == sideLong {
		extreme := math.Inf(1)
		for j := start; j <= i; j++ {
			extreme = math.Min(extreme, series.L[j])
		}
		return extreme
	}
	extreme := math.Inf(-1)
	for j := start; j <= i; j++ {
		extreme = math.Max(extreme, series.H[j])
	}
	return extreme
}

func triggerOK(series marketdata.Series, i int, s side, useTrigger bool) bool {
	if !useTrigger {
		return true
	}
	if s == sideLong {
		return longTrigger(series, i)
	}
	return shortTrigger(series, i)
}

func candleQualityOK(series marketdata.Series, i int, s side, tailMin float64, closeMin float64) bool {
	if closeMin != 0 && closeLocation(series, i, s) < closeMin {
		return false
	}
	if tailMin != 0 && tailRejection(series, i, s) < tailMin {
		return false
	}
	return true
}

func (b *broker) guardedCandleQualityOK(i int, s side) bool {
	if i < 0 || i >= b.series.Len() {
		return false
	}
	return candleQualityOK(b.series, i, s, b.params.TailRejectionMin, b.params.CloseLocationMin)
}

func (b *broker) typedEntryDistanceOK(i int, meta TradeMeta) bool {
	if b.params.SetupType == string(dsl.FamilyFailedBreakout) || b.params.MaxEntryDistanceATR == 0 {
		return true
	}
	if i < 0 || i >= len(b.series.C) || i >= len(b.cols.ATR) {
		return true
	}
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 {
		return true
	}
	level, ok := meta["levelPrice"].(float64)
	if !ok || !isFinite(level) {
		return true
	}
	return math.Abs(b.series.C[i]-level) <= atr*b.params.MaxEntryDistanceATR
}

func (b *broker) dayThemeAdmission(i int, s side) (string, bool) {
	if len(b.params.DayThemes) == 0 {
		return "", true
	}
	theme := b.inferDayTheme(i)
	if !containsString(b.params.DayThemes, theme) {
		return theme, false
	}
	switch theme {
	case "join_momentum":
		if i < 0 || i >= len(b.cols.TrendDir) {
			return theme, false
		}
		want := side(0)
		if b.cols.TrendDir[i] == trendUp {
			want = sideLong
		} else if b.cols.TrendDir[i] == trendDown {
			want = sideShort
		}
		return theme, want != 0 && s == want
	case "buy_lows":
		return theme, s == sideLong
	case "sell_highs":
		return theme, s == sideShort
	default:
		return theme, false
	}
}

func (b *broker) inferDayTheme(i int) string {
	if i < 0 || i >= b.series.Len() {
		return "stand_aside"
	}
	if i < len(b.cols.Regime) && i < len(b.cols.TrendDir) && b.cols.Regime[i] == regimeTrending {
		if b.cols.TrendDir[i] == trendUp || b.cols.TrendDir[i] == trendDown {
			return "join_momentum"
		}
	}
	if i < len(b.cols.OpenLocation) {
		switch openLocationName(b.cols.OpenLocation[i]) {
		case "nearPDH", "nearAH", "nearLH", "nearOvernightHigh":
			return "sell_highs"
		case "nearPDL", "nearAL", "nearLL", "nearOvernightLow":
			return "buy_lows"
		}
	}
	if i < len(b.cols.PriorDayH) && i < len(b.cols.PriorDayL) {
		hi := b.cols.PriorDayH[i]
		lo := b.cols.PriorDayL[i]
		close := b.series.C[i]
		if isFinite(hi) && isFinite(lo) && hi > lo && isFinite(close) {
			position := (close - lo) / (hi - lo)
			if position <= 0.38 {
				return "buy_lows"
			}
			if position >= 0.62 {
				return "sell_highs"
			}
		}
	}
	return "stand_aside"
}

func annotateDayTheme(meta TradeMeta, theme string) TradeMeta {
	if theme == "" {
		return meta
	}
	annotated := make(TradeMeta, len(meta)+1)
	for key, value := range meta {
		annotated[key] = value
	}
	annotated["dayTheme"] = theme
	return annotated
}

func tailRejection(series marketdata.Series, i int, s side) float64 {
	rng := math.Max(0, series.H[i]-series.L[i])
	if rng == 0 {
		return 0
	}
	upper := series.H[i] - math.Max(series.O[i], series.C[i])
	lower := math.Min(series.O[i], series.C[i]) - series.L[i]
	if s == sideShort {
		return upper / rng
	}
	return lower / rng
}

func gradeMeta(meta TradeMeta) TradeMeta {
	meta["gradeRequired"] = float64(0)
	meta["gradeScore"] = float64(0)
	return meta
}

func flagTradeMeta(meta TradeMeta) TradeMeta {
	return gradeMeta(meta)
}

func (b *broker) enterSetup(i int, setup setupPlan) bool {
	if b.hasPosition || len(b.pendingOrders) > 0 || len(b.limitOrders) > 0 {
		return false
	}
	if !b.marketGatesOK(i) || !b.guardedCandleQualityOK(i, setup.Side) {
		return false
	}
	theme, ok := b.dayThemeAdmission(i, setup.Side)
	if !ok {
		return false
	}
	if !b.typedEntryDistanceOK(i, setup.Meta) {
		return false
	}
	setup.Meta = annotateDayTheme(setup.Meta, theme)
	ord := order{
		Side:    setup.Side,
		SL:      setup.Stop,
		TP:      setup.Target,
		RiskUSD: b.params.RiskUSD,
		HasRisk: true,
		Tag:     setup.Tag,
		Meta:    setup.Meta,
		Index:   i,
	}
	if anchor, ok := setup.Meta["forwardBracketAnchor"].(string); ok && anchor == "entry-fill" {
		if distance, ok := numericMetaValue(setup.Meta["forwardStopDistance"]); ok && distance > 0 {
			ord.StopDistance = distance
			ord.HasStopDistance = true
		}
		if distance, ok := numericMetaValue(setup.Meta["forwardTargetDistance"]); ok && distance > 0 {
			ord.TargetDistance = distance
			ord.HasTargetDistance = true
		}
	}
	if b.captureEntries {
		b.capturedEntries = append(b.capturedEntries, ord)
		return true
	}
	if b.costs.fillsMarketAtNextOpen() {
		b.queueMarketAtNextOpen(&ord, i)
		return true
	}
	b.openPosition(setup.Side, b.series.C[i], ord, i)
	return true
}
func priorWeekLevels(series marketdata.Series, i int) (float64, float64, bool) {
	curWeek := localWeekKey(series.T[i])
	hi := math.Inf(-1)
	lo := math.Inf(1)
	found := false
	for j := i - 1; j >= 0; j-- {
		week := localWeekKey(series.T[j])
		if week == curWeek {
			continue
		}
		if week < curWeek-1 {
			break
		}
		hi = math.Max(hi, series.H[j])
		lo = math.Min(lo, series.L[j])
		found = true
	}
	return hi, lo, found
}
