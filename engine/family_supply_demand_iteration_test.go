package engine

import (
	"math"
	"testing"
)

func TestNextSupplyDemandZoneVisitsEqualAgeZonesInInsertionOrder(t *testing.T) {
	b := broker{sdZones: []sdZone{
		{CreatedAt: 12},
		{CreatedAt: 12},
		{CreatedAt: 12},
		{CreatedAt: 11},
	}}

	lastCreatedAt := math.MaxInt
	lastIdx := len(b.sdZones)
	for want := range b.sdZones {
		got := b.nextSupplyDemandZone(lastCreatedAt, lastIdx)
		if got != want {
			t.Fatalf("nextSupplyDemandZone() = %d, want %d", got, want)
		}
		lastCreatedAt = b.sdZones[got].CreatedAt
		lastIdx = got
	}
	if got := b.nextSupplyDemandZone(lastCreatedAt, lastIdx); got != -1 {
		t.Fatalf("nextSupplyDemandZone() after all zones = %d, want -1", got)
	}
}
