package main

import (
	"encoding/json"
	"github.com/spik3r/heisentick-strat/report"
)

func runBasketReport(raw string) []byte {
	result, err := report.DecodeBasketReport([]byte(raw))
	if err == nil {
		out, marshalErr := json.Marshal(result)
		if marshalErr == nil {
			return out
		}
		err = marshalErr
	}
	out, _ := json.Marshal(struct {
		Schema string `json:"schema"`
		Error  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{Schema: report.BasketResultSchema, Error: struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{"invalid-request", err.Error()}})
	return out
}
