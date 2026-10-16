package engine

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/testsupport"
)

func minimalDonchianConfig() dsl.Config {
	return dsl.Config{"setupType": string(dsl.FamilyDonchianBreakout), "donchianBreakout": map[string]any{"channelBars": 3}}
}

func TestDonchianConfigDefaultsAndNumericRepresentations(t *testing.T) {
	for name, mutate := range map[string]func(dsl.Config){
		"absent optional values": func(dsl.Config) {},
		"empty optional maps": func(c dsl.Config) {
			for _, k := range []string{"stop", "target", "entryMode"} {
				c[k] = map[string]any{}
			}
		},
		"typed configs": func(c dsl.Config) {
			c["setupType"] = dsl.FamilyDonchianBreakout
			c["donchianBreakout"] = dsl.Config{"channelBars": 3}
			c["stop"] = dsl.Config{}
			c["target"] = dsl.Config{}
			c["entryMode"] = dsl.Config{}
		},
		"canonical zero defaults": func(c dsl.Config) {
			c["stop"] = map[string]any{"type": "atr", "atrMult": 2, "paddingAtr": 0, "minAtr": 0.0, "maxAtr": int64(0), "extremeCandles": float32(0)}
			c["target"] = map[string]any{"edge": "range", "fallbackR": 1, "minR": 0.6, "dcbR": 0}
			mapValue(c, "donchianBreakout")["trendEmaLen"] = 0
			c["entryMode"] = map[string]any{"type": "market", "expireCandles": 5}
			c["entryTf"] = "current"
			c["sourceTimeframe"] = ""
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := minimalDonchianConfig()
			mutate(cfg)
			before, _ := json.Marshal(cfg)
			if err := validateDonchianBreakoutConfig(cfg); err != nil {
				t.Fatal(err)
			}
			after, _ := json.Marshal(cfg)
			if string(before) != string(after) {
				t.Fatal("validation mutated config")
			}
			p := paramsFromConfig(cfg)
			if p.DonchianBreakout.ChannelBars != 3 || p.DonchianBreakout.TrendEMALen != 0 || p.DonchianBreakout.StopATR != 2 || p.TargetR != 0 {
				t.Fatalf("defaults = %+v target=%v", p.DonchianBreakout, p.TargetR)
			}
		})
	}
	for _, raw := range []any{int(5), int64(5), float32(5), float64(5)} {
		cfg := minimalDonchianConfig()
		mapValue(cfg, "donchianBreakout")["channelBars"] = raw
		mapValue(cfg, "donchianBreakout")["trendEmaLen"] = raw
		cfg["stop"] = map[string]any{"atrMult": raw}
		cfg["target"] = map[string]any{"dcbR": raw}
		if err := validateDonchianBreakoutConfig(cfg); err != nil {
			t.Fatalf("%T: %v", raw, err)
		}
		p := paramsFromConfig(cfg)
		if p.DonchianBreakout.ChannelBars != 5 || p.DonchianBreakout.TrendEMALen != 5 || p.DonchianBreakout.StopATR != 5 || p.TargetR != 5 {
			t.Fatalf("%T: decoded %+v target=%v", raw, p.DonchianBreakout, p.TargetR)
		}
	}
	for _, zero := range []any{int(0), int64(0), float32(0), float64(0), math.Copysign(0, -1)} {
		cfg := minimalDonchianConfig()
		mapValue(cfg, "donchianBreakout")["trendEmaLen"] = zero
		cfg["target"] = map[string]any{"dcbR": zero}
		if err := validateDonchianBreakoutConfig(cfg); err != nil {
			t.Fatalf("zero %T: %v", zero, err)
		}
	}
}

func TestDonchianConfigRejectsMalformedExplicitValues(t *testing.T) {
	invalid := []any{nil, "3", true, json.Number("3"), uint(3), -1, 0.5, math.NaN(), math.Inf(1), math.Inf(-1), math.Ldexp(1, strconv.IntSize-1)}
	for _, field := range []string{"channelBars", "trendEmaLen"} {
		for _, raw := range invalid {
			cfg := minimalDonchianConfig()
			mapValue(cfg, "donchianBreakout")[field] = raw
			if err := validateDonchianBreakoutConfig(cfg); err == nil {
				t.Errorf("%s %T(%v) accepted", field, raw, raw)
			}
		}
	}
	for _, raw := range []any{nil, "2", true, math.NaN(), math.Inf(1), math.Inf(-1), -1, 0} {
		cfg := minimalDonchianConfig()
		cfg["stop"] = map[string]any{"atrMult": raw}
		if err := validateDonchianBreakoutConfig(cfg); err == nil {
			t.Errorf("atrMult %v accepted", raw)
		}
	}
	for _, raw := range []any{nil, "1", true, math.NaN(), math.Inf(1), math.Inf(-1), -1} {
		cfg := minimalDonchianConfig()
		cfg["target"] = map[string]any{"dcbR": raw}
		if err := validateDonchianBreakoutConfig(cfg); err == nil {
			t.Errorf("dcbR %v accepted", raw)
		}
	}
	for _, field := range []string{"donchianBreakout", "stop", "target", "entryMode"} {
		for _, raw := range []any{nil, map[string]any(nil), dsl.Config(nil), "invalid", 3, []any{}} {
			cfg := minimalDonchianConfig()
			cfg[field] = raw
			if err := validateDonchianBreakoutConfig(cfg); err == nil {
				t.Errorf("%s map %v accepted", field, raw)
			}
		}
	}
	for _, field := range []string{"paddingAtr", "minAtr", "maxAtr", "extremeCandles"} {
		for _, raw := range []any{1, -1, nil, "0", math.NaN()} {
			cfg := minimalDonchianConfig()
			cfg["stop"] = map[string]any{field: raw}
			if err := validateDonchianBreakoutConfig(cfg); err == nil {
				t.Errorf("unsupported stop %s=%v accepted", field, raw)
			}
		}
	}
	for _, tc := range []struct {
		key, member string
		raw         any
	}{
		{"stop", "type", "fixedAtr"}, {"stop", "type", nil}, {"stop", "type", 3},
		{"entryMode", "type", "limitSweptEdge"}, {"entryMode", "type", "nextOpen"}, {"entryMode", "type", nil}, {"entryMode", "type", 3},
		{"entryTf", "", "1h"}, {"entryTf", "", ""}, {"entryTf", "", nil}, {"entryTf", "", 3},
		{"sourceTimeframe", "", "4h"}, {"sourceTimeframe", "", nil}, {"sourceTimeframe", "", 3},
	} {
		cfg := minimalDonchianConfig()
		if tc.member == "" {
			cfg[tc.key] = tc.raw
		} else {
			cfg[tc.key] = map[string]any{tc.member: tc.raw}
		}
		if err := validateDonchianBreakoutConfig(cfg); err == nil {
			t.Errorf("%s.%s=%v accepted", tc.key, tc.member, tc.raw)
		}
	}
	for _, cfg := range []dsl.Config{nil, {"setupType": string(dsl.FamilyPriceMomentum), "stop": nil}, {"setupType": "flag", "donchianBreakout": "bad"}} {
		if err := validateDonchianBreakoutConfig(cfg); err != nil {
			t.Fatalf("changed other-family contract: %v", err)
		}
	}
	cfg := minimalDonchianConfig()
	delete(cfg, "donchianBreakout")
	if err := validateDonchianBreakoutConfig(cfg); err == nil {
		t.Fatal("missing channel map accepted")
	}
	cfg = minimalDonchianConfig()
	delete(mapValue(cfg, "donchianBreakout"), "channelBars")
	if err := validateDonchianBreakoutConfig(cfg); err == nil {
		t.Fatal("missing channel accepted")
	}
	cfg = minimalDonchianConfig()
	mapValue(cfg, "donchianBreakout")["channelBars"] = 0
	if err := validateDonchianBreakoutConfig(cfg); err == nil {
		t.Fatal("zero channel accepted")
	}
}

func TestDonchianCountPreservesRepresentableBoundaries(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	bound := math.Ldexp(1, strconv.IntSize-1)
	below := math.Trunc(math.Nextafter(bound, 0))
	for _, raw := range []any{maxInt, int64(maxInt), below, float64(2147483647)} {
		count, ok := donchianCount(raw)
		if !ok || count <= 0 {
			t.Fatalf("representable %T(%v) refused", raw, raw)
		}
		cfg := minimalDonchianConfig()
		mapValue(cfg, "donchianBreakout")["channelBars"] = raw
		mapValue(cfg, "donchianBreakout")["trendEmaLen"] = raw
		if err := validateDonchianBreakoutConfig(cfg); err != nil {
			t.Fatal(err)
		}
		p := paramsFromConfig(cfg)
		if p.DonchianBreakout.ChannelBars != count || p.DonchianBreakout.TrendEMALen != count {
			t.Fatalf("count %T(%v) changed: %+v", raw, raw, p.DonchianBreakout)
		}
		if reflect.TypeOf(raw).Kind() == reflect.Int || reflect.TypeOf(raw).Kind() == reflect.Int64 {
			if count != maxInt {
				t.Fatalf("native maximum rounded: %d != %d", count, maxInt)
			}
		}
	}
	for _, raw := range []any{bound, math.Nextafter(bound, math.Inf(1)), float32(bound), 1e20} {
		if count, ok := donchianCount(raw); ok {
			t.Fatalf("out-of-range %T(%v) accepted as %d", raw, raw, count)
		}
	}
}

func donchianValidationFixture(t *testing.T) (RunFixture, string) {
	t.Helper()
	dir := filepath.Join(testsupport.StratConformanceRoot(), "semantic", "dcb-signal-close-entry")
	f, err := LoadRunFixture(filepath.Join(dir, "fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(dir, "strategy.strat"))
	if err != nil {
		t.Fatal(err)
	}
	return f, string(source)
}

func TestDonchianMalformedConfigRejectedAcrossRunRoutes(t *testing.T) {
	fixture, source := donchianValidationFixture(t)
	request := func() RunRequest {
		parsed, err := dsl.Parse(source)
		if err != nil || len(parsed.Errors) != 0 {
			t.Fatalf("parse: %v %v", err, parsed.Errors)
		}
		return RunRequest{Config: parsed.Config, Series: marketdata.SeriesFromBars(fixture.Bars), Symbol: fixture.Symbol, Timeframe: fixture.Timeframe, Costs: fixture.Costs}
	}
	shared, err := PrepareSharedRunContext(request())
	if err != nil {
		t.Fatal(err)
	}
	routes := map[string]func(RunRequest) error{
		"Run":                     func(r RunRequest) error { _, err := Run(r); return err },
		"PrepareRun":              func(r RunRequest) error { _, err := PrepareRun(r); return err },
		"SharedContextKey":        func(r RunRequest) error { _, err := SharedContextKey(r); return err },
		"PrepareSharedRunContext": func(r RunRequest) error { _, err := PrepareSharedRunContext(r); return err },
		"PrepareVariant":          func(r RunRequest) error { _, err := shared.PrepareVariant(r.Config); return err },
	}
	mutations := map[string]func(dsl.Config){
		"overflow trend":    func(c dsl.Config) { mapValue(c, "donchianBreakout")["trendEmaLen"] = 1e20 },
		"NaN target":        func(c dsl.Config) { mapValue(c, "target")["dcbR"] = math.NaN() },
		"NaN stop":          func(c dsl.Config) { mapValue(c, "stop")["atrMult"] = math.NaN() },
		"unsupported entry": func(c dsl.Config) { mapValue(c, "entryMode")["type"] = "nextOpen" },
	}
	for name, mutate := range mutations {
		for route, run := range routes {
			for _, force := range []bool{false, true} {
				t.Run(name+"/"+route+"/force="+strconv.FormatBool(force), func(t *testing.T) {
					r := request()
					mutate(r.Config)
					r.Symbol = "EURUSD"
					r.ForceRoute = force
					if err := run(r); err == nil || !strings.Contains(err.Error(), "donchian breakout") {
						t.Fatalf("malformed config did not fail at Donchian validation: %v", err)
					}
				})
			}
		}
	}
	if _, err := RunFixtureCase(fixture, strings.Replace(source, "channel 3 candles", "channel 100000000000000000000 candles", 1)); err == nil {
		t.Fatal("fixture route accepted invalid count")
	}
}
