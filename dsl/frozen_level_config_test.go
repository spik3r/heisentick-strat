package dsl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func frozenConfigTestFile(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/frozen_level/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func frozenConfigTestFixture(t *testing.T) Config {
	t.Helper()
	cfg, err := DecodeFrozenLevelConfigJSON(frozenConfigTestFile(t, "config.normalized.json"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func frozenConfigTestSpec(cfg Config) map[string]any {
	return cfg["frozenLevelBreakout"].(map[string]any)
}

func frozenConfigTestIdentity(t *testing.T, cfg Config) FrozenLevelIdentityResult {
	t.Helper()
	identity, err := FrozenLevelIdentity(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func TestFrozenLevelReviewedIdentityVectors(t *testing.T) {
	cfg := frozenConfigTestFixture(t)
	identity := frozenConfigTestIdentity(t, cfg)
	if !bytes.Equal(identity.ConfigBytes, frozenConfigTestFile(t, "config.canonical.json")) {
		t.Fatal("canonical config differs from reviewed literal bytes")
	}
	if !bytes.Equal(identity.PolicyBytes, frozenConfigTestFile(t, "policy.canonical.json")) {
		t.Fatal("policy projection differs from reviewed literal bytes")
	}
	if identity.ConfigDigest != "55c56c3b9d50bdd3a026a77117d36033a276e4bfd15b49c43bd59a1f3194df8e" || len(identity.ConfigBytes) != 4864 {
		t.Fatal("reviewed config digest or size differs")
	}
	if identity.PolicyDigest != "9b670f1a12f6e317234a5b3a0f8c77e4d00d3a9ba46bac618f2ccb3b74b6170d" || len(identity.PolicyBytes) != 4624 {
		t.Fatal("reviewed policy digest or size differs")
	}
	if cfg["dslVersion"] != int64(7) || frozenConfigTestSpec(cfg)["timeoutMinutes"] != int64(60) || frozenConfigTestSpec(cfg)["activationR"] != float64(1) {
		t.Fatal("normalized numeric types differ")
	}
	if strings.HasSuffix(string(identity.ConfigBytes), "\n") || strings.Contains(string(identity.ConfigBytes), "<") || strings.Contains(string(identity.ConfigBytes), "\u2028") {
		t.Fatal("canonical Go escaping or final LF changed")
	}
	if !strings.Contains(string(identity.ConfigBytes), `\u2028`) || !strings.Contains(string(identity.ConfigBytes), `\u2029`) {
		t.Fatal("canonical Unicode separators must be escaped")
	}
	for _, name := range []string{"source", "calendar", "ordering", "schedule", "costs", "assumptions"} {
		payload := frozenConfigTestFile(t, "payloads/"+name+".json")
		digest := sha256.Sum256(payload)
		ref := frozenConfigTestSpec(cfg)["inputRefs"].(map[string]any)[name].(map[string]any)
		if ref["sha256"] != hex.EncodeToString(digest[:]) {
			t.Fatalf("%s payload byte identity differs", name)
		}
		var metadata struct {
			Qualified bool `json:"qualified"`
		}
		if err := json.Unmarshal(payload, &metadata); err != nil {
			t.Fatal(err)
		}
		if metadata.Qualified {
			t.Fatal("synthetic fixture must never claim qualified input")
		}
		withoutLF := sha256.Sum256(bytes.TrimSuffix(payload, []byte("\n")))
		if withoutLF == digest {
			t.Fatalf("%s payload must demonstrate exact trailing-LF identity", name)
		}
	}
}

func TestFrozenLevelIdentityProjectionAndIsolation(t *testing.T) {
	original := frozenConfigTestFixture(t)
	base := frozenConfigTestIdentity(t, original)
	tests := []struct {
		name          string
		policyChanges bool
		change        func(Config)
	}{
		{"name", false, func(c Config) { c["name"] = "Changed name" }},
		{"description", false, func(c Config) { c["description"] = "Changed description" }},
		{"lockMode", true, func(c Config) { frozenConfigTestSpec(c)["lockMode"] = "none" }},
		{"activationR", true, func(c Config) { frozenConfigTestSpec(c)["activationR"] = math.Nextafter(1, 2) }},
		{"priceGrid", true, func(c Config) { frozenConfigTestSpec(c)["priceGrid"] = math.Nextafter(0.01, 1) }},
	}
	for _, key := range []string{"timeoutMinutes", "maxPredecessorAgeMS", "maxReceiptAgeMS", "entryLatencyMS", "amendmentLatencyMS"} {
		key := key
		tests = append(tests, struct {
			name          string
			policyChanges bool
			change        func(Config)
		}{
			key, true, func(c Config) { m := frozenConfigTestSpec(c); m[key] = m[key].(int64) + 1 },
		})
	}
	for _, ref := range []string{"source", "calendar", "ordering", "schedule", "costs", "assumptions"} {
		for _, field := range []string{"id", "version", "sha256"} {
			ref, field := ref, field
			tests = append(tests, struct {
				name          string
				policyChanges bool
				change        func(Config)
			}{
				ref + "." + field, ref != "assumptions", func(c Config) {
					m := frozenConfigTestSpec(c)["inputRefs"].(map[string]any)[ref].(map[string]any)
					if field == "sha256" {
						m[field] = strings.Repeat("a", 64)
					} else {
						m[field] = "Changed"
					}
				},
			})
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := frozenConfigTestFixture(t)
			tt.change(cfg)
			got := frozenConfigTestIdentity(t, cfg)
			if got.ConfigDigest == base.ConfigDigest {
				t.Fatal("changed field must affect config digest")
			}
			if (got.PolicyDigest != base.PolicyDigest) != tt.policyChanges {
				t.Fatal("wrong policy inclusion/exclusion")
			}
		})
	}
	if !reflect.DeepEqual(base, frozenConfigTestIdentity(t, original)) {
		t.Fatal("identity mutated caller maps")
	}
	normalized, err := NormalizeFrozenLevelConfig(original)
	if err != nil {
		t.Fatal(err)
	}
	frozenConfigTestSpec(normalized)["inputRefs"].(map[string]any)["source"].(map[string]any)["id"] = "mutated"
	frozenConfigTestSpec(normalized)["profile"].(map[string]any)["pivot"].(map[string]any)["levels"].([]any)[0] = "mutated"
	if !reflect.DeepEqual(base, frozenConfigTestIdentity(t, original)) {
		t.Fatal("normalization aliases caller maps or slices")
	}
	profile := FrozenLevelProfile()
	profile["pivot"].(map[string]any)["levels"].([]any)[0] = "mutated"
	if FrozenLevelProfile()["pivot"].(map[string]any)["levels"].([]any)[0] != "S4" {
		t.Fatal("profile copies alias")
	}
}

func TestFrozenLevelExactConfigShape(t *testing.T) {
	cfg := frozenConfigTestFixture(t)
	var inspect func(map[string]any, []string)
	inspect = func(m map[string]any, path []string) {
		keys := make([]string, 0, len(m))
		for key := range m {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			for _, mutation := range []string{"missing", "null", "wrong-type"} {
				t.Run(strings.Join(append(path, key, mutation), "/"), func(t *testing.T) {
					candidate := frozenConfigTestFixture(t)
					object := map[string]any(candidate)
					for _, part := range path {
						object = object[part].(map[string]any)
					}
					switch mutation {
					case "missing":
						delete(object, key)
					case "null":
						object[key] = nil
					default:
						object[key] = []any{}
					}
					if _, err := NormalizeFrozenLevelConfig(candidate); err == nil {
						t.Fatal("malformed field accepted")
					}
					identity, err := FrozenLevelIdentity(candidate)
					if err == nil || !reflect.DeepEqual(identity, FrozenLevelIdentityResult{}) {
						t.Fatal("invalid root produced identity data")
					}
				})
			}
			if child, ok := m[key].(map[string]any); ok {
				inspect(child, append(append([]string{}, path...), key))
			}
		}
		t.Run(strings.Join(append(path, "unknown-field"), "/"), func(t *testing.T) {
			candidate := frozenConfigTestFixture(t)
			object := map[string]any(candidate)
			for _, part := range path {
				object = object[part].(map[string]any)
			}
			object["unexpected"] = true
			if _, err := NormalizeFrozenLevelConfig(candidate); err == nil {
				t.Fatal("unknown field accepted")
			}
		})
	}
	inspect(map[string]any(cfg), nil)
}

func TestFrozenLevelEveryFixedProfileValue(t *testing.T) {
	var inspect func(any, []string)
	inspect = func(value any, path []string) {
		if m, ok := value.(map[string]any); ok {
			for key, child := range m {
				inspect(child, append(append([]string{}, path...), key))
			}
			return
		}
		t.Run(strings.Join(path, "/"), func(t *testing.T) {
			cfg := frozenConfigTestFixture(t)
			m := frozenConfigTestSpec(cfg)["profile"].(map[string]any)
			for _, key := range path[:len(path)-1] {
				m = m[key].(map[string]any)
			}
			key := path[len(path)-1]
			switch v := value.(type) {
			case string:
				m[key] = v + "!"
			case bool:
				m[key] = !v
			case int64:
				m[key] = v - 1
			case []any:
				changed := append([]any{}, v...)
				changed[0], changed[1] = changed[1], changed[0]
				m[key] = changed
			default:
				t.Fatalf("uncovered constant type %T", value)
			}
			if _, err := NormalizeFrozenLevelConfig(cfg); err == nil {
				t.Fatal("changed fixed profile accepted")
			}
		})
	}
	inspect(FrozenLevelProfile(), nil)
}

func TestFrozenLevelGoMapLosslessTypesAndMetadata(t *testing.T) {
	base := frozenConfigTestIdentity(t, frozenConfigTestFixture(t))
	for _, version := range []any{int(7), int8(7), int16(7), int32(7), int64(7), uint(7), uint8(7), uint16(7), uint32(7), uint64(7), json.Number("7.00e0")} {
		cfg := frozenConfigTestFixture(t)
		cfg["dslVersion"], cfg["setupType"] = version, FamilyFrozenLevelBreakout
		if got := frozenConfigTestIdentity(t, cfg); !reflect.DeepEqual(got, base) {
			t.Fatalf("lossless alias %T changed identity", version)
		}
	}
	for _, value := range []any{float32(7), float64(7), "7", true, json.Number("7.0000000000000001"), uint64(math.MaxUint64), int64(math.MinInt64), struct{}{}, (*int)(nil)} {
		cfg := frozenConfigTestFixture(t)
		cfg["dslVersion"] = value
		if _, err := NormalizeFrozenLevelConfig(cfg); err == nil {
			t.Fatalf("invalid integer type/value accepted: %T %v", value, value)
		}
	}
	for _, value := range []any{math.NaN(), math.Inf(1), math.Inf(-1), math.Copysign(0, -1), float64(0), float32(1), json.Number("1e-10000"), json.Number("1e309"), uint64(9007199254740992)} {
		cfg := frozenConfigTestFixture(t)
		frozenConfigTestSpec(cfg)["activationR"] = value
		if _, err := NormalizeFrozenLevelConfig(cfg); err == nil {
			t.Fatalf("invalid decimal accepted: %T %v", value, value)
		}
	}
	for _, value := range []any{math.SmallestNonzeroFloat64, math.MaxFloat64, json.Number("5e-324"), int64(1), uint64(9007199254740991)} {
		cfg := frozenConfigTestFixture(t)
		frozenConfigTestSpec(cfg)["activationR"] = value
		if _, err := NormalizeFrozenLevelConfig(cfg); err != nil {
			t.Fatalf("finite positive value rejected: %v: %v", value, err)
		}
	}
	for _, name := range []string{" ", "\x00", "\ufffd", "\U0001d11e", "Name with trailing space "} {
		cfg := frozenConfigTestFixture(t)
		cfg["name"] = name
		normalized, err := NormalizeFrozenLevelConfig(cfg)
		if err != nil || normalized["name"] != name {
			t.Fatalf("valid name was altered or rejected: %q %v", name, err)
		}
	}
	for _, key := range []string{"name", "description"} {
		cfg := frozenConfigTestFixture(t)
		cfg[key] = string([]byte{0xff})
		if _, err := NormalizeFrozenLevelConfig(cfg); err == nil {
			t.Fatal("invalid UTF-8 map metadata accepted")
		}
	}
	for _, name := range []string{"", "-invalid", ".invalid", "a/b", "a:b", "é", "a\n", strings.Repeat("a", 65)} {
		cfg := frozenConfigTestFixture(t)
		frozenConfigTestSpec(cfg)["inputRefs"].(map[string]any)["source"].(map[string]any)["id"] = name
		if _, err := NormalizeFrozenLevelConfig(cfg); err == nil {
			t.Fatalf("invalid reference ID %q accepted", name)
		}
	}
	for _, digest := range []string{strings.Repeat("A", 64), strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("g", 64), strings.Repeat("a", 63) + "\n"} {
		cfg := frozenConfigTestFixture(t)
		frozenConfigTestSpec(cfg)["inputRefs"].(map[string]any)["source"].(map[string]any)["sha256"] = digest
		if _, err := NormalizeFrozenLevelConfig(cfg); err == nil {
			t.Fatalf("invalid SHA accepted: %q", digest)
		}
	}
}
