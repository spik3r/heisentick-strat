package engine

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func frozenAdmissionConfig(t *testing.T) dsl.Config {
	t.Helper()
	raw, err := os.ReadFile("../dsl/testdata/frozen_level/config.canonical.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := dsl.DecodeFrozenLevelConfigJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestFrozenRuntimeEveryDirectBoundaryRefusesReservedRoots(t *testing.T) {
	fixture, oldCfg := loadEntryAttemptCase(t, "family-channel-break-hold")
	old := RunRequest{Config: oldCfg, Series: marketdata.SeriesFromBars(fixture.Bars), Symbol: fixture.Symbol, Timeframe: fixture.Timeframe}
	shared, err := PrepareSharedRunContext(old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := shared.PrepareVariant(oldCfg); err != nil {
		t.Fatalf("legacy positive control: %v", err)
	}
	mutations := map[string]func(dsl.Config){
		"valid": func(c dsl.Config) {}, "family ID": func(c dsl.Config) { c["setupType"] = dsl.FamilyFrozenLevelBreakout },
		"missing object": func(c dsl.Config) { delete(c, "frozenLevelBreakout") },
		"null object":    func(c dsl.Config) { c["frozenLevelBreakout"] = nil }, "scalar object": func(c dsl.Config) { c["frozenLevelBreakout"] = true },
		"case object": func(c dsl.Config) {
			c["FrozenLevelBreakout"] = c["frozenLevelBreakout"]
			delete(c, "frozenLevelBreakout")
		},
		"case discriminator": func(c dsl.Config) { c["setupType"] = "FrozenLevelBreakout"; delete(c, "frozenLevelBreakout") },
		"case discriminator key": func(c dsl.Config) {
			c["SetupType"] = c["setupType"]
			c["setupType"] = "timedReturn"
			delete(c, "frozenLevelBreakout")
		},
		"extra key":                func(c dsl.Config) { c["clockRangeBreakout"] = map[string]any{} },
		"unknown integer coercion": func(c dsl.Config) { c["frozenLevelBreakout"].(map[string]any)["entryLatencyMS"] = float64(0) },
		"missing family":           func(c dsl.Config) { delete(c, "setupType") },
	}
	// Relabeling must fail even for a family with an earlier special dispatcher.
	for _, family := range []string{"timedReturn", "clockRangeBreakout", "flagContinuation", "channelBreakHold", "", "unknown"} {
		f := family
		mutations["relabeled "+f] = func(c dsl.Config) { c["setupType"] = f }
	}
	for name, mutate := range mutations {
		for _, route := range []string{"normal", "empty", "malformed", "off-route", "source", "force"} {
			t.Run(name+"/"+route, func(t *testing.T) {
				cfg := frozenAdmissionConfig(t)
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
					r.Symbol = "XAUUSD"
					r.Timeframe = "1m"
					cfg["sourceTimeframe"] = "15m"
					cfg["entryTf"] = "1m"
				case "force":
					r.ForceRoute = true
				}
				calls := map[string]func() error{
					"Run": func() error { _, e := Run(r); return e }, "PrepareRun": func() error { _, e := PrepareRun(r); return e },
					"SharedContextKey": func() error { _, e := SharedContextKey(r); return e }, "PrepareShared": func() error { _, e := PrepareSharedRunContext(r); return e },
					"validate": func() error { return validateRunRequest(r) }, "variant": func() error { _, e := shared.PrepareVariant(cfg); return e },
					"nil variant": func() error { var s *SharedRunContext; _, e := s.PrepareVariant(cfg); return e },
					"prefix":      func() error { _, e := RunPrefix(r); return e }, "resumable": func() error { _, _, e := RunPrefixResumable(r, json.RawMessage(`{}`)); return e },
				}
				for path, call := range calls {
					if err := call(); err == nil || !strings.Contains(err.Error(), dsl.FrozenLevelExecutionUnimplemented) {
						t.Errorf("%s bypassed reserved guard: %v", path, err)
					}
				}
			})
		}
	}
}

func TestFrozenRuntimePreparedAdmissionRootIsNotCallerAlias(t *testing.T) {
	for _, sourceEntry := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "source-entry"}[sourceEntry], func(t *testing.T) {
			cfg := dsl.Config{"setupType": "flagContinuation"}
			r := RunRequest{Config: cfg, Series: sourceProjectionSeries(0, 60000, 40), Symbol: "XAUUSD", Timeframe: "1m"}
			if sourceEntry {
				cfg["sourceTimeframe"] = "15m"
				cfg["entryTf"] = "1m"
				r.SourceSeries = sourceProjectionSeries(0, 900000, 3)
				r.SourceTimeframe = "15m"
			}
			prepared, err := PrepareRun(r)
			if err != nil {
				t.Fatal(err)
			}
			before, err := prepared.RunChecked(Costs{})
			if err != nil {
				t.Fatal(err)
			}
			cfg["setupType"] = dsl.FamilyFrozenLevelBreakout
			cfg["frozenLevelBreakout"] = nil
			if sourceEntry {
				if prepared.c5Config["setupType"] != "flagContinuation" {
					t.Fatal("retained caller root")
				}
				if _, found := prepared.c5Config["frozenLevelBreakout"]; found {
					t.Fatal("reserved object entered handle")
				}
			}
			after, err := prepared.RunChecked(Costs{})
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("prepared admitted strategy changed: %v", err)
			}
			if got := prepared.Run(Costs{}); !reflect.DeepEqual(before, got) {
				t.Fatal("legacy prepared Run changed after external relabel")
			}
			if _, err := Run(r); err == nil || !strings.Contains(err.Error(), dsl.FrozenLevelExecutionUnimplemented) {
				t.Fatal("new direct request admitted mutated root")
			}
		})
	}
}

func TestFrozenRuntimeLegacyWithoutMarkerIsUnchanged(t *testing.T) {
	fixture, cfg := loadEntryAttemptCase(t, "family-channel-break-hold")
	request := RunRequest{Config: cfg, Series: marketdata.SeriesFromBars(fixture.Bars), HTFSeries: marketdata.SeriesFromBars(fixture.HTFBars), Symbol: fixture.Symbol, Timeframe: fixture.Timeframe, Costs: fixture.Costs}
	if err := rejectFrozenLevelExecution(cfg); err != nil {
		t.Fatal(err)
	}
	direct, err := Run(request)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := PrepareSharedRunContext(request)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := shared.PrepareVariant(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := prepared.RunChecked(fixture.Costs)
	if err != nil || !reflect.DeepEqual(direct, got) {
		t.Fatalf("legacy direct/shared mismatch: %v", err)
	}
}
