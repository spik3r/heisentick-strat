package engine

import (
	"reflect"

	"github.com/spik3r/heisentick-strat/dsl"
)

func rawLengthNonempty(value any) bool {
	raw := reflect.ValueOf(value)
	if !raw.IsValid() {
		return false
	}
	switch raw.Kind() {
	case reflect.String, reflect.Slice, reflect.Array:
		return raw.Len() > 0
	default:
		return false
	}
}

func normalizedEntryMode(value string) string {
	switch value {
	case "limit", "limitSweptEdge":
		return "limit"
	default:
		return "market"
	}
}

func (p flagParams) familyDefaultStopLookback() int {
	switch p.SetupType {
	case string(dsl.FamilyRangeBreakFake):
		return 3
	case string(dsl.FamilyDayOpenReclaim):
		return 5
	default:
		return 0
	}
}
