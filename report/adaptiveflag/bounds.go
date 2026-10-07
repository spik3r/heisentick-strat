package adaptiveflag

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/spik3r/heisentick-strat/engine"
)

// RuntimeOutputBound covers each compact internal engine JSON allocation and
// the larger complete pretty runtime envelope before either allocation happens.
// This is not a process/RSS guarantee: Go/JS/caller copies may coexist.
func RuntimeOutputBound(rows uint64) (uint64, error) {
	if rows < 1 || rows > MaxRetainedRows {
		return 0, fmt.Errorf("adaptive runtime output bound requires 1..4096 retained rows")
	}
	run, err := boundJSON(reflect.TypeOf(engine.AdaptiveFlagResult{}), 1, "", rows)
	if err != nil {
		return 0, err
	}
	return run + 6*MaxSourceBytes + 65536 + 65536, nil
}

func boundJSON(t reflect.Type, depth uint64, name string, n uint64) (uint64, error) {
	switch t.Kind() {
	case reflect.Pointer:
		v, e := boundJSON(t.Elem(), depth, name, n)
		return v + 4, e
	case reflect.Bool:
		return 5, nil
	case reflect.Int, reflect.Int64, reflect.Float64:
		return 32, nil
	case reflect.String:
		m := uint64(1024)
		switch name {
		case "Side":
			m = 8
		case "Status":
			m = 16
		case "Reason":
			m = 32
		case "Kind":
			m = 64
		case "State":
			m = 32
		case "Basis":
			m = 256
		}
		return 6*m + 2, nil
	case reflect.Struct:
		v := uint64(2) + 2*depth + 1
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}
			tag := strings.Split(f.Tag.Get("json"), ",")[0]
			if tag == "-" {
				continue
			}
			if tag == "" {
				tag = f.Name
			}
			// Baseline runtime always passes zero overlay and rejects research output.
			if t == reflect.TypeOf(engine.AdaptiveFlagResult{}) && (f.Name == "ResearchPolicy" || f.Name == "ResearchPolicySHA256" || f.Name == "ResearchProducer") {
				continue
			}
			field, err := boundJSON(f.Type, depth+1, f.Name, n)
			if err != nil {
				return 0, err
			}
			// Counting embedded structs as nested objects deliberately overestimates
			// their flattened JSON representation, including keys and indentation.
			v += 2*(depth+1) + 6*uint64(len(tag)) + 2 + 2 + field + 2
		}
		return v, nil
	case reflect.Slice:
		count := n
		switch t.Elem().Name() {
		case "AdaptiveFlagEvent":
			count = 4 * n
		case "AdaptiveFlagGap", "AdaptiveFlagSnapshot", "AdaptiveFlagOrder", "AdaptiveFlagState":
		case "int":
			count = 26
		case "string":
			count = 13
		default:
			return 0, fmt.Errorf("adaptive runtime unreviewed output slice %s", t)
		}
		element, err := boundJSON(t.Elem(), depth+1, name, n)
		if err != nil {
			return 0, err
		}
		return 2 + 2*depth + 1 + count*(2*(depth+1)+element+2), nil
	default:
		return 0, fmt.Errorf("adaptive runtime unreviewed output type %s", t)
	}
}
