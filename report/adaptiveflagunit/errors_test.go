package adaptiveflagunit

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
)

func decodeCoreError(t *testing.T, err error) ErrorEnvelope {
	t.Helper()
	wire := errorJSON(err)
	if len(wire)+1 > MaxErrorBytes || !utf8.ValidString(wire) || strings.Contains(wire, "\n") {
		t.Fatalf("error wire is outside the compact bounded contract: %d bytes", len(wire))
	}
	var result ErrorEnvelope
	if err := json.Unmarshal([]byte(wire), &result); err != nil {
		t.Fatal(err)
	}
	if result.Schema != ErrorSchema || result.Error.Phase != errorPhase(result.Error.Code) || len(result.Error.Message) > maxErrorMessageBytes {
		t.Fatalf("invalid closed error: %+v", result)
	}
	return result
}

func TestCoreErrorWireAndGetters(t *testing.T) {
	err := newCoreError("nonfinite_projection_arithmetic", "gross", 0, 1023, 4095, "nonfinite scalar")
	core := err.(*coreError)
	if core.Error() != "nonfinite_projection_arithmetic" || core.Code() != core.Error() || core.Phase() != "projection" || core.Message() != "nonfinite scalar" {
		t.Fatal("stable error getters changed")
	}
	want := `{"schema":"adaptive-flag-unit-error-v1","error":{"code":"nonfinite_projection_arithmetic","phase":"projection","message":"nonfinite scalar","location":{"operation":"gross","rowIndex":0,"orderId":1023,"eventId":4095}}}`
	if got := errorJSON(fmt.Errorf("private wrapper: %w", err)); got != want {
		t.Fatalf("closed key order/locations changed:\n%s", got)
	}
	loc := core.Location()
	*loc.Operation, *loc.RowIndex, *loc.OrderID, *loc.EventID = "private", 99, 99, 99
	if errorJSON(err) != want {
		t.Fatal("a returned location aliases the core error")
	}
	for _, indices := range [][3]int{{-1, -1, -1}, {-2, 1024, 4096}, {1024, -4, -8}} {
		result := decodeCoreError(t, newCoreError("invalid_native_ledger", "", indices[0], indices[1], indices[2], "bad location"))
		if result.Error.Location != (ErrorLocation{}) {
			t.Fatal("out-of-domain locations must be null, never clamped")
		}
	}
}

func TestCoreErrorUTF8AndWireBounds(t *testing.T) {
	for _, message := range []string{
		strings.Repeat("a", 1025), strings.Repeat("🙂", 400), strings.Repeat("a", 1023) + "é",
		"a\xff\xfe" + strings.Repeat("🙂", 400), strings.Repeat("\xff", 9000) + "z",
		strings.Repeat("\x00", 1024), strings.Repeat("<&>\n\t\"\\", 1024),
		strings.Repeat("a", 1022) + "\xff" + "z", "valid\uFFFDreplacement",
	} {
		want := strings.ToValidUTF8(message, "\uFFFD")
		if len(want) > 1024 {
			want = want[:1024]
			for !utf8.ValidString(want) {
				want = want[:len(want)-1]
			}
		}
		result := decodeCoreError(t, newCoreError("projection_request_rejected", "projection_request", -1, -1, -1, message))
		if result.Error.Message != want {
			t.Fatalf("replacement/truncation differs: got %q, want %q", result.Error.Message, want)
		}
	}
}

func TestCoreErrorInheritedStageAPhases(t *testing.T) {
	for phase, code := range map[string]string{
		"request": "raw_request_rejected", "source": "raw_source_rejected", "input": "raw_input_rejected",
		"resource": "resource_limit", "execution": "raw_execution_rejected",
	} {
		t.Run(phase, func(t *testing.T) {
			err := adaptiveflag.Rejection(phase, strings.Repeat("🙂", 400))
			before := adaptiveflag.ErrorJSON(err)
			result := decodeCoreError(t, fmt.Errorf("owned raw preparation: %w", err))
			if result.Error.Code != code || result.Error.Phase != phase || result.Error.Location != (ErrorLocation{}) || len(result.Error.Message) > 1024 {
				t.Fatalf("wrong inherited error: %+v", result)
			}
			if adaptiveflag.ErrorJSON(err) != before {
				t.Fatal("Stage A error bytes changed")
			}
		})
	}
	result := decodeCoreError(t, errors.New("source input resource words do not select a phase"))
	if result.Error.Code != "raw_execution_rejected" || result.Error.Phase != "execution" {
		t.Fatal("message content was used to infer a phase")
	}
}

func TestCoreErrorRejectsMalformedInheritedSerialization(t *testing.T) {
	valid := `{"error":{"code":"ADAPTIVE_RUNTIME_REJECTED","phase":"source","message":""}}`
	if result, ok := inheritedErrorDetail(valid); !ok || result.Code != "raw_source_rejected" || result.Message != "" {
		t.Fatal("the actual Stage A shape was not accepted")
	}
	for _, raw := range []string{
		`null`, `{}`, `{"error":null}`, valid + `{}`,
		`{"schema":"invented","error":{"code":"ADAPTIVE_RUNTIME_REJECTED","phase":"source","message":""}}`,
		`{"error":{"code":"ADAPTIVE_RUNTIME_REJECTED","phase":"source"}}`,
		`{"error":{"code":"ADAPTIVE_RUNTIME_REJECTED","phase":"source","message":null}}`,
		`{"error":{"code":"ADAPTIVE_RUNTIME_REJECTED","phase":"projection","message":""}}`,
		`{"error":{"code":"OTHER","phase":"source","message":""}}`,
		`{"error":{"code":"ADAPTIVE_RUNTIME_REJECTED","phase":"source","message":"","extra":0}}`,
		`{"error":{"code":"ADAPTIVE_RUNTIME_REJECTED","phase":"source","phase":"input","message":""}}`,
	} {
		if _, ok := inheritedErrorDetail(raw); ok {
			t.Fatalf("malformed inherited error accepted: %s", raw)
		}
	}
}

type panicError struct{}

func (panicError) Error() string { panic("private panic payload and stack must not escape") }

func TestCoreErrorStaticFallback(t *testing.T) {
	var typedNil *coreError
	for _, err := range []error{nil, typedNil, panicError{},
		newCoreError("unknown", "gross", 1, 1, 1, "private payload"),
		newCoreError("invalid_native_ledger", "/private/path", 1, 1, 1, "private payload"),
		&coreError{code: "invalid_native_ledger", phase: "input", message: "private payload"},
	} {
		if got := errorJSON(err); got != staticInternalErrorJSON {
			t.Fatalf("expected static all-null fallback: %s", got)
		}
	}
	decodeCoreError(t, panicError{})
}
