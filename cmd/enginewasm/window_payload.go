package main

import (
	"encoding/json"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func runWindowPayload(request string, payload []byte) []byte {
	result, err := marketdata.WindowPayload([]byte(request), payload)
	if err == nil {
		out, marshalErr := json.Marshal(result)
		if marshalErr == nil {
			return out
		}
		err = marshalErr
	}
	out, _ := json.Marshal(map[string]any{"schema": marketdata.WindowPayloadResultSchema, "error": map[string]string{"code": "invalid-request", "message": err.Error()}})
	return out
}
