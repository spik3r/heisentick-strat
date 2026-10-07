package engine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const adaptiveFlagAdmissionSource = `dsl v7
strategy "Adaptive volume flag INITIAL synthetic control" {
  description "Input identity only; dedicated raw reference runner"
}
market {
  adaptiveflag timeframe M30
}
setup {
  type: adaptive volume flag
  adaptiveflag policy DELAYED_OHLC_REFERENCE_V1
  adaptiveflag bundle INITIAL
  adaptiveflag pivotSensitivity 3
  adaptiveflag minPoleATR 1.8
  adaptiveflag minFlagBars 3
  adaptiveflag maxFlagBars 16
  adaptiveflag maxFlagRetrace 0.5
  adaptiveflag useVolumeFilter true
  adaptiveflag useEMATrend true
  adaptiveflag fastEMALen 50
  adaptiveflag slowEMALen 200
  adaptiveflag targetR 2.5
  adaptiveflag atrStopMult 1.2
  adaptiveflag validBars 12
  adaptiveflag maxHold 60
  adaptiveflag atrLen 14
  adaptiveflag volumeSMALen 20
  adaptiveflag volumeSMAMult 0.9
  adaptiveflag flagWidthPoleMult 0.55
  adaptiveflag entryBufferATR 0.05
}
`

func adaptiveFlagAdmissionConfig(t *testing.T) dsl.Config {
	t.Helper()
	p, e := dsl.Parse(adaptiveFlagAdmissionSource)
	if e != nil || len(p.Errors) > 0 {
		t.Fatalf("parse:%v %v", e, p.Errors)
	}
	return p.Config
}
func TestAdaptiveFlagReservedEveryGenericBoundary(t *testing.T) {
	fixture, oldCfg := loadEntryAttemptCase(t, "family-channel-break-hold")
	old := RunRequest{Config: oldCfg, Series: marketdata.SeriesFromBars(fixture.Bars), Symbol: fixture.Symbol, Timeframe: fixture.Timeframe}
	shared, e := PrepareSharedRunContext(old)
	if e != nil {
		t.Fatal(e)
	}
	mutations := map[string]func(dsl.Config){"valid": func(c dsl.Config) {}, "missing object": func(c dsl.Config) { delete(c, "adaptiveVolumeFlag") }, "null": func(c dsl.Config) { c["adaptiveVolumeFlag"] = nil }, "scalar": func(c dsl.Config) { c["adaptiveVolumeFlag"] = true }, "typed family": func(c dsl.Config) { c["setupType"] = dsl.FamilyAdaptiveVolumeFlag }, "case object": func(c dsl.Config) { c["AdaptiveVolumeFlag"] = c["adaptiveVolumeFlag"]; delete(c, "adaptiveVolumeFlag") }, "case family": func(c dsl.Config) { c["setupType"] = "AdaptiveVolumeFlag"; delete(c, "adaptiveVolumeFlag") }, "case discriminator key": func(c dsl.Config) {
		c["SetupType"] = "AdaptiveVolumeFlag"
		c["setupType"] = "timedReturn"
		delete(c, "adaptiveVolumeFlag")
	}}
	for _, family := range []string{"timedReturn", "flagContinuation", "clockRangeBreakout", "frozenLevelBreakout", "", "unknown"} {
		f := family
		mutations["relabeled "+f] = func(c dsl.Config) { c["setupType"] = f }
	}
	for name, mutate := range mutations {
		for _, route := range []string{"normal", "empty", "malformed", "off-route", "source", "force"} {
			t.Run(name+"/"+route, func(t *testing.T) {
				cfg := adaptiveFlagAdmissionConfig(t)
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
					if e := call(); e == nil || !strings.Contains(e.Error(), dsl.AdaptiveFlagDedicatedRunnerRequired) {
						t.Errorf("%s bypass:%v", path, e)
					}
				}
			})
		}
	}
	if _, e := RunFixtureCase(RunFixture{}, adaptiveFlagAdmissionSource); e == nil || !strings.Contains(e.Error(), dsl.AdaptiveFlagDedicatedRunnerRequired) {
		t.Fatalf("fixture bypass:%v", e)
	}
	if _, e := Run(old); e != nil {
		t.Fatalf("legacy positive control:%v", e)
	}
}

func TestAdaptiveFlagAuthoredTailRejectedBeforeGenericExecution(t *testing.T) {
	for _, clause := range []string{"risk 100 USD adaptiveflag policy DELAYED_OHLC_REFERENCE_V1", "stop 2 ATR adaptiveflag policy DELAYED_OHLC_REFERENCE_V1", "stop 2 ATR adaptiveVolumeFlag null", "unknown adaptiveVolumeFlag null", "type: flag continuation adaptiveflag\npolicy DELAYED_OHLC_REFERENCE_V1"} {
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

func TestAdaptiveFlagUnicodeAliasesCannotReachGenericRoutes(t *testing.T) {
	fixture, legacy := loadEntryAttemptCase(t, "family-channel-break-hold")
	for _, space := range []string{"\u00a0", "\u2003", "\u202f", "\v", "\f", "\u2028"} {
		for _, kind := range []string{"internal key", "padded key", "family", "discriminator"} {
			cfg := dsl.Config{}
			for k, v := range legacy {
				cfg[k] = v
			}
			switch kind {
			case "internal key":
				cfg["adaptive"+space+"VolumeFlag"] = nil
			case "padded key":
				cfg[space+"adaptiveVolumeFlag"+space] = nil
			case "family":
				cfg["setupType"] = "adaptive" + space + "VolumeFlag"
			case "discriminator":
				cfg["setup"+space+"Type"] = "adaptiveVolumeFlag"
			}
			r := RunRequest{Config: cfg, Series: marketdata.SeriesFromBars(fixture.Bars), Symbol: fixture.Symbol, Timeframe: fixture.Timeframe}
			for name, call := range map[string]func() error{"Run": func() error { _, e := Run(r); return e }, "PrepareRun": func() error { _, e := PrepareRun(r); return e }, "prefix": func() error { _, e := RunPrefix(r); return e }, "shared": func() error { _, e := PrepareSharedRunContext(r); return e }} {
				if e := call(); e == nil || !strings.Contains(e.Error(), dsl.AdaptiveFlagDedicatedRunnerRequired) {
					t.Fatalf("%s %s unicode %q escaped: %v", name, kind, space, e)
				}
			}
		}
	}
}

func TestAdaptiveFlagPunctuationTailRejectedByPublicEngine(t *testing.T) {
	fixture, _ := loadEntryAttemptCase(t, "family-channel-break-hold")
	for _, head := range []string{"adaptive:flag", "adaptive.flag", "adaptive/flag", "adaptive | flag"} {
		source := "dsl v7\nstrategy \"Review\"\nsetup { type: flag continuation }\nrisk { stop 2 ATR " + head + " targetR 2.5 }"
		if _, e := RunFixtureCase(fixture, source); e == nil {
			t.Fatalf("generic execution accepted %q", head)
		}
	}
}

func TestAdaptiveFlagDamagedDiscriminatorRejectedAtEveryEngineBoundary(t *testing.T) {
	fixture, legacy := loadEntryAttemptCase(t, "family-channel-break-hold")
	old := RunRequest{Config: legacy, Series: marketdata.SeriesFromBars(fixture.Bars), Symbol: fixture.Symbol, Timeframe: fixture.Timeframe}
	shared, e := PrepareSharedRunContext(old)
	if e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"setup.type", "setup:type", "setup/type", "setup|type", "setup[type]", "Setup+Type"} {
		cfg := dsl.Config{}
		for k, v := range legacy {
			cfg[k] = v
		}
		cfg[key] = "adaptiveVolumeFlag"
		r := old
		r.Config = cfg
		calls := map[string]func() error{
			"Run": func() error { _, e := Run(r); return e }, "PrepareRun": func() error { _, e := PrepareRun(r); return e },
			"shared key": func() error { _, e := SharedContextKey(r); return e }, "shared": func() error { _, e := PrepareSharedRunContext(r); return e },
			"variant": func() error { _, e := shared.PrepareVariant(cfg); return e }, "validate": func() error { return validateRunRequest(r) },
			"prefix": func() error { _, e := RunPrefix(r); return e }, "resumable": func() error { _, _, e := RunPrefixResumable(r, json.RawMessage(`{}`)); return e },
		}
		for name, call := range calls {
			if e := call(); e == nil || !strings.Contains(e.Error(), dsl.AdaptiveFlagDedicatedRunnerRequired) {
				t.Fatalf("%s %s bypass:%v", key, name, e)
			}
		}
	}
}
