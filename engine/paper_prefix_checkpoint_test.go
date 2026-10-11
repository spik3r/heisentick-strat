package engine

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPaperEverySplitAndCrashRetry(t *testing.T) {
	for _, fill := range []string{"close", "nextOpen"} {
		r := paperORBRequest(t, fill)
		full, err := RunPaperPrefix(r)
		if err != nil {
			t.Fatal(err)
		}
		for split := 1; split <= len(r.Bars); split++ {
			prior, err := RunPaperPrefix(paperPrefix(r, split))
			if err != nil {
				t.Fatal(err)
			}
			next := r
			next.Checkpoint = prior.Checkpoint
			resumed, err := RunPaperPrefix(next)
			if err != nil {
				t.Fatal(err)
			}
			combined := append(append([]PaperEvent{}, prior.Events...), resumed.Events...)
			if !reflect.DeepEqual(combined, full.Events) || !reflect.DeepEqual(resumed.Account, full.Account) || !reflect.DeepEqual(resumed.CompletedTrades, full.CompletedTrades) || !reflect.DeepEqual(resumed.OpenPositions, full.OpenPositions) || string(resumed.Checkpoint) != string(full.Checkpoint) {
				t.Fatalf("split %d differs", split)
			}
			retry, err := RunPaperPrefix(next)
			if err != nil || !reflect.DeepEqual(retry, resumed) {
				t.Fatalf("crash retry %d: %v", split, err)
			}
			next.Checkpoint = resumed.Checkpoint
			duplicate, err := RunPaperPrefix(next)
			if err != nil || len(duplicate.Events) != 0 || duplicate.EventRange.From != duplicate.EventRange.To || string(duplicate.Checkpoint) != string(full.Checkpoint) {
				t.Fatalf("duplicate %d: %v", split, err)
			}
		}
	}
}

func TestPaperCheckpointTamperAndImmutableBinding(t *testing.T) {
	r := paperORBRequest(t, "close")
	prior, err := RunPaperPrefix(paperPrefix(r, 4))
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*PaperPrefixRequest){
		func(r *PaperPrefixRequest) { r.Source += "\n# changed source" }, func(r *PaperPrefixRequest) { r.Costs.FeePerUnit += .1 }, func(r *PaperPrefixRequest) { r.ActivationFromT += 300000 }, func(r *PaperPrefixRequest) { r.Units.QuantityUnit = "different" }, func(r *PaperPrefixRequest) { r.Route.StrategyID = "different" }, func(r *PaperPrefixRequest) {
			r.Bars = append([][]float64{}, r.Bars...)
			r.Bars[0] = append([]float64{}, r.Bars[0]...)
			r.Bars[0][5]++
		},
	} {
		bad := r
		bad.Checkpoint = prior.Checkpoint
		mutate(&bad)
		if result, err := RunPaperPrefix(bad); err == nil || result.Schema != "" {
			t.Fatal("changed binding/input accepted")
		}
	}
	var cp map[string]any
	json.Unmarshal(prior.Checkpoint, &cp)
	for _, key := range []string{"schema", "bindingDigest", "inputDigest", "stateDigest", "eventCount", "lastBarT", "processedBars", "closedThroughT"} {
		copy := map[string]any{}
		for k, v := range cp {
			copy[k] = v
		}
		switch v := copy[key].(type) {
		case string:
			copy[key] = v + "x"
		case float64:
			copy[key] = v + 1
		}
		raw, _ := json.Marshal(copy)
		bad := r
		bad.Checkpoint = raw
		if _, err := RunPaperPrefix(bad); err == nil {
			t.Fatalf("tamper %s", key)
		}
	}
	for _, raw := range []string{`{}`, `null null`, `{"schema":"heisentick-prefix-checkpoint-v1"}`, string(prior.Checkpoint) + ` {}`} {
		bad := r
		bad.Checkpoint = []byte(raw)
		if _, err := RunPaperPrefix(bad); err == nil {
			t.Fatal("bad checkpoint accepted")
		}
	}
	// Even a correctly rehashed caller assertion cannot replace replayed state.
	cp["stateDigest"] = paperSourceDigest("invented mutable account state")
	bad := r
	bad.Checkpoint, _ = json.Marshal(cp)
	if _, err := RunPaperPrefix(bad); err == nil {
		t.Fatal("self-consistent foreign state accepted")
	}
}

func TestPaperPrefixExtensionKeepsCausalEvents(t *testing.T) {
	r := paperORBRequest(t, "nextOpen")
	full, err := RunPaperPrefix(r)
	if err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= len(r.Bars); n++ {
		prefix, err := RunPaperPrefix(paperPrefix(r, n))
		if err != nil {
			t.Fatal(err)
		}
		expected := []PaperEvent{}
		for _, e := range full.Events {
			if e.Index < n {
				expected = append(expected, e)
			}
		}
		if !reflect.DeepEqual(prefix.Events, expected) {
			t.Fatalf("future bars changed prior events at %d", n)
		}
	}
}
