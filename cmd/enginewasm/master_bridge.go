package main

import "github.com/spik3r/heisentick-strat/report/masterstructural"

// This is a separate admission path, never a fallback from generic execution.
func runMasterPortableReportBTB1(metadata, source string, data []byte) ([]byte, error) {
	return masterstructural.BuildPortableV1(metadata, source, data)
}
