//go:build !js && !wasm

package main

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"os"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
	"github.com/spik3r/heisentick-strat/report/adaptiveflagunit"
)

func init() {
	nativeExtraCommands["adaptive-flag-history-unit-report"] = runAdaptiveFlagHistoryUnitReport
	nativeExtraUsage += "\n  heisentick adaptive-flag-history-unit-report --profile=adaptive-history-v1 --request-file=<JSON path> --projection-file=<JSON path> --dsl-file=<path> --bars-file=<six-column BTB1 path>\n\nNative-only continuous research accounting: at most 70,000 supplied rows including all warmup, 2,200 UTC trading date labels and 256 MiB output. Requires an external time/memory/output supervisor; profile selection is not study authorization.\n"
}

func runAdaptiveFlagHistoryUnitReport(args []string, out io.Writer) (err error) {
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
	allowed := map[string]bool{"profile": true, "request-file": true, "projection-file": true, "dsl-file": true, "bars-file": true}
	for key, values := range flags {
		if !allowed[key] || len(values) != 1 {
			return adaptiveflagunit.Rejection("projection_request_rejected", "native_io", "unit report rejects unknown or repeated flags")
		}
	}
	for _, key := range []string{"profile", "request-file", "projection-file", "dsl-file", "bars-file"} {
		if _, err = flags.required(key); err != nil {
			return adaptiveflagunit.Rejection("projection_request_rejected", "native_io", "unit report requires profile and all four file flags")
		}
	}
	if flags.one("profile", "") != adaptiveflag.HistoryProfile {
		return adaptiveflagunit.Rejection("projection_request_rejected", "native_io", "explicit adaptive-history-v1 native profile required")
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
	// bars file open. The original portable Stage A reader stays unchanged.
	prepared, err := adaptiveflagunit.PrepareHistory(flags.one("profile", ""), string(rawMetadata), string(projectionMetadata), string(source))
	if err != nil {
		return err
	}
	data, err := readAdaptiveFlagHistoryBarsFile(flags.one("bars-file", ""))
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

func readAdaptiveFlagHistoryBarsFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, adaptiveflagunit.Rejection("raw_input_rejected", "native_io", "cannot open native history BTB1")
	}
	defer file.Close()
	return readAdaptiveFlagHistoryBars(file)
}

func readAdaptiveFlagHistoryBars(reader io.Reader) ([]byte, error) {
	var header [16]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, adaptiveflagunit.Rejection("raw_input_rejected", "native_io", "cannot read native history BTB1 header")
	}
	count := uint64(binary.LittleEndian.Uint32(header[8:12]))
	expected := uint64(16) + count*48
	if expected > adaptiveflag.HistoryMaxInputBytes {
		return nil, adaptiveflagunit.Rejection("resource_limit", "native_io", "native history BTB1 exceeds supplied-row limit")
	}
	if err := adaptiveflag.ValidateHistoryHeader(header[:], int(expected)); err != nil {
		return nil, err
	}
	data := make([]byte, int(expected))
	copy(data, header[:])
	if _, err := io.ReadFull(reader, data[16:]); err != nil {
		return nil, adaptiveflagunit.Rejection("raw_input_rejected", "native_io", "native history BTB1 exact byte length mismatch")
	}
	var extra [1]byte
	n, err := io.ReadFull(reader, extra[:])
	if n != 0 || err != io.EOF {
		return nil, adaptiveflagunit.Rejection("raw_input_rejected", "native_io", "native history BTB1 exact byte length mismatch")
	}
	return data, nil
}
