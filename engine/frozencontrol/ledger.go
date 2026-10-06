// Package frozencontrol contains an unregistered synthetic trace coordinator.
// It has no exported runner or production qualification constructor.
package frozencontrol

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/spik3r/heisentick-strat/engine/frozenlevels"
)

// All values entering the ledger have passed the strict finite input boundary.
// Prices use the primitive's shortest-decimal contract; fees retain raw tokens.
func exactPrice(v float64) *big.Rat {
	r, ok := new(big.Rat).SetString(strconv.FormatFloat(v, 'f', -1, 64))
	if !ok {
		panic("validated finite price")
	}
	return r
}
func rational(s string) (*big.Rat, error) {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, fmt.Errorf("invalid-exact-ledger-value")
	}
	return r, nil
}
func times(a, b *big.Rat) *big.Rat { return new(big.Rat).Mul(a, b) }
func minus(a, b *big.Rat) *big.Rat { return new(big.Rat).Sub(a, b) }
func plus(a, b *big.Rat) *big.Rat  { return new(big.Rat).Add(a, b) }
func amount(r *big.Rat) string {
	d := new(big.Int).Set(r.Denom())
	two, five := 0, 0
	for _, step := range []struct {
		factor int64
		count  *int
	}{{2, &two}, {5, &five}} {
		f := big.NewInt(step.factor)
		for new(big.Int).Mod(d, f).Sign() == 0 {
			d.Quo(d, f)
			*step.count++
			if *step.count > 1024 {
				return r.RatString()
			}
		}
	}
	if d.Cmp(big.NewInt(1)) != 0 {
		return r.RatString()
	}
	places := two
	if five > places {
		places = five
	}
	s := r.FloatString(places)
	if places > 0 {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

type ledgerEntry struct {
	At   frozenlevels.EventKey `json:"at"`
	Kind string                `json:"kind"`
	USD  string                `json:"usd"`
}
type trade struct {
	Status       string        `json:"status"`
	QuantityOz   string        `json:"quantityOz"`
	Entry        *fill         `json:"entry"`
	Exit         *fill         `json:"exit"`
	GrossUSD     *string       `json:"grossUSD"`
	EntryFeeUSD  string        `json:"entryFeeUSD"`
	ExitFeeUSD   string        `json:"exitFeeUSD"`
	NetUSD       *string       `json:"netUSD"`
	NetR         *string       `json:"netR"`
	CashDeltaUSD string        `json:"cashDeltaUSD"`
	Ledger       []ledgerEntry `json:"ledger"`
}
type positionLedger struct {
	quantity, riskBudget, fee, entryFee, cash *big.Rat
	entry                                     float64
	side                                      frozenlevels.Direction
	result                                    trade
}

func newLedger(entry fill, quantity, riskBudget, fee *big.Rat, side frozenlevels.Direction) *positionLedger {
	charged := times(quantity, fee)
	cash := new(big.Rat).Neg(charged)
	l := &positionLedger{quantity: new(big.Rat).Set(quantity), riskBudget: new(big.Rat).Set(riskBudget), fee: new(big.Rat).Set(fee), entryFee: charged, cash: cash, entry: entry.Price, side: side}
	l.result = trade{Status: "open", QuantityOz: amount(quantity), Entry: &entry, EntryFeeUSD: amount(charged), ExitFeeUSD: "0", CashDeltaUSD: amount(cash), Ledger: []ledgerEntry{{At: entry.At, Kind: "entry-fee", USD: amount(cash)}}}
	return l
}
func (l *positionLedger) close(exit fill) {
	gross := times(times(minus(exactPrice(exit.Price), exactPrice(l.entry)), big.NewRat(int64(l.side), 1)), l.quantity)
	exitFee := times(l.quantity, l.fee)
	net := minus(minus(gross, l.entryFee), exitFee)
	l.cash = minus(plus(l.cash, gross), exitFee)
	if l.cash.Cmp(net) != 0 {
		panic("fee ledger failed conservation")
	}
	g, n, r := amount(gross), amount(net), amount(new(big.Rat).Quo(net, l.riskBudget))
	l.result.Status = "closed"
	l.result.Exit = &exit
	l.result.GrossUSD = &g
	l.result.NetUSD = &n
	l.result.NetR = &r
	l.result.ExitFeeUSD = amount(exitFee)
	l.result.CashDeltaUSD = amount(l.cash)
	l.result.Ledger = append(l.result.Ledger, ledgerEntry{At: exit.At, Kind: "gross-exit", USD: g}, ledgerEntry{At: exit.At, Kind: "exit-fee", USD: amount(new(big.Rat).Neg(exitFee))})
}
func (l *positionLedger) unresolved() { l.result.Status = "unresolved" }
