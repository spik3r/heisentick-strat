package engine

import (
	"math"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func colsWithATR(n int, atr float64) contextcols.Columns {
	cols := contextcols.Columns{ATR: make([]float64, n)}
	for i := range cols.ATR {
		cols.ATR[i] = atr
	}
	return cols
}

func TestFairValueGapDetectionUsesCompletedThreeBarPattern(t *testing.T) {
	series := marketdata.SeriesFromBars([]marketdata.Bar{
		{T: 0, O: 100, H: 101, L: 99, C: 100},
		{T: 1, O: 100, H: 104, L: 100, C: 104},
		{T: 2, O: 104, H: 106, L: 105, C: 105.5},
	})
	b := broker{series: series, cols: colsWithATR(3, 1), params: flagParams{FairValueGap: fairValueGapParams{MinGapATR: 0.5, MinDisplacementATR: 2}}}
	b.detectFairValueGap(1)
	if len(b.fvgZones) != 0 {
		t.Fatalf("gap became active before the third bar closed: %#v", b.fvgZones)
	}
	b.detectFairValueGap(2)
	if len(b.fvgZones) != 1 {
		t.Fatalf("fvg zones = %#v, want one completed gap", b.fvgZones)
	}
	zone := b.fvgZones[0]
	if zone.Side != sideLong || zone.Low != 101 || zone.High != 105 || zone.CreatedAt != 2 {
		t.Fatalf("zone = %#v, want bullish [101,105] created at 2", zone)
	}
	series.C[2] = 100.5
	b.pruneFairValueGaps(2)
	if len(b.fvgZones) != 0 {
		t.Fatalf("invalidated gap survived: %#v", b.fvgZones)
	}
}

func TestFairValueGapSetupUsesConfiguredReferenceAndCausalRisk(t *testing.T) {
	series := marketdata.SeriesFromBars([]marketdata.Bar{
		{T: 0, O: 100, H: 101, L: 99, C: 100},
		{T: 1, O: 100, H: 104, L: 100, C: 104},
		{T: 2, O: 104, H: 106, L: 105, C: 105.5},
		{T: 3, O: 105.5, H: 106, L: 102, C: 103},
	})
	b := broker{
		series: series,
		cols:   colsWithATR(4, 1),
		params: flagParams{AllowLong: true, FairValueGap: fairValueGapParams{MinGapATR: 0.5, MinDisplacementATR: 2, RetestCandles: 8, EntryReference: "midpoint"}, MinStopATR: 0.25, MaxStopATR: 3, StopBufferATR: 0.25, TargetR: 2},
	}
	b.detectFairValueGap(2)
	setup, reference, ok := b.fairValueGapSetup(3, &b.fvgZones[0])
	if !ok || reference != 103 {
		t.Fatalf("midpoint setup = %#v reference=%v ok=%v", setup, reference, ok)
	}
	if setup.Side != sideLong || setup.Stop != 100.75 || setup.Target != 107.5 {
		t.Fatalf("setup = %#v, want long stop 100.75 target 107.5", setup)
	}
	if setup.Meta["setup"] != "fairValueGap" || setup.Meta["gapId"] != "long:2" {
		t.Fatalf("setup metadata = %#v", setup.Meta)
	}
	b.params.TradeWindowUnrestricted = true
	b.onFairValueGapBar(3)
	if len(b.limitOrders) != 1 || b.limitOrders[0].ExpireBars != 1 || b.limitOrders[0].ExpireAt != 4 {
		t.Fatalf("FVG limit order = %#v, want one-bar expiry", b.limitOrders)
	}
}

func TestFairValueGapSetupUsesDirectionalFib618Reference(t *testing.T) {
	series := marketdata.SeriesFromBars([]marketdata.Bar{
		{T: 0, O: 100, H: 101, L: 99, C: 100},
		{T: 1, O: 100, H: 104, L: 100, C: 104},
		{T: 2, O: 104, H: 106, L: 105, C: 105.5},
		{T: 3, O: 105.5, H: 106, L: 102, C: 103},
	})
	b := broker{
		series: series,
		cols:   colsWithATR(4, 1),
		params: flagParams{AllowLong: true, FairValueGap: fairValueGapParams{MinGapATR: 0.5, MinDisplacementATR: 2, RetestCandles: 8, EntryReference: "fib618"}, MinStopATR: 0.25, MaxStopATR: 3, StopBufferATR: 0.25, TargetR: 2},
	}
	b.detectFairValueGap(2)
	_, reference, ok := b.fairValueGapSetup(3, &b.fvgZones[0])
	if !ok || math.Abs(reference-103.472) > 1e-9 {
		t.Fatalf("fib618 long reference=%v ok=%v, want 103.472", reference, ok)
	}

	b.params.AllowLong = false
	b.params.AllowShort = true
	zone := &fvgZone{Side: sideShort, Low: 101, High: 105, CreatedAt: 2}
	_, reference, ok = b.fairValueGapSetup(3, zone)
	if !ok || math.Abs(reference-102.528) > 1e-9 {
		t.Fatalf("fib618 short reference=%v ok=%v, want 102.528", reference, ok)
	}
}

func TestFairValueGapSweepGateRequiresSweptPivot(t *testing.T) {
	swept := []marketdata.Bar{
		{T: 0, O: 100, H: 100.8, L: 99.5, C: 100.2},
		{T: 1, O: 99.4, H: 99.8, L: 99.0, C: 99.6},
		{T: 2, O: 98.6, H: 99.2, L: 98.0, C: 98.9},
		{T: 3, O: 98.8, H: 99.4, L: 98.5, C: 99.1},
		{T: 4, O: 99.0, H: 99.8, L: 99.2, C: 99.5},
		{T: 5, O: 99.9, H: 100.2, L: 97.8, C: 99.9},
		{T: 6, O: 100, H: 104, L: 99.8, C: 103.5},
		{T: 7, O: 103.6, H: 104.4, L: 103.8, C: 104.2},
	}
	rules := func(side side, lookback, width int) fairValueGapParams {
		return fairValueGapParams{
			MinGapATR: 0.1, MinDisplacementATR: 0.8, RetestCandles: 8, EntryReference: "midpoint",
			SweepRequired: true, SweepLookback: lookback, SweepPivotWidth: width,
		}
	}

	b := broker{series: marketdata.SeriesFromBars(swept), cols: colsWithATR(len(swept), 1), params: flagParams{FairValueGap: rules(sideLong, 30, 2)}}
	b.detectFairValueGap(7)
	if len(b.fvgZones) != 1 {
		t.Fatalf("swept formation rejected: %#v", b.fvgZones)
	}

	noSweep := append([]marketdata.Bar(nil), swept...)
	noSweep[5] = marketdata.Bar{T: 5, O: 99.9, H: 100.2, L: 98.1, C: 99.9}
	b2 := broker{series: marketdata.SeriesFromBars(noSweep), cols: colsWithATR(len(noSweep), 1), params: flagParams{FairValueGap: rules(sideLong, 30, 2)}}
	b2.detectFairValueGap(7)
	if len(b2.fvgZones) != 0 {
		t.Fatalf("unswept formation admitted: %#v", b2.fvgZones)
	}

	lookbackWindow := []marketdata.Bar{
		{T: 0, O: 100, H: 100.8, L: 99.5, C: 100.2},
		{T: 1, O: 99.4, H: 99.8, L: 99.0, C: 99.6},
		{T: 2, O: 98.6, H: 99.2, L: 98.0, C: 98.9},
		{T: 3, O: 98.8, H: 99.4, L: 98.5, C: 99.1},
		{T: 4, O: 99.0, H: 99.8, L: 99.2, C: 99.5},
		{T: 5, O: 99.9, H: 100.2, L: 97.8, C: 99.9},
		{T: 6, O: 99.9, H: 100.0, L: 99.6, C: 99.8},
		{T: 7, O: 100, H: 104, L: 99.8, C: 103.5},
		{T: 8, O: 103.6, H: 104.4, L: 103.8, C: 104.2},
	}
	b3 := broker{series: marketdata.SeriesFromBars(lookbackWindow), cols: colsWithATR(len(lookbackWindow), 1)}
	b3.params = flagParams{FairValueGap: rules(sideLong, 3, 2)}
	b3.detectFairValueGap(8)
	if len(b3.fvgZones) != 0 {
		t.Fatalf("sweep outside lookahead window admitted: %#v", b3.fvgZones)
	}
	b3.params = flagParams{FairValueGap: rules(sideLong, 30, 2)}
	b3.detectFairValueGap(8)
	if len(b3.fvgZones) != 1 {
		t.Fatalf("in-window sweep rejected: %#v", b3.fvgZones)
	}
}

func TestFairValueGapFamilyIsImplementedAndConfigured(t *testing.T) {
	parsed, err := dsl.Parse(`dsl v7
setup { type: fvg gap minimum 0.1 ATR displacement minimum 0.8 ATR retest within 8 candles entry at midpoint }
`)
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("parse FVG: err=%v errors=%v", err, parsed.Errors)
	}
	params := paramsFromConfig(parsed.Config)
	if params.SetupType != string(dsl.FamilyFairValueGap) || !implementedFamily(params.SetupType) {
		t.Fatalf("FVG dispatch setupType=%q implemented=%v", params.SetupType, implementedFamily(params.SetupType))
	}
	if params.FairValueGap.MinGapATR != 0.1 || params.FairValueGap.MinDisplacementATR != 0.8 || params.FairValueGap.RetestCandles != 8 {
		t.Fatalf("FVG params = %#v", params.FairValueGap)
	}
}

func TestFairValueGapUsesDeclaredEntryWindow(t *testing.T) {
	params := paramsFromConfig(map[string]any{
		"setupType":    "fairValueGap",
		"fairValueGap": map[string]any{"entryExpireCandles": 12},
	})
	if params.FairValueGap.EntryExpireCandles != 12 {
		t.Fatalf("entry expiry = %d, want 12", params.FairValueGap.EntryExpireCandles)
	}
}
