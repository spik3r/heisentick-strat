package adaptiveflagunit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original internal hand outputs/errors are retained only when the local test
// evidence directory is explicitly set. This is not a production ledger route.
func TestProjectionFrozenHandOutputs(t *testing.T) {
	root := filepath.Join("..", "..", "testsupport", "testdata")
	data, err := os.ReadFile(filepath.Join(root, "adaptive-flag-unit-corpus-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Hand []struct {
			ID         string `json:"id"`
			File       string `json:"file"`
			SHA        string `json:"sha256"`
			WrapperSHA string `json:"referenceInputWrapperSha256"`
		} `json:"handLedgerInputs"`
	}
	if err = json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Hand) != 6 {
		t.Fatal("hand input count")
	}
	output := os.Getenv("ADAPTIVE_UNIT_TEST_OUTPUT_DIR")
	if output != "" {
		if err := os.MkdirAll(output, 0755); err != nil {
			t.Fatal(err)
		}
	}
	type result struct {
		ID         string `json:"id"`
		InputSHA   string `json:"inputSha256"`
		WrapperSHA string `json:"wrapperSha256"`
		File       string `json:"file"`
		SHA        string `json:"sha256"`
		ErrorCode  string `json:"errorCode"`
	}
	rows := make([]result, 0, 18)
	for i, h := range corpus.Hand {
		body, err := os.ReadFile(filepath.Join(root, h.File))
		if err != nil {
			t.Fatal(err)
		}
		if rawBytesSHA(body) != h.SHA {
			t.Fatal("hand input identity")
		}
		raw := handOwned(t, h.ID)
		if rawBytesSHA(raw.bytes) != h.WrapperSHA {
			t.Fatal("hand wrapper identity")
		}
		for j, policy := range []string{"RAW", "RAZOR_PROXY_BASIC", "RAZOR_PROXY_HARSH_AGGREGATE"} {
			p, err := projectCore(raw, unitRequest(policy))
			var encoded []byte
			code := ""
			if h.ID == "hand-invalid-risk-filled" {
				if p != nil || coreCode(t, err) != "invalid_planned_risk" {
					t.Fatal("expected whole invalid-risk refusal")
				}
				code = "invalid_planned_risk"
				encoded = []byte(errorJSON(err))
			} else {
				if err != nil {
					t.Fatal(err)
				}
				encoded, err = json.Marshal(p)
				if err != nil {
					t.Fatal(err)
				}
			}
			encoded = append(encoded, '\n')
			name := fmt.Sprintf("hand-projection-%02d-%d.json", i, j)
			if output != "" {
				if err := os.WriteFile(filepath.Join(output, name), encoded, 0644); err != nil {
					t.Fatal(err)
				}
			}
			rows = append(rows, result{h.ID + "/" + policy, h.SHA, h.WrapperSHA, name, rawBytesSHA(encoded), code})
		}
	}
	receipt := struct {
		Scope         string        `json:"scope"`
		UnitCorpusSHA string        `json:"unitCorpusSha256"`
		Build         BuildIdentity `json:"buildIdentity"`
		Cases         []result      `json:"cases"`
	}{"internal adapter-only; synthetic-exit and invented hash sentinels; not public runtime/native-fill evidence", rawBytesSHA(data), buildIdentity(), rows}
	if output != "" {
		b, err := json.MarshalIndent(receipt, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(output, "hand-corpus-receipt.json"), append(b, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range rows {
		if strings.Contains(row.ID, "invalid-risk-filled") != (row.ErrorCode != "") {
			t.Fatal("hand refusal inventory")
		}
	}
	t.Log("18 hand calls:15 complete internal projections and3 invalid-risk refusals")
}
