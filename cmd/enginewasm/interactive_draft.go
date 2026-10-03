package main

import (
	"encoding/json"
	"errors"

	native "github.com/spik3r/heisentick-strat/engine"
)

func runInteractiveDraft(raw, source string, inspect bool) []byte {
	var result any
	var err error
	schema := native.InteractiveDraftSchema
	if inspect {
		schema = native.InteractiveSourceProfileSchema
		result, err = native.InspectInteractiveSource([]byte(raw), source)
	} else {
		result, err = native.RunInteractiveDraftFixture([]byte(raw), source)
	}
	if err == nil {
		out, marshalErr := json.Marshal(result)
		if marshalErr == nil {
			return out
		}
		err = marshalErr
	}
	failure := interactiveFailure{Schema: schema}
	failure.Error.Code = "invalid-request"
	if errors.Is(err, native.ErrInteractiveUnsupported) {
		failure.Error.Code = "unsupported-route"
	}
	failure.Error.Message = err.Error()
	out, _ := json.Marshal(failure)
	return out
}
