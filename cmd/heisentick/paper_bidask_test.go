//go:build !js

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/testsupport"
)

func TestPaperBidAskCommandSafeRefusals(t *testing.T) {
	for _, args := range [][]string{{"paper-bidask"}, {"paper-bidask", "--request-file=/missing/private/source"}, {"paper-bidask", "--other=secret"}} {
		var out bytes.Buffer
		if e := run(args, &out); e != nil {
			t.Fatal(e)
		}
		var v map[string]any
		if json.Unmarshal(out.Bytes(), &v) != nil || v["status"] != "refused" || v["schema"] != "heisentick-paper-bidask-v1" || len(v) != 3 || strings.Contains(out.String(), "private") || strings.Contains(out.String(), "secret") {
			t.Fatalf("unsafe refusal %s", out.String())
		}
	}
}
func TestPaperBidAskNativeExecutableBoundedInput(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "heisentick")
	goTool := filepath.Join(runtime.GOROOT(), "bin", "go")
	build := exec.Command(goTool, "build", "-buildvcs=false", "-o", binary, ".")
	if b, e := build.CombinedOutput(); e != nil {
		t.Fatalf("native command build: %v %s", e, b)
	}
	checkPaperBidAskNativeSuccess(t, binary, dir)
	bad := filepath.Join(dir, "private-source.json")
	if e := os.WriteFile(bad, []byte(`{"source":"PRIVATE_SOURCE_SECRET","schema":"wrong"}`), 0600); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{bad, dir} {
		cmd := exec.Command(binary, "paper-bidask", "--request-file="+path)
		out, e := cmd.CombinedOutput()
		if e != nil || bytes.Contains(out, []byte("PRIVATE_SOURCE_SECRET")) || bytes.Contains(out, []byte(dir)) {
			t.Fatalf("unsafe native refusal: %v %s", e, out)
		}
		var v map[string]any
		if json.Unmarshal(out, &v) != nil || v["status"] != "refused" {
			t.Fatalf("not one complete refusal %s", out)
		}
	}
	fifo := filepath.Join(dir, "fifo")
	if e := syscall.Mkfifo(fifo, 0600); e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(binary, "paper-bidask", "--request-file="+fifo)
	done := make(chan error, 1)
	go func() {
		out, e := cmd.CombinedOutput()
		if e == nil && !bytes.Contains(out, []byte("PAPER_BIDASK_INPUT")) {
			e = &bidAskCLIUnexpected{}
		}
		done <- e
	}()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("FIFO blocked bounded command")
	}
	link := filepath.Join(dir, "symlink")
	if e := os.Symlink(bad, link); e != nil {
		t.Fatal(e)
	}
	out, e := exec.Command(binary, "paper-bidask", "--request-file="+link).CombinedOutput()
	if e != nil || !bytes.Contains(out, []byte("PAPER_BIDASK_INPUT")) {
		t.Fatalf("symlink accepted %v %s", e, out)
	}
}

type bidAskCLIUnexpected struct{}

func (*bidAskCLIUnexpected) Error() string { return "unexpected native FIFO result" }

// All observations are invented. The expected fills/PnL below are independently
// specified constants, never values read back from another engine invocation.
func paperBidAskCLIRequest(t *testing.T, short bool) engine.PaperBidAskRequest {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(testsupport.MustRepoRoot(), "conformance/parse/setup-opening-range-breakout.strat"))
	if err != nil {
		t.Fatal(err)
	}
	r := engine.PaperBidAskRequest{
		Schema: engine.PaperBidAskRequestSchema, Source: string(source),
		Route:           engine.PaperRoute{StrategyID: "invented-cli", Symbol: "XAUUSD", Timeframe: "5m", RangeMethod: "zone"},
		Identity:        engine.BidAskIdentity{ArtifactDigest: strings.Repeat("a", 64), PrincipalDigest: strings.Repeat("1", 64), AccountDigest: strings.Repeat("2", 64), RunID: strings.Repeat("3", 64), Epoch: strings.Repeat("4", 64)},
		Product:         engine.BidAskProduct{Provider: "oanda-v20", Instrument: "XAU_USD", AttestationDigest: strings.Repeat("5", 64), AccountCurrency: "USD", PriceUnit: "USD/troy-ounce", QuantityUnit: "troy-ounce", Multiplier: "1"},
		Economics:       engine.BidAskEconomics{StartEquity: "1000", EntryFee: "0", ExitFee: "0", SlippageFixed: "0", SlippageBPS: "0", QuantityQuantum: "0.25", QuantityMinimum: "0.25", QuantityMaximum: "10000", NotionalPolicy: "none", NotionalMaximum: "0"},
		Policy:          engine.BidAskPolicy{Model: engine.PaperBidAskModel, MaxQuoteAgeMS: "5000", MaxSignalAgeMS: "290000", Coverage: "refuse-discontinuous-exposure", Arithmetic: "decimal-rational-family-shortest-v1"},
		ActivationFromT: "2025-01-01T00:00:00Z",
	}
	stamp := func(d time.Duration) string {
		return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(d).Format(time.RFC3339Nano)
	}
	appendRecord := func(v engine.BidAskRecord) {
		v.Sequence, v.Epoch = strconv.Itoa(len(r.Records)+1), r.Identity.Epoch
		r.Records = append(r.Records, v)
	}
	for i := 0; i < 4; i++ {
		v := engine.BidAskOHLC{Open: "100", High: "100.5", Low: "99.5", Close: "100"}
		if i == 3 {
			v = engine.BidAskOHLC{Open: "100", High: "101.5", Low: "99.75", Close: "101"}
			if short {
				v = engine.BidAskOHLC{Open: "100", High: "100.25", Low: "98.5", Close: "99"}
			}
		}
		bar := &engine.BidAskBar{OpenT: stamp(time.Duration(i) * 5 * time.Minute), CloseT: stamp(time.Duration(i+1) * 5 * time.Minute), ReceivedT: stamp(time.Duration(i+1)*5*time.Minute + time.Nanosecond), ProviderID: strings.Repeat("6", 64), ProvenanceDigest: strings.Repeat("7", 64), Mid: v, Bid: v, Ask: v, PriceCount: "1", Complete: true}
		appendRecord(engine.BidAskRecord{Kind: "bar", Bar: bar, CommitT: bar.ReceivedT})
	}
	episode := &engine.BidAskEpisode{SignalSequence: "4", AdmittedT: stamp(20*time.Minute + 100*time.Millisecond), DeadlineT: stamp(20*time.Minute + 200*time.Second)}
	appendRecord(engine.BidAskRecord{Kind: "episode", Episode: episode, CommitT: episode.AdmittedT})
	for i := 1; i <= 2; i++ {
		at := 20*time.Minute + time.Duration(i)*time.Second
		q := &engine.BidAskQuote{ProviderT: stamp(at), RequestStartT: stamp(at - time.Millisecond), ReceivedT: stamp(at + time.Millisecond), QualifiedT: stamp(at + 2*time.Millisecond), ProviderID: strings.Repeat("8", 64), ProvenanceDigest: strings.Repeat("9", 64), AccountDigest: r.Identity.AccountDigest, Instrument: r.Product.Instrument, Tradeable: true}
		bid, ask := "101", "101.2"
		if short {
			bid, ask = "98.8", "99"
		}
		if i == 1 {
			q.EpisodeSignalSequence = "4"
		} else {
			bid, ask = "100", "100.2"
		}
		q.Bids = []engine.BidAskTier{{Price: engine.BidAskDecimal(bid), Liquidity: "10000"}}
		q.Asks = []engine.BidAskTier{{Price: engine.BidAskDecimal(ask), Liquidity: "10000"}}
		appendRecord(engine.BidAskRecord{Kind: "quote", Quote: q, CommitT: stamp(at + 3*time.Millisecond)})
	}
	return r
}

func checkPaperBidAskNativeSuccess(t *testing.T, binary, dir string) {
	t.Helper()
	path := filepath.Join(dir, "invented-request.json")
	invoke := func(r engine.PaperBidAskRequest) []byte {
		t.Helper()
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(binary, "paper-bidask", "--request-file="+path)
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err = cmd.Run(); err != nil || stderr.Len() != 0 || !bytes.HasSuffix(stdout.Bytes(), []byte("\n")) {
			t.Fatalf("native output %v %s %s", err, stdout.String(), stderr.String())
		}
		return stdout.Bytes()
	}
	decode := func(raw []byte) engine.PaperBidAskResult {
		t.Helper()
		var v engine.PaperBidAskResult
		if err := json.Unmarshal(raw, &v); err != nil || v.Mode != "replay-verified-candidate" {
			t.Fatalf("native success %v %s", err, raw)
		}
		return v
	}
	for _, short := range []bool{false, true} {
		t.Run(fmt.Sprint("native-short-", short), func(t *testing.T) {
			r := paperBidAskCLIRequest(t, short)
			prefix := r
			prefix.Records = r.Records[:6]
			opened := decode(invoke(prefix))
			entry, side, balance, net := "101.2", "long", "760", "-240"
			if short {
				entry, side, balance, net = "98.8", "short", "720", "-280"
			}
			if opened.Position == nil || opened.Position.Entry != entry || opened.Position.Side != side || opened.Position.Quantity != "200" || opened.Account.Equity != "960" || opened.Account.Balance != "1000" {
				t.Fatalf("independent entry expectation %+v", opened)
			}
			r.Checkpoint = opened.Checkpoint
			closed := decode(invoke(r))
			if closed.Position != nil || closed.Account.Balance != balance || closed.CompletedTrades.NetPnL != net || closed.CompletedTrades.Completed != 1 {
				t.Fatalf("independent stop expectation %+v", closed)
			}
			r.Checkpoint = closed.Checkpoint
			same := decode(invoke(r))
			if len(same.Events) != 0 || same.StateDigest != closed.StateDigest || string(same.Checkpoint) != string(closed.Checkpoint) {
				t.Fatal("native replay duplicated effect")
			}
		})
	}
	for _, tc := range []struct {
		mutate func(*engine.PaperBidAskRequest)
		code   string
	}{
		{func(r *engine.PaperBidAskRequest) {
			r.Source += "\nmanagement {\n trail 1 ATR after 1R\n trail off\n}\n"
		}, "TRAILING_UNQUALIFIED"},
		{func(r *engine.PaperBidAskRequest) { r.Source += "\nfilters { side both } hidden ignored text\n" }, "ADMISSION"},
		{func(r *engine.PaperBidAskRequest) { r.Records[5].Quote.ProviderT = "2025-01-01T00:20:01.0000000001Z" }, "TIMESTAMP"},
		{func(r *engine.PaperBidAskRequest) { r.Checkpoint = json.RawMessage(`{"unknown":true}`) }, "CHECKPOINT"},
	} {
		r := paperBidAskCLIRequest(t, false)
		tc.mutate(&r)
		raw := invoke(r)
		var v map[string]any
		if json.Unmarshal(raw, &v) != nil || len(v) != 3 || v["status"] != "refused" || v["code"] != "PAPER_BIDASK_"+tc.code {
			t.Fatalf("typed native refusal want %s: %s", tc.code, raw)
		}
	}
	large := filepath.Join(dir, "oversize.json")
	file, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate(engine.PaperBidAskMaxBytes + 1)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(binary, "paper-bidask", "--request-file="+large).CombinedOutput()
	if err != nil || !bytes.Contains(out, []byte(`"code":"PAPER_BIDASK_INPUT"`)) {
		t.Fatalf("oversize file accepted %v %s", err, out)
	}
}

type paperBidAskOutputFailure struct{ short bool }

func (w paperBidAskOutputFailure) Write(b []byte) (int, error) {
	if w.short {
		return len(b) - 1, nil
	}
	return 0, errors.New("PRIVATE_OUTPUT_DETAIL")
}
func TestPaperBidAskOutputFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "request.json")
	raw, _ := json.Marshal(paperBidAskCLIRequest(t, false))
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{nil, {"--request-file=" + path}} {
		for _, short := range []bool{false, true} {
			if err := runPaperBidAsk(args, paperBidAskOutputFailure{short}); err == nil || err.Error() != "PAPER_BIDASK_OUTPUT" {
				t.Fatalf("unsafe output error: %v", err)
			}
		}
	}
	if err := runPaperBidAsk([]string{"--request-file=" + path}, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestPaperBidAskInheritedReadOnlyDescriptor(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "heisentick")
	if out, e := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-buildvcs=false", "-o", binary, ".").CombinedOutput(); e != nil {
		t.Fatalf("build %v %s", e, out)
	}
	raw, e := json.Marshal(paperBidAskCLIRequest(t, false))
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "request.json")
	if e = os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	invoke := func(file *os.File, arg string) []byte {
		t.Helper()
		cmd := exec.Command(binary, "paper-bidask", arg)
		cmd.ExtraFiles = []*os.File{file}
		out, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("descriptor command %v %s", e, out)
		}
		return out
	}
	read, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer read.Close()
	out := invoke(read, "--request-fd=3")
	var result engine.PaperBidAskResult
	if json.Unmarshal(out, &result) != nil || result.Mode != "replay-verified-candidate" {
		t.Fatalf("read descriptor refused %s", out)
	}
	if _, e = read.Stat(); e != nil {
		t.Fatal("child closed parent descriptor", e)
	}
	write, e := os.OpenFile(path, os.O_RDWR, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer write.Close()
	if out = invoke(write, "--request-fd=3"); !bytes.Contains(out, []byte("PAPER_BIDASK_INPUT")) {
		t.Fatalf("writable descriptor admitted %s", out)
	}
	directory, e := os.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer directory.Close()
	if out = invoke(directory, "--request-fd=3"); !bytes.Contains(out, []byte("PAPER_BIDASK_INPUT")) {
		t.Fatalf("directory admitted %s", out)
	}
	pipeReader, pipeWriter, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer pipeReader.Close()
	defer pipeWriter.Close()
	if out = invoke(pipeReader, "--request-fd=3"); !bytes.Contains(out, []byte("PAPER_BIDASK_INPUT")) {
		t.Fatalf("pipe admitted %s", out)
	}
	for _, arg := range []string{"--request-fd=0", "--request-fd=03", "--request-fd=65", "--request-fd=missing", "--request-fd="} {
		if out = invoke(read, arg); !bytes.Contains(out, []byte("PAPER_BIDASK_ARGUMENTS")) {
			t.Fatalf("bad descriptor argument %s", out)
		}
	}
}
