package engine

import (
	"math"
	"sort"

	"github.com/spik3r/heisentick-strat/marketdata"
)

// VolumeProfileRow holds the volume assigned to one price interval.
type VolumeProfileRow struct {
	Index        int
	PriceLow     float64
	PriceHigh    float64
	DisplayPrice float64
	TotalVolume  float64
	UpVolume     float64
	DownVolume   float64
	Delta        float64
	VolumePct    float64
	IsPOC        bool
	IsValueArea  bool
}

// VolumeProfile is a completed, deterministic OHLCV volume profile. Its row
// allocation and value-area tie rules match the browser VolumeProfile class.
type VolumeProfile struct {
	TickSize             float64
	RowsLayout           string
	RowSize              float64
	ValueAreaPercent     float64
	ProfileHigh          float64
	ProfileLow           float64
	TotalVolume          float64
	UpVolume             float64
	DownVolume           float64
	Rows                 []VolumeProfileRow
	POC                  float64
	VAH                  float64
	VAL                  float64
	EffectiveTicksPerRow int
}

type volumeBin struct {
	total float64
	up    float64
	down  float64
}

// ComputeVolumeProfile distributes each bar's volume across the price rows it
// spans, in proportion to overlap. Bars with non-positive or non-finite volume
// are ignored, as they are by the browser implementation.
func ComputeVolumeProfile(bars []marketdata.Bar, tickSize float64, rowsLayout string, rowSize, valueAreaPercent float64) (VolumeProfile, bool) {
	if !(tickSize > 0) || !isFinite(tickSize) || !(rowSize > 0) || !isFinite(rowSize) {
		return VolumeProfile{}, false
	}
	if rowsLayout != "number_of_rows" && rowsLayout != "ticks_per_row" {
		return VolumeProfile{}, false
	}
	if valueAreaPercent == 0 || !isFinite(valueAreaPercent) {
		valueAreaPercent = 70
	}
	profile := VolumeProfile{TickSize: tickSize, RowsLayout: rowsLayout, RowSize: rowSize, ValueAreaPercent: valueAreaPercent, ProfileLow: math.Inf(1), ProfileHigh: math.Inf(-1)}
	valid := make([]marketdata.Bar, 0, len(bars))
	for _, bar := range bars {
		if !isFinite(bar.V) || bar.V <= 0 || !isFinite(bar.O) || !isFinite(bar.H) || !isFinite(bar.L) || !isFinite(bar.C) || bar.H < bar.L {
			continue
		}
		valid = append(valid, bar)
		profile.ProfileHigh = math.Max(profile.ProfileHigh, bar.H)
		profile.ProfileLow = math.Min(profile.ProfileLow, bar.L)
		profile.TotalVolume += bar.V
		if bar.C >= bar.O {
			profile.UpVolume += bar.V
		} else {
			profile.DownVolume += bar.V
		}
	}
	if len(valid) == 0 {
		return VolumeProfile{}, false
	}

	ticksPerRow := 1
	anchor := 0.0
	if rowsLayout == "ticks_per_row" {
		ticksPerRow = max(1, int(math.Floor(rowSize+0.5)))
	} else {
		spanTicks := max(1, int(math.Ceil((profile.ProfileHigh-profile.ProfileLow)/tickSize)))
		ideal := float64(spanTicks) / rowSize
		lowTicks := max(1, int(math.Floor(ideal)))
		highTicks := max(1, int(math.Ceil(ideal)))
		rowsLow := int(math.Ceil(float64(spanTicks) / float64(lowTicks)))
		rowsHigh := int(math.Ceil(float64(spanTicks) / float64(highTicks)))
		if math.Abs(float64(rowsLow)-rowSize) < math.Abs(float64(rowsHigh)-rowSize) {
			ticksPerRow = lowTicks
		} else {
			ticksPerRow = highTicks
		}
		anchor = profile.ProfileLow
	}
	rowHeight := float64(ticksPerRow) * tickSize
	anchorTick := jsRound(anchor / tickSize)
	highTick := jsRound(profile.ProfileHigh / tickSize)
	rowIndex := func(price float64) int {
		priceTick := jsRound(price / tickSize)
		if rowsLayout == "number_of_rows" {
			if priceTick <= anchorTick {
				return 0
			}
			if priceTick >= highTick {
				idx := int(math.Floor(float64(highTick-anchorTick) / float64(ticksPerRow)))
				if anchorTick+idx*ticksPerRow == highTick {
					return max(0, idx-1)
				}
				return idx
			}
			return int(math.Floor(float64(priceTick-anchorTick) / float64(ticksPerRow)))
		}
		return int(math.Floor(float64(priceTick) / float64(ticksPerRow)))
	}
	bins := map[int]*volumeBin{}
	add := func(index int, volume float64, up bool) {
		bin := bins[index]
		if bin == nil {
			bin = &volumeBin{}
			bins[index] = bin
		}
		bin.total += volume
		if up {
			bin.up += volume
		} else {
			bin.down += volume
		}
	}
	for _, bar := range valid {
		up := bar.C >= bar.O
		if bar.H == bar.L {
			add(rowIndex(bar.H), bar.V, up)
			continue
		}
		low, high := rowIndex(bar.L), rowIndex(bar.H)
		if low == high {
			add(low, bar.V, up)
			continue
		}
		for index := low; index <= high; index++ {
			rowLow := anchor + float64(index)*rowHeight
			rowHigh := anchor + float64(index+1)*rowHeight
			overlap := math.Max(0, math.Min(bar.H, rowHigh)-math.Max(bar.L, rowLow))
			if overlap > 0 {
				add(index, overlap/(bar.H-bar.L)*bar.V, up)
			}
		}
	}
	indices := make([]int, 0, len(bins))
	for index := range bins {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	profile.EffectiveTicksPerRow = ticksPerRow
	maxVolume := -1.0
	pocIndex := -1
	for _, index := range indices {
		bin := bins[index]
		low := anchor + float64(index)*rowHeight
		high := anchor + float64(index+1)*rowHeight
		row := VolumeProfileRow{Index: index, PriceLow: low, PriceHigh: high, DisplayPrice: (low + high) / 2, TotalVolume: bin.total, UpVolume: bin.up, DownVolume: bin.down, Delta: bin.up - bin.down, VolumePct: bin.total / profile.TotalVolume * 100}
		if bin.total > maxVolume || (math.Abs(bin.total-maxVolume) < 1e-10 && (pocIndex < 0 || low > profile.Rows[pocIndex].PriceLow)) {
			maxVolume, pocIndex = bin.total, len(profile.Rows)
		}
		profile.Rows = append(profile.Rows, row)
	}
	if pocIndex < 0 {
		return VolumeProfile{}, false
	}
	profile.Rows[pocIndex].IsPOC = true
	profile.POC = profile.Rows[pocIndex].DisplayPrice
	profile.Rows[pocIndex].IsValueArea = true
	current := profile.Rows[pocIndex].TotalVolume
	target := profile.TotalVolume * valueAreaPercent / 100
	upIndex, downIndex := pocIndex+1, pocIndex-1
	for current < target && (upIndex < len(profile.Rows) || downIndex >= 0) {
		hasUp, hasDown := upIndex < len(profile.Rows), downIndex >= 0
		if hasUp && hasDown {
			upVol, downVol := profile.Rows[upIndex].TotalVolume, profile.Rows[downIndex].TotalVolume
			if upVol > downVol {
				current += upVol
				profile.Rows[upIndex].IsValueArea = true
				upIndex++
			} else if downVol > upVol {
				current += downVol
				profile.Rows[downIndex].IsValueArea = true
				downIndex--
			} else if upIndex-pocIndex < pocIndex-downIndex {
				current += upVol
				profile.Rows[upIndex].IsValueArea = true
				upIndex++
			} else if pocIndex-downIndex < upIndex-pocIndex {
				current += downVol
				profile.Rows[downIndex].IsValueArea = true
				downIndex--
			} else {
				current += upVol
				profile.Rows[upIndex].IsValueArea = true
				upIndex++
			}
		} else if hasUp {
			current += profile.Rows[upIndex].TotalVolume
			profile.Rows[upIndex].IsValueArea = true
			upIndex++
		} else {
			current += profile.Rows[downIndex].TotalVolume
			profile.Rows[downIndex].IsValueArea = true
			downIndex--
		}
	}
	profile.VAL, profile.VAH = math.Inf(1), math.Inf(-1)
	for _, row := range profile.Rows {
		if row.IsValueArea {
			profile.VAL = math.Min(profile.VAL, row.PriceLow)
			profile.VAH = math.Max(profile.VAH, row.PriceHigh)
		}
	}
	return profile, true
}

func jsRound(value float64) int {
	return int(math.Floor(value + 0.5))
}
