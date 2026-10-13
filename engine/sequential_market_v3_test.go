package engine_test

// V3 market route: declared closures, observed-bar continuity, reopened fills
// and open terminal positions.
//
// The accepted execution corpus is the oracle. A corpus case fixes, on a
// contiguous series, every decision, fill, exit and P&L by index and price. Its
// expected values are reused three ways without reading any engine output:
//
//  1. Closures: holes are inserted before chosen bars and declared exactly in a
//     calendar whose digest is computed here, independently of the engine. Every
//     index, price and quantity must stay as the corpus says (observed bars are
//     counted across the hole, the pending entry fills at the reopened open), the
//     opportunity ids carry the re-timed bar opens, and equity has one mark per
//     observed bar.
//  2. Truncation: a series cut while a corpus trade is in progress ends with that
//     trade open. The completed trades before it, the open trade, marked equity,
//     drawdown, paid fees and completed-trade statistics are recomputed from the
//     corpus numbers.
//  3. Metamorphic: with holes declared, results equal the contiguous run bit for
//     bit, apart from timestamps.
//
// Tolerance: relative 1e-9, no absolute floor (seqAcceptanceRelative) against
// the oracle; bit-exact for engine-versus-engine comparisons.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/report"
)

type seqV3Route struct {
	symbol, timeframe string
	stepMS            int64
}

var seqV3Routes = []seqV3Route{{"SYN", "5m", 300_000}, {"XAUUSD", "1h", 3_600_000}, {"GBPUSD", "15m", 900_000}, {"SPX500", "1d", 86_400_000}, {"EURUSD", "1m", 60_000}}

// seqV3Calendar builds a calendar document and its digest from first principles.
func seqV3Calendar(route seqV3Route, id string, closures [][2]int64) (string, engine.SequentialClosureCalendar) {
	pairs := make([]string, len(closures))
	for i, c := range closures {
		pairs[i] = fmt.Sprintf("[%d,%d]", c[0], c[1])
	}
	canonical := fmt.Sprintf(`{"schema":"sequential-closure-calendar-v1","calendarId":"%s","provider":"invented-venue","symbol":"%s","timeframe":"%s","version":"1","closures":[%s]}`, id, route.symbol, route.timeframe, strings.Join(pairs, ","))
	sum := sha256.Sum256([]byte(canonical))
	items := make([]string, len(closures))
	for i, c := range closures {
		items[i] = fmt.Sprintf(`{"startMs":%d,"endMs":%d}`, c[0], c[1])
	}
	raw := fmt.Sprintf(`{"schema":"sequential-closure-calendar-v1","calendarId":"%s","provider":"invented-venue","symbol":"%s","timeframe":"%s","version":"1","closures":[%s],"sha256":"%s"}`,
		id, route.symbol, route.timeframe, strings.Join(items, ","), hex.EncodeToString(sum[:]))
	calendar, err := engine.DecodeSequentialClosureCalendar(raw)
	if err != nil {
		panic(fmt.Sprintf("independent calendar rejected: %v\n%s", err, raw))
	}
	return raw, calendar
}

// seqV3Retime re-times contiguous bars onto a route, inserting holes[k] missing
// bars before bar k. It returns the bars and the exact closure intervals.
func seqV3Retime(base []marketdata.Bar, route seqV3Route, holes map[int]int) ([]marketdata.Bar, [][2]int64) {
	bars := make([]marketdata.Bar, len(base))
	var closures [][2]int64
	shift := 0
	for i, b := range base {
		if m := holes[i]; m > 0 && i > 0 {
			start := int64(seqMarketMonday) + int64(i-1+shift)*route.stepMS + route.stepMS
			shift += m
			closures = append(closures, [2]int64{start, start + int64(m)*route.stepMS})
		}
		b.T = seqMarketMonday + float64(int64(i+shift)*route.stepMS)
		bars[i] = b
	}
	return bars, closures
}

var seqV3IDPattern = regexp.MustCompile(`seq\.full\.public_approx\.v1:SYN:5m:(buy|sell):(\d+)`)

// seqV3Expected re-labels the corpus ids for another route and timeline: the
// id embeds the symbol, timeframe and open time of the Setup-9 bar.
func seqV3Expected(raw json.RawMessage, c seqAcceptanceCase, route seqV3Route, bars []marketdata.Bar) json.RawMessage {
	out := seqV3IDPattern.ReplaceAllStringFunc(string(raw), func(match string) string {
		parts := seqV3IDPattern.FindStringSubmatch(match)
		old, _ := strconv.ParseInt(parts[2], 10, 64)
		index := int((old - c.Series.StartOpenMS) / c.Series.TimeframeMS)
		return fmt.Sprintf("seq.full.public_approx.v1:%s:%s:%s:%d", route.symbol, route.timeframe, parts[1], int64(bars[index].T))
	})
	return json.RawMessage(out)
}

func seqV3Okay(t *testing.T, c seqAcceptanceCase, side string) (seqAcceptanceOK, bool) {
	t.Helper()
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(c.Variants[side].Expected, &probe); err != nil {
		t.Fatal(err)
	}
	var outcome string
	if err := json.Unmarshal(probe["outcome"], &outcome); err != nil {
		t.Fatal("missing outcome")
	}
	var want seqAcceptanceOK
	if outcome == "ok" {
		seqAcceptanceReadJSON(t, c.Variants[side].Expected, &want)
	}
	return want, outcome == "ok"
}

func seqV3Case(t *testing.T, c seqAcceptanceCase, side string, route seqV3Route, cut int, holes map[int]int, fee float64) (engine.SequentialMarketV3Run, []marketdata.Bar, seqAcceptanceConfig, error) {
	t.Helper()
	v := c.Variants[side]
	base := seqAcceptanceBars(t, c, v)
	if cut >= 0 {
		base = base[:cut+1]
	}
	// Holes beyond the cut do not exist.
	used := map[int]int{}
	for k, m := range holes {
		if k > 0 && k < len(base) {
			used[k] = m
		}
	}
	bars, closures := seqV3Retime(base, route, used)
	var cfg seqAcceptanceConfig
	seqAcceptanceReadJSON(t, c.Config, &cfg)
	costs, err := seqAcceptanceCosts(c.Policy, cfg)
	if err != nil {
		t.Fatal(err)
	}
	costs.FeePerUnit = fee
	fixture := seqMarketFixture(t, c.ID+"."+side, route.symbol, route.timeframe, bars, costs)
	var calendar *engine.SequentialClosureCalendar
	if len(closures) > 0 {
		_, decoded := seqV3Calendar(route, "calendar-"+c.ID, closures)
		calendar = &decoded
	}
	run, runErr := engine.RunSequentialMarketV3(fixture, seqMarketSource(route.symbol, route.timeframe, c.Policy, cfg.Risk, cfg.MaxNotional), calendar)
	if runErr == nil && len(closures) > 0 {
		if run.Calendar == nil || run.Calendar.AppliedCount != len(closures) || run.Calendar.DeclaredClosures != len(closures) {
			t.Fatalf("calendar use: %+v", run.Calendar)
		}
		for i, applied := range run.Calendar.Applied {
			if applied.StartMS != closures[i][0] || applied.EndMS != closures[i][1] || applied.MissingBars != int((closures[i][1]-closures[i][0])/route.stepMS) {
				t.Fatalf("applied closure %d: %+v", i, applied)
			}
		}
	}
	return run, bars, cfg, runErr
}

// seqV3Positions picks bars before which a closure is placed: inside the
// Setup, around the decision, between decision and entry (the reopened fill),
// during the position, before and after the exit.
func seqV3Positions(want seqAcceptanceOK, n int) []int {
	set := map[int]bool{1: true, n - 1: true}
	if len(want.Decisions) > 0 {
		d := want.Decisions[0]
		for _, k := range []int{d.SetupFirst + 3, d.DecisionIndex, d.DecisionIndex + 1} {
			set[k] = true
		}
	}
	if len(want.Trades) > 0 {
		tr := want.Trades[0]
		for _, k := range []int{tr.EntryIndex, tr.EntryIndex + 1, tr.Exit.Index, tr.Exit.Index + 1} {
			set[k] = true
		}
	}
	var out []int
	for k := 1; k < n; k++ {
		if set[k] {
			out = append(out, k)
		}
	}
	return out
}

func TestSequentialMarketV3ClosuresKeepCorpusResults(t *testing.T) {
	cases, executed := seqMarketBinding(t), 0
	for ci, c := range cases {
		for _, side := range []string{"long", "short"} {
			want, ok := seqV3Okay(t, c, side)
			if !ok {
				continue
			}
			v := c.Variants[side]
			base := seqAcceptanceBars(t, c, v)
			positions := seqV3Positions(want, len(base))
			variants := []map[int]int{}
			for i, k := range positions {
				variants = append(variants, map[int]int{k: []int{1, 3, 61}[(i+ci)%3]})
			}
			all := map[int]int{}
			for i, k := range positions {
				all[k] = []int{2, 1, 7, 61}[(i+ci)%4]
			}
			variants = append(variants, all)
			for vi, holes := range variants {
				route := seqV3Routes[(ci+vi)%len(seqV3Routes)]
				fee := []float64{0, 0.02}[(ci+vi)%2]
				name := fmt.Sprintf("%s/%s/%s_%s/holes%v/fee%g", c.ID, side, route.symbol, route.timeframe, holes, fee)
				t.Run(name, func(t *testing.T) {
					run, bars, cfg, err := seqV3Case(t, c, side, route, -1, holes, fee)
					if err != nil {
						t.Fatalf("declared closures refused: %v", err)
					}
					executed++
					if run.Open != nil {
						t.Fatalf("unexpected open position %+v", run.Open)
					}
					cc := c
					cc.Series.Symbol, cc.Series.Timeframe, cc.Series.TimeframeMS = route.symbol, route.timeframe, route.stepMS
					vv := v
					vv.Expected = seqV3Expected(v.Expected, c, route, bars)
					fixtureCosts := run.Run.Costs
					envelope := seqAcceptanceEnvelope{Schema: "dsl-conformance-trades-v1", Case: c.ID + "." + side, StrategyID: c.ID + "." + side, Symbol: route.symbol, Timeframe: route.timeframe, RangeMethod: "zone", Costs: fixtureCosts}
					// The comparator checks fee-free gross P&L; fees are checked by the oracle below.
					if fee == 0 {
						seqAcceptanceResult(t, cc, vv, cfg, bars, envelope, run.Run, nil)
					}
					metrics, perr := report.ProjectSequentialMarketMetrics(run.Run, run.Accounting)
					if perr != nil {
						t.Fatalf("metrics: %v", perr)
					}
					seqMarketCheckProjection(t, metrics.SequentialMetrics, bars, seqMarketExpectedTrades(want, 1), nil, fee*0+run.Run.Costs.FeePerUnit)
					if len(run.Accounting.Marks) != len(bars) {
						t.Fatalf("marks %d, want one per observed bar (%d)", len(run.Accounting.Marks), len(bars))
					}
					if len(run.Run.Trades) != len(want.Trades) {
						t.Fatalf("trades %d, want %d", len(run.Run.Trades), len(want.Trades))
					}
					for i, g := range run.Run.Trades {
						w := want.Trades[i]
						seqAcceptanceExact(t, "entryIndex", g.EntryIndex, w.EntryIndex)
						seqAcceptanceExact(t, "exitIndex", g.ExitIndex, w.Exit.Index)
						seqAcceptanceExact(t, "entryT", g.EntryT, bars[w.EntryIndex].T)
						seqAcceptanceNear(t, "entry", g.Entry, w.EntryFill)
						seqAcceptanceNear(t, "exit", g.Exit, w.Exit.Price)
						seqAcceptanceNear(t, "size", g.Size, w.Size)
						seqAcceptanceNear(t, "exit credit", g.PnL, w.GrossPnL-run.Run.Costs.FeePerUnit*w.Size)
					}
				})
			}
		}
	}
	if executed < 300 {
		t.Fatalf("only %d closure variants executed", executed)
	}
}

func TestSequentialMarketV3OpenTerminalPosition(t *testing.T) {
	executed, opens := 0, 0
	for ci, c := range seqMarketBinding(t) {
		for _, side := range []string{"long", "short"} {
			want, ok := seqV3Okay(t, c, side)
			if !ok || len(want.Trades) == 0 {
				continue
			}
			expected := seqMarketExpectedTrades(want, 1)
			cuts := map[int]bool{}
			for _, tr := range want.Trades {
				for _, k := range []int{tr.EntryIndex, tr.EntryIndex + 1, (tr.EntryIndex + tr.Exit.Index) / 2, tr.Exit.Index - 1} {
					if k >= tr.EntryIndex && k < tr.Exit.Index {
						cuts[k] = true
					}
				}
				cuts[want.Decisions[0].DecisionIndex] = true // a pending order on the last bar
			}
			for cut := range cuts {
				for hi, holes := range []map[int]int{nil, {cut: 5}, {1: 2, cut: 3}} {
					route := seqV3Routes[(ci+hi)%len(seqV3Routes)]
					fee := []float64{0.03, 0}[(ci+hi)%2]
					t.Run(fmt.Sprintf("%s/%s/cut%d/%v/%s", c.ID, side, cut, holes, route.symbol), func(t *testing.T) {
						run, bars, _, err := seqV3Case(t, c, side, route, cut, holes, fee)
						if err != nil {
							t.Fatalf("refused: %v", err)
						}
						executed++
						var completed []seqMarketExpected
						var open *seqMarketExpected
						for i, tr := range expected {
							switch {
							case want.Trades[i].Exit.Index <= cut:
								completed = append(completed, tr)
							case want.Trades[i].EntryIndex <= cut:
								held := tr
								open = &held
							}
						}
						if len(run.Run.Trades) != len(completed) {
							t.Fatalf("completed trades %d, want %d", len(run.Run.Trades), len(completed))
						}
						if (run.Open != nil) != (open != nil) {
							t.Fatalf("open = %+v, want open %v", run.Open, open != nil)
						}
						metrics, perr := report.ProjectSequentialMarketMetrics(run.Run, run.Accounting)
						if perr != nil {
							t.Fatalf("metrics: %v", perr)
						}
						seqMarketCheckProjection(t, metrics.SequentialMetrics, bars, completed, open, run.Run.Costs.FeePerUnit)
						if len(metrics.Equity) != len(bars) {
							t.Fatalf("equity points %d, bars %d", len(metrics.Equity), len(bars))
						}
						markedNet := 0.0
						if open != nil {
							opens++
							var wt seqAcceptanceTrade
							for i := range expected {
								if want.Trades[i].EntryIndex <= cut && want.Trades[i].Exit.Index > cut {
									wt = want.Trades[i]
								}
							}
							o := run.Open
							seqAcceptanceExact(t, "open.entryIndex", o.EntryIndex, wt.EntryIndex)
							seqAcceptanceExact(t, "open.side", o.Side, wt.Side)
							seqAcceptanceExact(t, "open.entryT", o.EntryT, bars[wt.EntryIndex].T)
							seqAcceptanceNear(t, "open.entry", o.Entry, wt.EntryFill)
							seqAcceptanceNear(t, "open.size", o.Size, wt.Size)
							seqAcceptanceNear(t, "open.stop", o.Stop, wt.Stop)
							seqAcceptanceNear(t, "open.target", o.Target, wt.Target)
							seqAcceptanceExact(t, "open.barsHeld", o.BarsHeld, cut-wt.EntryIndex+1)
							seqAcceptanceNear(t, "open.markPrice", o.MarkPrice, bars[cut].C)
							dir := 1.0
							if wt.Side == "short" {
								dir = -1
							}
							unrealized := dir * (bars[cut].C - wt.EntryFill) * wt.Size
							if unrealized == 0 {
								seqAcceptanceExact(t, "open.unrealized (zero)", o.UnrealizedPnL, 0.0)
							} else {
								seqAcceptanceNear(t, "open.unrealized", o.UnrealizedPnL, unrealized)
							}
							seqAcceptanceNear(t, "open.entryFeePaid", o.EntryFeePaid, run.Run.Costs.FeePerUnit*wt.Size)
							markedNet = unrealized - o.EntryFeePaid
						}
						for _, tr := range completed {
							markedNet += tr.grossPnL - 2*run.Run.Costs.FeePerUnit*tr.size
						}
						seqAcceptanceNear(t, "marked.net", metrics.Marked.Net, markedNet)
						seqAcceptanceNear(t, "marked.endEquity", metrics.Marked.EndEquity, seqMarketStartEquity+markedNet)
						seqAcceptanceNear(t, "marked.returnPct", metrics.Marked.ReturnPct, markedNet/seqMarketStartEquity*100)
						if open == nil {
							seqAcceptanceNear(t, "flat: marked equals settled net", metrics.Marked.Net, metrics.Headline.Net)
						} else {
							// The distinction the owner asked for: marked return differs from
							// completed-trade net profit by exactly the open position's marked P&L.
							seqAcceptanceNear(t, "marked.openUnrealizedPnl", metrics.Marked.OpenUnrealizedPnL, run.Open.UnrealizedPnL)
							seqAcceptanceNear(t, "marked.openEntryFeePaid", metrics.Marked.OpenEntryFeePaid, run.Open.EntryFeePaid)
						}
						if cut == want.Decisions[0].DecisionIndex && open == nil && len(want.Trades) > 0 && want.Trades[0].EntryIndex == cut+1 {
							if !run.PendingOrderRejected {
								t.Fatal("a decision on the last bar must be a no_next_bar outcome")
							}
						}
					})
				}
			}
		}
	}
	if executed < 300 || opens < 100 {
		t.Fatalf("executed %d variants, %d with an open position; the corpus is not being exercised", executed, opens)
	}
}

// Declared holes must not change anything but timestamps. Random walks give
// many trades, both policies and openings of every kind; the full result is
// compared bit for bit with the contiguous run.
func TestSequentialMarketV3DeclaredHolesEqualContiguousRun(t *testing.T) {
	trades := 0
	for _, policy := range []string{"E1", "E2"} {
		for seed := int64(1); seed <= 6; seed++ {
			route := seqV3Routes[int(seed)%len(seqV3Routes)]
			base := causalityWalk(seed, 700, 1.1)
			holes := map[int]int{}
			for i := 3; i < len(base); i += 17 + int(seed) {
				holes[i] = 1 + (i*7)%50
			}
			contiguous, _ := seqV3Retime(base, route, nil)
			withHoles, closures := seqV3Retime(base, route, holes)
			costs := engine.Costs{FillOn: "open", Slippage: 0.0001, FeePerUnit: 0.000002, StartEquity: 10000}
			source := seqMarketSource(route.symbol, route.timeframe, policy, 50, 100000)
			plain, err := engine.RunSequentialMarketV3(seqMarketFixture(t, "walk", route.symbol, route.timeframe, contiguous, costs), source, nil)
			if err != nil {
				t.Fatalf("contiguous: %v", err)
			}
			_, calendar := seqV3Calendar(route, "walk", closures)
			declared, err := engine.RunSequentialMarketV3(seqMarketFixture(t, "walk", route.symbol, route.timeframe, withHoles, costs), source, &calendar)
			if err != nil {
				t.Fatalf("declared: %v", err)
			}
			trades += len(plain.Run.Trades)
			if !reflect.DeepEqual(stripTimes(plain.Run.Trades), stripTimes(declared.Run.Trades)) {
				t.Fatalf("%s seed %d: trades differ across declared holes", policy, seed)
			}
			if !reflect.DeepEqual(equityOnly(plain.Accounting), equityOnly(declared.Accounting)) {
				t.Fatalf("%s seed %d: marks differ across declared holes", policy, seed)
			}
			if !reflect.DeepEqual(auditShape(plain.Run.SequentialFull), auditShape(declared.Run.SequentialFull)) {
				t.Fatalf("%s seed %d: decisions differ across declared holes", policy, seed)
			}
			if (plain.Open == nil) != (declared.Open == nil) || plain.PendingOrderRejected != declared.PendingOrderRejected {
				t.Fatalf("%s seed %d: terminal state differs", policy, seed)
			}
		}
	}
	if trades < 20 {
		t.Fatalf("only %d trades; the walks do not exercise the scheduler", trades)
	}
}

func stripTimes(in []engine.Trade) []engine.Trade {
	out := make([]engine.Trade, len(in))
	for i, tr := range in {
		tr.EntryT, tr.ExitT = 0, 0
		meta := engine.TradeMeta{}
		for k, v := range tr.Meta {
			if k != "opportunityId" && k != "episodeId" {
				meta[k] = v
			}
		}
		tr.Meta = meta
		out[i] = tr
	}
	return out
}

func equityOnly(a engine.SequentialAccounting) any {
	type mark struct{ Realized, Unrealized, Equity float64 }
	marks := make([]mark, len(a.Marks))
	for i, m := range a.Marks {
		marks[i] = mark{m.Realized, m.Unrealized, m.Equity}
	}
	return marks
}

func auditShape(a *engine.SequentialFullAudit) any {
	type row struct {
		Trigger, Side, Status, Reason string
		Setup, First, Decision, Fill  int
		Stop, ATR                     float64
	}
	var out []row
	for _, o := range a.Opportunities {
		fill := -1
		if o.FillIndex != nil {
			fill = *o.FillIndex
		}
		out = append(out, row{o.Trigger, o.Side, string(o.Status), string(o.Reason), o.SetupIndex, o.SetupFirstIndex, o.DecisionIndex, fill, o.Stop, o.SignalATR})
	}
	return out
}

func causalityWalk(seed int64, n int, level float64) []marketdata.Bar {
	bars := make([]marketdata.Bar, n)
	state := uint64(seed)*6364136223846793005 + 1442695040888963407
	next := func() float64 {
		state = state*6364136223846793005 + 1442695040888963407
		return float64(state>>11) / float64(1<<53)
	}
	price := level
	for i := range bars {
		open := price
		closePrice := open + (next()-0.5)*0.02*level
		bars[i] = marketdata.Bar{O: open, H: math.Max(open, closePrice) + next()*0.006*level, L: math.Min(open, closePrice) - next()*0.006*level, C: closePrice, V: 1}
		price = closePrice
	}
	return bars
}

// Nothing after a bar may change what was known at it, with holes before and
// after the cut. Prefixes may end with a position open; the open trade must then
// equal the full run's trade in progress at that bar.
func TestSequentialMarketV3PrefixCausalityWithClosures(t *testing.T) {
	route := seqV3Routes[1]
	costs := engine.Costs{FillOn: "open", Slippage: 0.0001, FeePerUnit: 0.000002, StartEquity: 10000}
	opened := 0
	for _, policy := range []string{"E1", "E2"} {
		for seed := int64(1); seed <= 3; seed++ {
			source := seqMarketSource(route.symbol, route.timeframe, policy, 50, 100000)
			holes := func(offset int) map[int]int {
				m := map[int]int{}
				for i := 5 + offset; i < 520; i += 23 + int(seed) {
					m[i] = 1 + (i*5)%40
				}
				return m
			}
			base := causalityWalk(seed, 520, 1.1)
			fullBars, fullClosures := seqV3Retime(base, route, holes(0))
			_, fullCal := seqV3Calendar(route, "full", fullClosures)
			full, err := engine.RunSequentialMarketV3(seqMarketFixture(t, "walk", route.symbol, route.timeframe, fullBars, costs), source, &fullCal)
			if err != nil {
				t.Fatal(err)
			}
			for cut := 30; cut < len(base); cut += 11 {
				prefixBars := fullBars[:cut+1]
				prefix, err := engine.RunSequentialMarketV3(seqMarketFixture(t, "walk", route.symbol, route.timeframe, prefixBars, costs), source, &fullCal)
				if err != nil {
					t.Fatalf("%s seed %d cut %d: %v", policy, seed, cut, err)
				}
				// Different future bars and different future holes.
				tail := causalityWalk(seed+500, len(base)-cut-1, 1.1)
				changed := append(append([]marketdata.Bar{}, base[:cut+1]...), tail...)
				shifted := holes(3)
				for k := range shifted {
					if k <= cut {
						delete(shifted, k)
					}
				}
				for k, m := range holes(0) {
					if k <= cut {
						shifted[k] = m
					}
				}
				changedBars, changedClosures := seqV3Retime(changed, route, shifted)
				_, changedCal := seqV3Calendar(route, "changed", changedClosures)
				other, err := engine.RunSequentialMarketV3(seqMarketFixture(t, "walk", route.symbol, route.timeframe, changedBars, costs), source, &changedCal)
				if err != nil {
					t.Fatalf("%s seed %d cut %d changed: %v", policy, seed, cut, err)
				}
				for _, got := range []engine.SequentialMarketV3Run{prefix, other} {
					if !reflect.DeepEqual(got.Accounting.Marks[:cut+1], full.Accounting.Marks[:cut+1]) {
						t.Fatalf("%s seed %d cut %d: marks through the cut changed", policy, seed, cut)
					}
					var closed []engine.Trade
					for _, tr := range full.Run.Trades {
						if tr.ExitIndex <= cut {
							closed = append(closed, tr)
						}
					}
					gotClosed := []engine.Trade{}
					for _, tr := range got.Run.Trades {
						if tr.ExitIndex <= cut {
							gotClosed = append(gotClosed, tr)
						}
					}
					if len(closed) == 0 {
						closed = []engine.Trade{}
					}
					if !reflect.DeepEqual(closed, gotClosed) {
						t.Fatalf("%s seed %d cut %d: completed trades changed", policy, seed, cut)
					}
					settled := func(a *engine.SequentialFullAudit) any {
						var rows []engine.SequentialFullOpportunity
						for _, o := range a.Opportunities {
							if o.DecisionIndex < cut {
								rows = append(rows, o)
							}
						}
						return rows
					}
					if !reflect.DeepEqual(settled(full.Run.SequentialFull), settled(got.Run.SequentialFull)) {
						t.Fatalf("%s seed %d cut %d: settled decisions changed", policy, seed, cut)
					}
				}
				if prefix.Open != nil {
					opened++
					var inProgress *engine.Trade
					for i := range full.Run.Trades {
						if tr := full.Run.Trades[i]; tr.EntryIndex <= cut && tr.ExitIndex > cut {
							inProgress = &full.Run.Trades[i]
						}
					}
					if inProgress == nil && (full.Open == nil || full.Open.EntryIndex > cut) {
						t.Fatalf("%s seed %d cut %d: prefix has an open trade the full run never held", policy, seed, cut)
					}
					if inProgress != nil && (inProgress.EntryIndex != prefix.Open.EntryIndex || !seqAcceptanceClose(inProgress.Entry, prefix.Open.Entry) || !seqAcceptanceClose(inProgress.Size, prefix.Open.Size)) {
						t.Fatalf("%s seed %d cut %d: open trade %+v differs from trade in progress %+v", policy, seed, cut, *prefix.Open, *inProgress)
					}
				}
			}
		}
	}
	if opened < 5 {
		t.Fatalf("only %d prefixes ended with an open position", opened)
	}
}
