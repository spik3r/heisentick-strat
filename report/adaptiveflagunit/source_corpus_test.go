package adaptiveflagunit

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
)

// This internal test obtains each owned ledger from one unchanged Stage A Go
// run over frozen invented bytes. It introduces no new public transport and
// never imports or invokes Python. Optional output is original test evidence.
func TestProjectionSourceBackedFrozenCorpus(t *testing.T) {
	root := filepath.Join("..", "..", "testsupport", "testdata")
	data, err := os.ReadFile(filepath.Join(root, "adaptive-flag-runtime-corpus-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if rawBytesSHA(data) != "af392754f48672cfe31d7161454dfc35ee73977e4c38f0a72b21c15be6fb2f97" {
		t.Fatal("raw corpus identity changed")
	}
	var corpus struct {
		Sources map[string]struct {
			Text string `json:"text"`
		} `json:"sources"`
		Assets map[string]struct {
			Payload string `json:"payload"`
		} `json:"assets"`
		RawCases []struct {
			ID        string `json:"id"`
			SourceSHA string `json:"sourceSha256"`
			Metadata  string `json:"metadataJSON"`
			BTB1SHA   string `json:"btb1Sha256"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	unitBytes, err := os.ReadFile(filepath.Join(root, "adaptive-flag-unit-corpus-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var units struct {
		Cases []struct {
			ID       string `json:"id"`
			RawID    string `json:"rawCaseId"`
			Metadata string `json:"projectionMetadataJSON"`
			SHA      string `json:"projectionMetadataSha256"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(unitBytes, &units); err != nil {
		t.Fatal(err)
	}
	if len(corpus.RawCases) != 96 || len(units.Cases) != 288 {
		t.Fatal("frozen source case cardinality")
	}
	output := os.Getenv("ADAPTIVE_UNIT_TEST_OUTPUT_DIR")
	if output != "" {
		if err := os.MkdirAll(output, 0755); err != nil {
			t.Fatal(err)
		}
	}
	type row struct {
		ID                                  string `json:"id"`
		RawFile                             string `json:"rawFile"`
		RawSHA                              string `json:"rawSha256"`
		ProjectionFile                      string `json:"projectionFile"`
		ProjectionSHA                       string `json:"projectionSha256"`
		RequestSHA                          string `json:"projectionRequestSha256"`
		Orders, Costs, Closed, Marks, Daily int
	}
	rows := make([]row, 0, 288)
	for i, c := range corpus.RawCases {
		bars, err := base64.StdEncoding.DecodeString(corpus.Assets[c.BTB1SHA].Payload)
		if err != nil {
			t.Fatal(err)
		}
		if rawBytesSHA(bars) != c.BTB1SHA {
			t.Fatal("BTB1 fixture identity")
		}
		source := corpus.Sources[c.SourceSHA].Text
		if rawBytesSHA([]byte(source)) != c.SourceSHA {
			t.Fatal("DSL fixture identity")
		}
		rawBytes, err := adaptiveflag.BuildRuntime(c.Metadata, source, bars)
		if err != nil {
			t.Fatalf("raw%s: %v", c.ID, err)
		}
		var wrapper struct {
			DSL    string                    `json:"dslSha256"`
			Config string                    `json:"configSha256"`
			BTB1   string                    `json:"btb1Sha256"`
			Run    engine.AdaptiveFlagResult `json:"run"`
		}
		if err = json.Unmarshal(rawBytes, &wrapper); err != nil {
			t.Fatal(err)
		}
		owned := ownedRaw{wrapper.Run, rawBytes, wrapper.DSL, wrapper.Config, wrapper.BTB1}
		rawSHA := rawBytesSHA(rawBytes)
		rawName := fmt.Sprintf("raw-%03d.json", i)
		if output != "" {
			if err := os.WriteFile(filepath.Join(output, rawName), rawBytes, 0644); err != nil {
				t.Fatal(err)
			}
		}
		for j := 0; j < 3; j++ {
			u := units.Cases[3*i+j]
			if u.RawID != c.ID || rawBytesSHA([]byte(u.Metadata)) != u.SHA {
				t.Fatal("unit case binding")
			}
			var request Request
			if err := json.Unmarshal([]byte(u.Metadata), &request); err != nil {
				t.Fatal(err)
			}
			p, err := projectCore(owned, request)
			if err != nil {
				t.Fatalf("unit%s: %s", u.ID, errorJSON(err))
			}
			if p.Manifest.RawEnvelopeSHA256 != rawSHA || rawBytesSHA(owned.bytes) != rawSHA {
				t.Fatal("raw identity changed")
			}
			encoded, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			encoded = append(encoded, '\n')
			name := fmt.Sprintf("projection-%03d-%d.json", i, j)
			if output != "" {
				if err := os.WriteFile(filepath.Join(output, name), encoded, 0644); err != nil {
					t.Fatal(err)
				}
			}
			rows = append(rows, row{u.ID, rawName, rawSHA, name, rawBytesSHA(encoded), u.SHA, len(p.Orders), len(p.CostEvents), len(p.ClosedTrades), len(p.Marks), len(p.Daily)})
		}
	}
	receipt := struct {
		Scope                       string `json:"scope"`
		RawCorpusSHA, UnitCorpusSHA string
		Build                       BuildIdentity
		Cases                       []row
	}{"internal Go core invented corpus only; no Python oracle or public unit transport", rawBytesSHA(data), rawBytesSHA(unitBytes), buildIdentity(), rows}
	if output != "" {
		encoded, err := json.MarshalIndent(receipt, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(output, "core-corpus-receipt.json"), append(encoded, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("96 owned raw reports / %d fixed-profile Go core projections passed", len(rows))
}
