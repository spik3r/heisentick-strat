package adaptiveflagunit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// These are serialization tests over fixed reviewed constants, not a cost
// calculator, economics projector, strategy invocation or Python oracle run.
func TestFrozenPolicyIdentityBytesAndHashes(t *testing.T) {
	root := filepath.Join("..", "..", "testsupport", "testdata")
	data, err := os.ReadFile(filepath.Join(root, "adaptive-flag-unit", "policy-identity-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Policy struct {
			Path string `json:"path"`
			SHA  string `json:"sha256"`
		} `json:"policyIdentityVectors"`
	}
	rawManifest, err := os.ReadFile(filepath.Join(root, "adaptive-flag-unit-corpus-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(rawManifest, &manifest); err != nil {
		t.Fatal(err)
	}
	digest := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	if manifest.Policy.Path != "adaptive-flag-unit/policy-identity-vectors.json" || digest(data) != manifest.Policy.SHA {
		t.Fatal("frozen policy vector file identity changed")
	}
	var fixture struct {
		Vectors []struct {
			ID    string `json:"id"`
			JSON  string `json:"compactJSON"`
			Bytes int    `json:"utf8Bytes"`
			SHA   string `json:"sha256"`
		} `json:"vectors"`
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Vectors) != 4 {
		t.Fatal("expected exactly three economic and one numerical vector")
	}
	zero, spread, commission, aggregate := 0.0, 0.06, 0.035, 0.155
	values := map[string]any{}
	policies := []CostPolicy{{"RAW", 0, &zero, &zero, nil}, {"RAZOR_PROXY_BASIC", 0.095, &spread, &commission, nil}, {"RAZOR_PROXY_HARSH_AGGREGATE", 0.155, nil, nil, &aggregate}}
	for _, policy := range policies {
		values["economic-"+policy.Name] = EconomicPolicy{"UNIT_POINT_VALUE_1-go-v1", "UNIT_POINT_VALUE_1", 1, 1, policy, "unknown_unmodeled"}
	}
	values["numerical-BINARY64_ORDERED_V1"] = NumericalPolicy{"adaptive-unit-numerical-policy-go-v1", "BINARY64_ORDERED_V1", "chronological positive-zero", "separate binary64", "direct"}
	seen := map[string]bool{}
	for _, vector := range fixture.Vectors {
		t.Run(vector.ID, func(t *testing.T) {
			value, ok := values[vector.ID]
			if !ok || seen[vector.ID] {
				t.Fatal("unexpected/duplicate vector")
			}
			seen[vector.ID] = true
			if len(vector.JSON) != vector.Bytes || digest([]byte(vector.JSON)) != vector.SHA {
				t.Fatal("literal vector bytes/hash disagree")
			}
			actual, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if string(actual) != vector.JSON {
				t.Fatalf("serializer order/number tokens differ:\nactual %s\nfrozen %s", actual, vector.JSON)
			}
			if digest(actual) != vector.SHA {
				t.Fatal("Go policy hash differs")
			}
		})
	}
}
