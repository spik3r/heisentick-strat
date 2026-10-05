package frozenlevels

import (
	"fmt"
	"math/big"
	"strconv"
)

type Direction int

const (
	Long  Direction = 1
	Short Direction = -1
)

func (d Direction) valid() bool { return d == Long || d == Short }

// ScaleLocation identifies LOCATION only. It does not define initial risk,
// activation, a target, or a broker-valid executable stop.
type ScaleLocation struct {
	Version           string  `json:"version"`
	OpeningID         string  `json:"openingId"`
	Divisor           float64 `json:"divisor"`
	UnroundedDistance float64 `json:"unroundedDistance"`
	CentDistance      float64 `json:"centDistance"`
	Stop              float64 `json:"stop"`
	Grid              float64 `json:"grid"`
	Rounding          string  `json:"rounding"`
}

func decimal(x float64) *big.Rat {
	r, ok := new(big.Rat).SetString(strconv.FormatFloat(x, 'f', -1, 64))
	if !ok {
		panic("validated finite decimal")
	}
	return r
}
func halfEvenInteger(x *big.Rat) *big.Int {
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(x.Num(), x.Denom(), r)
	cmp := new(big.Int).Lsh(r, 1).Cmp(x.Denom())
	if cmp > 0 || (cmp == 0 && q.Bit(0) == 1) {
		q.Add(q, big.NewInt(1))
	}
	return q
}
func gridSnap(raw *big.Rat, grid float64, up bool) float64 {
	return (price{raw}).snap(inputPrice(grid), up).project()
}
func PriorOpenScaleLocation(f FrozenOpening, entry float64, side Direction, grid float64) (ScaleLocation, error) {
	location, _, err := priorOpenScaleLocation(f, entry, side, grid)
	return location, err
}
func priorOpenScaleLocation(f FrozenOpening, entry float64, side Direction, grid float64) (ScaleLocation, price, error) {
	copy := f
	copy.ID = ""
	if f.Version != ContractVersion || f.ID == "" || identity(copy) != f.ID {
		return ScaleLocation{}, price{}, fmt.Errorf("invalid-opening-identity")
	}
	if !finitePositive(entry) || !side.valid() || !finitePositive(grid) {
		return ScaleLocation{}, price{}, fmt.Errorf("invalid-scale-parameters")
	}
	rawDistance := f.Opening.Price / 3800
	if !finitePositive(rawDistance) {
		return ScaleLocation{}, price{}, fmt.Errorf("invalid-scale-distance")
	}
	// Preserve the archival decimal(str(float division)) half-even convention.
	cents := halfEvenInteger(new(big.Rat).Mul(decimal(rawDistance), big.NewRat(100, 1)))
	distance := new(big.Rat).Quo(new(big.Rat).SetInt(cents), big.NewRat(100, 1))
	dist, _ := distance.Float64()
	signed := new(big.Rat).Mul(distance, big.NewRat(int64(side), 1))
	raw := new(big.Rat).Add(decimal(entry), signed)
	if raw.Sign() <= 0 {
		return ScaleLocation{}, price{}, fmt.Errorf("nonpositive-scale-stop")
	}
	exactStop := (price{raw}).snap(inputPrice(grid), side == Short)
	stop := exactStop.project()
	if !finitePositive(stop) || directedDifference(exactStop, inputPrice(entry), side).sign() <= 0 {
		return ScaleLocation{}, price{}, fmt.Errorf("scale-stop-not-profitable")
	}
	return ScaleLocation{Version: ContractVersion, OpeningID: f.ID, Divisor: 3800, UnroundedDistance: rawDistance, CentDistance: dist, Stop: stop, Grid: grid, Rounding: "decimal-float-quotient-half-even-cents-then-grid-toward-entry"}, exactStop, nil
}
