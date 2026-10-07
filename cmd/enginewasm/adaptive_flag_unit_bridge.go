package main

import "github.com/spik3r/heisentick-strat/report/adaptiveflagunit"

// Unit economics can only be derived from a fresh, owned Stage A raw run.
// This separate route leaves the existing raw and generic routes unchanged.
func runAdaptiveFlagUnitReportBTB1(rawMetadata, projectionMetadata, source string, data []byte) ([]byte, error) {
	return adaptiveflagunit.BuildRuntime(rawMetadata, projectionMetadata, source, data)
}
