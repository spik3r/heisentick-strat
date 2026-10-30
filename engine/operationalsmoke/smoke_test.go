package operationalsmoke_test

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	smoke "github.com/spik3r/heisentick-strat/engine/operationalsmoke"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestSyntheticContracts(t *testing.T) {
	data, err := os.ReadFile("testdata/legacy-canaries.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Description string
		Cases       []struct {
			Name        string
			Profile     smoke.Profile
			Bars        [][3]float64 // Unix milliseconds, open, close.
			Index       int
			HasPosition bool
			Parameters  smoke.Parameters
			Repeat      int
			Expected    *smoke.Intent
		}
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) != 43 {
		t.Fatalf("contract corpus changed unexpectedly: %d cases", len(corpus.Cases))
	}
	seen := make(map[string]bool)
	for _, tc := range corpus.Cases {
		if seen[tc.Name] {
			t.Fatalf("duplicate vector name: %s", tc.Name)
		}
		seen[tc.Name] = true
		t.Run(tc.Name, func(t *testing.T) {
			bars := make([]marketdata.Bar, len(tc.Bars))
			for i, b := range tc.Bars {
				bars[i] = marketdata.Bar{T: b[0], O: b[1], H: math.Max(b[1], b[2]), L: math.Min(b[1], b[2]), C: b[2]}
			}
			// The caller supplies adjacent indexed bars; no scanning or sorting.
			var previous, current *marketdata.Bar
			if tc.Index >= 0 && tc.Index < len(bars) {
				current = &bars[tc.Index]
				if tc.Index >= 1 {
					previous = &bars[tc.Index-1]
				}
			}
			repeats := tc.Repeat
			if repeats == 0 {
				repeats = 1
			}
			for i := 0; i < repeats; i++ {
				got, emitted, err := smoke.Evaluate(tc.Profile, previous, current, tc.HasPosition, tc.Parameters)
				if err != nil {
					t.Fatal(err)
				}
				if emitted != (tc.Expected != nil) {
					t.Fatalf("emitted=%v, expected=%+v", emitted, tc.Expected)
				}
				if tc.Expected == nil {
					if !reflect.DeepEqual(got, smoke.Intent{}) {
						t.Fatalf("no-signal result contains an intent: %+v", got)
					}
					continue
				}
				want := *tc.Expected
				if got.Side != want.Side || got.Tag != want.Tag || (got.Size == nil) != (want.Size == nil) {
					t.Fatalf("intent identity/presence: got %+v, want %+v", got, want)
				}
				closeTo(t, "signal close", got.SignalClose, want.SignalClose)
				closeTo(t, "stop", got.SL, want.SL)
				closeTo(t, "target", got.TP, want.TP)
				closeTo(t, "risk USD", got.RiskUSD, want.RiskUSD)
				if want.Size != nil {
					closeTo(t, "size", *got.Size, *want.Size)
				}
			}
		})
	}
}

func closeTo(t *testing.T, field string, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-want) > 1e-10 {
		t.Errorf("%s: got %.17g, want %.17g", field, got, want)
	}
}

func TestWarningsAndExactIDs(t *testing.T) {
	for _, tc := range []struct {
		profile smoke.Profile
		id      string
		name    string
	}{
		{smoke.BTCUSD, "forwardExecutionSmokeThreeCandleBtcusdOneMinute", "BAD TEST ONLY — BTCUSD 1m Two-Candle Streak"},
		{smoke.AnySymbol, "forwardExecutionSmokeTwoCandleAnySymbolOneMinute", "BAD TEST ONLY — Any-Symbol 1m Two-Candle Streak"},
	} {
		d, err := smoke.Describe(tc.profile)
		if err != nil || string(d.ID) != tc.id || d.Name != tc.name || !d.Unprofitable || !d.OperationalCanary || d.ReviewNote != smoke.Warning {
			t.Fatalf("definition lost identity/warning: %+v, %v", d, err)
		}
		if !strings.Contains(d.ReviewNote, "ineligible for research promotion or book membership") {
			t.Fatal("missing promotion warning")
		}
	}
	for _, profile := range []smoke.Profile{"", "btc", "forwardExecutionSmokeTwoCandleBtcusdOneMinute", "FORWARDEXECUTIONSMOKETHREECANDLEBTCUSDONEMINUTE"} {
		if _, err := smoke.Describe(profile); !errors.Is(err, smoke.ErrUnknownProfile) {
			t.Fatalf("accepted unknown ID %q: %v", profile, err)
		}
		got, emitted, err := smoke.Evaluate(profile, nil, nil, false, smoke.Parameters{})
		if !errors.Is(err, smoke.ErrUnknownProfile) || emitted || !reflect.DeepEqual(got, smoke.Intent{}) {
			t.Fatalf("unknown ID must refuse without intent: %+v, %v", got, err)
		}
	}
}

func pair() (*marketdata.Bar, *marketdata.Bar) {
	return &marketdata.Bar{T: 0, O: 76000, C: 76100}, &marketdata.Bar{T: 60000, O: 76100, C: 76200}
}

func TestUnsupportedParametersRefuseExplicitly(t *testing.T) {
	previous, current := pair()
	for _, profile := range []smoke.Profile{smoke.BTCUSD, smoke.AnySymbol} {
		for _, value := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
			for _, field := range []string{"stop", "target", "risk", "size"} {
				p := smoke.Parameters{}
				switch field {
				case "stop":
					if profile == smoke.BTCUSD {
						p.StopDistance = &value
					} else {
						p.StopDistancePct = &value
					}
				case "target":
					if profile == smoke.BTCUSD {
						p.TargetDistance = &value
					} else {
						p.TargetDistancePct = &value
					}
				case "risk":
					p.RiskUSD = &value
				case "size":
					p.Size = &value
				}
				got, emitted, err := smoke.Evaluate(profile, previous, current, false, p)
				if !errors.Is(err, smoke.ErrUnsupportedParameter) || emitted || !reflect.DeepEqual(got, smoke.Intent{}) {
					t.Fatalf("%s %s=%v: got %+v, %v", profile, field, value, got, err)
				}
			}
		}
	}
	zero := 0.0
	for _, tc := range []struct {
		profile smoke.Profile
		params  smoke.Parameters
	}{
		{smoke.BTCUSD, smoke.Parameters{StopDistancePct: &zero}},
		{smoke.BTCUSD, smoke.Parameters{TargetDistancePct: &zero}},
		{smoke.AnySymbol, smoke.Parameters{StopDistance: &zero}},
		{smoke.AnySymbol, smoke.Parameters{TargetDistance: &zero}},
	} {
		_, emitted, err := smoke.Evaluate(tc.profile, previous, current, false, tc.params)
		if emitted || !errors.Is(err, smoke.ErrUnsupportedParameter) {
			t.Fatalf("cross-profile override was silently ignored: %v", err)
		}
	}
}

func TestInvalidNumbersAndOverflowRefuseWithoutIntent(t *testing.T) {
	for _, profile := range []smoke.Profile{smoke.BTCUSD, smoke.AnySymbol} {
		for field := 0; field < 4; field++ {
			for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
				previous, current := pair()
				fields := []*float64{&previous.O, &previous.C, &current.O, &current.C}
				*fields[field] = value
				got, emitted, err := smoke.Evaluate(profile, previous, current, false, smoke.Parameters{})
				if emitted || !errors.Is(err, smoke.ErrInvalidBar) || !reflect.DeepEqual(got, smoke.Intent{}) {
					t.Fatalf("nonfinite bar produced %+v, %v", got, err)
				}
			}
		}
	}
	previous := &marketdata.Bar{O: 1, C: 2}
	current := &marketdata.Bar{O: 2, C: math.MaxFloat64}
	distance := math.MaxFloat64
	for _, tc := range []struct {
		profile smoke.Profile
		params  smoke.Parameters
	}{
		{smoke.BTCUSD, smoke.Parameters{TargetDistance: &distance}},
		{smoke.AnySymbol, smoke.Parameters{StopDistancePct: &distance}},
	} {
		got, emitted, err := smoke.Evaluate(tc.profile, previous, current, false, tc.params)
		if emitted || !errors.Is(err, smoke.ErrInvalidBracket) || !reflect.DeepEqual(got, smoke.Intent{}) {
			t.Fatalf("overflow produced %+v, %v", got, err)
		}
	}
}

func TestOnlyOpenCloseDriveSignalAndResultDoesNotAlias(t *testing.T) {
	previous, current := pair()
	// Signal evaluation does not validate feed, OHLC bounds, volume or timestamps.
	previous.T, current.T = math.NaN(), math.Inf(1)
	previous.H, current.L, current.V = -1, 999999, math.NaN()
	size, risk := 2.5, 42.0
	got, emitted, err := smoke.Evaluate(smoke.BTCUSD, previous, current, false, smoke.Parameters{Size: &size, RiskUSD: &risk})
	if err != nil || !emitted || got.SL != 76000 || got.TP != 76400 {
		t.Fatalf("unrelated bar fields changed signal: %+v, %v", got, err)
	}
	size, risk, current.C = 99, 99, 999999
	if got.Size == nil || *got.Size != 2.5 || got.RiskUSD != 42 || got.SignalClose != 76200 {
		t.Fatalf("returned intent aliased mutable inputs: %+v", got)
	}
}

func TestPositionAndMissingHistorySuppressBeforeBarInspection(t *testing.T) {
	bad := &marketdata.Bar{O: math.NaN(), C: math.NaN()}
	for _, profile := range []smoke.Profile{smoke.BTCUSD, smoke.AnySymbol} {
		for _, tc := range []struct {
			previous, current *marketdata.Bar
			position          bool
		}{{bad, bad, true}, {nil, bad, false}, {bad, nil, false}} {
			got, emitted, err := smoke.Evaluate(profile, tc.previous, tc.current, tc.position, smoke.Parameters{})
			if err != nil || emitted || !reflect.DeepEqual(got, smoke.Intent{}) {
				t.Fatalf("suppression failed: %+v, %v", got, err)
			}
		}
	}
}
