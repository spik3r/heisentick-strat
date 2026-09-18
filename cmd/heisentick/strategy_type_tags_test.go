package main

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
)

type namedStrategyTagAnySlice []any
type namedStrategyTagMapSlice []map[string]any
type namedStrategyTagConfigSlice []dsl.Config

func TestStrategyTypeTagsCountsSupportedContainersForEveryTag(t *testing.T) {
	containers := []struct {
		name  string
		value any
		want  int
	}{
		{
			name: "parser any slice counts every entry",
			value: []any{
				map[string]any{"scope": "london"},
				nil,
				42,
			},
			want: 3,
		},
		{
			name: "plain map slice counts every entry",
			value: []map[string]any{
				{"scope": "london"},
				nil,
				{"malformed": true},
			},
			want: 3,
		},
		{
			name: "dsl config slice counts every entry",
			value: []dsl.Config{
				{"scope": "london"},
				nil,
				{"malformed": true},
			},
			want: 3,
		},
		{name: "empty any slice", value: []any{}, want: 0},
		{name: "empty plain map slice", value: []map[string]any{}, want: 0},
		{name: "empty dsl config slice", value: []dsl.Config{}, want: 0},
		{name: "nil any slice", value: []any(nil), want: 0},
		{name: "nil plain map slice", value: []map[string]any(nil), want: 0},
		{name: "nil dsl config slice", value: []dsl.Config(nil), want: 0},
	}
	fields := []struct {
		name string
		key  string
		get  func(reportStrategyTypeTags) int
	}{
		{name: "session bias", key: "sessionBiasFilters", get: func(tags reportStrategyTypeTags) int { return tags.SessionBiasFilterCount }},
		{name: "seasonality", key: "seasonalityFilters", get: func(tags reportStrategyTypeTags) int { return tags.SeasonalityFilterCount }},
		{name: "volume profile", key: "vp", get: func(tags reportStrategyTypeTags) int { return tags.VPAFilterCount }},
	}

	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			for _, container := range containers {
				t.Run(container.name, func(t *testing.T) {
					tags := strategyTypeTags(dsl.Config{field.key: container.value})
					if got := field.get(tags); got != container.want {
						t.Fatalf("count = %d, want %d", got, container.want)
					}
					if total := tags.SessionBiasFilterCount + tags.SeasonalityFilterCount + tags.VPAFilterCount; total != container.want {
						t.Fatalf("unrelated tag changed: %#v", tags)
					}
				})
			}
		})
	}
}

func TestStrategyTypeTagsRejectsUnsupportedContainers(t *testing.T) {
	if tags := strategyTypeTags(nil); tags != (reportStrategyTypeTags{}) {
		t.Fatalf("missing containers produced tags: %#v", tags)
	}

	unsupported := []struct {
		name  string
		value any
	}{
		{name: "untyped nil", value: nil},
		{name: "scalar", value: "filter"},
		{name: "map", value: map[string]any{"scope": "london"}},
		{name: "array", value: [1]any{map[string]any{}}},
		{name: "named any slice", value: namedStrategyTagAnySlice{map[string]any{}}},
		{name: "named map slice", value: namedStrategyTagMapSlice{{}}},
		{name: "named config slice", value: namedStrategyTagConfigSlice{{}}},
	}

	for _, tt := range unsupported {
		t.Run(tt.name, func(t *testing.T) {
			tags := strategyTypeTags(dsl.Config{
				"sessionBiasFilters": tt.value,
				"seasonalityFilters": tt.value,
				"vp":                 tt.value,
			})
			if tags != (reportStrategyTypeTags{}) {
				t.Fatalf("unsupported container produced tags: %#v", tags)
			}
		})
	}
}

func TestStrategyTypeTagsPreservesInputsAndCanonicalJSONOrder(t *testing.T) {
	cfg := dsl.Config{
		"sessionBiasFilters": []any{
			map[string]any{"scope": "london", "mode": "up"},
			map[string]any{"scope": "ny", "mode": "down"},
		},
		"seasonalityFilters": []map[string]any{
			{"dimension": "intraday"},
		},
		"vp": []dsl.Config{
			{"scope": "day"},
			{"scope": "week"},
			{"scope": "session"},
		},
	}
	wantConfig := dsl.Config{
		"sessionBiasFilters": []any{
			map[string]any{"scope": "london", "mode": "up"},
			map[string]any{"scope": "ny", "mode": "down"},
		},
		"seasonalityFilters": []map[string]any{
			{"dimension": "intraday"},
		},
		"vp": []dsl.Config{
			{"scope": "day"},
			{"scope": "week"},
			{"scope": "session"},
		},
	}

	tags := strategyTypeTags(cfg)
	if !reflect.DeepEqual(cfg, wantConfig) {
		t.Fatalf("strategyTypeTags mutated config\n got: %#v\nwant: %#v", cfg, wantConfig)
	}
	encoded, err := json.Marshal(tags)
	if err != nil {
		t.Fatalf("marshal strategy tags: %v", err)
	}
	const wantJSON = `{"sessionBiasFilterCount":2,"seasonalityFilterCount":1,"vpaFilterCount":3}`
	if string(encoded) != wantJSON {
		t.Fatalf("strategy tag JSON = %s, want %s", encoded, wantJSON)
	}
}
