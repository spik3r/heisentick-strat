package report

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

func basketFloat(v float64) *float64 { return &v }
func basketInt64(v int64) *int64     { return &v }
func basketTime(s string) int64 {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t.UnixMilli()
}
func basketExampleTrade(year int, pnl float64) BasketTrade {
	return BasketTrade{EntryT: time.Date(year, 2, 3, 0, 0, 0, 0, time.UTC).UnixMilli(), Entry: 24, Size: 5, PnL: pnl, InitialSL: basketFloat(20), SL: basketFloat(23)}
}
func basketExampleRequest() BasketRequest {
	return BasketRequest{Schema: BasketRequestSchema, NowMS: basketTime("2032-06-01T00:00:00Z"), Identity: json.RawMessage(`{"run":"invented-example","build":{"digest":"opaque"}}`), Legs: []BasketLegInput{{Symbol: "TEST_A", TF: "2h", Bars: 300, Identity: json.RawMessage(`{"run":"invented-leg"}`), Provenance: json.RawMessage(`{"input":"invented"}`), DataSpan: &BasketSpan{FirstBarT: basketInt64(basketTime("2029-01-01T00:00:00Z")), LastBarT: basketInt64(basketTime("2032-01-01T00:00:00Z"))}, Trades: []BasketTrade{basketExampleTrade(2030, 40), basketExampleTrade(2031, -20)}}}}
}
func requireBasket(t *testing.T, r BasketRequest) BasketResult {
	t.Helper()
	out, err := BuildBasketReport(r)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func equalBasketFloat(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-10 {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestBasketInitialRiskAndCashCounts(t *testing.T) {
	req := basketExampleRequest()
	req.Legs[0].Trades = []BasketTrade{basketExampleTrade(2030, 40), basketExampleTrade(2030, -20), basketExampleTrade(2030, 10), basketExampleTrade(2030, 6), basketExampleTrade(2030, 4), basketExampleTrade(2030, 2)}
	req.Legs[0].Trades[2].InitialSL = nil
	req.Legs[0].Trades[2].SL = basketFloat(22) // +1R via fallback.
	req.Legs[0].Trades[3].InitialSL = nil
	req.Legs[0].Trades[3].SL = nil
	req.Legs[0].Trades[4].InitialSL = basketFloat(24) // Zero risk distance.
	req.Legs[0].Trades[5].Size = 0
	out := requireBasket(t, req)
	leg := out.Legs[0]
	if leg.Trades != 6 || leg.UnmeasurableTrades != 3 || leg.R.Trades != 3 || leg.Currency.Trades != 6 {
		t.Fatalf("bad counts %+v", leg)
	}
	equalBasketFloat(t, leg.R.SumR, 2)
	equalBasketFloat(t, leg.R.ExpectancyR, 2.0/3)
	equalBasketFloat(t, leg.Currency.Net, 42)
	equalBasketFloat(t, out.Aggregate.ExpectancyR, 2.0/6)
	equalBasketFloat(t, *leg.R.PFR, 3)
}

func TestBasketZeroPriceStopIsNotZeroRisk(t *testing.T) {
	trade := basketExampleTrade(2030, 40)
	trade.InitialSL = basketFloat(0)
	equalBasketFloat(t, *basketRealizedR(trade), 1.0/3)
}

func TestBasketNetCostsNotChargedTwice(t *testing.T) {
	req := basketExampleRequest()
	trade := basketExampleTrade(2031, 17)
	req.Legs[0].Trades = []BasketTrade{trade}
	out := requireBasket(t, req)
	equalBasketFloat(t, out.Legs[0].R.SumR, 0.85)
	equalBasketFloat(t, out.Aggregate.Currency.Net, 17)
}

func TestBasketUTCEntryYearAndIncompleteCurrentYear(t *testing.T) {
	req := basketExampleRequest()
	req.NowMS = basketTime("2032-01-01T00:00:00Z")
	before, after := basketExampleTrade(2031, -20), basketExampleTrade(2032, -40)
	before.EntryT = basketTime("2031-12-31T23:59:59Z")
	before.ExitT = basketTime("2032-01-02T12:00:00Z")
	after.EntryT = req.NowMS
	req.Legs[0].Trades = []BasketTrade{after, before}
	out := requireBasket(t, req)
	years := out.Legs[0].Years
	if len(years) != 2 || years[0].Year != 2031 || !years[0].Complete || years[1].Complete || years[1].Year != 2032 {
		t.Fatalf("bad years %+v", years)
	}
	if len(out.Aggregate.NegativeCompleteYears) != 1 || out.Aggregate.NegativeCompleteYears[0] != 2031 || out.Aggregate.YearStable {
		t.Fatalf("bad stability %+v", out.Aggregate)
	}
	if out.Legs[0].RecentNegativeYearRun != 1 {
		t.Fatal("current-year loss entered completed-year streak")
	}
}

func TestBasketRDrawdownUsesReportingOrder(t *testing.T) {
	stats := basketRStats([]float64{1, -1, -1, 3})
	equalBasketFloat(t, stats.MaxDrawdownR, 2)
	equalBasketFloat(t, stats.SumR, 2)
	equalBasketFloat(t, stats.WinRatePct, 50)
}

func TestBasketPoolRecombinesRiskNotCurrencyOrRatios(t *testing.T) {
	req := basketExampleRequest()
	second := req.Legs[0]
	second.Symbol = "TEST_B"
	second.Trades = []BasketTrade{basketExampleTrade(2030, 120), basketExampleTrade(2031, -80)}
	for i := range second.Trades {
		second.Trades[i].Size = 10
	}
	req.Legs = append(req.Legs, second)
	out := requireBasket(t, req)
	equalBasketFloat(t, out.Aggregate.GrossProfitR, 5)
	equalBasketFloat(t, out.Aggregate.GrossLossR, 3)
	equalBasketFloat(t, *out.Aggregate.PFR, 5.0/3)
	equalBasketFloat(t, *out.Aggregate.Currency.PF, 1.6)
	if len(out.Symbols) != 2 {
		t.Fatal("symbols missing")
	}
}

func TestBasketSymbolFrequencyUsesLongestRoundedOfferedHistory(t *testing.T) {
	req := basketExampleRequest()
	second := req.Legs[0]
	second.TF = "6h"
	second.DataSpan = &BasketSpan{FirstBarT: basketInt64(0), LastBarT: basketInt64(2 * int64(basketMSPerYear))}
	req.Legs = append(req.Legs, second)
	out := requireBasket(t, req)
	equalBasketFloat(t, *out.Legs[0].DataSpan.Years, 3)
	equalBasketFloat(t, *out.Symbols[0].TradesPerYear, 1.33)
	req.Legs[0].DataSpan = nil
	req.Legs = req.Legs[:1]
	out = requireBasket(t, req)
	if out.Symbols[0].TradesPerYear != nil {
		t.Fatal("missing coverage became known frequency")
	}
}

func TestBasketEmptyAndNoLossRepresentation(t *testing.T) {
	req := basketExampleRequest()
	req.Legs[0].Trades = []BasketTrade{}
	empty := []BasketTrade{}
	req.Legs[0].HarshTrades = &empty
	out := requireBasket(t, req)
	if *out.Aggregate.PFR != 0 || out.Aggregate.Harsh != nil || out.Legs[0].Harsh == nil || out.Legs[0].Years == nil {
		t.Fatalf("bad empty report %+v", out)
	}
	req.Legs[0].Trades = []BasketTrade{basketExampleTrade(2031, 0)}
	out = requireBasket(t, req)
	if *out.Aggregate.PFR != 0 {
		t.Fatal("flat factor not zero")
	}
	req.Legs[0].Trades[0].PnL = 20
	out = requireBasket(t, req)
	if out.Aggregate.PFR != nil || out.Aggregate.PFRReason != ProfitFactorNoLosses {
		t.Fatal("no-loss PF representation changed")
	}
	req.Legs = []BasketLegInput{}
	out = requireBasket(t, req)
	raw, err := json.Marshal(out)
	if err != nil || strings.Contains(string(raw), `"legs":null`) {
		t.Fatal("empty arrays not stable")
	}
}

func TestBasketPartialExitReconstruction(t *testing.T) {
	req := basketExampleRequest()
	a := basketExampleTrade(2030, 12)
	a.Size = 2
	a.Partial = true
	a.ExitIndex = 5
	b := a
	b.Partial = false
	b.Size = 3
	b.PnL = 18
	b.ExitIndex = 8
	b.SL = basketFloat(25)
	req.Legs[0].Trades = []BasketTrade{a, b}
	out := requireBasket(t, req)
	if out.Legs[0].Trades != 1 {
		t.Fatal("partial exits counted as entries")
	}
	equalBasketFloat(t, out.Legs[0].R.SumR, 1.5)
	equalBasketFloat(t, out.Legs[0].Currency.Net, 30)
	req.Legs[0].Trades[1].InitialSL = basketFloat(19)
	if _, err := BuildBasketReport(req); err == nil {
		t.Fatal("inconsistent initial risk accepted")
	}
	req.Legs[0].Trades = []BasketTrade{a}
	if _, err := BuildBasketReport(req); err == nil {
		t.Fatal("unfinished partial position accepted")
	}
}

func TestBasketNonPartialEntriesAreNotMerged(t *testing.T) {
	req := basketExampleRequest()
	a := basketExampleTrade(2030, 20)
	req.Legs[0].Trades = []BasketTrade{a, a}
	out := requireBasket(t, req)
	if out.Legs[0].Trades != 2 {
		t.Fatal("non-partial entries merged")
	}
}

func TestBasketOpenAndPrefixExcluded(t *testing.T) {
	req := basketExampleRequest()
	open := basketExampleTrade(2030, 1000)
	open.Status = "open"
	prefix := open
	prefix.Status = "prefix"
	req.Legs[0].Trades = append(req.Legs[0].Trades, open, prefix)
	out := requireBasket(t, req)
	if out.Legs[0].ExcludedTrades != 2 || out.Aggregate.Trades != 2 {
		t.Fatal("open/prefix records counted")
	}
	equalBasketFloat(t, out.Aggregate.Currency.Net, 20)
}

func TestBasketHarshCompletenessAndStreak(t *testing.T) {
	req := basketExampleRequest()
	req.Legs[0].Trades = []BasketTrade{basketExampleTrade(2028, -20), basketExampleTrade(2030, -20), basketExampleTrade(2031, -20), basketExampleTrade(2032, 60)}
	harsh := []BasketTrade{basketExampleTrade(2031, 20)}
	req.Legs[0].HarshTrades = &harsh
	second := req.Legs[0]
	second.TF = "6h"
	second.HarshTrades = nil
	req.Legs = append(req.Legs, second)
	out := requireBasket(t, req)
	if out.Aggregate.Harsh == nil || !out.Symbols[0].MissingHarsh || out.Symbols[0].Harsh != nil || out.Symbols[0].RecentNegativeYearRun != 2 {
		t.Fatalf("harsh/streak wrong %+v", out.Symbols)
	}
}

func TestBasketJSONIdentityDigestAndAllowlist(t *testing.T) {
	req := basketExampleRequest()
	req.Legs[0].HarshIdentity = json.RawMessage(`{"nested":{"cost":"alternate"}}`)
	raw, _ := json.Marshal(req)
	out, err := DecodeBasketReport(raw)
	if err != nil {
		t.Fatal(err)
	}
	if out.RequestSHA256 != fmt.Sprintf("%x", sha256.Sum256(raw)) || string(out.Identity) != string(req.Identity) || string(out.Legs[0].Provenance) != string(req.Legs[0].Provenance) || string(out.Legs[0].HarshIdentity) != string(req.Legs[0].HarshIdentity) {
		t.Fatal("request identity lost")
	}
	bad := []string{
		strings.Replace(string(raw), `"schema":`, `"Schema":`, 1),
		strings.Replace(string(raw), `"schema":`, `"extra":true,"schema":`, 1),
		strings.Replace(string(raw), `"schema":`, `"schema":"duplicate","schema":`, 1),
		strings.Replace(string(raw), `"pnl":40`, `"pnl":null`, 1),
		strings.Replace(string(raw), `"entry":24`, `"entry":"24"`, 1),
		strings.Replace(string(raw), `"size":5`, `"unknown":3,"size":5`, 1),
		strings.Replace(string(raw), `"nowMs":1969660800000`, `"nowMs":null`, 1),
		string(raw) + ` {}`,
	}
	for i, b := range bad {
		if b == string(raw) {
			t.Fatalf("mutation %d did not apply", i)
		}
		if _, err := DecodeBasketReport([]byte(b)); err == nil {
			t.Fatalf("mutation %d accepted", i)
		}
	}
}

func TestBasketBoundsAndNonfinite(t *testing.T) {
	req := basketExampleRequest()
	req.Legs[0].Trades[0].PnL = math.Inf(1)
	if _, err := BuildBasketReport(req); err == nil {
		t.Fatal("nonfinite accepted")
	}
	req = basketExampleRequest()
	req.Legs[0].Trades = []BasketTrade{basketExampleTrade(2030, math.MaxFloat64), basketExampleTrade(2031, math.MaxFloat64)}
	if _, err := BuildBasketReport(req); err == nil {
		t.Fatal("overflow accepted")
	}
	req = basketExampleRequest()
	req.Legs[0].Trades = make([]BasketTrade, MaxBasketTrades+1)
	if _, err := BuildBasketReport(req); err == nil {
		t.Fatal("trade bound ignored")
	}
	if _, err := DecodeBasketReport(make([]byte, MaxBasketJSONBytes+1)); err == nil {
		t.Fatal("byte bound ignored")
	}
	req = basketExampleRequest()
	req.Identity = json.RawMessage(`{"large":"` + strings.Repeat("a", maxBasketMetadataBytes) + `"}`)
	if _, err := BuildBasketReport(req); err == nil {
		t.Fatal("metadata bound ignored")
	}
}

func TestBasketConfiguredBarOnly(t *testing.T) {
	req := basketExampleRequest()
	req.Legs[0].Trades = []BasketTrade{basketExampleTrade(2030, 40)}
	harsh := []BasketTrade{basketExampleTrade(2030, 10)}
	req.Legs[0].HarshTrades = &harsh
	yes := true
	req.SourceIsDSL = &yes
	req.Bar = &BasketBar{Basket: []string{"TEST_A"}, MinAggregateTrades: 1, MinAggregatePFR: 2, MinPerSymbolPFR: 2, MinTradesPerYearPerSymbol: 0.1, MinAggregateHarshExpectancyR: 0.1, MinPerSymbolHarshPFR: 2, RequireAggregateYearStability: true, MaxTrailingNegativeYearsPerSymbol: 0, RequireDSLSource: true}
	out := requireBasket(t, req)
	if out.Verdict == nil || !out.Verdict.Pass {
		t.Fatalf("expected configured pass: %+v", out.Verdict)
	}
	req.Bar.MinAggregateHarshExpectancyR = 0.5
	out = requireBasket(t, req)
	if out.Verdict.Pass {
		t.Fatal("strict harsh threshold treated as inclusive")
	}
	req.Bar.Basket = []string{"TEST_B"}
	out = requireBasket(t, req)
	if out.Verdict.Components[0].Pass {
		t.Fatal("coverage bypass")
	}
	raw, _ := json.Marshal(req)
	broken := strings.Replace(string(raw), `"minAggregateTrades":1,`, "", 1)
	if _, err := DecodeBasketReport([]byte(broken)); err == nil {
		t.Fatal("missing explicit threshold accepted")
	}
	req.Bar = nil
	out = requireBasket(t, req)
	if out.Verdict != nil {
		t.Fatal("default bar invented")
	}
}

func TestBasketDecimalRoundingMatchesBinaryValue(t *testing.T) {
	for _, tc := range []struct{ input, want float64 }{{2.675, 2.67}, {0.125, 0.13}, {1.005, 1}, {3.375, 3.38}, {-0.125, -0.13}} {
		equalBasketFloat(t, *basketRound2(tc.input), tc.want)
	}
}

func TestBasketPartialFallbackRiskCannotShrink(t *testing.T) {
	req := basketExampleRequest()
	a := basketExampleTrade(2030, 12)
	a.Size = 2
	a.Partial = true
	a.ExitIndex = 1
	a.InitialSL = nil
	a.SL = basketFloat(20)
	b := a
	b.Size = 3
	b.PnL = 18
	b.Partial = false
	b.ExitIndex = 2
	b.SL = basketFloat(23)
	req.Legs[0].Trades = []BasketTrade{a, b}
	if _, err := BuildBasketReport(req); err == nil {
		t.Fatal("moving fallback stop inflated reconstructed initial R")
	}
	req.Legs[0].Trades[1].SL = basketFloat(20)
	out := requireBasket(t, req)
	equalBasketFloat(t, out.Legs[0].R.SumR, 1.5)
	req.Legs[0].Trades[1].SL = nil
	if _, err := BuildBasketReport(req); err == nil {
		t.Fatal("missing fallback stop accepted as invariant")
	}
}

func TestBasketIncompleteHarshEvidenceFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		harsh                  []BasketTrade
		unmeasurable, excluded int
	}{
		{"empty", []BasketTrade{}, 0, 0},
		{"missing denominator", []BasketTrade{{EntryT: 0, Entry: 24, Size: 5, PnL: 10}}, 1, 0},
		{"excluded prefix", []BasketTrade{{EntryT: 0, Entry: 24, Size: 5, PnL: 10, InitialSL: basketFloat(20), Status: "prefix"}}, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := basketExampleRequest()
			req.Bar = &BasketBar{Basket: []string{"TEST_A"}, MinAggregateTrades: 1}
			req.Legs[0].HarshTrades = &tc.harsh
			second := req.Legs[0]
			second.TF = "6h"
			measured := []BasketTrade{basketExampleTrade(2030, 40)}
			second.HarshTrades = &measured
			req.Legs = append(req.Legs, second)
			out := requireBasket(t, req)
			if out.Aggregate.Harsh == nil || out.Aggregate.Harsh.SumR != 2 {
				t.Fatal("descriptive measured harsh stats lost")
			}
			if !out.Legs[0].MissingHarsh || !out.Aggregate.MissingHarsh || !out.Symbols[0].MissingHarsh || out.Symbols[0].Harsh != nil {
				t.Fatal("incomplete harsh evidence represented as complete")
			}
			if out.Aggregate.HarshUnmeasurableTrades != tc.unmeasurable || out.Aggregate.HarshExcludedTrades != tc.excluded {
				t.Fatal("harsh exclusions hidden")
			}
			failed := map[string]bool{}
			for _, c := range out.Verdict.Components {
				if !c.Pass {
					failed[c.Name] = true
				}
			}
			if !failed["harsh-costs"] || !failed["per-symbol-harsh"] {
				t.Fatalf("harsh evidence bypassed verdict: %+v", out.Verdict)
			}
		})
	}
}

func TestBasketUnrelatedPartialDoesNotMergeIndependentRows(t *testing.T) {
	req := basketExampleRequest()
	independent := basketExampleTrade(2030, 20)
	partial := basketExampleTrade(2031, 12)
	partial.Size = 2
	partial.Partial = true
	terminal := partial
	terminal.Partial = false
	terminal.Size = 3
	terminal.PnL = 18
	req.Legs[0].Trades = []BasketTrade{independent, independent, partial, terminal}
	out := requireBasket(t, req)
	if out.Legs[0].Trades != 3 {
		t.Fatalf("unrelated partial merged independent rows: got %d reporting trades", out.Legs[0].Trades)
	}
	equalBasketFloat(t, out.Legs[0].R.SumR, 3.5)
	req.Legs[0].Trades = append(req.Legs[0].Trades, terminal)
	if _, err := BuildBasketReport(req); err == nil {
		t.Fatal("partial entry with multiple terminal exits accepted")
	}
}
