package main

import (
	"errors"

	"github.com/spik3r/heisentick-strat/engine"
)

// This smaller, explicit interactive transport limit does not alter the native
// producer's 128 MiB input/output contract or its execution/accounting semantics.
const paperPrefixWASMMaxRequestBytes = 16 << 20

func paperPrefixFailure(code string) string {
	switch code {
	case "PAPER_PREFIX_ARGUMENTS", "PAPER_PREFIX_INPUT_LIMIT", "PAPER_PREFIX_INPUT",
		"PAPER_PREFIX_ACCOUNTING", "PAPER_PREFIX_ACTIVATION", "PAPER_PREFIX_ADMISSION",
		"PAPER_PREFIX_BARS", "PAPER_PREFIX_CHECKPOINT", "PAPER_PREFIX_CONTEXT_UNSUPPORTED",
		"PAPER_PREFIX_COSTS", "PAPER_PREFIX_EVENT_LIMIT", "PAPER_PREFIX_FAMILY_UNSUPPORTED",
		"PAPER_PREFIX_FILL_UNSUPPORTED", "PAPER_PREFIX_NONFINITE", "PAPER_PREFIX_OUTPUT_LIMIT",
		"PAPER_PREFIX_ROUTE_UNSUPPORTED", "PAPER_PREFIX_SOURCE", "PAPER_PREFIX_UNITS_UNSUPPORTED":
	default:
		code = "PAPER_PREFIX_INTERNAL"
	}
	return `{"ok":false,"error":{"code":"` + code + `"}}`
}

func paperPrefixError(err error) string {
	var typed *engine.PaperPrefixError
	if errors.As(err, &typed) && typed != nil {
		return paperPrefixFailure(typed.Code)
	}
	return paperPrefixFailure("PAPER_PREFIX_INTERNAL")
}

// Success is exactly the producer encoder's document. This adapter performs
// transport admission only; it neither repairs input nor projects financial data.
func runPaperPrefixJSON(raw string) string {
	if len(raw) > paperPrefixWASMMaxRequestBytes {
		return paperPrefixFailure("PAPER_PREFIX_INPUT_LIMIT")
	}
	request, err := engine.DecodePaperPrefixRequest([]byte(raw))
	if err != nil {
		return paperPrefixError(err)
	}
	result, err := engine.RunPaperPrefix(request)
	if err != nil {
		return paperPrefixError(err)
	}
	encoded, err := engine.EncodePaperPrefixResult(result)
	if err != nil {
		return paperPrefixError(err)
	}
	return string(encoded)
}
