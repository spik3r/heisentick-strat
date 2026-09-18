package engine

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
)

func TestParamsFromConfigCarriesTargetEdge(t *testing.T) {
	tests := []struct {
		name string
		cfg  dsl.Config
		want string
	}{
		{name: "missing defaults to range", cfg: dsl.Config{}, want: "range"},
		{name: "range", cfg: dsl.Config{"target": map[string]any{"edge": "range"}}, want: "range"},
		{name: "channel", cfg: dsl.Config{"target": map[string]any{"edge": "channel"}}, want: "channel"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := paramsFromConfig(tt.cfg).TargetEdge; got != tt.want {
				t.Fatalf("target edge = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContextOptionsRequestsAuthoredChannelTarget(t *testing.T) {
	tests := []struct {
		name string
		cfg  dsl.Config
		want bool
	}{
		{name: "range target stays disabled", cfg: dsl.Config{"target": map[string]any{"edge": "range"}}, want: false},
		{name: "channel target infers context", cfg: dsl.Config{"target": map[string]any{"edge": "channel"}}, want: true},
		{name: "explicit channel remains enabled", cfg: dsl.Config{"channel": map[string]any{"enabled": true}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contextOptions(RunFixture{}, tt.cfg).Channel.Enabled; got != tt.want {
				t.Fatalf("channel context enabled = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLevelSweepTargetUsesFreshChannelAndPreservesFallbackChain(t *testing.T) {
	r := rbfRange{High: 120, Low: 80}
	tests := []struct {
		name       string
		side       side
		targetEdge string
		age        int32
		upper      float64
		lower      float64
		targetR    float64
		minR       float64
		want       float64
		wantOK     bool
	}{
		{name: "fresh channel long", side: sideLong, targetEdge: "channel", age: 2, upper: 130, lower: 70, targetR: 1, minR: 0.6, want: 130, wantOK: true},
		{name: "fresh channel short", side: sideShort, targetEdge: "channel", age: 2, upper: 130, lower: 70, targetR: 1, minR: 0.6, want: 70, wantOK: true},
		{name: "stale channel falls back to range", side: sideLong, targetEdge: "channel", age: 13, upper: 130, lower: 70, targetR: 1, minR: 0.6, want: 120, wantOK: true},
		{name: "missing channel falls back to range", side: sideShort, targetEdge: "channel", age: -1, upper: 130, lower: 70, targetR: 1, minR: 0.6, want: 80, wantOK: true},
		{name: "wrong-side channel falls back to R", side: sideLong, targetEdge: "channel", age: 0, upper: 90, lower: 70, targetR: 1, minR: 0.6, want: 110, wantOK: true},
		{name: "too-close channel falls back to R", side: sideLong, targetEdge: "channel", age: 0, upper: 105, lower: 70, targetR: 1, minR: 0.6, want: 110, wantOK: true},
		{name: "invalid channel falls back to R", side: sideShort, targetEdge: "channel", age: 0, upper: 130, lower: math.NaN(), targetR: 1, minR: 0.6, want: 90, wantOK: true},
		{name: "fallback below minimum rejects", side: sideLong, targetEdge: "channel", age: 0, upper: 90, lower: 70, targetR: 0.5, minR: 0.6, want: 105, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := broker{
				params: flagParams{
					TargetEdge: tt.targetEdge,
					TargetR:    tt.targetR,
					MinTargetR: tt.minR,
					ChannelBreakHold: channelBreakHoldParams{
						ActiveWithinCandles: 12,
					},
				},
				cols: contextcols.Columns{LastChannel: contextcols.ChannelColumns{
					SinceActive: []int32{tt.age},
					Upper:       []float64{tt.upper},
					Lower:       []float64{tt.lower},
				}},
			}
			got, ok := b.levelSweepTarget(0, r, tt.side, 100, 10)
			if ok != tt.wantOK || math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("target = %v, ok = %v; want %v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestRunFixtureCaseHonorsOppositeChannelEdgeTarget(t *testing.T) {
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), "family-channel-break-hold.fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), "family-level-sweep.strat"))
	if err != nil {
		t.Fatalf("read DSL: %v", err)
	}
	withChannelContext := strings.Replace(string(source), "filters {", "filters {\n  channel active within 1000 candles", 1)
	channelTarget := strings.Replace(withChannelContext, "take profit: opposite range edge", "take profit: opposite channel edge", 1)
	if withChannelContext == string(source) || channelTarget == withChannelContext {
		t.Fatal("fixture source mutations did not apply")
	}

	rangeResult, err := RunFixtureCase(fixture, withChannelContext)
	if err != nil {
		t.Fatalf("run range target: %v", err)
	}
	channelResult, err := RunFixtureCase(fixture, channelTarget)
	if err != nil {
		t.Fatalf("run channel target: %v", err)
	}
	if len(rangeResult.Trades) != 1 || len(channelResult.Trades) != 1 {
		t.Fatalf("trade counts = range %d, channel %d; want one each", len(rangeResult.Trades), len(channelResult.Trades))
	}
	rangeTrade := rangeResult.Trades[0]
	channelTrade := channelResult.Trades[0]
	if rangeTrade.EntryIndex != 823 || channelTrade.EntryIndex != rangeTrade.EntryIndex || channelTrade.Entry != rangeTrade.Entry {
		t.Fatalf("entries drifted: range %#v, channel %#v", rangeTrade, channelTrade)
	}
	if math.Abs(rangeTrade.InitialTP-2023.9340535714284) > 1e-9 {
		t.Fatalf("range target = %.15f, want %.15f", rangeTrade.InitialTP, 2023.9340535714284)
	}
	if math.Abs(channelTrade.InitialTP-1979.0134768386959) > 1e-9 {
		t.Fatalf("channel target = %.15f, want %.15f", channelTrade.InitialTP, 1979.0134768386959)
	}
}
