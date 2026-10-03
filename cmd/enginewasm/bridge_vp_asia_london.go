package main

import (
	"encoding/json"
	"errors"

	native "github.com/spik3r/heisentick-strat/engine"
)

func authoredVPWideFixtureID(raw string) bool {
	var identity struct {
		StrategyID string `json:"strategyId"`
	}
	return json.Unmarshal([]byte(raw), &identity) == nil && identity.StrategyID == "vpAsiaLondonSweepContinuationFiveMinuteWideAsia"
}

// The existing native --interactive and WASM engineRunInteractiveFixture
// transports both use this adapter. Only the exact active authored ID selects
// it; a source string is an unsupported combination, never silently ignored.
func runAuthoredVPWideInteractive(raw, source string) []byte {
	var err error
	if source != "" {
		err = errors.New("authored VP Asia-London wide does not accept DSL source")
	} else {
		result, runErr := native.RunAuthoredVPAsiaLondonWideInteractive([]byte(raw))
		if runErr == nil {
			out, marshalErr := json.Marshal(result)
			if marshalErr == nil {
				return out
			}
			err = marshalErr
		} else {
			err = runErr
		}
	}
	failure := interactiveFailure{Schema: native.AuthoredVPAsiaLondonWideInteractiveSchema}
	failure.Error.Code = "invalid-request"
	if errors.Is(err, native.ErrInteractiveUnsupported) {
		failure.Error.Code = "unsupported-route"
	}
	failure.Error.Message = err.Error()
	out, _ := json.Marshal(failure)
	return out
}
