package report

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// 2024-01-01T00:00:00Z; entries below step one hour apart so the session
// phase (local hour) and the year differ between trades.
const statsBaseT = 1_704_067_200_000

func statsTrade(entryOffsetHours int, side, reason string, pnl float64) engine.Trade {
	entryT := float64(statsBaseT + int64(entryOffsetHours)*3_600_000)
	return engine.Trade{
		EntryIndex: entryOffsetHours,
		EntryT:     entryT,
		ExitIndex:  entryOffsetHours + 1,
		ExitT:      entryT + 1_800_000,
		PnL:        pnl,
		Side:       side,
		Reason:     reason,
	}
}

func mustRow(t *testing.T, trades []engine.Trade) CostRow {
	t.Helper()
	row, err := summarize("realistic", 0.06, engine.RunResult{Costs: engine.Costs{StartEquity: 10000}, Trades: trades})
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	return row
}

func TestHeadlineMatchesAppMappingOfPrimaryRow(t *testing.T) {
	trades := []engine.Trade{
		statsTrade(2, "long", "tp", 150),
		statsTrade(0, "short", "sl", -100),
		statsTrade(5, "long", "sl", -50),
	}
	row := mustRow(t, trades)
	headline := headlineFromRow(row, trades)

	if headline.Trades != 3 {
		t.Fatalf("trades = %d, want 3", headline.Trades)
	}
	// The fraction is the row's percentage divided by 100, as the app's
	// mapping computes it, so it may differ from 1/3 in the last bit.
	if headline.WinRate == nil || *headline.WinRate != row.WinRate/100 || math.Abs(*headline.WinRate-1.0/3) > 1e-12 {
		t.Fatalf("winRate = %v, want fraction %v", headline.WinRate, row.WinRate/100)
	}
	if headline.ProfitFactor == nil || *headline.ProfitFactor != 1 || headline.ProfitFactorReason != "" {
		t.Fatalf("profitFactor = %v reason %q, want 1 with no reason", headline.ProfitFactor, headline.ProfitFactorReason)
	}
	if headline.Net != 0 || headline.Expectancy == nil || *headline.Expectancy != 0 {
		t.Fatalf("net = %v expectancy = %v, want 0 and 0", headline.Net, headline.Expectancy)
	}
	// Equity 10000 → 9900 → 10050 → 10000: the drop of 100 against the
	// final peak of 10050, as the cost row's dd reports it.
	if headline.MaxDrawdown != row.DD || headline.MaxDrawdown != 100.0/10050*100 {
		t.Fatalf("maxDrawdown = %v, want row dd %v", headline.MaxDrawdown, row.DD)
	}
	if headline.MaxDrawdownUnit != "percent-of-peak-equity" {
		t.Fatalf("maxDrawdownUnit = %q", headline.MaxDrawdownUnit)
	}
	if headline.FirstTradeT == nil || *headline.FirstTradeT != statsBaseT {
		t.Fatalf("firstTradeT = %v, want earliest entry %d", headline.FirstTradeT, int64(statsBaseT))
	}
	if headline.LastTradeT == nil || *headline.LastTradeT != statsBaseT+5*3_600_000+1_800_000 {
		t.Fatalf("lastTradeT = %v, want latest exit", headline.LastTradeT)
	}
}

func TestHeadlineNullsAndProfitFactorReasons(t *testing.T) {
	t.Run("no trades", func(t *testing.T) {
		headline := headlineFromRow(mustRow(t, nil), nil)
		if headline.Trades != 0 || headline.WinRate != nil || headline.Expectancy != nil {
			t.Fatalf("empty headline = %+v, want null winRate and expectancy", headline)
		}
		if headline.ProfitFactor != nil || headline.ProfitFactorReason != ProfitFactorNoTrades {
			t.Fatalf("profitFactor = %v reason %q, want nil no-trades", headline.ProfitFactor, headline.ProfitFactorReason)
		}
		if headline.FirstTradeT != nil || headline.LastTradeT != nil || headline.MaxDrawdown != 0 || headline.Net != 0 {
			t.Fatalf("empty headline = %+v, want null trade times and zero net/drawdown", headline)
		}
		encoded, err := json.Marshal(headline)
		if err != nil {
			t.Fatal(err)
		}
		want := `{"trades":0,"winRate":null,"profitFactor":null,"profitFactorReason":"no-trades","net":0,"expectancy":null,"maxDrawdown":0,"maxDrawdownUnit":"percent-of-peak-equity","firstTradeT":null,"lastTradeT":null}`
		if string(encoded) != want {
			t.Fatalf("headline JSON = %s\nwant %s", encoded, want)
		}
	})

	t.Run("no losses", func(t *testing.T) {
		trades := []engine.Trade{statsTrade(0, "long", "tp", 40), statsTrade(1, "long", "tp", 60)}
		headline := headlineFromRow(mustRow(t, trades), trades)
		if headline.ProfitFactor != nil || headline.ProfitFactorReason != ProfitFactorNoLosses {
			t.Fatalf("profitFactor = %v reason %q, want nil no-losses", headline.ProfitFactor, headline.ProfitFactorReason)
		}
		if headline.WinRate == nil || *headline.WinRate != 1 || headline.Net != 100 || *headline.Expectancy != 50 {
			t.Fatalf("headline = %+v", headline)
		}
	})
}

func TestGroupingsOrderedByKeyWithFractionWinRate(t *testing.T) {
	trades := []engine.Trade{
		statsTrade(0, "short", "sl", -100), // 2024, phase at local hour 10
		statsTrade(2, "long", "tp", 150),   // 2024, local hour 12
		statsTrade(12, "long", "sl", -50),  // 2024, local hour 22
		statsTrade(24*366, "long", "eod", 20),
	}
	groupings := groupTrades(trades)

	keysOf := func(groups []Group) []string {
		keys := make([]string, len(groups))
		for i, group := range groups {
			keys[i] = group.Key
		}
		return keys
	}
	if got := keysOf(groupings.Side); !reflect.DeepEqual(got, []string{"long", "short"}) {
		t.Fatalf("side keys = %v", got)
	}
	if got := keysOf(groupings.ExitReason); !reflect.DeepEqual(got, []string{"eod", "sl", "tp"}) {
		t.Fatalf("exit keys = %v", got)
	}
	if got := keysOf(groupings.Year); !reflect.DeepEqual(got, []string{"2024", "2025"}) {
		t.Fatalf("year keys = %v", got)
	}
	wantSessions := []string{}
	seen := map[string]bool{}
	for _, trade := range trades {
		if phase := sessionPhase(trade.EntryT); !seen[phase] {
			seen[phase] = true
			wantSessions = append(wantSessions, phase)
		}
	}
	got := keysOf(groupings.Session)
	if len(got) != len(wantSessions) || !sortedStrings(got) {
		t.Fatalf("session keys = %v, want %d distinct phases in key order", got, len(wantSessions))
	}

	long := groupings.Side[0]
	if long.Trades != 3 || long.WinRate != 2.0/3 || long.Net != 120 || long.Expectancy != 40 {
		t.Fatalf("long group = %+v", long)
	}
	if long.ProfitFactor == nil || *long.ProfitFactor != 170.0/50 {
		t.Fatalf("long profitFactor = %v", long.ProfitFactor)
	}
	tp := groupings.ExitReason[2]
	if tp.ProfitFactor != nil || tp.ProfitFactorReason != ProfitFactorNoLosses || tp.WinRate != 1 {
		t.Fatalf("tp group = %+v, want nil profit factor with no-losses", tp)
	}
	sl := groupings.ExitReason[1]
	if sl.ProfitFactor == nil || *sl.ProfitFactor != 0 || sl.WinRate != 0 || sl.Net != -150 {
		t.Fatalf("sl group = %+v, want profit factor 0", sl)
	}
}

func TestGroupingsEmptyTradesSerializeAsEmptyLists(t *testing.T) {
	encoded, err := json.Marshal(groupTrades(nil))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"session":[],"side":[],"exitReason":[],"year":[]}`; string(encoded) != want {
		t.Fatalf("groupings JSON = %s, want %s", encoded, want)
	}
}

func TestHoldoutSplitsOnEntryTimeInclusiveWithoutDoubleCounting(t *testing.T) {
	trades := []engine.Trade{
		statsTrade(0, "long", "tp", 100),
		statsTrade(1, "long", "sl", -40),
		statsTrade(2, "short", "tp", 30), // entry exactly at the boundary
		statsTrade(3, "short", "sl", -10),
	}
	boundary := int64(statsBaseT + 2*3_600_000)
	holdout, err := splitHoldout(trades, boundary)
	if err != nil {
		t.Fatalf("splitHoldout: %v", err)
	}
	if holdout.FromT != boundary {
		t.Fatalf("fromT = %d, want %d", holdout.FromT, boundary)
	}
	if holdout.InSample.Trades != 2 || holdout.Holdout.Trades != 2 {
		t.Fatalf("split = %d in-sample, %d holdout; want 2 and 2", holdout.InSample.Trades, holdout.Holdout.Trades)
	}
	if holdout.InSample.Trades+holdout.Holdout.Trades != len(trades) {
		t.Fatalf("split double counts or drops trades")
	}
	if holdout.InSample.Net != 60 || holdout.Holdout.Net != 20 {
		t.Fatalf("nets = %v / %v, want 60 / 20", holdout.InSample.Net, holdout.Holdout.Net)
	}
	if *holdout.Holdout.FirstTradeT != boundary {
		t.Fatalf("holdout firstTradeT = %d, want the boundary entry %d", *holdout.Holdout.FirstTradeT, boundary)
	}
	if *holdout.InSample.LastTradeT >= boundary+1_800_000 {
		t.Fatalf("in-sample lastTradeT = %d overlaps the holdout", *holdout.InSample.LastTradeT)
	}
	// Each side's equity curve restarts at 10000: the in-sample drop of 40
	// after a peak of 10100.
	if holdout.InSample.MaxDrawdown != 40.0/10100*100 {
		t.Fatalf("in-sample maxDrawdown = %v", holdout.InSample.MaxDrawdown)
	}

	empty, err := splitHoldout(trades, statsBaseT-1)
	if err != nil {
		t.Fatalf("splitHoldout before all trades: %v", err)
	}
	if empty.InSample.Trades != 0 || empty.InSample.ProfitFactorReason != ProfitFactorNoTrades || empty.Holdout.Trades != 4 {
		t.Fatalf("boundary before every trade = %+v", empty)
	}
}

func TestDateBoundsReportTheSuppliedSeries(t *testing.T) {
	// The caller trims the window and warm-up before Build; the bounds are
	// those of what it supplied.
	series := marketdata.NewSeries(5)
	for i := range series.T {
		series.T[i] = float64(statsBaseT + int64(i)*300_000)
	}
	warmedUp := marketdata.Series{T: series.T[2:], O: series.O[2:], H: series.H[2:], L: series.L[2:], C: series.C[2:], V: series.V[2:]}
	bounds := dateBounds(warmedUp)
	want := DateBounds{FirstT: statsBaseT + 2*300_000, LastT: statsBaseT + 4*300_000, Bars: 3}
	if bounds != want {
		t.Fatalf("dateBounds = %+v, want %+v", bounds, want)
	}
	if empty := dateBounds(marketdata.Series{}); empty != (DateBounds{}) {
		t.Fatalf("empty dateBounds = %+v", empty)
	}
}

func sortedStrings(values []string) bool {
	for i := 1; i < len(values); i++ {
		if strings.Compare(values[i-1], values[i]) > 0 {
			return false
		}
	}
	return true
}
