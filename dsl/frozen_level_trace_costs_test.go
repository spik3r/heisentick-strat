package dsl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
)

func pilotCostJSON(fee string) []byte {
	return []byte(`{"schema":"frozen-quote-costs-pilot-v1","id":"SyntheticCosts","version":"v1","sourceRef":{"id":"SyntheticSource","version":"v1","sha256":"` + strings.Repeat("a", 64) + `"},"currency":"USD","quantityUnits":"troy-ounce","spread":"observed-paired-quotes-no-second-charge","commissionPerOzPerFillUSD":` + fee + `,"commissionTiming":"entry-and-exit-on-fill","extraAdversePricePenalty":0,"financing":"not-applicable-synthetic-intraday-fixture"}`)
}
func TestFrozenTraceCostsExactRawBoundary(t *testing.T) {
	for _, fee := range []string{"0", "0.03", "0.30", "3e-2", "0.0000000000000000001"} {
		c, e := DecodeFrozenTraceCostsJSON(pilotCostJSON(fee))
		if e != nil || c.CommissionPerOzPerFillUSD != fee || c.ID != "SyntheticCosts" {
			t.Fatalf("exact fee token %s: %v", fee, e)
		}
	}
	for _, fee := range []string{`"0.03"`, "null", "true", "-0", "-0.0e0", "-0.03", "NaN", "Infinity", "1e309", "1e-10000", "1e10001", strings.Repeat("1", 1025)} {
		c, e := DecodeFrozenTraceCostsJSON(pilotCostJSON(fee))
		if e == nil || c.ID != "" || c.CommissionPerOzPerFillUSD != "" {
			t.Fatalf("bad fee %s accepted or partial DTO", fee)
		}
	}
	for _, penalty := range []string{"0.01", "-0", "null", `"0"`} {
		raw := bytes.Replace(pilotCostJSON("0.03"), []byte(`"extraAdversePricePenalty":0`), []byte(`"extraAdversePricePenalty":`+penalty), 1)
		if _, e := DecodeFrozenTraceCostsJSON(raw); e == nil {
			t.Fatal("unsupported/invalid penalty accepted")
		}
	}
	raw := pilotCostJSON("0.03")
	var root map[string]any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if e := d.Decode(&root); e != nil {
		t.Fatal(e)
	}
	count := 0
	check := func(m map[string]any) {
		keys := make([]string, 0, len(m))
		for key := range m {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			old := m[key]
			delete(m, key)
			b, _ := json.Marshal(root)
			if _, e := DecodeFrozenTraceCostsJSON(b); e == nil {
				t.Fatal("missing field", key)
			}
			count++
			m[key] = nil
			b, _ = json.Marshal(root)
			if _, e := DecodeFrozenTraceCostsJSON(b); e == nil {
				t.Fatal("null field", key)
			}
			count++
			m[key] = old
		}
		m["extra"] = true
		b, _ := json.Marshal(root)
		if _, e := DecodeFrozenTraceCostsJSON(b); e == nil {
			t.Fatal("unknown field")
		}
		count++
		delete(m, "extra")
	}
	check(root)
	check(root["sourceRef"].(map[string]any))
	if count != 30 {
		t.Fatalf("missing cost-field mutation coverage: %d", count)
	}
	for _, bad := range [][]byte{bytes.Replace(raw, []byte(`"id":`), []byte(`"\u0069d":"Duplicate","id":`), 1), bytes.Replace(raw, []byte(`"id":"SyntheticCosts"`), []byte(`"id":"\ud800"`), 1), append([]byte{255}, raw...), append(append([]byte(nil), raw...), []byte("{}")...), bytes.Replace(raw, []byte(`"currency":"USD"`), []byte(`"currency":"EUR"`), 1), bytes.Replace(raw, []byte(`"financing":"not-applicable-synthetic-intraday-fixture"`), []byte(`"financing":"zero-default"`), 1)} {
		if _, e := DecodeFrozenTraceCostsJSON(bad); e == nil {
			t.Fatal("bad pilot cost transport/terms accepted")
		}
	}
	fmt.Printf("SYNTHETIC_COST_BOUNDARY=%d\n", count)
}
