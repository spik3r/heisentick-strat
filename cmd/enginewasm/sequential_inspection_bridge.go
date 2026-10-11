package main

import (
	"encoding/json"
	"github.com/spik3r/heisentick-strat/engine"
)

func sequentialInspectionJSON(result engine.SequentialSourceInspection) string {
	raw, err := json.Marshal(result)
	if err != nil {
		raw, _ = json.Marshal(engine.SequentialInspectionFailure("SEQUENTIAL_INSPECTION_SERIALIZATION"))
	}
	return string(raw)
}

func inspectSequentialSource(source string) string {
	return sequentialInspectionJSON(engine.InspectSequentialSource(source))
}
