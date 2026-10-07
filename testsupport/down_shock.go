package testsupport

import (
	"time"

	"github.com/spik3r/heisentick-strat/marketdata"
)

// DownShockSource and DownShockBars are invented source-plumbing regression
// inputs, not historical market data or a strategy-performance fixture.
const DownShockSource = `dsl v7
strategy "Invented down-shock source plumbing"
market conditions {
  slices(XAUUSD 1m)
}
setup {
  type: down shock rebound
  source timeframe 15m
  entryTf 1m
  shock entry immediate
  shock target 13 bp
}
execution {
  risk 200 USD
}
`

// DownShockBars supplies 460 flat weekday source bars in one time-of-day slot,
// then a down shock, with chart opens immediately before and at its known close.
func DownShockBars() (chart, source []marketdata.Bar) {
	for day := time.Date(2020, 1, 6, 9, 45, 0, 0, time.UTC); len(source) < 461; day = day.AddDate(0, 0, 1) {
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		source = append(source, marketdata.Bar{T: float64(day.UnixMilli()), O: 100, H: 100.1, L: 99.9, C: 100, V: 1})
	}
	source[460].H, source[460].L, source[460].C = 100.2, 98, 98.2
	knownAt := source[460].T + 15*60_000
	chart = []marketdata.Bar{
		{T: knownAt - 60_000, O: 98.3, H: 98.4, L: 98.1, C: 98.2, V: 1},
		{T: knownAt, O: 98.2, H: 98.8, L: 98.1, C: 98.6, V: 1},
	}
	return chart, source
}
