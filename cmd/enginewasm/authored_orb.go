package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spik3r/heisentick-strat/authoredorb"
)

// runAuthoredOrb is a whole-strategy native/WASM entrypoint for the authored
// JavaScript ORB pair. It deliberately does not parse a Strat source string.
func runAuthoredOrb(raw string) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	var request authoredorb.Request
	if err := decoder.Decode(&request); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("trailing JSON after authored ORB request")
		}
		return nil, fmt.Errorf("trailing authored ORB JSON: %w", err)
	}
	result, err := authoredorb.Run(request)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
