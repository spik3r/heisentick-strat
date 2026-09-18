package engine

// reviewedInstrumentPip mirrors the registered pip sizes used by the JS
// engine. Fixed-pip money logic must fail closed when a route is absent.
func reviewedInstrumentPip(symbol string) (float64, bool) {
	switch symbol {
	case "XAUUSD":
		return 0.1, true
	case "USDJPY", "GBPJPY", "LIGHTCMDUSD", "BRENTCMDUSD", "BTCUSDT":
		return 0.01, true
	case "GASCMDUSD":
		return 0.001, true
	case "AUS200", "NAS100", "US500":
		return 1, true
	case "EURUSD", "GBPUSD", "AUDUSD", "EURGBP":
		return 0.0001, true
	default:
		return 0, false
	}
}
