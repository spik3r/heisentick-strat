package engine

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
)

func ordinaryAdmissionSource(t *testing.T) string {
	t.Helper()
	b, e := os.ReadFile("../conformance/run/sar-named-level-allow.strat")
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func TestOrdinaryBacktestPreparationNeverPromisesExecution(t *testing.T) {
	source := ordinaryAdmissionSource(t)
	for _, variant := range []string{source, strings.Replace(source, "risk 200 USD", "risk 137 USD", 1), strings.Replace(source, "sar trend bearish", "sar trend bullish", 1), strings.Replace(source, "Named Level Sweep Synthetic", "New ordinary source without hash registration", 1), strings.Replace(source, "filters { sar trend bearish }", "", 1)} {
		for _, data := range []bool{false, true} {
			raw := ordinaryTestRequest(t, "inspect", func(m map[string]any) {
				m["route"].(map[string]any)["symbol"] = "XAUUSD"
				m["route"].(map[string]any)["timeframe"] = "4h"
				if !data {
					m["data"] = nil
				} else {
					m["data"].(map[string]any)["closedThroughT"] = 18000000
				}
			})
			c, e := DecodeOrdinaryBacktestRequest("inspect", raw, variant)
			if e != nil {
				t.Fatal(e)
			}
			p, e := PrepareOrdinaryBacktestAdmission(c)
			if e != nil {
				t.Fatal(e)
			}
			if p.Classification().Kind != "ordinary" || p.Preflight() != "not-evaluated" || p.DataValidated() != data || !ordinaryHex(p.ConfigIdentity(), 64) {
				t.Fatalf("wrong preliminary facts: %#v", p)
			}
			parsed, e := dsl.Parse(variant)
			if e != nil {
				t.Fatal(e)
			}
			cfg, e := json.Marshal(parsed.Config)
			if e != nil {
				t.Fatal(e)
			}
			if p.ConfigIdentity() != ordinaryIdentity(cfg).SHA256 {
				t.Fatal("config digest was reconstructed rather than hashing actual Go config")
			}
		}
	}
	c, e := DecodeOrdinaryBacktestRequest("inspect", ordinaryTestRequest(t, "inspect", nil), "dsl v999\n")
	if e != nil {
		t.Fatal(e)
	}
	p, e := PrepareOrdinaryBacktestAdmission(c)
	if p != nil {
		t.Fatal("parse rejection invented config identity")
	}
	ordinaryTestFault(t, e, "PARSE_REJECTED")
}

func TestOrdinaryBacktestDedicatedClassificationPreservesGuards(t *testing.T) {
	cases := []struct {
		name   string
		config dsl.Config
	}{
		{"sequential", dsl.Config{"setupType": string(dsl.FamilySequentialFull)}},
		{"gold", dsl.Config{"setupType": "goldFlagReference"}},
		{"adaptive", dsl.Config{"setupType": string(dsl.FamilyAdaptiveVolumeFlag)}},
		{"master", dsl.Config{"setupType": string(dsl.FamilyMasterStructural)}},
		{"regime", dsl.Config{"setupType": "regimeEngine"}},
		{"range", dsl.Config{"setupType": string(dsl.FamilyRangeReversion)}},
		{"range typed", dsl.Config{"setupType": dsl.FamilyRangeReversion}},
		{"range alias", dsl.Config{"SETUPTYPE": "Range-Reversion"}},
		{"range hidden", dsl.Config{"setupType": "namedLevelSweep", "nested": map[string]any{"rangeReversion": nil}}},
		{"range hidden alias", dsl.Config{"setupType": "namedLevelSweep", "nested": map[string]any{"Range-Reversion": false}}},
		{"clock", dsl.Config{"setupType": "clockRangeBreakout"}},
		{"clock discriminator alias", dsl.Config{"SETUP-TYPE": "Clock_Range_Breakout"}},
		{"clock hidden", dsl.Config{"setupType": "namedLevelSweep", "hidden": map[string]any{"Clock-Range-Breakout": nil}}},
		{"timed", dsl.Config{"setupType": "timedReturn"}},
		{"timed hidden", dsl.Config{"setupType": "namedLevelSweep", "hidden": map[string]any{"TIMED_RETURN": nil}}},
		{"frozen", dsl.Config{"setupType": "frozenLevelBreakout"}},
		{"frozen hidden", dsl.Config{"setupType": "namedLevelSweep", "hidden": map[string]any{"frozenLevelBreakout": nil}}},
		{"r11", dsl.Config{"setupType": "R11"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ordinaryClassifyConfig(c.config)
			if got.Kind != "dedicated" {
				t.Fatalf("dedicated intent fell through: %#v", got)
			}
			if strings.Contains(c.name, "range") && rejectDedicatedRangeReversionExecution(c.config) == nil {
				t.Fatal("ordinary classification drifted from existing runner range guard")
			}
		})
	}
	ordinary := dsl.Config{"setupType": "namedLevelSweep", "name": "clockRangeBreakout is just a description", "description": "timedReturn words are not config"}
	if c := ordinaryClassifyConfig(ordinary); c.Kind != "ordinary" {
		t.Fatalf("unrelated strategy labels became an allowlist: %#v", c)
	}
	for _, cfg := range []dsl.Config{{"setupType": "namedLevelSweep", "sourceTimeframe": "4h"}, {"setupType": "namedLevelSweep", "htf": map[string]any{"mode": "notAgainst", "timeframe": "1d"}}} {
		if c := ordinaryClassifyConfig(cfg); c.Kind != "unsupported" {
			t.Fatal("source/HTF profile presented as ordinary", c)
		}
	}
	for i := 0; i < 50; i++ {
		cfg := dsl.Config{"setupType": "namedLevelSweep", "z": map[string]any{"timedReturn": nil}, "a": map[string]any{"clockRangeBreakout": nil}}
		if c := ordinaryClassifyConfig(cfg); c.Family != "clockRangeBreakout" {
			t.Fatal("classification depends on map iteration order", c)
		}
	}
	if c := ordinaryClassifyConfig(dsl.Config{"setupType": "new-unimplemented-family"}); c.Kind != "unsupported" {
		t.Fatal("unknown family gained admission")
	}
}

func TestOrdinaryBacktestDataPreflightRefusals(t *testing.T) {
	source := ordinaryAdmissionSource(t)
	cases := []struct {
		name, code string
		mutate     func(map[string]any)
	}{
		{"timestamp order", "DATA_INVALID", func(m map[string]any) { d := m["data"].(map[string]any); d["bars"].([]any)[1].([]any)[0] = 0 }},
		{"OHLC high", "DATA_INVALID", func(m map[string]any) { m["data"].(map[string]any)["bars"].([]any)[0].([]any)[2] = 9 }},
		{"OHLC low", "DATA_INVALID", func(m map[string]any) { m["data"].(map[string]any)["bars"].([]any)[0].([]any)[3] = 12 }},
		{"first endpoint", "DATA_INVALID", func(m map[string]any) { m["data"].(map[string]any)["firstT"] = 1 }},
		{"last endpoint", "DATA_INVALID", func(m map[string]any) { m["data"].(map[string]any)["lastT"] = 1 }},
		{"forming last", "FORMING_BAR", func(m map[string]any) { m["data"].(map[string]any)["closedThroughT"] = 7199999 }},
		{"unsupported timeframe", "ROUTE_UNSUPPORTED", func(m map[string]any) { m["route"].(map[string]any)["timeframe"] = "calendar-month" }},
	}
	for _, v := range cases {
		t.Run(v.name, func(t *testing.T) {
			raw := ordinaryTestRequest(t, "run", v.mutate)
			c, e := DecodeOrdinaryBacktestRequest("run", raw, source)
			if e != nil {
				t.Fatal(e)
			}
			p, e := PrepareOrdinaryBacktestAdmission(c)
			ordinaryTestFault(t, e, v.code)
			if p == nil || p.ConfigIdentity() == "" || p.DataValidated() {
				t.Fatal("admission refusal lost identity or claimed validated data")
			}
			if c.RequestJSON() != raw {
				t.Fatal("preflight sorted/truncated/repaired raw history")
			}
		})
	}
	// Ordinary preliminary admission imposes no invented positive-price rule.
	raw := ordinaryTestRequest(t, "run", func(m map[string]any) {
		m["data"].(map[string]any)["bars"] = []any{[]any{0, -2, -1, -3, -2, 0}, []any{3600000, -2, 0, -4, -1, 1}}
	})
	c, e := DecodeOrdinaryBacktestRequest("run", raw, source)
	if e != nil {
		t.Fatal(e)
	}
	p, e := PrepareOrdinaryBacktestAdmission(c)
	if e != nil || !p.DataValidated() {
		t.Fatalf("finite negative prices were financially reinterpreted: %v", e)
	}
}
