package engine

import (
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestSharedAdmissionListValidationRejectsMalformedContainersAcrossPublicBoundaries(t *testing.T) {
	type namedStrings []string
	type namedString string
	var nilStringPointer *string
	invalid := []struct {
		name  string
		value any
	}{
		{name: "nil", value: nil},
		{name: "typed nil pointer", value: nilStringPointer},
		{name: "scalar", value: "open"},
		{name: "map", value: map[string]any{"phase": "open"}},
		{name: "other slice", value: []int{1}},
		{name: "array", value: [1]string{"open"}},
		{name: "named slice", value: namedStrings{"open"}},
		{name: "mixed any slice", value: []any{"open", 1}},
		{name: "nonstring any slice", value: []any{1, true}},
		{name: "nil any element", value: []any{nil}},
		{name: "named string element", value: []any{namedString("open")}},
	}

	for _, field := range sharedAdmissionListFields {
		for _, tt := range invalid {
			t.Run(field+"/"+tt.name, func(t *testing.T) {
				request := admissionListValidationRequest()
				request.Config[field] = tt.value
				want := "engine config " + field + " must be a string list"
				for _, flow := range validationPublicFlows() {
					t.Run(flow.name, func(t *testing.T) {
						err := runWithoutPanic(t, func() error { return flow.run(request) })
						if err == nil || err.Error() != want {
							t.Fatalf("%s error = %v, want %q", flow.name, err, want)
						}
					})
				}
			})
		}
	}
}

func TestSharedAdmissionListValidationAcceptsSupportedContainersAcrossPublicBoundaries(t *testing.T) {
	type stringSliceAlias = []string
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
		{name: "string slice alias", present: true, value: stringSliceAlias{"open"}},
		{name: "string slice contents are shape only", present: true, value: []string{"unknown", "", "unknown"}},
		{name: "any slice contents are shape only", present: true, value: []any{"unknown", "", "unknown"}},
	}

	for _, field := range sharedAdmissionListFields {
		for _, tt := range accepted {
			t.Run(field+"/"+tt.name, func(t *testing.T) {
				request := admissionListValidationRequest()
				if tt.present {
					request.Config[field] = tt.value
				}
				for _, flow := range validationPublicFlows() {
					t.Run(flow.name, func(t *testing.T) {
						if err := runWithoutPanic(t, func() error { return flow.run(request) }); err != nil {
							t.Fatalf("%s rejected accepted %s container: %v", flow.name, field, err)
						}
					})
				}
			})
		}
	}
}

func TestSharedAdmissionListValidationSupportsNamedFamilyAndDoesNotMutateContainers(t *testing.T) {
	values := []any{"open", "close"}
	wantValues := append([]any(nil), values...)
	request := admissionListValidationRequest()
	request.Config["setupType"] = dsl.FamilyRangeBreakFake
	request.Config["sessionPhases"] = values

	for _, flow := range validationPublicFlows() {
		t.Run(flow.name, func(t *testing.T) {
			if err := runWithoutPanic(t, func() error { return flow.run(request) }); err != nil {
				t.Fatalf("%s rejected named family with valid list: %v", flow.name, err)
			}
			if !reflect.DeepEqual(values, wantValues) {
				t.Fatalf("%s mutated caller list: got %v want %v", flow.name, values, wantValues)
			}
		})
	}
	shared, err := PrepareSharedRunContext(request)
	if err != nil {
		t.Fatalf("prepare named-family shared context: %v", err)
	}
	if _, err := shared.PrepareVariant(request.Config); err != nil {
		t.Fatalf("PrepareVariant rejected named family with valid list: %v", err)
	}

	request.Config["sessionPhases"] = "malformed"
	wantErr := "engine config sessionPhases must be a string list"
	for _, flow := range validationPublicFlows() {
		t.Run(flow.name+" rejects malformed", func(t *testing.T) {
			if err := runWithoutPanic(t, func() error { return flow.run(request) }); err == nil || err.Error() != wantErr {
				t.Fatalf("%s error = %v, want %q", flow.name, err, wantErr)
			}
		})
	}
	if _, err := shared.PrepareVariant(request.Config); err == nil || err.Error() != wantErr {
		t.Fatalf("PrepareVariant named-family error = %v, want %q", err, wantErr)
	}
}

func TestSharedAdmissionListValidationAppliesToLaterSharedVariants(t *testing.T) {
	request := admissionListValidationRequest()
	shared, err := PrepareSharedRunContext(request)
	if err != nil {
		t.Fatalf("prepare shared context: %v", err)
	}

	for _, field := range sharedAdmissionListFields {
		t.Run(field+" rejects malformed", func(t *testing.T) {
			cfg := dsl.Config{"setupType": string(dsl.FamilyRangeBreakFake), field: "malformed"}
			want := "engine config " + field + " must be a string list"
			if _, err := shared.PrepareVariant(cfg); err == nil || err.Error() != want {
				t.Fatalf("PrepareVariant error = %v, want %q", err, want)
			}
		})

		t.Run(field+" accepts string list", func(t *testing.T) {
			cfg := dsl.Config{"setupType": string(dsl.FamilyRangeBreakFake), field: []any{"unknown", "", "unknown"}}
			if _, err := shared.PrepareVariant(cfg); err != nil {
				t.Fatalf("PrepareVariant rejected accepted %s container: %v", field, err)
			}
		})
	}
}

func TestSharedAdmissionListValidationUsesDeterministicFieldOrder(t *testing.T) {
	for first := range sharedAdmissionListFields {
		request := admissionListValidationRequest()
		for _, field := range sharedAdmissionListFields[first:] {
			request.Config[field] = "malformed"
		}
		want := "engine config " + sharedAdmissionListFields[first] + " must be a string list"
		if _, err := SharedContextKey(request); err == nil || err.Error() != want {
			t.Fatalf("first invalid field %d error = %v, want %q", first, err, want)
		}
	}
}

func TestSharedAdmissionListValidationKeepsExistingPrecedence(t *testing.T) {
	if _, err := SharedContextKey(RunRequest{}); err == nil || err.Error() != "engine config is required" {
		t.Fatalf("nil config error = %v, want engine config precedence", err)
	}

	emptyMarket := admissionListValidationRequest()
	emptyMarket.Config["sessionPhases"] = "malformed"
	emptyMarket.Series = marketdata.Series{}
	if _, err := SharedContextKey(emptyMarket); err == nil || err.Error() != "market series is empty" {
		t.Fatalf("empty market error = %v, want market precedence", err)
	}

	unsupported := admissionListValidationRequest()
	unsupported.Config["setupType"] = "unsupported"
	unsupported.Config["sessionPhases"] = "malformed"
	if _, err := SharedContextKey(unsupported); err == nil || err.Error() != `setup family "unsupported" is not implemented` {
		t.Fatalf("unsupported family error = %v, want family precedence", err)
	}

	shared, err := PrepareSharedRunContext(admissionListValidationRequest())
	if err != nil {
		t.Fatalf("prepare shared context: %v", err)
	}
	if _, err := shared.PrepareVariant(nil); err == nil || err.Error() != "engine config is required" {
		t.Fatalf("nil variant error = %v, want engine config precedence", err)
	}
	if _, err := shared.PrepareVariant(unsupported.Config); err == nil || err.Error() != `setup family "unsupported" is not implemented` {
		t.Fatalf("unsupported variant error = %v, want family precedence", err)
	}
}

func TestSharedAdmissionListValidationKeepsC54Precedence(t *testing.T) {
	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-double-top-bottom")
	cfg := cloneTestConfig(t, reviewedConfig)
	mapValue(cfg, "doubleTopBottom")["pivotWindow"] = float64(-1)
	cfg["sessionPhases"] = "malformed"
	request := doubleTopPivotRequest(fixture, cfg)
	if _, err := SharedContextKey(request); err == nil || err.Error() != doubleTopPivotValidationError {
		t.Fatalf("standard boundary error = %v, want C54 precedence %q", err, doubleTopPivotValidationError)
	}

	shared, err := PrepareSharedRunContext(doubleTopPivotRequest(fixture, reviewedConfig))
	if err != nil {
		t.Fatalf("prepare reviewed shared context: %v", err)
	}
	if _, err := shared.PrepareVariant(cfg); err == nil || err.Error() != doubleTopPivotValidationError {
		t.Fatalf("shared variant error = %v, want C54 precedence %q", err, doubleTopPivotValidationError)
	}
}

func TestSharedAdmissionListValidationDoesNotExpandToOtherLists(t *testing.T) {
	request := admissionListValidationRequest()
	request.Config["triggerCandles"] = "malformed"
	request.Config["levelPriority"] = "malformed"
	request.Config["vwapExtensionFade"] = dsl.Config{"sessionPhases": "malformed"}
	request.Config["supplyDemand"] = dsl.Config{"patterns": "malformed"}

	for _, flow := range validationPublicFlows() {
		t.Run(flow.name, func(t *testing.T) {
			if err := runWithoutPanic(t, func() error { return flow.run(request) }); err != nil {
				t.Fatalf("%s rejected excluded list consumer: %v", flow.name, err)
			}
		})
	}
}

func admissionListValidationRequest() RunRequest {
	return validationRequest(validationSeries(), marketdata.Series{})
}
