package engine

// implementedRunCases lists the run-corpus cases the Go engine reproduces.
// Together with runConformanceTODO (engine/run_todo.go) it is the run
// scoreboard: every conformance/run fixture must appear in exactly one of the
// two, and cmd/conformance writes both lists into conformance/metadata.json.
var implementedRunCases = []string{
	stage1RunCase,
	"family-range-break-fake",
	"family-opening-range-breakout",
	"family-inside-day-expansion",
	"family-day-open-reclaim",
	"family-daily-flush-failure",
	"deployed-dsl-session-expansion-ny",
	"deployed-dsl-trend-pullback-xauusd-four-hour-close-resume",
	"family-break-retest",
	"family-supply-demand",
	"family-double-top-bottom",
	"deployed-dsl-dual-ema-resumption-xauusd-four-hour",
	"family-fib-continuation",
	"family-channel-break-hold",
	"family-level-sweep",
	"family-triple-push-exhaustion",
	"family-vwap-extension-fade",
	"deployed-dsl-close-vwap-extreme-magnet-defensive",
	"family-volume-anomaly-exhaustion",
	"family-elder-triple-screen",
	"family-elder-triple-screen-trail-override",
	"family-price-momentum",
	"family-price-momentum-guarded",
	"family-fair-value-gap",
	"family-weekend-extreme-fade",
	"family-intra-hour-run-exhaustion",
	"family-sma-golden-cross",
	"family-sma-golden-cross-protected",
	"deployed-dsl-sma-golden-cross-xauusd-one-minute-canary",
	"family-keltner-reversion",
	"family-keltner-expansion",
	"money-risk-sizing",
	"money-partial-exit",
	"money-stop-distance-gate",
}

// ImplementedRunCases returns the run-corpus cases whose golden the Go engine
// generates, in declaration order.
func ImplementedRunCases() []string {
	out := make([]string, len(implementedRunCases))
	copy(out, implementedRunCases)
	return out
}

// RunConformanceTODO returns the run-corpus cases the Go engine does not yet
// reproduce, keyed by case name with the reviewed reason. No golden is
// generated for these.
func RunConformanceTODO() map[string]string {
	out := make(map[string]string, len(runConformanceTODO))
	for name, reason := range runConformanceTODO {
		out[name] = reason
	}
	return out
}

func runCaseImplemented(caseName string) bool {
	for _, implemented := range implementedRunCases {
		if caseName == implemented {
			return true
		}
	}
	return false
}

// ConformanceProjection is the run result as the corpus records it: the
// trade list with the runtime-only Partial flag cleared. The generator and
// the Go conformance test both serialize this projection, so a golden and
// the engine cannot disagree about which fields are compared.
func ConformanceProjection(result RunResult) RunResult {
	if result.Trades == nil {
		return result
	}
	trades := make([]Trade, len(result.Trades))
	copy(trades, result.Trades)
	result.Trades = trades
	for i := range result.Trades {
		result.Trades[i].Partial = false
	}
	return result
}
