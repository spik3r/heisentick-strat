package engine

import (
	"math"
	"reflect"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// sourceProjectionIndexes maps every chart decision close to the latest
// completed source bar. A missing intraday source interval fails closed rather
// than reusing stale context. It mirrors engine/context/sourceProjection.js.
func sourceProjectionIndexes(chart, source marketdata.Series) []int {
	indexes := make([]int, chart.Len())
	for i := range indexes {
		indexes[i] = -1
	}
	chartDuration := inferSeriesDurationMs(chart)
	sourceDuration := inferSeriesDurationMs(source)
	if chartDuration <= 0 || sourceDuration <= 0 {
		return indexes
	}
	sourceIndex := -1
	for i := 0; i < chart.Len(); i++ {
		decision := chart.T[i] + chartDuration
		for sourceIndex+1 < source.Len() && source.T[sourceIndex+1]+sourceDuration <= decision {
			sourceIndex++
		}
		if sourceIndex < 0 {
			continue
		}
		sourceClose := source.T[sourceIndex] + sourceDuration
		nextOpen := decision
		if sourceIndex+1 < source.Len() {
			nextOpen = source.T[sourceIndex+1]
		}
		if decision-sourceClose >= sourceDuration && nextOpen-sourceClose >= sourceDuration {
			continue
		}
		indexes[i] = sourceIndex
	}
	return indexes
}

// projectSourceColumns retains chart-length execution while copying every
// exported context column from its causally completed source row. Reflection is
// deliberately constrained to the typed context column tree so a new context
// family cannot silently skip projection when added later.
func projectSourceColumns(chart, source marketdata.Series, sourceCols contextcols.Columns) contextcols.Columns {
	indexes := sourceProjectionIndexes(chart, source)
	projected := projectSourceValue(reflect.ValueOf(sourceCols), indexes).Interface().(contextcols.Columns)
	projected.Length = chart.Len()
	return projected
}

func projectSourceInt8(chart, source marketdata.Series, values []int8) []int8 {
	if values == nil {
		return nil
	}
	indexes := sourceProjectionIndexes(chart, source)
	out := make([]int8, len(indexes))
	for i, sourceIndex := range indexes {
		if sourceIndex >= 0 && sourceIndex < len(values) {
			out[i] = values[sourceIndex]
		}
	}
	return out
}

func projectSourceFloat64(chart, source marketdata.Series, values []float64) []float64 {
	if values == nil {
		return nil
	}
	indexes := sourceProjectionIndexes(chart, source)
	out := make([]float64, len(indexes))
	for i := range out {
		out[i] = math.NaN()
	}
	for i, sourceIndex := range indexes {
		if sourceIndex >= 0 && sourceIndex < len(values) {
			out[i] = values[sourceIndex]
		}
	}
	return out
}

func projectSourceValue(value reflect.Value, indexes []int) reflect.Value {
	switch value.Kind() {
	case reflect.Struct:
		out := reflect.New(value.Type()).Elem()
		for i := 0; i < value.NumField(); i++ {
			if out.Field(i).CanSet() {
				out.Field(i).Set(projectSourceValue(value.Field(i), indexes))
			}
		}
		return out
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeSlice(value.Type(), len(indexes), len(indexes))
		for i, sourceIndex := range indexes {
			if sourceIndex >= 0 && sourceIndex < value.Len() {
				out.Index(i).Set(value.Index(sourceIndex))
			} else if value.Type().Elem().Kind() == reflect.Float64 {
				out.Index(i).SetFloat(math.NaN())
			}
		}
		return out
	default:
		return value
	}
}
