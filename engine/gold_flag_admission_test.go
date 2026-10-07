package engine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const goldFlagAdmissionSource = `dsl v7
strategy "Dedicated native fixture" { description "Synthetic" }
market { goldflag timeframe M30 from M15 }
setup { type: gold flag reference
goldflag policy PR388_CAUSAL_STRESS_V1
}`

func goldFlagAdmissionConfig(t *testing.T) dsl.Config {
	t.Helper()
	p, e := dsl.Parse(goldFlagAdmissionSource)
	if e != nil || len(p.Errors) > 0 {
		t.Fatalf("parse:%v %v", e, p.Errors)
	}
	return p.Config
}
func TestGoldFlagReservedEveryGenericBoundary(t *testing.T) {
	fixture, oldCfg := loadEntryAttemptCase(t, "family-channel-break-hold")
	old := RunRequest{Config: oldCfg, Series: marketdata.SeriesFromBars(fixture.Bars), Symbol: fixture.Symbol, Timeframe: fixture.Timeframe}
	shared, e := PrepareSharedRunContext(old)
	if e != nil {
		t.Fatal(e)
	}
	mutations := map[string]func(dsl.Config){"valid": func(c dsl.Config) {}, "missing object": func(c dsl.Config) { delete(c, "goldFlagReference") }, "null": func(c dsl.Config) { c["goldFlagReference"] = nil }, "scalar": func(c dsl.Config) { c["goldFlagReference"] = true }, "typed family": func(c dsl.Config) { c["setupType"] = dsl.FamilyGoldFlagReference }, "case object": func(c dsl.Config) { c["GoldFlagReference"] = c["goldFlagReference"]; delete(c, "goldFlagReference") }, "case family": func(c dsl.Config) { c["setupType"] = "GoldFlagReference"; delete(c, "goldFlagReference") }, "case discriminator key": func(c dsl.Config) {
		c["SetupType"] = "GoldFlagReference"
		c["setupType"] = "timedReturn"
		delete(c, "goldFlagReference")
	}}
	for _, family := range []string{"timedReturn", "flagContinuation", "clockRangeBreakout", "frozenLevelBreakout", "", "unknown"} {
		f := family
		mutations["relabeled "+f] = func(c dsl.Config) { c["setupType"] = f }
	}
	for name, mutate := range mutations {
		for _, route := range []string{"normal", "empty", "malformed", "off-route", "source", "force"} {
			t.Run(name+"/"+route, func(t *testing.T) {
				cfg := goldFlagAdmissionConfig(t)
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
					if e := call(); e == nil || !strings.Contains(e.Error(), dsl.GoldFlagReferenceDedicatedRunnerRequired) {
						t.Errorf("%s bypass:%v", path, e)
					}
				}
			})
		}
	}
	if _, e := RunFixtureCase(RunFixture{}, goldFlagAdmissionSource); e == nil || !strings.Contains(e.Error(), dsl.GoldFlagReferenceDedicatedRunnerRequired) {
		t.Fatalf("fixture bypass:%v", e)
	}
	if _, e := Run(old); e != nil {
		t.Fatalf("legacy positive control:%v", e)
	}
}

func TestGoldFlagAuthoredTailRejectedBeforeGenericExecution(t *testing.T) {
	for _, clause := range []string{"risk 100 USD goldflag policy PR388_CAUSAL_STRESS_V1", "stop 2 ATR goldflag policy PR388_CAUSAL_STRESS_V1", "stop 2 ATR goldFlagReference null", "unknown goldFlagReference null", "type: flag continuation goldflag\npolicy PR388_CAUSAL_STRESS_V1"} {
		source := "dsl v7\nstrategy \"Adversarial\"\nsetup { " + clause + " }"
		parsed, e := dsl.Parse(source)
		if e != nil || len(parsed.Errors) == 0 || len(parsed.Config) != 0 {
			t.Fatalf("tail became runnable config: %q %v %+v", clause, e, parsed)
		}
		for _, fixture := range []RunFixture{{}, {Symbol: "XAUUSD", Timeframe: "30m"}} {
			if _, e := RunFixtureCase(fixture, source); e == nil {
				t.Fatal("generic fixture accepted tail", clause)
			}
		}
	}
}

func TestGoldFlagUnicodeAliasesCannotReachGenericRoutes(t *testing.T) {
	fixture, legacy := loadEntryAttemptCase(t, "family-channel-break-hold")
	for _, space := range []string{"\u00a0", "\u2003", "\u202f", "\v", "\f", "\u2028"} {
		for _, kind := range []string{"internal key", "padded key", "family", "discriminator"} {
			cfg := dsl.Config{}
			for k, v := range legacy {
				cfg[k] = v
			}
			switch kind {
			case "internal key":
				cfg["gold"+space+"FlagReference"] = nil
			case "padded key":
				cfg[space+"goldFlagReference"+space] = nil
			case "family":
				cfg["setupType"] = "gold" + space + "FlagReference"
			case "discriminator":
				cfg["setup"+space+"Type"] = "goldFlagReference"
			}
			r := RunRequest{Config: cfg, Series: marketdata.SeriesFromBars(fixture.Bars), Symbol: fixture.Symbol, Timeframe: fixture.Timeframe}
			for name, call := range map[string]func() error{"Run": func() error { _, e := Run(r); return e }, "PrepareRun": func() error { _, e := PrepareRun(r); return e }, "prefix": func() error { _, e := RunPrefix(r); return e }, "shared": func() error { _, e := PrepareSharedRunContext(r); return e }} {
				if e := call(); e == nil || !strings.Contains(e.Error(), dsl.GoldFlagReferenceDedicatedRunnerRequired) {
					t.Fatalf("%s %s unicode %q escaped: %v", name, kind, space, e)
				}
			}
		}
	}
}
