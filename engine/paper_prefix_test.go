package engine

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func paperORBRequest(t testing.TB, fill string) PaperPrefixRequest {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), "../parse/setup-opening-range-breakout.strat"))
	if err != nil {
		t.Fatal(err)
	}
	const base = 1735689600000
	rows := [][]float64{}
	for i := 0; i < 3; i++ {
		rows = append(rows, []float64{base + float64(i*300000), 100, 100.5, 99.5, 100, 1})
	}
	rows = append(rows, []float64{base + 900000, 100, 101.5, 99.75, 101, 1}, []float64{base + 1200000, 101, 102.5, 100.75, 102, 1}, []float64{base + 1500000, 102, 104, 101.5, 103, 1})
	return PaperPrefixRequest{Schema: PaperPrefixRequestSchema, Source: string(source), Route: PaperRoute{"paper-orb", "XAUUSD", "5m", "zone"}, Units: PaperUnits{"USD", "USD-per-unit", "abstract-unit", 1}, Costs: PaperCosts{5000, .1, fill, .25, 0}, ActivationFromT: base, ClosedThroughT: base + 1800000, Bars: rows}
}
func paperPrefix(r PaperPrefixRequest, n int) PaperPrefixRequest {
	r.Bars = r.Bars[:n]
	r.ClosedThroughT = int64(r.Bars[n-1][0]) + paperTimeframeMS(r.Route.Timeframe)
	r.Checkpoint = nil
	return r
}
func paperNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %.17g want %.17g", got, want)
	}
}

func TestPaperORBHandDerivedCostsAndEndpoint(t *testing.T) {
	for _, fill := range []string{"close", "nextOpen"} {
		t.Run(fill, func(t *testing.T) {
			r := paperORBRequest(t, fill)
			first, err := RunPaperPrefix(paperPrefix(r, 4))
			if err != nil {
				t.Fatal(err)
			}
			if fill == "nextOpen" {
				if len(first.OpenPositions) != 0 || first.Pending.Market != 1 || first.Account.Fees != 0 {
					t.Fatalf("pending: %+v", first)
				}
			} else {
				if len(first.OpenPositions) != 1 {
					t.Fatalf("open: %+v", first)
				}
				paperNear(t, first.OpenPositions[0].Entry, 101.25)
				paperNear(t, first.OpenPositions[0].Size, 200)
				paperNear(t, first.Account.RealizedNet, -20)
				paperNear(t, first.Account.Unrealized, -50)
				paperNear(t, first.Account.Equity, 4930)
			}
			full, err := RunPaperPrefix(r)
			if err != nil {
				t.Fatal(err)
			}
			if len(full.OpenPositions) != 0 || full.CompletedTrades.Count != 1 || full.CompletedTrades.Wins != 1 {
				t.Fatalf("complete: %+v", full)
			}
			paperNear(t, full.Account.RealizedNet, 310)
			paperNear(t, full.Account.Equity, 5310)
			paperNear(t, full.Account.EntryFees, 20)
			paperNear(t, full.Account.ExitFees, 20)
			paperNear(t, full.CompletedTrades.NetPnL, 310)
			if full.CompletedTrades.ProfitFactor != nil || full.CompletedTrades.ProfitFactorStatus != "no-losses" {
				t.Fatal("profit factor")
			}
			if fill == "close" {
				paperNear(t, full.Account.MaxDrawdown, 70)
				paperNear(t, full.Account.MaxDrawdownFraction, .014)
			}
		})
	}
}

func TestPaperWarmupDoesNotConsumeORBDay(t *testing.T) {
	r := paperORBRequest(t, "close")
	// Historical breakout at index 3, re-entry into range, fresh breakout at 5.
	r.Bars[4] = []float64{r.Bars[4][0], 100, 100.5, 99.5, 100, 1}
	r.Bars[5] = []float64{r.Bars[5][0], 100, 101.5, 99.75, 101, 1}
	r.ActivationFromT = int64(r.Bars[4][0])
	warm, err := RunPaperPrefix(paperPrefix(r, 4))
	if err != nil {
		t.Fatal(err)
	}
	if len(warm.Events) != 0 || len(warm.OpenPositions) != 0 || warm.Account.Equity != 5000 || warm.Account.Fees != 0 {
		t.Fatal("warm-up traded")
	}
	r.Checkpoint = warm.Checkpoint
	live, err := RunPaperPrefix(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(live.OpenPositions) != 1 || live.OpenPositions[0].EntryIndex != 5 {
		t.Fatalf("historical opportunity consumed: %+v", live)
	}
	for _, event := range live.Events {
		if event.Index < 4 {
			t.Fatal("historical event")
		}
	}
	r.ActivationFromT++
	r.Checkpoint = nil
	if _, err := RunPaperPrefix(r); err == nil {
		t.Fatal("rounded missing activation")
	}
}

func TestPaperLegacySourceAndWarmup(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), "../parse/setup-legacy-setup9.strat"))
	if err != nil {
		t.Fatal(err)
	}
	r := paperORBRequest(t, "close")
	r.Source = string(source)
	r.Route.StrategyID = "paper-legacy"
	r.Route.Timeframe = "1h"
	r.Bars = traceBars(ramp(14, 100, 1))
	r.ActivationFromT = int64(r.Bars[12][0])
	r.ClosedThroughT = int64(r.Bars[13][0]) + 3600000
	got, err := RunPaperPrefix(paperPrefix(r, 13))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.OpenPositions) != 1 || got.OpenPositions[0].EntryIndex != 12 || got.OpenPositions[0].Side != "short" {
		t.Fatalf("legacy: %+v", got)
	}
	if got.Account.Fees <= 0 {
		t.Fatal("entry fee missing")
	}
	for _, event := range got.Events {
		if event.Index != 12 {
			t.Fatal("warm-up event")
		}
	}
	r.Costs.FillOn = "nextOpen"
	if _, err := RunPaperPrefix(r); err == nil || err.Error() != "PAPER_PREFIX_FILL_UNSUPPORTED" {
		t.Fatalf("legacy fill: %v", err)
	}
}

func TestPaperAdmissionAndStrictJSON(t *testing.T) {
	r := paperORBRequest(t, "close")
	raw, _ := json.Marshal(r)
	decoded, err := DecodePaperPrefixRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, r) {
		t.Fatal("request changed")
	}
	for _, mutate := range []func(*PaperPrefixRequest){func(r *PaperPrefixRequest) { r.Costs.StartEquity = 0 }, func(r *PaperPrefixRequest) { r.Costs.FeePerUnit = -1 }, func(r *PaperPrefixRequest) { r.Costs.SlippageBps = math.Inf(1) }, func(r *PaperPrefixRequest) { r.Costs.FillOn = "open" }, func(r *PaperPrefixRequest) { r.Units.Multiplier = 2 }, func(r *PaperPrefixRequest) { r.ClosedThroughT-- }, func(r *PaperPrefixRequest) { r.Route.Symbol = "WRONG" }, func(r *PaperPrefixRequest) { r.Source += "\nfilters { higher timeframe must agree }" }} {
		bad := r
		mutate(&bad)
		if _, err := RunPaperPrefix(bad); err == nil {
			t.Fatal("bad request accepted")
		}
	}
	for _, bad := range []string{
		strings.Replace(string(raw), `"schema":`, `"schema":"duplicate","schema":`, 1),
		strings.Replace(string(raw), `"feePerUnit":0.1,`, "", 1),
		strings.Replace(string(raw), `"feePerUnit":0.1`, `"FeePerUnit":0.1`, 1),
		strings.Replace(string(raw), `"feePerUnit":0.1`, `"feePerUnit":null`, 1),
		strings.Replace(string(raw), `,1]`, `,null]`, 1),
		strings.Replace(string(raw), `"source":"`, `"source":"\ud800`, 1),
		string(raw) + ` {}`,
	} {
		if _, err := DecodePaperPrefixRequest([]byte(bad)); err == nil {
			t.Fatalf("bad JSON accepted: %s", bad[:120])
		}
	}
	r.Checkpoint = json.RawMessage("null")
	if _, err := RunPaperPrefix(r); err != nil {
		t.Fatal(err)
	}
}

func TestPaperTransportExactTimestampAndUnicode(t *testing.T) {
	r := paperORBRequest(t, "close")
	raw, _ := json.Marshal(r)
	for _, number := range []string{"1735689600000.00001", "9007199254740990.1", "1e-100000000", "0e100000000", "9007199254740992"} {
		bad := strings.Replace(string(raw), "[[1735689600000,", "[["+number+",", 1)
		if _, err := DecodePaperPrefixRequest([]byte(bad)); err == nil {
			t.Fatalf("rounded timestamp %s", number)
		}
	}
	for _, number := range []string{"1735689600000.0", "1.7356896e12"} {
		good := strings.Replace(string(raw), "[[1735689600000,", "[["+number+",", 1)
		decoded, err := DecodePaperPrefixRequest([]byte(good))
		if err != nil || decoded.Bars[0][0] != r.Bars[0][0] {
			t.Fatalf("exact decimal timestamp %s: %v", number, err)
		}
	}
	for _, escape := range []string{`\ud800`, `\udfff`, `\ud800x`, `\ud800\ud800`, `\udc00\ud800`} {
		bad := strings.Replace(string(raw), `"source":"`, `"source":"`+escape, 1)
		if _, err := DecodePaperPrefixRequest([]byte(bad)); err == nil {
			t.Fatalf("surrogate accepted %s", escape)
		}
	}
	for _, suffix := range []string{"\r\n# Ω 🌱\r\n", "\n# �\n"} {
		r.Source += suffix
		encoded, _ := json.Marshal(r)
		decoded, err := DecodePaperPrefixRequest(encoded)
		if err != nil || decoded.Source != r.Source || paperSourceDigest(decoded.Source) != paperSourceDigest(r.Source) {
			t.Fatal("valid Unicode/CRLF changed")
		}
	}
	good := strings.Replace(string(raw), `"source":"`, `"source":"# \ud83c\udf31\n`, 1)
	if _, err := DecodePaperPrefixRequest([]byte(good)); err != nil {
		t.Fatal("valid surrogate pair rejected", err)
	}
}
