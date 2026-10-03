package engine

import (
	"math"
	"sort"

	"github.com/spik3r/heisentick-strat/marketdata"
)

// vpNYHandoffState ports withVpNyHandoffVeto's default 07:00-12:00 UTC
// source and 12:00-16:00 UTC destination windows. It is attached only by the
// explicit interactive composition entry point, never by ordinary DSL runs.
type vpNYHandoffState struct {
	key                  int64
	initialized, pending bool
	active, resolved     bool
	destStart, destEnd   float64
	predicted            string
	expectedEdge         float64
	targetDistanceRange  float64
}

const vpHour = 3_600_000.0

func (v *vpNYHandoffState) update(series marketdata.Series, i int) {
	t := series.T[i]
	key := int64(math.Floor(t / dayMS))
	if !v.initialized || v.key != key {
		*v = vpNYHandoffState{key: key, initialized: true, pending: true,
			destStart: float64(key)*dayMS + 12*vpHour,
			destEnd:   float64(key)*dayMS + 16*vpHour}
	}
	if v.pending && t >= v.destStart {
		v.build(series, i)
	}
	if !v.active || v.resolved {
		return
	}
	if v.predicted == "high" && series.H[i] > v.expectedEdge {
		v.resolved = true
	}
	if v.predicted == "low" && series.L[i] < v.expectedEdge {
		v.resolved = true
	}
}

func (v *vpNYHandoffState) build(series marketdata.Series, i int) {
	start := float64(v.key) * dayMS
	source := make([]marketdata.Bar, 0, 20)
	var destOpen float64
	gotDest := false
	for j := i; j >= 0 && series.T[j] >= start+7*vpHour; j-- {
		t := series.T[j]
		if t >= v.destStart && t < v.destEnd {
			destOpen = series.O[j] // reverse scan leaves the earliest destination open
			gotDest = true
		}
		if t >= start+7*vpHour && t < start+12*vpHour {
			source = append(source, series.Bar(j))
		}
	}
	if len(source) < 4 {
		v.pending = false
		return
	}
	for left, right := 0, len(source)-1; left < right; left, right = left+1, right-1 {
		source[left], source[right] = source[right], source[left]
	}
	low, high := math.Inf(1), math.Inf(-1)
	for _, bar := range source {
		low = math.Min(low, bar.L)
		high = math.Max(high, bar.H)
	}
	if !(high > low) {
		v.pending = false
		return
	}
	val, vah, ok := vpNYValueArea(source, 0.1)
	if !ok {
		v.pending = false
		return
	}
	if !gotDest { // The JS wrapper retries when the destination has no bars yet.
		return
	}
	v.pending = false
	if destOpen > vah {
		v.predicted, v.expectedEdge = "high", high
	} else if destOpen < val {
		v.predicted, v.expectedEdge = "low", low
	} else {
		return
	}
	v.targetDistanceRange = math.Abs(v.expectedEdge-destOpen) / (high - low)
	v.active = v.targetDistanceRange <= 0.25
}

func (v *vpNYHandoffState) inWindow(t float64) bool {
	return v.active && t >= v.destStart && t < v.destEnd
}

func (v *vpNYHandoffState) block(t float64, side side) bool {
	if !v.inWindow(t) || v.resolved {
		return false
	}
	return (v.predicted == "high" && side == sideShort) ||
		(v.predicted == "low" && side == sideLong)
}

func (v *vpNYHandoffState) entryMeta(t float64) (TradeMeta, bool) {
	if !v.inWindow(t) {
		return nil, false
	}
	return TradeMeta{
		"predicted": v.predicted, "expectedEdge": v.expectedEdge,
		"targetDistanceRange": v.targetDistanceRange,
	}, true
}

// vpNYValueArea mirrors VolumeProfile's number_of_rows/24, 70% value area
// algorithm. JS anchors price rows at the source profile low and distributes
// each candle's volume in proportion to its overlap with those rows.
func vpNYValueArea(bars []marketdata.Bar, tick float64) (val, vah float64, ok bool) {
	low, high, totalVolume := math.Inf(1), math.Inf(-1), 0.0
	for _, bar := range bars {
		if bar.V <= 0 {
			continue
		}
		low, high = math.Min(low, bar.L), math.Max(high, bar.H)
		totalVolume += bar.V
	}
	if !isFinite(low) || !isFinite(high) || totalVolume <= 0 {
		return 0, 0, false
	}
	totalTicks := math.Max(1, math.Ceil((high-low)/tick))
	ideal := totalTicks / 24
	lowTicks := math.Max(1, math.Floor(ideal))
	highTicks := math.Max(1, math.Ceil(ideal))
	ticksPerRow := highTicks
	if math.Abs(math.Ceil(totalTicks/lowTicks)-24) < math.Abs(math.Ceil(totalTicks/highTicks)-24) {
		ticksPerRow = lowTicks
	}
	rowHeight := ticksPerRow * tick
	anchorTick, highTick := math.Round(low/tick), math.Round(high/tick)
	rowIndex := func(price float64) int {
		priceTick := math.Round(price / tick)
		if priceTick <= anchorTick {
			return 0
		}
		if priceTick >= highTick {
			index := math.Floor((highTick - anchorTick) / ticksPerRow)
			if anchorTick+index*ticksPerRow == highTick {
				return int(math.Max(0, index-1))
			}
			return int(index)
		}
		return int(math.Floor((priceTick - anchorTick) / ticksPerRow))
	}
	bins := make(map[int]float64)
	for _, bar := range bars {
		if bar.V <= 0 {
			continue
		}
		lowIndex, highIndex := rowIndex(bar.L), rowIndex(bar.H)
		if lowIndex == highIndex || bar.H == bar.L {
			bins[lowIndex] += bar.V
			continue
		}
		for index := lowIndex; index <= highIndex; index++ {
			rowLow := low + float64(index)*rowHeight
			overlap := math.Max(0, math.Min(bar.H, rowLow+rowHeight)-math.Max(bar.L, rowLow))
			if overlap > 0 {
				bins[index] += overlap / (bar.H - bar.L) * bar.V
			}
		}
	}
	if len(bins) == 0 {
		return 0, 0, false
	}
	indices := make([]int, 0, len(bins))
	for index := range bins {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	poc, maxVolume := 0, -1.0
	for j, index := range indices {
		vol := bins[index]
		if vol > maxVolume {
			poc, maxVolume = j, vol
		} else if math.Abs(vol-maxVolume) < 1e-10 && index > indices[poc] {
			// JS moves the POC on a near tie without lowering the maximum
			// used to compare subsequent rows.
			poc = j
		}
	}
	// Value area starts with the selected POC row's actual volume, which
	// can be just below the retained maximum after a near tie.
	current := bins[indices[poc]]
	bottom, top := poc, poc
	for up, down := poc+1, poc-1; current < totalVolume*0.7 && (up < len(indices) || down >= 0); {
		chooseUp := down < 0
		if !chooseUp && up < len(indices) {
			upVolume, downVolume := bins[indices[up]], bins[indices[down]]
			chooseUp = upVolume > downVolume ||
				(upVolume == downVolume && up-poc <= poc-down)
		}
		if chooseUp {
			current += bins[indices[up]]
			top, up = up, up+1
		} else {
			current += bins[indices[down]]
			bottom, down = down, down-1
		}
	}
	return low + float64(indices[bottom])*rowHeight,
		low + float64(indices[top]+1)*rowHeight, true
}
