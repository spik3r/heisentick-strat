package dsl

import "fmt"

// AdaptiveFlagRuntimeMaxSourceBytes bounds the dedicated runtime source entrypoint.
// The generic parser and existing native runner retain their original admission.
const AdaptiveFlagRuntimeMaxSourceBytes = 4096

// ParseAdaptiveVolumeFlagRuntimeSource admits a bounded source directly to the
// existing strict adaptive parser. A nil error does not imply source admission:
// callers must still inspect Errors, Diagnostics and Config in the ParseResult.
func ParseAdaptiveVolumeFlagRuntimeSource(source string) (ParseResult, error) {
	if len(source) > AdaptiveFlagRuntimeMaxSourceBytes {
		return ParseResult{}, fmt.Errorf("adaptive flag runtime source exceeds %d bytes", AdaptiveFlagRuntimeMaxSourceBytes)
	}
	return parseAdaptiveVolumeFlagSource(source), nil
}
