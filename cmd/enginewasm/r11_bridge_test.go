package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	native "github.com/spik3r/heisentick-strat/engine"
)

func r11TestSource(t *testing.T, id string) string {
	t.Helper()
	path := filepath.Join("testdata", id+".strat")
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes)
}

func r11TestFixture(tf, id string) r11Fixture {
	step := int64(30 * 60 * 1000)
	if tf == "1h" {
		step = 60 * 60 * 1000
	}
	start := int64(1704067200000)
	entry := make([][]float64, 100)
	for i := range entry {
		timestamp := start + int64(i)*step
		close := 2000 + float64(i)*0.1
		entry[i] = []float64{float64(timestamp), close, close + 0.5, close - 0.5, close, 100}
	}
	source := make([][]float64, 30)
	for i := range source {
		timestamp := start + int64(i)*4*60*60*1000
		close := 2000 + float64(i)*0.2
		source[i] = []float64{float64(timestamp), close, close + 1, close - 1, close, 400}
	}
	from := start + 20*step
	to := start + 90*step
	return r11Fixture{
		Schema: r11FixtureSchema, ContractVersion: 1, StrategyID: id, Symbol: "XAUUSD",
		Timeframe: tf, SourceTimeframe: "4h", TradeFromMS: from, TradeToMS: to,
		Execution: r11Execution{SlippagePerFill: 0.06, CommissionPerUnitSide: 0.5, Units: 1},
		EntryBars: entry, SourceBars: source,
	}
}

func r11TestJSON(t *testing.T, fixture r11Fixture) string {
	t.Helper()
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func r11DecodeEnvelope(t *testing.T, raw []byte) r11Envelope {
	t.Helper()
	var envelope r11Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, raw)
	}
	return envelope
}

func TestRunR11FixtureReturnsPinnedNativeLedgerAndIdentity(t *testing.T) {
	for _, tc := range []struct{ tf, id string }{
		{"30m", "r11-management-target-3r-30m"},
		{"1h", "r11-entry-chop-45-1h"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			source := r11TestSource(t, tc.id)
			fixture := r11TestFixture(tc.tf, tc.id)
			raw := r11TestJSON(t, fixture)
			got := r11DecodeEnvelope(t, runR11Fixture(raw, source))
			if got.Schema != r11ResultSchema || got.ContractVersion != 1 || got.Error != nil || got.Identity == nil || got.Run == nil {
				t.Fatalf("unexpected result envelope: %+v", got)
			}
			fixtureDigest := sha256.Sum256([]byte(raw))
			dslDigest := sha256.Sum256([]byte(source))
			if got.Identity.FixtureSHA256 != hex.EncodeToString(fixtureDigest[:]) || got.Identity.DSLSHA256 != hex.EncodeToString(dslDigest[:]) {
				t.Fatalf("identity hashes differ: %+v", got.Identity)
			}
			if got.Identity.StrategyID != fixture.StrategyID || got.Identity.Symbol != fixture.Symbol || got.Identity.Timeframe != fixture.Timeframe || got.Identity.SourceTimeframe != fixture.SourceTimeframe || got.Identity.TradeFromMS != fixture.TradeFromMS || got.Identity.TradeToMS != fixture.TradeToMS || got.Identity.Execution != fixture.Execution {
				t.Fatalf("identity does not echo the request: %+v", got.Identity)
			}
			parsed, err := dsl.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			entry, err := r11RowsToSeries(fixture.EntryBars, fixture.Timeframe)
			if err != nil {
				t.Fatal(err)
			}
			sourceBars, err := r11RowsToSeries(fixture.SourceBars, "4h")
			if err != nil {
				t.Fatal(err)
			}
			want, err := native.RunRangeReversion(native.RangeReversionRequest{
				Config: parsed.Config, EntrySeries: entry, SourceSeries: sourceBars,
				Window:    native.RangeReversionWindow{TradeFromMS: fixture.TradeFromMS, TradeToMS: fixture.TradeToMS},
				Execution: native.RangeReversionExecution{SlippagePerFill: fixture.Execution.SlippagePerFill, CommissionPerUnitSide: fixture.Execution.CommissionPerUnitSide, Units: 1},
			})
			if err != nil {
				t.Fatal(err)
			}
			gotJSON, err := json.Marshal(got.Run)
			if err != nil {
				t.Fatal(err)
			}
			wantJSON, err := json.Marshal(want)
			if err != nil {
				t.Fatal(err)
			}
			if string(gotJSON) != string(wantJSON) {
				t.Fatalf("bridge changed native result\n got: %s\nwant: %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestPinnedR11StrategyRejectsDailyCHOPExtension(t *testing.T) {
	source := r11TestSource(t, "r11-entry-chop-45-1h")
	parsed, err := dsl.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := dsl.DecodeRangeReversion(parsed.Config)
	if err != nil {
		t.Fatal(err)
	}
	fixture := r11TestFixture("1h", "r11-entry-chop-45-1h")
	spec.Rules.DailyCHOP = &dsl.RangeReversionDailyCHOP{Period: 14, Min: 38.2, Max: 61.8}
	if err := validateR11Strategy(fixture, spec); err == nil {
		t.Fatal("pinned R11 browser identity accepted a Daily CHOP extension")
	}
}

func TestRunR11FixtureRejectsInvalidIdentityAndStrictJSON(t *testing.T) {
	source := r11TestSource(t, "r11-management-target-3r-30m")
	fixture := r11TestFixture("30m", "r11-management-target-3r-30m")
	valid := r11TestJSON(t, fixture)
	cases := []struct {
		name, raw, source, code string
	}{
		{"wrong schema version", strings.Replace(valid, `"contractVersion":1`, `"contractVersion":2`, 1), source, "R11_FIXTURE_INVALID"},
		{"unknown field", strings.Replace(valid, `"schema":"strat-r11-fixture-v1"`, `"schema":"strat-r11-fixture-v1","extra":true`, 1), source, "R11_FIXTURE_INVALID"},
		{"missing field", strings.Replace(valid, `"sourceTimeframe":"4h",`, ``, 1), source, "R11_FIXTURE_INVALID"},
		{"duplicate root key", strings.Replace(valid, `"schema":"strat-r11-fixture-v1"`, `"schema":"strat-r11-fixture-v1","schema":"strat-r11-fixture-v1"`, 1), source, "R11_FIXTURE_INVALID"},
		{"duplicate nested key", strings.Replace(valid, `"units":1`, `"units":1,"units":1`, 1), source, "R11_FIXTURE_INVALID"},
		{"missing slippage", strings.Replace(valid, `"slippagePerFill":0.06,`, ``, 1), source, "R11_FIXTURE_INVALID"},
		{"null slippage", strings.Replace(valid, `"slippagePerFill":0.06`, `"slippagePerFill":null`, 1), source, "R11_FIXTURE_INVALID"},
		{"missing commission", strings.Replace(valid, `,"commissionPerUnitSide":0.5`, ``, 1), source, "R11_FIXTURE_INVALID"},
		{"null commission", strings.Replace(valid, `"commissionPerUnitSide":0.5`, `"commissionPerUnitSide":null`, 1), source, "R11_FIXTURE_INVALID"},
		{"missing unit field", strings.Replace(valid, `,"units":1`, ``, 1), source, "R11_FIXTURE_INVALID"},
		{"null unit field", strings.Replace(valid, `"units":1`, `"units":null`, 1), source, "R11_FIXTURE_INVALID"},
		{"null source cell", strings.Replace(valid, `,2000.5,`, `,null,`, 1), source, "R11_FIXTURE_INVALID"},
		{"trailing JSON", valid + `{}`, source, "R11_FIXTURE_INVALID"},
		{"unknown strategy", strings.Replace(valid, fixture.StrategyID, "r11-entry-chop-46-1h", 1), source, "R11_IDENTITY_MISMATCH"},
		{"wrong DSL digest", valid, source + " ", "R11_IDENTITY_MISMATCH"},
		{"wrong route", strings.Replace(valid, `"timeframe":"30m"`, `"timeframe":"1h"`, 1), source, "R11_FIXTURE_INVALID"},
		{"wrong chart grid", strings.Replace(valid, fmt.Sprintf("%d", int64(fixture.EntryBars[1][0])), fmt.Sprintf("%d", int64(fixture.EntryBars[1][0])+1), 1), source, "R11_FIXTURE_INVALID"},
		{"non-unit sizing", strings.Replace(valid, `"units":1`, `"units":2`, 1), source, "R11_FIXTURE_INVALID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := r11DecodeEnvelope(t, runR11Fixture(tc.raw, tc.source))
			if got.Error == nil || got.Error.Code != tc.code {
				t.Fatalf("got error %+v, want code %s", got.Error, tc.code)
			}
		})
	}
}

func TestR11ValidationRejectsMalformedBarsAndLimits(t *testing.T) {
	fixture := r11TestFixture("30m", "r11-management-target-3r-30m")
	mutations := map[string]func(*r11Fixture){
		"short row":           func(f *r11Fixture) { f.EntryBars[0] = f.EntryBars[0][:5] },
		"duplicate timestamp": func(f *r11Fixture) { f.EntryBars[1][0] = f.EntryBars[0][0] },
		"invalid OHLC":        func(f *r11Fixture) { f.EntryBars[0][3] = f.EntryBars[0][1] + 1 },
		"negative volume":     func(f *r11Fixture) { f.SourceBars[0][5] = -1 },
		"off-grid H4":         func(f *r11Fixture) { f.SourceBars[0][0] += 1 },
		"epoch-zero start":    func(f *r11Fixture) { f.TradeFromMS = 0 },
		"only one usable H4":  func(f *r11Fixture) { f.TradeToMS = int64(f.SourceBars[1][0]) },
		"entry cap":           func(f *r11Fixture) { f.EntryBars = make([][]float64, r11MaxEntryBars+1) },
		"source cap":          func(f *r11Fixture) { f.SourceBars = make([][]float64, r11MaxSourceBars+1) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			f := fixture
			f.EntryBars = append([][]float64(nil), fixture.EntryBars...)
			f.SourceBars = append([][]float64(nil), fixture.SourceBars...)
			mutate(&f)
			if err := validateR11Fixture(f); err == nil {
				t.Fatal("invalid fixture accepted")
			}
		})
	}
}

func TestR11FixtureJSONWalkRejectsDuplicatesAndExtraNesting(t *testing.T) {
	fixture := r11TestFixture("30m", "r11-management-target-3r-30m")
	valid := r11TestJSON(t, fixture)
	if err := validateR11FixtureJSON(valid); err != nil {
		t.Fatalf("rejected valid fixture: %v", err)
	}
	for name, raw := range map[string]string{
		"duplicate root key":   strings.Replace(valid, `"schema":"strat-r11-fixture-v1"`, `"schema":"strat-r11-fixture-v1","schema":"strat-r11-fixture-v1"`, 1),
		"duplicate nested key": strings.Replace(valid, `"units":1`, `"units":1,"units":1`, 1),
		"extra nesting":        strings.Replace(valid, `"entryBars":[[`, `"entryBars":[[[`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateR11FixtureJSON(raw); err == nil {
				t.Fatal("accepted invalid fixture JSON")
			}
		})
	}
}

func TestR11OutputBudgetBoundsBeforeEncoding(t *testing.T) {
	if got := r11EstimatedOutputBytes(0, 0); got >= r11MaxOutput {
		t.Fatalf("empty output estimate is unexpectedly large: %d", got)
	}
	if got := r11EstimatedOutputBytes(r11MaxSignals, r11MaxTrades); got <= r11MaxOutput {
		t.Fatalf("max signal/trade counts should exceed output budget: %d", got)
	}
}
