package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
)

func TestBasketReportBridge(t *testing.T) {
	raw := `{"schema":"basket-report-request-v1","nowMs":1969660800000,"identity":{"run":"invented"},"legs":[{"symbol":"TEST_A","tf":"2h","bars":2,"identity":{"input":"synthetic"},"trades":[{"entryT":1938297600000,"entry":12,"size":3,"pnl":9,"initialSl":11,"sl":12}]}]}`
	var result struct {
		Schema        string
		RequestSHA256 string
		Aggregate     struct {
			Trades int
			SumR   float64
		}
		Error any
	}
	if err := json.Unmarshal(runBasketReport(raw), &result); err != nil {
		t.Fatal(err)
	}
	if result.Schema != "basket-report-result-v1" || result.RequestSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(raw))) || result.Aggregate.Trades != 1 || result.Aggregate.SumR != 3 || result.Error != nil {
		t.Fatalf("bad output %+v", result)
	}
}

func TestBasketReportBridgeRejectsUnknownExecutionInput(t *testing.T) {
	for _, raw := range []string{"", `{"schema":"basket-report-request-v1","nowMs":0,"identity":{},"legs":[],"source":"execute nothing"}`} {
		var result struct {
			Schema string
			Error  struct {
				Code    string
				Message string
			}
		}
		if err := json.Unmarshal(runBasketReport(raw), &result); err != nil {
			t.Fatal(err)
		}
		if result.Schema != "basket-report-result-v1" || result.Error.Code != "invalid-request" || result.Error.Message == "" {
			t.Fatalf("bad failure %+v", result)
		}
	}
}
