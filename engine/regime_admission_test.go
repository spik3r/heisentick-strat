package engine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const regimeAdmissionSource = `dsl v7
strategy "Dedicated native fixture" { description "Synthetic" }
market { regime timeframe M30 from M5 }
setup { type: regime engine
regime profile v9-floor-half-v1
regime mode source-like-v1
}`

func regimeAdmissionConfig(t *testing.T) dsl.Config {
	t.Helper()
	p, e := dsl.Parse(regimeAdmissionSource)
	if e != nil || len(p.Errors) > 0 {
		t.Fatalf("parse:%v %v", e, p.Errors)
	}
	return p.Config
}
func TestRegimeReservedEveryGenericBoundary(t *testing.T) {
	fixture, oldCfg := loadEntryAttemptCase(t, "family-channel-break-hold")
	old := RunRequest{Config: oldCfg, Series: marketdata.SeriesFromBars(fixture.Bars), Symbol: fixture.Symbol, Timeframe: fixture.Timeframe}
	shared, e := PrepareSharedRunContext(old)
	if e != nil {
		t.Fatal(e)
	}
	mutations := map[string]func(dsl.Config){"valid": func(c dsl.Config) {}, "missing object": func(c dsl.Config) { delete(c, "regimeEngine") }, "null": func(c dsl.Config) { c["regimeEngine"] = nil }, "scalar": func(c dsl.Config) { c["regimeEngine"] = true }, "typed family": func(c dsl.Config) { c["setupType"] = dsl.FamilyRegimeEngine }, "case object": func(c dsl.Config) { c["RegimeEngine"] = c["regimeEngine"]; delete(c, "regimeEngine") }, "case family": func(c dsl.Config) { c["setupType"] = "RegimeEngine"; delete(c, "regimeEngine") }, "case discriminator key": func(c dsl.Config) {
		c["SetupType"] = "RegimeEngine"
		c["setupType"] = "timedReturn"
		delete(c, "regimeEngine")
	}}
	for _, family := range []string{"timedReturn", "flagContinuation", "clockRangeBreakout", "frozenLevelBreakout", "", "unknown"} {
		f := family
		mutations["relabeled "+f] = func(c dsl.Config) { c["setupType"] = f }
	}
	for name, mutate := range mutations {
		for _, route := range []string{"normal", "empty", "malformed", "off-route", "source", "force"} {
			t.Run(name+"/"+route, func(t *testing.T) {
				cfg := regimeAdmissionConfig(t)
				mutate(cfg)
				r := old
				r.Config = cfg
				switch route {
				case "empty":
					r.Series = marketdata.Series{}
				case "malformed":
					r.Series.O = nil
				case "off-route":
					r.Symbol = "WRONG"
					r.Timeframe = "1w"
				case "source":
					cfg["sourceTimeframe"] = "15m"
					cfg["entryTf"] = "1m"
					r.Symbol = "XAUUSD"
					r.Timeframe = "1m"
				case "force":
					r.ForceRoute = true
				}
				calls := map[string]func() error{"Run": func() error { _, e := Run(r); return e }, "PrepareRun": func() error { _, e := PrepareRun(r); return e }, "shared key": func() error { _, e := SharedContextKey(r); return e }, "shared": func() error { _, e := PrepareSharedRunContext(r); return e }, "variant": func() error { _, e := shared.PrepareVariant(cfg); return e }, "nil variant": func() error { var s *SharedRunContext; _, e := s.PrepareVariant(cfg); return e }, "validate": func() error { return validateRunRequest(r) }, "prefix": func() error { _, e := RunPrefix(r); return e }, "resumable": func() error { _, _, e := RunPrefixResumable(r, json.RawMessage(`{}`)); return e }}
				for path, call := range calls {
					if e := call(); e == nil || !strings.Contains(e.Error(), dsl.RegimeEngineDedicatedRunnerRequired) {
						t.Errorf("%s bypass:%v", path, e)
					}
				}
			})
		}
	}
	if _, e := RunFixtureCase(RunFixture{}, regimeAdmissionSource); e == nil || !strings.Contains(e.Error(), dsl.RegimeEngineDedicatedRunnerRequired) {
		t.Fatalf("fixture bypass:%v", e)
	}
	if _, e := Run(old); e != nil {
		t.Fatalf("legacy positive control:%v", e)
	}
}
