package engine

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
)

type paperVWAPCase struct {
	Name     string      `json:"name"`
	Side     string      `json:"side"`
	Bars     [][]float64 `json:"bars"`
	Costs    PaperCosts  `json:"costs"`
	Expected struct {
		State, Reason                                            string
		EntryIndex, ExitIndex                                    int
		Entry, InitialStop, CurrentStop, Target, Size, Exit, Net float64
	} `json:"expected"`
}

func paperVWAPCases(t *testing.T) (string, []paperVWAPCase) {
	t.Helper()
	source, err := os.ReadFile("testdata/paper_vwap/defensive-risk50.strat")
	if err != nil {
		t.Fatal(err)
	}
	if paperSourceDigest(string(source)) != "98abb9dc45ae9b4c4cf379ebd9981f3cd9d3bd276fac4ad6186c3c8cddc1efb4" {
		t.Fatal("fixed research source changed")
	}
	raw, err := os.ReadFile("testdata/paper_vwap/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Schema string
		Cases  []paperVWAPCase
	}
	if json.Unmarshal(raw, &corpus) != nil || corpus.Schema != "paper-vwap-invented-cases-v1" || len(corpus.Cases) != 28 {
		t.Fatal("invalid invented corpus")
	}
	return string(source), corpus.Cases
}

func paperVWAPRequest(source string, c paperVWAPCase) PaperPrefixRequest {
	return PaperPrefixRequest{Schema: PaperPrefixRequestSchema, Source: source, Route: PaperRoute{"authored-vwap", "XAUUSD", "15m", "zone"}, Units: PaperUnits{"USD", "USD-per-unit", "abstract-unit", 1}, Costs: c.Costs, ActivationFromT: int64(c.Bars[32][0]), ClosedThroughT: int64(c.Bars[len(c.Bars)-1][0]) + 900000, Bars: c.Bars}
}

func TestPaperVWAPInventedHandDerivedCases(t *testing.T) {
	source, cases := paperVWAPCases(t)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			r := paperVWAPRequest(source, c)
			result, err := RunPaperPrefix(r)
			if err != nil {
				t.Fatal(err)
			}
			var entry, exit *PaperEvent
			for i := range result.Events {
				e := &result.Events[i]
				switch e.Kind {
				case "entry":
					if entry != nil {
						t.Fatal("extra entry")
					}
					entry = e
				case "exit":
					if exit != nil {
						t.Fatal("extra exit")
					}
					exit = e
				case "partial":
					t.Fatal("VEF emitted partial")
				case "mark":
					if e.Index < 32 {
						t.Fatal("warm-up mark")
					}
				}
			}
			if entry == nil || entry.Index != c.Expected.EntryIndex || entry.Effect.Side != c.Side {
				t.Fatal("entry", entry)
			}
			paperNear(t, entry.Effect.Price, c.Expected.Entry)
			paperNear(t, entry.Effect.Size, c.Expected.Size)
			paperNear(t, *entry.Effect.Stop, c.Expected.InitialStop)
			paperNear(t, *entry.Effect.Target, c.Expected.Target)
			if c.Expected.State == "open" {
				if len(result.OpenPositions) != 1 || result.CompletedTrades.Count != 0 || exit != nil {
					t.Fatal("open endpoint liquidated", result)
				}
				p := result.OpenPositions[0]
				paperNear(t, p.SL, c.Expected.CurrentStop)
				paperNear(t, p.InitialSL, c.Expected.InitialStop)
				if p.PositionID != entry.PositionID || result.CompletedTrades.ProfitFactorStatus != "no-trades" {
					t.Fatal("position/statistics identity")
				}
			} else {
				if len(result.OpenPositions) != 0 || result.CompletedTrades.Count != 1 || exit == nil || exit.Index != c.Expected.ExitIndex || exit.Effect.Reason != c.Expected.Reason {
					t.Fatal("closed endpoint", result)
				}
				paperNear(t, exit.Effect.Price, c.Expected.Exit)
				paperNear(t, result.Account.RealizedNet, c.Expected.Net)
				paperNear(t, result.CompletedTrades.NetPnL, c.Expected.Net)
				paperNear(t, *exit.Effect.CompletedNetPnL, *exit.Effect.ExitCredit-entry.Effect.Fee)
				paperNear(t, *exit.Effect.Stop, c.Expected.CurrentStop)
			}
			paperNear(t, result.Account.Cash, 10000+result.Account.RealizedNet)
			paperNear(t, result.Account.Equity, result.Account.Cash+result.Account.Unrealized)
			paperNear(t, result.Account.EntryFees, entry.Effect.Fee)
			if exit != nil {
				paperNear(t, result.Account.ExitFees, exit.Effect.Fee)
			}
			for _, e := range result.Events {
				if e.Account != nil {
					paperNear(t, e.Account.Drawdown, e.Account.PeakEquity-e.Account.Equity)
				}
			}
		})
	}
}

func TestPaperVWAPReplayAllShortCaseSplits(t *testing.T) {
	source, cases := paperVWAPCases(t)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			r := paperVWAPRequest(source, c)
			full, err := RunPaperPrefix(r)
			if err != nil {
				t.Fatal(err)
			}
			for split := 1; split <= len(r.Bars); split++ {
				prior, err := RunPaperPrefix(paperPrefix(r, split))
				if err != nil {
					t.Fatal(split, err)
				}
				next := r
				next.Checkpoint = prior.Checkpoint
				got, err := RunPaperPrefix(next)
				if err != nil {
					t.Fatal(split, err)
				}
				if !reflect.DeepEqual(got.Events, full.Events[prior.EventRange.To:]) || !reflect.DeepEqual(got.Account, full.Account) || !reflect.DeepEqual(got.OpenPositions, full.OpenPositions) || !reflect.DeepEqual(got.CompletedTrades, full.CompletedTrades) || !bytes.Equal(got.Checkpoint, full.Checkpoint) {
					t.Fatal("replay mismatch", split)
				}
				repeated, err := RunPaperPrefix(next)
				if err != nil || !reflect.DeepEqual(repeated, got) {
					t.Fatal("retry mismatch", split, err)
				}
			}
		})
	}
}

func TestPaperVWAPAdmissionAndErasedAuthoredIntent(t *testing.T) {
	source, cases := paperVWAPCases(t)
	base := paperVWAPRequest(source, cases[0])
	for name, extra := range map[string]string{
		"dropped approach":      "filters { approach 0 ATR side both }",
		"dropped breakeven":     "management { breakeven after 1R trail off }",
		"trailing partial":      "management { breakeven off } partial 0.5 at 1R",
		"dropped weekday":       "market conditions { weekday monday sessions london }",
		"deprecated hold alias": "management { maxHoldBars 24 }",
		"empty braces":          "{}", "extra brace": "management { trail off } }",
		"partial": "management { partial 0.5 at 1R }", "zero partial": "management { partial 0 at 1R }",
		"trail": "management { trail 1 ATR }", "trail overwritten": "management { trail 1 ATR trail off }",
		"take default": "target { take profit at opposite range edge }", "fixed stop": "risk { stop 1 ATR }",
		"limit": "execution { enter limit swept edge }", "trigger": "filters { trigger any }",
		"grade": "filters { context 0 }", "prior": "market conditions { prior day range 0 ATR }",
		"neutral ignored": "filters { approach 0 ATR }", "replaced type": "setup { type: vwap extension fade }",
		"source": "filters { source timeframe 15m }", "htf": "filters { higher timeframe must agree }",
		"bad numeric": "execution { risk 0 USD }", "fraction count": "management { wait 1.5 candles after trade }",
		"metadata garbage": "strategy \"name\" take profit at opposite range edge",
		"trailing garbage": "target { target vwap unexpected }", "ignored fallback": "target { fallback 1R }", "zero distance": "setup { distance from vwap at least 0 ATR }",
	} {
		t.Run(name, func(t *testing.T) {
			r := base
			r.Source += "\n" + extra
			if _, err := RunPaperPrefix(r); err == nil {
				t.Fatal("unsupported authored clause admitted")
			}
		})
	}
	for name, mutate := range map[string]func(*PaperPrefixRequest){
		"nextOpen": func(r *PaperPrefixRequest) { r.Costs.FillOn = "nextOpen" }, "timeframe": func(r *PaperPrefixRequest) { r.Route.Timeframe = "5m" }, "pivot": func(r *PaperPrefixRequest) { r.Route.RangeMethod = "pivot" }, "currency": func(r *PaperPrefixRequest) { r.Units.QuoteCurrency = "EUR" }, "multiplier": func(r *PaperPrefixRequest) { r.Units.Multiplier = 2 }, "off route": func(r *PaperPrefixRequest) { r.Route.Symbol = "SYN" },
	} {
		t.Run(name, func(t *testing.T) {
			r := base
			mutate(&r)
			if _, err := RunPaperPrefix(r); err == nil {
				t.Fatal("unsupported request admitted")
			}
		})
	}
	for name, mutate := range map[string]func(*PaperPrefixRequest){
		"metadata": func(r *PaperPrefixRequest) {
			r.Source = strings.Replace(r.Source, "Close VWAP Extreme Magnet Defensive", "Unregistered authored source", 1)
		},
		"whitespace": func(r *PaperPrefixRequest) {
			r.Source = strings.ReplaceAll(r.Source, "  ", "\t")
			r.Source += "\n# harmless comment\n"
		},
		"symbol": func(r *PaperPrefixRequest) {
			r.Source = strings.ReplaceAll(r.Source, "XAUUSD", "SYN")
			r.Route.Symbol = "SYN"
		},
		"side long":     func(r *PaperPrefixRequest) { r.Source += "\nfilters { side long only }\n" },
		"neutral trail": func(r *PaperPrefixRequest) { r.Source += "\nmanagement { trail off }\n" },
	} {
		t.Run(name, func(t *testing.T) {
			r := base
			mutate(&r)
			got, err := RunPaperPrefix(r)
			if err != nil || len(got.OpenPositions) != 1 {
				t.Fatal("technical source rejected", err)
			}
		})
	}
	neutral, _ := dsl.Parse("dsl v7\nsetup { type: vwap extension fade }\n")
	for name, mutate := range map[string]func(dsl.Config){
		"unknown sibling":      func(c dsl.Config) { mapValue(c, "vwapExtensionFade")["unreviewedNestedOption"] = 0 },
		"nested below scalar":  func(c dsl.Config) { mapValue(c, "vwapExtensionFade")["minDistanceAtr"] = map[string]any{"unknown": 0} },
		"nested below list":    func(c dsl.Config) { c["symbols"] = []any{map[string]any{"unknown": 0}} },
		"unknown slice member": func(c dsl.Config) { c["slices"] = []any{map[string]any{"symbol": "XAUUSD", "tf": "15m", "unknown": 0}} },
	} {
		parsed, _ := dsl.Parse(source)
		mutate(parsed.Config)
		if paperVWAPConfigSupported(map[string]any(parsed.Config), map[string]any(neutral.Config), "") {
			t.Fatal(name, "unknown nested leaf admitted")
		}
	}
}

func TestPaperVWAPWarmupAndComputedContext(t *testing.T) {
	source, cases := paperVWAPCases(t)
	for _, c := range []paperVWAPCase{cases[0], cases[13]} {
		r := paperVWAPRequest(source, c)
		p, _, err := preparePaperPrefix(r)
		if err != nil {
			t.Fatal(err)
		}
		paperNear(t, p.cols.ATR[32], 15.0/28)
		paperNear(t, p.cols.VWAP[32], 100)
		paperNear(t, math.Abs(p.cols.VWAPDistanceATR[32]), 7)
		paperNear(t, p.cols.ER[32], 3.0/19)
		if p.cols.Regime[32] != regimeChoppy || p.cols.SessionPhase[32] != 4 {
			t.Fatal("unexpected causal context")
		}
		// Identical fresh setup after a would-be historical entry uses the same seen key.
		row := append([]float64(nil), r.Bars[32]...)
		row[0] += 900000
		r.Bars = append(append([][]float64(nil), r.Bars...), row)
		r.ClosedThroughT += 900000
		r.ActivationFromT = int64(row[0])
		warm, err := RunPaperPrefix(paperPrefix(r, 33))
		if err != nil || len(warm.Events) != 0 || warm.Account.Equity != 10000 {
			t.Fatal("historical effects", err)
		}
		r.Checkpoint = warm.Checkpoint
		fresh, err := RunPaperPrefix(r)
		if err != nil || len(fresh.OpenPositions) != 1 || fresh.OpenPositions[0].EntryIndex != 33 {
			t.Fatal("warm-up consumed opportunity", err, fresh)
		}
		r.Checkpoint = nil
		r.ActivationFromT--
		if _, err := RunPaperPrefix(r); err == nil {
			t.Fatal("misaligned activation admitted")
		}
	}
}

func TestPaperVWAPReceiptBindsEveryFamilyStateField(t *testing.T) {
	source, cases := paperVWAPCases(t)
	r := paperVWAPRequest(source, cases[0])
	p, binding, err := preparePaperPrefix(r)
	if err != nil {
		t.Fatal(err)
	}
	result, original, err := executePaperPrefix(r, p, binding)
	if err != nil {
		t.Fatal(err)
	}
	c := p.broker.paperAccounting
	for name, mutate := range map[string]func(*broker){"last": func(b *broker) { b.vefLast++ }, "has last": func(b *broker) { b.hasVEFLast = !b.hasVEFLast }, "seen day": func(b *broker) { b.seen.vef.day++ }, "seen keys": func(b *broker) { b.seen.vef.keys = append(append([]string(nil), b.seen.vef.keys...), "other") }} {
		copy := p.broker
		mutate(&copy)
		got, err := newPaperCheckpoint(result, &copy, c)
		if err != nil || got.StateDigest == original.StateDigest {
			t.Fatal(name, "omitted state", err)
		}
	}
	snapshot := paperVWAPStateFromBroker(&p.broker)
	snapshot.SeenKeys[0] = "changed"
	if p.broker.seen.vef.keys[0] == "changed" {
		t.Fatal("aliased seen keys")
	}
	// The added optional VEF field must not alter old ORB/legacy digest bytes.
	legacySource, err := os.ReadFile(runFixtureDir() + "/../parse/setup-legacy-setup9.strat")
	if err != nil {
		t.Fatal(err)
	}
	legacy := paperORBRequest(t, "close")
	legacy.Source = string(legacySource)
	legacy.Route.StrategyID = "paper-legacy"
	legacy.Route.Timeframe = "1h"
	legacy.Bars = traceBars(ramp(14, 100, 1))
	legacy.ActivationFromT = int64(legacy.Bars[12][0])
	legacy.ClosedThroughT = int64(legacy.Bars[13][0]) + 3600000
	for _, old := range []PaperPrefixRequest{paperORBRequest(t, "close"), paperORBRequest(t, "nextOpen"), legacy} {
		p, binding, err := preparePaperPrefix(old)
		if err != nil {
			t.Fatal(err)
		}
		res, cp, err := executePaperPrefix(old, p, binding)
		if err != nil {
			t.Fatal(err)
		}
		c := p.broker.paperAccounting
		want, err := paperDigest(struct {
			Broker             prefixCheckpointState
			Account            PaperAccount
			Statistics         PaperTradeStatistics
			Open               *paperOpenAccount
			LastIndex, Ordinal int
		}{checkpointStateFromBroker(&p.broker), c.account, c.statistics, c.open, c.lastIndex, c.ordinal})
		if err != nil || cp.StateDigest != want || len(res.Checkpoint) == 0 {
			t.Fatal("old receipt changed")
		}
	}
}

func TestPaperVWAPControlledPredicatesAndAttemptState(t *testing.T) {
	source, cases := paperVWAPCases(t)
	r := paperVWAPRequest(source, cases[0])
	p, _, err := preparePaperPrefix(r)
	if err != nil {
		t.Fatal(err)
	}
	b := &p.broker
	b.reset(p.series, p.cols, nil, nil, nil, p.params, p.fixture, nil)
	// Controlled columns isolate predicate boundaries; public cases above compute them.
	i := 32
	b.cols.VWAPDistanceATR[i] = -3
	if _, _, ok := b.vwapExtensionFadeSetup(i, sideLong); !ok {
		t.Fatal("inclusive 3ATR")
	}
	b.cols.VWAPDistanceATR[i] = math.Nextafter(-3, 0)
	if _, _, ok := b.vwapExtensionFadeSetup(i, sideLong); ok {
		t.Fatal("below 3ATR")
	}
	b.cols.VWAPDistanceATR[i] = -3
	b.cols.Regime[i] = regimeTrending
	if _, _, ok := b.vwapExtensionFadeSetup(i, sideLong); ok {
		t.Fatal("trend accepted")
	}
	b.cols.Regime[i] = regimeChoppy
	// Family setup succeeds but the shared movement gate rejects entry. State still consumes.
	b.params.MaxMovementER = 0
	b.onVWAPExtensionFadeBar(i)
	if b.hasPosition || !b.hasVEFLast || b.vefLast != i || len(b.seen.vef.keys) != 1 {
		t.Fatal("attempt semantics changed")
	}
	before := paperVWAPStateFromBroker(b)
	b.onVWAPExtensionFadeBar(i)
	if !reflect.DeepEqual(before, paperVWAPStateFromBroker(b)) {
		t.Fatal("cooldown changed on rejected repeat")
	}
	// Seen set uses local midnight, whereas VWAP's Build uses UTC midnight.
	t0 := float64(1735739100000)
	t1 := t0 + 900000 // 13:45 then 14:00 UTC.
	var seen dailySeenSet
	seen.add(localDayKey(t0), "long:close:1000")
	if seen.seen(localDayKey(t1), "long:close:1000") || contextcols.UTCDayKey(int64(t0)) != contextcols.UTCDayKey(int64(t1)) {
		t.Fatal("local-day reset")
	}
}

func TestPaperVWAPVolumeCausalityAndSparseBars(t *testing.T) {
	source, cases := paperVWAPCases(t)
	for _, sideIndex := range []int{0, 13} {
		r := paperVWAPRequest(source, cases[sideIndex])
		r.Bars = copyPaperVWAPBars(r.Bars)
		for i := range r.Bars {
			r.Bars[i][5] = 1
		}
		r.Bars[0][5] = 1024
		got, err := RunPaperPrefix(r)
		if err != nil || len(got.OpenPositions) != 1 {
			t.Fatal("positive volumes", err)
		}
		full, _, err := preparePaperPrefix(r)
		if err != nil {
			t.Fatal(err)
		}
		// Existing same-series projection cannot infer duration from one row.
		// With positive distance, neither its unavailable context nor the later
		// first-row distance zero can signal; compare available context from two.
		for n := 2; n <= len(r.Bars); n++ {
			p, _, err := preparePaperPrefix(paperPrefix(r, n))
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < n; i++ {
				for _, pair := range [][2]float64{{p.cols.ATR[i], full.cols.ATR[i]}, {p.cols.VWAP[i], full.cols.VWAP[i]}, {p.cols.ER[i], full.cols.ER[i]}, {p.cols.VWAPDistanceATR[i], full.cols.VWAPDistanceATR[i]}} {
					if !paperSame(pair[0], pair[1]) {
						t.Fatal("future context leak", n, i, pair)
					}
				}
				if p.cols.Regime[i] != full.cols.Regime[i] || p.cols.SessionPhase[i] != full.cols.SessionPhase[i] {
					t.Fatal("future regime leak", n, i)
				}
			}
		}
		p, _, err := preparePaperPrefix(paperPrefix(r, 2))
		if err != nil {
			t.Fatal(err)
		}
		paperNear(t, p.cols.ATR[0], .5)
		for i := range r.Bars {
			r.Bars[i][5] = 0
		}
		got, err = RunPaperPrefix(r)
		if err != nil || len(got.OpenPositions) != 0 || got.CompletedTrades.ProfitFactorStatus != "no-trades" {
			t.Fatal("zero denominator signaled", err)
		}
	}
	r := paperVWAPRequest(source, cases[10])
	r.Bars = copyPaperVWAPBars(r.Bars)
	for i := 33; i < len(r.Bars); i++ {
		r.Bars[i][0] += 86400000
	}
	r.ClosedThroughT += 86400000
	got, err := RunPaperPrefix(r)
	if err != nil {
		t.Fatal(err)
	}
	var exit *PaperEvent
	for i := range got.Events {
		if got.Events[i].Kind == "exit" {
			exit = &got.Events[i]
		}
	}
	if exit == nil || exit.Index != 56 || exit.Effect.Reason != "time" {
		t.Fatal("time exit used clock duration instead of observed bars")
	}
	for name, mutate := range map[string]func(*PaperPrefixRequest){
		"duplicate": func(r *PaperPrefixRequest) { r.Bars[20][0] = r.Bars[19][0] }, "reversed": func(r *PaperPrefixRequest) { r.Bars[20][0] = r.Bars[18][0] }, "fraction gap": func(r *PaperPrefixRequest) { r.Bars[20][0]++ }, "incomplete": func(r *PaperPrefixRequest) { r.ClosedThroughT-- },
	} {
		bad := r
		bad.Bars = copyPaperVWAPBars(r.Bars)
		mutate(&bad)
		if _, err := RunPaperPrefix(bad); err == nil {
			t.Fatal(name, "accepted")
		}
	}
}

func copyPaperVWAPBars(rows [][]float64) [][]float64 {
	out := make([][]float64, len(rows))
	for i := range rows {
		out[i] = append([]float64(nil), rows[i]...)
	}
	return out
}

func TestPaperVWAPRepeatedBreakevenUsesCurrentStop(t *testing.T) {
	source, cases := paperVWAPCases(t)
	for _, offset := range []int{0, 13} {
		r := paperVWAPRequest(source, cases[5+offset])
		r.Bars = copyPaperVWAPBars(r.Bars)
		row := []float64{r.Bars[len(r.Bars)-1][0] + 900000, 96.5, 98, 96.28125, 96.3125, 0}
		if offset != 0 {
			row[1], row[2], row[3], row[4] = 200-row[1], 200-row[3], 200-row[2], 200-row[4]
		}
		r.Bars = append(r.Bars, row)
		r.ClosedThroughT += 900000
		before, err := RunPaperPrefix(paperPrefix(r, len(r.Bars)-1))
		if err != nil {
			t.Fatal(err)
		}
		r.Checkpoint = before.Checkpoint
		after, err := RunPaperPrefix(r)
		if err != nil || len(after.OpenPositions) != 1 {
			t.Fatal("repeated breakeven", err)
		}
		a, b := before.OpenPositions[0], after.OpenPositions[0]
		if a.PositionID != b.PositionID || a.InitialSL != b.InitialSL || a.SL == b.SL {
			t.Fatal("protection snapshot did not tighten")
		}
		if len(after.Events) != 1 || after.Events[0].Kind != "mark" {
			t.Fatal("invented protection effect")
		}
		if math.Abs(row[4]-a.Entry) >= math.Abs(a.Entry-a.InitialSL)*.25 {
			t.Fatal("test does not distinguish initial from current stop")
		}
	}
}

func TestPaperVWAPCooldownSeenAndTimeOrdering(t *testing.T) {
	source, cases := paperVWAPCases(t)
	r := paperVWAPRequest(source, cases[0])
	p, _, err := preparePaperPrefix(r)
	if err != nil {
		t.Fatal(err)
	}
	// Controlled index32 has a complete valid computed setup. Holding Last at
	// index21 versus20 isolates strict <12 without changing source thresholds.
	for _, distance := range []int{11, 12} {
		b := p.broker
		b.reset(p.series, p.cols, nil, nil, nil, p.params, p.fixture, nil)
		b.hasVEFLast = true
		b.vefLast = 32 - distance
		b.onVWAPExtensionFadeBar(32)
		if b.hasPosition != (distance == 12) {
			t.Fatal("cooldown boundary", distance)
		}
	}
	b := p.broker
	b.reset(p.series, p.cols, nil, nil, nil, p.params, p.fixture, nil)
	_, key, ok := b.vwapExtensionFadeSetup(32, sideLong)
	if !ok {
		t.Fatal("seed setup missing")
	}
	day := localDayKey(b.series.T[32])
	b.seen.vef.add(day, key)
	b.onVWAPExtensionFadeBar(32)
	if b.hasPosition {
		t.Fatal("same bucket reused")
	}
	b.seen.vef.day--
	b.onVWAPExtensionFadeBar(32)
	if !b.hasPosition {
		t.Fatal("new day failed to reset seen")
	}
	// Exit index does not restart family cooldown.
	b.hasPosition = false
	b.hasVEFLast = true
	b.vefLast = 20
	b.lastExitIndex = 31
	b.seen.vef.keys = nil
	b.onVWAPExtensionFadeBar(32)
	if !b.hasPosition {
		t.Fatal("exit restarted attempt cooldown")
	}
	// Same key's opposite side and other price bucket remain distinct.
	var seen dailySeenSet
	seen.add(day, "long:close:1000")
	if seen.seen(day, "short:close:1000") || seen.seen(day, "long:close:1001") {
		t.Fatal("seen key overbroad")
	}
	// On a 24-bar endpoint, existing intrabar exit precedes management/time.
	for _, kind := range []string{"sl", "tp"} {
		r := paperVWAPRequest(source, cases[10])
		r.Bars = copyPaperVWAPBars(r.Bars)
		last := r.Bars[len(r.Bars)-1]
		if kind == "sl" {
			last[3] = 95.25
		} else {
			last[2] = 100.5
		}
		got, err := RunPaperPrefix(r)
		if err != nil {
			t.Fatal(err)
		}
		var exit *PaperEvent
		for i := range got.Events {
			if got.Events[i].Kind == "exit" {
				exit = &got.Events[i]
			}
		}
		if exit == nil || exit.Effect.Reason != kind {
			t.Fatal("time beat intrabar exit", kind)
		}
	}
}

func TestPaperVWAPSeenResetPublicReplay(t *testing.T) {
	source, _ := paperVWAPCases(t)
	raw, err := os.ReadFile("testdata/paper_vwap/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		StateCase struct {
			paperVWAPCase
			EntryIndexes, Splits []int
		}
	}
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	c := corpus.StateCase
	r := paperVWAPRequest(source, c.paperVWAPCase)
	full, err := RunPaperPrefix(r)
	if err != nil {
		t.Fatal(err)
	}
	var entries []int
	for _, e := range full.Events {
		if e.Kind == "entry" {
			entries = append(entries, e.Index)
		}
	}
	if !reflect.DeepEqual(entries, c.EntryIndexes) || len(full.OpenPositions) != 1 || full.OpenPositions[0].EntryIndex != 128 || full.CompletedTrades.Count != 1 {
		t.Fatal("daily seen reuse", entries, full)
	}
	for _, split := range c.Splits {
		prior, err := RunPaperPrefix(paperPrefix(r, split))
		if err != nil {
			t.Fatal(err)
		}
		next := r
		next.Checkpoint = prior.Checkpoint
		got, err := RunPaperPrefix(next)
		if err != nil || !bytes.Equal(got.Checkpoint, full.Checkpoint) || !reflect.DeepEqual(got.Events, full.Events[prior.EventRange.To:]) {
			t.Fatal("state replay", split, err)
		}
	}
}

func TestPaperVWAPBothSidePredicateBoundaries(t *testing.T) {
	source, cases := paperVWAPCases(t)
	for _, index := range []int{0, 13} {
		r := paperVWAPRequest(source, cases[index])
		p, _, err := preparePaperPrefix(r)
		if err != nil {
			t.Fatal(err)
		}
		b := p.broker
		b.reset(p.series, p.cols, nil, nil, nil, p.params, p.fixture, nil)
		side := sideLong
		if index != 0 {
			side = sideShort
		}
		i := 32
		sign := -float64(side)
		b.cols.VWAPDistanceATR[i] = sign * 3
		if _, _, ok := b.vwapExtensionFadeSetup(i, side); !ok {
			t.Fatal("exact distance rejected")
		}
		b.cols.VWAPDistanceATR[i] = sign * math.Nextafter(3, 0)
		if _, _, ok := b.vwapExtensionFadeSetup(i, side); ok {
			t.Fatal("below distance accepted")
		}
		b.cols.VWAPDistanceATR[i] = sign * 3
		b.params.VWAPExtensionFade.TailRejectionMin = .5
		if _, _, ok := b.vwapExtensionFadeSetup(i, side); !ok {
			t.Fatal("tail equality")
		}
		b.params.VWAPExtensionFade.TailRejectionMin = math.Nextafter(.5, 1)
		if _, _, ok := b.vwapExtensionFadeSetup(i, side); ok {
			t.Fatal("tail below threshold")
		}
		b.params.VWAPExtensionFade.TailRejectionMin = .35
		b.params.VWAPExtensionFade.CloseLocationMin = .75
		if _, _, ok := b.vwapExtensionFadeSetup(i, side); !ok {
			t.Fatal("close equality")
		}
		b.params.VWAPExtensionFade.CloseLocationMin = math.Nextafter(.75, 1)
		if _, _, ok := b.vwapExtensionFadeSetup(i, side); ok {
			t.Fatal("close below threshold")
		}
		b.params.VWAPExtensionFade.CloseLocationMin = .55
		b.params.VWAPExtensionFade.MaxStopATR = 1
		if _, _, ok := b.vwapExtensionFadeSetup(i, side); ok {
			t.Fatal("oversized stop accepted")
		}
		b.params.VWAPExtensionFade.MaxStopATR = 2
		b.params.VWAPExtensionFade.MinStopATR = 2
		if _, _, ok := b.vwapExtensionFadeSetup(i, side); ok {
			t.Fatal("undersized stop accepted")
		}
		b.params.VWAPExtensionFade.MinStopATR = .4
		b.cols.SessionPhase[i] = 1
		if _, _, ok := b.vwapExtensionFadeSetup(i, side); ok {
			t.Fatal("wrong phase accepted")
		}
		b.cols.SessionPhase[i] = 4
		// VWAP behind the entry falls back to the authored fixed-R level.
		b.cols.VWAP[i] = b.series.C[i] - float64(side)
		setup, _, ok := b.vwapExtensionFadeSetup(i, side)
		if !ok {
			t.Fatal("fixed-R fallback rejected")
		}
		paperNear(t, math.Abs(setup.Target-b.series.C[i]), math.Abs(b.series.C[i]-setup.Stop))
	}
}

func TestPaperVWAPCoverageRegressionMatrix(t *testing.T) {
	source, cases := paperVWAPCases(t)
	for _, extra := range []string{
		"filters { approach 0 ATR side both }",
		"management { breakeven after 1R trail off }",
		"market conditions { weekday monday sessions london }",
		"market conditions { hour 12 sessions london }",
		"market conditions { local weekday in (monday) sessions london }",
		"market conditions { seasonality off sessions london }",
		"filters { unrecognized 1 side both }",
		"management { trail off } partial 0.5 at 1R",
		"management { breakeven off } partial 0.5 at 1R",
		"filters { side both } higher timeframe must agree",
		"risk { stop size <= 2 ATR } stop 1 ATR",
		"market conditions { symbols(XAUUSD) } source timeframe 15m",
		"strategy \"innocent\" {} take profit at opposite range edge",
		"management { trail { off } }",
		"management {\n{ trail off\n}",
		"management {\ntrail off }\n}",
		"management { trail off }}",
		"management {{ trail off }",
		"filters { } approach 0 ATR",
		"market conditions {} weekday monday",
		"management { partial 0 at 1R trail off }",
		"management { trail 1 ATR trail off }",
		"target { take profit at opposite range edge target 1R }",
	} {
		r := paperVWAPRequest(source, cases[0])
		r.Source += "\n" + extra
		if _, err := RunPaperPrefix(r); err == nil {
			t.Fatal("discarded authored content admitted", extra)
		}
	}
	for _, extra := range []string{
		"filters { side both }",
		"management { trail off }",
		"management { breakeven off }",
		"management { trail off } # partial 0.5 at 1R is only a comment",
		"market conditions { sessions london ny }",
		"strategy \"Unregistered VEF\" { description \"partial target stop words\" }",
		"filters:\n  side both\n}",
		"market conditions { symbols(XAUUSD) timeframes(15m) }",
		"management {\n  maxHoldCandles 24\n  wait 12 candles after trade\n}",
	} {
		r := paperVWAPRequest(source, cases[0])
		r.Source += "\n" + extra
		if _, err := RunPaperPrefix(r); err != nil {
			t.Fatal("supported source refused", extra, err)
		}
	}
}
