package engine

import (
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
)

func TestPreHandlerTrailDispatchParity(t *testing.T) {
	trailFamilies := []struct {
		setupType string
		close     float64
		wantStop  float64
	}{
		{setupType: string(dsl.FamilyFlagContinuation), close: 106, wantStop: 104},
		{setupType: string(dsl.FamilyBreakRetest), close: 106, wantStop: 104},
		{setupType: string(dsl.FamilyOpeningRangeBreakout), close: 106, wantStop: 104},
		{setupType: string(dsl.FamilySessionBreakHold), close: 106, wantStop: 104},
		{setupType: string(dsl.FamilyChannelBreakHold), close: 106, wantStop: 104},
		{setupType: string(dsl.FamilyTriplePushExhaustion), close: 106, wantStop: 104},
		{setupType: string(dsl.FamilyFibContinuation), close: 106, wantStop: 104},
		{setupType: string(dsl.FamilyInsideDayExpansion), close: 106, wantStop: 104},
		{setupType: string(dsl.FamilyDayOpenReclaim), close: 106, wantStop: 104},
		{setupType: string(dsl.FamilySupplyDemand), close: 106, wantStop: 104},
		{setupType: string(dsl.FamilyDoubleTopBottom), close: 106, wantStop: 104},
		{setupType: string(dsl.FamilyTrendPullback), close: 106, wantStop: 104},
		{setupType: string(dsl.FamilyFailedBreakout), close: 106, wantStop: 104},
		{setupType: string(dsl.FamilyElderTripleScreen), close: 111, wantStop: 109},
	}
	for _, tt := range trailFamilies {
		t.Run(tt.setupType+" dispatches pre-handler trail", func(t *testing.T) {
			b := trailDispatchBroker(tt.setupType, tt.close)

			b.onBar(0)

			if got := b.position.SL; got != tt.wantStop {
				t.Fatalf("stop = %v, want pre-handler trail stop %v", got, tt.wantStop)
			}
		})
	}

	nonTrailFamilies := []string{
		string(dsl.FamilyRangeBreakFake),
		string(dsl.FamilyVWAPExtensionFade),
		string(dsl.FamilyVolumeAnomalyExhaustion),
	}
	for _, setupType := range nonTrailFamilies {
		t.Run(setupType+" keeps generic trail disabled", func(t *testing.T) {
			b := trailDispatchBroker(setupType, 106)

			b.onBar(0)

			if got, want := b.position.SL, 90.0; got != want {
				t.Fatalf("stop = %v, want unchanged stop %v", got, want)
			}
		})
	}
}

func TestFlagGenericTrailPrecedesPartialAndBreakeven(t *testing.T) {
	b := trailDispatchBroker(string(dsl.FamilyFlagContinuation), 106)
	b.params.Partial = partialParams{Enabled: true, Fraction: 0.5, TriggerR: 1.25}

	b.onBar(0)

	if len(b.trades) != 1 {
		t.Fatalf("trades = %d, want one partial after trail tightened risk: %+v", len(b.trades), b.trades)
	}
	partial := b.trades[0]
	if partial.Reason != "partial" || partial.Size != 5 || partial.SL != 104 {
		t.Fatalf("partial = %+v, want size-5 partial carrying trail stop 104", partial)
	}
	if !b.hasPosition || !b.position.PartialTaken || b.position.Size != 5 || b.position.SL != 104 {
		t.Fatalf("remaining position = %+v, want size-5 remainder with trail stop 104", b.position)
	}
}

func TestElderPreHandlerTrailPrecedesConfiguredDedicatedTrigger(t *testing.T) {
	b := trailDispatchBroker(string(dsl.FamilyElderTripleScreen), 111)
	b.params.ElderTripleScreen.TrailTriggerR = 1.5

	b.onBar(0)

	if got, want := b.position.SL, 109.0; got != want {
		t.Fatalf("stop = %v, want 1R pre-handler Elder trail stop %v", got, want)
	}
}

func TestGenericTrailUsesCurrentStopRisk(t *testing.T) {
	b := trailDispatchBroker(string(dsl.FamilyBreakRetest), 103)
	b.position.InitialSL = 90
	b.position.SL = 99
	b.params.Trail = trailParams{ATR: 1, TriggerR: 2}

	b.onBar(0)

	if got, want := b.position.SL, 102.0; got != want {
		t.Fatalf("stop = %v, want trail from current-stop risk at %v", got, want)
	}
}

func TestGenericTrailDispatchUsesNormalizedFlagSetupType(t *testing.T) {
	params := paramsFromConfig(dsl.Config{})
	if got, want := params.SetupType, string(dsl.FamilyFlagContinuation); got != want {
		t.Fatalf("default setup type = %q, want normalized %q", got, want)
	}
	if !usesGenericTrail(params.SetupType) {
		t.Fatal("normalized default flag setup did not enable generic trail dispatch")
	}
	if usesGenericTrail("") {
		t.Fatal("blank setup type should be normalized before trail dispatch")
	}
}

func trailDispatchBroker(setupType string, close float64) broker {
	b := partialDispatchBroker(setupType, []float64{close})
	b.params.Partial = partialParams{}
	b.params.BreakevenR = 0
	b.params.Trail = trailParams{ATR: 2, TriggerR: 0.5}
	b.params.ElderTripleScreen.TrailATR = 2
	b.params.ElderTripleScreen.TrailTriggerR = 1.5
	return b
}
