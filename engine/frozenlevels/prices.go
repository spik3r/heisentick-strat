package frozenlevels

import "math/big"

// price is the exact rational value of the shortest base-10 representation of
// a finite input float. Off-grid input prices are retained without rounding.
// All arithmetic creates a fresh value; no caller can mutate a retained price.
// Conversion to float is presentation only and never drives a threshold test.
type price struct{ value *big.Rat }

func inputPrice(x float64) price  { return price{decimal(x)} }
func (p price) add(q price) price { return price{new(big.Rat).Add(p.value, q.value)} }
func (p price) sub(q price) price { return price{new(big.Rat).Sub(p.value, q.value)} }
func (p price) mul(q price) price { return price{new(big.Rat).Mul(p.value, q.value)} }
func (p price) cmp(q price) int   { return p.value.Cmp(q.value) }
func (p price) sign() int         { return p.value.Sign() }
func (p price) project() float64  { f, _ := p.value.Float64(); return f }
func directedDifference(a, b price, side Direction) price {
	return a.sub(b).mul(inputPrice(float64(side)))
}
func (p price) snap(grid price, up bool) price {
	scaled := new(big.Rat).Quo(p.value, grid.value)
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(scaled.Num(), scaled.Denom(), r)
	if up && r.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	return price{new(big.Rat).Mul(new(big.Rat).SetInt(q), grid.value)}
}
