package report

import (
	"strings"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestRouteWarningsFlagWeekendExtremeFadeWithoutWeekendBars(t *testing.T) {
	cfg, route, _ := loadConformanceCase(t, "deployed-dsl-weekend-extreme-fade-btcusdt-four-hour")
	if warnings := RouteWarningsForMode(cfg, route, "declared"); len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none for a series with weekend bars", warnings)
	}
	var weekdays []marketdata.Bar
	for _, bar := range route.Series.Bars() {
		day := time.UnixMilli(int64(bar.T)).UTC().Weekday()
		if day != time.Saturday && day != time.Sunday {
			weekdays = append(weekdays, bar)
		}
	}
	route.Series = marketdata.SeriesFromBars(weekdays)
	warnings := RouteWarningsForMode(cfg, route, "declared")
	if len(warnings) != 1 || !strings.Contains(warnings[0], "weekend extreme fade cannot enter") {
		t.Fatalf("warnings = %v, want the weekend extreme fade warning", warnings)
	}
}
