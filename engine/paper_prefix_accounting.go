package engine

import "math"

type PaperAccount struct {
	StartEquity         float64 `json:"startEquity"`
	RealizedNet         float64 `json:"realizedNet"`
	Unrealized          float64 `json:"unrealized"`
	Cash                float64 `json:"cash"`
	Equity              float64 `json:"equity"`
	EntryFees           float64 `json:"entryFees"`
	ExitFees            float64 `json:"exitFees"`
	Fees                float64 `json:"fees"`
	PeakEquity          float64 `json:"peakEquity"`
	Drawdown            float64 `json:"drawdown"`
	DrawdownFraction    float64 `json:"drawdownFraction"`
	MaxDrawdown         float64 `json:"maxDrawdown"`
	MaxDrawdownFraction float64 `json:"maxDrawdownFraction"`
}

type PaperTradeStatistics struct {
	Count              int      `json:"count"`
	Wins               int      `json:"wins"`
	Losses             int      `json:"losses"`
	Breakeven          int      `json:"breakeven"`
	NetPnL             float64  `json:"netPnl"`
	GrossWins          float64  `json:"grossWins"`
	GrossLosses        float64  `json:"grossLosses"`
	WinRate            *float64 `json:"winRate"`
	ProfitFactor       *float64 `json:"profitFactor"`
	ProfitFactorStatus string   `json:"profitFactorStatus"`
}

// Effect prices and quantities are broker observations. ExitCredit is not net
// of the original entry fee; CompletedNetPnL exists only on final flatten.
type PaperExecutionEffect struct {
	Side            string   `json:"side"`
	Price           float64  `json:"price"`
	Size            float64  `json:"size"`
	Fee             float64  `json:"fee"`
	Stop            *float64 `json:"stop"`
	Target          *float64 `json:"target"`
	ExitCredit      *float64 `json:"exitCredit,omitempty"`
	CompletedNetPnL *float64 `json:"completedNetPnl,omitempty"`
	Reason          string   `json:"reason,omitempty"`
	Rule            string   `json:"rule,omitempty"`
}

type PaperEvent struct {
	EventID        string                `json:"eventId"`
	Kind           string                `json:"kind"`
	PositionID     string                `json:"positionId,omitempty"`
	Index          int                   `json:"index"`
	T              float64               `json:"t"`
	Ordinal        int                   `json:"ordinal"`
	RealizedBefore float64               `json:"realizedBefore"`
	RealizedAfter  float64               `json:"realizedAfter"`
	Effect         *PaperExecutionEffect `json:"effect,omitempty"`
	Account        *PaperAccount         `json:"account,omitempty"`
}

type paperOpenAccount struct {
	PositionID    string  `json:"positionId"`
	EntryIndex    int     `json:"entryIndex"`
	EntryT        float64 `json:"entryT"`
	Side          string  `json:"side"`
	Entry         float64 `json:"entry"`
	RemainingSize float64 `json:"remainingSize"`
	EntryFee      float64 `json:"entryFee"`
	ExitCredits   float64 `json:"exitCredits"`
}

type paperCollector struct {
	binding    string
	account    PaperAccount
	statistics PaperTradeStatistics
	open       *paperOpenAccount
	events     []PaperEvent
	lastIndex  int
	ordinal    int
	err        error
}

func newPaperCollector(binding string, start float64) *paperCollector {
	return &paperCollector{binding: binding, account: PaperAccount{StartEquity: start, Cash: start, Equity: start, PeakEquity: start}, statistics: PaperTradeStatistics{ProfitFactorStatus: "no-trades"}, events: []PaperEvent{}, lastIndex: -1}
}
func paperSame(a, b float64) bool { return math.Float64bits(a) == math.Float64bits(b) }
func (c *paperCollector) fail() {
	if c.err == nil {
		c.err = paperError("ACCOUNTING")
	}
}
func (c *paperCollector) finite(values ...float64) bool {
	for _, v := range values {
		if !isFinite(v) {
			c.err = paperError("NONFINITE")
			return false
		}
	}
	return true
}
func (c *paperCollector) observeBefore(before float64) bool {
	if c.err != nil {
		return false
	}
	if !paperSame(c.account.RealizedNet, before) {
		c.fail()
		return false
	}
	return true
}
func (c *paperCollector) emit(event PaperEvent) {
	if c.err != nil {
		return
	}
	if len(c.events) >= PaperPrefixMaxEvents {
		c.err = paperError("EVENT_LIMIT")
		return
	}
	if event.Index != c.lastIndex {
		if event.Index < c.lastIndex {
			c.fail()
			return
		}
		c.lastIndex = event.Index
		c.ordinal = 0
	}
	event.Ordinal = c.ordinal
	c.ordinal++
	id, err := paperDigest(struct {
		Domain, Binding, Kind, Position string
		Index                           int
		T                               float64
		Ordinal                         int
	}{"paper-effect-v1", c.binding, event.Kind, event.PositionID, event.Index, event.T, event.Ordinal})
	if err != nil {
		c.err = err
		return
	}
	event.EventID = id
	c.events = append(c.events, event)
}
func paperOptional(value float64, absent bool) *float64 {
	if absent {
		return nil
	}
	return &value
}

func (c *paperCollector) entry(b *broker, before float64) {
	if !c.observeBefore(before) {
		return
	}
	p := b.position
	fee := float64(b.costs.FeePerUnit * p.Size)
	if c.open != nil || !b.hasPosition || p.Size <= 0 || !c.finite(p.Entry, p.Size, fee, b.realized) {
		c.fail()
		return
	}
	ordinal := 0
	if c.lastIndex == p.EntryIndex {
		ordinal = c.ordinal
	}
	id, err := paperDigest(struct {
		Domain, Binding, Side string
		Index                 int
		T                     float64
		Ordinal               int
	}{"paper-position-v1", c.binding, p.Side.String(), p.EntryIndex, p.EntryT, ordinal})
	if err != nil {
		c.err = err
		return
	}
	c.open = &paperOpenAccount{PositionID: id, EntryIndex: p.EntryIndex, EntryT: p.EntryT, Side: p.Side.String(), Entry: p.Entry, RemainingSize: p.Size, EntryFee: fee}
	c.account.RealizedNet = b.realized
	c.account.EntryFees = float64(c.account.EntryFees + fee)
	c.account.Fees = float64(c.account.EntryFees + c.account.ExitFees)
	c.emit(PaperEvent{Kind: "entry", PositionID: id, Index: p.EntryIndex, T: p.EntryT, RealizedBefore: before, RealizedAfter: b.realized, Effect: &PaperExecutionEffect{Side: p.Side.String(), Price: p.Entry, Size: p.Size, Fee: fee, Stop: paperOptional(p.SL, p.NoStop), Target: paperOptional(p.TP, p.NoTarget)}})
}

func (c *paperCollector) exit(b *broker, before, credit float64, index int, partial bool) {
	if !c.observeBefore(before) {
		return
	}
	if c.open == nil || len(b.trades) == 0 {
		c.fail()
		return
	}
	trade := b.trades[len(b.trades)-1]
	open := c.open
	if trade.EntryIndex != open.EntryIndex || !paperSame(trade.EntryT, open.EntryT) || trade.Side != open.Side || !paperSame(trade.Entry, open.Entry) || trade.Size <= 0 || trade.Size > open.RemainingSize || trade.Partial != partial || !paperSame(trade.PnL, credit) {
		c.fail()
		return
	}
	if partial && (!b.hasPosition || !isFinite(b.position.Size) || b.position.Size < 0 || b.position.Size > open.RemainingSize) {
		c.fail()
		return
	}
	if !partial && (b.hasPosition || !paperSame(trade.Size, open.RemainingSize)) {
		c.fail()
		return
	}
	fee := float64(b.costs.FeePerUnit * trade.Size)
	if !c.finite(fee, credit, b.realized, trade.Exit) {
		return
	}
	open.ExitCredits = float64(open.ExitCredits + credit)
	// Observe the broker remainder. ARM64 may fuse oldSize - oldSize*fraction;
	// recomputing oldSize - already-rounded closedSize would change its bits.
	if partial {
		open.RemainingSize = b.position.Size
	} else {
		open.RemainingSize = 0
	}
	c.account.RealizedNet = b.realized
	c.account.ExitFees = float64(c.account.ExitFees + fee)
	c.account.Fees = float64(c.account.EntryFees + c.account.ExitFees)
	effect := &PaperExecutionEffect{Side: trade.Side, Price: trade.Exit, Size: trade.Size, Fee: fee, Stop: paperOptional(trade.SL, trade.NoStop), Target: paperOptional(trade.TP, trade.NoTarget), ExitCredit: &credit, Reason: trade.Reason, Rule: trade.Rule}
	kind := "partial"
	if !partial {
		kind = "exit"
		net := float64(open.ExitCredits - open.EntryFee)
		effect.CompletedNetPnL = &net
		s := &c.statistics
		s.Count++
		s.NetPnL = float64(s.NetPnL + net)
		switch {
		case net > 0:
			s.Wins++
			s.GrossWins = float64(s.GrossWins + net)
		case net < 0:
			s.Losses++
			s.GrossLosses = float64(s.GrossLosses - net)
		default:
			s.Breakeven++
		}
		rate := float64(s.Wins) / float64(s.Count)
		s.WinRate = &rate
		if s.GrossLosses > 0 {
			factor := float64(s.GrossWins / s.GrossLosses)
			s.ProfitFactor = &factor
			s.ProfitFactorStatus = "defined"
		} else {
			s.ProfitFactor = nil
			s.ProfitFactorStatus = "no-losses"
		}
		c.open = nil
	}
	c.emit(PaperEvent{Kind: kind, PositionID: open.PositionID, Index: index, T: b.series.T[index], RealizedBefore: before, RealizedAfter: b.realized, Effect: effect})
}

func (c *paperCollector) mark(b *broker, index int) {
	if c.err != nil {
		return
	}
	if !paperSame(b.realized, c.account.RealizedNet) || (c.open != nil) != b.hasPosition {
		c.fail()
		return
	}
	unrealized := float64(0)
	if b.hasPosition {
		if !paperSame(c.open.RemainingSize, b.position.Size) {
			c.fail()
			return
		}
		difference := float64(b.series.C[index] - b.position.Entry)
		points := float64(difference * float64(b.position.Side))
		unrealized = float64(points * b.position.Size)
	}
	a := &c.account
	a.Unrealized = unrealized
	a.Cash = float64(a.StartEquity + b.realized)
	a.Equity = float64(a.Cash + unrealized)
	if a.Equity > a.PeakEquity {
		a.PeakEquity = a.Equity
	}
	a.Drawdown = float64(a.PeakEquity - a.Equity)
	a.DrawdownFraction = float64(a.Drawdown / a.PeakEquity)
	if a.Drawdown > a.MaxDrawdown {
		a.MaxDrawdown = a.Drawdown
	}
	if a.DrawdownFraction > a.MaxDrawdownFraction {
		a.MaxDrawdownFraction = a.DrawdownFraction
	}
	if !c.finite(a.RealizedNet, a.Unrealized, a.Cash, a.Equity, a.EntryFees, a.ExitFees, a.Fees, a.PeakEquity, a.Drawdown, a.DrawdownFraction, a.MaxDrawdown, a.MaxDrawdownFraction, c.statistics.NetPnL, c.statistics.GrossWins, c.statistics.GrossLosses) {
		return
	}
	snapshot := *a
	c.emit(PaperEvent{Kind: "mark", Index: index, T: b.series.T[index], RealizedBefore: b.realized, RealizedAfter: b.realized, Account: &snapshot})
}
