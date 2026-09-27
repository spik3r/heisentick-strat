package contextcols

import (
	"fmt"
	"time"

	"github.com/spik3r/heisentick-strat/marketdata"
)

const seasonalityDayMS = int64(24 * time.Hour / time.Millisecond)

// SeasonalitySpec requests one causal seasonal classification series.
type SeasonalitySpec struct {
	Dimension    string
	Lookback     string
	LookbackDays int
	MinSamples   int
}

// SeasonalityKey identifies one requested dimension, lookback, and classifier threshold.
type SeasonalityKey struct {
	Dimension  string
	Lookback   string
	MinSamples int
}

// SeasonalityEntry contains the prior-data statistics visible at one bar.
type SeasonalityEntry struct {
	Bucket           string
	Bullish          int
	Bearish          int
	Doji             int
	Total            int
	DirectionalCount int
	BullishPercent   *float64
	Classification   string
	MinSamples       int
	Lookback         string
	LookbackDays     int
	Next             *SeasonalityProjection
}

// SeasonalityProjection is the prior-data classification for the next UTC
// hour, when the requested dimension is intraday.
type SeasonalityProjection struct {
	Bucket           string
	DirectionalCount int
	BullishPercent   *float64
	Classification   string
}

type seasonalityObservation struct {
	time int64
	dir  int8
}

type seasonalityBucket struct {
	bullish int
	bearish int
	doji    int
	queue   []seasonalityObservation
}

// ComputeCausalSeasonality matches the browser runtime's bar-by-bar seasonal
// calculation: each output row is read before the current bar is added.
func ComputeCausalSeasonality(series marketdata.Series, specs []SeasonalitySpec) map[SeasonalityKey][]SeasonalityEntry {
	result := make(map[SeasonalityKey][]SeasonalityEntry)
	if len(series.T) == 0 || len(specs) == 0 {
		return result
	}
	states := make([]seasonalityState, 0, len(specs))
	for _, raw := range specs {
		spec := normalizeSeasonalitySpec(raw)
		key := SeasonalityKey{Dimension: spec.Dimension, Lookback: spec.Lookback, MinSamples: spec.MinSamples}
		if _, exists := result[key]; exists {
			continue
		}
		result[key] = make([]SeasonalityEntry, len(series.T))
		states = append(states, seasonalityState{spec: spec, key: key, buckets: make(map[string]*seasonalityBucket)})
	}

	for i := range series.T {
		timestamp := int64(series.T[i])
		for stateIndex := range states {
			state := &states[stateIndex]
			bucketName, nextBucket := seasonalityBucketNames(timestamp, state.spec.Dimension)
			entry := state.bucket(bucketName)
			state.prune(entry, timestamp)
			stat := seasonalityEntry(entry, state.spec, bucketName)
			if nextBucket != "" {
				next := state.buckets[nextBucket]
				if next != nil {
					state.prune(next, timestamp)
					nextStat := seasonalityEntry(next, state.spec, nextBucket)
					stat.Next = &SeasonalityProjection{
						Bucket: nextStat.Bucket, DirectionalCount: nextStat.DirectionalCount,
						BullishPercent: nextStat.BullishPercent, Classification: nextStat.Classification,
					}
				}
			}
			result[state.key][i] = stat
		}

		direction := int8(0)
		if series.C[i] > series.O[i] {
			direction = 1
		} else if series.C[i] < series.O[i] {
			direction = -1
		}
		for stateIndex := range states {
			state := &states[stateIndex]
			bucketName, _ := seasonalityBucketNames(timestamp, state.spec.Dimension)
			bucket := state.bucket(bucketName)
			switch direction {
			case 1:
				bucket.bullish++
			case -1:
				bucket.bearish++
			default:
				bucket.doji++
			}
			if state.spec.LookbackDays > 0 {
				bucket.queue = append(bucket.queue, seasonalityObservation{time: timestamp, dir: direction})
			}
		}
	}
	return result
}

type seasonalityState struct {
	spec    SeasonalitySpec
	key     SeasonalityKey
	buckets map[string]*seasonalityBucket
}

func normalizeSeasonalitySpec(spec SeasonalitySpec) SeasonalitySpec {
	if spec.Lookback == "" {
		spec.Lookback = "all"
	}
	if spec.LookbackDays <= 0 {
		spec.LookbackDays = 0
		spec.Lookback = "all"
	}
	if spec.MinSamples <= 0 {
		spec.MinSamples = 5
	}
	return spec
}

func (state *seasonalityState) bucket(name string) *seasonalityBucket {
	entry := state.buckets[name]
	if entry == nil {
		entry = &seasonalityBucket{}
		state.buckets[name] = entry
	}
	return entry
}

func (state *seasonalityState) prune(entry *seasonalityBucket, now int64) {
	if state.spec.LookbackDays <= 0 {
		return
	}
	cutoff := now - int64(state.spec.LookbackDays)*seasonalityDayMS
	count := 0
	for count < len(entry.queue) && entry.queue[count].time < cutoff {
		old := entry.queue[count]
		switch old.dir {
		case 1:
			entry.bullish--
		case -1:
			entry.bearish--
		default:
			entry.doji--
		}
		count++
	}
	if count > 0 {
		copy(entry.queue, entry.queue[count:])
		entry.queue = entry.queue[:len(entry.queue)-count]
	}
}

func seasonalityEntry(bucket *seasonalityBucket, spec SeasonalitySpec, name string) SeasonalityEntry {
	directional := bucket.bullish + bucket.bearish
	var bullishPercent *float64
	if directional > 0 {
		value := float64(bucket.bullish) / float64(directional) * 100
		bullishPercent = &value
	}
	return SeasonalityEntry{
		Bucket: name, Bullish: bucket.bullish, Bearish: bucket.bearish, Doji: bucket.doji,
		Total: bucket.bullish + bucket.bearish + bucket.doji, DirectionalCount: directional,
		BullishPercent: bullishPercent, Classification: classifySeasonality(bullishPercent, directional, spec.MinSamples),
		MinSamples: spec.MinSamples, Lookback: spec.Lookback, LookbackDays: spec.LookbackDays,
	}
}

func classifySeasonality(percent *float64, count, minSamples int) string {
	if percent == nil || count < minSamples {
		return "insufficient_data"
	}
	switch {
	case *percent >= 76:
		return "extreme_bullish"
	case *percent >= 56:
		return "bullish"
	case *percent >= 45:
		return "neutral"
	case *percent >= 26:
		return "bearish"
	default:
		return "extreme_bearish"
	}
}

func seasonalityBucketNames(timestamp int64, dimension string) (string, string) {
	d := time.UnixMilli(timestamp).UTC()
	switch dimension {
	case "intraday":
		return fmt.Sprintf("%02d:00", d.Hour()), fmt.Sprintf("%02d:00", (d.Hour()+1)%24)
	case "dayOfWeek":
		return d.Weekday().String(), ""
	case "weekOfYear":
		_, week := d.ISOWeek()
		return fmt.Sprintf("Week %d", week), ""
	case "monthly":
		return d.Month().String(), ""
	default:
		return "unknown", ""
	}
}
