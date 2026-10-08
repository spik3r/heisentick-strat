package adaptiveflagunit

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
)

const maxErrorMessageBytes = 1024

// This literal is independent of the serializer and is also the panic fallback.
const staticInternalErrorJSON = `{"schema":"adaptive-flag-unit-error-v1","error":{"code":"internal_error","phase":"execution","message":"adaptive unit runtime failed","location":{"operation":null,"rowIndex":null,"orderId":null,"eventId":null}}}`

type coreError struct {
	code, phase, message, operation string
	row, order, event               int
}

// Error exposes the stable code to ordinary native stderr diagnostics. The
// bounded descriptive message is available only through the closed envelope.
func (e *coreError) Error() string   { return e.code }
func (e *coreError) Code() string    { return e.code }
func (e *coreError) Phase() string   { return e.phase }
func (e *coreError) Message() string { return e.message }
func (e *coreError) Location() ErrorLocation {
	var op *string
	if e.operation != "" {
		value := e.operation
		op = &value
	}
	return ErrorLocation{op, errorIndex(e.row, 1023), errorIndex(e.order, 1023), errorIndex(e.event, 4095)}
}

func errorPhase(code string) string {
	switch code {
	case "raw_request_rejected", "projection_request_rejected":
		return "request"
	case "raw_source_rejected":
		return "source"
	case "raw_input_rejected", "projection_domain_rejected":
		return "input"
	case "resource_limit":
		return "resource"
	case "raw_execution_rejected", "internal_error":
		return "execution"
	case "invalid_native_ledger", "invalid_planned_risk", "nonfinite_projection_arithmetic", "ledger_reconciliation_failed", "daily_reconciliation_failed":
		return "projection"
	}
	return ""
}

func validErrorOperation(operation string) bool {
	switch operation {
	case "", "raw_request", "projection_request", "source", "btb1_header", "btb1_copy", "raw_build", "raw_decode",
		"time_domain", "calendar_count", "output_bound", "exposure_count", "lifecycle", "distance", "quantity",
		"entry_charge", "exit_charge", "cost_component", "fill_risk_raw", "display_adjustment", "fill_risk_effective",
		"gross", "gross_realized", "paid_cost", "net", "closed_net", "open_gross", "marked", "alternate",
		"reconciliation_delta", "close_drawdown", "closed_drawdown", "summary_positive", "summary_negative",
		"summary_net", "summary_mean", "summary_pf", "daily_change", "daily_reconciliation", "calendar_year_sum",
		"hypothetical_exit_cost", "hypothetical_liquidation", "identity", "serialize", "transport", "native_io":
		return true
	}
	return false
}

func errorIndex(value, maximum int) *int {
	if value < 0 || value > maximum {
		return nil
	}
	return &value
}

// boundedErrorMessage is strings.ToValidUTF8 followed by a UTF8-boundary byte
// truncation, without allocating a copy proportional to an unbounded message.
func boundedErrorMessage(message string) string {
	var out strings.Builder
	if len(message) < maxErrorMessageBytes {
		out.Grow(len(message))
	} else {
		out.Grow(maxErrorMessageBytes)
	}
	for len(message) > 0 && out.Len() < maxErrorMessageBytes {
		r, size := utf8.DecodeRuneInString(message)
		if r == utf8.RuneError && size == 1 {
			if out.Len()+3 > maxErrorMessageBytes {
				break
			}
			out.WriteRune(utf8.RuneError)
			// Match ToValidUTF8's one replacement for each run of invalid bytes.
			for len(message) > 0 {
				r, size = utf8.DecodeRuneInString(message)
				if r != utf8.RuneError || size != 1 {
					break
				}
				message = message[1:]
			}
			continue
		}
		if out.Len()+size > maxErrorMessageBytes {
			break
		}
		out.WriteString(message[:size])
		message = message[size:]
	}
	return out.String()
}

func newCoreError(code, operation string, row, order, event int, message string) error {
	phase := errorPhase(code)
	if phase == "" || !validErrorOperation(operation) {
		return &coreError{"internal_error", "execution", "adaptive unit runtime failed", "", -1, -1, -1}
	}
	return &coreError{code, phase, boundedErrorMessage(message), operation, row, order, event}
}

// inheritedErrorDetail accepts exactly Stage A's owned, compact ErrorJSON
// shape. Stage A has no top-level schema field. Re-encoding its private wire
// shape also detects duplicate/missing keys and any unreviewed extra fields.
func inheritedErrorDetail(raw string) (ErrorDetail, bool) {
	var wire struct {
		Error *struct {
			Code    *string `json:"code"`
			Phase   *string `json:"phase"`
			Message *string `json:"message"`
		} `json:"error"`
	}
	if len(raw)+1 > MaxErrorBytes || !utf8.ValidString(raw) {
		return ErrorDetail{}, false
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&wire) != nil || wire.Error == nil || wire.Error.Code == nil || wire.Error.Phase == nil || wire.Error.Message == nil || *wire.Error.Code != "ADAPTIVE_RUNTIME_REJECTED" {
		return ErrorDetail{}, false
	}
	owned, err := json.Marshal(wire)
	if err != nil || !bytes.Equal(owned, []byte(raw)) {
		return ErrorDetail{}, false
	}
	var code string
	switch *wire.Error.Phase {
	case "request":
		code = "raw_request_rejected"
	case "source":
		code = "raw_source_rejected"
	case "input":
		code = "raw_input_rejected"
	case "resource":
		code = "resource_limit"
	case "execution":
		code = "raw_execution_rejected"
	default:
		return ErrorDetail{}, false
	}
	return ErrorDetail{Code: code, Phase: *wire.Error.Phase, Message: boundedErrorMessage(*wire.Error.Message)}, true
}

func errorJSON(err error) (output string) {
	output = staticInternalErrorJSON
	defer func() {
		if recover() != nil {
			output = staticInternalErrorJSON
		}
	}()
	if err == nil {
		return output
	}
	var detail ErrorDetail
	var core *coreError
	if errors.As(err, &core) {
		if core == nil || errorPhase(core.code) != core.phase || core.phase == "" || !validErrorOperation(core.operation) {
			return output
		}
		detail = ErrorDetail{core.code, core.phase, boundedErrorMessage(core.message), core.Location()}
	} else {
		var ok bool
		detail, ok = inheritedErrorDetail(adaptiveflag.ErrorJSON(err))
		if !ok {
			return output
		}
	}
	encoded, marshalErr := json.Marshal(ErrorEnvelope{ErrorSchema, detail})
	// Reserve the native transport's newline even though this helper is compact.
	if marshalErr != nil || len(encoded)+1 > MaxErrorBytes {
		return output
	}
	return string(encoded)
}
