package engine

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const interactiveSMASource = `dsl v7
strategy "Interactive SMA" {
  description "Small marked-equity fixture."
}

market conditions {
  slices(XAUUSD 4h)
}
setup {
  type: sma golden cross
  sma fast 2
  sma slow 3
}
filters {
  side long only
}`

func TestInteractiveDualEMAUsesOrdinaryTradeAndEquityPath(t *testing.T) {
	base := filepath.Join("..", "conformance", "run", "deployed-dsl-dual-ema-resumption-xauusd-four-hour")
	raw, err := os.ReadFile(base + ".fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(base + ".strat")
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunInteractiveFixture(raw, string(source))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := DecodeRunFixture(raw)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := RunFixtureCase(fixture, string(source))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Run, legacy) || result.Run.TradeCount == 0 ||
		len(result.EquityCurve) != len(fixture.Bars) ||
		len(result.ClosedEquity) != len(fixture.Bars) || result.Skips == nil || result.SkipDiagnostics != "measured" ||
		result.SkipReasonSchema != InteractiveSkipReasonSchema ||
		result.Stats.EndEquity != result.CashEndEquity {
		t.Fatalf("dual EMA interactive contract disagrees with ordinary run: %+v", result)
	}
	var prefixFixture RunFixture
	if err := json.Unmarshal(raw, &prefixFixture); err != nil {
		t.Fatal(err)
	}
	prefixFixture.RawBars = prefixFixture.RawBars[:len(prefixFixture.RawBars)/2]
	prefixRaw, err := json.Marshal(prefixFixture)
	if err != nil {
		t.Fatal(err)
	}
	prefix, err := RunInteractiveFixture(prefixRaw, string(source))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prefix.EquityCurve, result.EquityCurve[:len(prefix.EquityCurve)]) ||
		!reflect.DeepEqual(prefix.ClosedEquity, result.ClosedEquity[:len(prefix.ClosedEquity)]) {
		t.Fatal("dual EMA prior equity marks changed after a suffix was appended")
	}
}

func TestInteractiveNamedLevelSweepPreservesFixtureTradesAndCausalMarks(t *testing.T) {
	base := filepath.Join("..", "conformance", "run", "research-dsl-daily-snd-retest-xauusd-4h")
	raw, err := os.ReadFile(base + ".fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(base + ".strat")
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunInteractiveFixture(raw, string(source))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := DecodeRunFixture(raw)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := RunFixtureCase(fixture, string(source))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Run, legacy) || result.Run.TradeCount == 0 ||
		len(result.EquityCurve) != len(fixture.Bars) ||
		len(result.ClosedEquity) != len(fixture.Bars) || result.Skips == nil || result.SkipDiagnostics != "measured" ||
		result.SkipReasonSchema != InteractiveSkipReasonSchema ||
		result.Stats.EndEquity != result.CashEndEquity {
		t.Fatalf("named level sweep interactive contract disagrees with fixture run: %+v", result)
	}
	var prefixFixture RunFixture
	if err := json.Unmarshal(raw, &prefixFixture); err != nil {
		t.Fatal(err)
	}
	prefixFixture.RawBars = prefixFixture.RawBars[:len(prefixFixture.RawBars)/2]
	prefixRaw, err := json.Marshal(prefixFixture)
	if err != nil {
		t.Fatal(err)
	}
	prefix, err := RunInteractiveFixture(prefixRaw, string(source))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prefix.EquityCurve, result.EquityCurve[:len(prefix.EquityCurve)]) ||
		!reflect.DeepEqual(prefix.ClosedEquity, result.ClosedEquity[:len(prefix.ClosedEquity)]) {
		t.Fatal("named level sweep prior equity marks changed after a suffix was appended")
	}
}

func TestInteractiveNamedLevelSweepWaitsForPriorDayAndPreservesOpenPrefix(t *testing.T) {
	base := filepath.Join("..", "conformance", "run", "family-named-level-sweep")
	raw, err := os.ReadFile(base + ".fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(base + ".strat")
	if err != nil {
		t.Fatal(err)
	}
	var fixture RunFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	run := func(count int) InteractiveRunResult {
		t.Helper()
		prefix := fixture
		prefix.RawBars = fixture.RawBars[:count]
		encoded, err := json.Marshal(prefix)
		if err != nil {
			t.Fatal(err)
		}
		result, err := RunInteractiveFixture(encoded, string(source))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	beforeDayClose, open, full := run(6), run(11), run(len(fixture.RawBars))
	if beforeDayClose.Run.TradeCount != 0 || open.Run.TradeCount != 1 ||
		open.Run.Trades[0].EntryIndex != 10 || full.Run.TradeCount != 1 ||
		full.Run.Trades[0].EntryIndex != 10 || full.Run.Trades[0].ExitIndex != 11 {
		t.Fatalf("prior-day availability/open-prefix trade timing = %d/%+v/%+v",
			beforeDayClose.Run.TradeCount, open.Run.Trades, full.Run.Trades)
	}
	for _, prefix := range []InteractiveRunResult{beforeDayClose, open} {
		if !reflect.DeepEqual(prefix.EquityCurve, full.EquityCurve[:len(prefix.EquityCurve)]) ||
			!reflect.DeepEqual(prefix.ClosedEquity, full.ClosedEquity[:len(prefix.ClosedEquity)]) {
			t.Fatal("named-level sweep prior marks changed after a suffix was appended")
		}
	}
	htfSource := strings.Replace(string(source), "  side long only", "  side long only\n  higher timeframe must be directional and agree", 1)
	if _, err := RunInteractiveFixture(raw, htfSource); !errors.Is(err, ErrInteractiveUnsupported) {
		t.Fatalf("HTF-gated named-level sweep without HTF bars = %v, want unsupported", err)
	}
}

func TestInteractiveSkipReasonsUseGoGatePriorityAndCausalPrefixes(t *testing.T) {
	base := filepath.Join("..", "conformance", "run", "research-dsl-daily-snd-retest-xauusd-4h")
	raw, err := os.ReadFile(base + ".fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(base + ".strat")
	if err != nil {
		t.Fatal(err)
	}
	var fixture RunFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	check := func(n int, want map[string]int) {
		t.Helper()
		prefix := fixture
		prefix.RawBars = fixture.RawBars[:n]
		encoded, err := json.Marshal(prefix)
		if err != nil {
			t.Fatal(err)
		}
		result, err := RunInteractiveFixture(encoded, string(source))
		if err != nil {
			t.Fatal(err)
		}
		if result.SkipDiagnostics != "measured" || result.SkipReasonSchema != InteractiveSkipReasonSchema ||
			!reflect.DeepEqual(result.Skips, want) {
			t.Fatalf("%d-bar Go gate counts = %v (%s/%s), want %v", n,
				result.Skips, result.SkipDiagnostics, result.SkipReasonSchema, want)
		}
	}
	check(1, map[string]int{skipUTCWindow: 1})
	// On the first six bars, three 00:00/04:00 UTC bars fail the window;
	// the remaining three are inside the window but have no allowed prior day.
	check(6, map[string]int{skipUTCWindow: 3, skipPriorDayTypeAllow: 3})
	check(len(fixture.RawBars), map[string]int{
		skipUTCWindow: 1605, skipPriorDayTypeAllow: 1649, skipMovementER: 13,
	})
}

func TestInteractiveSkipRecorderKeepsFirstReasonPerBar(t *testing.T) {
	b := broker{skipCounts: map[string]int{}, lastSkipIndex: -1}
	b.recordInteractiveSkip(4, skipUTCWindow)
	b.recordInteractiveSkip(4, skipPriorDayTypeAllow)
	b.recordInteractiveSkip(5, "")
	b.recordInteractiveSkip(5, skipPriorDayTypeAllow)
	if !reflect.DeepEqual(b.skipCounts, map[string]int{skipUTCWindow: 1, skipPriorDayTypeAllow: 1}) {
		t.Fatalf("first-reason counts = %v", b.skipCounts)
	}
}

func interactiveSMAFixture(t *testing.T, bars [][]float64) []byte {
	t.Helper()
	fixture := RunFixture{
		Schema: runFixtureSchema, Case: "interactive-sma", StrategyID: "sma",
		Symbol: "XAUUSD", Timeframe: "4h", RangeMethod: "zone",
		Costs:   Costs{FillOn: "close", StartEquity: 10000, FeePerUnit: 0.1},
		RawBars: bars,
	}
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestInteractiveResultUsesCausalMarksAndFeeInclusiveStats(t *testing.T) {
	bars := [][]float64{
		{0, 3, 3, 3, 3, 1}, {1, 2, 2, 2, 2, 1}, {2, 1, 1, 1, 1, 1},
		{3, 4, 4, 4, 4, 1}, {4, 5, 5, 3, 3, 1}, {5, 3, 3, 0, 0, 1},
		{6, 6, 6, 2, 2, 1},
	}
	raw := interactiveSMAFixture(t, bars)
	result, err := RunInteractiveFixture(raw, interactiveSMASource)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := DecodeRunFixture(raw)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := RunFixtureCase(fixture, interactiveSMASource)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Run, legacy) {
		t.Fatalf("interactive trades differ from conformance: %+v / %+v", result.Run, legacy)
	}
	if result.Schema != InteractiveRunSchema || result.Run.TradeCount != 1 ||
		len(result.Provenance.FixtureSHA256) != 64 || len(result.Provenance.SourceSHA256) != 64 ||
		result.Skips == nil || result.SkipDiagnostics != "measured" ||
		result.SkipReasonSchema != InteractiveSkipReasonSchema {
		t.Fatalf("interactive envelope = %+v", result)
	}
	jsonResult, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonResult), `"skips":{}`) {
		t.Fatalf("empty measured map serialized as %s", jsonResult)
	}
	for _, check := range []struct {
		name string
		got  float64
		want float64
	}{
		{"entry mark", result.EquityCurve[4], 9997.9},
		{"intratrade mark", result.EquityCurve[5], 9994.9},
		{"closed entry fee", result.ClosedEquity[5], 9999.9},
		{"rule exit mark", result.EquityCurve[6], 10000.8},
		{"cash end", result.CashEndEquity, 10000.8},
		{"fee-inclusive net", result.Stats.Net, 0.8},
		{"legacy trade net", result.Stats.TradeNet, 0.9},
		{"marked drawdown", result.Stats.MaxDD, 5.1},
		{"closed drawdown", result.Stats.MaxClosedDD, 0.1},
		{"marked percentage drawdown", result.Stats.MaxDDpct, 0.051},
	} {
		if math.IsNaN(check.got) || math.IsInf(check.got, 0) || math.Abs(check.got-check.want) > 1e-9 {
			t.Errorf("%s = %.12f, want %.12f", check.name, check.got, check.want)
		}
	}
	if result.Stats.ProfitFactor != nil || result.Stats.ProfitFactorState != "unbounded" {
		t.Fatalf("profit factor = %v/%s", result.Stats.ProfitFactor, result.Stats.ProfitFactorState)
	}
	if len(result.TradeNetPnL) != 1 || math.Abs(result.TradeNetPnL[0]-0.8) > 1e-9 {
		t.Fatalf("fee-inclusive trade outcomes = %v, want [0.8]", result.TradeNetPnL)
	}
	prefix, err := RunInteractiveFixture(interactiveSMAFixture(t, bars[:6]), interactiveSMASource)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prefix.EquityCurve, result.EquityCurve[:6]) ||
		!reflect.DeepEqual(prefix.ClosedEquity, result.ClosedEquity[:6]) ||
		prefix.CashEndEquity >= prefix.EquityCurve[5] {
		t.Fatalf("prefix changed prior marks or lost final exit cost: %+v", prefix)
	}
}

func TestInteractiveTradeOutcomesAlignWithPartialAndFinalExits(t *testing.T) {
	trades := []Trade{{PnL: 1.8, Size: 0.4, Partial: true}, {PnL: 2.7, Size: 0.6}}
	stats, outcomes := interactiveStats(trades, nil, nil,
		Costs{StartEquity: 10000, FeePerUnit: 0.1}, 10004.4)
	if len(outcomes) != 2 || math.Abs(outcomes[0]-1.76) > 1e-9 ||
		math.Abs(outcomes[1]-2.64) > 1e-9 {
		t.Fatalf("partial/final outcomes = %v, want [1.76 2.64]", outcomes)
	}
	if math.Abs(outcomes[0]+outcomes[1]-stats.Net) > 1e-9 {
		t.Fatalf("outcome sum = %.12f, cash net = %.12f", outcomes[0]+outcomes[1], stats.Net)
	}
	_, empty := interactiveStats(nil, nil, nil, Costs{StartEquity: 10000}, 10000)
	encoded, err := json.Marshal(InteractiveRunResult{TradeNetPnL: empty})
	if err != nil || !strings.Contains(string(encoded), `"tradeNetPnl":[]`) {
		t.Fatalf("zero-trade outcomes serialization = %s, %v", encoded, err)
	}
}

func TestInteractiveRejectsIncompleteRoutesAndInputs(t *testing.T) {
	raw := interactiveSMAFixture(t, [][]float64{{0, 1, 1, 1, 1, 1}})
	for _, source := range []string{
		"broken DSL",
		`dsl v7
strategy "Down shock" {}
market conditions { slices(XAUUSD 4h) }
setup { type: down shock rebound }`,
	} {
		if _, err := RunInteractiveFixture(raw, source); err == nil {
			t.Fatalf("accepted source %q", source)
		}
	}
	var fixture RunFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture.RawSourceBars = [][]float64{{0, 1, 1, 1, 1, 1}}
	withSource, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RunInteractiveFixture(withSource, interactiveSMASource); !errors.Is(err, ErrInteractiveUnsupported) {
		t.Fatalf("source route error = %v", err)
	}
	fixture.RawSourceBars = nil
	fixture.Costs.StartEquity = -1
	withBadCosts, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RunInteractiveFixture(withBadCosts, interactiveSMASource); err == nil {
		t.Fatal("negative start equity accepted")
	}
	for _, costs := range []string{
		`{"startEquity":0}`, `{"startEquity":null}`, `{"fillOn":null}`,
		`{"fillOn":""}`, `{"feePerUnit":null}`, `{"commission":1}`,
	} {
		raw := []byte(`{"schema":"dsl-conformance-run-fixture-v1","costs":` + costs +
			`,"symbol":"XAUUSD","timeframe":"4h","bars":[[0,1,1,1,1,1]]}`)
		if _, err := RunInteractiveFixture(raw, interactiveSMASource); err == nil {
			t.Errorf("accepted invalid explicit costs %s", costs)
		}
	}
	for _, invalid := range []string{
		`{"schema":"dsl-conformance-run-fixture-v1","costs":{},"symbol":"XAUUSD","timeframe":"4h","bars":[[0,1,1,1,null,1]]}`,
		`{"schema":"dsl-conformance-run-fixture-v1","costs":{},"symbol":"XAUUSD","timeframe":"4h","bars":[[0,1,1,1,1,1]],"forceRoute":true}`,
		`{"schema":"dsl-conformance-run-fixture-v1","costs":{},"symbol":"XAUUSD","timeframe":"4h","bars":[[0,1,1,1,1,1]],"contextOptions":{"pivotK":1}}`,
	} {
		if _, err := RunInteractiveFixture([]byte(invalid), interactiveSMASource); err == nil {
			t.Errorf("accepted unrepresented interactive input %s", invalid)
		}
	}
}
