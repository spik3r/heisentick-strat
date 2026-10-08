package adaptiveflagunit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"runtime"
	"runtime/debug"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/engine"
)

func policyFor(name string) (CostPolicy, error) {
	zero, spread, commission, aggregate := 0.0, 0.06, 0.035, 0.155
	switch name {
	case "RAW":
		return CostPolicy{"RAW", 0, &zero, &zero, nil}, nil
	case "RAZOR_PROXY_BASIC":
		return CostPolicy{"RAZOR_PROXY_BASIC", 0.095, &spread, &commission, nil}, nil
	case "RAZOR_PROXY_HARSH_AGGREGATE":
		return CostPolicy{"RAZOR_PROXY_HARSH_AGGREGATE", 0.155, nil, nil, &aggregate}, nil
	}
	return CostPolicy{}, newCoreError("projection_request_rejected", "projection_request", -1, -1, -1, "unsupported cost policy")
}

func economicPolicy(policy CostPolicy) EconomicPolicy {
	return EconomicPolicy{"UNIT_POINT_VALUE_1-go-v1", "UNIT_POINT_VALUE_1", 1, 1, policy, "unknown_unmodeled"}
}

func numericalDefinition() NumericalPolicy {
	return NumericalPolicy{"adaptive-unit-numerical-policy-go-v1", "BINARY64_ORDERED_V1", "chronological positive-zero", "separate binary64", "direct"}
}

func samePolicyScalar(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return math.Float64bits(*a) == math.Float64bits(*b)
}

func economicPolicySHA256(policy CostPolicy) (string, error) {
	fixed, err := policyFor(policy.Name)
	if err != nil {
		return "", err
	}
	if math.Float64bits(policy.PerFill) != math.Float64bits(fixed.PerFill) || !samePolicyScalar(policy.Spread, fixed.Spread) || !samePolicyScalar(policy.Commission, fixed.Commission) || !samePolicyScalar(policy.Aggregate, fixed.Aggregate) {
		return "", newCoreError("internal_error", "identity", -1, -1, -1, "cost policy differs from the fixed definition")
	}
	return identityStructSHA256(economicPolicy(policy))
}

func numericalPolicySHA256() (string, error) {
	return identityStructSHA256(numericalDefinition())
}

func identityStructSHA256(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", newCoreError("internal_error", "identity", -1, -1, -1, "identity serialization failed")
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func validIdentitySourceID(id string) bool {
	if len(id) < 1 || len(id) > 128 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == ':' || c == '/' || c == '-' {
			continue
		}
		return false
	}
	return true
}

func lowercaseHexOfLength(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// Field order is Python's lexicographically sorted compact JSON for this
// restricted string/integer/null domain. No general Python float or Unicode
// serialization, raw input digest, execution window or config enters these IDs.
type originPivotIdentity struct {
	ConfirmationOpenMS int64 `json:"confirmation_open_ms"`
	OccurrenceOpenMS   int64 `json:"occurrence_open_ms"`
}

type originEpisodeIdentity struct {
	DataSourceID     string               `json:"data_source_id"`
	HighPivot        *originPivotIdentity `json:"high_pivot"`
	LowPivot         *originPivotIdentity `json:"low_pivot"`
	Side             string               `json:"side"`
	SourcePineSHA256 string               `json:"source_pine_sha256"`
	TimeframeMS      int64                `json:"timeframe_ms"`
}

type originCandidateIdentity struct {
	DataSourceID      string               `json:"data_source_id"`
	DecisionBarOpenMS int64                `json:"decision_bar_open_ms"`
	HighPivot         *originPivotIdentity `json:"high_pivot"`
	LowPivot          *originPivotIdentity `json:"low_pivot"`
	Side              string               `json:"side"`
	SourcePineSHA256  string               `json:"source_pine_sha256"`
	TimeframeMS       int64                `json:"timeframe_ms"`
}

func originIdentity(run *engine.AdaptiveFlagResult, side string, row int, sourceID string) (episode, candidate string, err error) {
	reject := func(message string) (string, string, error) {
		return "", "", newCoreError("invalid_native_ledger", "identity", row, -1, -1, message)
	}
	if !validIdentitySourceID(sourceID) {
		return "", "", newCoreError("projection_request_rejected", "identity", row, -1, -1, "invalid data source identity")
	}
	if run == nil || row < 0 || row >= len(run.Snapshots) || row >= MaxRetainedRows || len(run.Snapshots) > MaxRetainedRows {
		return reject("identity requires an admitted decision row")
	}
	if (side != "long" && side != "short") || !lowercaseHexOfLength(run.SourcePineSHA256, 64) || run.TimeframeMS <= 0 || run.TimeframeMS >= MaxCalendarEndpointMS {
		return reject("invalid native causal identity fields")
	}
	validOpen := func(open int64) bool {
		return open >= 0 && open < MaxCalendarEndpointMS && open%run.TimeframeMS == 0
	}
	decision := run.Snapshots[row]
	if !validOpen(decision.OpenT) {
		return reject("invalid decision timestamp for causal identity")
	}
	anchors := [2]*originPivotIdentity{}
	for i, pivot := range [2]*engine.AdaptiveFlagPivot{decision.LastHigh, decision.LastLow} {
		if pivot == nil {
			continue
		}
		if pivot.Index < 0 || pivot.Index > pivot.ConfirmationIdx || pivot.ConfirmationIdx > row {
			return reject("noncausal pivot in causal identity")
		}
		occurrence, confirmation := run.Snapshots[pivot.Index].OpenT, run.Snapshots[pivot.ConfirmationIdx].OpenT
		if !validOpen(occurrence) || !validOpen(confirmation) || occurrence > confirmation || confirmation > decision.OpenT {
			return reject("invalid pivot timestamp for causal identity")
		}
		anchors[i] = &originPivotIdentity{confirmation, occurrence}
	}
	episode, err = identityStructSHA256(originEpisodeIdentity{sourceID, anchors[0], anchors[1], side, run.SourcePineSHA256, run.TimeframeMS})
	if err != nil {
		return "", "", err
	}
	candidate, err = identityStructSHA256(originCandidateIdentity{sourceID, decision.OpenT, anchors[0], anchors[1], side, run.SourcePineSHA256, run.TimeframeMS})
	if err != nil {
		return "", "", err
	}
	return episode, candidate, nil
}

func buildIdentity() BuildIdentity {
	info, ok := debug.ReadBuildInfo()
	return identityFromBuildInfo(runtime.Version(), runtime.Compiler, runtime.GOOS, runtime.GOARCH, info, ok)
}

func identityFromBuildInfo(version, compiler, goos, goarch string, info *debug.BuildInfo, ok bool) BuildIdentity {
	identity := BuildIdentity{GoVersion: version, Compiler: compiler, GOOS: goos, GOARCH: goarch}
	if !ok || info == nil {
		return identity
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			identity.VCSRevision = setting.Value
		case "vcs.modified":
			if setting.Value == "true" || setting.Value == "false" {
				modified := setting.Value == "true"
				identity.VCSModified = &modified
			}
		}
	}
	// Verified means a complete artifact VCS stamp, never a borrowed checkout
	// identity or proof of qualification. Incomplete build metadata stays explicit.
	identity.Verified = lowercaseHexOfLength(identity.VCSRevision, 40) && identity.VCSModified != nil
	return identity
}

// The builder must call this before emitting buildIdentity(). Dynamic identity
// values are retained truthfully; an overlong/invalid stamp is rejected, never
// truncated, replaced with a borrowed revision or silently certified.
func validateBuildIdentity(identity BuildIdentity) error {
	for _, field := range []struct {
		value string
		limit int
	}{{identity.GoVersion, 64}, {identity.Compiler, 64}, {identity.GOOS, 16}, {identity.GOARCH, 16}} {
		if len(field.value) > field.limit {
			return newCoreError("resource_limit", "identity", -1, -1, -1, "runtime identity exceeds its byte bound")
		}
		if field.value == "" || !utf8.ValidString(field.value) {
			return newCoreError("internal_error", "identity", -1, -1, -1, "runtime identity is invalid")
		}
	}
	if len(identity.VCSRevision) > 40 {
		return newCoreError("resource_limit", "identity", -1, -1, -1, "VCS revision exceeds its byte bound")
	}
	if identity.VCSRevision != "" && !lowercaseHexOfLength(identity.VCSRevision, 40) {
		return newCoreError("internal_error", "identity", -1, -1, -1, "VCS revision is invalid")
	}
	if identity.Verified && (identity.VCSRevision == "" || identity.VCSModified == nil) {
		return newCoreError("internal_error", "identity", -1, -1, -1, "verified build identity requires a complete VCS stamp")
	}
	return nil
}
