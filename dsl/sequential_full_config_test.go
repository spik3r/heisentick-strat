package dsl

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
)

func sequentialFullAssertConfigError(t *testing.T, err error) {
	t.Helper()
	var typed *SequentialFullConfigError
	if !errors.As(err, &typed) || typed.Code != "unsupported_config" || typed.Field == "" || typed.Reason == "" {
		t.Fatalf("expected typed config refusal, got %T %v", err, err)
	}
}

func TestSequentialFullDirectConfigClosedFields(t *testing.T) {
	for _, mutate := range []func(Config){
		func(c Config) { c["setupType"] = "legacySetup9" },
		func(c Config) { c["setupType"] = "sequential full" },
		func(c Config) { c["dslVersion"] = 6 },
		func(c Config) { c["dslVersion"] = json.Number("7.00000000000000001") },
		func(c Config) { c["legacySetup9"] = map[string]any{"profile": LegacySetup9Profile} },
		func(c Config) { c["riskUsd"] = 25.0 },
		func(c Config) { c["slices"] = []any{} },
		func(c Config) { c["sourceTimeframe"] = "1m" },
		func(c Config) { c["sequentialFull"] = nil },
		func(c Config) { c["name"] = " " },
		func(c Config) { c["description"] = nil },
		func(c Config) { c["sequentialFull"].(map[string]any)["contractVersion"] = "v2" },
		func(c Config) { c["sequentialFull"].(map[string]any)["profile"] = LegacySetup9Profile },
		func(c Config) { c["sequentialFull"].(map[string]any)["policy"] = "P1" },
		func(c Config) { c["sequentialFull"].(map[string]any)["setupCount"] = 9 },
		func(c Config) { c["sequentialFull"].(map[string]any)["qualifier8Vs5"] = false },
		func(c Config) { c["sequentialFull"].(map[string]any)["riskUsd"] = "25" },
		func(c Config) { c["sequentialFull"].(map[string]any)["riskUsd"] = math.NaN() },
		func(c Config) { c["sequentialFull"].(map[string]any)["maxNotionalUsd"] = math.Inf(1) },
		func(c Config) { c["sequentialFull"].(map[string]any)["maxNotionalUsd"] = 0 },
		func(c Config) { c["sequentialFull"].(map[string]any)["symbol"] = "SYNTH OTHER" },
		func(c Config) { c["sequentialFull"].(map[string]any)["timeframe"] = "1M" },
	} {
		cfg := sequentialFullParseOK(t, sequentialFullTestSource)
		mutate(cfg)
		_, err := DecodeSequentialFullConfig(cfg)
		sequentialFullAssertConfigError(t, err)
	}
	for _, key := range []string{"dslVersion", "name", "description", "setupType", "sequentialFull"} {
		cfg := sequentialFullParseOK(t, sequentialFullTestSource)
		delete(cfg, key)
		_, err := DecodeSequentialFullConfig(cfg)
		sequentialFullAssertConfigError(t, err)
	}
	for _, key := range []string{"contractVersion", "profile", "policy", "symbol", "timeframe", "riskUsd", "maxNotionalUsd"} {
		cfg := sequentialFullParseOK(t, sequentialFullTestSource)
		delete(cfg["sequentialFull"].(map[string]any), key)
		_, err := DecodeSequentialFullConfig(cfg)
		sequentialFullAssertConfigError(t, err)
	}
}

func TestSequentialFullJSONStrictnessAndFreshProjection(t *testing.T) {
	cfg := sequentialFullParseOK(t, sequentialFullTestSource)
	raw, _ := json.Marshal(cfg)
	got, err := DecodeSequentialFullConfigJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := DecodeSequentialFullConfig(cfg)
	actual, err := DecodeSequentialFullConfig(got)
	if err != nil || actual != want {
		t.Fatalf("raw decoder changed policy: %+v %v", actual, err)
	}
	for _, bad := range []string{
		string(raw) + `{}`, `null`, `[]`,
		strings.Replace(string(raw), `"riskUsd":25`, `"riskUsd":25,"riskUsd":30`, 1),
		strings.Replace(string(raw), `"riskUsd":25`, `"riskUsd":25,"risk\u0055sd":30`, 1),
		strings.Replace(string(raw), `"dslVersion":7`, `"dslVersion":7.0000000000000000001`, 1),
		strings.Replace(string(raw), `"riskUsd":25`, `"riskUsd":1e9999`, 1),
		strings.Replace(string(raw), `"riskUsd":25`, `"riskUsd":1e-9999`, 1),
		strings.Replace(string(raw), `"name":"Sequential synthetic E1"`, `"name":"\ud800"`, 1),
	} {
		if bad == string(raw) {
			t.Fatal("bad mutation did not change fixture")
		}
		_, err := DecodeSequentialFullConfigJSON([]byte(bad))
		sequentialFullAssertConfigError(t, err)
	}
	got["sequentialFull"].(map[string]any)["policy"] = "P1"
	again, _ := DecodeSequentialFullConfig(cfg)
	if again != want {
		t.Fatal("decoded projection aliased caller config")
	}
	cfg["setupType"] = FamilySequentialFull
	if _, err := DecodeSequentialFullConfig(cfg); err != nil {
		t.Fatalf("FamilyID adapter refused: %v", err)
	}
}

func TestSequentialFullReservedDirectConfig(t *testing.T) {
	for _, cfg := range []Config{
		{"sequentialFull": nil}, {"SEQUENTIAL_FULL": 0}, {"seq.full.public_approx.v1": false},
		{"setupType": "sequential full"}, {"setupType": FamilySequentialFull},
		{"setupType": "sequential"},
		{"setupType": "flagContinuation", "legacySetup9": map[string]any{"profile": SequentialFullProfileID}},
		{"setupType": "flagContinuation", "unknown": []any{map[string]any{"profile": "SEQ_FULL_PUBLIC_APPROX_V2"}}},
		{"setupType": "flagContinuation", "unknown": map[string]string{"profile": SequentialFullProfileID}},
		{"setupType": "flagContinuation", "unknown": []map[string]string{{"profile": SequentialFullProfileID}}},
		{"setupType": "flagContinuation", "name": map[any]any{"profile": SequentialFullProfileID}},
		{"setupType": "flagContinuation", "description": []string{SequentialFullProfileID}},
	} {
		if !IsSequentialFullReserved(cfg) {
			t.Fatalf("reserved config escaped: %+v", cfg)
		}
		_, err := DecodeSequentialFullConfig(cfg)
		sequentialFullAssertConfigError(t, err)
	}
	for _, cfg := range []Config{nil,
		{"setupType": "legacySetup9", "legacySetup9": map[string]any{"profile": LegacySetup9Profile}},
		{"setupType": "flagContinuation", "name": SequentialFullProfileID, "description": "sequentialFull"},
		{"setupType": "flagContinuation", "symbols": []any{"SEQUENTIALFULL"}},
		{"setupType": "flagContinuation", "slices": []any{map[string]any{"symbol": "SEQUENTIALFULL", "tf": "1m"}}},
	} {
		if IsSequentialFullReserved(cfg) {
			t.Fatalf("non-full config captured: %+v", cfg)
		}
	}
	cycle := Config{}
	cycle["recursive"] = cycle
	if !IsSequentialFullReserved(cycle) {
		t.Fatal("cyclic hand-built config fell through")
	}
}

func TestSequentialFullTimeframeCapability(t *testing.T) {
	for label, minutes := range map[string]int64{"1m": 1, "5m": 5, "15m": 15, "30m": 30, "1h": 60, "4h": 240, "1d": 1440} {
		value, ok := SequentialFullTimeframeMS(label)
		if !ok || value != minutes*60000 {
			t.Fatalf("%s: %d %v", label, value, ok)
		}
	}
	for _, label := range []string{"", "1M", "M1", "01m", "2m", "0m", "-1m", "1w", "1 m", "1m ", "1.5h"} {
		if _, ok := SequentialFullTimeframeMS(label); ok {
			t.Fatalf("unsupported timeframe accepted: %q", label)
		}
	}
}

func TestSequentialFullNativeMapShapesCannotBypassReservation(t *testing.T) {
	type namedKey string
	discriminator := "sequential"
	discriminatorPointer := &discriminator
	var wrappedDiscriminator any = &discriminatorPointer
	cycle := map[any]any{}
	cycle["self"] = cycle
	integerCycle := map[int]any{}
	integerCycle[1] = integerCycle
	pointerCycle := map[any]any{}
	pointerCycle["self"] = &pointerCycle
	for name, payload := range map[string]any{
		"interface-keyed profile": map[any]any{"profile": SequentialFullProfileID},
		"integer-keyed profile":   map[int]string{1: SequentialFullProfileID},
		"interface-keyed field":   map[any]any{"sequentialFull": nil},
		"interface discriminator": map[any]any{"setupType": "sequential"},
		"named discriminator":     map[any]any{namedKey("setupType"): "sequential"},
		"pointer discriminator":   map[string]any{"setupType": &discriminator},
		"wrapped discriminator":   map[any]any{"setupType": &wrappedDiscriminator},
		"distinct native keys":    map[any]any{"profile": "ordinary", namedKey("profile"): SequentialFullProfileID},
		"nested mixed maps":       map[int]any{1: map[any]any{nil: []map[int]string{{2: SequentialFullProfileID}}}},
		"exported struct profile": struct{ Profile string }{SequentialFullProfileID},
		"struct discriminator":    struct{ SetupType string }{"sequential"},
		"inaccessible struct":     struct{ profile string }{SequentialFullProfileID},
		"tagged family field": struct {
			Value any `json:"sequentialFull"`
		}{nil},
		"tagged discriminator": struct {
			Value string `json:"setupType"`
		}{"sequential"},
		"tagged wrapped discriminator": struct {
			Value any `json:"setupType,omitempty"`
		}{&wrappedDiscriminator},
		"interface map cycle": cycle,
		"integer map cycle":   integerCycle,
		"map pointer cycle":   pointerCycle,
	} {
		t.Run(name, func(t *testing.T) {
			cfg := Config{"setupType": "flagContinuation", "extra": payload}
			if !IsSequentialFullReserved(cfg) {
				// Do not format payload: cyclic values must remain safe even on failure.
				t.Fatal("native aggregate bypassed full-profile reservation")
			}
			_, err := DecodeSequentialFullConfig(cfg)
			sequentialFullAssertConfigError(t, err)
		})
	}
	if IsSequentialFullReserved(Config{"setupType": "flagContinuation", "extra": map[any]any{1: "ordinary"}}) {
		t.Fatal("ordinary native map acquired full-profile intent")
	}
}
