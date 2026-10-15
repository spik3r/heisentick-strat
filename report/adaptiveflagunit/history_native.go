//go:build !js && !wasm

package adaptiveflagunit

import (
	"encoding/json"

	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
)

const (
	HistoryEnvelopeSchema       = "strat-adaptive-volume-flag-unit-history-v1"
	HistoryMaxCalendarDays      = adaptiveflag.HistoryMaxCalendarDays
	HistoryMaxExposureMidnights = 30000
	HistoryMaxExposureGaps      = 70000
	HistoryMaxOutputBytes       = 256 * 1024 * 1024
	HistoryLongH1MaxOutputBytes = 512 * 1024 * 1024
)

type HistoryEnvelope struct {
	Schema                  string                      `json:"schema"`
	Profile                 string                      `json:"profile"`
	RawEnvelopeSchema       string                      `json:"rawEnvelopeSchema"`
	RawEnvelopeSHA256       string                      `json:"rawEnvelopeSha256"`
	RawEnvelopeBytes        int                         `json:"rawEnvelopeBytes"`
	RawRequest              adaptiveflag.RuntimeRequest `json:"rawRequest"`
	RawRequestSHA256        string                      `json:"rawRequestSha256"`
	ProjectionRequest       Request                     `json:"projectionRequest"`
	ProjectionRequestSHA256 string                      `json:"projectionRequestSha256"`
	Projection              *Projection                 `json:"projection"`
}

// PreparedHistory owns strict metadata and source; it accepts only supplied BTB1
// bytes. No ledger upload, arbitrary limit, or browser admission is exposed.
type PreparedHistory struct {
	raw        *adaptiveflag.PreparedHistory
	request    Request
	requestSHA string
	profile    string
}

func PrepareHistory(profile, rawMetadata, projectionMetadata, source string) (*PreparedHistory, error) {
	if !adaptiveflag.IsHistoryProfile(profile) {
		return nil, Rejection("projection_request_rejected", "projection_request", "explicit adaptive-history-v1 native profile required")
	}
	request, err := DecodeRuntimeRequest(projectionMetadata)
	if err != nil {
		return nil, err
	}
	raw, err := adaptiveflag.PrepareHistory(profile, rawMetadata, source)
	if err != nil {
		return nil, err
	}
	return &PreparedHistory{raw, request, rawBytesSHA([]byte(projectionMetadata)), profile}, nil
}

func BuildHistory(profile, rawMetadata, projectionMetadata, source string, data []byte) ([]byte, error) {
	p, err := PrepareHistory(profile, rawMetadata, projectionMetadata, source)
	if err != nil {
		return nil, err
	}
	return p.Build(data)
}

func (p *PreparedHistory) Build(data []byte) (output []byte, err error) {
	defer func() {
		if recover() != nil {
			output = nil
			err = Rejection("internal_error", "", "adaptive native history runtime failed")
		}
	}()
	if p == nil || p.raw == nil {
		return nil, Rejection("projection_request_rejected", "projection_request", "native history source was not prepared")
	}
	document, err := p.raw.Build(data)
	if err != nil {
		return nil, err
	}
	// The raw allocation is row/input bounded. Validate all actual fields and
	// global exposure cardinalities before raw serialization or unit allocation.
	limits := historyLimitsFor(p.profile)
	ctx, err := validateRunWithLimits(&document.Run, limits)
	if err != nil {
		return nil, err
	}
	if _, err = historyProjectionBoundWithLimits(&document.Run, ctx, p.profile); err != nil {
		return nil, Rejection("resource_limit", "output_bound", "native history projection conservative bound exceeded")
	}
	maxOutput := historyMaxOutputBytes(p.profile)
	rawBound, err := historyJSONBoundForProfile(p.profile, document)
	if err != nil || rawBound+1 > maxOutput {
		return nil, Rejection("resource_limit", "output_bound", "native history raw conservative bound exceeded")
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return nil, Rejection("internal_error", "serialize", "native history raw serialization failed")
	}
	raw = append(raw, '\n')
	if uint64(len(raw)) > rawBound+1 {
		return nil, Rejection("resource_limit", "output_bound", "native history raw bound violated")
	}
	owned := ownedRaw{document.Run, raw, document.DSLSHA256, document.ConfigSHA256, document.BTB1SHA256}
	projection, err := projectWithLimits(owned, p.request, limits)
	if err != nil {
		return nil, err
	}
	envelope := HistoryEnvelope{HistoryEnvelopeSchema, p.profile, adaptiveflag.HistoryRawSchema, rawBytesSHA(raw), len(raw), document.Request, document.RequestSHA256, p.request, p.requestSHA, projection}
	bound, err := historyJSONBoundForProfile(p.profile, envelope)
	if err != nil || bound+1 > maxOutput {
		return nil, Rejection("resource_limit", "output_bound", "native history final conservative bound exceeded")
	}
	output, err = json.Marshal(envelope)
	if err != nil {
		return nil, Rejection("internal_error", "serialize", "native history final serialization failed")
	}
	output = append(output, '\n')
	if uint64(len(output)) > bound+1 {
		return nil, Rejection("resource_limit", "output_bound", "native history final bound violated")
	}
	return output, nil
}
