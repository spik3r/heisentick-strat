package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestChannelBreakHoldOwnsRequiredChannelContext(t *testing.T) {
	const caseName = "family-channel-break-hold"
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), caseName+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
	if err != nil {
		t.Fatalf("read DSL source: %v", err)
	}
	baselineParsed, err := dsl.Parse(string(source))
	if err != nil {
		t.Fatalf("parse baseline DSL: %v", err)
	}
	if len(baselineParsed.Errors) > 0 {
		t.Fatalf("baseline DSL diagnostics: %v", baselineParsed.Errors)
	}
	baseline, err := RunFixtureCase(fixture, string(source))
	if err != nil {
		t.Fatalf("run baseline fixture: %v", err)
	}
	if baseline.TradeCount != 1 || len(baseline.Trades) != 1 {
		t.Fatalf("baseline trades = %d/%d, want reviewed fixture count 1", baseline.TradeCount, len(baseline.Trades))
	}
	if trade := baseline.Trades[0]; trade.EntryIndex != 961 || trade.TP != 2030.41513012681 {
		t.Fatalf("baseline reviewed trade entry/tp = %d/%g, want 961/2030.41513012681", trade.EntryIndex, trade.TP)
	}

	mutated := strings.Replace(string(source), "  channel active within 12 candles\n", "", 1)
	if mutated == string(source) {
		t.Fatal("fixture source did not contain the redundant channel-active directive")
	}
	parsed, err := dsl.Parse(mutated)
	if err != nil {
		t.Fatalf("parse mutated DSL: %v", err)
	}
	if len(parsed.Errors) > 0 {
		t.Fatalf("mutated DSL diagnostics: %v", parsed.Errors)
	}
	channel := mapValue(parsed.Config, "channel")
	if boolValue(channel, "enabled", false) {
		t.Fatal("Go parser unexpectedly inferred channel.enabled from setup type")
	}
	if got := intValue(channel, "activeWithinCandles", 0); got != 12 {
		t.Fatalf("default channel activeWithinCandles = %d, want 12", got)
	}
	params := paramsFromConfig(parsed.Config)
	if params.ChannelBreakHold.ActiveWithinCandles != 12 || params.ChannelBreakHold.ChannelMinWidthATR != 0 ||
		params.ChannelBreakHold.ChannelMaxWidthATR != 0 || len(params.ChannelBreakHold.ChannelDirections) != 0 {
		t.Fatalf("default channel-break-hold params drifted: %+v", params.ChannelBreakHold)
	}
	if !contextOptions(fixture, parsed.Config).Channel.Enabled {
		t.Fatal("channelBreakHold setup type did not request its required channel context")
	}

	request := RunRequest{
		Config:          baselineParsed.Config,
		Series:          marketdata.SeriesFromBars(fixture.Bars),
		HTFSeries:       marketdata.SeriesFromBars(fixture.HTFBars),
		StrategyID:      fixture.StrategyID,
		Symbol:          fixture.Symbol,
		Timeframe:       fixture.Timeframe,
		HigherTimeframe: fixture.HigherTimeframe,
		RangeMethod:     fixture.RangeMethod,
		Costs:           fixture.Costs,
	}
	baselineKey, err := SharedContextKey(request)
	if err != nil {
		t.Fatalf("baseline shared-context key: %v", err)
	}
	request.Config = parsed.Config
	mutatedKey, err := SharedContextKey(request)
	if err != nil {
		t.Fatalf("mutated shared-context key: %v", err)
	}
	if mutatedKey != baselineKey {
		t.Fatalf("redundant channel directive changed shared context\n baseline: %s\n mutated: %s", baselineKey, mutatedKey)
	}

	result, err := RunFixtureCase(fixture, mutated)
	if err != nil {
		t.Fatalf("run fixture without redundant channel directive: %v", err)
	}
	if got, want := canonicalJSON(result), canonicalJSON(baseline); got != want {
		t.Fatalf("intrinsic channel context changed reviewed output\n got: %s\nwant: %s", got, want)
	}
}
