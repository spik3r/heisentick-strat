package engine

import "github.com/spik3r/heisentick-strat/dsl"

// reviewedInstrumentPip mirrors the registered pip sizes used by the JS
// engine. Fixed-pip money logic must fail closed when a route is absent.
func reviewedInstrumentPip(symbol string) (float64, bool) {
	return dsl.ReviewedInstrumentPip(symbol)
}
