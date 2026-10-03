package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Exact archived dslText, pinned by the composition's source hash. The
// strategy remains app-owned; this string is a self-contained oracle input.
const vpNYHandoffBaseSource = `dsl v6
# Session Break-Hold - 15m NY Focus Balanced
strategy "Session Break-Hold 15m NY Focus Balanced" {
  description "Join confirmed continuation after selected prior-session breaks hold during NY. Balanced copy uses a 1.15R target to improve win rate and yearly floor while keeping the same review sample."
}

market conditions {
  symbols(XAUUSD)
  timeframes(15m)
  sessions(ny)
  day type in (trending, choppy)
}

levels {
  priority(AH, LH, LL)
}

setup {
  type: session break hold
  hold 3 candles
  stop inside range 0.5 ATR
}

filters {
  higher timeframe must agree
}

target {
  target 1.15R capped at pole
}

management {
  move stop to breakeven after 0.5R plus 0.05 ATR
  wait 12 candles after trade
}

execution {
  risk: 200 USD
}`

type vpOracleTrade struct {
	EntryT      float64   `json:"entryT"`
	ExitT       float64   `json:"exitT"`
	Side        string    `json:"side"`
	Tag         string    `json:"tag"`
	Entry       float64   `json:"entry"`
	Exit        float64   `json:"exit"`
	PnL         float64   `json:"pnl"`
	VPNYHandoff TradeMeta `json:"vpNyHandoff"`
}

type vpOracleResult struct {
	Trades      []vpOracleTrade `json:"trades"`
	VPSkipCount int             `json:"vpSkipCount"`
	Net         float64         `json:"net"`
}

type vpOracle struct {
	Schema           string                    `json:"schema"`
	BaseSourceSHA256 string                    `json:"baseSourceSha256"`
	ModifiedBar      struct{ T, O, L float64 } `json:"modifiedBar"`
	Base             vpOracleResult            `json:"base"`
	Wrapped          vpOracleResult            `json:"wrapped"`
	Allowed          struct {
		ModifiedOpen float64 `json:"modifiedOpen"`
		Trade        []struct {
			EntryT      float64   `json:"entryT"`
			Side        string    `json:"side"`
			VPNYHandoff TradeMeta `json:"vpNyHandoff"`
		} `json:"trade"`
		VPSkipCount int `json:"vpSkipCount"`
	} `json:"allowed"`
}

func vpOracleFixture(t *testing.T, wrapped bool) (RunFixture, vpOracle) {
	t.Helper()
	path := filepath.Join("testdata", "vp-ny-handoff-js-oracle.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var oracle vpOracle
	if err := json.Unmarshal(data, &oracle); err != nil {
		t.Fatal(err)
	}
	if oracle.Schema != "vp-ny-handoff-js-oracle-v1" {
		t.Fatalf("oracle schema %q", oracle.Schema)
	}
	hash := sha256.Sum256([]byte(vpNYHandoffBaseSource))
	if hex.EncodeToString(hash[:]) != oracle.BaseSourceSHA256 || oracle.BaseSourceSHA256 != vpNYHandoffBaseSourceSHA256 {
		t.Fatal("archived DSL source differs from JS oracle")
	}
	fixture, err := LoadRunFixture(filepath.Join("..", "conformance", "run", "research-dsl-session-bias-seasonal-convergence-quality-fifteen.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range fixture.RawBars {
		if row[0] == oracle.ModifiedBar.T {
			row[1], row[3] = oracle.ModifiedBar.O, oracle.ModifiedBar.L
			found = true
			break
		}
	}
	if !found {
		t.Fatal("oracle-modified 12:00 bar is absent")
	}
	fixture.Case = "vp-ny-handoff-whole-strategy"
	fixture.StrategyID = "dslSessionBreakHoldNyFocusBalancedFifteen"
	if wrapped {
		fixture.StrategyID = vpNYHandoffStrategyID
	}
	return fixture, oracle
}

func runVPOracleFixture(t *testing.T, fixture RunFixture, wrapped bool) InteractiveRunResult {
	t.Helper()
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	var result InteractiveRunResult
	if wrapped {
		result, err = RunInteractiveVPNYHandoffVetoFixture(raw, vpNYHandoffBaseSource)
	} else {
		result, err = RunInteractiveFixture(raw, vpNYHandoffBaseSource)
	}
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func checkVPOracleResult(t *testing.T, result InteractiveRunResult, expected vpOracleResult) {
	t.Helper()
	if len(result.Run.Trades) != len(expected.Trades) || result.Stats.Trades != len(expected.Trades) {
		t.Fatalf("Go trades = %d, JS trades = %d", len(result.Run.Trades), len(expected.Trades))
	}
	for i, trade := range result.Run.Trades {
		want := expected.Trades[i]
		vpMetaMatches := trade.Meta["vpNyHandoff"] == nil && want.VPNYHandoff == nil
		if want.VPNYHandoff != nil {
			vpMetaMatches = reflect.DeepEqual(trade.Meta["vpNyHandoff"], want.VPNYHandoff)
		}
		if trade.EntryT != want.EntryT || trade.ExitT != want.ExitT || trade.Side != want.Side || trade.Tag != want.Tag ||
			math.Abs(trade.Entry-want.Entry) > 1e-8 || math.Abs(trade.Exit-want.Exit) > 1e-8 ||
			math.Abs(trade.PnL-want.PnL) > 1e-8 || !vpMetaMatches {
			t.Fatalf("trade %d = %+v, JS = %+v", i, trade, want)
		}
	}
	if result.Skips[skipVPNYUnresolvedRaid] != expected.VPSkipCount || math.Abs(result.Stats.Net-expected.Net) > 1e-8 {
		t.Fatalf("Go VP skips/net = %d/%g, JS = %d/%g", result.Skips[skipVPNYUnresolvedRaid], result.Stats.Net, expected.VPSkipCount, expected.Net)
	}
}

func TestInteractiveVPNYHandoffWholeStrategyJSOracle(t *testing.T) {
	baseFixture, oracle := vpOracleFixture(t, false)
	base := runVPOracleFixture(t, baseFixture, false)
	checkVPOracleResult(t, base, oracle.Base)
	if base.Schema != InteractiveRunSchema {
		t.Fatalf("base schema %q", base.Schema)
	}
	wrappedFixture, _ := vpOracleFixture(t, true)
	wrapped := runVPOracleFixture(t, wrappedFixture, true)
	checkVPOracleResult(t, wrapped, oracle.Wrapped)
	if wrapped.Schema != InteractiveVPNYHandoffSchema || wrapped.SkipReasonSchema != InteractiveVPNYSkipReasonSchema ||
		wrapped.Stats.EndEquity != wrapped.CashEndEquity || wrapped.SkipDiagnostics != "measured" {
		t.Fatalf("composition envelope: schema=%s skips=%s equity=%g cash=%g", wrapped.Schema, wrapped.SkipDiagnostics, wrapped.Stats.EndEquity, wrapped.CashEndEquity)
	}
	if base.Run.TradeCount != wrapped.Run.TradeCount+1 || base.Run.Trades[4].EntryT != 1553086800000 {
		t.Fatal("VP veto did not remove the real Session Break Hold entry")
	}
}

func TestInteractiveVPNYHandoffAllowedEntryMetadataJSOracle(t *testing.T) {
	fixture, oracle := vpOracleFixture(t, true)
	for _, row := range fixture.RawBars {
		if row[0] == oracle.ModifiedBar.T {
			row[1], row[3] = oracle.Allowed.ModifiedOpen, 1304.638
			break
		}
	}
	result := runVPOracleFixture(t, fixture, true)
	if result.Skips[skipVPNYUnresolvedRaid] != oracle.Allowed.VPSkipCount || len(oracle.Allowed.Trade) != 1 {
		t.Fatalf("allowed oracle shape or VP skips: %+v", oracle.Allowed)
	}
	want := oracle.Allowed.Trade[0]
	found := false
	for _, trade := range result.Run.Trades {
		if trade.EntryT == want.EntryT {
			found = true
			if trade.Side != want.Side || !reflect.DeepEqual(trade.Meta["vpNyHandoff"], want.VPNYHandoff) {
				t.Fatalf("allowed trade VP metadata = %v, JS = %v", trade.Meta["vpNyHandoff"], want.VPNYHandoff)
			}
		}
	}
	if !found {
		t.Fatal("allowed JS VP metadata entry missing")
	}
}

func TestInteractiveVPNYHandoffPrefixHTFAndAccounting(t *testing.T) {
	fixture, _ := vpOracleFixture(t, true)
	full := runVPOracleFixture(t, fixture, true)
	cut := -1
	for i, row := range fixture.RawBars {
		if row[0] == 1553090400000 {
			cut = i + 1
			break
		}
	}
	if cut < 0 {
		t.Fatal("prefix cut bar absent")
	}
	fixture.RawBars = fixture.RawBars[:cut]
	lastChart := fixture.RawBars[cut-1][0]
	n := 0
	for n < len(fixture.RawHTFBars) && fixture.RawHTFBars[n][0] <= lastChart {
		n++
	}
	fixture.RawHTFBars = fixture.RawHTFBars[:n]
	prefix := runVPOracleFixture(t, fixture, true)
	if !reflect.DeepEqual(prefix.EquityCurve, full.EquityCurve[:cut]) ||
		!reflect.DeepEqual(prefix.ClosedEquity, full.ClosedEquity[:cut]) ||
		prefix.Skips[skipVPNYUnresolvedRaid] != 1 {
		t.Fatal("future chart/HTF suffix changed VP decisions or prior equity marks")
	}
	if prefix.Stats.EndEquity != prefix.CashEndEquity || math.Abs(prefix.Stats.Net-(prefix.CashEndEquity-fixture.Costs.StartEquity)) > 1e-9 {
		t.Fatal("prefix cash/equity accounting disagrees")
	}
	fixture.Costs.FeePerUnit = 0.25
	withFee := runVPOracleFixture(t, fixture, true)
	if withFee.Stats.Net >= prefix.Stats.Net || withFee.Stats.Trades != prefix.Stats.Trades ||
		withFee.Stats.EndEquity != withFee.CashEndEquity || len(withFee.TradeNetPnL) != withFee.Stats.Trades {
		t.Fatal("fee-inclusive VP trade, cash, and stats contract changed")
	}
}

func TestInteractiveVPNYHandoffFailsClosed(t *testing.T) {
	fixture, _ := vpOracleFixture(t, true)
	encode := func(f RunFixture) []byte {
		t.Helper()
		raw, err := json.Marshal(f)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	if _, err := RunInteractiveFixture(encode(fixture), vpNYHandoffBaseSource); !errors.Is(err, ErrInteractiveUnsupported) {
		t.Fatalf("unwrapped VP ID = %v", err)
	}
	if _, err := RunFixtureCase(fixture, vpNYHandoffBaseSource); err == nil {
		t.Fatal("generic fixture route silently discarded the VP veto")
	}
	if _, err := PrepareRun(RunRequest{StrategyID: vpNYHandoffStrategyID}); err == nil {
		t.Fatal("generic prepared-run route silently admitted the VP wrapper")
	}
	if _, err := RunInteractiveVPNYHandoffVetoFixture(encode(fixture), vpNYHandoffBaseSource+"\n"); !errors.Is(err, ErrInteractiveUnsupported) {
		t.Fatalf("unpinned source = %v", err)
	}
	bad := fixture
	bad.RawBars = append([][]float64(nil), fixture.RawBars...)
	bad.RawBars[0] = append([]float64(nil), bad.RawBars[0]...)
	bad.RawBars[0][5] = 0
	if _, err := RunInteractiveVPNYHandoffVetoFixture(encode(bad), vpNYHandoffBaseSource); !errors.Is(err, ErrInteractiveUnsupported) {
		t.Fatalf("unusable VP volume = %v", err)
	}
	bad = fixture
	bad.RawHTFBars = nil
	if _, err := RunInteractiveVPNYHandoffVetoFixture(encode(bad), vpNYHandoffBaseSource); !errors.Is(err, ErrInteractiveUnsupported) {
		t.Fatalf("missing HTF = %v", err)
	}
	bad = fixture
	bad.Timeframe = "1h"
	if _, err := RunInteractiveVPNYHandoffVetoFixture(encode(bad), vpNYHandoffBaseSource); !errors.Is(err, ErrInteractiveUnsupported) {
		t.Fatalf("unsupported VP timeframe = %v", err)
	}
}
