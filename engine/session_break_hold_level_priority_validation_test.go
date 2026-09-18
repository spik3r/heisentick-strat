package engine

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const sessionBreakHoldLevelPriorityValidationError = "engine config levelPriority must be a list"

func TestSessionBreakHoldLevelPriorityRejectsMalformedContainersAcrossPublicBoundaries(t *testing.T) {
	type namedStrings []string
	type namedAny []any
	var nilStringPointer *string
	invalid := []struct {
		name  string
		value any
	}{
		{name: "nil", value: nil},
		{name: "typed nil pointer", value: nilStringPointer},
		{name: "scalar", value: "AH"},
		{name: "map", value: map[string]any{"level": "AH"}},
		{name: "dsl Config", value: dsl.Config{"level": "AH"}},
		{name: "other slice", value: []int{1}},
		{name: "array", value: [1]string{"AH"}},
		{name: "named string slice", value: namedStrings{"AH"}},
		{name: "named any slice", value: namedAny{"AH"}},
	}

	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			request := reviewedSessionBreakHoldValidationRequest(t)
			request.Config["levelPriority"] = tt.value
			for _, flow := range validationPublicFlows() {
				t.Run(flow.name, func(t *testing.T) {
					err := runWithoutPanic(t, func() error { return flow.run(request) })
					if err == nil || err.Error() != sessionBreakHoldLevelPriorityValidationError {
						t.Fatalf("%s error = %v, want %q", flow.name, err, sessionBreakHoldLevelPriorityValidationError)
					}
				})
			}
		})
	}
}

func TestSessionBreakHoldLevelPriorityAcceptsSupportedContainersAcrossPublicBoundaries(t *testing.T) {
	type stringSliceAlias = []string
	type anySliceAlias = []any
	type namedString string
	var nilStrings []string
	var nilAny []any
	accepted := []struct {
		name    string
		present bool
		value   any
	}{
		{name: "absent"},
		{name: "empty string slice", present: true, value: []string{}},
		{name: "empty any slice", present: true, value: []any{}},
		{name: "typed nil string slice", present: true, value: nilStrings},
		{name: "typed nil any slice", present: true, value: nilAny},
		{name: "string slice alias", present: true, value: stringSliceAlias{"AH", "LL"}},
		{name: "any slice alias", present: true, value: anySliceAlias{"AL", "LH"}},
		{name: "unknown empty and duplicate strings", present: true, value: []string{"unknown", "", "AH", "AH"}},
		{name: "mixed any slice", present: true, value: []any{1, "LL", namedString("AH"), nil, "LH", true}},
	}

	for _, tt := range accepted {
		t.Run(tt.name, func(t *testing.T) {
			request := reviewedSessionBreakHoldValidationRequest(t)
			if tt.present {
				request.Config["levelPriority"] = tt.value
			} else {
				delete(request.Config, "levelPriority")
			}
			for _, flow := range validationPublicFlows() {
				t.Run(flow.name, func(t *testing.T) {
					if err := runWithoutPanic(t, func() error { return flow.run(request) }); err != nil {
						t.Fatalf("%s rejected accepted container: %v", flow.name, err)
					}
				})
			}
		})
	}
}

func TestSessionBreakHoldLevelPriorityMixedFilteringOrderAndOwnership(t *testing.T) {
	type namedString string
	priority := []any{1, "LL", namedString("AH"), "AH", "unknown", nil, "LH", "LL", true}
	wantSource := append([]any(nil), priority...)
	params := paramsFromConfig(dsl.Config{
		"setupType":     string(dsl.FamilySessionBreakHold),
		"levelPriority": priority,
	})
	if got, want := params.LevelPriority, []string{"LL", "AH", "unknown", "LH", "LL"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded priority = %v, want %v", got, want)
	}
	if got, want := params.SBHAllowedLevels, []string{"LL", "AH", "LH", "LL"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("allowed levels = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(priority, wantSource) {
		t.Fatalf("caller priority mutated: got %v want %v", priority, wantSource)
	}
	params.SBHAllowedLevels[0] = "changed"
	if priority[1] != "LL" || params.LevelPriority[0] != "LL" {
		t.Fatalf("allowed-level mutation leaked: source=%v decoded=%v", priority, params.LevelPriority)
	}
}

func TestSessionBreakHoldLevelPrioritySupportsNamedFamilyAndIgnoresInactiveFamilies(t *testing.T) {
	for _, setupType := range []any{string(dsl.FamilySessionBreakHold), dsl.FamilySessionBreakHold} {
		cfg := dsl.Config{"setupType": setupType, "levelPriority": "AH"}
		if err := validateSessionBreakHoldLevelPriority(cfg); err == nil || err.Error() != sessionBreakHoldLevelPriorityValidationError {
			t.Fatalf("setup type %T error = %v, want %q", setupType, err, sessionBreakHoldLevelPriorityValidationError)
		}
	}

	for _, family := range []dsl.FamilyID{
		dsl.FamilyFlagContinuation,
		dsl.FamilyDayOpenReclaim,
		dsl.FamilyBreakRetest,
		dsl.FamilyFailedBreakout,
		dsl.FamilyVolumeAnomalyExhaustion,
	} {
		cfg := dsl.Config{"setupType": family, "levelPriority": "malformed"}
		if err := validateSessionBreakHoldLevelPriority(cfg); err != nil {
			t.Fatalf("inactive family %q rejected: %v", family, err)
		}
	}

	request := reviewedSessionBreakHoldValidationRequest(t)
	request.Config["setupType"] = dsl.FamilySessionBreakHold
	request.Config["levelPriority"] = "AH"
	for _, flow := range validationPublicFlows() {
		t.Run(flow.name, func(t *testing.T) {
			err := runWithoutPanic(t, func() error { return flow.run(request) })
			if err == nil || err.Error() != sessionBreakHoldLevelPriorityValidationError {
				t.Fatalf("%s named-family error = %v, want %q", flow.name, err, sessionBreakHoldLevelPriorityValidationError)
			}
		})
	}
}

func TestSessionBreakHoldLevelPriorityValidationPrecedence(t *testing.T) {
	emptyMarket := reviewedSessionBreakHoldValidationRequest(t)
	emptyMarket.Config["levelPriority"] = "AH"
	emptyMarket.Series = marketdata.Series{}
	if _, err := SharedContextKey(emptyMarket); err == nil || err.Error() != "market series is empty" {
		t.Fatalf("empty-market error = %v, want market precedence", err)
	}

	unsupported := reviewedSessionBreakHoldValidationRequest(t)
	unsupported.Config["setupType"] = "unsupported"
	unsupported.Config["levelPriority"] = "AH"
	if _, err := SharedContextKey(unsupported); err == nil || err.Error() != `setup family "unsupported" is not implemented` {
		t.Fatalf("unsupported-family error = %v, want family precedence", err)
	}

	sharedAdmission := reviewedSessionBreakHoldValidationRequest(t)
	sharedAdmission.Config["sessionPhases"] = "open"
	sharedAdmission.Config["levelPriority"] = "AH"
	if _, err := SharedContextKey(sharedAdmission); err == nil || err.Error() != "engine config sessionPhases must be a string list" {
		t.Fatalf("shared-admission error = %v, want C55 precedence", err)
	}

	malformedSeries := reviewedSessionBreakHoldValidationRequest(t)
	malformedSeries.Config["levelPriority"] = "AH"
	malformedSeries.Series.O = malformedSeries.Series.O[:1]
	if _, err := SharedContextKey(malformedSeries); err == nil || err.Error() != sessionBreakHoldLevelPriorityValidationError {
		t.Fatalf("malformed-series error = %v, want level-priority precedence", err)
	}
}

func TestSessionBreakHoldLevelPriorityReviewedConfigAndLaterVariant(t *testing.T) {
	request := reviewedSessionBreakHoldValidationRequest(t)
	if got, want := stringSliceValue(request.Config["levelPriority"]), []string{"AH", "AL", "LH", "LL"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("reviewed priority = %v, want %v", got, want)
	}
	for _, flow := range validationPublicFlows() {
		t.Run(flow.name, func(t *testing.T) {
			if err := runWithoutPanic(t, func() error { return flow.run(request) }); err != nil {
				t.Fatalf("%s rejected reviewed config: %v", flow.name, err)
			}
		})
	}

	shared, err := PrepareSharedRunContext(request)
	if err != nil {
		t.Fatalf("prepare reviewed shared context: %v", err)
	}
	malformed := cloneTestConfig(t, request.Config)
	malformed["levelPriority"] = "AH"
	if _, err := shared.PrepareVariant(malformed); err == nil || err.Error() != sessionBreakHoldLevelPriorityValidationError {
		t.Fatalf("later variant error = %v, want %q", err, sessionBreakHoldLevelPriorityValidationError)
	}
	valid := cloneTestConfig(t, request.Config)
	valid["levelPriority"] = []any{1, "AH", nil, "LL"}
	if _, err := shared.PrepareVariant(valid); err != nil {
		t.Fatalf("later valid mixed variant rejected: %v", err)
	}
}

func reviewedSessionBreakHoldValidationRequest(t *testing.T) RunRequest {
	t.Helper()
	sourcePath := filepath.Join(filepath.Dir(runFixtureDir()), "parse", "setup-session-break-hold.strat")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read reviewed Session Break Hold source: %v", err)
	}
	parsed, err := dsl.Parse(string(source))
	if err != nil {
		t.Fatalf("parse reviewed Session Break Hold source: %v", err)
	}
	if len(parsed.Errors) != 0 {
		t.Fatalf("reviewed Session Break Hold diagnostics: %v", parsed.Errors)
	}
	return RunRequest{
		Config:    parsed.Config,
		Series:    validationSeries(),
		HTFSeries: marketdata.Series{},
	}
}
