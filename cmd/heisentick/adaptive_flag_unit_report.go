package main

import (
	"encoding/json"
	"io"
	"os"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
	"github.com/spik3r/heisentick-strat/report/adaptiveflagunit"
)

func runAdaptiveFlagUnitReport(args []string, out io.Writer) (err error) {
	outputStarted := false
	defer func() {
		if recover() != nil {
			err = adaptiveflagunit.Rejection("internal_error", "", "adaptive unit runtime failed")
		}
		if err == nil {
			return
		}
		closed := adaptiveflagunit.ErrorJSON(err)
		var wire adaptiveflagunit.ErrorEnvelope
		if json.Unmarshal([]byte(closed), &wire) != nil || wire.Error.Code == "" {
			err = adaptiveflagunit.Rejection("internal_error", "", "adaptive unit runtime failed")
			closed = adaptiveflagunit.ErrorJSON(err)
			wire.Error.Code = "internal_error"
		}
		// Preserve the original cause while exposing only its already-mapped
		// stable code to main's stderr diagnostic. Emit the rendered bytes above.
		err = &adaptiveUnitNativeDiagnostic{code: wire.Error.Code, cause: err}
		if !outputStarted {
			// An admitted failure is one complete closed document. If stdout has
			// already failed, never append a second document to its partial prefix.
			outputStarted = true
			if writeErr := writeAdaptiveFlagUnitOutput(out, []byte(closed+"\n")); writeErr != nil {
				err = writeErr
			}
		}
	}()

	flags, err := parseTimedReportFlags(args)
	if err != nil {
		return adaptiveflagunit.Rejection("projection_request_rejected", "native_io", "unit report requires explicit nonempty file flags")
	}
	allowed := map[string]bool{"request-file": true, "projection-file": true, "dsl-file": true, "bars-file": true}
	for key, values := range flags {
		if !allowed[key] || len(values) != 1 {
			return adaptiveflagunit.Rejection("projection_request_rejected", "native_io", "unit report rejects unknown or repeated flags")
		}
	}
	for _, key := range []string{"request-file", "projection-file", "dsl-file", "bars-file"} {
		if _, err = flags.required(key); err != nil {
			return adaptiveflagunit.Rejection("projection_request_rejected", "native_io", "unit report requires all four file flags")
		}
	}
	rawMetadata, err := readAdaptiveFlagUnitFile(flags.one("request-file", ""), adaptiveflag.MaxMetadataBytes, "raw_request_rejected")
	if err != nil {
		return err
	}
	projectionMetadata, err := readAdaptiveFlagUnitFile(flags.one("projection-file", ""), adaptiveflagunit.MaxProjectionMetadataBytes, "projection_request_rejected")
	if err != nil {
		return err
	}
	source, err := readAdaptiveFlagUnitFile(flags.one("dsl-file", ""), dsl.AdaptiveFlagRuntimeMaxSourceBytes, "raw_source_rejected")
	if err != nil {
		return err
	}
	// Both metadata documents and complete source admission precede even the
	// bars file open. The existing Stage A bounded BTB1 reader stays unchanged.
	prepared, err := adaptiveflagunit.PrepareRuntime(string(rawMetadata), string(projectionMetadata), string(source))
	if err != nil {
		return err
	}
	data, err := readAdaptiveFlagRuntimeBarsFile(flags.one("bars-file", ""))
	if err != nil {
		return err
	}
	report, err := prepared.Build(data)
	if err != nil {
		return err
	}
	// All validation and serialization finishes before the first success byte.
	outputStarted = true
	return writeAdaptiveFlagUnitOutput(out, report)
}

type adaptiveUnitNativeDiagnostic struct {
	code  string
	cause error
}

func (e *adaptiveUnitNativeDiagnostic) Error() string { return e.code }
func (e *adaptiveUnitNativeDiagnostic) Unwrap() error { return e.cause }

func readAdaptiveFlagUnitFile(path string, maximum int, code string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, adaptiveflagunit.Rejection(code, "native_io", "cannot open unit report input file")
	}
	defer file.Close()
	return readAdaptiveFlagUnitBounded(file, maximum, code)
}

func readAdaptiveFlagUnitBounded(reader io.Reader, maximum int, code string) ([]byte, error) {
	// LimitReader also bounds streams and files that grow after opening.
	data, err := io.ReadAll(io.LimitReader(reader, int64(maximum)+1))
	if err != nil {
		return nil, adaptiveflagunit.Rejection(code, "native_io", "cannot read unit report input file")
	}
	if len(data) > maximum {
		return nil, adaptiveflagunit.Rejection("resource_limit", "native_io", "unit report input file exceeds its byte limit")
	}
	return data, nil
}

func writeAdaptiveFlagUnitOutput(out io.Writer, data []byte) (err error) {
	defer func() {
		if recover() != nil {
			err = adaptiveflagunit.Rejection("internal_error", "", "adaptive unit runtime failed")
		}
	}()
	n, writeErr := out.Write(data)
	if writeErr != nil || n != len(data) {
		return adaptiveflagunit.Rejection("raw_execution_rejected", "native_io", "cannot write complete unit report output")
	}
	return nil
}
