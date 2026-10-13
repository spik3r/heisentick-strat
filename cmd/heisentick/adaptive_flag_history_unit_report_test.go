//go:build !js && !wasm

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
	"github.com/spik3r/heisentick-strat/report/adaptiveflagunit"
)

func adaptiveHistoryCLIInputs(t *testing.T) (args []string, request, projection, sourcePath, barsPath string, source, bars []byte) {
	t.Helper()
	args, request, projection, sourcePath, barsPath, source, bars = adaptiveUnitCLIInputs(t)
	args = append(args, "--profile="+adaptiveflag.HistoryProfile)
	return
}

func TestAdaptiveFlagHistoryCLISharedCompleteBytes(t *testing.T) {
	args, rp, pp, sp, _, source, bars := adaptiveHistoryCLIInputs(t)
	for _, cost := range []string{"RAW", "RAZOR_PROXY_BASIC", "RAZOR_PROXY_HARSH_AGGREGATE"} {
		t.Run(cost, func(t *testing.T) {
			raw := adaptiveRuntimeMetadata + strings.Repeat(" ", adaptiveflag.MaxMetadataBytes-len(adaptiveRuntimeMetadata))
			projection := strings.Replace(adaptiveUnitProjectionMetadata, `"RAW"`, `"`+cost+`"`, 1)
			projection += strings.Repeat(" ", adaptiveflagunit.MaxProjectionMetadataBytes-len(projection))
			text := string(source) + strings.Repeat(" ", dsl.AdaptiveFlagRuntimeMaxSourceBytes-len(source))
			for path, data := range map[string]string{rp: raw, pp: projection, sp: text} {
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := adaptiveflagunit.BuildHistory(adaptiveflag.HistoryProfile, raw, projection, text, bars)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err = run(append([]string{"adaptive-flag-history-unit-report"}, args...), &out); err != nil {
				t.Fatal(err, out.String())
			}
			if !bytes.Equal(want, out.Bytes()) || !json.Valid(out.Bytes()) || !bytes.HasSuffix(out.Bytes(), []byte("\n")) {
				t.Fatal("CLI changed complete shared builder bytes")
			}
			var doc map[string]json.RawMessage
			if err = json.Unmarshal(out.Bytes(), &doc); err != nil {
				t.Fatal(err)
			}
			if string(doc["schema"]) != `"strat-adaptive-volume-flag-unit-history-v1"` || string(doc["profile"]) != `"adaptive-history-v1"` || string(doc["rawEnvelopeSchema"]) != `"strat-adaptive-volume-flag-history-raw-v1"` || doc["raw"] != nil {
				t.Fatal("history envelope must be distinct and must not embed a duplicate raw report")
			}
		})
	}
}

func TestAdaptiveFlagHistoryCLIClosedFlags(t *testing.T) {
	args, _, _, _, _, _, _ := adaptiveHistoryCLIInputs(t)
	extra := []string{"--max-rows=70001", "--max-days=2201", "--max-output=1", "--equity=1", "--quantity=1", "--costs=RAW", "--include-registry=false", "--research-ablation=G1", "--trade-from=0", "--help", "positional", "--", "--=value", "--profile=", "--profile", "--profile=ADAPTIVE-HISTORY-V1"}
	extra = append(extra, args...)
	for _, value := range extra {
		var out bytes.Buffer
		err := runAdaptiveFlagHistoryUnitReport(append(append([]string{}, args...), value), &out)
		adaptiveUnitCLIError(t, err, out.Bytes(), "projection_request_rejected")
	}
	for i := range args {
		var out bytes.Buffer
		input := append(append([]string{}, args[:i]...), args[i+1:]...)
		err := runAdaptiveFlagHistoryUnitReport(input, &out)
		adaptiveUnitCLIError(t, err, out.Bytes(), "projection_request_rejected")
	}
	for _, profile := range []string{"adaptive-history-v2", "ADAPTIVE-HISTORY-V1", " adaptive-history-v1", "adaptive-history-v1 "} {
		input := append([]string{}, args...)
		input[len(input)-1] = "--profile=" + profile
		var out bytes.Buffer
		err := runAdaptiveFlagHistoryUnitReport(input, &out)
		adaptiveUnitCLIError(t, err, out.Bytes(), "projection_request_rejected")
	}
	var separate []string
	for _, arg := range args {
		key, value, _ := strings.Cut(arg, "=")
		separate = append(separate, key, value)
	}
	if err := runAdaptiveFlagHistoryUnitReport(separate, io.Discard); err != nil {
		t.Fatal("explicit separate flag values refused", err)
	}
}

func TestAdaptiveFlagHistoryCLIAdmissionBeforeBars(t *testing.T) {
	args, rp, pp, sp, bp, source, bars := adaptiveHistoryCLIInputs(t)
	cases := []struct{ raw, projection, source, code string }{
		{"{}", adaptiveUnitProjectionMetadata, string(source), "raw_request_rejected"},
		{adaptiveRuntimeMetadata, "{}", string(source), "projection_request_rejected"},
		{adaptiveRuntimeMetadata, adaptiveUnitProjectionMetadata, "", "raw_source_rejected"},
		{adaptiveRuntimeMetadata, adaptiveUnitProjectionMetadata, strings.Replace(string(source), "bundle INITIAL", "bundle CUSTOM", 1), "raw_source_rejected"},
		{adaptiveRuntimeMetadata, adaptiveUnitProjectionMetadata, strings.Replace(string(source), "timeframe M30", "timeframe H1", 1), "raw_source_rejected"},
		{adaptiveRuntimeMetadata, strings.Replace(adaptiveUnitProjectionMetadata, `"RAW"`, `"BASIC"`, 1), string(source), "projection_request_rejected"},
		{adaptiveRuntimeMetadata, strings.Replace(adaptiveUnitProjectionMetadata, `"costPolicy":"RAW"`, `"costPolicy":"RAW","cost\u0050olicy":"RAW"`, 1), string(source), "projection_request_rejected"},
		{adaptiveRuntimeMetadata + strings.Repeat(" ", 4097-len(adaptiveRuntimeMetadata)), adaptiveUnitProjectionMetadata, string(source), "resource_limit"},
		{adaptiveRuntimeMetadata, adaptiveUnitProjectionMetadata + strings.Repeat(" ", 1025-len(adaptiveUnitProjectionMetadata)), string(source), "resource_limit"},
		{adaptiveRuntimeMetadata, adaptiveUnitProjectionMetadata, string(source) + strings.Repeat(" ", 4097-len(source)), "resource_limit"},
		{strings.Replace(adaptiveRuntimeMetadata, "null", `{"tradeFromMs":0,"tradeToMs":190166400000}`, 1), adaptiveUnitProjectionMetadata, string(source), "resource_limit"}, // 2,201 UTC days.
	}
	for i, tc := range cases {
		if err := os.Remove(bp); err != nil {
			t.Fatal(err)
		}
		for path, data := range map[string]string{rp: tc.raw, pp: tc.projection, sp: tc.source} {
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
		}
		var out bytes.Buffer
		err := runAdaptiveFlagHistoryUnitReport(args, &out)
		adaptiveUnitCLIError(t, err, out.Bytes(), tc.code)
		if strings.Contains(out.String(), bp) {
			t.Fatalf("case %d disclosed unopened bars path", i)
		}
		for path, data := range map[string][]byte{rp: []byte(adaptiveRuntimeMetadata), pp: []byte(adaptiveUnitProjectionMetadata), sp: source, bp: bars} {
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := runAdaptiveFlagHistoryUnitReport(args, io.Discard); err != nil {
			t.Fatalf("case %d poisoned later valid invocation: %v", i, err)
		}
	}
	// Profile admission precedes even metadata opens.
	for _, path := range []string{rp, pp, sp, bp} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	args[len(args)-1] = "--profile=unknown"
	var out bytes.Buffer
	err := runAdaptiveFlagHistoryUnitReport(args, &out)
	adaptiveUnitCLIError(t, err, out.Bytes(), "projection_request_rejected")
}

func TestAdaptiveFlagHistoryBTB1HeaderFirstExactRead(t *testing.T) {
	_, _, _, data := adaptiveCLIInputs(t)
	for _, mutate := range []func([]byte){
		func(b []byte) { b[0] = 'X' },
		func(b []byte) { binary.LittleEndian.PutUint32(b[4:8], 2) },
		func(b []byte) { binary.LittleEndian.PutUint32(b[8:12], 0) },
		func(b []byte) { binary.LittleEndian.PutUint32(b[8:12], adaptiveflag.HistoryMaxRows+1) },
		func(b []byte) { binary.LittleEndian.PutUint32(b[8:12], ^uint32(0)) },
		func(b []byte) { binary.LittleEndian.PutUint32(b[12:16], 7) },
	} {
		header := append([]byte{}, data[:16]...)
		mutate(header)
		r := &adaptiveRuntimeCountReader{reader: io.MultiReader(bytes.NewReader(header), adaptiveRuntimeFailReader{})}
		got, err := readAdaptiveFlagHistoryBars(r)
		if err == nil || got != nil || r.read != 16 || strings.Contains(err.Error(), "invented read failure") {
			t.Fatal("invalid native header reached payload read", r.read, err)
		}
	}
	for _, input := range [][]byte{data[:15], data[:len(data)-1], append(append([]byte{}, data...), 0), append(append([]byte{}, data...), make([]byte, 4096)...)} {
		r := &adaptiveRuntimeCountReader{reader: bytes.NewReader(input)}
		got, err := readAdaptiveFlagHistoryBars(r)
		if err == nil || got != nil || r.read > len(data)+1 {
			t.Fatal("malformed transport escaped exact size read", len(input), r.read, err)
		}
	}
	maximum := make([]byte, adaptiveflag.HistoryMaxInputBytes)
	copy(maximum, data[:16])
	binary.LittleEndian.PutUint32(maximum[8:12], adaptiveflag.HistoryMaxRows)
	for _, input := range [][]byte{data, maximum} {
		got, err := readAdaptiveFlagHistoryBars(bytes.NewReader(input))
		if err != nil || !bytes.Equal(got, input) {
			t.Fatal("exact admitted input bytes changed", len(input), err)
		}
	}
}

func TestAdaptiveFlagHistoryCLIOutputFailures(t *testing.T) {
	args, _, _, _, _, _, _ := adaptiveHistoryCLIInputs(t)
	for _, input := range [][]string{args, append(append([]string{}, args...), "--unknown=true")} {
		for _, mode := range []string{"fail", "short", "partial-error", "panic"} {
			writer := &adaptiveUnitOutputWriter{mode: mode}
			err := runAdaptiveFlagHistoryUnitReport(input, writer)
			code := "raw_execution_rejected"
			if mode == "panic" {
				code = "internal_error"
			}
			if err == nil || err.Error() != code || writer.calls != 1 {
				t.Fatal("partial or failed output accepted/retried", mode, err, writer.calls)
			}
			if strings.Contains(adaptiveflagunit.ErrorJSON(err), "private payload") || strings.Contains(adaptiveflagunit.ErrorJSON(err), "invented") {
				t.Fatal("uncontrolled writer error exposed", err)
			}
		}
	}
	if err := runAdaptiveFlagHistoryUnitReport(args, io.Discard); err != nil {
		t.Fatal("output failure poisoned next invocation", err)
	}
}

func TestAdaptiveFlagHistoryCLIInputFileFailures(t *testing.T) {
	for index, code := range []string{"raw_request_rejected", "raw_source_rejected", "raw_input_rejected", "projection_request_rejected"} {
		args, _, _, _, _, _, _ := adaptiveHistoryCLIInputs(t)
		_, path, _ := strings.Cut(args[index], "=")
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		err := runAdaptiveFlagHistoryUnitReport(args, &out)
		adaptiveUnitCLIError(t, err, out.Bytes(), code)
	}
}

func TestAdaptiveFlagHistoryCLIProcessExit(t *testing.T) {
	if os.Getenv("HEISENTICK_HISTORY_NATIVE_TEST_CHILD") == "1" {
		for i, arg := range os.Args {
			if arg == "--" {
				os.Args = append([]string{"heisentick"}, os.Args[i+1:]...)
				main()
				os.Exit(0)
			}
		}
		os.Exit(2)
	}
	args, _, _, _, _, _, _ := adaptiveHistoryCLIInputs(t)
	for _, valid := range []bool{false, true} {
		input := append([]string{"-test.run=^TestAdaptiveFlagHistoryCLIProcessExit$", "--", "adaptive-flag-history-unit-report"}, args...)
		if !valid {
			input = append(input, "--unapproved=true")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		command := exec.CommandContext(ctx, os.Args[0], input...)
		command.Env = append(os.Environ(), "HEISENTICK_HISTORY_NATIVE_TEST_CHILD=1")
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		cancel()
		if !json.Valid(stdout.Bytes()) || !bytes.HasSuffix(stdout.Bytes(), []byte("\n")) {
			t.Fatal("process emitted incomplete document", valid, err, stdout.String(), stderr.String())
		}
		if valid {
			if err != nil || stderr.Len() != 0 || !bytes.Contains(stdout.Bytes(), []byte(`"schema":"strat-adaptive-volume-flag-unit-history-v1"`)) {
				t.Fatal("valid native process failed", err, stderr.String())
			}
		} else {
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() == 0 || stderr.String() != "heisentick: projection_request_rejected\n" {
				t.Fatal("rejected native process lost stable nonzero diagnostic", err, stderr.String())
			}
		}
	}
}
