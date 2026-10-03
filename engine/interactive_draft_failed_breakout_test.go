package engine

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
)

const draftFailedBreakoutSource = `dsl v7
strategy "Fixed Failed Breakout" { description "Synthetic fixed setup; mutable risk only." }
market conditions { slices(XAUUSD 5m) }
setup { type: failed breakout }
filters {
 side long only
 range method zone
}
execution { risk 200 USD }`

func failedBreakoutDraftFixture(t *testing.T, count int) []byte {
	t.Helper()
	rows := make([][]float64, count)
	for i := range rows {
		c := 100 + math.Sin(float64(i)*math.Pi/6)
		rows[i] = []float64{1767225600000 + float64(i)*300000, c - 0.1, c + 0.3, c - 0.6, c, 1}
	}
	raw, err := json.Marshal(map[string]any{"schema": "dsl-conformance-run-fixture-v1", "case": "fixed-failed-breakout",
		"strategyId": InteractiveDraftStrategyID, "symbol": "XAUUSD", "timeframe": "5m", "rangeMethod": "zone",
		"costs": map[string]any{"startEquity": 10000, "fillOn": "nextOpen", "feePerUnit": 0.1, "slippage": 0.06, "slippageBps": 0}, "bars": rows})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestInteractiveDraftFixedFailedBreakoutRiskAndPrefixes(t *testing.T) {
	raw := failedBreakoutDraftFixture(t, 1600)
	profile, err := InspectInteractiveSource([]byte(`{"schema":"dsl-interactive-source-profile-v1","symbol":"XAUUSD","timeframe":"5m"}`), draftFailedBreakoutSource)
	if err != nil || profile.Profile.Family != "failedBreakout" || profile.Profile.RangeMethod != "zone" {
		t.Fatalf("profile: %v / %#v", err, profile.Profile)
	}
	full, err := RunInteractiveDraftFixture(raw, draftFailedBreakoutSource)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Run.Trades) == 0 {
		t.Fatal("synthetic fixture needs a trade witness")
	}
	first := full.Run.Trades[0]
	if len(full.Run.Trades) != 5 || first.EntryIndex != 297 || first.ExitIndex != 303 || math.Abs(first.Entry-98.96) > 1e-9 || first.Meta["levelKey"] != "PDL" {
		t.Fatal("synthetic prior-day level / raw next-open witness changed")
	}
	if math.Abs(first.Size-200/(first.Entry-first.InitialSL)) > 1e-9 || math.Abs(full.TradeNetPnL[0]-(first.PnL-0.1*first.Size)) > 1e-9 {
		t.Fatal("sizing or entry+exit-fee accounting witness changed")
	}
	changed, err := RunInteractiveDraftFixture(raw, strings.Replace(draftFailedBreakoutSource, "risk 200 USD", "risk 100 USD", 1))
	if err != nil || len(changed.Run.Trades) != len(full.Run.Trades) {
		t.Fatalf("risk change: %v", err)
	}
	for i, trade := range full.Run.Trades {
		other := changed.Run.Trades[i]
		if trade.EntryIndex != other.EntryIndex || trade.ExitIndex != other.ExitIndex || trade.Entry != other.Entry || trade.Exit != other.Exit || math.Abs(trade.Size-2*other.Size) > 1e-9 {
			t.Fatalf("risk sizing changed signal/fill timing or did not halve size: %d sizes %.17g / %.17g", i, trade.Size, other.Size)
		}
		if math.Abs(full.TradeNetPnL[i]-2*changed.TradeNetPnL[i]) > 1e-9 {
			t.Fatal("fee-inclusive net did not scale with risk")
		}
	}
	if full.Provenance.SourceSHA256 == changed.Provenance.SourceSHA256 || full.Stats.Net == changed.Stats.Net {
		t.Fatal("risk mutation lost source identity or net effect")
	}
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	fields["strategyId"] = "unregisteredFixedFailedBreakout"
	ordinaryRaw, _ := json.Marshal(fields)
	ordinary, err := RunInteractiveFixture(ordinaryRaw, draftFailedBreakoutSource)
	if err != nil || !reflect.DeepEqual(full.Run.Trades, ordinary.Run.Trades) || !reflect.DeepEqual(full.Stats, ordinary.Stats) || !reflect.DeepEqual(full.EquityCurve, ordinary.EquityCurve) {
		t.Fatal("draft changed ordinary Go behavior")
	}
	for _, n := range []int{288, 296, 297, 298, 303, 304, 600, 900, 1200, 1599} {
		prefix, err := RunInteractiveDraftFixture(failedBreakoutDraftFixture(t, n), draftFailedBreakoutSource)
		if err != nil || !reflect.DeepEqual(prefix.EquityCurve, full.EquityCurve[:n]) || !reflect.DeepEqual(prefix.ClosedEquity, full.ClosedEquity[:n]) {
			t.Fatalf("noncausal range/level/equity prefix %d: %v", n, err)
		}
	}
}

func TestInteractiveDraftFailedBreakoutStrictRefusals(t *testing.T) {
	for name, source := range map[string]string{
		"missing risk":          strings.Replace(draftFailedBreakoutSource, "execution { risk 200 USD }", "", 1),
		"missing range":         strings.Replace(draftFailedBreakoutSource, " range method zone", "", 1),
		"short":                 strings.Replace(draftFailedBreakoutSource, "side long only", "side short only", 1),
		"range variation":       strings.Replace(draftFailedBreakoutSource, "range method zone", "range method pivot", 1),
		"risk zero":             strings.Replace(draftFailedBreakoutSource, "200 USD", "0 USD", 1),
		"risk suffix":           strings.Replace(draftFailedBreakoutSource, "200 USD", "200R USD", 1),
		"risk overflow":         strings.Replace(draftFailedBreakoutSource, "200 USD", "1e309 USD", 1),
		"risk exponent plus":    strings.Replace(draftFailedBreakoutSource, "200 USD", "1e+2 USD", 1),
		"risk exponent minus":   strings.Replace(draftFailedBreakoutSource, "200 USD", "1e-2 USD", 1),
		"risk exponent":         strings.Replace(draftFailedBreakoutSource, "200 USD", "1e2 USD", 1),
		"alias exponent":        strings.Replace(draftFailedBreakoutSource, "risk 200 USD", "riskUsd 1e+2", 1),
		"risk unit":             strings.Replace(draftFailedBreakoutSource, "200 USD", "200 EUR", 1),
		"risk extra":            strings.Replace(draftFailedBreakoutSource, "200 USD", "200 USD ignored", 1),
		"alias duplicate":       draftFailedBreakoutSource + "\nexecution { riskUsd 100 }",
		"duplicate side":        strings.Replace(draftFailedBreakoutSource, "side long only", "side long only\n side long only", 1),
		"duplicate slices":      strings.Replace(draftFailedBreakoutSource, "XAUUSD 5m", "XAUUSD 5m, XAUUSD 5m", 1),
		"inline prefix":         strings.Replace(draftFailedBreakoutSource, "risk 200 USD", "ignored risk 200 USD", 1),
		"section suffix":        draftFailedBreakoutSource + " ignored",
		"unknown empty section": draftFailedBreakoutSource + "\ntriggers {}",
		"empty section":         draftFailedBreakoutSource + "\nexecution {}",
		"unclosed":              strings.TrimSuffix(draftFailedBreakoutSource, "}"),
		"unmatched":             draftFailedBreakoutSource + "\n}",
		"type suffix":           strings.Replace(draftFailedBreakoutSource, "failed breakout", "failed breakout ignored", 1),
	} {
		t.Run(name, func(t *testing.T) {
			parsed, _ := dsl.ParseStrict(source)
			if len(parsed.Errors) == 0 {
				t.Fatal("strict source accepted unsupported intent")
			}
			if _, err := RunInteractiveDraftFixture(failedBreakoutDraftFixture(t, 1), source); err == nil {
				t.Fatal("unsupported intent executed")
			}
		})
	}
	for _, directive := range []string{"when price sweeps range.low by 0.05 within 2 then signal long", "approach at least 5 candles toward level", "candle in (pin)", "stop 2 ATR", "stop beyond last 4 candle extreme by 0.5 ATR", "target 2 R", "take profit opposite channel edge", "trail 2 ATR", "move stop to breakeven after 0.5R plus 0.02 ATR", "partial at 1R", "entry limit", "priority(PDL)", "day type in (ranging)", "higher timeframe must not be against", "source timeframe 1h", "context 2"} {
		source := strings.Replace(draftFailedBreakoutSource, " range method zone", " range method zone\n "+directive, 1)
		if _, err := InspectInteractiveSource([]byte(`{"schema":"dsl-interactive-source-profile-v1","symbol":"XAUUSD","timeframe":"5m"}`), source); err == nil {
			t.Fatalf("unqualified directive admitted: %s", directive)
		}
	}
	for _, mode := range []string{"heikinAshi"} {
		if _, err := InspectInteractiveSource([]byte(`{"schema":"dsl-interactive-source-profile-v1","symbol":"XAUUSD","timeframe":"5m","calculationSource":"`+mode+`"}`), draftFailedBreakoutSource); err == nil {
			t.Fatal("HA preflight admitted")
		}
		var fields map[string]any
		_ = json.Unmarshal(failedBreakoutDraftFixture(t, 1), &fields)
		fields["calculationSource"] = mode
		raw, _ := json.Marshal(fields)
		if _, err := RunInteractiveDraftFixture(raw, draftFailedBreakoutSource); err == nil {
			t.Fatal("HA execution admitted")
		}
	}
	for _, source := range []string{draftFailedBreakoutSource + "\n# a legitimate comment\n", strings.Replace(draftFailedBreakoutSource, "risk 200 USD", "riskUsd 200.5", 1)} {
		if parsed, _ := dsl.ParseStrict(source); len(parsed.Errors) != 0 || len(parsed.Warnings) != 0 {
			t.Fatalf("qualified metadata/risk alias rejected: %v / %v", parsed.Errors, parsed.Warnings)
		}
	}
	metadata := strings.Replace(draftFailedBreakoutSource, "Synthetic fixed setup; mutable risk only.", "my name Alpha description intact", 1)
	parsed, _ := dsl.ParseStrict(metadata)
	if len(parsed.Errors) != 0 || len(parsed.Warnings) != 0 || parsed.Config["name"] != "Fixed Failed Breakout" || parsed.Config["description"] != "my name Alpha description intact" {
		t.Fatal("strict inline metadata must retain audited quoted text")
	}
}
