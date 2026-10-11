package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/engine"
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
func r11ReservationFixture() engine.RunFixture {
	start := time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC).UnixMilli()
	bars := make([]marketdata.Bar, 60)
	for i := range bars {
		c := 100 + float64(i)*2
		bars[i] = marketdata.Bar{T: float64(start + int64(i)*300000), O: c, H: c + .5, L: c - .5, C: c, V: 1}
	}
	return engine.RunFixture{Schema: "dsl-conformance-run-fixture-v1", Case: "invented-R11-reservation-witness", Symbol: "XAUUSD", Timeframe: "5m", Bars: bars, Costs: engine.Costs{FillOn: "close", StartEquity: 10000}}
}

func TestR11ReservedSelectorFixtureAndColumnsFailClosed(t *testing.T) {
	fixture := r11ReservationFixture()
	for _, b := range fixture.Bars {
		fixture.RawBars = append(fixture.RawBars, []float64{b.T, b.O, b.H, b.L, b.C, b.V})
	}
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	var cols [6][]float64
	for _, b := range fixture.Bars {
		row := [6]float64{b.T, b.O, b.H, b.L, b.C, b.V}
		for i, v := range row {
			cols[i] = append(cols[i], v)
		}
	}
	meta := `{"schema":"enginewasm-columnar-v1","symbol":"XAUUSD","timeframe":"5m","costs":{"fillOn":"close","startEquity":10000}}`
	positive, err := runFixture(string(raw), r11ReservationLegacySource)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Trades []json.RawMessage `json:"trades"`
	}
	if err = json.Unmarshal(positive, &envelope); err != nil || len(envelope.Trades) != 9 {
		t.Fatalf("fixture positive: trades=%d err=%v", len(envelope.Trades), err)
	}
	columnPositive, err := runColumns(meta, r11ReservationLegacySource, cols)
	if err != nil || len(columnPositive.Values)/columnTradeWidth != 9 {
		t.Fatalf("columns positive: trades=%d err=%v", len(columnPositive.Values)/columnTradeWidth, err)
	}
	for _, selector := range []string{`type "range reversion"`, `type range-reversion`, `type range_reversion`, "type range\u00a0reversion", `type range reversion`, `type=range-reversion`} {
		for _, sep := range []string{"\n", " "} {
			source := strings.Replace(r11ReservationLegacySource, "setup {", "setup {\n"+selector, 1)
			if sep == " " {
				source = strings.Replace(source, "setup {\n"+selector+"\n type: price momentum\n lookback 1 candles\n neutral zone 1 percent\n}", "setup { "+selector+" type: price momentum lookback 1 candles neutral zone 1 percent }", 1)
			}
			if out, err := runFixture(string(raw), source); err == nil || !strings.Contains(err.Error(), "range reversion") || out != nil {
				t.Errorf("fixture selector %q escaped: %v", selector, err)
			}
			if _, err := runColumns(meta, source, cols); err == nil || !strings.Contains(err.Error(), "range reversion") {
				t.Errorf("column selector %q escaped: %v", selector, err)
			}
		}
	}
}
