package contextcols

// Options controls the causal core context columns implemented in this slice.
type Options struct {
	ATRLen               int
	ERLen                int
	EMAFastLen           int
	EMAFastSlopeLen      int
	PivotK               int
	TrendER              float64
	TrendPersistenceBars int
	TrendEnterER         float64
	TrendExitER          float64
	TrendERMode          string
	Selective            bool
	NeedSessionPhase     bool
	NeedPriorDay         bool
	NeedRegimeTrend      bool
	NeedRangeActive      bool
	NeedOpenLocation     bool
	// NeedReportTradeContext retains causal entry-bar labels for rich reports.
	NeedReportTradeContext bool
	Range                  RangeOptions
	Channel                ChannelOptions
	RangeStatsLookback     int
}

func (o Options) normalized() Options {
	if o.ATRLen <= 0 {
		o.ATRLen = 14
	}
	if o.ERLen <= 0 {
		o.ERLen = 10
	}
	if o.PivotK <= 0 {
		o.PivotK = 3
	}
	if o.TrendER <= 0 {
		o.TrendER = 0.4
	}
	if o.TrendPersistenceBars <= 0 {
		o.TrendPersistenceBars = 1
	}
	if o.TrendEnterER <= 0 {
		o.TrendEnterER = o.TrendER
	}
	if o.TrendExitER <= 0 || o.TrendExitER > o.TrendEnterER {
		o.TrendExitER = o.TrendEnterER
	}
	o.Range = o.Range.normalized()
	o.Channel = o.Channel.normalized()
	if o.RangeStatsLookback <= 0 {
		o.RangeStatsLookback = 10
	}
	return o
}
