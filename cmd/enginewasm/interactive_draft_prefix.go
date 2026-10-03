package main

import (
	"encoding/json"
	"errors"

	native "github.com/spik3r/heisentick-strat/engine"
)

func runInteractiveDraftPrefix(raw, source string) []byte {
	result, err := native.RunInteractiveDraftPrefixFixture([]byte(raw), source)
	if err == nil {
		out, marshalErr := json.Marshal(result)
		if marshalErr == nil {
			return out
		}
		err = marshalErr
	}
	code := "invalid-request"
	var unsupported *native.PrefixUnsupportedError
	if errors.Is(err, native.ErrInteractiveUnsupported) || errors.As(err, &unsupported) {
		code = "unsupported-route"
	}
	out, _ := json.Marshal(map[string]any{"schema": native.InteractiveDraftPrefixSchema,
		"error": map[string]string{"code": code, "message": err.Error()}})
	return out
}
