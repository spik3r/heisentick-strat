package engine

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/spik3r/heisentick-strat/dsl"
)

// AdaptiveFlagResearchAblation is a closed, value-only native research request.
// Its zero value preserves the legacy runner and its serialized output.
type AdaptiveFlagResearchAblation string

const (
	AdaptiveFlagResearchRetraceCapOff    AdaptiveFlagResearchAblation = "C_RETRACE_CAP_OFF_CORE_V1"
	AdaptiveFlagResearchWidthCapOff      AdaptiveFlagResearchAblation = "C_WIDTH_CAP_OFF_CORE_V1"
	AdaptiveFlagResearchBothCapsOff      AdaptiveFlagResearchAblation = "C_RETRACE_WIDTH_CAP_OFF_CORE_V1"
	AdaptiveFlagResearchSchema                                        = "strat-adaptive-volume-flag-cap-research-v1"
	AdaptiveFlagResearchCLISchema                                     = "strat-adaptive-volume-flag-cli-cap-research-v1"
	AdaptiveFlagResearchDefinition                                    = "adaptive-G123-cap-only-fixed-core-v1"
	AdaptiveFlagResearchDefinitionSHA256                              = "332b5037247bbd904f4e05923e286385cc6b9b53e3723297447810139982869e"
	AdaptiveFlagResearchProducerContract                              = "adaptive-G123-native-overlay-v1"
)

// AdaptiveFlagResearchPolicy contains only values. Field order is the frozen
// hash encoding: compact Go encoding/json UTF-8 bytes, no newline or hash field.
// BaseConfigSHA256 keeps its legacy meaning; it cannot identify a research run.
type AdaptiveFlagResearchPolicy struct {
	Contract           string                       `json:"contract"`
	DefinitionID       string                       `json:"definitionId"`
	DefinitionSHA256   string                       `json:"definitionSha256"`
	CandidateID        string                       `json:"candidateId"`
	Request            AdaptiveFlagResearchAblation `json:"request"`
	CorePolicy         string                       `json:"corePolicy"`
	RetraceCapEnabled  bool                         `json:"retraceCapEnabled"`
	WidthCapEnabled    bool                         `json:"widthCapEnabled"`
	EMAEnabled         bool                         `json:"emaEnabled"`
	VolumeEnabled      bool                         `json:"volumeEnabled"`
	BaseBundle         string                       `json:"baseBundle"`
	BaseConfigSHA256   string                       `json:"baseConfigSha256"`
	SourcePineSHA256   string                       `json:"sourcePineSha256"`
	ReferenceSHA256    string                       `json:"referenceSha256"`
	NumericalPolicy    string                       `json:"numericalPolicy"`
	ExecutionSemantics string                       `json:"executionSemantics"`
	ProducerContract   string                       `json:"producerContract"`
}

// AdaptiveFlagResearchProducer reports available build provenance, never a
// guessed clean commit. Qualification must additionally pin source/binary hashes.
type AdaptiveFlagResearchProducer struct {
	Contract    string `json:"contract"`
	GoVersion   string `json:"goVersion"`
	VCSRevision string `json:"vcsRevision"`
	VCSModified bool   `json:"vcsModified"`
}

func adaptiveFlagResearchPolicy(request AdaptiveFlagResearchAblation, spec dsl.AdaptiveFlagSpec) (*AdaptiveFlagResearchPolicy, error) {
	if request == "" {
		return nil, nil
	}
	if runtime.GOARCH == "wasm" {
		return nil, fmt.Errorf("adaptive flag research requires native dedicated runner")
	}
	p := AdaptiveFlagResearchPolicy{
		Contract: "adaptive-G123-cap-policy-v1", DefinitionID: AdaptiveFlagResearchDefinition,
		DefinitionSHA256: AdaptiveFlagResearchDefinitionSHA256, Request: request,
		CorePolicy: "SOURCE_BOTH_PIVOTS_IMPULSE_V1", RetraceCapEnabled: true, WidthCapEnabled: true,
		EMAEnabled: true, VolumeEnabled: true, BaseBundle: "SNAPSHOT_C",
		SourcePineSHA256: AdaptiveFlagSourcePineSHA256, ReferenceSHA256: AdaptiveFlagReferenceSHA256,
		NumericalPolicy: dsl.AdaptiveFlagNumericalPolicy, ExecutionSemantics: AdaptiveFlagExecutionSemantics,
		ProducerContract: AdaptiveFlagResearchProducerContract,
	}
	switch request {
	case AdaptiveFlagResearchRetraceCapOff:
		p.CandidateID, p.RetraceCapEnabled = "G1", false
	case AdaptiveFlagResearchWidthCapOff:
		p.CandidateID, p.WidthCapEnabled = "G2", false
	case AdaptiveFlagResearchBothCapsOff:
		p.CandidateID, p.RetraceCapEnabled, p.WidthCapEnabled = "G3", false, false
	default:
		return nil, fmt.Errorf("unknown adaptive flag research ablation %q", request)
	}
	expected, _ := dsl.AdaptiveFlagPreset("SNAPSHOT_C")
	if spec.Bundle != "SNAPSHOT_C" || spec.Rules != expected {
		return nil, fmt.Errorf("adaptive flag research requires exact SNAPSHOT_C bundle and rules")
	}
	var err error
	p.BaseConfigSHA256, err = adaptiveFlagHash(spec)
	if err != nil {
		return nil, fmt.Errorf("adaptive flag research base identity: %w", err)
	}
	return &p, nil
}

// Overlay only effective validity and its unchanged quote. Source features,
// source impulses, pivot availability and every portfolio rule remain untouched.
func adaptiveFlagResearchOverlay(rows []AdaptiveFlagSnapshot, r dsl.AdaptiveFlagRules, p AdaptiveFlagResearchPolicy) {
	for i := range rows {
		s := &rows[i]
		s.BullValid = s.BullImpulse && s.BullRetrace != nil && (!p.RetraceCapEnabled || *s.BullRetrace <= float64(s.BullHeight*r.MaxFlagRetrace)) && (!p.WidthCapEnabled || s.FlagWidth <= float64(s.BullHeight*r.FlagWidthPoleMult)) && s.TrendBull && s.VolumeOK
		s.BearValid = s.BearImpulse && s.BearRetrace != nil && (!p.RetraceCapEnabled || *s.BearRetrace <= float64(s.BearHeight*r.MaxFlagRetrace)) && (!p.WidthCapEnabled || s.FlagWidth <= float64(s.BearHeight*r.FlagWidthPoleMult)) && s.TrendBear && s.VolumeOK
		s.Candidate = adaptiveFlagCandidate(*s, r)
	}
}

func adaptiveFlagResearchBuild() *AdaptiveFlagResearchProducer {
	p := &AdaptiveFlagResearchProducer{Contract: AdaptiveFlagResearchProducerContract, GoVersion: runtime.Version()}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				p.VCSRevision = s.Value
			case "vcs.modified":
				p.VCSModified = s.Value == "true"
			}
		}
	}
	return p
}
