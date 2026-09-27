package engine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestMarketNonSessionGatesHonorsLocalWeekdayAndHour(t *testing.T) {
	timestamp := time.Date(2025, time.January, 3, 14, 0, 0, 0, time.UTC).UnixMilli() // Saturday 00:00 at UTC+10.
	b := broker{
		series: marketdata.Series{T: []float64{float64(timestamp)}},
		params: flagParams{
			LocalWeekdays: []string{"Sat"},
			LocalHours:    []int{0},
			MaxMovementER: 1,
		},
		cols: contextcols.Columns{Regime: []int8{0}, ER: []float64{0}},
	}
	if !b.marketNonSessionGatesOK(0) {
		t.Fatal("UTC+10 Saturday midnight should pass the matching include gates")
	}
	b.params.BlockedLocalWeekdays = []string{"Sat"}
	if b.marketNonSessionGatesOK(0) {
		t.Fatal("blocked local weekday should reject a bar that passed the allowlist")
	}
	b.params.BlockedLocalWeekdays = nil
	b.params.LocalHours = []int{1}
	if b.marketNonSessionGatesOK(0) {
		t.Fatal("local hour outside the include list should be rejected")
	}
	b.params.LocalHours = nil
	b.params.BlockedLocalHours = []int{0}
	if b.marketNonSessionGatesOK(0) {
		t.Fatal("blocked local hour should reject a matching bar")
	}
	b.params.BlockedLocalHours = nil
	b.series.T = nil
	if b.marketNonSessionGatesOK(0) {
		t.Fatal("missing timestamp must fail closed when a local gate is configured")
	}
}

func TestMarketNonSessionGatesHonorsOpenLocation(t *testing.T) {
	b := broker{
		params: flagParams{OpenLocations: []string{"nearAH"}, MaxMovementER: 1},
		cols:   contextcols.Columns{OpenLocation: []int8{7, 8}, Regime: []int8{0, 0}, ER: []float64{0, 0}},
	}
	if !b.marketNonSessionGatesOK(0) {
		t.Fatal("nearAH should pass the open-location allowlist")
	}
	if b.marketNonSessionGatesOK(1) {
		t.Fatal("nearAL should fail a nearAH-only open-location allowlist")
	}
	b.cols.OpenLocation = nil
	if b.marketNonSessionGatesOK(0) {
		t.Fatal("missing open-location context must fail closed")
	}
}

func TestContextOptionsRequestsOpenLocationForGate(t *testing.T) {
	cfg := map[string]any{
		"setupType":     "flagContinuation",
		"openLocations": []any{"nearAH"},
	}
	options := contextOptions(RunFixture{}, cfg)
	if !options.Selective || !options.NeedOpenLocation {
		t.Fatalf("context options = %+v, want selective open-location context", options)
	}
}

func TestParamsFromConfigMapsLocalMarketContextGates(t *testing.T) {
	parsed, err := dsl.Parse(`dsl v7
market conditions {
  local weekday in (Mon, Tue)
  local weekday not in (Fri)
  local hour in (9, 10)
  local hour not in (23)
  open location in (nearAH, nearDO)
}
setup { type: failed breakout }
`)
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("Parse errors=%v err=%v", parsed.Errors, err)
	}
	params := paramsFromConfig(parsed.Config)
	if !reflect.DeepEqual(params.LocalWeekdays, []string{"Mon", "Tue"}) ||
		!reflect.DeepEqual(params.BlockedLocalWeekdays, []string{"Fri"}) ||
		!reflect.DeepEqual(params.LocalHours, []int{9, 10}) ||
		!reflect.DeepEqual(params.BlockedLocalHours, []int{23}) ||
		!reflect.DeepEqual(params.OpenLocations, []string{"nearAH", "nearDayOpen"}) {
		t.Fatalf("market gate params = %+v", params)
	}
}

func TestRunFixtureCaseBlocksEntriesOnEveryLocalWeekday(t *testing.T) {
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), "family-break-retest.fixture.json"))
	if err != nil {
		t.Fatalf("LoadRunFixture: %v", err)
	}
	sourcePath := filepath.Join(runFixtureDir(), "family-break-retest.strat")
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	source := string(sourceBytes)
	baseline, err := RunFixtureCase(fixture, source)
	if err != nil {
		t.Fatalf("RunFixtureCase baseline: %v", err)
	}
	if baseline.TradeCount == 0 {
		t.Fatal("baseline fixture no longer exercises entries")
	}
	filteredSource := strings.Replace(source, "market conditions {", "market conditions {\n  local weekday not in (Sun, Mon, Tue, Wed, Thu, Fri, Sat)", 1)
	if filteredSource == source {
		t.Fatal("fixture source has no market conditions block")
	}
	filtered, err := RunFixtureCase(fixture, filteredSource)
	if err != nil {
		t.Fatalf("RunFixtureCase with weekday gate: %v", err)
	}
	if filtered.TradeCount != 0 || len(filtered.Trades) != 0 {
		t.Fatalf("all-weekday block produced %d trades; want none", filtered.TradeCount)
	}
}
