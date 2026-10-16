package main

import (
	"errors"

	"github.com/spik3r/heisentick-strat/engine"
)

// Refusals share the native command's complete envelope. Only fixed codes may
// cross this boundary; error messages can contain source or other private data.
func paperBidAskFailure(code string) string {
	switch code {
	case "PAPER_BIDASK_ARGUMENTS", "PAPER_BIDASK_ACTIVATION", "PAPER_BIDASK_ADMISSION",
		"PAPER_BIDASK_BAR", "PAPER_BIDASK_BAR_COMPONENTS", "PAPER_BIDASK_BAR_LIMIT",
		"PAPER_BIDASK_CHECKPOINT", "PAPER_BIDASK_COMMIT_ORDER", "PAPER_BIDASK_CONTEXT_UNSUPPORTED",
		"PAPER_BIDASK_DECIMAL", "PAPER_BIDASK_ECONOMICS", "PAPER_BIDASK_EPISODE_CLOCK",
		"PAPER_BIDASK_EPISODE_DEADLINE", "PAPER_BIDASK_EPISODE_STATE", "PAPER_BIDASK_EVENT_LIMIT", "PAPER_BIDASK_EXPERIMENT", "PAPER_BIDASK_CALENDAR", "PAPER_BIDASK_CALENDAR_CONTRADICTION", "PAPER_BIDASK_CALENDAR_HORIZON", "PAPER_BIDASK_CLOSURE", "PAPER_BIDASK_CLOSURE_STATE", "PAPER_BIDASK_EXPERIMENT_CLOCK", "PAPER_BIDASK_EXPERIMENT_HORIZON",
		"PAPER_BIDASK_FAMILY_NUMERIC", "PAPER_BIDASK_FAMILY_UNQUALIFIED", "PAPER_BIDASK_GAP",
		"PAPER_BIDASK_IDENTITY", "PAPER_BIDASK_INPUT", "PAPER_BIDASK_INTERNAL",
		"PAPER_BIDASK_MULTIPLE_DECISIONS", "PAPER_BIDASK_NONFINITE", "PAPER_BIDASK_ORDER_UNQUALIFIED",
		"PAPER_BIDASK_OUTPUT", "PAPER_BIDASK_OUTPUT_LIMIT", "PAPER_BIDASK_PARTIAL_UNQUALIFIED",
		"PAPER_BIDASK_POLICY", "PAPER_BIDASK_PRODUCT_UNSUPPORTED", "PAPER_BIDASK_QUOTE",
		"PAPER_BIDASK_QUOTE_CLOCK", "PAPER_BIDASK_QUOTE_CONFLICT", "PAPER_BIDASK_QUOTE_DEPTH",
		"PAPER_BIDASK_QUOTE_IDENTITY_LIMIT", "PAPER_BIDASK_QUOTE_SPREAD", "PAPER_BIDASK_RECORD",
		"PAPER_BIDASK_RISK", "PAPER_BIDASK_ROUTE", "PAPER_BIDASK_ROUTE_UNSUPPORTED",
		"PAPER_BIDASK_SEAL", "PAPER_BIDASK_SEAL_CLOCK", "PAPER_BIDASK_SEQUENCE",
		"PAPER_BIDASK_SOURCE", "PAPER_BIDASK_SOURCE_NUMERIC", "PAPER_BIDASK_TIMESTAMP",
		"PAPER_BIDASK_TRAILING_UNQUALIFIED", "PAPER_BIDASK_UNRESOLVED_EPISODE", "PAPER_BIDASK_VOLUME":
	default:
		code = "PAPER_BIDASK_INTERNAL"
	}
	return `{"schema":"` + engine.PaperBidAskSchema + `","status":"refused","code":"` + code + `"}`
}

func paperBidAskError(err error) string {
	var typed *engine.PaperBidAskError
	if errors.As(err, &typed) && typed != nil {
		return paperBidAskFailure(typed.Code)
	}
	return paperBidAskFailure("PAPER_BIDASK_INTERNAL")
}

// This is only transport. The public native API owns all source admission,
// execution, financial arithmetic, identities, replay and resource policy.
func runPaperBidAskJSON(raw string) string {
	if len(raw) > engine.PaperBidAskMaxBytes {
		return paperBidAskFailure("PAPER_BIDASK_INPUT")
	}
	request, err := engine.DecodePaperBidAskRequest([]byte(raw))
	if err != nil {
		return paperBidAskError(err)
	}
	result, err := engine.RunPaperBidAsk(request)
	if err != nil {
		return paperBidAskError(err)
	}
	encoded, err := engine.EncodePaperBidAskResult(result)
	if err != nil {
		return paperBidAskError(err)
	}
	return string(encoded)
}
