package adaptiveflagunit

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
)

const (
	MaxRetainedRows                  = 1024
	MaxCalendarDays                  = 366
	MaxExposureMidnights             = 732
	MaxExposureGaps                  = 1024
	MaxProjectionMetadataBytes       = 1024
	MaxOutputBytes                   = 64 * 1024 * 1024
	MaxErrorBytes                    = 8192
	MaxCalendarEndpointMS      int64 = 253402300800000
	FixedAllowance                   = 1048576
)

// BoundTerms is a reviewable type-derived compact-JSON bound. Repeated elements
// include a comma/key allowance. Exposure variable lists are counted globally,
// not once per position. This bounds encoded bytes, never peak RSS.
type BoundTerms struct {
	Fixed, Orders, ClosedTrades, CostEvents, Marks, Rejections uint64
	Daily, EntryYear, CalendarYear, Midnights, Gaps, Error     uint64
}

func cardinalities(n, d, e, g uint64) error {
	if n < 1 || n > MaxRetainedRows || d < 1 || d > MaxCalendarDays || e > MaxExposureMidnights || g > MaxExposureGaps {
		return fmt.Errorf("unit bound cardinality outside B0 profile")
	}
	return nil
}

// DesignOutputBound implements the exact independently reviewed v2 allowance.
func DesignOutputBound(n, d, e, g uint64) (uint64, error) {
	if err := cardinalities(n, d, e, g); err != nil {
		return 0, err
	}
	raw, err := adaptiveflag.RuntimeOutputBound(n)
	if err != nil {
		return 0, err
	}
	return raw + FixedAllowance + n*(4096+12288+2*2048+4096+128) + d*(2048+2*2048) + 32*e + 256*g, nil
}

// ActualTypeBounds rejects newly introduced unreviewed fields, collections and
// strings rather than silently borrowing a typical-size estimate.
func ActualTypeBounds() (BoundTerms, error) {
	var out BoundTerms
	entries := []struct {
		dst   *uint64
		typ   reflect.Type
		extra uint64
	}{
		{&out.Fixed, reflect.TypeOf(Envelope{}), 1},
		{&out.Orders, reflect.TypeOf(ProjectedOrder{}), 1},
		{&out.ClosedTrades, reflect.TypeOf(ClosedTrade{}), 1},
		{&out.CostEvents, reflect.TypeOf(CostEvent{}), 1},
		{&out.Marks, reflect.TypeOf(Mark{}), 1},
		{&out.Rejections, reflect.TypeOf(InvalidRiskRejection{}), 1},
		{&out.Daily, reflect.TypeOf(Daily{}), 1},
		{&out.EntryYear, reflect.TypeOf(Summary{}), 8}, // four ASCII year digits, quotes, colon, comma
		{&out.CalendarYear, reflect.TypeOf(CalendarYear{}), 8},
		{&out.Gaps, reflect.TypeOf(ExposureGap{}), 1},
		{&out.Error, reflect.TypeOf(ErrorEnvelope{}), 1},
	}
	for _, entry := range entries {
		v, err := compactTypeBound(entry.typ, reflect.StructField{}, nil)
		if err != nil {
			return out, err
		}
		*entry.dst = v + entry.extra
	}
	out.Midnights = 21 // signed int64 at most20 bytes plus comma; admitted dates need fewer
	return out, nil
}

func GeneratedOutputBound(n, d, e, g uint64) (uint64, error) {
	if err := cardinalities(n, d, e, g); err != nil {
		return 0, err
	}
	terms, err := ActualTypeBounds()
	if err != nil {
		return 0, err
	}
	raw, err := adaptiveflag.RuntimeOutputBound(n)
	if err != nil {
		return 0, err
	}
	return raw + terms.Fixed + n*(terms.Orders+terms.ClosedTrades+2*terms.CostEvents+terms.Marks+terms.Rejections) + d*(terms.Daily+terms.EntryYear+terms.CalendarYear) + e*terms.Midnights + g*terms.Gaps, nil
}

// The keys below are the complete collection cardinality contract. Any new
// collection is an error until explicitly assigned to a repeated-object term.
func collectionIsExternal(owner reflect.Type, field string) bool {
	switch owner {
	case reflect.TypeOf(Envelope{}):
		return field == "Raw"
	case reflect.TypeOf(Projection{}):
		switch field {
		case "Orders", "InvalidPlannedRiskRejections", "CostEvents", "ClosedTrades", "ByEntryYearCohort", "ByCalendarYearMarkedChange", "Marks", "Daily":
			return true
		}
	case reflect.TypeOf(Exposure{}):
		switch field {
		case "QuoteGapsBridged", "UTCMidnightsDefinitelyCrossed", "UTCMidnightsPossiblyCrossed":
			return true
		}
	}
	return false
}

func compactTypeBound(t reflect.Type, f reflect.StructField, owner reflect.Type) (uint64, error) {
	switch t.Kind() {
	case reflect.Pointer:
		v, e := compactTypeBound(t.Elem(), f, owner)
		if v < 4 {
			v = 4
		}
		return v, e
	case reflect.Bool:
		return 5, nil
	case reflect.Int, reflect.Int64:
		return 20, nil
	case reflect.Float64:
		return 32, nil
	case reflect.String:
		if literal := f.Tag.Get("unitliteral"); literal != "" {
			return uint64(len(strconv.Quote(literal))), nil
		}
		length := stringLimit(f, owner)
		n, err := strconv.ParseUint(length, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("unbounded string %s", f.Name)
		}
		return 6*n + 2, nil // every UTF8 input byte may become a six-byte JSON escape
	case reflect.Struct:
		sum := uint64(2)
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" && field.Anonymous {
				v, err := compactTypeBound(field.Type, field, t)
				if err != nil {
					return 0, err
				}
				sum += v
				continue // braces deliberately overcount flattening
			}
			if name == "" {
				return 0, fmt.Errorf("unreviewed untagged field %s.%s", t.Name(), field.Name)
			}
			var v uint64
			var err error
			if collectionIsExternal(t, field.Name) {
				if expectedCollectionType(t, field.Name) != field.Type {
					return 0, fmt.Errorf("reviewed collection type changed: %s.%s", t.Name(), field.Name)
				}
				v = 2
			} else {
				v, err = compactTypeBound(field.Type, field, t)
			}
			if err != nil {
				return 0, err
			}
			// Existing tag keys are ASCII and are encoded using exactly their quoted bytes.
			sum += uint64(len(strconv.Quote(name))) + 1 + v + 1
		}
		return sum, nil
	default:
		return 0, fmt.Errorf("unreviewed type/collection %s for %s", t, f.Name)
	}
}

func stringLimit(f reflect.StructField, owner reflect.Type) string {
	if n := f.Tag.Get("unitmax"); n != "" {
		return n
	}
	switch owner {
	case reflect.TypeOf(dsl.AdaptiveFlagSpec{}):
		switch f.Name {
		case "Policy", "NumericalPolicy":
			return "64"
		case "Timeframe":
			return "3"
		case "Bundle":
			return "10"
		}
	case reflect.TypeOf(engine.AdaptiveFlagEventTime{}):
		if f.Name == "Kind" {
			return "40"
		}
	case reflect.TypeOf(engine.AdaptiveFlagQueuedExit{}):
		if f.Name == "Reason" {
			return "32"
		}
	case reflect.TypeOf(engine.AdaptiveFlagState{}):
		if f.Name == "Status" {
			return "16"
		}
	}
	return ""
}

// Every globally charged collection must retain the element type whose term is
// independently bounded above. A field-name match alone is not a type proof.
func expectedCollectionType(owner reflect.Type, name string) reflect.Type {
	if owner == reflect.TypeOf(Envelope{}) && name == "Raw" {
		return reflect.TypeOf(json.RawMessage{})
	}
	if owner == reflect.TypeOf(Projection{}) {
		switch name {
		case "Orders":
			return reflect.TypeOf([]ProjectedOrder{})
		case "InvalidPlannedRiskRejections":
			return reflect.TypeOf([]InvalidRiskRejection{})
		case "CostEvents":
			return reflect.TypeOf([]CostEvent{})
		case "ClosedTrades":
			return reflect.TypeOf([]ClosedTrade{})
		case "ByEntryYearCohort":
			return reflect.TypeOf(map[string]Summary{})
		case "ByCalendarYearMarkedChange":
			return reflect.TypeOf(map[string]CalendarYear{})
		case "Marks":
			return reflect.TypeOf([]Mark{})
		case "Daily":
			return reflect.TypeOf([]Daily{})
		}
	}
	if owner == reflect.TypeOf(Exposure{}) {
		switch name {
		case "QuoteGapsBridged":
			return reflect.TypeOf([]ExposureGap{})
		case "UTCMidnightsDefinitelyCrossed", "UTCMidnightsPossiblyCrossed":
			return reflect.TypeOf([]int64{})
		}
	}
	return nil
}
