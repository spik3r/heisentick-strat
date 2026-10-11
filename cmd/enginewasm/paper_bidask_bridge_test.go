package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/testsupport"
)

// Shared invented native-CLI observation recipe, not a copied engine result.
func paperBidAskBridgeRequest(t *testing.T, short bool) engine.PaperBidAskRequest {
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

func paperBidAskBridgeRun(t *testing.T, request engine.PaperBidAskRequest) (engine.PaperBidAskResult, string) {
	t.Helper()
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := engine.DecodePaperBidAskRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.RunPaperBidAsk(decoded)
	if err != nil {
		t.Fatal(err)
	}
	want, err := engine.EncodePaperBidAskResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if got := runPaperBidAskJSON(string(raw)); got != string(want) {
		t.Fatal("transport changed complete producer document")
	}
	return result, string(want)
}

func TestPaperBidAskBridgeExactAPIAndEverySplit(t *testing.T) {
	for _, short := range []bool{false, true} {
		request := paperBidAskBridgeRequest(t, short)
		request.Source = strings.Replace(request.Source, "risk: 200 USD", "risk: 46 USD", 1)
		full, _ := paperBidAskBridgeRun(t, request)
		wantBalance, wantNet := "944.8", "-55.2"
		if short {
			wantBalance, wantNet = "935.6", "-64.4"
		}
		if full.Account.Balance != wantBalance || full.CompletedTrades.NetPnL != wantNet || full.CompletedTrades.Completed != 1 {
			t.Fatalf("independent sampled-exit values differ: %+v %+v", full.Account, full.CompletedTrades)
		}
		opened := request
		opened.Records = request.Records[:6]
		entry, _ := paperBidAskBridgeRun(t, opened)
		if entry.Position == nil || entry.Position.Quantity != "46" || entry.Account.Equity != "990.8" {
			t.Fatal("independent risk/entry values differ")
		}
		for split := 0; split <= len(request.Records); split++ {
			first := request
			first.Records = append([]engine.BidAskRecord{}, request.Records[:split]...)
			prefix, _ := paperBidAskBridgeRun(t, first)
			first.Checkpoint = prefix.Checkpoint
			retry, _ := paperBidAskBridgeRun(t, first)
			if len(retry.Events) != 0 || string(retry.Checkpoint) != string(prefix.Checkpoint) || retry.StateDigest != prefix.StateDigest {
				t.Fatal("same-prefix retry changed receipt or effects")
			}
			next := request
			next.Checkpoint = prefix.Checkpoint
			extended, _ := paperBidAskBridgeRun(t, next)
			events := append(append([]engine.BidAskEvent{}, prefix.Events...), extended.Events...)
			if !reflect.DeepEqual(events, full.Events) || !reflect.DeepEqual(extended.Account, full.Account) || !reflect.DeepEqual(extended.CompletedTrades, full.CompletedTrades) || !reflect.DeepEqual(extended.Position, full.Position) || !reflect.DeepEqual(extended.Pending, full.Pending) || extended.StateDigest != full.StateDigest || string(extended.Checkpoint) != string(full.Checkpoint) {
				t.Fatalf("split %d drift", split)
			}
		}
	}
}

func TestPaperBidAskBridgeStrictTransport(t *testing.T) {
	request := paperBidAskBridgeRequest(t, false)
	raw, _ := json.Marshal(request)
	_, healthy := paperBidAskBridgeRun(t, request)
	for _, bad := range []string{
		"", "null", "[]", "{}", string(raw) + " {}", string(raw) + "x", string([]byte{0xff}),
		strings.Replace(string(raw), `"schema":`, `"SCHEMA":`, 1),
		strings.Replace(string(raw), `"schema":`, `"unknown":0,"schema":`, 1),
		strings.Replace(string(raw), `"schema":`, `"schema":"duplicate","schema":`, 1),
		strings.Replace(string(raw), `"schema":`, `"\u0073chema":"duplicate","schema":`, 1),
		strings.Replace(string(raw), `"source":"`, `"source":"\ud800`, 1),
		strings.Replace(string(raw), `"source":"`, `"source":"\udc00`, 1),
		strings.Replace(string(raw), `"entryFee":"0"`, `"entryFee":0`, 1),
		strings.Replace(string(raw), `"records":`, `"checkpoint":null,"records":`, 1),
		strings.Replace(string(raw), `"complete":true`, `"complete":null`, 1),
		strings.Repeat("[", 65) + strings.Repeat("]", 65),
	} {
		if got := runPaperBidAskJSON(bad); got != paperBidAskFailure("PAPER_BIDASK_INPUT") {
			t.Fatalf("wrong input refusal %s", got)
		}
		if runPaperBidAskJSON(string(raw)) != healthy {
			t.Fatal("malformed request damaged later invocation")
		}
	}
	exact := string(raw) + strings.Repeat(" ", engine.PaperBidAskMaxBytes-len(raw))
	if runPaperBidAskJSON(exact) != healthy {
		t.Fatal("exact input byte budget refused")
	}
	for _, bad := range []string{exact + " ", strings.Repeat("Ω", engine.PaperBidAskMaxBytes/2+1)} {
		if runPaperBidAskJSON(bad) != paperBidAskFailure("PAPER_BIDASK_INPUT") {
			t.Fatal("input byte budget bypassed")
		}
	}
}

func TestPaperBidAskBridgeSafeErrorsAndOutputLimit(t *testing.T) {
	for _, err := range []error{nil, (*engine.PaperBidAskError)(nil), errors.New("private source /path secret"), &engine.PaperBidAskError{Code: `PAPER_BIDASK_INPUT","secret":"private`}} {
		if got := paperBidAskError(err); got != paperBidAskFailure("PAPER_BIDASK_INTERNAL") {
			t.Fatalf("unsafe error %s", got)
		}
	}
	for _, code := range []string{"PAPER_BIDASK_CHECKPOINT", "PAPER_BIDASK_QUOTE_CONFLICT", "PAPER_BIDASK_OUTPUT_LIMIT", "PAPER_BIDASK_PARTIAL_UNQUALIFIED", "PAPER_BIDASK_TIMESTAMP"} {
		if got := paperBidAskError(fmt.Errorf("private wrapper: %w", &engine.PaperBidAskError{Code: code})); got != paperBidAskFailure(code) {
			t.Fatal("typed code lost")
		}
	}
	_, err := engine.EncodePaperBidAskResult(engine.PaperBidAskResult{Coverage: strings.Repeat("x", engine.PaperBidAskMaxBytes)})
	if err == nil || paperBidAskError(err) != paperBidAskFailure("PAPER_BIDASK_OUTPUT_LIMIT") {
		t.Fatal("producer output bound lost")
	}
}

// A newly introduced native refusal must not silently become INTERNAL here.
func TestPaperBidAskBridgeCoversNativeTypedCodes(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(testsupport.MustRepoRoot(), "engine", "paper_bidask*.go"))
	if err != nil || len(paths) == 0 {
		t.Fatal("native model sources missing")
	}
	pattern := regexp.MustCompile(`bidAskError\("([A-Z_]+)"\)`)
	seen := map[string]bool{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range pattern.FindAllSubmatch(raw, -1) {
			code := "PAPER_BIDASK_" + string(match[1])
			seen[code] = true
			var envelope map[string]string
			if err := json.Unmarshal([]byte(paperBidAskError(&engine.PaperBidAskError{Code: code})), &envelope); err != nil {
				t.Fatal(err)
			}
			if len(envelope) != 3 || envelope["schema"] != engine.PaperBidAskSchema || envelope["status"] != "refused" || envelope["code"] != code {
				t.Fatalf("native refusal lost: %s", code)
			}
		}
	}
	if len(seen) == 0 {
		t.Fatal("native refusal inspection was empty")
	}
}
