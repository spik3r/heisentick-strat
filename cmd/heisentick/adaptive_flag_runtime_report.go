package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
)

func runAdaptiveFlagRuntimeReport(args []string, out io.Writer) error {
	flags, err := parseTimedReportFlags(args)
	if err != nil {
		return adaptiveflag.Rejection("request", err.Error())
	}
	allowed := map[string]bool{"request-file": true, "dsl-file": true, "bars-file": true}
	for key, values := range flags {
		if !allowed[key] || len(values) != 1 {
			return adaptiveflag.Rejection("request", fmt.Sprintf("adaptive-flag-runtime-report rejects unknown/repeated --%s", key))
		}
	}
	for _, key := range []string{"request-file", "dsl-file", "bars-file"} {
		if _, err = flags.required(key); err != nil {
			return adaptiveflag.Rejection("request", err.Error())
		}
	}
	metadata, err := readAdaptiveFlagRuntimeFile(flags.one("request-file", ""), adaptiveflag.MaxMetadataBytes, "request")
	if err != nil {
		return err
	}
	source, err := readAdaptiveFlagRuntimeFile(flags.one("dsl-file", ""), dsl.AdaptiveFlagRuntimeMaxSourceBytes, "source")
	if err != nil {
		return err
	}
	// The shared preparation admits the complete strict ParseResult, decoder,
	// named profile and timeframe before the bars file is even opened.
	prepared, err := adaptiveflag.PrepareRuntime(string(metadata), string(source))
	if err != nil {
		return err
	}
	data, err := readAdaptiveFlagRuntimeBarsFile(flags.one("bars-file", ""))
	if err != nil {
		return err
	}
	raw, err := prepared.Build(data)
	if err != nil {
		return err
	}
	// Build validates and serializes the complete report before any success bytes.
	if _, err = out.Write(raw); err != nil {
		return adaptiveflag.Rejection("execution", err.Error())
	}
	return nil
}

func readAdaptiveFlagRuntimeFile(path string, max int, phase string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, adaptiveflag.Rejection(phase, err.Error())
	}
	defer file.Close()
	return readAdaptiveFlagRuntimeBounded(file, max, phase)
}

func readAdaptiveFlagRuntimeBounded(reader io.Reader, max int, phase string) ([]byte, error) {
	// A stat check alone does not bound streams or a concurrently growing file.
	data, err := io.ReadAll(io.LimitReader(reader, int64(max)+1))
	if err != nil {
		return nil, adaptiveflag.Rejection(phase, err.Error())
	}
	if len(data) > max {
		return nil, adaptiveflag.Rejection("resource", fmt.Sprintf("adaptive flag runtime %s exceeds %d bytes", phase, max))
	}
	return data, nil
}

func readAdaptiveFlagRuntimeBarsFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, adaptiveflag.Rejection("input", err.Error())
	}
	defer file.Close()
	return readAdaptiveFlagRuntimeBars(file)
}

func readAdaptiveFlagRuntimeBars(reader io.Reader) ([]byte, error) {
	var header [16]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, adaptiveflag.Rejection("input", fmt.Sprintf("read adaptive flag BTB1 header: %v", err))
	}
	// Compute in uint64 and bound before conversion to int on any architecture.
	// Shape/header admission precedes the full bounded transport allocation.
	count := uint64(binary.LittleEndian.Uint32(header[8:12]))
	expected := uint64(len(header)) + count*6*8
	if expected > adaptiveflag.MaxInputBytes {
		return nil, adaptiveflag.Rejection("resource", "adaptive flag runtime BTB1 exceeds supplied-row limit")
	}
	if err := adaptiveflag.ValidateRuntimeHeader(header[:], int(expected)); err != nil {
		return nil, err
	}
	remaining := int(expected) - len(header)
	body, err := io.ReadAll(io.LimitReader(reader, int64(remaining)+1))
	if err != nil {
		return nil, adaptiveflag.Rejection("input", err.Error())
	}
	if len(body) != remaining {
		return nil, adaptiveflag.Rejection("input", "adaptive flag BTB1 exact byte length mismatch")
	}
	data := make([]byte, int(expected))
	copy(data, header[:])
	copy(data[len(header):], body)
	return data, nil
}
