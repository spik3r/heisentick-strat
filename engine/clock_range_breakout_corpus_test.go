package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/testsupport"
)

// The clock range breakout conformance corpus. Every scenario below carries its
// hand-derived expectation; the committed fixtures are checked byte-for-byte
// against these builders and the committed goldens are generated from the
// fixtures by `go run ./cmd/conformance regen`. Run with CRB_WRITE_CORPUS=1 to
// rewrite the fixture, program and parse inputs from this table.

type crbCorpusCase struct {
	name      string
	group     string // acceptance group(s) the case pins
	opts      crbOptions
	symbol    string
	timeframe string
	costs     Costs
	bars      []marketdata.Bar
	want      []crbWant
	// outcome is the expected audit outcome and reason of the first range day.
	outcome, reason string
}

const crbCorpusPrefix = "family-clock-range-breakout-"

func crbCorpusBarsAt(start float64, low, high float64, skip []int, after ...marketdata.Bar) []marketdata.Bar {
	return append(crbRangeBars(start, low, high, skip...), after...)
}

func crbCorpusCases() []crbCorpusCase {
	closeAt := func(open float64) marketdata.Bar { return crbBar(crbBaseClose, open, open+1, open-1, open) }
	enterLong := crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101)
	enterShort := crbBar(crbBaseRangeEnd, 99.5, 99.8, 98.5, 99)
	closeRule := func(w crbWant, exit float64) crbWant {
		w.exit, w.reason, w.rule, w.exitReason = exit, "rule", "clock-range-close", "clock"
		return w
	}
	baseMeta := func(w crbWant) crbWant {
		w.checkMeta, w.rangeHigh, w.rangeLow, w.rangeBars, w.dayKey, w.orderPlacedAt = true, 101, 99, 36, "2026-03-10", crbBaseRangeEnd
		return w
	}
	day := 24 * 60 * 60 * 1000.0
	oneMinute := func(bars []marketdata.Bar) []marketdata.Bar {
		var out []marketdata.Bar
		for _, bar := range bars {
			for k := 0; k < 5; k++ {
				out = append(out, marketdata.Bar{T: bar.T + float64(k)*60000, O: bar.O, H: bar.H, L: bar.L, C: bar.C, V: 1})
			}
		}
		return out
	}
	shape := crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 100.3)
	shape.V = 5e9
	longLong := baseMeta(closeRule(crbWant{side: "long", entry: 101, sl: 99.99, checkIndices: true, entryIndex: 36, exitIdx: 37}, 105))
	fractional := func(start float64, offset, rng, closeText string, closeT float64, key string) ([]marketdata.Bar, crbOptions, crbWant) {
		end := start + 36*crbFiveMinutes
		bars := crbCorpusBarsAt(start, 99, 101, nil, crbBar(end, 100.5, 101.5, 100.2, 101), crbBar(closeT, 102, 103, 101, 102))
		want := crbWant{side: "long", entry: 101, exit: 102, reason: "rule", rule: "clock-range-close", sl: 99.99, exitReason: "clock",
			checkMeta: true, rangeHigh: 101, rangeLow: 99, rangeBars: 36, dayKey: key, orderPlacedAt: end,
			checkTimes: true, entryT: end, exitT: closeT, checkIndices: true, entryIndex: 36, exitIdx: 37}
		return bars, crbOptions{offset: offset, rng: rng, closeAt: closeText}, want
	}
	midnightBars, midnightOpts, midnightWant := fractional(crbUTC(2026, time.March, 9, 22, 0), "UTC+10", "08:00 to 11:00", "20:00", crbUTC(2026, time.March, 10, 10, 0), "2026-03-10")
	yearBars, yearOpts, yearWant := fractional(crbUTC(2026, time.January, 1, 3, 5), "UTC-8", "19:05 to 22:05", "10:00", crbUTC(2026, time.January, 1, 18, 0), "2025-12-31")
	minusBars, minusOpts, minusWant := fractional(crbUTC(2026, time.March, 10, 12, 35), "UTC-3:30", "09:05 to 12:05", "20:00", crbUTC(2026, time.March, 10, 23, 30), "2026-03-10")
	plusBars, plusOpts, plusWant := fractional(crbUTC(2026, time.March, 10, 2, 35), "UTC+5:30", "08:05 to 11:05", "22:00", crbUTC(2026, time.March, 10, 16, 30), "2026-03-10")
	yearEndBars, yearEndOpts, yearEndWant := fractional(crbUTC(2026, time.December, 31, 1, 5), "UTC+10", "11:05 to 14:05", "03:00", crbUTC(2026, time.December, 31, 17, 0), "2026-12-31")
	monthEndBars, monthEndOpts, monthEndWant := fractional(crbUTC(2026, time.February, 28, 1, 5), "UTC+10", "11:05 to 14:05", "03:00", crbUTC(2026, time.February, 28, 17, 0), "2026-02-28")
	leapBars, leapOpts, leapWant := fractional(crbUTC(2028, time.February, 29, 1, 5), "UTC+10", "11:05 to 14:05", "03:00", crbUTC(2028, time.February, 29, 17, 0), "2028-02-29")

	var multi []marketdata.Bar
	multi = append(multi, crbRangeBars(crbBaseRangeStart, 99, 101)...)
	multi = append(multi, enterLong, crbBar(crbBaseClose, 102, 103, 101, 102))
	multi = append(multi, crbRangeBars(crbBaseRangeStart+day, 99, 101)...)
	multi = append(multi, crbBar(crbBaseRangeEnd+day, 99.5, 99.8, 98.5, 99), crbBar(crbBaseClose+day, 98, 99, 97, 98))
	multi = append(multi, crbRangeBars(crbBaseRangeStart+2*day, 99, 101, 1, 2, 4, 5, 6, 8)...)
	multi = append(multi, crbBar(crbBaseRangeEnd+2*day, 100.5, 101.5, 100.2, 101), crbBar(crbBaseClose+2*day, 102, 103, 101, 102))

	var flat []marketdata.Bar
	for slot := 0; slot < 36; slot++ {
		flat = append(flat, crbBar(crbBaseRangeStart+float64(slot)*crbFiveMinutes, 100, 100, 100, 100))
	}
	flat = append(flat, enterLong, closeAt(102))

	return []crbCorpusCase{
		{name: "numeric-tie", group: "8", opts: crbOptions{risk: "101"},
			bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, crbBar(crbBaseRangeEnd, 100, 102, 98, 100), crbBar(crbEntryAt(1), 100, 103, 97, 100)),
			want: []crbWant{baseMeta(crbWant{side: "long", entry: 101, exit: 99.99, reason: "sl", sl: 99.99, exitReason: "stop", checkIndices: true, entryIndex: 36, exitIdx: 36, checkSize: true, size: 100})}},
		{name: "ordinary-long", group: "5,1", bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, enterLong, closeAt(105)), want: []crbWant{longLong}},
		{name: "ordinary-long-utc2", group: "1,11", opts: crbOptions{offset: "UTC+2", rng: "03:05 to 06:05", closeAt: "19:00"},
			bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, enterLong, closeAt(105)), want: []crbWant{longLong}},
		{name: "gap-through-long", group: "5", bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, crbBar(crbBaseRangeEnd, 102, 103, 101.5, 102.5), closeAt(105)),
			want: []crbWant{baseMeta(closeRule(crbWant{side: "long", entry: 102, sl: 100.98, checkIndices: true, entryIndex: 36, exitIdx: 37}, 105))}},
		{name: "ordinary-short", group: "5", bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, enterShort, closeAt(97)),
			want: []crbWant{baseMeta(closeRule(crbWant{side: "short", entry: 99, sl: 99.99, checkIndices: true, entryIndex: 36, exitIdx: 37}, 97))}},
		{name: "gap-through-short", group: "5", bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, crbBar(crbBaseRangeEnd, 98, 98.5, 97, 97.5), closeAt(97)),
			want: []crbWant{baseMeta(closeRule(crbWant{side: "short", entry: 98, sl: 98.98, checkIndices: true, entryIndex: 36, exitIdx: 37}, 97))}},
		{name: "nearer-trigger-short", group: "5,6", bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, crbBar(crbBaseRangeEnd, 99.7, 102, 98, 100)),
			want: []crbWant{baseMeta(crbWant{side: "short", entry: 99, exit: 99.99, reason: "sl", sl: 99.99, exitReason: "stop", checkIndices: true, entryIndex: 36, exitIdx: 36})}},
		{name: "long-only-sell-touch", group: "5", opts: crbOptions{side: "long only"}, bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, enterShort, closeAt(100)),
			outcome: ClockDayExpired},
		{name: "expiry-bar-excluded", group: "5", opts: crbOptions{expire: "15:00"},
			bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, crbBar(crbBaseRangeEnd, 100, 100.4, 99.6, 100),
				crbBar(crbUTC(2026, time.March, 10, 5, 0), 100, 101.5, 100, 101), crbBar(crbUTC(2026, time.March, 10, 5, 5), 100, 105, 95, 100), closeAt(102)),
			outcome: ClockDayExpired},
		{name: "expiry-last-bar-fills", group: "5", opts: crbOptions{expire: "15:00"},
			bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, crbBar(crbBaseRangeEnd, 100, 100.4, 99.6, 100),
				crbBar(crbUTC(2026, time.March, 10, 4, 55), 100, 101.5, 100, 101), closeAt(102)),
			want: []crbWant{baseMeta(closeRule(crbWant{side: "long", entry: 101, sl: 99.99, checkIndices: true, entryIndex: 37, exitIdx: 38}, 102))}},
		{name: "later-gap-stop", group: "6", bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, enterLong, crbBar(crbEntryAt(1), 99.5, 99.8, 99, 99.2)),
			want: []crbWant{baseMeta(crbWant{side: "long", entry: 101, exit: 99.5, reason: "sl", sl: 99.99, exitReason: "stop", checkIndices: true, entryIndex: 36, exitIdx: 37})}},
		{name: "later-touch-stop", group: "6", bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, enterLong, crbBar(crbEntryAt(1), 101, 101.2, 99.9, 100), crbBar(crbEntryAt(2), 100, 105, 95, 100)),
			want: []crbWant{baseMeta(crbWant{side: "long", entry: 101, exit: 99.99, reason: "sl", sl: 99.99, exitReason: "stop", checkIndices: true, entryIndex: 36, exitIdx: 37})}},
		{name: "clock-close-before-extremes", group: "6", bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, enterLong, crbBar(crbBaseClose, 101.5, 120, 50, 100)),
			want: []crbWant{baseMeta(closeRule(crbWant{side: "long", entry: 101, sl: 99.99, checkIndices: true, entryIndex: 36, exitIdx: 37}, 101.5))}},
		{name: "missing-close-quote", group: "7", bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 100.9), crbBar(crbUTC(2026, time.March, 10, 17, 35), 100.8, 130, 50, 100)),
			want: []crbWant{baseMeta(closeRule(crbWant{side: "long", entry: 101, sl: 99.99, checkIndices: true, entryIndex: 36, exitIdx: 37, checkTimes: true, entryT: crbBaseRangeEnd, exitT: crbUTC(2026, time.March, 10, 17, 35)}, 100.8))}},
		{name: "terminal-last-close", group: "7", bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 100.9)),
			want: []crbWant{baseMeta(crbWant{side: "long", entry: 101, exit: 100.9, reason: "end-of-test", sl: 99.99, exitReason: "end-of-data", checkIndices: true, entryIndex: 36, exitIdx: 36})}},
		{name: "coverage-33-accepted", group: "4", bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, []int{1, 2, 4}, enterLong, closeAt(105)),
			want: []crbWant{func() crbWant { w := longLong; w.rangeBars, w.entryIndex, w.exitIdx = 33, 33, 34; return w }()}},
		{name: "coverage-32-rejected", group: "4", bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, []int{1, 2, 4, 5}, enterLong, closeAt(105)),
			outcome: ClockDayRejected, reason: ClockReasonInsufficient},
		{name: "coverage-35-missing-final-slot", group: "4,12", bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, []int{35}, enterLong, closeAt(105)),
			outcome: ClockDayRejected, reason: ClockReasonMissingFinalSlot},
		{name: "flat-range-rejected", group: "4", bars: flat, outcome: ClockDayRejected, reason: ClockReasonNonpositiveWidth},
		{name: "pip-buffer-xauusd", group: "9", opts: crbOptions{buffer: "10", side: "long only"},
			bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, crbBar(crbBaseRangeEnd, 101.5, 102.2, 101.2, 102), closeAt(103)),
			want: []crbWant{baseMeta(closeRule(crbWant{side: "long", entry: 102, sl: 100.98, checkIndices: true, entryIndex: 36, exitIdx: 37}, 103))}},
		{name: "pip-buffer-eurusd-short", group: "9", symbol: "EURUSD", opts: crbOptions{buffer: "15", side: "short only", route: "slices(EURUSD 5m)"},
			bars: append(crbRangeBars(crbBaseRangeStart, 1.0990, 1.1010), crbBar(crbBaseRangeEnd, 1.0985, 1.0988, 1.0970, 1.0972), crbBar(crbBaseClose, 1.0960, 1.0970, 1.0950, 1.0960)),
			want: []crbWant{{side: "short", entry: 1.0975, exit: 1.0960, reason: "rule", rule: "clock-range-close", sl: 1.0975 * 1.01, exitReason: "clock",
				checkMeta: true, rangeHigh: 1.1010, rangeLow: 1.0990, rangeBars: 36, dayKey: "2026-03-10", orderPlacedAt: crbBaseRangeEnd}}},
		{name: "slippage-fee-sizing", group: "9", opts: crbOptions{risk: "101.5"}, costs: Costs{Slippage: 0.5, FeePerUnit: 0.01},
			bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, crbBar(crbBaseRangeEnd, 100.8, 101.4, 100.6, 101), closeAt(103)),
			want: []crbWant{baseMeta(closeRule(crbWant{side: "long", entry: 101.5, sl: 100.485, checkIndices: true, entryIndex: 36, exitIdx: 37, checkSize: true, size: 100}, 102.5))}},
		{name: "explicit-zero-risk", group: "9", opts: crbOptions{risk: "0"}, bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, enterLong, closeAt(105)),
			want: []crbWant{func() crbWant { w := longLong; w.checkSize, w.size = true, 0; return w }()}},
		{name: "no-inherited-management", group: "9", bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, enterLong,
			crbBar(crbEntryAt(1), 101, 103, 100.9, 102.9), crbBar(crbEntryAt(2), 102.9, 103, 100.5, 101), crbBar(crbBaseClose, 101.2, 104, 100, 101)),
			want: []crbWant{baseMeta(closeRule(crbWant{side: "long", entry: 101, sl: 99.99, checkIndices: true, entryIndex: 36, exitIdx: 39}, 101.2))}},
		{name: "fill-on-open", group: "10", costs: Costs{FillOn: "open"}, bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, enterLong, closeAt(105)), want: []crbWant{longLong}},
		{name: "fill-bar-shape", group: "10", bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, shape, closeAt(105)), want: []crbWant{longLong}},
		{name: "midnight-range", group: "1", bars: midnightBars, opts: midnightOpts, want: []crbWant{midnightWant}},
		{name: "year-boundary-negative-offset", group: "1", bars: yearBars, opts: yearOpts, want: []crbWant{yearWant}},
		{name: "fractional-offset-minus-3-30", group: "1", bars: minusBars, opts: minusOpts, want: []crbWant{minusWant}},
		{name: "fractional-offset-plus-5-30", group: "1", bars: plusBars, opts: plusOpts, want: []crbWant{plusWant}},
		{name: "year-end", group: "1", bars: yearEndBars, opts: yearEndOpts, want: []crbWant{yearEndWant}},
		{name: "month-end", group: "1", bars: monthEndBars, opts: monthEndOpts, want: []crbWant{monthEndWant}},
		{name: "empty-range-rejected", group: "4,12", bars: []marketdata.Bar{crbBar(crbUTC(2026, time.March, 9, 12, 0), 100, 100.1, 99.9, 100), enterLong, closeAt(105)},
			outcome: ClockDayRejected, reason: ClockReasonEmptyRange},
		{name: "range-end-bar-excluded", group: "4", bars: crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, crbBar(crbBaseRangeEnd, 100.5, 105, 100.2, 104), closeAt(104)),
			want: []crbWant{baseMeta(closeRule(crbWant{side: "long", entry: 101, sl: 99.99, checkIndices: true, entryIndex: 36, exitIdx: 37}, 104))}},
		{name: "leap-day", group: "1", bars: leapBars, opts: leapOpts, want: []crbWant{leapWant}},
		{name: "one-minute", group: "3,10", timeframe: "1m", opts: crbOptions{route: "slices(XAUUSD 1m)"},
			bars: append(oneMinute(crbCorpusBarsAt(crbBaseRangeStart, 99, 101, nil, enterLong)), closeAt(105)),
			want: []crbWant{func() crbWant {
				w := baseMeta(closeRule(crbWant{side: "long", entry: 101, sl: 99.99, checkIndices: true, entryIndex: 180, exitIdx: 185}, 105))
				w.rangeBars = 180
				return w
			}()}},
		{name: "multiple-days", group: "5,12", bars: multi, want: []crbWant{
			baseMeta(closeRule(crbWant{side: "long", entry: 101, sl: 99.99}, 102)),
			func() crbWant {
				w := baseMeta(closeRule(crbWant{side: "short", entry: 99, sl: 99.99}, 98))
				w.dayKey, w.orderPlacedAt = "2026-03-11", crbBaseRangeEnd+day
				return w
			}()}},
	}
}

func (c crbCorpusCase) fixtureName() string { return crbCorpusPrefix + c.name }

func (c crbCorpusCase) symbolOrDefault() string {
	if c.symbol != "" {
		return c.symbol
	}
	return "XAUUSD"
}

func (c crbCorpusCase) timeframeOrDefault() string {
	if c.timeframe != "" {
		return c.timeframe
	}
	return "5m"
}

func (c crbCorpusCase) program() string {
	opts := c.opts
	opts.name = c.fixtureName()
	opts.desc = fmt.Sprintf("synthetic clock range breakout conformance case; acceptance group(s) %s", c.group)
	return opts.program()
}

type crbFixtureFile struct {
	Schema          string         `json:"schema"`
	Case            string         `json:"case"`
	StrategyID      string         `json:"strategyId"`
	Symbol          string         `json:"symbol"`
	Timeframe       string         `json:"timeframe"`
	HigherTimeframe any            `json:"higherTimeframe"`
	RangeMethod     string         `json:"rangeMethod"`
	Source          map[string]any `json:"source"`
	Costs           map[string]any `json:"costs"`
	ContextOptions  map[string]any `json:"contextOptions"`
	HTFBars         [][]float64    `json:"htfBars"`
	Bars            [][]float64    `json:"bars"`
}

func (c crbCorpusCase) fixtureJSON() []byte {
	costs := c.costs.normalized()
	rows := make([][]float64, len(c.bars))
	for i, bar := range c.bars {
		rows[i] = []float64{bar.T, bar.O, bar.H, bar.L, bar.C, bar.V}
	}
	file := crbFixtureFile{
		Schema: runFixtureSchema, Case: c.fixtureName(), StrategyID: c.fixtureName(), Symbol: c.symbolOrDefault(), Timeframe: c.timeframeOrDefault(),
		RangeMethod: "zone", Source: map[string]any{"bars": map[string]any{"kind": "synthetic-clock-range-breakout"}, "htfBars": nil},
		Costs:          map[string]any{"feePerUnit": costs.FeePerUnit, "fillOn": costs.FillOn, "slippage": costs.Slippage, "startEquity": costs.StartEquity},
		ContextOptions: map[string]any{}, HTFBars: [][]float64{}, Bars: rows,
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(file); err != nil {
		panic(err)
	}
	return out.Bytes()
}

func crbCorpusRunDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(testsupport.StratConformanceRoot(), "run")
}

// The committed fixtures and programs are exactly the builders' output, so a
// reviewer can regenerate and diff them.
func TestClockRangeBreakoutCorpusInputsAreReproducible(t *testing.T) {
	dir := crbCorpusRunDir(t)
	write := os.Getenv("CRB_WRITE_CORPUS") != ""
	for _, c := range crbCorpusCases() {
		for suffix, want := range map[string][]byte{".fixture.json": c.fixtureJSON(), ".strat": []byte(c.program())} {
			path := filepath.Join(dir, c.fixtureName()+suffix)
			if write {
				if err := os.WriteFile(path, want, 0o644); err != nil {
					t.Fatal(err)
				}
				continue
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, want) {
				t.Errorf("%s is missing or differs from the scenario builder (rewrite with CRB_WRITE_CORPUS=1): %v", path, err)
			}
		}
	}
}

// Each committed fixture, run through the fixture path, gives the trades this
// table derives by hand. The goldens are generated from the same runs, so a
// golden cannot drift from these expectations unnoticed.
func TestClockRangeBreakoutCorpusMatchesHandDerivedExpectations(t *testing.T) {
	dir := crbCorpusRunDir(t)
	for _, c := range crbCorpusCases() {
		t.Run(c.name, func(t *testing.T) {
			fixture, err := LoadRunFixture(filepath.Join(dir, c.fixtureName()+".fixture.json"))
			if err != nil {
				t.Skipf("fixture not written yet: %v", err)
			}
			source, err := os.ReadFile(filepath.Join(dir, c.fixtureName()+".strat"))
			if err != nil {
				t.Fatal(err)
			}
			result, err := RunFixtureCase(fixture, string(source))
			if err != nil {
				t.Fatal(err)
			}
			if result.TradeCount != len(c.want) {
				t.Fatalf("trade count = %d, want %d: %+v", result.TradeCount, len(c.want), result.Trades)
			}
			for i, want := range c.want {
				crbCheckTrade(t, fmt.Sprintf("%s[%d]", c.name, i), result.Trades[i], want)
			}
			if len(c.want) == 0 && c.outcome != "" {
				_, prepared, err := crbRun(t, string(source), c.symbolOrDefault(), c.timeframeOrDefault(), fixture.Bars, fixture.Costs)
				if err != nil {
					t.Fatal(err)
				}
				if day := crbDay(t, prepared, 0); day.Outcome != c.outcome || day.Reason != c.reason {
					t.Errorf("first day = %+v, want %s/%s", day, c.outcome, c.reason)
				}
			}
		})
	}
}
