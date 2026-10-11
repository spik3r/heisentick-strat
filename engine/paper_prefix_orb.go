package engine

import (
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/spik3r/heisentick-strat/dsl"
)

// paperORBContext names input-derived context, not a research capability grant.
// Consumers with a narrower execution model can refuse individual categories.
type paperORBContext uint32

const (
	paperORBSessionPhase paperORBContext = 1 << iota
	paperORBDayMovement
	paperORBLocalCalendar
	paperORBOpenLocation
	paperORBPriorDay
	paperORBDayTheme
	paperORBRMV
	paperORBSeasonality
)

// These flags retain every authored occurrence. In particular a later off/reset
// cannot erase enabled partial/trailing intent for a narrower model's admission.
// This receipt is neither a configuration nor an alternative source parser.
type paperORBAuthoredFeatures struct {
	Partial, Trailing, Breakeven, HTF, SourceEntry bool
	Contexts                                       paperORBContext
}

func validatePaperORB(source string, cfg dsl.Config, route PaperRoute) (paperORBAuthoredFeatures, error) {
	features, ok := paperORBAuthoredSupported(source)
	if !ok {
		return features, paperError("ADMISSION")
	}
	neutral, err := dsl.Parse("dsl v7\nsetup { type: opening range breakout }\n")
	if err != nil || len(neutral.Errors) != 0 || !paperORBConfigSupported(map[string]any(cfg), map[string]any(neutral.Config), "") || !paperORBValuesSupported(cfg) {
		return features, paperError("ADMISSION")
	}
	if !compiledRouteAllowed(cfg, route.Symbol, route.Timeframe) {
		return features, paperError("ROUTE_UNSUPPORTED")
	}
	p := paramsFromConfig(cfg)
	if p.ORBFixedStopPips > 0 || p.ORBFixedTargetPips > 0 {
		if _, ok := reviewedInstrumentPip(route.Symbol); !ok {
			return features, paperError("ADMISSION")
		}
	}
	return features, nil
}

// Exact variable leaves. A supported parent never admits arbitrary descendants.
var paperORBLeaves = map[string]string{
	"name": "text", "description": "text", "symbols": "symbols", "timeframes": "timeframes", "slices": "slices",
	"allowLong": "flag", "allowShort": "flag",
	"sessions.asia": "flag", "sessions.mid": "flag", "sessions.london": "flag", "sessions.ny": "flag",
	"openingRangeBreakout.firstMinutes": "count", "openingRangeBreakout.firstCandles": "count", "openingRangeBreakout.utcSlotMinutes": "slot",
	"openingRangeBreakout.openingSessions": "sessions", "openingRangeBreakout.breakBufferAtr": "nonnegative", "openingRangeBreakout.holdCandles": "positiveCount",
	"tradeWindowMode": "windowMode", "tradeWindowSegments": "segments", "tradeWindowMinuteRange.from": "nonnegative", "tradeWindowMinuteRange.to": "positive",
	"tradeWindowUTCHourRange.from": "nonnegative", "tradeWindowUTCHourRange.to": "positive",
	"sessionPhases": "phases", "localWeekdays": "weekdays", "blockedLocalWeekdays": "weekdays", "localHours": "hours", "blockedLocalHours": "hours",
	"openLocations": "locations", "priorDayTypes": "priorTypes", "blockedPriorDayTypes": "priorTypes", "priorDay.minRangeAtr": "nullableNonnegative",
	"dayTypes": "dayTypes", "dayTypeErEscape": "escape", "dayThemes": "themes", "maxMovementEr": "nonnegative",
	"tailRejectionMin": "fraction", "closeLocationMin": "fraction", "relativeMeasuredVolatility.atrPeriod": "positiveCount", "relativeMeasuredVolatility.lookback": "positiveCount",
	"relativeMeasuredVolatility.op": "rmvOp", "relativeMeasuredVolatility.value": "rmvValue", "seasonalityFilters": "seasonality",
	"stop.type": "stopType", "stop.atr": "positive", "stop.pips": "positive", "stop.paddingAtr": "nonnegative", "stop.minAtr": "nonnegative", "stop.maxAtr": "positive",
	"target.orbR": "positive", "target.orbRange": "nonnegative", "target.pips": "positive", "riskUsd": "positive",
	"breakeven.atR": "nonnegative", "breakeven.offsetAtr": "nonnegative", "partial.enabled": "flag", "partial.fraction": "nonnegative", "partial.triggerR": "positive", "partial.moveBreakeven": "flag",
	"trail.atr": "nonnegative", "trail.triggerR": "nonnegative", "cooldownCandles": "nonnegative", "maxHoldCandles": "nonnegative",
}

func paperORBConfigSupported(actual, neutral map[string]any, prefix string) bool {
	for key, value := range actual {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		base, exists := neutral[key]
		if kind, supported := paperORBLeaves[path]; supported {
			if !paperORBLeafSupported(kind, value) {
				return false
			}
			continue
		}
		if nested, ok := value.(map[string]any); ok {
			baseline, baselineMap := base.(map[string]any)
			if !baselineMap {
				// Optional maps have an explicitly enumerated set of permitted children.
				if path != "relativeMeasuredVolatility" && path != "tradeWindowMinuteRange" && path != "tradeWindowUTCHourRange" {
					return false
				}
			}
			if !paperORBConfigSupported(nested, baseline, path) {
				return false
			}
		} else if !exists || !reflect.DeepEqual(value, base) {
			return false
		}
	}
	for key := range neutral {
		if _, ok := actual[key]; !ok {
			return false
		}
	}
	return true
}

func paperORBNumber(value any) (float64, bool) {
	switch n := value.(type) {
	case int:
		return float64(n), true
	case float64:
		return n, isFinite(n)
	default:
		return 0, false
	}
}
func paperORBStrings(value any) ([]string, bool) {
	switch values := value.(type) {
	case []string:
		return values, true
	case []any:
		out := make([]string, len(values))
		for i, raw := range values {
			s, ok := raw.(string)
			if !ok {
				return nil, false
			}
			out[i] = s
		}
		return out, true
	default:
		return nil, false
	}
}
func paperORBIn(s string, values ...string) bool {
	for _, value := range values {
		if s == value {
			return true
		}
	}
	return false
}

func paperORBLeafSupported(kind string, value any) bool {
	switch kind {
	case "text":
		_, ok := value.(string)
		return ok
	case "slices":
		rows, ok := value.([]any)
		if !ok {
			return false
		}
		for _, raw := range rows {
			row, ok := raw.(map[string]any)
			if !ok || len(row) != 2 {
				return false
			}
			symbol, sok := row["symbol"].(string)
			tf, tok := row["tf"].(string)
			if !sok || !tok || !paperORBSymbol.MatchString(symbol) || paperTimeframeMS(tf) == 0 {
				return false
			}
		}
		return true
	case "seasonality":
		return paperORBSeasonalitySupported(value)
	case "hours":
		rows, ok := value.([]any)
		if !ok {
			return false
		}
		for _, raw := range rows {
			n, ok := paperORBNumber(raw)
			if !ok || n < 0 || n > 23 || math.Trunc(n) != n {
				return false
			}
		}
		return true
	case "symbols", "timeframes", "sessions", "segments", "phases", "weekdays", "locations", "priorTypes", "dayTypes", "themes":
		values, ok := paperORBStrings(value)
		if !ok {
			return false
		}
		for _, v := range values {
			switch kind {
			case "symbols":
				ok = paperORBSymbol.MatchString(v)
			case "timeframes":
				ok = paperTimeframeMS(v) != 0
			case "sessions":
				ok = paperORBIn(v, "asia", "mid", "london", "ny")
			case "segments":
				ok = paperORBSegment.MatchString(v)
			case "phases":
				ok = paperORBIn(v, "open", "middle", "lunch", "close", "overnight", "other")
			case "weekdays":
				ok = paperORBIn(v, "Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat")
			case "locations":
				ok = paperORBIn(v, "nearPDH", "nearPDL", "nearPDO", "nearPDC", "nearDayOpen", "nearRangeMid", "nearAH", "nearAL", "nearLH", "nearLL", "nearOvernightHigh", "nearOvernightLow", "nearCurrentOpen", "other")
			case "priorTypes":
				ok = paperORBIn(v, "range", "trend", "wide", "narrow", "outside")
			case "dayTypes":
				ok = paperORBIn(v, "trending", "ranging", "choppy")
			case "themes":
				ok = paperORBIn(v, "join_momentum", "buy_lows", "sell_highs", "stand_aside")
			}
			if !ok {
				return false
			}
		}
		return true
	case "windowMode", "stopType", "rmvOp":
		s, ok := value.(string)
		if !ok {
			return false
		}
		if kind == "windowMode" {
			return s == "unrestricted"
		}
		if kind == "stopType" {
			return s == "fixedAtr" || s == "pips"
		}
		return paperORBIn(s, "above", "below", "atLeast", "atMost")
	case "nullableNonnegative":
		if value == nil {
			return true
		}
	}
	n, ok := paperORBNumber(value)
	if !ok {
		return false
	}
	switch kind {
	case "flag":
		return n == 0 || n == 1
	case "positive":
		return n > 0
	case "nonnegative", "nullableNonnegative":
		return n >= 0
	case "count":
		return n >= 0 && n < math.Exp2(31) && math.Trunc(n) == n
	case "positiveCount":
		return n > 0 && n < math.Exp2(31) && math.Trunc(n) == n
	case "slot":
		return n >= 60 && n <= 1440 && math.Mod(n, 60) == 0
	case "fraction":
		return n >= 0 && n <= 1
	case "escape":
		return n == -1 || n >= 0
	case "rmvValue":
		return n >= 0 && n <= 100
	default:
		return false
	}
}

func paperORBValuesSupported(cfg dsl.Config) bool {
	p := paramsFromConfig(cfg)
	if p.MinStopATR > p.MaxStopATR || (p.Partial.Enabled && (p.Partial.Fraction <= 0 || p.Partial.TriggerR <= 0)) {
		return false
	}
	// Fractional hold thresholds are intentionally preserved; cooldown uses the
	// existing rounded integer conversion and must stay representable on all targets.
	if p.CooldownBars < 0 || numberFromAny(cfg["cooldownCandles"], 0) >= math.Exp2(31)-1 {
		return false
	}
	for _, key := range []string{"tradeWindowMinuteRange", "tradeWindowUTCHourRange"} {
		if cfg[key] == nil {
			continue
		}
		bounds := mapValue(cfg, key)
		if len(bounds) != 2 {
			return false
		}
		from, to := numberValue(bounds, "from", -1), numberValue(bounds, "to", -1)
		if from < 0 || to <= from || (key == "tradeWindowUTCHourRange" && to > 24) {
			return false
		}
	}
	if raw, present := cfg["relativeMeasuredVolatility"]; present {
		rmv, ok := raw.(map[string]any)
		if !ok || (len(rmv) != 2 && len(rmv) != 4) {
			return false
		}
		for _, key := range []string{"atrPeriod", "lookback"} {
			if _, exists := rmv[key]; !exists {
				return false
			}
		}
		if len(rmv) == 4 {
			if _, exists := rmv["op"]; !exists {
				return false
			}
			if _, exists := rmv["value"]; !exists {
				return false
			}
		}
	}
	if err := validateRMVConfig(cfg); err != nil {
		return false
	}
	return true
}

func paperORBSeasonalitySupported(value any) bool {
	rows, ok := value.([]any)
	if !ok {
		return false
	}
	for _, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			return false
		}
		mode, ok := row["mode"].(string)
		if !ok || !paperORBIn(mode, "classification", "supportsEntry") {
			return false
		}
		fields := 5
		if mode == "classification" {
			fields++
		}
		if len(row) != fields {
			return false
		}
		for _, key := range []string{"dimension", "lookback", "lookbackDays", "minSamples", "mode"} {
			if _, exists := row[key]; !exists {
				return false
			}
		}
		dimension, ok := row["dimension"].(string)
		if !ok || !paperORBIn(dimension, "intraday", "dayOfWeek", "weekOfYear", "monthly") {
			return false
		}
		lookback, ok := row["lookback"].(string)
		if !ok {
			return false
		}
		days, daysOK := paperORBNumber(row["lookbackDays"])
		if lookback == "all" {
			if row["lookbackDays"] != nil {
				return false
			}
		} else {
			if !daysOK || days <= 0 || math.Trunc(days) != days || days >= math.Exp2(31) || lookback != strconv.FormatFloat(days, 'f', 0, 64)+"d" {
				return false
			}
		}
		if !paperORBLeafSupported("positiveCount", row["minSamples"]) {
			return false
		}
		if mode == "classification" {
			classes, ok := paperORBStrings(row["classifications"])
			if !ok || len(classes) == 0 {
				return false
			}
			for _, class := range classes {
				if !paperORBIn(class, "insufficient_data", "extreme_bullish", "bullish", "neutral", "bearish", "extreme_bearish") {
					return false
				}
			}
		}
	}
	return true
}

var paperORBMetadata = regexp.MustCompile(`(?i)^(?:strategy|name|description) (?:"[^"]*"|'[^']*')$`)

var paperORBSymbol = regexp.MustCompile(`^[A-Za-z0-9_./:-]+$`)
var paperORBSegment = regexp.MustCompile(`^(?:asia|mid|london|ny)\.(?:open|middle|close|all)$`)
var paperORBNumberToken = regexp.MustCompile(`^(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:e-?[0-9]+)?(?:r|%)?$`)

type paperORBForm struct {
	pattern *regexp.Regexp
	context paperORBContext
}

var paperORBForms = func() map[string][]paperORBForm {
	n := `(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:e-?[0-9]+)?`
	session := `(?:asia|mid|london|ny)`
	tf := `(?:1m|5m|15m|30m|1h|4h|1d)`
	symbol := `[a-z0-9_./:-]+`
	forms := map[string][]paperORBForm{}
	add := func(head, pattern string, context paperORBContext) {
		forms[head] = append(forms[head], paperORBForm{regexp.MustCompile(`(?i)^` + pattern + `$`), context})
	}
	add("dsl", `dsl (?:v?[67])`, 0)
	add("type", `type opening range breakout`, 0)
	add("slices", `slices(?: `+symbol+` `+tf+`)*`, 0)
	add("symbols", `symbols(?: `+symbol+`)*`, 0)
	add("timeframes", `timeframes(?: `+tf+`)*`, 0)
	add("sessions", `sessions(?: `+session+`)*`, 0)
	add("opening", `opening range first `+n+` (?:candles?|bars?|minutes?)(?: in(?: `+session+`)+)?`, 0)
	add("opening", `opening range every `+n+` hours utc(?: in(?: `+session+`)+)?`, 0)
	add("break", `break beyond (?:range )?edge(?: (?:by|buffer) `+n+`(?: atr)?)?`, 0)
	add("hold", `hold `+n+` (?:candles?|bars?)`, 0)
	add("side", `side (?:(?:both|long|short)(?: only)?|only (?:long|short))`, 0)
	add("direction", `direction (?:(?:both|long|short)(?: only)?|only (?:long|short))`, 0)
	add("trade", `trade window unrestricted`, 0)
	add("trade", `trade window utc hours `+n+` to `+n, 0)
	add("trade", `trade window minutes `+n+` to `+n, 0)
	segment := session + `(?:[._-]| )(?:open|middle|close|all|first|second|third|full)`
	add("trade", `trade window in `+segment+`(?: `+segment+`)*`, 0)
	add("session", `session phase in(?: (?:open|middle|lunch|close|overnight|other))+`, paperORBSessionPhase)
	weekday := `(?:sun(?:day)?|mon(?:day)?|tue(?:sday)?|wed(?:nesday)?|thu(?:rsday)?|fri(?:day)?|sat(?:urday)?)`
	add("local", `local weekday (?:not )?in(?: `+weekday+`)+`, paperORBLocalCalendar)
	add("weekday", `weekday (?:not )?in(?: `+weekday+`)+`, paperORBLocalCalendar)
	add("local", `local hour (?:not )?in(?: [0-9]+(?::00)?)+`, paperORBLocalCalendar)
	add("hour", `hour (?:not )?in(?: [0-9]+(?::00)?)+`, paperORBLocalCalendar)
	add("open", `open location in(?: (?:near )?[a-z0-9_.-]+)+`, paperORBOpenLocation)
	add("prior", `prior day type (?:not )?in(?: (?:range|trend|wide|narrow|outside))+`, paperORBPriorDay)
	add("prior", `prior day range (?:at least |>= )?`+n+`(?: atr)?`, paperORBPriorDay)
	add("day", `day type in(?: (?:trending|ranging|choppy))+(?: strictly| or movement (?:below |<= )?`+n+`)?`, paperORBDayMovement)
	add("day", `day theme in(?: (?:join_momentum|buy_lows|sell_highs|stand_aside))+`, paperORBDayTheme)
	for _, head := range []string{"movement", "maxmovementer", "trendiness"} {
		add(head, head+`(?: below| <=)? `+n, paperORBDayMovement)
	}
	add("tail", `tail rejection (?:at least |>= )?`+n, 0)
	add("close", `close location (?:at least |>= )?`+n, 0)
	add("rmv", `rmv (?:above|over|below|under|at least|at most) `+n, paperORBRMV)
	add("rmv", `rmv (?:atr period|lookback) `+n, paperORBRMV)
	// The ordinary seasonality parser already strictly validates every token.
	add("seasonality", `seasonality .+`, paperORBSeasonality)
	add("stop", `stop `+n+` (?:atr|pips)`, 0)
	add("stop", `stop (?:beyond )?opposite range edge(?: (?:(?:by|plus|\+) )?`+n+`(?: atr)?)?(?: min `+n+`(?: atr)?)?(?: max `+n+`(?: atr)?)?`, 0)
	add("stop", `stop size <= `+n+`(?: atr)?`, 0)
	add("stop", `stop size between `+n+` and `+n+`(?: atr)?`, 0)
	add("stop", `stop size min `+n+`(?: atr)?(?: max `+n+`(?: atr)?)?`, 0)
	add("stop", `stop size max `+n+`(?: atr)?`, 0)
	add("target", `target `+n+` ?r`, 0)
	add("target", `target `+n+` (?:range|pips)`, 0)
	add("risk", `risk `+n+` usd`, 0)
	add("riskusd", `riskusd `+n, 0)
	add("move", `move stop to breakeven after `+n+` ?r(?: plus `+n+`(?: atr)?)?`, 0)
	add("breakeven", `breakeven off`, 0)
	add("breakeven", `breakeven atr `+n+`(?: offsetatr `+n+`)?`, 0)
	add("partial", `partial (?:half|quarter|`+n+`%?) at `+n+` ?r(?: (?:move (?:stop )?to )?breakeven)?`, 0)
	add("trail", `trail off`, 0)
	add("trail", `trail `+n+`(?: atr)?(?: after `+n+` ?r)?`, 0)
	add("wait", `wait `+n+` (?:candles?|bars?) after trade`, 0)
	add("maxholdcandles", `maxholdcandles `+n, 0)
	add("maxholdbars", `maxholdbars `+n, 0)
	add("higher", `higher timeframe off`, 0)
	add("entrytf", `entrytf current`, 0)
	return forms
}()

func paperORBAuthoredSupported(source string) (paperORBAuthoredFeatures, bool) {
	features := paperORBAuthoredFeatures{}
	audit := dsl.AuditOrdinarySource(source)
	if len(audit.Issues) != 0 {
		return features, false
	}
	typeSeen := false
	for _, directive := range audit.Directives {
		tokens := directive.Tokens
		head := strings.ToLower(tokens[0])
		text := strings.ToLower(strings.Join(tokens, " "))
		if head == "strategy" || head == "name" || head == "description" {
			if !paperORBMetadata.MatchString(strings.Join(tokens, " ")) {
				return features, false
			}
			continue
		}
		if head == "higher" && text != "higher timeframe off" {
			features.HTF = true
		}
		if head == "source" || (head == "entrytf" && text != "entrytf current") {
			features.SourceEntry = true
		}
		if head == "type" {
			if text != "type opening range breakout" {
				return features, false
			}
			typeSeen = true
		}
		if !typeSeen && paperORBIn(head, "opening", "break", "hold", "stop", "target", "move", "breakeven", "partial", "trail", "wait", "maxholdcandles", "maxholdbars") {
			return features, false
		}
		matched := false
		for _, form := range paperORBForms[head] {
			if form.pattern.MatchString(text) {
				features.Contexts |= form.context
				matched = true
				break
			}
		}
		if !matched || !paperORBAuthoredValues(tokens) {
			return features, false
		}
		switch head {
		case "partial":
			features.Partial = true
		case "trail":
			if text != "trail off" && paperORBTokenNumber(tokens[1]) > 0 {
				features.Trailing = true
			}
		case "move", "breakeven":
			features.Breakeven = true
		}
	}
	return features, typeSeen
}

func paperORBTokenNumber(token string) float64 {
	token = strings.ToLower(token)
	if token == "half" {
		return .5
	}
	if token == "quarter" {
		return .25
	}
	n, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSuffix(token, "r"), "%"), 64)
	if strings.HasSuffix(token, "%") {
		n /= 100
	}
	return n
}
func paperORBAuthoredValues(tokens []string) bool {
	// Every numeric occurrence is bounded before a later directive can erase it.
	for _, token := range tokens {
		if paperORBNumberToken.MatchString(strings.ToLower(token)) && !isFinite(paperORBTokenNumber(token)) {
			return false
		}
	}
	head := strings.ToLower(tokens[0])
	if paperORBIn(head, "tail", "close") && paperORBTokenNumber(tokens[len(tokens)-1]) > 1 {
		return false
	}
	if head == "wait" && paperORBTokenNumber(tokens[1]) >= math.Exp2(31)-1 {
		return false
	}
	if head == "opening" && strings.EqualFold(tokens[2], "first") {
		n := paperORBTokenNumber(tokens[3])
		if n <= 0 || n >= math.Exp2(31) || n != math.Trunc(n) {
			return false
		}
	}
	if head == "stop" {
		var minimum, maximum float64
		haveMin, haveMax := false, false
		for i, token := range tokens {
			if i+1 >= len(tokens) {
				continue
			}
			switch strings.ToLower(token) {
			case "min", "between":
				minimum, haveMin = paperORBTokenNumber(tokens[i+1]), true
			case "max", "<=", "and":
				maximum, haveMax = paperORBTokenNumber(tokens[i+1]), true
			}
		}
		if haveMax && (maximum <= 0 || (haveMin && minimum > maximum)) {
			return false
		}
	}
	if paperORBIn(head, "risk", "riskusd", "hold", "partial") && paperORBTokenNumber(tokens[1]) <= 0 {
		return false
	}
	if head == "hold" && (paperORBTokenNumber(tokens[1]) >= math.Exp2(31) || math.Trunc(paperORBTokenNumber(tokens[1])) != paperORBTokenNumber(tokens[1])) {
		return false
	}
	if head == "stop" && paperORBNumberToken.MatchString(strings.ToLower(tokens[1])) && paperORBTokenNumber(tokens[1]) <= 0 {
		return false
	}
	if head == "target" && paperORBTokenNumber(tokens[1]) <= 0 && !strings.EqualFold(tokens[len(tokens)-1], "range") {
		return false
	}
	if head == "partial" {
		for i, token := range tokens {
			if strings.EqualFold(token, "at") && paperORBTokenNumber(tokens[i+1]) <= 0 {
				return false
			}
		}
	}
	return true
}
