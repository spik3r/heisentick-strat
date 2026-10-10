package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/marketdata"
)

const r11ReservationLegacySource = `dsl v7
strategy "Invented admission witness" { description "Synthetic bars only" }
market conditions {
 slices(XAUUSD 5m)
 sessions(asia, london, ny)
 day type in (trending, ranging, choppy)
}
setup {
 type: price momentum
 lookback 1 candles
 neutral zone 1 percent
}
risk {
 stop beyond last 1 candle extreme by 0 ATR
 stop size min 0 max 10
}
target { target 1R }
management { wait 1 candles after trade }
execution { risk: 101 USD }
`

// Entirely invented arithmetic-price bars; this is an admission witness only.
func r11ReservationFixture() RunFixture {
	start := time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC).UnixMilli()
	bars := make([]marketdata.Bar, 60)
	for i := range bars {
		c := 100 + float64(i)*2
		bars[i] = marketdata.Bar{T: float64(start + int64(i)*300000), O: c, H: c + .5, L: c - .5, C: c, V: 1}
	}
	return RunFixture{Case: "invented-R11-reservation-witness", Symbol: "XAUUSD", Timeframe: "5m", Bars: bars, Costs: Costs{FillOn: "close", StartEquity: 10000}}
}

func TestRangeReversionMalformedSelectorCannotExecuteLegacyTrades(t *testing.T) {
	fixture := r11ReservationFixture()
	positive, err := RunFixtureCase(fixture, r11ReservationLegacySource)
	if err != nil || len(positive.Trades) != 9 {
		t.Fatalf("invented positive control: trades=%d err=%v", len(positive.Trades), err)
	}
	for _, selector := range []string{`type "range reversion"`, `type range-reversion`, `type range_reversion`, "type range\u00a0reversion", `type range reversion`, `type=range-reversion`} {
		for _, sep := range []string{"\n", " "} {
			source := strings.Replace(r11ReservationLegacySource, "setup {", "setup {\n"+selector, 1)
			if sep == " " {
				source = strings.Replace(source, "setup {\n"+selector+"\n type: price momentum\n lookback 1 candles\n neutral zone 1 percent\n}", "setup { "+selector+" type: price momentum lookback 1 candles neutral zone 1 percent }", 1)
			}
			for _, input := range []RunFixture{fixture, {}} {
				got, err := RunFixtureCase(input, source)
				if err == nil || !strings.Contains(err.Error(), "range reversion") || len(got.Trades) != 0 {
					t.Errorf("selector %q escaped: trades=%d err=%v", selector, len(got.Trades), err)
				}
			}
		}
	}
}
