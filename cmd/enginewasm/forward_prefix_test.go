package main

import (
	"encoding/json"
	"testing"

	native "github.com/spik3r/heisentick-strat/engine"
)

func TestForwardPrefixStructuredFailure(t *testing.T) {
	var response struct {
		Schema string `json:"schema"`
		Error  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(runForwardPrefix("", ""), &response); err != nil {
		t.Fatal(err)
	}
	if response.Schema != native.ForwardPrefixSchema || response.Error.Code != "invalid-input" || response.Error.Message == "" {
		t.Fatalf("missing shared failure envelope: %+v", response)
	}
}
