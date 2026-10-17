package engine_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/testsupport"
)

func inspectionSource(policy string) string {
	return "dsl v7\nstrategy \"Authored Ω\" { description \"No frozen storage identity\" }\n" +
		"market { sequential symbol XAUUSD sequential timeframe 15m }\n" +
		"setup { type: sequential full sequential profile seq.full.public_approx.v1 sequential policy " + policy + " }\n" +
		"risk { sequential riskUsd 31 sequential maxNotionalUsd 1000 }\n"
}

func TestSequentialInspectionExistingParserCorpus(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(testsupport.StratConformanceRoot(), "parse", "*.strat"))
	if err != nil || len(paths) != 165 {
		t.Fatalf("parser corpus: %d %v", len(paths), err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := dsl.Parse(string(bytes))
			if err != nil {
				t.Fatal(err)
			}
			got := engine.InspectSequentialSource(string(bytes))
			if !got.OK || got.Error != nil || got.Result == nil || !reflect.DeepEqual(*got.Result, parsed) {
				t.Fatalf("Go parse result changed: %+v", got)
			}
			if got.GrammarHash != dsl.GrammarManifestSHA256 {
				t.Fatal("grammar identity")
			}
			if len(parsed.Errors) > 0 {
				if got.ConfigIdentity != nil || got.Classification != nil || got.Capabilities.SourceParsed.Status != "unsupported" || got.Capabilities.EngineSupported.Status == "supported" {
					t.Fatal("parse error acquired support")
				}
			} else if got.ConfigIdentity == nil || got.Capabilities.SourceParsed.Status != "supported" {
				t.Fatal("valid parse identity missing")
			}
		})
	}
}

func TestSequentialInspectionFullFactsAndNoReadiness(t *testing.T) {
	for _, policy := range []string{"E1", "E2"} {
		source := inspectionSource(policy)
		got := engine.InspectSequentialSource(source)
		if !got.OK || got.Result == nil || len(got.Result.Errors) != 0 {
			t.Fatalf("parse: %+v", got)
		}
		if got.Classification == nil || got.Classification.Family != "sequentialFull" || got.Classification.Policy != policy || got.Classification.Symbol != "XAUUSD" || got.Classification.Timeframe != "15m" {
			t.Fatalf("classification: %+v", got.Classification)
		}
		c := got.Capabilities
		if c.EngineSupported.Status != "supported" || c.GenericSourceExecution.Status != "supported" || c.SourceAccounting.Status != "unsupported" || c.FixtureAccounting.Status != "unknown" {
			t.Fatalf("source/fixture distinction: %+v", c)
		}
		for _, fact := range []engine.InspectionFact{c.CalendarExecution, c.PreserveOpenEndpoint, c.PreservedPrefix, c.CheckpointResume} {
			if fact.Status != "unsupported" {
				t.Fatal("unimplemented capability promoted")
			}
		}
		for _, fact := range []engine.InspectionFact{c.Recovery, c.MarketDataReadiness, c.ConsumerIntegration} {
			if fact.Status != "unknown" {
				t.Fatal("uninspected readiness promoted")
			}
		}
		costs := got.GenericExecutionContract.Costs
		if !reflect.DeepEqual(costs.FillOn, []string{"open", "nextOpen"}) || costs.ZeroStartEquityDefault != 10000 || costs.NumericDomain != "finite-nonnegative" || costs.TradePnL != "exit-credit-excludes-entry-fee" {
			t.Fatalf("retained cost contract: %+v", costs)
		}
		sha := sha256.Sum256([]byte(source))
		if got.Source.SHA256 != hex.EncodeToString(sha[:]) || got.Source.UTF8Bytes != len(source) {
			t.Fatal("source binding")
		}
		cfg, err := json.Marshal(got.Result.Config)
		if err != nil {
			t.Fatal(err)
		}
		sha = sha256.Sum256(cfg)
		if got.ConfigIdentity.SHA256 != hex.EncodeToString(sha[:]) || got.ConfigIdentity.Encoding != engine.SequentialInspectionConfigEncoding {
			t.Fatal("Go config binding")
		}
	}
}

func TestSequentialInspectionIdentityAndIsolation(t *testing.T) {
	source := inspectionSource("E1")
	a := engine.InspectSequentialSource(source)
	b := engine.InspectSequentialSource("# harmless source comment\n" + source + "\n")
	if a.Source.SHA256 == b.Source.SHA256 || a.ConfigIdentity.SHA256 != b.ConfigIdentity.SHA256 {
		t.Fatal("raw and config identity conflated")
	}
	c := engine.InspectSequentialSource(strings.Replace(source, "riskUsd 31", "riskUsd 32", 1))
	if a.ConfigIdentity.SHA256 == c.ConfigIdentity.SHA256 {
		t.Fatal("config change not bound")
	}
	before, _ := json.Marshal(engine.InspectSequentialSource(source))
	a.Result.Config["name"] = "mutated"
	a.Result.Config["sequentialFull"].(map[string]any)["riskUsd"] = 99
	a.Capabilities.EngineSupported.Status = "unsupported"
	a.GenericExecutionContract.Costs.FillOn[0] = "close"
	a.GenericExecutionContract.Surfaces[0] = "invented"
	a.Capabilities.GenericSourceExecution.Constraints[0] = "invented"
	after, _ := json.Marshal(engine.InspectSequentialSource(source))
	if string(before) != string(after) {
		t.Fatal("inspection state aliases a prior caller result")
	}
}

func TestSequentialInspectionOutOfScopeAndMalformedSource(t *testing.T) {
	root := testsupport.MustRepoRoot()
	for _, path := range []string{"conformance/parse/setup-legacy-setup9.strat", "conformance/parse/setup-legacy-setup9-perf-seasonal.strat"} {
		raw, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		got := engine.InspectSequentialSource(string(raw))
		if !got.OK || len(got.Result.Errors) != 0 || got.Capabilities.EngineSupported.Status != "unknown" || got.GenericExecutionContract != nil {
			t.Fatalf("out-of-scope became supported/refused: %+v", got)
		}
	}
	for _, replacement := range [][2]string{{"policy E1", "policy P1"}, {"riskUsd 31", "riskUsd 0"}, {"timeframe 15m", "timeframe 13m"}, {"public_approx.v1", "public_approx.v99"}, {"symbol XAUUSD", "symbol xauusd"}} {
		got := engine.InspectSequentialSource(strings.Replace(inspectionSource("E1"), replacement[0], replacement[1], 1))
		if !got.OK || got.Result == nil || len(got.Result.Errors) == 0 || got.ConfigIdentity != nil || got.GenericExecutionContract != nil || got.Capabilities.EngineSupported.Status == "supported" {
			t.Fatalf("invalid source became support: %+v", got)
		}
	}
}

func TestSequentialInspectionTransportBoundsAndSafeFailures(t *testing.T) {
	for _, source := range []string{"", string([]byte{0xff}), strings.Repeat("x", engine.SequentialInspectionMaxSource+1)} {
		got := engine.InspectSequentialSource(source)
		if got.OK || got.Error == nil || got.Error.Code != "SEQUENTIAL_INSPECTION_SOURCE" || got.Result != nil || got.Source != nil || got.Capabilities != nil || got.ConfigIdentity != nil || got.Classification != nil {
			t.Fatalf("unsafe source accepted: %+v", got)
		}
	}
	got := engine.SequentialInspectionFailure("secret raw exception")
	bytes, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bytes), "secret") || got.Error.Code != "SEQUENTIAL_INSPECTION_INTERNAL" {
		t.Fatal("raw error escaped")
	}
	valid := engine.InspectSequentialSource(inspectionSource("E1"))
	if valid.Producer.GoVersion == "" || valid.Producer.GOOS == "" || valid.Producer.GOARCH == "" || valid.Producer.ModuleVersion == "" {
		t.Fatal("empty producer stamp")
	}
	if strings.Contains(string(bytes), "assetSha256") || strings.Contains(string(bytes), "releaseTag") {
		t.Fatal("self-asserted artifact identity")
	}
}
