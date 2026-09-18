package dsl

import "encoding/json"

// FamilyID names the canonical setup family selected by a DSL document.
type FamilyID string

const (
	FamilyFailedBreakout          FamilyID = "failedBreakout"
	FamilyFlagContinuation        FamilyID = "flagContinuation"
	FamilyBreakRetest             FamilyID = "breakRetest"
	FamilyOpeningRangeBreakout    FamilyID = "openingRangeBreakout"
	FamilySessionBreakHold        FamilyID = "sessionBreakHold"
	FamilyChannelBreakHold        FamilyID = "channelBreakHold"
	FamilyInsideDayExpansion      FamilyID = "insideDayExpansion"
	FamilyDayOpenReclaim          FamilyID = "dayOpenReclaim"
	FamilySupplyDemand            FamilyID = "supplyDemand"
	FamilyDoubleTopBottom         FamilyID = "doubleTopBottom"
	FamilyRangeBreakFake          FamilyID = "rangeBreakFake"
	FamilyTrendPullback           FamilyID = "trendPullback"
	FamilyDualEMAResumption       FamilyID = "dualEmaResumption"
	FamilySMAGoldenCross          FamilyID = "smaGoldenCross"
	FamilyFibContinuation         FamilyID = "fibContinuation"
	FamilyTriplePushExhaustion    FamilyID = "triplePushExhaustion"
	FamilyVWAPExtensionFade       FamilyID = "vwapExtensionFade"
	FamilyVolumeAnomalyExhaustion FamilyID = "volumeAnomalyExhaustion"
	FamilyElderTripleScreen       FamilyID = "elderTripleScreen"
	FamilyPriceMomentum           FamilyID = "priceMomentum"
	FamilyDailyFlushFailure       FamilyID = "dailyFlushFailure"
	FamilyFairValueGap            FamilyID = "fairValueGap"
	FamilyWeekendExtremeFade      FamilyID = "weekendExtremeFade"
	FamilyIntraHourRunExhaustion  FamilyID = "intraHourRunExhaustion"
	FamilyKeltnerReversion        FamilyID = "keltnerReversion"
	FamilyKeltnerExpansion        FamilyID = "keltnerExpansion"
)

// ParseResult mirrors the DSL compiler result contract: a config plus both
// human-readable and structured diagnostics.
type ParseResult struct {
	Config      Config
	Errors      []string
	Warnings    []string
	Diagnostics []Diagnostic
}

// MarshalJSON emits the language-agnostic conformance envelope fields for a
// parse result. Tests compare this shape with strat/conformance/parse.
func (result ParseResult) MarshalJSON() ([]byte, error) {
	type parseResultJSON struct {
		Config      Config       `json:"cfg"`
		Errors      []string     `json:"errors"`
		Warnings    []string     `json:"warnings"`
		Diagnostics []Diagnostic `json:"diagnostics"`
	}
	return json.Marshal(parseResultJSON{
		Config:      result.Config,
		Errors:      result.Errors,
		Warnings:    result.Warnings,
		Diagnostics: result.Diagnostics,
	})
}

// Diagnostic is the structured diagnostic shape described by strat/docs/dsl-spec.md.
type Diagnostic struct {
	Severity   DiagnosticSeverity `json:"severity"`
	Message    string             `json:"message"`
	Line       *int               `json:"line"`
	Column     *int               `json:"col"`
	Suggestion string             `json:"suggestion,omitempty"`
}

// DiagnosticSeverity is the stable set of DSL diagnostic severities.
type DiagnosticSeverity string

const (
	DiagnosticError   DiagnosticSeverity = "error"
	DiagnosticWarning DiagnosticSeverity = "warning"
)

// Config is the canonical JSON-facing parser output. The parser keeps this
// adapter at the JS/corpus boundary so conformance can compare the emitted cfg
// byte-for-byte while internal parsing routes setup families through FamilyID
// and shared helpers instead of exposing callers to map mutation.
type Config map[string]any
