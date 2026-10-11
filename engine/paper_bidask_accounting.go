package engine

import (
	"math/big"
	"strconv"
)

func newRatAdd(a, b *big.Rat) *big.Rat   { return new(big.Rat).Add(a, b) }
func newRatSub(a, b *big.Rat) *big.Rat   { return new(big.Rat).Sub(a, b) }
func newRatMul(a, b *big.Rat) *big.Rat   { return new(big.Rat).Mul(a, b) }
func newRatQuo(a, b *big.Rat) *big.Rat   { return new(big.Rat).Quo(a, b) }
func bidAskRat(v BidAskDecimal) *big.Rat { r, _ := bidAskNumber(v); return r }
func bidAskFamily(v float64) (*big.Rat, error) {
	if !isFinite(v) {
		return nil, bidAskError("FAMILY_NUMERIC")
	}
	s := strconv.FormatFloat(v, 'g', -1, 64)
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Num().BitLen() > 256 || r.Denom().BitLen() > 256 {
		return nil, bidAskError("FAMILY_NUMERIC")
	}
	return r, nil
}
func bidAskCash(r *big.Rat) string {
	// All cash operands are terminating base-ten rationals. Division is used only
	// for downward lot sizing; ratios are separately serialized as fractions.
	d := new(big.Int).Set(r.Denom())
	two, five := big.NewInt(2), big.NewInt(5)
	n2, n5 := 0, 0
	for new(big.Int).Mod(d, two).Sign() == 0 {
		d.Quo(d, two)
		n2++
	}
	for new(big.Int).Mod(d, five).Sign() == 0 {
		d.Quo(d, five)
		n5++
	}
	if d.Cmp(big.NewInt(1)) != 0 {
		return r.RatString()
	}
	if n5 > n2 {
		n2 = n5
	}
	return bidAskTrimDecimal(r.FloatString(n2))
}
func bidAskSigned(v *big.Rat, s side) *big.Rat {
	if s == sideShort {
		return new(big.Rat).Neg(v)
	}
	return new(big.Rat).Set(v)
}
func bidAskAdverse(p *big.Rat, s side, entry bool, e BidAskEconomics) *big.Rat {
	slip := newRatAdd(bidAskRat(e.SlippageFixed), newRatQuo(newRatMul(new(big.Rat).Abs(p), bidAskRat(e.SlippageBPS)), big.NewRat(10000, 1)))
	if entry {
		return newRatAdd(p, bidAskSigned(slip, s))
	}
	return newRatSub(p, bidAskSigned(slip, s))
}
func bidAskFloorQuantity(budget, unitLoss, quantum *big.Rat) *big.Rat {
	units := newRatQuo(newRatQuo(budget, unitLoss), quantum)
	whole := new(big.Int).Quo(units.Num(), units.Denom())
	return newRatMul(new(big.Rat).SetInt(whole), quantum)
}

type BidAskAccount struct {
	StartEquity        string      `json:"startEquity"`
	Balance            string      `json:"balance"`
	RealizedNet        string      `json:"realizedNet"`
	PaidFees           string      `json:"paidFees"`
	UnrealizedPnL      string      `json:"unrealizedPnl"`
	Equity             string      `json:"equity"`
	PeakEquity         string      `json:"peakEquity"`
	MaximumDrawdownUSD string      `json:"maximumDrawdownUsd"`
	MarkCount          int         `json:"markCount"`
	MarkBasis          string      `json:"markBasis"`
	MarkClock          BidAskClock `json:"markClock"`
	MarkStatus         string      `json:"markStatus"`
	Sampling           string      `json:"sampling"`
}
type BidAskStatistics struct {
	Completed            int    `json:"completed"`
	Winners              int    `json:"winners"`
	Losers               int    `json:"losers"`
	Flat                 int    `json:"flat"`
	NetPnL               string `json:"netPnl"`
	GrossProfit          string `json:"grossProfit"`
	GrossLoss            string `json:"grossLoss"`
	WinRateFraction      string `json:"winRateFraction"`
	ProfitFactorFraction string `json:"profitFactorFraction"`
	ProfitFactorStatus   string `json:"profitFactorStatus"`
}
type bidAskCollector struct {
	start, balance, fees, unrealized, equity, peak, drawdown, tradeNet, grossProfit, grossLoss *big.Rat
	account                                                                                    BidAskAccount
	stats                                                                                      BidAskStatistics
}

func newBidAskCollector(start BidAskDecimal) *bidAskCollector {
	s := bidAskRat(start)
	return &bidAskCollector{start: s, balance: new(big.Rat).Set(s), fees: new(big.Rat), unrealized: new(big.Rat), equity: new(big.Rat).Set(s), peak: new(big.Rat).Set(s), drawdown: new(big.Rat), tradeNet: new(big.Rat), grossProfit: new(big.Rat), grossLoss: new(big.Rat), account: BidAskAccount{MarkBasis: "initial-account", MarkStatus: "unobserved", Sampling: "entry-and-straddle-quotes;completed-full-side-bars"}}
}
func (c *bidAskCollector) entry(fee *big.Rat) { c.balance.Sub(c.balance, fee); c.fees.Add(c.fees, fee) }
func (c *bidAskCollector) exit(entry, fill, qty, entryFee, exitFee *big.Rat, s side) *big.Rat {
	gross := newRatMul(bidAskSigned(newRatSub(fill, entry), s), qty)
	credit := newRatSub(gross, exitFee)
	net := newRatSub(credit, entryFee)
	c.balance.Add(c.balance, credit)
	c.fees.Add(c.fees, exitFee)
	c.tradeNet.Add(c.tradeNet, net)
	c.stats.Completed++
	if net.Sign() > 0 {
		c.stats.Winners++
		c.grossProfit.Add(c.grossProfit, net)
	} else if net.Sign() < 0 {
		c.stats.Losers++
		c.grossLoss.Sub(c.grossLoss, net)
	} else {
		c.stats.Flat++
	}
	return net
}
func (c *bidAskCollector) mark(entry, price, qty *big.Rat, s side, basis string, clock BidAskClock) {
	c.unrealized.SetInt64(0)
	if entry != nil {
		c.unrealized.Mul(bidAskSigned(newRatSub(price, entry), s), qty)
	}
	c.equity.Add(c.balance, c.unrealized)
	if c.equity.Cmp(c.peak) > 0 {
		c.peak.Set(c.equity)
	}
	dd := newRatSub(c.peak, c.equity)
	if dd.Cmp(c.drawdown) > 0 {
		c.drawdown.Set(dd)
	}
	c.account.MarkCount++
	c.account.MarkBasis = basis
	c.account.MarkClock = clock
	c.account.MarkStatus = "as-of-recorded-observation"
}
func (c *bidAskCollector) snapshot() (BidAskAccount, BidAskStatistics) {
	a := c.account
	a.StartEquity = bidAskCash(c.start)
	a.Balance = bidAskCash(c.balance)
	a.RealizedNet = bidAskCash(newRatSub(c.balance, c.start))
	a.PaidFees = bidAskCash(c.fees)
	a.UnrealizedPnL = bidAskCash(c.unrealized)
	a.Equity = bidAskCash(c.equity)
	a.PeakEquity = bidAskCash(c.peak)
	a.MaximumDrawdownUSD = bidAskCash(c.drawdown)
	s := c.stats
	s.NetPnL = bidAskCash(c.tradeNet)
	s.GrossProfit = bidAskCash(c.grossProfit)
	s.GrossLoss = bidAskCash(c.grossLoss)
	s.WinRateFraction = "0"
	if s.Completed > 0 {
		s.WinRateFraction = big.NewRat(int64(s.Winners), int64(s.Completed)).RatString()
	}
	s.ProfitFactorFraction = ""
	s.ProfitFactorStatus = "no-losses"
	if c.grossLoss.Sign() > 0 {
		s.ProfitFactorFraction = newRatQuo(c.grossProfit, c.grossLoss).RatString()
		s.ProfitFactorStatus = "finite"
	}
	if s.Completed == 0 {
		s.ProfitFactorStatus = "no-trades"
	}
	return a, s
}
