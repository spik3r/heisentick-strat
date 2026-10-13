package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
)

func TestAdaptiveFlagResultGuardPrecedesSerialization(t *testing.T) {
	// This owned result cannot be JSON encoded. The sentinel must win before
	// encoding is attempted; a nil/accepting guard retains the original error.
	invalid := AdaptiveFlagResult{Snapshots: []AdaptiveFlagSnapshot{{TrueRange: math.NaN()}}}
	if _, err := json.Marshal(invalid); err == nil {
		t.Fatal("invalid serializer control")
	}
	refusal := errors.New("invented guard refusal")
	calls := 0
	if err := adaptiveFlagValidateResultJSON(invalid, func(result AdaptiveFlagResult) error {
		calls++
		if len(result.Snapshots) != 1 {
			t.Fatal("incomplete result")
		}
		return refusal
	}); err != refusal || calls != 1 {
		t.Fatal("guard ordering", calls, err)
	}
	_, marshalErr := json.Marshal(invalid)
	expected := fmt.Errorf("adaptive volume flag nonfinite raw result: %w", marshalErr).Error()
	for _, guard := range []func(AdaptiveFlagResult) error{nil, func(AdaptiveFlagResult) error { return nil }} {
		err := adaptiveFlagValidateResultJSON(invalid, guard)
		if err == nil || err.Error() != expected {
			t.Fatal("serialization diagnostic changed", err)
		}
	}
}

func TestAdaptiveFlagResultGuardSameRunnerCompleteOnceOrZero(t *testing.T) {
	for _, bundle := range []string{"INITIAL", "TWEAKED", "SNAPSHOT_C"} {
		rules, err := dsl.AdaptiveFlagPreset(bundle)
		if err != nil {
			t.Fatal(err)
		}
		request := AdaptiveFlagRequest{Config: dsl.AdaptiveFlagConfig("Invented guard control", "Synthetic only", "M30", bundle, rules), Series: adaptiveTestSeries(adaptiveTestBreakout())}
		want, err := RunAdaptiveVolumeFlag(request)
		if err != nil {
			t.Fatal(err)
		}
		oldBytes, _ := json.Marshal(want)
		for _, active := range []bool{false, true} {
			calls := 0
			var guard func(AdaptiveFlagResult) error
			if active {
				guard = func(result AdaptiveFlagResult) error {
					calls++
					b, _ := json.Marshal(result)
					if !bytes.Equal(b, oldBytes) || len(result.Orders) == 0 || len(result.Events) == 0 || len(result.States) != len(result.Snapshots) {
						t.Fatal("guard did not see complete final result")
					}
					return nil
				}
			}
			got, err := RunAdaptiveVolumeFlagWithResultGuard(request, guard)
			if err != nil {
				t.Fatal(err)
			}
			gotBytes, _ := json.Marshal(got)
			if !bytes.Equal(gotBytes, oldBytes) || (active && calls != 1) || (!active && calls != 0) {
				t.Fatal("guard altered result or invocation count", calls)
			}
		}
		refusal := errors.New("invented resource refusal")
		calls := 0
		got, err := RunAdaptiveVolumeFlagWithResultGuard(request, func(AdaptiveFlagResult) error { calls++; return refusal })
		if err != refusal || calls != 1 || !reflect.DeepEqual(got, AdaptiveFlagResult{}) {
			t.Fatal("partial result or wrapped guard failure", calls, err)
		}
		request.Window = &AdaptiveFlagExecutionWindow{TradeFromMS: 1, TradeToMS: 2}
		original, originalErr := RunAdaptiveVolumeFlag(request)
		calls = 0
		refused, guardedErr := RunAdaptiveVolumeFlagWithResultGuard(request, func(AdaptiveFlagResult) error { calls++; return nil })
		if originalErr == nil || guardedErr == nil || originalErr.Error() != guardedErr.Error() || calls != 0 || !reflect.DeepEqual(original, refused) {
			t.Fatal("early error changed or guard ran before complete result")
		}
	}
}
