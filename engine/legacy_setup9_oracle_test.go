package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// The oracle file holds the signals and trades the PINNED archived JavaScript
// produced on invented bars (see testdata/legacy_setup9/oracle/gen.mjs). Every
// float is compared exactly: the Go port keeps the JavaScript operation order.

type oracleFile struct {
	Schema   string            `json:"schema"`
	JSCommit string            `json:"jsCommit"`
	Sources  map[string]string `json:"sources"`
	Cases    []oracleCase      `json:"cases"`
}

type oracleCase struct {
	Name      string      `json:"name"`
	Note      string      `json:"note"`
	Timeframe string      `json:"timeframe"`
	Bars      [][]float64 `json:"bars"`
	Stored    []int       `json:"stored"`
	Runs      []oracleRun `json:"runs"`
}

type oracleRun struct {
	Profile string `json:"profile"`
	Costs   struct {
		Slippage    float64 `json:"slippage"`
		FeePerUnit  float64 `json:"feePerUnit"`
		SlippageBps float64 `json:"slippageBps"`
	} `json:"costs"`
	Signals []oracleSignal `json:"signals"`
	Trades  []oracleTrade  `json:"trades"`
}

type oracleSignal struct {
	Index int     `json:"index"`
	Side  string  `json:"side"`
	SL    float64 `json:"sl"`
	TP    float64 `json:"tp"`
	Tag   string  `json:"tag"`
	Next  *struct {
		Bucket           string   `json:"bucket"`
		DirectionalCount int      `json:"directionalCount"`
		BullishPercent   *float64 `json:"bullishPercent"`
	} `json:"next"`
}

type oracleTrade struct {
	Side       string  `json:"side"`
	Entry      float64 `json:"entry"`
	Exit       float64 `json:"exit"`
	SL         float64 `json:"sl"`
	TP         float64 `json:"tp"`
	Size       float64 `json:"size"`
	EntryIndex int     `json:"entryIndex"`
	ExitIndex  int     `json:"exitIndex"`
	EntryT     float64 `json:"entryT"`
	ExitT      float64 `json:"exitT"`
	Points     float64 `json:"points"`
	PnL        float64 `json:"pnl"`
	Reason     string  `json:"reason"`
	Tag        string  `json:"tag"`
}

var legacySetup9OraclePath = filepath.Join("testdata", "legacy_setup9", "oracle.json")

func loadLegacySetup9Oracle(t *testing.T) oracleFile {
	t.Helper()
	raw, err := os.ReadFile(legacySetup9OraclePath)
	if err != nil {
		t.Fatal(err)
	}
	var file oracleFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	if file.Schema != "ht227-legacy-setup9-oracle-v1" || file.JSCommit != "48a1761867342494c69c77e7ecce325e10d5d935" {
		t.Fatalf("oracle identity = %q %q", file.Schema, file.JSCommit)
	}
	return file
}

func legacySetup9Source(profile, timeframe string) string {
	return fmt.Sprintf("dsl v7\nstrategy \"legacy setup 9 test\" {\n  description \"invented traces\"\n}\nmarket conditions {\n  slices(XAUUSD %s)\n}\nsetup {\n  type: legacy setup 9\n  sequential profile %s\n}\n", timeframe, profile)
}

func legacySetup9Config(t *testing.T, profile, timeframe string) dsl.Config {
	t.Helper()
	parsed, err := dsl.Parse(legacySetup9Source(profile, timeframe))
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("parse: %v %v", err, parsed.Errors)
	}
	return parsed.Config
}

func oracleSeries(bars [][]float64) marketdata.Series {
	rows := make([]marketdata.Bar, len(bars))
	for i, b := range bars {
		rows[i] = marketdata.Bar{T: b[0], O: b[1], H: b[2], L: b[3], C: b[4], V: b[5]}
	}
	return marketdata.SeriesFromBars(rows)
}

func legacySetup9Request(t *testing.T, profile, timeframe string, series marketdata.Series, costs Costs) RunRequest {
	t.Helper()
	costs.FillOn = "close"
	return RunRequest{
		Config: legacySetup9Config(t, profile, timeframe), Series: series, Symbol: "XAUUSD", Timeframe: timeframe,
		StrategyID: "legacy-setup9-test", RangeMethod: "zone", Costs: costs,
	}
}

func sameFloat(a, b float64) bool { return math.Float64bits(a) == math.Float64bits(b) }

func jsReason(reason string) string {
	if reason == "eod" {
		return ReasonEndOfTest
	}
	return reason
}

func TestLegacySetup9OracleFileIsPinned(t *testing.T) {
	file := loadLegacySetup9Oracle(t)
	if len(file.Sources) != 8 {
		t.Fatalf("oracle records %d source hashes, want 8", len(file.Sources))
	}
	for path, digest := range file.Sources {
		if len(digest) != 64 {
			t.Fatalf("%s digest = %q", path, digest)
		}
	}
	raw, _ := os.ReadFile(legacySetup9OraclePath)
	sum := sha256.Sum256(raw)
	t.Logf("oracle.json sha256 %s (%d cases)", hex.EncodeToString(sum[:]), len(file.Cases))
}

func TestLegacySetup9CounterMatchesOracle(t *testing.T) {
	for _, c := range loadLegacySetup9Oracle(t).Cases {
		series := oracleSeries(c.Bars)
		got := legacySetup9Counter(series.C)
		for i := range got {
			if int(got[i]) != c.Stored[i] {
				t.Fatalf("%s: stored[%d] = %d, JS %d", c.Name, i, got[i], c.Stored[i])
			}
		}
	}
}

func TestLegacySetup9MatchesPinnedJavaScript(t *testing.T) {
	file := loadLegacySetup9Oracle(t)
	var trades, signals int
	for _, c := range file.Cases {
		series := oracleSeries(c.Bars)
		for _, run := range c.Runs {
			name := fmt.Sprintf("%s/%s/slip%v", c.Name, run.Profile, run.Costs.Slippage)
			costs := Costs{Slippage: run.Costs.Slippage, FeePerUnit: run.Costs.FeePerUnit}
			request := legacySetup9Request(t, run.Profile, c.Timeframe, series, costs)
			// Run applies the conformance serializer's 15-digit rounding; the
			// exact comparison uses the broker's own trades.
			if _, err := Run(request); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			rawRun, err := PrepareRun(request)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			rawTrades := append([]Trade(nil), rawRun.runRaw(costs.normalized())...)
			if len(rawTrades) != len(run.Trades) {
				t.Fatalf("%s: %d trades, JS %d", name, len(rawTrades), len(run.Trades))
			}
			for k, want := range run.Trades {
				got := rawTrades[k]
				checks := []struct {
					field     string
					got, want float64
				}{
					{"entry", got.Entry, want.Entry}, {"exit", got.Exit, want.Exit}, {"sl", got.SL, want.SL},
					{"tp", got.TP, want.TP}, {"size", got.Size, want.Size}, {"entryT", got.EntryT, want.EntryT},
					{"exitT", got.ExitT, want.ExitT}, {"points", got.Points, want.Points},
				}
				for _, check := range checks {
					if !sameFloat(check.got, check.want) {
						t.Fatalf("%s: trade %d %s = %v, JS %v", name, k, check.field, check.got, check.want)
					}
				}
				// Documented difference D1: the shared broker computes
				// points*size - fee*size, which arm64 fuses into one rounding. The
				// unfused value must equal JavaScript exactly; the engine value may
				// differ from it by at most 2 ulp (and equals it on amd64).
				unfused := float64(want.Points*want.Size) - float64(run.Costs.FeePerUnit*want.Size)
				if !sameFloat(unfused, want.PnL) {
					t.Fatalf("%s: trade %d JS pnl %v is not the unfused formula %v", name, k, want.PnL, unfused)
				}
				if gap := math.Abs(got.PnL - want.PnL); gap > 2*math.Abs(math.Nextafter(want.PnL, math.Inf(1))-want.PnL) {
					t.Fatalf("%s: trade %d pnl = %v, JS %v", name, k, got.PnL, want.PnL)
				}
				if got.Side != want.Side || got.EntryIndex != want.EntryIndex || got.ExitIndex != want.ExitIndex ||
					got.Reason != jsReason(want.Reason) || got.Tag != want.Tag {
					t.Fatalf("%s: trade %d = %+v, JS %+v", name, k, got, want)
				}
			}
			trades += len(run.Trades)

			// Rule decisions on every bar, independent of position state.
			prepared, err := PrepareRun(request)
			if err != nil {
				t.Fatal(err)
			}
			prepared.broker.reset(prepared.series, prepared.cols, prepared.htfTrend, prepared.ema, prepared.emaSlope, prepared.params, prepared.fixture, nil)
			want := map[int]oracleSignal{}
			for _, s := range run.Signals {
				want[s.Index] = s
			}
			for i := 0; i < series.Len(); i++ {
				side, ok := prepared.broker.legacySetup9Signal(i)
				js, jsOK := want[i]
				if ok != jsOK {
					t.Fatalf("%s: bar %d signal = %v, JS %v", name, i, ok, jsOK)
				}
				if !ok {
					continue
				}
				signals++
				if side.String() != js.Side {
					t.Fatalf("%s: bar %d side %s, JS %s", name, i, side, js.Side)
				}
			}
		}
	}
	if trades < 1000 || signals < 1000 {
		t.Fatalf("oracle domain too thin: %d trades, %d signal evaluations", trades, signals)
	}
	t.Logf("compared %d trades and %d signals exactly", trades, signals)
}

// The seasonal context the Go engine reads must equal the context the JS
// strategy read at every signal bar, so the gate sees identical inputs.
func TestLegacySetup9SeasonalContextMatchesOracleAtSignals(t *testing.T) {
	seen := 0
	for _, c := range loadLegacySetup9Oracle(t).Cases {
		series := oracleSeries(c.Bars)
		for _, run := range c.Runs {
			if run.Profile != dsl.LegacySetup9PerfSeasonalProfile || run.Costs.Slippage != 0 {
				continue
			}
			request := legacySetup9Request(t, run.Profile, c.Timeframe, series, Costs{})
			prepared, err := PrepareRun(request)
			if err != nil {
				t.Fatal(err)
			}
			rows := prepared.cols.Seasonality
			for _, s := range run.Signals {
				var next *struct {
					n int
					p *float64
				}
				for _, series := range rows {
					if n := series[s.Index].Next; n != nil {
						next = &struct {
							n int
							p *float64
						}{n.DirectionalCount, n.BullishPercent}
					}
				}
				if next == nil || s.Next == nil || next.n != s.Next.DirectionalCount || next.p == nil || s.Next.BullishPercent == nil || !sameFloat(*next.p, *s.Next.BullishPercent) {
					t.Fatalf("%s bar %d: Go next %+v, JS %+v", c.Name, s.Index, next, s.Next)
				}
				seen++
			}
		}
	}
	if seen < 40 {
		t.Fatalf("only %d seasonal signals compared", seen)
	}
}

func TestLegacySetup9OracleCoversBoundaryCases(t *testing.T) {
	file := loadLegacySetup9Oracle(t)
	byName := map[string]oracleCase{}
	for _, c := range file.Cases {
		byName[c.Name] = c
	}
	seasonalSignals := func(name string) []oracleSignal {
		for _, run := range byName[name].Runs {
			if run.Profile == dsl.LegacySetup9PerfSeasonalProfile && run.Costs.Slippage == 0 {
				return run.Signals
			}
		}
		t.Fatalf("missing case %s", name)
		return nil
	}
	rawSignals := func(name string) []oracleSignal {
		for _, run := range byName[name].Runs {
			if run.Profile == dsl.LegacySetup9Profile && run.Costs.Slippage == 0 {
				return run.Signals
			}
		}
		return nil
	}
	fires := map[string]string{
		"seas-long-60pct-10samples": "long", "seas-long-60pct-with-dojis": "long", "seas-long-7of11": "long",
		"seas-short-40pct-10samples": "short", "seas-window-fresh-bullish": "long",
	}
	silent := []string{
		"seas-long-9samples", "seas-long-50pct", "seas-long-40pct", "seas-long-6of11",
		"seas-short-9samples", "seas-short-50pct", "seas-short-60pct", "seas-short-5of11", "seas-window-stale-bullish",
	}
	for name, side := range fires {
		got := seasonalSignals(name)
		if len(got) != 1 || got[0].Side != side {
			t.Fatalf("%s: seasonal signals = %+v, want one %s", name, got, side)
		}
	}
	for _, name := range silent {
		if got := seasonalSignals(name); len(got) != 0 {
			t.Fatalf("%s: seasonal signals = %+v, want none", name, got)
		}
		// The raw profile still signals: only the seasonal gate removes it.
		if len(rawSignals(name)) != 1 {
			t.Fatalf("%s: raw profile signals = %+v, want one", name, rawSignals(name))
		}
	}
	// Exact threshold: 6/10 bullish = 60 for the long gate, 4/10 = 40 for the short gate, with 10 samples.
	for _, name := range []string{"seas-long-60pct-10samples", "seas-short-40pct-10samples"} {
		next := seasonalSignals(name)[0].Next
		if next == nil || next.DirectionalCount != 10 || next.BullishPercent == nil || (*next.BullishPercent != 60 && *next.BullishPercent != 40) {
			t.Fatalf("%s: next = %+v, want 10 samples at exactly 60 or 40 percent", name, next)
		}
	}
}
