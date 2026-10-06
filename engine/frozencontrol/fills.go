package frozencontrol

import (
	"fmt"
	"math"
	"math/big"

	"github.com/spik3r/heisentick-strat/engine/frozenlevels"
)

type fill struct {
	At     frozenlevels.EventKey `json:"at"`
	Side   string                `json:"quoteSide"`
	Price  float64               `json:"price"`
	Reason string                `json:"reason"`
}

func entryFill(q frozenlevels.Quote, side frozenlevels.Direction) fill {
	f := fill{At: q.Event, Reason: "strictly-subsequent-entry", Side: "ask", Price: q.Ask}
	if side == frozenlevels.Short {
		f.Side, f.Price = "bid", q.Bid
	}
	return f
}
func exitFill(q frozenlevels.Quote, side frozenlevels.Direction, reason string, target float64) (fill, error) {
	f := fill{At: q.Event, Reason: reason, Side: "bid", Price: q.Bid}
	if side == frozenlevels.Short {
		f.Side, f.Price = "ask", q.Ask
	}
	switch reason {
	case "stop":
	case "target":
		f.Price = target
	default:
		return fill{}, fmt.Errorf("unsupported-exit-reason")
	}
	return f, nil
}
func structuralStop(window frozenlevels.Window, side frozenlevels.Direction, grid float64) (float64, error) {
	x := window.Low
	if side == frozenlevels.Short {
		x = window.High
	}
	scaled := new(big.Rat).Quo(exactPrice(x), exactPrice(grid))
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(scaled.Num(), scaled.Denom(), r)
	if side == frozenlevels.Short {
		if r.Sign() > 0 {
			q.Add(q, big.NewInt(1))
		}
		q.Add(q, big.NewInt(1))
	} else {
		q.Sub(q, big.NewInt(1))
	}
	exactStop := new(big.Rat).Mul(new(big.Rat).SetInt(q), exactPrice(grid))
	stop, _ := exactStop.Float64()
	if stop <= 0 || math.IsInf(stop, 0) || math.IsNaN(stop) {
		return 0, fmt.Errorf("invalid-structural-stop")
	}
	if exactPrice(stop).Cmp(exactStop) != 0 {
		return 0, fmt.Errorf("unrepresentable-exact-structural-stop")
	}
	return stop, nil
}
func quantityFor(entry, stop float64, side frozenlevels.Direction, riskBudget *big.Rat) (*big.Rat, error) {
	risk := times(minus(exactPrice(entry), exactPrice(stop)), big.NewRat(int64(side), 1))
	if risk.Sign() <= 0 {
		return nil, fmt.Errorf("nonpositive-entry-risk")
	}
	q := new(big.Rat).Quo(riskBudget, risk)
	display, _ := q.Float64()
	if display <= 0 || math.IsInf(display, 0) || math.IsNaN(display) {
		return nil, fmt.Errorf("unrepresentable-quantity")
	}
	return q, nil
}
