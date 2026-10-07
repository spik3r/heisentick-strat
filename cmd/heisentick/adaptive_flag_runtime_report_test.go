package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
)

const adaptiveRuntimeMetadata = `{"schema":"adaptive-flag-runtime-request-v1","numericalPolicy":"BINARY64_ORDERED_V1","symbol":"XAUUSD","timeframe":"M30","executionWindow":null}`

func adaptiveRuntimeCLIInputs(t *testing.T) (args []string, requestPath, sourcePath, barsPath string, source, data []byte) {
	t.Helper()
	sourcePath, barsPath, source, data = adaptiveCLIInputs(t)
	requestPath = filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(requestPath, []byte(adaptiveRuntimeMetadata), 0600); err != nil {
		t.Fatal(err)
	}
	args = []string{"--request-file=" + requestPath, "--dsl-file=" + sourcePath, "--bars-file=" + barsPath}
	return
}

func adaptiveRuntimeErrorPhase(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("runtime unexpectedly succeeded")
	}
	var got struct {
		Error struct{ Code, Phase, Message string } `json:"error"`
	}
	if e := json.Unmarshal([]byte(adaptiveflag.ErrorJSON(err)), &got); e != nil {
		t.Fatal(e)
	}
	if got.Error.Code != "ADAPTIVE_RUNTIME_REJECTED" || len(got.Error.Message) > 1024 || !utf8.ValidString(got.Error.Message) {
		t.Fatal("invalid bounded rejection", got)
	}
	return got.Error.Phase
}

func TestAdaptiveFlagRuntimeCLISharedReportExactBytes(t *testing.T) {
	args, rp, _, _, source, data := adaptiveRuntimeCLIInputs(t)
	for _, metadata := range []string{
		adaptiveRuntimeMetadata,
		strings.Replace(adaptiveRuntimeMetadata, "null", `{"tradeFromMs":48600000,"tradeToMs":50400000}`, 1),
		adaptiveRuntimeMetadata + strings.Repeat(" ", adaptiveflag.MaxMetadataBytes-len(adaptiveRuntimeMetadata)),
	} {
		if err := os.WriteFile(rp, []byte(metadata), 0600); err != nil {
			t.Fatal(err)
		}
		want, err := adaptiveflag.BuildRuntime(metadata, string(source), data)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err = run(append([]string{"adaptive-flag-runtime-report"}, args...), &out); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out.Bytes(), want) || !bytes.HasSuffix(out.Bytes(), []byte("\n")) || !json.Valid(out.Bytes()) {
			t.Fatal("native bytes diverge from shared complete report")
		}
		for _, forbidden := range []string{`"researchPolicy"`, `"researchProducer"`, `"qty"`, `"equity"`, `"netPnl"`, `"returnPct"`} {
			if bytes.Contains(out.Bytes(), []byte(forbidden)) {
				t.Fatal("runtime leaked unapproved field", forbidden)
			}
		}
	}
}

func TestAdaptiveFlagRuntimeCLIClosedFlags(t *testing.T) {
	args, _, _, _, _, _ := adaptiveRuntimeCLIInputs(t)
	for _, extra := range []string{
		"--equity=0", "--startEquity=0", "--statistics=0", "--pnl=0", "--returnPct=0", "--costs=0", "--risk=0", "--quantity=0", "--fee=0", "--spread=0", "--slippage=0", "--fillOn=0", "--research-ablation=G1", "--checkpoint=0", "--resume=0", "--sourceTf=M30", "--htfBars=0", "--aggregation=0", "--trade-from=0", "--trade-to=0", "--help", "positional", "--", "--=value", "--request-file=", args[0], args[1], args[2],
	} {
		var out bytes.Buffer
		err := runAdaptiveFlagRuntimeReport(append(append([]string{}, args...), extra), &out)
		if phase := adaptiveRuntimeErrorPhase(t, err); phase != "request" || out.Len() != 0 {
			t.Fatal("invalid flag emitted success or wrong phase", extra, phase)
		}
	}
	for missing := range args {
		var out bytes.Buffer
		input := append(append([]string{}, args[:missing]...), args[missing+1:]...)
		if phase := adaptiveRuntimeErrorPhase(t, runAdaptiveFlagRuntimeReport(input, &out)); phase != "request" || out.Len() != 0 {
			t.Fatal("missing required flag", missing, phase)
		}
	}
	// Both existing explicit flag spellings remain accepted by the new command.
	var separate []string
	for _, arg := range args {
		key, value, _ := strings.Cut(arg, "=")
		separate = append(separate, key, value)
	}
	if err := runAdaptiveFlagRuntimeReport(separate, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestAdaptiveFlagRuntimeCLIExactSourceLimitIdentity(t *testing.T) {
	args, _, sp, _, source, data := adaptiveRuntimeCLIInputs(t)
	input := string(source) + strings.Repeat(" ", dsl.AdaptiveFlagRuntimeMaxSourceBytes-len(source))
	if err := os.WriteFile(sp, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	want, err := adaptiveflag.BuildRuntime(adaptiveRuntimeMetadata, input, data)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = runAdaptiveFlagRuntimeReport(args, &out); err != nil || !bytes.Equal(want, out.Bytes()) {
		t.Fatal("4096-byte admitted source identity changed", err)
	}
}

func TestAdaptiveFlagRuntimeCLIRejectsSourceBeforeBarsOpen(t *testing.T) {
	args, rp, sp, bp, source, _ := adaptiveRuntimeCLIInputs(t)
	if err := os.Remove(bp); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ metadata, source, phase string }{
		{"{}", string(source), "request"},
		{adaptiveRuntimeMetadata, "", "source"},
		{adaptiveRuntimeMetadata, "dsl v7", "source"},
		{adaptiveRuntimeMetadata, string(source) + " setup {}", "source"},
		{adaptiveRuntimeMetadata, strings.Replace(string(source), "bundle INITIAL", "bundle CUSTOM", 1), "source"},
		{adaptiveRuntimeMetadata, strings.Replace(string(source), "timeframe M30", "timeframe H1", 1), "source"},
		{adaptiveRuntimeMetadata, string(source) + strings.Repeat(" ", 4097-len(source)), "resource"},
		{adaptiveRuntimeMetadata + strings.Repeat(" ", 4097-len(adaptiveRuntimeMetadata)), string(source), "resource"},
	}
	for i, tc := range cases {
		if err := os.WriteFile(rp, []byte(tc.metadata), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sp, []byte(tc.source), 0600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		err := runAdaptiveFlagRuntimeReport(args, &out)
		if phase := adaptiveRuntimeErrorPhase(t, err); phase != tc.phase || out.Len() != 0 || strings.Contains(err.Error(), bp) {
			t.Fatalf("case %d read bars before complete source admission: phase=%s error=%v", i, phase, err)
		}
	}
}

type adaptiveRuntimeCountReader struct {
	reader io.Reader
	read   int
}

func (r *adaptiveRuntimeCountReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read += n
	return n, err
}

type adaptiveRuntimeFailReader struct{}

func (adaptiveRuntimeFailReader) Read([]byte) (int, error) {
	return 0, errors.New("invented read failure")
}

func TestAdaptiveFlagRuntimeBoundedReaders(t *testing.T) {
	for _, max := range []int{adaptiveflag.MaxMetadataBytes, dsl.AdaptiveFlagRuntimeMaxSourceBytes} {
		for _, size := range []int{max - 1, max, max + 1, max * 4} {
			r := &adaptiveRuntimeCountReader{reader: strings.NewReader(strings.Repeat("x", size))}
			got, err := readAdaptiveFlagRuntimeBounded(r, max, "source")
			if r.read > max+1 || (size <= max && (err != nil || len(got) != size)) || (size > max && (err == nil || got != nil)) {
				t.Fatal("max+1 read bound broken", max, size, r.read, err)
			}
		}
	}
	if phase := adaptiveRuntimeErrorPhase(t, func() error {
		_, err := readAdaptiveFlagRuntimeBounded(adaptiveRuntimeFailReader{}, 4096, "source")
		return err
	}()); phase != "source" {
		t.Fatal("read error lost phase", phase)
	}
}

func TestAdaptiveFlagRuntimeBTB1HeaderFirstBoundedRead(t *testing.T) {
	_, _, _, data := adaptiveCLIInputs(t)
	for _, mutation := range []func([]byte){
		func(h []byte) { h[0] = 'X' },
		func(h []byte) { binary.LittleEndian.PutUint32(h[4:8], 2) },
		func(h []byte) { binary.LittleEndian.PutUint32(h[8:12], 0) },
		func(h []byte) { binary.LittleEndian.PutUint32(h[8:12], adaptiveflag.MaxProvidedRows+1) },
		func(h []byte) { binary.LittleEndian.PutUint32(h[8:12], ^uint32(0)) },
		func(h []byte) { binary.LittleEndian.PutUint32(h[12:16], 7) },
	} {
		header := append([]byte{}, data[:16]...)
		mutation(header)
		r := &adaptiveRuntimeCountReader{reader: io.MultiReader(bytes.NewReader(header), adaptiveRuntimeFailReader{})}
		got, err := readAdaptiveFlagRuntimeBars(r)
		if err == nil || got != nil || r.read != 16 || strings.Contains(err.Error(), "invented read failure") {
			t.Fatal("invalid header reached body read", r.read, err)
		}
	}
	for _, input := range [][]byte{data[:15], data[:len(data)-1], append(append([]byte{}, data...), 0), append(append([]byte{}, data...), make([]byte, 4096)...)} {
		r := &adaptiveRuntimeCountReader{reader: bytes.NewReader(input)}
		got, err := readAdaptiveFlagRuntimeBars(r)
		if err == nil || got != nil || r.read > len(data)+1 {
			t.Fatal("short/extra input escaped exact bounded read", len(input), r.read, err)
		}
	}
	got, err := readAdaptiveFlagRuntimeBars(bytes.NewReader(data))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("exact supplied bytes not preserved", err)
	}
}

func TestAdaptiveFlagLegacyAdmissionAndErrorOrderUnchanged(t *testing.T) {
	sp, bp, source, _ := adaptiveCLIInputs(t)
	base := []string{"--dsl-file=" + sp, "--bars-file=" + bp}
	longSource := append(append([]byte{}, source...), bytes.Repeat([]byte(" "), 4097-len(source))...)
	if err := os.WriteFile(sp, longSource, 0600); err != nil {
		t.Fatal(err)
	}
	if err := runAdaptiveFlagReport(base, io.Discard); err != nil {
		t.Fatal("new source cap leaked into legacy command", err)
	}
	if err := os.Remove(bp); err != nil {
		t.Fatal(err)
	}
	// These values were always rejected by the engine only after bars admission.
	for _, extras := range [][]string{
		{"--research-ablation=invalid"},
		{"--trade-from=1970-01-01T00:00:01Z", "--trade-to=1970-01-01T01:00:00Z"},
	} {
		err := runAdaptiveFlagReport(append(append([]string{}, base...), extras...), io.Discard)
		if err == nil || !strings.Contains(err.Error(), bp) {
			t.Fatal("legacy engine validation moved before bars read", err)
		}
	}
	if err := os.WriteFile(sp, []byte("dsl v7 setup { type: adaptive volume flag }"), 0600); err != nil {
		t.Fatal(err)
	}
	err := runAdaptiveFlagReport(base, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "adaptive flag DSL parse errors") || strings.Contains(err.Error(), bp) {
		t.Fatal("legacy source parsing moved after bars read", err)
	}
}
