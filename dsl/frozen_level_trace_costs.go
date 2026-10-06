package dsl

import (
	"encoding/json"
	"fmt"
)

// FrozenTraceCosts is an inspection value, not authority to run a strategy.
// The fee retains its original exact decimal token; no rounded float drives
// monetary reconciliation. Only the explicit synthetic intraday pilot is read.
type FrozenTraceCosts struct {
	ID, Version               string
	SourceRef                 FrozenQuoteRef
	CommissionPerOzPerFillUSD string
}

func DecodeFrozenTraceCostsJSON(raw []byte) (FrozenTraceCosts, error) {
	v, err := strictFrozenJSON(raw)
	if err != nil {
		return FrozenTraceCosts{}, err
	}
	r := frozenInputReader{}
	m := r.object(v, "costs", "schema", "id", "version", "sourceRef", "currency", "quantityUnits", "spread", "commissionPerOzPerFillUSD", "commissionTiming", "extraAdversePricePenalty", "financing")
	r.text(m["schema"], "costs.schema", "frozen-quote-costs-pilot-v1")
	r.text(m["currency"], "costs.currency", "USD")
	r.text(m["quantityUnits"], "costs.quantityUnits", "troy-ounce")
	r.text(m["spread"], "costs.spread", "observed-paired-quotes-no-second-charge")
	r.text(m["commissionTiming"], "costs.commissionTiming", "entry-and-exit-on-fill")
	r.text(m["financing"], "costs.financing", "not-applicable-synthetic-intraday-fixture")
	c := FrozenTraceCosts{ID: r.registry(m["id"], "costs.id", 64), Version: r.registry(m["version"], "costs.version", 32), SourceRef: r.ref(m["sourceRef"], "costs.sourceRef")}
	for _, key := range []string{"commissionPerOzPerFillUSD", "extraAdversePricePenalty"} {
		n, ok := m[key].(json.Number)
		if !ok {
			r.fail(fmt.Errorf("costs.%s requires a raw JSON number", key))
			continue
		}
		p, e := frozenNumberParts(string(n))
		if e != nil {
			r.fail(e)
			continue
		}
		if p.negative {
			r.fail(fmt.Errorf("costs.%s must be nonnegative", key))
			continue
		}
		if key == "extraAdversePricePenalty" {
			if p.digits != "" {
				r.fail(fmt.Errorf("costs.extraAdversePricePenalty is unsupported unless exactly zero"))
			}
		} else {
			if p.digits != "" {
				_, e = frozenConfigDecimal(n, "costs."+key)
				r.fail(e)
			}
			c.CommissionPerOzPerFillUSD = string(n)
		}
	}
	if r.err != nil {
		return FrozenTraceCosts{}, r.err
	}
	return c, nil
}
