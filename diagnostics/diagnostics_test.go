package diagnostics

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/report"
)

type fixture struct {
	RecentFrom string           `json:"recentFrom"`
	MinTrades  int              `json:"minTrades"`
	Full       report.Document  `json:"full"`
	Recent     *report.Document `json:"recent"`
	Harsh      *report.Document `json:"harsh"`
}

// pfTolerance is the relative tolerance for the one raw float the JS emits
// (`pf`); every other field is compared exactly, after rendering the Go
// result in the JS row shape (whole-number fields as numbers, two-decimal
// fields as toFixed strings, "-" for nil).
const pfTolerance = 1e-12

func TestFixturesMatchJSDerivation(t *testing.T) {
	names, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, path := range names {
		if strings.HasSuffix(path, ".expected.json") {
			continue
		}
		count++
		name := strings.TrimSuffix(filepath.Base(path), ".json")
		t.Run(name, func(t *testing.T) {
			var in fixture
			readJSON(t, path, &in)
			var expected map[string]any
			readJSON(t, filepath.Join("testdata", name+".expected.json"), &expected)

			full, err := PassFromDocument(in.Full)
			if err != nil {
				t.Fatal(err)
			}
			input := Input{Full: full, RecentFrom: in.RecentFrom, MinTrades: in.MinTrades}
			if in.Recent != nil {
				recent, err := PassFromDocument(*in.Recent)
				if err != nil {
					t.Fatal(err)
				}
				input.Recent = &recent
			}
			if in.Harsh != nil {
				harsh, err := PassFromDocument(*in.Harsh)
				if err != nil {
					t.Fatal(err)
				}
				input.Harsh = &harsh
			}
			result := Evaluate(input)
			got := jsRow(name, result)

			wantPf, gotPf := expected["pf"].(float64), got["pf"].(float64)
			if diff := math.Abs(gotPf - wantPf); diff > pfTolerance*math.Max(1, math.Abs(wantPf)) {
				t.Errorf("pf: got %v want %v", gotPf, wantPf)
			}
			delete(expected, "pf")
			delete(got, "pf")
			if !reflect.DeepEqual(got, expected) {
				gotJSON, _ := json.MarshalIndent(got, "", "  ")
				wantJSON, _ := json.MarshalIndent(expected, "", "  ")
				t.Errorf("row mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
			}
		})
	}
	if count != 6 {
		t.Fatalf("expected 6 fixtures, found %d", count)
	}
}

// jsRow renders a Result as the JS `out` row, using only the JS conventions
// the Result doc comment states, so the comparison stays a field-for-field
// check of the derivation.
func jsRow(id string, result Result) map[string]any {
	fixed := func(value *float64) any {
		if value == nil {
			return "-"
		}
		return jsToFixed(*value, 2)
	}
	number := func(value *float64) any {
		if value == nil {
			return "-"
		}
		return *value
	}
	posYears := "-"
	if result.Years != 0 {
		posYears = fmt.Sprintf("%d/%d", result.PositiveYears, result.Years)
	}
	var partialYear, partialYearNet any
	if result.PartialYear != nil {
		partialYear = float64(*result.PartialYear)
	}
	if result.PartialYearNet != nil {
		partialYearNet = *result.PartialYearNet
	}
	warnings, codes := []any{}, []any{}
	for _, warning := range result.Warnings {
		warnings = append(warnings, warning.Message)
		codes = append(codes, warning.Code)
	}
	return map[string]any{
		"id":                   id,
		"trades":               float64(result.Trades),
		"pf":                   result.PF,
		"net":                  result.Net,
		"posSlicePct":          float64(result.PosSlicePct),
		"bestSlicePct":         float64(result.BestSlicePct),
		"posYears":             posYears,
		"worstYearPf":          fixed(result.WorstYearPf),
		"recentYearNet":        number(result.RecentYearNet),
		"completedYears":       float64(result.CompletedYears),
		"worstCompletedYearPf": fixed(result.WorstCompletedYearPf),
		"partialYear":          partialYear,
		"partialYearNet":       partialYearNet,
		"recentFrom":           result.RecentFrom,
		"recentPf":             fixed(result.RecentPf),
		"recentDeltaPf":        fixed(result.RecentDeltaPf),
		"harshPf":              fixed(result.HarshPf),
		"costDeltaPf":          fixed(result.CostDeltaPf),
		"warnings":             warnings,
		"warningCodes":         codes,
	}
}

func TestJSToFixedMatchesNode(t *testing.T) {
	var cases []struct {
		Value  any    `json:"value"`
		Digits int    `json:"digits"`
		Text   string `json:"text"`
	}
	readJSON(t, filepath.Join("testdata", "tofixed.expected.json"), &cases)
	if len(cases) != 20 {
		t.Fatalf("expected 20 cases, found %d", len(cases))
	}
	for _, c := range cases {
		value, ok := c.Value.(float64)
		if !ok {
			if c.Value != "-0" {
				t.Fatalf("unexpected value %v", c.Value)
			}
			value = math.Copysign(0, -1)
		}
		if got := jsToFixed(value, c.Digits); got != c.Text {
			t.Errorf("toFixed(%v, %d): got %q want %q", c.Value, c.Digits, got, c.Text)
		}
	}
}

func TestJSRound(t *testing.T) {
	for _, c := range []struct{ in, want float64 }{
		{2.5, 3}, {-2.5, -2}, {-3.5, -3}, {0.49999999999999994, 0}, {-0.4, 0}, {1234.5, 1235},
	} {
		if got := jsRound(c.in); got != c.want {
			t.Errorf("jsRound(%v): got %v want %v", c.in, got, c.want)
		}
	}
}

func TestEvaluateWithoutRecentAndHarsh(t *testing.T) {
	pf := 1.4
	result := Evaluate(Input{
		Full:       Pass{Trades: 200, PF: &pf, Net: 1000.4, SliceNets: []float64{600, 400}},
		RecentFrom: "2024-01-01",
		MinTrades:  DefaultMinTrades,
	})
	if result.RecentPf != nil || result.RecentDeltaPf != nil || result.HarshPf != nil || result.CostDeltaPf != nil {
		t.Errorf("expected nil recent and harsh metrics, got %+v", result)
	}
	if result.Net != 1000 || result.PosSlicePct != 100 || result.BestSlicePct != 60 {
		t.Errorf("unexpected slice metrics: %+v", result)
	}
	if result.Years != 0 || result.WorstYearPf == nil || *result.WorstYearPf != 0 || result.RecentYearNet == nil || *result.RecentYearNet != 0 {
		t.Errorf("empty years should mirror the JS empty yearly shape: %+v", result)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %v", result.Codes())
	}
}

func TestPassFromDocumentErrors(t *testing.T) {
	if _, err := PassFromDocument(report.Document{}); err == nil {
		t.Error("expected an error for a document without cost rows")
	}
	doc := report.Document{Costs: []report.CostRow{{Trades: 1}}}
	if _, err := PassFromDocument(doc); err == nil {
		t.Error("expected an error for a document without groupings")
	}
	doc.Groupings = &report.Groupings{Year: []report.Group{{Key: "n/a"}}}
	if _, err := PassFromDocument(doc); err == nil {
		t.Error("expected an error for a non-numeric year key")
	}
}

func readJSON(t *testing.T, path string, into any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, into); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}
