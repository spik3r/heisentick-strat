package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
	"github.com/spik3r/heisentick-strat/report/adaptiveflagunit"
)

const adaptiveUnitProjectionMetadata = `{"schema":"adaptive-flag-unit-request-v1","scenario":"UNIT_POINT_VALUE_1","numericalPolicy":"BINARY64_ORDERED_V1","costPolicy":"RAW","dataSource":{"id":"native-fixture/v1","sourceSha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}`

func adaptiveUnitCLIInputs(t *testing.T) (args []string, requestPath, projectionPath, sourcePath, barsPath string, source, data []byte) {
	t.Helper()
	args, requestPath, sourcePath, barsPath, source, data = adaptiveRuntimeCLIInputs(t)
	projectionPath = filepath.Join(t.TempDir(), "projection.json")
	if err := os.WriteFile(projectionPath, []byte(adaptiveUnitProjectionMetadata), 0600); err != nil {
		t.Fatal(err)
	}
	args = append(args, "--projection-file="+projectionPath)
	return
}

func adaptiveUnitCLIError(t *testing.T, err error, output []byte, code string) {
	t.Helper()
	if err == nil {
		t.Fatal("unit report unexpectedly succeeded")
	}
	if err.Error() != code {
		t.Fatalf("native stderr diagnostic is not the stable code: %v", err)
	}
	if len(output) > adaptiveflagunit.MaxErrorBytes || !utf8.Valid(output) {
		t.Fatalf("not one complete closed native error: %q (%v)", output, err)
	}
	var got adaptiveflagunit.ErrorEnvelope
	if e := json.Unmarshal(output, &got); e != nil || got.Schema != adaptiveflagunit.ErrorSchema || got.Error.Code != code {
		t.Fatalf("wrong closed native error: %s (%v)", output, e)
	}
	compact, e := json.Marshal(got)
	if e != nil || !bytes.Equal(output, append(compact, '\n')) {
		t.Fatalf("not the exact closed native error shape and newline: %q", output)
	}
}

func TestAdaptiveFlagUnitCLISharedReportExactBytes(t *testing.T) {
	args, rp, pp, sp, _, source, data := adaptiveUnitCLIInputs(t)
	for _, policy := range []string{"RAW", "RAZOR_PROXY_BASIC", "RAZOR_PROXY_HARSH_AGGREGATE"} {
		t.Run(policy, func(t *testing.T) {
			projection := strings.Replace(adaptiveUnitProjectionMetadata, `"RAW"`, `"`+policy+`"`, 1)
			projection += strings.Repeat(" ", adaptiveflagunit.MaxProjectionMetadataBytes-len(projection))
			metadata := adaptiveRuntimeMetadata + strings.Repeat(" ", adaptiveflag.MaxMetadataBytes-len(adaptiveRuntimeMetadata))
			sourceText := string(source) + strings.Repeat(" ", dsl.AdaptiveFlagRuntimeMaxSourceBytes-len(source))
			for path, contents := range map[string]string{rp: metadata, pp: projection, sp: sourceText} {
				if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
					t.Fatal(err)
				}
			}
			prepared, err := adaptiveflagunit.PrepareRuntime(metadata, projection, sourceText)
			if err != nil {
				t.Fatal(err)
			}
			want, err := prepared.Build(data)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err = run(append([]string{"adaptive-flag-unit-report"}, args...), &out); err != nil {
				t.Fatal(err, out.String())
			}
			if !bytes.Equal(out.Bytes(), want) || !bytes.HasSuffix(out.Bytes(), []byte("\n")) || !json.Valid(out.Bytes()) {
				t.Fatal("native output differs from the shared complete report")
			}
			raw, err := adaptiveflag.BuildRuntime(metadata, sourceText, data)
			if err != nil || !bytes.Contains(out.Bytes(), append([]byte(`"raw":`), raw...)) {
				t.Fatal("native output changed the complete Stage A raw byte segment", err)
			}
		})
	}
}

func TestAdaptiveFlagUnitCLIClosedFlags(t *testing.T) {
	args, _, _, _, _, _, _ := adaptiveUnitCLIInputs(t)
	for _, extra := range []string{
		"--equity=1", "--quantity=1", "--costs=RAW", "--include-registry=false", "--research-ablation=G1", "--trade-from=0", "--help", "positional", "--", "--=value", "--projection-file=", "--projection-file", args[0], args[1], args[2], args[3],
	} {
		var out bytes.Buffer
		err := runAdaptiveFlagUnitReport(append(append([]string{}, args...), extra), &out)
		adaptiveUnitCLIError(t, err, out.Bytes(), "projection_request_rejected")
	}
	for missing := range args {
		var out bytes.Buffer
		input := append(append([]string{}, args[:missing]...), args[missing+1:]...)
		adaptiveUnitCLIError(t, runAdaptiveFlagUnitReport(input, &out), out.Bytes(), "projection_request_rejected")
	}
	var separate []string
	for _, arg := range args {
		key, value, _ := strings.Cut(arg, "=")
		separate = append(separate, key, value)
	}
	if err := runAdaptiveFlagUnitReport(separate, io.Discard); err != nil {
		t.Fatal("separate explicit file flag spelling rejected", err)
	}
}

func TestAdaptiveFlagUnitCLIRejectsBeforeBarsOpenAndRecovers(t *testing.T) {
	args, rp, pp, sp, bp, source, data := adaptiveUnitCLIInputs(t)
	cases := []struct{ raw, projection, source, code string }{
		{"{}", adaptiveUnitProjectionMetadata, string(source), "raw_request_rejected"},
		{adaptiveRuntimeMetadata, "{}", string(source), "projection_request_rejected"},
		{adaptiveRuntimeMetadata, adaptiveUnitProjectionMetadata, "", "raw_source_rejected"},
		{adaptiveRuntimeMetadata, adaptiveUnitProjectionMetadata, "dsl v7", "raw_source_rejected"},
		{adaptiveRuntimeMetadata, adaptiveUnitProjectionMetadata, strings.Replace(string(source), "bundle INITIAL", "bundle CUSTOM", 1), "raw_source_rejected"},
		{adaptiveRuntimeMetadata, adaptiveUnitProjectionMetadata, strings.Replace(string(source), "timeframe M30", "timeframe H1", 1), "raw_source_rejected"},
		{adaptiveRuntimeMetadata, strings.Replace(adaptiveUnitProjectionMetadata, `"costPolicy":"RAW"`, `"costPolicy":"RAW","cost\u0050olicy":"RAW"`, 1), string(source), "projection_request_rejected"},
		{adaptiveRuntimeMetadata, adaptiveUnitProjectionMetadata + "\xff", string(source), "projection_request_rejected"},
		{adaptiveRuntimeMetadata + strings.Repeat(" ", 4097-len(adaptiveRuntimeMetadata)), adaptiveUnitProjectionMetadata, string(source), "resource_limit"},
		{adaptiveRuntimeMetadata, adaptiveUnitProjectionMetadata + strings.Repeat(" ", 1025-len(adaptiveUnitProjectionMetadata)), string(source), "resource_limit"},
		{adaptiveRuntimeMetadata, adaptiveUnitProjectionMetadata, string(source) + strings.Repeat(" ", 4097-len(source)), "resource_limit"},
	}
	for i, tc := range cases {
		if err := os.Remove(bp); err != nil {
			t.Fatal(err)
		}
		for path, contents := range map[string]string{rp: tc.raw, pp: tc.projection, sp: tc.source} {
			if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
		}
		var out bytes.Buffer
		err := runAdaptiveFlagUnitReport(args, &out)
		adaptiveUnitCLIError(t, err, out.Bytes(), tc.code)
		if strings.Contains(out.String(), bp) {
			t.Fatalf("case %d opened missing bars before admission", i)
		}
		for path, contents := range map[string][]byte{rp: []byte(adaptiveRuntimeMetadata), pp: []byte(adaptiveUnitProjectionMetadata), sp: source, bp: data} {
			if err := os.WriteFile(path, contents, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := runAdaptiveFlagUnitReport(args, io.Discard); err != nil {
			t.Fatalf("case %d poisoned the next valid call: %v", i, err)
		}
	}
}

func TestAdaptiveFlagUnitCLIBoundedRoleReads(t *testing.T) {
	for _, role := range []struct {
		maximum int
		code    string
	}{
		{adaptiveflag.MaxMetadataBytes, "raw_request_rejected"},
		{adaptiveflagunit.MaxProjectionMetadataBytes, "projection_request_rejected"},
		{dsl.AdaptiveFlagRuntimeMaxSourceBytes, "raw_source_rejected"},
	} {
		for _, size := range []int{role.maximum - 1, role.maximum, role.maximum + 1, role.maximum * 4} {
			r := &adaptiveRuntimeCountReader{reader: strings.NewReader(strings.Repeat("x", size))}
			data, err := readAdaptiveFlagUnitBounded(r, role.maximum, role.code)
			if r.read > role.maximum+1 || (size <= role.maximum && (err != nil || len(data) != size)) || (size > role.maximum && (err == nil || data != nil)) {
				t.Fatal("maximum+1 read bound failed", role, size, r.read, err)
			}
			if err != nil && err.Error() != "resource_limit" {
				t.Fatal("oversized file did not preserve resource code", err)
			}
		}
		if _, err := readAdaptiveFlagUnitBounded(adaptiveRuntimeFailReader{}, role.maximum, role.code); err == nil || err.Error() != role.code {
			t.Fatal("read error lost file role", role, err)
		}
	}
}

func TestAdaptiveFlagUnitCLIInputFileFailures(t *testing.T) {
	for index, code := range []string{"raw_request_rejected", "raw_source_rejected", "raw_input_rejected", "projection_request_rejected"} {
		args, _, _, _, _, _, _ := adaptiveUnitCLIInputs(t)
		_, path, _ := strings.Cut(args[index], "=")
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		err := runAdaptiveFlagUnitReport(args, &out)
		adaptiveUnitCLIError(t, err, out.Bytes(), code)
	}
	args, _, _, _, bp, _, data := adaptiveUnitCLIInputs(t)
	for _, malformed := range [][]byte{data[:15], data[:len(data)-1], append(append([]byte{}, data...), 0)} {
		if err := os.WriteFile(bp, malformed, 0600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		adaptiveUnitCLIError(t, runAdaptiveFlagUnitReport(args, &out), out.Bytes(), "raw_input_rejected")
	}
}

type adaptiveUnitOutputWriter struct {
	buffer bytes.Buffer
	mode   string
	calls  int
}

func (w *adaptiveUnitOutputWriter) Write(data []byte) (int, error) {
	w.calls++
	switch w.mode {
	case "fail":
		return 0, errors.New("invented stdout failure with private payload")
	case "short":
		return w.buffer.Write(data[:len(data)/2])
	case "partial-error":
		n, _ := w.buffer.Write(data[:len(data)/2])
		return n, errors.New("invented partial stdout failure")
	case "panic":
		panic("invented panic with private payload")
	}
	return w.buffer.Write(data)
}

func TestAdaptiveFlagUnitCLIOutputFailures(t *testing.T) {
	args, _, _, _, _, _, _ := adaptiveUnitCLIInputs(t)
	for _, input := range [][]string{args, append(append([]string{}, args...), "--unknown=true")} {
		for _, mode := range []string{"fail", "short", "partial-error", "panic"} {
			writer := &adaptiveUnitOutputWriter{mode: mode}
			err := runAdaptiveFlagUnitReport(input, writer)
			code := "raw_execution_rejected"
			if mode == "panic" {
				code = "internal_error"
			}
			if err == nil || err.Error() != code || writer.calls != 1 {
				t.Fatal("stdout failure was successful or retried", mode, err, writer.calls)
			}
			if strings.Contains(adaptiveflagunit.ErrorJSON(err), "private payload") || strings.Contains(adaptiveflagunit.ErrorJSON(err), "invented") {
				t.Fatal("writer failure exposed uncontrolled message", err)
			}
		}
	}
	if err := runAdaptiveFlagUnitReport(args, io.Discard); err != nil {
		t.Fatal("stdout failure poisoned a later valid call", err)
	}
}

func TestAdaptiveFlagUnitCLIProcessExit(t *testing.T) {
	if os.Getenv("HEISENTICK_UNIT_NATIVE_TEST_CHILD") == "1" {
		for i, arg := range os.Args {
			if arg == "--" {
				os.Args = append([]string{"heisentick"}, os.Args[i+1:]...)
				main()
				os.Exit(0)
			}
		}
		os.Exit(2)
	}
	args, _, _, _, _, _, _ := adaptiveUnitCLIInputs(t)
	for _, valid := range []bool{false, true} {
		input := append([]string{"-test.run=^TestAdaptiveFlagUnitCLIProcessExit$", "--", "adaptive-flag-unit-report"}, args...)
		if !valid {
			input = append(input, "--unapproved=true")
		}
		command := exec.Command(os.Args[0], input...)
		command.Env = append(os.Environ(), "HEISENTICK_UNIT_NATIVE_TEST_CHILD=1")
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		if !json.Valid(stdout.Bytes()) || !bytes.HasSuffix(stdout.Bytes(), []byte("\n")) {
			t.Fatal("process did not emit complete JSON", valid, err, stdout.String(), stderr.String())
		}
		if valid {
			if err != nil || stderr.Len() != 0 || !bytes.Contains(stdout.Bytes(), []byte(`"schema":"strat-adaptive-volume-flag-unit-runtime-v1"`)) {
				t.Fatal("valid process failed", err, stderr.String())
			}
		} else {
			var exitError *exec.ExitError
			if !errors.As(err, &exitError) || exitError.ExitCode() == 0 || stderr.String() != "heisentick: projection_request_rejected\n" {
				t.Fatal("rejected process lacks nonzero exit and stable diagnostic", err, stderr.String())
			}
			if !bytes.Contains(stdout.Bytes(), []byte(`"schema":"adaptive-flag-unit-error-v1"`)) {
				t.Fatal("nonzero process output must only be an error document")
			}
		}
	}
}
