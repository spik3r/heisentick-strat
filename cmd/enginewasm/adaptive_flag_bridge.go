package main

import "github.com/spik3r/heisentick-strat/report/adaptiveflag"

// The dedicated route shares the report builder. Generic execution routes keep
// their existing adaptive-family refusals and never fall back to this route.
func runAdaptiveFlagReportBTB1(metadata, source string, data []byte) ([]byte, error) {
	return adaptiveflag.BuildRuntime(metadata, source, data)
}
