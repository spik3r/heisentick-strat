package engine

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// The ema-ribbon family (spec/dsl-spec-families/emaRibbon.md) follows a
// trend while a ribbon of EMAs stays in order. Everything it decides depends
// only on bars up to and including the decision bar, so the whole rule is
// precomputed once per run into the two setup columns the broker already
// carries (b.ema and b.emaSlope), and the per-bar step only places orders.
// No broker or flagParams field is added.
//
//	b.ema[i]       signed stop distance for an entry decided at bar i's close:
//	               +d long, -d short, 0 none (before position and side checks)
//	b.emaSlope[i]  exit permission at bar i's close: bit 1 lets a long exit
//	               (ribbon no longer long-aligned, inside the act window),
//	               bit 2 lets a short exit
const (
	emaRibbonExitLong  = 1
	emaRibbonExitShort = 2
)

type emaRibbonTrendFilter struct {
	hours      float64
	pair       bool
	fast, slow int
}

type emaRibbonSpec struct {
	emas          []int
	pullback      int
	filter        *emaRibbonTrendFilter
	shortDailyEMA int
	hasWindow     bool
	actFrom       int
	actTo         int
	stopATR       float64
	targetR       float64
}

var errEMARibbonConfig = errors.New("ema ribbon config is invalid")

// emaRibbonMaxTargetR bounds `target NR`; the parser refuses larger values,
// so an authored target can never overflow into "no target".
const emaRibbonMaxTargetR = 1000

// emaRibbonBarMs is the explicit bar length of each supported route
// timeframe. Decisions use it instead of anything inferred from the series,
// so appending bars can never change an earlier decision.
var emaRibbonBarMs = map[string]float64{
	"1m": 60000, "5m": 300000, "15m": 900000, "30m": 1800000, "1h": 3600000, "2h": 7200000, "4h": 14400000,
}

// decodeEMARibbonSpec reads the parsed config strictly; a malformed raw
// config is refused rather than defaulted.
func decodeEMARibbonSpec(cfg dsl.Config) (emaRibbonSpec, error) {
	var spec emaRibbonSpec
	family, err := donchianConfigMap(cfg, "emaRibbon")
	if err != nil || family == nil {
		return spec, errEMARibbonConfig
	}
	rawEMAs, ok := family["emas"].([]any)
	if !ok || len(rawEMAs) < 2 || len(rawEMAs) > 8 {
		return spec, errEMARibbonConfig
	}
	previous := 0
	for _, raw := range rawEMAs {
		n, ok := donchianCount(raw)
		if !ok || n <= previous || n > 100000 {
			return spec, errEMARibbonConfig
		}
		spec.emas = append(spec.emas, n)
		previous = n
	}
	if raw, present := family["pullbackEma"]; present {
		n, ok := donchianCount(raw)
		if !ok || n > 100000 {
			return spec, errEMARibbonConfig
		}
		spec.pullback = n
	}
	if raw, present := family["trendFilter"]; present {
		filter, ok := raw.(map[string]any)
		if !ok {
			return spec, errEMARibbonConfig
		}
		hours := map[string]float64{"2h": 2, "4h": 4, "8h": 8, "1d": 24}[emaRibbonString(filter["timeframe"])]
		if hours == 0 {
			return spec, errEMARibbonConfig
		}
		spec.filter = &emaRibbonTrendFilter{hours: hours}
		switch emaRibbonString(filter["mode"]) {
		case "ribbon":
		case "pair":
			fast, fastOK := donchianCount(filter["fast"])
			slow, slowOK := donchianCount(filter["slow"])
			if !fastOK || !slowOK || fast >= slow || slow > 100000 {
				return spec, errEMARibbonConfig
			}
			spec.filter.pair, spec.filter.fast, spec.filter.slow = true, fast, slow
		default:
			return spec, errEMARibbonConfig
		}
	}
	if raw, present := family["shortDailyEma"]; present {
		n, ok := donchianCount(raw)
		if !ok || n > 100000 {
			return spec, errEMARibbonConfig
		}
		spec.shortDailyEMA = n
	}
	_, hasFrom := family["actFromUtc"]
	_, hasTo := family["actToUtc"]
	if hasFrom || hasTo {
		from, fromOK := donchianNumber(family["actFromUtc"])
		to, toOK := donchianNumber(family["actToUtc"])
		if !fromOK || !toOK || from != math.Trunc(from) || to != math.Trunc(to) || from < 0 || from > 23 || to < 0 || to > 23 || from == to {
			return spec, errEMARibbonConfig
		}
		spec.hasWindow, spec.actFrom, spec.actTo = true, int(from), int(to)
	}
	stop, err := donchianConfigMap(cfg, "stop")
	if err != nil || stop == nil || emaRibbonString(stop["type"]) != "atr" {
		return spec, errEMARibbonConfig
	}
	stopATR, ok := donchianNumber(stop["atrMult"])
	if !ok || !(stopATR > 0) {
		return spec, errEMARibbonConfig
	}
	spec.stopATR = stopATR
	target, err := donchianConfigMap(cfg, "target")
	if err != nil {
		return spec, errEMARibbonConfig
	}
	if target != nil {
		if raw, present := target["erR"]; present {
			r, ok := donchianNumber(raw)
			if !ok || r < 0 || r > emaRibbonMaxTargetR {
				return spec, errEMARibbonConfig
			}
			spec.targetR = r
		}
	}
	return spec, nil
}

func emaRibbonString(value any) string {
	s, _ := value.(string)
	return s
}

// validateEMARibbonConfig refuses an ema-ribbon config the family cannot run.
func validateEMARibbonConfig(cfg dsl.Config) error {
	if setupTypeFromAny(cfg["setupType"]) != string(dsl.FamilyEMARibbon) {
		return nil
	}
	if source, ok := cfg["sourceTimeframe"].(string); ok && source != "" {
		return errors.New("ema ribbon does not support source timeframe execution")
	}
	_, err := decodeEMARibbonSpec(cfg)
	return err
}

// validateEMARibbonRun refuses a run the family cannot evaluate causally: an
// unsupported route timeframe, a trend filter not coarser than and aligned
// with the bar, or a series that is malformed, off the timeframe's UTC grid,
// or spaced more finely than one bar.
func validateEMARibbonRun(cfg dsl.Config, timeframe string, series marketdata.Series) error {
	if setupTypeFromAny(cfg["setupType"]) != string(dsl.FamilyEMARibbon) {
		return nil
	}
	spec, err := decodeEMARibbonSpec(cfg)
	if err != nil {
		return err
	}
	barMs, ok := emaRibbonBarMs[timeframe]
	if !ok {
		return fmt.Errorf("ema ribbon: unsupported route timeframe %q (supported: 1m 5m 15m 30m 1h 2h 4h)", timeframe)
	}
	if spec.filter != nil {
		width := spec.filter.hours * 3600000
		if width <= barMs || math.Mod(width, barMs) != 0 {
			return fmt.Errorf("ema ribbon: trend filter timeframe must be coarser than and a multiple of the %s route", timeframe)
		}
	}
	if err := validateSeriesShape("market", series); err != nil {
		return fmt.Errorf("ema ribbon: malformed series: %w", err)
	}
	if err := validateSeriesValues("market", series); err != nil {
		return fmt.Errorf("ema ribbon: malformed series: %w", err)
	}
	for i := 0; i < series.Len(); i++ {
		if series.H[i] < series.L[i] || series.O[i] > series.H[i] || series.O[i] < series.L[i] || series.C[i] > series.H[i] || series.C[i] < series.L[i] {
			return fmt.Errorf("ema ribbon: malformed series: bar %d has inconsistent OHLC", i)
		}
		if math.Mod(series.T[i], barMs) != 0 {
			return fmt.Errorf("ema ribbon: bar %d is not on the %s UTC grid", i, timeframe)
		}
		if i > 0 && series.T[i]-series.T[i-1] < barMs {
			return fmt.Errorf("ema ribbon: bar %d is less than one %s bar after the previous bar", i, timeframe)
		}
	}
	return nil
}

// emaRibbonEMA is an EMA of values seeded with the first value; index j is
// valid once j >= 3 x length (the warm-up the research harness uses).
func emaRibbonEMA(values []float64, length int) []float64 {
	out := make([]float64, len(values))
	k := 2 / (float64(length) + 1)
	value := 0.0
	for j, v := range values {
		if j == 0 {
			value = v
		} else {
			value += k * (v - value)
		}
		if j >= 3*length {
			out[j] = value
		} else {
			out[j] = math.NaN()
		}
	}
	return out
}

// emaRibbonAlignment returns +1 when every value is strictly above the next,
// -1 when strictly below, 0 otherwise or when any value is not finite.
func emaRibbonAlignment(values []float64) int {
	up, down := true, true
	for k, v := range values {
		if !isFinite(v) {
			return 0
		}
		if k > 0 {
			if !(values[k-1] > v) {
				up = false
			}
			if !(values[k-1] < v) {
				down = false
			}
		}
	}
	switch {
	case up:
		return 1
	case down:
		return -1
	}
	return 0
}

// emaRibbonBuckets groups bars into UTC-aligned buckets of the given width
// and returns, for each bar i, the index of the last bucket fully closed by
// that bar's close (T[i] + barDuration), or -1. Each bucket's close is the
// close of its last bar.
func emaRibbonBuckets(series marketdata.Series, widthMs, barMs float64) ([]float64, []int) {
	var closes, ends []float64
	last := math.NaN()
	for i := 0; i < series.Len(); i++ {
		key := series.T[i] - math.Mod(series.T[i], widthMs)
		if key != last {
			closes = append(closes, series.C[i])
			ends = append(ends, key+widthMs)
			last = key
		} else {
			closes[len(closes)-1] = series.C[i]
		}
	}
	done := make([]int, series.Len())
	p := -1
	for i := 0; i < series.Len(); i++ {
		decision := series.T[i] + barMs
		for p+1 < len(ends) && ends[p+1] <= decision {
			p++
		}
		done[i] = p
	}
	return closes, done
}

// emaRibbonATR is Wilder's ATR(14) seeded with the first bar's range; index
// i is valid once i >= 14.
func emaRibbonATR(series marketdata.Series) []float64 {
	out := make([]float64, series.Len())
	value := 0.0
	for i := 0; i < series.Len(); i++ {
		tr := series.H[i] - series.L[i]
		if i > 0 {
			tr = math.Max(tr, math.Max(math.Abs(series.H[i]-series.C[i-1]), math.Abs(series.L[i]-series.C[i-1])))
			value += (tr - value) / 14
		} else {
			value = tr
		}
		if i >= 14 {
			out[i] = value
		} else {
			out[i] = math.NaN()
		}
	}
	return out
}

func emaRibbonInWindow(spec emaRibbonSpec, t float64) bool {
	if !spec.hasWindow {
		return true
	}
	h := time.UnixMilli(int64(t)).UTC().Hour()
	if spec.actFrom < spec.actTo {
		return h >= spec.actFrom && h < spec.actTo
	}
	return h >= spec.actFrom || h < spec.actTo
}

// computeEMARibbonColumns builds the entry and exit columns described above.
// barMs is the route timeframe's bar length; each decision at bar i reads
// only bars 0..i and the constant barMs, so it is prefix invariant.
func computeEMARibbonColumns(series marketdata.Series, spec emaRibbonSpec, barMs float64) ([]float64, []float64) {
	n := series.Len()
	entry := make([]float64, n)
	exit := make([]float64, n)
	if n == 0 || !(barMs > 0) {
		return entry, exit
	}
	lines := make([][]float64, len(spec.emas))
	for k, length := range spec.emas {
		lines[k] = emaRibbonEMA(series.C, length)
	}
	var pullback []float64
	if spec.pullback > 0 {
		pullback = emaRibbonEMA(series.C, spec.pullback)
	}
	atr := emaRibbonATR(series)
	align := make([]int, n)
	values := make([]float64, len(lines))
	for i := 0; i < n; i++ {
		for k := range lines {
			values[k] = lines[k][i]
		}
		align[i] = emaRibbonAlignment(values)
	}

	var filterDir []int
	if spec.filter != nil && barMs > 0 {
		closes, done := emaRibbonBuckets(series, spec.filter.hours*3600000, barMs)
		lengths := spec.emas
		if spec.filter.pair {
			lengths = []int{spec.filter.fast, spec.filter.slow}
		}
		htf := make([][]float64, len(lengths))
		for k, length := range lengths {
			htf[k] = emaRibbonEMA(closes, length)
		}
		filterDir = make([]int, n)
		bucketValues := make([]float64, len(htf))
		for i := 0; i < n; i++ {
			j := done[i]
			if j < 0 {
				continue
			}
			for k := range htf {
				bucketValues[k] = htf[k][j]
			}
			filterDir[i] = emaRibbonAlignment(bucketValues)
		}
	}
	var dailyBias []int
	if spec.shortDailyEMA > 0 && barMs > 0 {
		closes, done := emaRibbonBuckets(series, 24*3600000, barMs)
		daily := emaRibbonEMA(closes, spec.shortDailyEMA)
		dailyBias = make([]int, n)
		for i := 0; i < n; i++ {
			j := done[i]
			if j < 0 || !isFinite(daily[j]) {
				continue
			}
			switch {
			case closes[j] > daily[j]:
				dailyBias[i] = 1
			case closes[j] < daily[j]:
				dailyBias[i] = -1
			}
		}
	}

	for i := 0; i < n; i++ {
		inWindow := barMs > 0 && emaRibbonInWindow(spec, series.T[i]+barMs)
		if inWindow {
			flags := 0
			if align[i] != 1 {
				flags |= emaRibbonExitLong
			}
			if align[i] != -1 {
				flags |= emaRibbonExitShort
			}
			exit[i] = float64(flags)
		}
		a := align[i]
		if a == 0 || i == 0 || !inWindow || !isFinite(atr[i]) {
			continue
		}
		if align[i-1] == a {
			// Already aligned: only a pullback re-entry can trigger.
			if pullback == nil || !isFinite(pullback[i-1]) {
				continue
			}
			if a > 0 && !(series.C[i-1] < pullback[i-1]) || a < 0 && !(series.C[i-1] > pullback[i-1]) {
				continue
			}
		}
		if a > 0 && !(series.C[i] > lines[0][i]) || a < 0 && !(series.C[i] < lines[0][i]) {
			continue
		}
		if filterDir != nil && filterDir[i] != a {
			continue
		}
		if a < 0 && dailyBias != nil && dailyBias[i] != -1 {
			continue
		}
		distance := spec.stopATR * atr[i]
		if !isFinite(distance) || !(distance > 0) {
			continue
		}
		entry[i] = float64(a) * distance
	}
	return entry, exit
}

// computeSetupColumns returns the broker's setup columns for a run: the
// ema-ribbon family's precomputed decisions, or the shared setup EMA.
func computeSetupColumns(series marketdata.Series, params flagParams, cfg dsl.Config, timeframe string) ([]float64, []float64) {
	if params.SetupType == string(dsl.FamilyEMARibbon) {
		// validateEMARibbonRun refuses an undecodable config or unsupported
		// timeframe on every run path before this point; empty columns are a
		// fail-closed fallback.
		spec, err := decodeEMARibbonSpec(cfg)
		if err != nil {
			return make([]float64, series.Len()), make([]float64, series.Len())
		}
		return computeEMARibbonColumns(series, spec, emaRibbonBarMs[timeframe])
	}
	return computeSetupEMA(series, params)
}

func (b *broker) emaRibbonExitPending(entryIndex int) bool {
	for _, pending := range b.pendingExits {
		if pending.PositionEntryIndex == entryIndex {
			return true
		}
	}
	return false
}

func (b *broker) onEMARibbonBar(i int) {
	if i >= len(b.ema) || i >= len(b.emaSlope) {
		return
	}
	if b.hasPosition {
		flags := int(b.emaSlope[i])
		want := emaRibbonExitLong
		if b.position.Side == sideShort {
			want = emaRibbonExitShort
		}
		if flags&want != 0 && !b.emaRibbonExitPending(b.position.EntryIndex) {
			b.pendingExits = append(b.pendingExits, pendingExit{PositionEntryIndex: b.position.EntryIndex, Index: i + 1, Rule: "ribbon-break"})
		}
		return
	}
	// No entry on a bar where a position closed, or while an order waits.
	if b.lastExitIndex == i || len(b.pendingOrders) > 0 {
		return
	}
	signal := b.ema[i]
	if !isFinite(signal) || signal == 0 {
		return
	}
	s := sideLong
	if signal < 0 {
		s = sideShort
	}
	p := b.params
	if s == sideLong && !p.AllowLong || s == sideShort && !p.AllowShort {
		return
	}
	distance := math.Abs(signal)
	ord := order{
		Side: s, RiskUSD: p.RiskUSD, HasRisk: true, Tag: "DSL-ER", Index: i + 1,
		StopDistance: distance, HasStopDistance: true, GapAwareStop: true, NoTarget: true,
		Meta: TradeMeta{"setup": "emaRibbon", "side": s.String(), "signalIndex": float64(i), "stopDistance": distance},
	}
	if p.TargetR > 0 {
		// An authored target must never degrade to "no target": when the
		// target distance or price would not be finite, take no trade.
		targetDistance := p.TargetR * distance
		close := b.series.C[i]
		if !isFinite(targetDistance) || !(targetDistance > 0) || !isFinite(close+targetDistance) || !isFinite(close-targetDistance) {
			return
		}
		ord.TargetDistance = targetDistance
		ord.HasTargetDistance = true
		ord.NoTarget = false
	}
	b.pendingOrders = append(b.pendingOrders, ord)
}
