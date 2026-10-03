package main

import (
	"encoding/json"
	"testing"
)

func TestInteractiveDraftTransportFailures(t *testing.T) {
	for _, inspect := range []bool{true, false} {
		var result struct {
			Schema string `json:"schema"`
			Error  struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(runInteractiveDraft("", "", inspect), &result); err != nil {
			t.Fatal(err)
		}
		if result.Schema == "" || result.Error.Code != "invalid-request" || result.Error.Message == "" {
			t.Fatalf("failure = %#v", result)
		}
	}
}
