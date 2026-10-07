package adaptiveflagunit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/spik3r/heisentick-strat/dsl"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFrozenPreOutcomeInputsAndHandVectors(t *testing.T) {
	root := filepath.Join("..", "..", "testsupport", "testdata")
	read := func(name string) []byte {
		t.Helper()
		b, e := os.ReadFile(filepath.Join(root, name))
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	digest := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	var corpus struct {
		SourceCorpus struct {
			Path  string `json:"path"`
			SHA   string `json:"sha256"`
			Count int    `json:"caseCount"`
		} `json:"sourceCorpus"`
		Cases []struct {
			ID       string `json:"id"`
			RawID    string `json:"rawCaseId"`
			Metadata string `json:"projectionMetadataJSON"`
			SHA      string `json:"projectionMetadataSha256"`
		} `json:"cases"`
		Hand []struct {
			ID       string `json:"id"`
			File     string `json:"file"`
			SHA      string `json:"sha256"`
			Executed bool   `json:"oracleExecuted"`
			Wrapper  struct {
				DSL    string `json:"dslSha256"`
				Config string `json:"configSha256"`
				BTB1   string `json:"btb1Sha256"`
			} `json:"referenceInputWrapper"`
			WrapperSHA         string `json:"referenceInputWrapperSha256"`
			ComparisonDomain   string `json:"comparisonDomain"`
			PublicAdmission    *bool  `json:"publicRuntimeEnvelopeAdmissionClaim"`
			NativeFillEvidence *bool  `json:"nativeStrategyFillEvidence"`
		} `json:"handLedgerInputs"`
		Vectors struct {
			Path string `json:"path"`
			SHA  string `json:"sha256"`
		} `json:"handOperationVectors"`
	}
	if err := json.Unmarshal(read("adaptive-flag-unit-corpus-v1.json"), &corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.SourceCorpus.SHA != "af392754f48672cfe31d7161454dfc35ee73977e4c38f0a72b21c15be6fb2f97" || digest(read(corpus.SourceCorpus.Path)) != corpus.SourceCorpus.SHA {
		t.Fatal("original Stage A corpus changed")
	}
	if corpus.SourceCorpus.Count != 96 || len(corpus.Cases) != 288 || len(corpus.Hand) != 6 {
		t.Fatal("frozen case count changed")
	}
	seen := map[string]bool{}
	for _, c := range corpus.Cases {
		if seen[c.ID] || len(c.Metadata) > MaxProjectionMetadataBytes || digest([]byte(c.Metadata)) != c.SHA {
			t.Fatalf("invalid frozen case%s", c.ID)
		}
		seen[c.ID] = true
		var request Request
		if err := json.Unmarshal([]byte(c.Metadata), &request); err != nil {
			t.Fatal(err)
		}
		if request.Schema != RequestSchema || request.Scenario != "UNIT_POINT_VALUE_1" || request.NumericalPolicy != "BINARY64_ORDERED_V1" {
			t.Fatal("wrong fixture metadata")
		}
		if c.ID != c.RawID+"/"+request.CostPolicy {
			t.Fatal("policy identity mismatch")
		}
	}
	for _, h := range corpus.Hand {

		if h.ComparisonDomain != "internal complete projection mapping; adapter-only invented input" || h.PublicAdmission == nil || *h.PublicAdmission || h.NativeFillEvidence == nil || *h.NativeFillEvidence {
			t.Fatal("hand fixture scope must stay explicitly adapter-only")
		}
		if h.Executed || !strings.HasPrefix(h.File, "adaptive-flag-unit/") || digest(read(h.File)) != h.SHA {
			t.Fatalf("hand fixture identity%s", h.ID)
		}
		if h.Wrapper.DSL != strings.Repeat("5", 64) || h.Wrapper.Config != strings.Repeat("6", 64) || h.Wrapper.BTB1 != strings.Repeat("7", 64) {
			t.Fatal("invented wrapper domains changed")
		}
		tail, err := json.Marshal(h.Wrapper)
		if err != nil {
			t.Fatal(err)
		}
		wrapped := append([]byte(`{"run":`), read(h.File)...)
		wrapped = append(wrapped, ',')
		wrapped = append(wrapped, tail[1:]...)
		wrapped = append(wrapped, '\n')
		if digest(wrapped) != h.WrapperSHA {
			t.Fatal("exact reference wrapper bytes changed")
		}
		var ledger map[string]json.RawMessage
		if err := json.Unmarshal(read(h.File), &ledger); err != nil {
			t.Fatal(err)
		}
		if len(ledger) == 0 {
			t.Fatal("empty invented input")
		}
		var actualConfig dsl.AdaptiveFlagSpec
		if err := json.Unmarshal(ledger["effectiveConfig"], &actualConfig); err != nil {
			t.Fatal(err)
		}
		rules, err := dsl.AdaptiveFlagPreset("INITIAL")
		if err != nil {
			t.Fatal(err)
		}
		expectedConfig := dsl.AdaptiveFlagSpec{Policy: dsl.AdaptiveFlagPolicy, NumericalPolicy: dsl.AdaptiveFlagNumericalPolicy, Timeframe: "M30", Bundle: "INITIAL", Rules: rules}
		if !reflect.DeepEqual(actualConfig, expectedConfig) {
			t.Fatalf("hand context not complete canonical INITIAL: %s", h.ID)
		}
	}
	if digest(read(corpus.Vectors.Path)) != corpus.Vectors.SHA {
		t.Fatal("hand vector hash")
	}
	var vectors struct {
		Vectors []struct {
			ID    string `json:"id"`
			Gross string `json:"grossHex"`
			Net   string `json:"netHex"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(read(corpus.Vectors.Path), &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors.Vectors) != 2 || vectors.Vectors[0].Gross != "0x1.3333333333334p+0" || vectors.Vectors[0].Net != "0x1.2e5604189374cp+0" || vectors.Vectors[1].Gross != "0x1.3333333333333p+0" || vectors.Vectors[1].Net != "0x1.2e5604189374bp+0" {
		t.Fatal("distinct pre-outcome hand vectors changed")
	}
}

func TestFloatTokenShapeRetainsNegativeZero(t *testing.T) {
	value := Mark{MarkedPnLR: math.Copysign(0, -1)}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"marked_pnl_R":-0,`) {
		t.Fatal("Go float negative-zero token lost")
	}
	var tokens map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &tokens); err != nil {
		t.Fatal(err)
	}
	if string(tokens["marked_pnl_R"]) != "-0" {
		t.Fatal("lossless field token lost")
	}
}
