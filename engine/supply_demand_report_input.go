package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const (
	SupplyDemandRetestRequestSchema          = "strat-supply-demand-retest-request-v1"
	SupplyDemandRetestProfile                = "supply-demand-retest-bounded-v1"
	SupplyDemandRetestBasis                  = "supply-demand-retest-broker-close-mtm-v1"
	SupplyDemandRetestHTFPolicy              = "completed-daily-direction-decision-time-weekend-v1"
	SupplyDemandRetestMaxRequestBytes        = 64 << 20
	SupplyDemandRetestMaxSourceBytes         = 1 << 20
	SupplyDemandRetestMaxOutputBytes         = 64 << 20
	SupplyDemandRetestMaxRows                = 500000
	SupplyDemandRetestMaxWorkScore    uint64 = 500000000
)

// SupplyDemandRetestError is a bounded machine-readable dedicated refusal.
// Field is a code-owned path, never a user-provided value or parser diagnostic.
type SupplyDemandRetestError struct {
	Code     string
	Field    string
	BarIndex int
}

func (e *SupplyDemandRetestError) Error() string {
	return fmt.Sprintf("supply-demand-retest: %s (%s, row %d)", e.Code, e.Field, e.BarIndex)
}
func supplyDemandReportError(code, field string, index int) error {
	return &SupplyDemandRetestError{Code: code, Field: field, BarIndex: index}
}

type SupplyDemandRetestSeriesIdentity struct {
	Count      int      `json:"count"`
	FirstT     *float64 `json:"firstT"`
	LastT      *float64 `json:"lastT"`
	DurationMS int64    `json:"durationMs"`
}
type SupplyDemandRetestLimits struct {
	MaxRequestBytes int    `json:"maxRequestBytes"`
	MaxSourceBytes  int    `json:"maxSourceBytes"`
	MaxOutputBytes  int    `json:"maxOutputBytes"`
	MaxRows         int    `json:"maxRows"`
	MaxWorkScore    uint64 `json:"maxWorkScore"`
}
type SupplyDemandRetestIdentity struct {
	RequestSHA256   string                           `json:"requestSha256"`
	SourceSHA256    string                           `json:"sourceSha256"`
	Case            string                           `json:"case"`
	StrategyID      string                           `json:"strategyId"`
	Symbol          string                           `json:"symbol"`
	Timeframe       string                           `json:"timeframe"`
	SourceTimeframe string                           `json:"sourceTimeframe"`
	HigherTimeframe string                           `json:"higherTimeframe"`
	RangeMethod     string                           `json:"rangeMethod"`
	HTFMode         string                           `json:"htfMode"`
	HTFPolicy       string                           `json:"htfPolicy"`
	Profile         string                           `json:"profile"`
	Basis           string                           `json:"basis"`
	TerminalPolicy  string                           `json:"terminalPolicy"`
	Chart           SupplyDemandRetestSeriesIdentity `json:"chart"`
	Source          SupplyDemandRetestSeriesIdentity `json:"source"`
	Daily           SupplyDemandRetestSeriesIdentity `json:"daily"`
	Costs           Costs                            `json:"costs"`
	Limits          SupplyDemandRetestLimits         `json:"limits"`
	WorkScore       uint64                           `json:"workScore"`
}
type admittedSupplyDemandRetest struct {
	fixture  RunFixture
	cfg      dsl.Config
	identity SupplyDemandRetestIdentity
}

func prepareSupplyDemandRetest(raw []byte, source string) (admittedSupplyDemandRetest, error) {
	var out admittedSupplyDemandRetest
	invalid := func(field string) (admittedSupplyDemandRetest, error) {
		return out, supplyDemandReportError("invalid-input", field, -1)
	}
	if len(raw) == 0 || len(raw) > SupplyDemandRetestMaxRequestBytes {
		return invalid("request.bytes")
	}
	if len(source) == 0 || len(source) > SupplyDemandRetestMaxSourceBytes || !utf8.ValidString(source) {
		return invalid("source.bytes")
	}
	if !supplyDemandFiniteSourceTokens(source) {
		return out, supplyDemandReportError("unsupported-profile", "source.numeric", -1)
	}
	if !supplyDemandJSONUnicode(raw) {
		return invalid("request.unicode")
	}
	fields, err := supplyDemandClosedObject(raw, []string{"schema", "case", "strategyId", "symbol", "timeframe", "sourceTimeframe", "rangeMethod", "costs", "bars", "sourceBars", "higherTimeframe", "sourceHtfBars"})
	if err != nil {
		return invalid("request.fields")
	}
	var f RunFixture
	for _, v := range []struct {
		key   string
		dst   *string
		max   int
		empty bool
	}{
		{"schema", &f.Schema, 64, false}, {"case", &f.Case, 256, false}, {"strategyId", &f.StrategyID, 128, false}, {"symbol", &f.Symbol, 32, false},
		{"timeframe", &f.Timeframe, 16, false}, {"sourceTimeframe", &f.SourceTimeframe, 16, false}, {"higherTimeframe", &f.HigherTimeframe, 16, true}, {"rangeMethod", &f.RangeMethod, 16, false},
	} {
		if len(fields[v.key]) == 0 || fields[v.key][0] != '"' || json.Unmarshal(fields[v.key], v.dst) != nil || !supplyDemandLabel(*v.dst, v.max, v.empty) {
			return invalid(v.key)
		}
	}
	if f.Schema != SupplyDemandRetestRequestSchema {
		return invalid("schema")
	}
	step := int64(0)
	switch f.Timeframe {
	case "15m":
		step = 900000
	case "30m":
		step = 1800000
	case "1h":
		step = 3600000
	}
	if step == 0 || f.SourceTimeframe != "4h" || f.RangeMethod != "zone" {
		return out, supplyDemandReportError("unsupported-route", "route", -1)
	}
	costs, err := supplyDemandClosedObject(fields["costs"], []string{"feePerUnit", "slippage", "slippageBps", "startEquity", "fillOn"})
	if err != nil {
		return invalid("costs.fields")
	}
	for _, v := range []struct {
		key string
		dst *float64
	}{{"feePerUnit", &f.Costs.FeePerUnit}, {"slippage", &f.Costs.Slippage}, {"slippageBps", &f.Costs.SlippageBps}, {"startEquity", &f.Costs.StartEquity}} {
		n, e := supplyDemandJSONNumber(costs[v.key])
		if e != nil || n < 0 || (v.key == "startEquity" && n == 0) {
			return invalid("costs." + v.key)
		}
		*v.dst = n
	}
	if json.Unmarshal(costs["fillOn"], &f.Costs.FillOn) != nil || (f.Costs.FillOn != "close" && f.Costs.FillOn != "open" && f.Costs.FillOn != "nextOpen") {
		return invalid("costs.fillOn")
	}
	parsed, err := dsl.Parse(source)
	if err != nil || len(parsed.Errors) > 0 {
		return out, supplyDemandReportError("unsupported-profile", "source.parse", -1)
	}
	cfg := parsed.Config
	if err := validateSupplyDemandReportConfig(cfg); err != nil {
		return out, err
	}
	enabled, err := supplyDemandEntryRetestEnabled(cfg)
	if err != nil || !enabled || setupTypeFromAny(cfg["setupType"]) != string(dsl.FamilySupplyDemand) {
		return out, supplyDemandReportError("unsupported-profile", "supplyDemand.retestOnEntryTimeframe", -1)
	}
	if validateSupplyDemandEntryRetest(cfg, f.Timeframe) != nil || !sourceEntryConfig(cfg, f) || !compiledRouteAllowed(cfg, f.Symbol, f.Timeframe) {
		return out, supplyDemandReportError("unsupported-route", "source.route", -1)
	}
	mode := stringValue(mapValue(cfg, "htf"), "mode", "off")
	dailyStep := int64(0)
	policy := "off"
	switch mode {
	case "off":
		if f.HigherTimeframe != "" {
			return out, supplyDemandReportError("unsupported-profile", "higherTimeframe", -1)
		}
	case "notAgainst":
		htf := stringValue(mapValue(cfg, "htf"), "timeframe", "auto")
		if f.HigherTimeframe != "1d" || (htf != "auto" && htf != "daily" && htf != "1d") {
			return out, supplyDemandReportError("unsupported-profile", "higherTimeframe", -1)
		}
		dailyStep = 86400000
		policy = SupplyDemandRetestHTFPolicy
	default:
		return out, supplyDemandReportError("unsupported-profile", "htf.mode", -1)
	}
	remaining := SupplyDemandRetestMaxRows
	f.Bars, err = supplyDemandReadBars(fields["bars"], "bars", step, remaining, false)
	if err != nil {
		return out, err
	}
	remaining -= len(f.Bars)
	f.SourceBars, err = supplyDemandReadBars(fields["sourceBars"], "sourceBars", 14400000, remaining, false)
	if err != nil {
		return out, err
	}
	remaining -= len(f.SourceBars)
	f.SourceHTFBars, err = supplyDemandReadBars(fields["sourceHtfBars"], "sourceHtfBars", dailyStep, remaining, mode == "off")
	if err != nil {
		return out, err
	}
	score, err := supplyDemandWorkScore(len(f.Bars), len(f.SourceBars), len(f.SourceHTFBars), cfg)
	if err != nil {
		return out, err
	}
	requestHash, sourceHash := sha256.Sum256(raw), sha256.Sum256([]byte(source))
	out = admittedSupplyDemandRetest{fixture: f, cfg: cfg, identity: SupplyDemandRetestIdentity{
		RequestSHA256: hex.EncodeToString(requestHash[:]), SourceSHA256: hex.EncodeToString(sourceHash[:]), Case: f.Case, StrategyID: f.StrategyID, Symbol: f.Symbol,
		Timeframe: f.Timeframe, SourceTimeframe: f.SourceTimeframe, HigherTimeframe: f.HigherTimeframe, RangeMethod: f.RangeMethod, HTFMode: mode, HTFPolicy: policy,
		Profile: SupplyDemandRetestProfile, Basis: SupplyDemandRetestBasis, TerminalPolicy: "last-close-liquidation", Costs: f.Costs,
		Chart: supplyDemandSeriesIdentity(f.Bars, step), Source: supplyDemandSeriesIdentity(f.SourceBars, 14400000), Daily: supplyDemandSeriesIdentity(f.SourceHTFBars, dailyStep),
		Limits: SupplyDemandRetestLimits{SupplyDemandRetestMaxRequestBytes, SupplyDemandRetestMaxSourceBytes, SupplyDemandRetestMaxOutputBytes, SupplyDemandRetestMaxRows, SupplyDemandRetestMaxWorkScore}, WorkScore: score,
	}}
	return out, nil
}

func supplyDemandSeriesIdentity(bars []marketdata.Bar, step int64) SupplyDemandRetestSeriesIdentity {
	out := SupplyDemandRetestSeriesIdentity{Count: len(bars), DurationMS: step}
	if len(bars) > 0 {
		first, last := bars[0].T, bars[len(bars)-1].T
		out.FirstT = &first
		out.LastT = &last
	}
	return out
}
func supplyDemandLabel(s string, max int, empty bool) bool {
	if (!empty && s == "") || len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func supplyDemandClosedObject(raw []byte, keys []string) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	t, e := d.Token()
	if e != nil || t != json.Delim('{') {
		return nil, fmt.Errorf("object")
	}
	allowed := make(map[string]bool, len(keys))
	for _, k := range keys {
		allowed[k] = true
	}
	out := make(map[string]json.RawMessage, len(keys))
	for d.More() {
		t, e = d.Token()
		if e != nil {
			return nil, e
		}
		k, ok := t.(string)
		if !ok || !allowed[k] || out[k] != nil {
			return nil, fmt.Errorf("field")
		}
		var v json.RawMessage
		if e = d.Decode(&v); e != nil {
			return nil, e
		}
		out[k] = v
	}
	if _, e = d.Token(); e != nil {
		return nil, e
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, fmt.Errorf("trailing")
	}
	if len(out) != len(keys) {
		return nil, fmt.Errorf("missing")
	}
	return out, nil
}
func supplyDemandJSONNumber(raw []byte) (float64, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	t, e := d.Token()
	n, ok := t.(json.Number)
	if e != nil || !ok {
		return 0, fmt.Errorf("number")
	}
	v, e := n.Float64()
	if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("finite")
	}
	if _, e = d.Token(); e != io.EOF {
		return 0, fmt.Errorf("trailing")
	}
	return v, nil
}
func supplyDemandReadBars(raw []byte, field string, step int64, remaining int, empty bool) ([]marketdata.Bar, error) {
	bad := func(index int) ([]marketdata.Bar, error) {
		return nil, supplyDemandReportError("invalid-input", field, index)
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	t, e := d.Token()
	if e != nil || t != json.Delim('[') {
		return bad(-1)
	}
	bars := make([]marketdata.Bar, 0)
	minDelta := float64(0)
	for d.More() {
		i := len(bars)
		if i >= remaining {
			return nil, supplyDemandReportError("invalid-input", "rows.limit", -1)
		}
		if empty {
			return nil, supplyDemandReportError("unsupported-profile", field, -1)
		}
		t, e = d.Token()
		if e != nil || t != json.Delim('[') {
			return bad(i)
		}
		var row [6]float64
		for j := 0; j < 6; j++ {
			if !d.More() {
				return bad(i)
			}
			t, e = d.Token()
			n, ok := t.(json.Number)
			if e != nil || !ok {
				return bad(i)
			}
			if j == 0 && !sequentialExactJSONTimestamp(string(n)) {
				return bad(i)
			}
			row[j], e = n.Float64()
			if e != nil || !isFiniteDerivedOutput(row[j]) {
				return bad(i)
			}
		}
		if d.More() {
			return bad(i)
		}
		if t, e = d.Token(); e != nil || t != json.Delim(']') {
			return bad(i)
		}
		if step <= 0 || row[0] > float64(1<<53-1)-float64(step) || math.Mod(row[0], float64(step)) != 0 {
			return bad(i)
		}
		if i > 0 {
			delta := row[0] - bars[i-1].T
			if delta <= 0 || math.Mod(delta, float64(step)) != 0 {
				return bad(i)
			}
			if minDelta == 0 || delta < minDelta {
				minDelta = delta
			}
		}
		o, h, l, c, v := row[1], row[2], row[3], row[4], row[5]
		if math.Abs(o) > 1e8 || math.Abs(h) > 1e8 || math.Abs(l) > 1e8 || math.Abs(c) > 1e8 || h < l || h < o || h < c || l > o || l > c || v < 0 || v > 1e12 {
			return bad(i)
		}
		bars = append(bars, marketdata.Bar{T: row[0], O: o, H: h, L: l, C: c, V: v})
	}
	if t, e = d.Token(); e != nil || t != json.Delim(']') {
		return bad(-1)
	}
	if _, e = d.Token(); e != io.EOF {
		return bad(-1)
	}
	if !empty && (len(bars) < 2 || minDelta != float64(step)) {
		return bad(-1)
	}
	return bars, nil
}

// encoding/json replaces lone escaped surrogates. Validate their original
// spelling first, in a single linear scan bounded by the request byte limit.
func supplyDemandJSONUnicode(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	in := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if !in {
			if c == '"' {
				in = true
			}
			continue
		}
		if c == '"' {
			in = false
			continue
		}
		if c != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		unit, ok := supplyDemandHexUnit(raw, i+1)
		if !ok {
			return false
		}
		i += 4
		if unit >= 0xdc00 && unit <= 0xdfff {
			return false
		}
		if unit >= 0xd800 && unit <= 0xdbff {
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			low, ok := supplyDemandHexUnit(raw, i+3)
			if !ok || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return !in
}
func supplyDemandHexUnit(raw []byte, i int) (uint16, bool) {
	if i+4 > len(raw) {
		return 0, false
	}
	var n uint16
	for _, b := range raw[i : i+4] {
		n <<= 4
		switch {
		case b >= '0' && b <= '9':
			n += uint16(b - '0')
		case b >= 'a' && b <= 'f':
			n += uint16(b - 'a' + 10)
		case b >= 'A' && b <= 'F':
			n += uint16(b - 'A' + 10)
		default:
			return 0, false
		}
	}
	return n, true
}

// Validate the entire parsed value tree before any legacy int conversion.
func validateSupplyDemandReportConfig(cfg dsl.Config) error {
	nodes, elements := 0, 0
	var walk func(any, string, int) error
	bad := func(field string) error { return supplyDemandReportError("unsupported-profile", field, -1) }
	walk = func(v any, path string, depth int) error {
		switch x := v.(type) {
		case int:
			v = float64(x)
		case int64:
			v = float64(x)
		case dsl.Config:
			v = map[string]any(x)
		}
		nodes++
		if nodes > 2048 || depth > 16 {
			return bad("config.shape")
		}
		switch x := v.(type) {
		case nil, bool:
			return nil
		case string:
			max := 256
			if strings.HasSuffix(path, ".name") || strings.HasSuffix(path, ".description") {
				max = 1024
			}
			if len(x) > max || !utf8.ValidString(x) {
				return bad("config.string")
			}
		case float64:
			limit := float64(1e6)
			if path == "config.riskUsd" {
				limit = 1e12
			}
			if !isFiniteDerivedOutput(x) || math.Abs(x) > limit || (path == "config.riskUsd" && x < 0) {
				return bad("config.numeric")
			}
		case map[string]any:
			if len(x) > 128 {
				return bad("config.map")
			}
			for k, v := range x {
				if len(k) > 256 || !utf8.ValidString(k) {
					return bad("config.key")
				}
				if err := walk(v, path+"."+k, depth+1); err != nil {
					return err
				}
			}
		case []any:
			if len(x) > 64 {
				return bad("config.array")
			}
			elements += len(x)
			if elements > 128 {
				return bad("config.arrays")
			}
			for _, v := range x {
				if err := walk(v, path+"[]", depth+1); err != nil {
					return err
				}
			}
		default:
			return bad("config.type")
		}
		return nil
	}
	if err := walk(cfg, "config", 0); err != nil {
		return err
	}
	if boolValue(mapValue(cfg, "partial"), "enabled", false) {
		return bad("partial.enabled")
	}
	if boolValue(mapValue(cfg, "channel"), "enabled", false) || stringValue(mapValue(cfg, "target"), "edge", "") == "channel" {
		return bad("channel")
	}
	if a, ok := cfg["seasonalityFilters"].([]any); ok && len(a) > 0 {
		return bad("seasonalityFilters")
	}
	if _, ok := cfg["relativeMeasuredVolatility"]; ok {
		return bad("relativeMeasuredVolatility")
	}
	sd := mapValue(cfg, "supplyDemand")
	count := func(m map[string]any, k string, def, min, max float64) error {
		v := numberValue(m, k, def)
		if v != math.Trunc(v) || v < min || v > max {
			return bad("config.count." + k)
		}
		return nil
	}
	for _, v := range []struct {
		k             string
		def, min, max float64
	}{{"minBaseCandles", 2, 1, 256}, {"maxBaseCandles", 5, 1, 256}, {"impulseCandles", 4, 1, 256}, {"sourceCandles", 4, 0, 256}, {"maxZoneAgeCandles", 72, 0, 1e6}, {"minWaitCandles", 1, 0, 1e6}, {"maxReactionCandles", 0, 0, 1e6}, {"maxRetests", 1, 0, 1e6}, {"chochCandles", 0, 0, 1e6}} {
		if err := count(sd, v.k, v.def, v.min, v.max); err != nil {
			return err
		}
	}
	minLook := float64(0)
	if numberValue(sd, "chochCandles", 0) > 0 {
		minLook = 1
	}
	if err := count(sd, "chochLookbackCandles", 3, minLook, 256); err != nil {
		return err
	}
	if numberValue(sd, "minBaseCandles", 2) > numberValue(sd, "maxBaseCandles", 5) {
		return bad("supplyDemand.baseOrder")
	}
	for _, v := range []struct {
		m   map[string]any
		k   string
		def float64
	}{{cfg, "cooldownCandles", 12}, {cfg, "maxHoldCandles", 0}, {mapValue(cfg, "entryMode"), "expireCandles", 5}, {mapValue(cfg, "stop"), "extremeCandles", 0}} {
		if err := count(v.m, v.k, v.def, 0, 1e6); err != nil {
			return err
		}
	}
	for _, k := range []string{"localHours", "blockedLocalHours", "newYorkHours", "blockedNewYorkHours"} {
		if a, ok := cfg[k].([]any); ok {
			for _, x := range a {
				v := numberFromAny(x, math.NaN())
				if !isFiniteDerivedOutput(v) || v != math.Trunc(v) || v < 0 || v > 23 {
					return bad(k)
				}
			}
		}
	}
	// Only explicit thresholds are bounded here. Absent internal infinity
	// defaults stay absent and retain their established unlimited meaning.
	for _, group := range []struct {
		m    map[string]any
		keys []string
	}{{sd, []string{"maxBaseBodyAtr", "maxBaseBodyToRange", "maxZoneAtr", "minSourceAtr", "impulseAtr", "minDepartureBodyToRange", "retestToleranceAtr", "invalidationAtr"}}, {mapValue(cfg, "stop"), []string{"minAtr", "maxAtr", "paddingAtr"}}, {mapValue(cfg, "target"), []string{"sdR", "minR", "fallbackR"}}, {mapValue(cfg, "breakeven"), []string{"atR", "offsetAtr"}}, {mapValue(cfg, "trail"), []string{"atr", "triggerR"}}, {cfg, []string{"riskUsd", "maxMovementEr", "dayTypeErEscape", "tailRejectionMin", "closeLocationMin"}}, {mapValue(cfg, "priorDay"), []string{"minRangeAtr"}}, {mapValue(cfg, "rangeBreakFake"), []string{"tailRejectionMin"}}} {
		for _, k := range group.keys {
			if v, ok := group.m[k]; ok && numberFromAny(v, math.NaN()) < 0 {
				return bad("config.threshold." + k)
			}
		}
	}
	stop := mapValue(cfg, "stop")
	if numberValue(stop, "minAtr", 0.5) > numberValue(stop, "maxAtr", math.Inf(1)) {
		return bad("stop.order")
	}
	return nil
}

func supplyDemandWorkScore(c, s, d int, cfg dsl.Config) (uint64, error) {
	bad := func() (uint64, error) { return 0, supplyDemandReportError("work-limit", "workScore", -1) }
	if c < 0 || s < 0 || d < 0 {
		return bad()
	}
	sd := mapValue(cfg, "supplyDemand")
	b := uint64(numberValue(sd, "maxBaseCandles", 5))
	min := uint64(numberValue(sd, "minBaseCandles", 2))
	imp := uint64(numberValue(sd, "impulseCandles", 4))
	look := uint64(0)
	if numberValue(sd, "chochCandles", 0) > 0 {
		look = uint64(numberValue(sd, "chochLookbackCandles", 3))
	}
	if b < min || b > 256 || imp > 256 || look > 256 {
		return bad()
	}
	k := b - min + 1
	factors := [][2]uint64{{uint64(s), k*(b+2*imp+512) + 16384}, {uint64(c), 16384 + 80*(look+64)}, {uint64(d), 512}}
	sum := uint64(0)
	for _, v := range factors {
		if v[0] > math.MaxUint64/v[1] {
			return bad()
		}
		term := v[0] * v[1]
		if term > SupplyDemandRetestMaxWorkScore || sum > SupplyDemandRetestMaxWorkScore-term {
			return bad()
		}
		sum += term
	}
	return sum, nil
}

// Reject explicit nonfinite/range-overflow numeric spellings before the legacy
// parser can discard them in favor of a fallback. Use the actual lexical token
// view, which splits '+', while retaining original scientific exponent spelling
// for a second bounded range-overflow check. Label-only directives are excluded.
func supplyDemandFiniteSourceTokens(source string) bool {
	numericLines := map[int]bool{}
	for _, directive := range dsl.OrdinarySourceDirectives(source) {
		if len(directive.Tokens) == 0 {
			continue
		}
		head := strings.ToLower(directive.Tokens[0])
		if directive.Section == "strategy" || head == "strategy" || head == "name" || head == "description" || head == "symbols" || head == "slices" || head == "timeframes" {
			continue
		}
		numericLines[directive.Line] = true
		for _, token := range directive.Tokens {
			token = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(token, "R"), "r"), "%")
			v, err := strconv.ParseFloat(token, 64)
			if errors.Is(err, strconv.ErrRange) || (err == nil && !isFiniteDerivedOutput(v)) {
				return false
			}
		}
	}
	for n, line := range strings.Split(source, "\n") {
		if numericLines[n+1] && !supplyDemandFiniteSourceExponent(line) {
			return false
		}
	}
	return true
}
func supplyDemandFiniteSourceExponent(source string) bool {
	var quote byte
	start := -1
	check := func(end int) bool {
		if start < 0 {
			return true
		}
		token := source[start:end]
		start = -1
		return supplyDemandFiniteJoinedToken(token)
	}
	for i := 0; i < len(source); i++ {
		c := source[i]
		if quote != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			if !check(i) {
				return false
			}
			quote = c
			continue
		}
		if c == '#' {
			return check(i)
		}
		if c <= ' ' || strings.ContainsRune("(),{}:<>=", rune(c)) {
			if !check(i) {
				return false
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	return check(len(source))
}

// The ordinary lexer separates '+' everywhere, including authored exponent
// signs. Retain a sign only after a valid exponent prefix (1e, 0x1p, ...), so a
// joined operand such as 2.5+1e+309 is checked as 2.5 and 1e+309. Numeric parsing
// is linear/bounded in the source token; exponent magnitude never allocates.
func supplyDemandFiniteJoinedToken(token string) bool {
	check := func(part string) bool {
		part = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(part, "R"), "r"), "%")
		_, err := strconv.ParseFloat(part, 64)
		return !errors.Is(err, strconv.ErrRange)
	}
	start := 0
	for i := 0; i < len(token); i++ {
		if token[i] != '+' || i == start {
			continue
		}
		prefix := token[start:i]
		exponentSign := false
		if len(prefix) > 0 && strings.ContainsRune("eEpP", rune(prefix[len(prefix)-1])) {
			_, err := strconv.ParseFloat(prefix+"0", 64)
			exponentSign = err == nil
		}
		if exponentSign {
			continue
		}
		if !check(prefix) {
			return false
		}
		start = i + 1
	}
	return check(token[start:])
}
