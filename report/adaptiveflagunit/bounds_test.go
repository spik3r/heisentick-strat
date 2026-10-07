package adaptiveflagunit

import (
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestReviewedProfileAndCompleteActualTypeBound(t *testing.T) {
	got, err := DesignOutputBound(1024, 366, 732, 1024)
	if err != nil || got != 52429496 || got > MaxOutputBytes {
		t.Fatalf("design %d %v", got, err)
	}
	terms, err := ActualTypeBounds()
	if err != nil {
		t.Fatal(err)
	}
	limits := BoundTerms{Fixed: FixedAllowance, Orders: 4096, ClosedTrades: 12288, CostEvents: 2048, Marks: 4096, Rejections: 128, Daily: 2048, EntryYear: 2048, CalendarYear: 2048, Midnights: 32, Gaps: 256, Error: MaxErrorBytes}
	a, b := reflect.ValueOf(terms), reflect.ValueOf(limits)
	for i := 0; i < a.NumField(); i++ {
		if a.Field(i).Uint() > b.Field(i).Uint() {
			t.Errorf("%s derived%d exceeds%d", a.Type().Field(i).Name, a.Field(i).Uint(), b.Field(i).Uint())
		}
	}
	generated, err := GeneratedOutputBound(1024, 366, 732, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if generated > got {
		t.Fatalf("generated%d exceeds reviewed%d", generated, got)
	}
	t.Logf("actual terms %+v; generated %d; reviewed %d", terms, generated, got)
	for _, v := range [][4]uint64{{0, 1, 0, 0}, {1025, 1, 0, 0}, {1, 0, 0, 0}, {1, 367, 0, 0}, {1, 1, 733, 0}, {1, 1, 0, 1025}, {math.MaxUint64, 1, 0, 0}} {
		if _, err := GeneratedOutputBound(v[0], v[1], v[2], v[3]); err == nil {
			t.Errorf("accepted excessive cardinalities%v", v)
		}
	}
}

// These values maximize wire shapes, not valid financial ledgers. No strategy
// or accounting code consumes them. Every dynamically escaped string is tested.
func maximumShape(t *testing.T, typ reflect.Type, f reflect.StructField, owner reflect.Type) reflect.Value {
	t.Helper()
	v := reflect.New(typ).Elem()
	switch typ.Kind() {
	case reflect.Pointer:
		v.Set(reflect.New(typ.Elem()))
		v.Elem().Set(maximumShape(t, typ.Elem(), f, owner))
	case reflect.Struct:
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if field.PkgPath == "" {
				v.Field(i).Set(maximumShape(t, field.Type, field, typ))
			}
		}
	case reflect.String:
		if literal := f.Tag.Get("unitliteral"); literal != "" {
			v.SetString(literal)
			break
		}
		n, err := strconv.Atoi(stringLimit(f, owner))
		if err != nil {
			t.Fatalf("unbounded string%s.%s", owner, f.Name)
		}
		v.SetString(strings.Repeat("\x00", n))
	case reflect.Float64:
		v.SetFloat(-math.MaxFloat64)
	case reflect.Int, reflect.Int64:
		// Both32/64 bit targets are covered by a20-byte bound; host values stay valid.
		if typ.Bits() == 32 {
			v.SetInt(math.MinInt32)
		} else {
			v.SetInt(math.MinInt64)
		}
	case reflect.Bool:
		v.SetBool(false)
	case reflect.Slice:
		v.Set(reflect.MakeSlice(typ, 0, 0))
	case reflect.Map:
		v.Set(reflect.MakeMap(typ))
	default:
		t.Fatalf("unknown maximum shape%s", typ)
	}
	return v
}
func shape[T any](t *testing.T) T {
	var zero T
	return maximumShape(t, reflect.TypeOf(zero), reflect.StructField{}, nil).Interface().(T)
}

func TestMaximumRepeatedCollectionsFitBothBounds(t *testing.T) {
	e := shape[Envelope](t)
	// A real raw report is not run by B0. Its independently proven byte allowance
	// is charged separately. RawMessage contains an invented empty object here.
	e.Raw = json.RawMessage("{}")
	order := shape[ProjectedOrder](t)
	trade := shape[ClosedTrade](t)
	cost := shape[CostEvent](t)
	mark := shape[Mark](t)
	reject := shape[InvalidRiskRejection](t)
	day := shape[Daily](t)
	for i := 0; i < MaxRetainedRows; i++ {
		e.Projection.Orders = append(e.Projection.Orders, order)
		e.Projection.ClosedTrades = append(e.Projection.ClosedTrades, trade)
		e.Projection.CostEvents = append(e.Projection.CostEvents, cost, cost)
		e.Projection.Marks = append(e.Projection.Marks, mark)
		e.Projection.InvalidPlannedRiskRejections = append(e.Projection.InvalidPlannedRiskRejections, reject)
	}
	for i := 0; i < MaxCalendarDays; i++ {
		e.Projection.Daily = append(e.Projection.Daily, day)
		year := strconv.Itoa(2000 + i)
		e.Projection.ByEntryYearCohort[year] = shape[Summary](t)
		e.Projection.ByCalendarYearMarkedChange[year] = shape[CalendarYear](t)
	}
	// Put all globally budgeted variable exposure lists in the extra terminal
	// object; repeated closed-trade scalar exposures still occur1024 times.
	for i := 0; i < MaxExposureMidnights/2; i++ {
		e.Projection.Terminal.Exposure.UTCMidnightsDefinitelyCrossed = append(e.Projection.Terminal.Exposure.UTCMidnightsDefinitelyCrossed, math.MinInt64)
		e.Projection.Terminal.Exposure.UTCMidnightsPossiblyCrossed = append(e.Projection.Terminal.Exposure.UTCMidnightsPossiblyCrossed, math.MinInt64)
	}
	for i := 0; i < MaxExposureGaps; i++ {
		e.Projection.Terminal.Exposure.QuoteGapsBridged = append(e.Projection.Terminal.Exposure.QuoteGapsBridged, shape[ExposureGap](t))
	}
	encoded, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	terms, err := ActualTypeBounds()
	if err != nil {
		t.Fatal(err)
	}
	projectionBound := terms.Fixed + 1024*(terms.Orders+terms.ClosedTrades+2*terms.CostEvents+terms.Marks+terms.Rejections) + 366*(terms.Daily+terms.EntryYear+terms.CalendarYear) + 732*terms.Midnights + 1024*terms.Gaps
	if uint64(len(encoded)+1) > projectionBound {
		t.Fatalf("shape%d exceeds generated%d", len(encoded)+1, projectionBound)
	}
	t.Logf("complete maximum collection shape bytes%d; generated projection allowance%d", len(encoded)+1, projectionBound)
	errShape := shape[ErrorEnvelope](t)
	errBytes, err := json.Marshal(errShape)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(len(errBytes)+1) > terms.Error || len(errBytes)+1 > MaxErrorBytes {
		t.Fatal("error shape exceeds bound")
	}
	// Both absent and emitted-null fill-only fields must be representable without
	// custom marshaling (which might hide semantics in the B0 declarations).
	empty := ProjectedOrder{}
	b, _ := json.Marshal(empty)
	if strings.Contains(string(b), "raw_entry") {
		t.Fatal("unfilled field emitted")
	}
	var absent *float64
	empty.DisplayProxyAdjustedEntry = &absent
	b, _ = json.Marshal(empty)
	if !strings.Contains(string(b), `"display_proxy_adjusted_entry":null`) {
		t.Fatal("filled diagnostic null lost")
	}
}

func TestBoundFailsOnUnknownCollectionsAndStrings(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(struct {
		New []float64 `json:"new"`
	}{}), reflect.TypeOf(struct {
		Reason string `json:"reason"`
	}{})} {
		if _, err := compactTypeBound(typ, reflect.StructField{}, nil); err == nil {
			t.Errorf("unreviewed type admitted%s", typ)
		}
	}
}
