package main

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
)

func TestActiveSessionsAcceptsEngineBooleanAndNumericForms(t *testing.T) {
	onValues := []struct {
		name  string
		value any
	}{
		{name: "bool true", value: true},
		{name: "int positive", value: int(1)},
		{name: "int negative", value: int(-1)},
		{name: "int64 positive", value: int64(1)},
		{name: "int64 negative", value: int64(-1)},
		{name: "float64 positive", value: float64(0.5)},
		{name: "float64 negative", value: float64(-0.5)},
		{name: "float64 NaN", value: math.NaN()},
		{name: "float64 positive infinity", value: math.Inf(1)},
		{name: "float64 negative infinity", value: math.Inf(-1)},
	}
	containers := []struct {
		name string
		wrap func(map[string]any) any
	}{
		{name: "map string any", wrap: func(values map[string]any) any { return values }},
		{name: "dsl config", wrap: func(values map[string]any) any { return dsl.Config(values) }},
	}

	for _, container := range containers {
		t.Run(container.name, func(t *testing.T) {
			for _, tt := range onValues {
				t.Run(tt.name, func(t *testing.T) {
					sessions := map[string]any{
						"ny":     tt.value,
						"mid":    tt.value,
						"asia":   tt.value,
						"london": tt.value,
					}
					got := activeSessions(dsl.Config{"sessions": container.wrap(sessions)})
					want := []string{"asia", "london", "ny"}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("activeSessions(%T values %T(%v)) = %#v, want %#v", container.wrap(sessions), tt.value, tt.value, got, want)
					}
				})
			}
		})
	}
}

func TestActiveSessionsKeepsZeroFalseAndUnsupportedValuesOff(t *testing.T) {
	type namedBool bool
	type namedInt int
	type namedInt64 int64
	type namedFloat64 float64
	type namedSessions map[string]any
	offValues := []struct {
		name  string
		value any
	}{
		{name: "bool false", value: false},
		{name: "int zero", value: int(0)},
		{name: "int64 zero", value: int64(0)},
		{name: "float64 zero", value: float64(0)},
		{name: "nil", value: nil},
		{name: "string", value: "1"},
		{name: "float32", value: float32(1)},
		{name: "uint", value: uint(1)},
		{name: "map", value: map[string]any{"enabled": true}},
		{name: "slice", value: []any{1}},
		{name: "named bool", value: namedBool(true)},
		{name: "named int", value: namedInt(1)},
		{name: "named int64", value: namedInt64(1)},
		{name: "named float64", value: namedFloat64(1)},
	}

	for _, tt := range offValues {
		t.Run(tt.name, func(t *testing.T) {
			got := activeSessions(dsl.Config{"sessions": map[string]any{
				"asia":   tt.value,
				"london": tt.value,
				"ny":     tt.value,
				"mid":    true,
			}})
			if got == nil || len(got) != 0 {
				t.Fatalf("activeSessions(%T(%v)) = %#v, want nonnil empty slice", tt.value, tt.value, got)
			}
		})
	}

	for _, tt := range []struct {
		name     string
		sessions any
	}{
		{name: "missing", sessions: nil},
		{name: "nil container", sessions: nil},
		{name: "map string int remains unsupported", sessions: map[string]int{"asia": 1, "london": 1, "ny": 1}},
		{name: "named map remains unsupported", sessions: namedSessions{"asia": true, "london": true, "ny": true}},
		{name: "scalar remains unsupported", sessions: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := dsl.Config{}
			if tt.name != "missing" {
				cfg["sessions"] = tt.sessions
			}
			got := activeSessions(cfg)
			if got == nil || len(got) != 0 {
				t.Fatalf("activeSessions(%T) = %#v, want nonnil empty slice", tt.sessions, got)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal active sessions: %v", err)
			}
			if string(encoded) != "[]" {
				t.Fatalf("activeSessions JSON = %s, want []", encoded)
			}
		})
	}
}

func TestActiveSessionsDoesNotMutateSupportedContainers(t *testing.T) {
	plain := map[string]any{"ny": true, "mid": true, "asia": int64(-1), "london": float64(0.5)}
	named := dsl.Config{"ny": true, "mid": true, "asia": int64(-1), "london": float64(0.5)}
	plainBefore := map[string]any{"ny": true, "mid": true, "asia": int64(-1), "london": float64(0.5)}
	namedBefore := dsl.Config{"ny": true, "mid": true, "asia": int64(-1), "london": float64(0.5)}

	for name, sessions := range map[string]any{
		"map string any": plain,
		"dsl config":     named,
	} {
		t.Run(name, func(t *testing.T) {
			if got, want := activeSessions(dsl.Config{"sessions": sessions}), []string{"asia", "london", "ny"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("activeSessions() = %#v, want %#v", got, want)
			}
		})
	}

	if !reflect.DeepEqual(plain, plainBefore) {
		t.Fatalf("plain session map mutated: got %#v want %#v", plain, plainBefore)
	}
	if !reflect.DeepEqual(named, namedBefore) {
		t.Fatalf("dsl.Config session map mutated: got %#v want %#v", named, namedBefore)
	}

	first := activeSessions(dsl.Config{"sessions": plain})
	second := activeSessions(dsl.Config{"sessions": plain})
	first[0] = "changed"
	if got, want := second, []string{"asia", "london", "ny"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("returned session slices share storage: got %#v want %#v", got, want)
	}
	if !reflect.DeepEqual(plain, plainBefore) {
		t.Fatalf("returned slice mutation changed source: got %#v want %#v", plain, plainBefore)
	}
}
