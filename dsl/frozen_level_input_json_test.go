package dsl

import (
	"fmt"
	"math/big"
	"strings"
	"testing"
)

// An independent rational oracle checks the actual original-byte decoder's
// side-order boundary, including float-equal pairs and decimal exponent aliases.
func TestFrozenInputExactPriceOrderOracle(t *testing.T) {
	values := []string{"1e-100", "1e-20", "0.01", "0.01000000000000000001", "0.1", "0.10", "1", "1.0", "1e0", "9.9999999999999999999", "10", "10.00000000000000001", "99.99999999999999999", "100", "100.00000000000000001", "100.03000000000000002", "1e3", "1e20", "1e100", "1e308"}
	count := 0
	for _, bid := range values {
		for _, ask := range values {
			b, bok := new(big.Rat).SetString(bid)
			a, aok := new(big.Rat).SetString(ask)
			if !bok || !aok {
				t.Fatal("oracle token")
			}
			raw := []byte(fmt.Sprintf(`{"schema":"frozen-quote-snapshot-v1","id":"Synthetic","version":"v1","dataset":"Synthetic","symbol":"XAUUSD","priceUnits":"USD-per-troy-ounce","sides":"paired-bid-ask","sequenceMethod":"provider-value","rows":[{"eventMS":0,"sequence":0,"availableMS":0,"bid":%s,"ask":%s}]}`, bid, ask))
			s, e := DecodeFrozenQuoteSnapshotJSON(raw)
			crossed := b.Cmp(a) > 0
			if crossed {
				if e == nil || !strings.Contains(e.Error(), "raw bid exceeds ask") || s.Rows != nil {
					t.Fatalf("crossed %s/%s was not atomically rejected: %v", bid, ask, e)
				}
			} else if e != nil || len(s.Rows) != 1 {
				t.Fatalf("valid %s/%s: %v", bid, ask, e)
			}
			count++
		}
	}
	fmt.Printf("FROZEN_INPUT_PRICE_ORACLE=%d\n", count)
}
