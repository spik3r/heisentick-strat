package adaptiveflag

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

type runtimeRejection struct{ phase, message string }

func (e *runtimeRejection) Error() string { return e.message }

// Rejection caps every new-runtime error without changing legacy errors.
func Rejection(phase, message string) error {
	switch phase {
	case "request", "source", "input", "resource", "execution":
	default:
		phase = "execution"
	}
	message = strings.ToValidUTF8(message, "\uFFFD")
	if len(message) > 1024 {
		message = message[:1024]
		for !utf8.ValidString(message) {
			message = message[:len(message)-1]
		}
	}
	return &runtimeRejection{phase, message}
}
func ErrorJSON(err error) string {
	phase, message := "execution", "adaptive runtime rejected"
	if err != nil {
		message = err.Error()
	}
	var rejection *runtimeRejection
	if errors.As(err, &rejection) {
		phase = rejection.phase
	}
	rejection = Rejection(phase, message).(*runtimeRejection)
	out, _ := json.Marshal(struct {
		Error struct {
			Code    string `json:"code"`
			Phase   string `json:"phase"`
			Message string `json:"message"`
		} `json:"error"`
	}{Error: struct {
		Code    string `json:"code"`
		Phase   string `json:"phase"`
		Message string `json:"message"`
	}{"ADAPTIVE_RUNTIME_REJECTED", rejection.phase, rejection.message}})
	return string(out)
}
