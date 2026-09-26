package engine

import (
	"math"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestComputeVolumeProfileMatchesBrowserAllocationFixture(t *testing.T) {
	bars := []marketdata.Bar{
		{O: 100, H: 102, L: 100, C: 101, V: 10},
		{O: 101, H: 104, L: 101, C: 103, V: 20},
		{O: 103, H: 104, L: 100, C: 100, V: 30},
		{O: 100, H: 103, L: 100, C: 102, V: 40},
		{O: 100, H: 105, L: 100, C: 105, V: 0}, // ignored by the browser implementation
	}
	profile, ok := ComputeVolumeProfile(bars, 1, "number_of_rows", 2, 70)
	if !ok {
		t.Fatal("ComputeVolumeProfile returned no profile")
	}
	assertProfileNear(t, profile.ProfileHigh, 104)
	assertProfileNear(t, profile.ProfileLow, 100)
	assertProfileNear(t, profile.TotalVolume, 100)
	assertProfileNear(t, profile.UpVolume, 70)
	assertProfileNear(t, profile.DownVolume, 30)
	assertProfileNear(t, profile.POC, 101)
	assertProfileNear(t, profile.VAL, 100)
	assertProfileNear(t, profile.VAH, 104)
	if profile.EffectiveTicksPerRow != 2 || len(profile.Rows) != 2 {
		t.Fatalf("effective rows = %d / %d, want 2 / 2", profile.EffectiveTicksPerRow, len(profile.Rows))
	}
	want := []VolumeProfileRow{
		{Index: 0, PriceLow: 100, PriceHigh: 102, DisplayPrice: 101, TotalVolume: 58.33333333333333, UpVolume: 43.33333333333333, DownVolume: 15, Delta: 28.33333333333333, VolumePct: 58.33333333333333, IsPOC: true, IsValueArea: true},
		{Index: 1, PriceLow: 102, PriceHigh: 104, DisplayPrice: 103, TotalVolume: 41.666666666666664, UpVolume: 26.666666666666664, DownVolume: 15, Delta: 11.666666666666664, VolumePct: 41.666666666666664, IsValueArea: true},
	}
	for i, expected := range want {
		actual := profile.Rows[i]
		for name, pair := range map[string][2]float64{
			"price low": {actual.PriceLow, expected.PriceLow}, "price high": {actual.PriceHigh, expected.PriceHigh},
			"total volume": {actual.TotalVolume, expected.TotalVolume}, "up volume": {actual.UpVolume, expected.UpVolume},
			"down volume": {actual.DownVolume, expected.DownVolume}, "delta": {actual.Delta, expected.Delta},
			"volume percent": {actual.VolumePct, expected.VolumePct},
		} {
			if math.Abs(pair[0]-pair[1]) > 1e-10 {
				t.Errorf("row %d %s = %.15g, want %.15g", i, name, pair[0], pair[1])
			}
		}
		if actual.IsPOC != expected.IsPOC || actual.IsValueArea != expected.IsValueArea {
			t.Errorf("row %d flags = POC %t, VA %t; want POC %t, VA %t", i, actual.IsPOC, actual.IsValueArea, expected.IsPOC, expected.IsValueArea)
		}
	}
}

func TestComputeVolumeProfileTieBreaksTowardHigherPriceRow(t *testing.T) {
	bars := []marketdata.Bar{
		{O: 1, H: 2, L: 1, C: 2, V: 10},
		{O: 3, H: 4, L: 3, C: 4, V: 10},
	}
	profile, ok := ComputeVolumeProfile(bars, 1, "ticks_per_row", 1, 70)
	if !ok {
		t.Fatal("ComputeVolumeProfile returned no profile")
	}
	if profile.POC != 3.5 {
		t.Fatalf("POC = %v, want higher-price row midpoint 3.5", profile.POC)
	}
}

func TestComputeVolumeProfileRejectsInvalidConfiguration(t *testing.T) {
	bars := []marketdata.Bar{{O: 1, H: 2, L: 1, C: 2, V: 1}}
	for _, tc := range []struct {
		name     string
		tickSize float64
		layout   string
		rowSize  float64
	}{
		{name: "zero tick", tickSize: 0, layout: "number_of_rows", rowSize: 24},
		{name: "unknown layout", tickSize: 1, layout: "other", rowSize: 24},
		{name: "zero rows", tickSize: 1, layout: "number_of_rows", rowSize: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := ComputeVolumeProfile(bars, tc.tickSize, tc.layout, tc.rowSize, 70); ok {
				t.Fatal("ComputeVolumeProfile returned a profile for invalid configuration")
			}
		})
	}
}

func assertProfileNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-10 {
		t.Fatalf("got %.15g, want %.15g", got, want)
	}
}
