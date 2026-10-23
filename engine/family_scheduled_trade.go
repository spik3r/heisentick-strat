package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
)

type ScheduledTradeRow struct {
	Date       string  `json:"date"`
	Status     string  `json:"status"`
	EntryIndex *int    `json:"entryIndex,omitempty"`
	ExitIndex  *int    `json:"exitIndex,omitempty"`
	StopPrice  float64 `json:"stopPrice,omitempty"`
	GapStop    bool    `json:"gapStop,omitempty"`
	Detail     string  `json:"detail,omitempty"`
}
type ScheduledTradeAudit struct {
	Schema                  string                 `json:"schema"`
	CalendarSHA256          string                 `json:"calendarSha256"`
	CoverageFromDate        string                 `json:"coverageFromDate"`
	CoverageToDateExclusive string                 `json:"coverageToDateExclusive"`
	ExecutionModel          string                 `json:"executionModel"`
	FeeModel                string                 `json:"feeModel"`
	GapBars                 int                    `json:"gapBars"` // missing expected bars while a scheduled position is open
	GapObservations         int                    `json:"gapObservations"`
	Rows                    []ScheduledTradeRow    `json:"rows"`
	OpenPosition            *ScheduledOpenPosition `json:"openPosition"`
}
type ScheduledOpenPosition struct {
	Date       string  `json:"date"`
	Side       string  `json:"side"`
	Entry      float64 `json:"entry"`
	Stop       float64 `json:"stop"`
	EntryIndex int     `json:"entryIndex"`
	LastIndex  int     `json:"lastIndex"`
}

func isScheduledTrade(cfg dsl.Config) bool {
	return cfg != nil && cfg["setupType"] == string(dsl.FamilyScheduledTrade)
}
func scheduledPipKnown(symbol string) bool {
	return dsl.ScheduledPipKnownSymbol(symbol)
}
func scheduledPipSize(symbol string) float64 {
	if len(symbol) >= 3 && symbol[len(symbol)-3:] == "JPY" {
		return .01
	}
	return .0001
}
func runScheduledTrade(request RunRequest) (RunResult, error) {
	spec, err := dsl.DecodeScheduledTrade(request.Config)
	if err != nil {
		return RunResult{}, err
	}
	if request.TimedCalendar != nil || request.SourceSeries.Len() != 0 || request.HTFSeries.Len() != 0 || request.SourceHTFSeries.Len() != 0 || request.SourceTimeframe != "" || request.HigherTimeframe != "" || request.ExecutionWindow != nil || request.ForceRoute || request.ReportTradeContext {
		return RunResult{}, fmt.Errorf("scheduled trade rejects calendars, source/HTF, clipping, transfer and generic report context")
	}
	type route struct {
		Symbol string `json:"symbol"`
		TF     string `json:"tf"`
	}
	raw, _ := json.Marshal(request.Config["slices"])
	var routes []route
	_ = json.Unmarshal(raw, &routes)
	if len(routes) != 1 || routes[0].Symbol != request.Symbol || routes[0].TF != request.Timeframe {
		return RunResult{}, fmt.Errorf("scheduled request must match its one declared slice")
	}
	delta, err := timedDuration(request.Timeframe)
	if err != nil || (request.Timeframe != "1m" && request.Timeframe != "5m") {
		return RunResult{}, fmt.Errorf("scheduled trade supports 1m or 5m only")
	}
	if err = validateTimedSeries(request.Series, delta); err != nil {
		return RunResult{}, err
	}
	if err = validateScheduledCosts(request.Costs); err != nil {
		return RunResult{}, err
	}
	if !scheduledPipKnown(request.Symbol) {
		return RunResult{}, fmt.Errorf("unsupported scheduled pip map symbol %q", request.Symbol)
	}
	from, to, err := scheduledDateRange(request.Series.T[0], request.Series.T[request.Series.Len()-1])
	if err != nil {
		return RunResult{}, err
	}
	if from.Format("2006-01-02") < japanCalendarFrom || to.Format("2006-01-02") > japanCalendarThrough {
		return RunResult{}, fmt.Errorf("scheduled data outside Japan calendar coverage")
	}
	clockEntry, _ := dsl.TimedClockMinute(spec.Entry)
	clockExit, _ := dsl.TimedClockMinute(spec.Exit)
	byTime := make(map[int64]int, request.Series.Len())
	for i, t := range request.Series.T {
		byTime[int64(t)] = i
	}
	calendarHash := sha256.Sum256(japanHolidayJSON)
	fileHash := hex.EncodeToString(calendarHash[:])
	audit := &ScheduledTradeAudit{Schema: "strat-scheduled-trade-audit-v1", CalendarSHA256: fileHash, CoverageFromDate: japanCalendarFrom, CoverageToDateExclusive: "2027-01-01", ExecutionModel: "exact-scheduled-open-with-open-gap-stop-and-first-open-at-or-after-time-exit", FeeModel: "two-sided fixed fee per unit; broker slippage applied at each fill", Rows: []ScheduledTradeRow{}}
	var b broker
	b.series = request.Series
	b.costs = request.Costs.normalized()
	pip := scheduledPipSize(request.Symbol)
	entryMinute := int64(clockEntry) * 60000
	exitMinute := int64(clockExit) * 60000
	lastExitIndex := -1
	firstDay := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	lastDay := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	for day := firstDay; !day.After(lastDay); day = day.AddDate(0, 0, 1) {
		if !isJapanSettlementDate(day) {
			continue
		}
		date := day.Format("2006-01-02")
		entryT := day.UnixMilli() + entryMinute
		exitT := day.UnixMilli() + exitMinute
		row := ScheduledTradeRow{Date: date, Status: "missing-entry-bar"}
		ei, hasEntry := byTime[entryT]
		if !hasEntry {
			audit.Rows = append(audit.Rows, row)
			continue
		}
		row.EntryIndex = &ei
		if lastExitIndex >= ei {
			row.Status = "busy-at-entry"
			row.Detail = "prior scheduled position exits at or after this entry observation"
			audit.Rows = append(audit.Rows, row)
			continue
		}
		if ei == 0 || int64(request.Series.T[ei-1]) != entryT-delta {
			row.Status = "missing-contiguous-prior"
			audit.Rows = append(audit.Rows, row)
			continue
		}
		s := sideShort
		if spec.Side == "long" {
			s = sideLong
		}
		stopDist := spec.StopPips * pip
		meta := TradeMeta{"setup": "scheduledTrade", "date": date, "scheduledEntryT": float64(entryT), "scheduledExitT": float64(exitT), "calendarSha256": fileHash, "pipSize": pip, "stopPips": spec.StopPips}
		b.openPosition(s, request.Series.O[ei], order{Side: s, Size: 1, HasSize: true, StopDistance: stopDist, HasStopDistance: true, NoTarget: true, GapAwareStop: true, Tag: "DSL-SCHEDULED-TRADE", Meta: meta}, ei)
		stop := b.position.SL
		row.StopPrice = stop
		closed := false
		previousT := int64(request.Series.T[ei])
		for i := ei; i < request.Series.Len(); i++ {
			t := int64(request.Series.T[i])
			o, h, l := request.Series.O[i], request.Series.H[i], request.Series.L[i]
			if i > ei && t-previousT > delta {
				missing := int((t-previousT)/delta) - 1
				if missing > 0 {
					audit.GapBars += missing
					audit.GapObservations++
				}
			}
			previousT = t
			gap := (s == sideLong && o <= stop) || (s == sideShort && o >= stop)
			if gap {
				b.closePosition(o, i, ReasonRule, "scheduled-gap-stop")
				b.trades[len(b.trades)-1].PnL -= request.Costs.FeePerUnit
				row.Status = "stopped"
				row.ExitIndex = &i
				row.GapStop = true
				closed = true
				break
			}
			if t >= exitT {
				b.closePosition(o, i, ReasonRule, "scheduled-time-exit")
				b.trades[len(b.trades)-1].PnL -= request.Costs.FeePerUnit
				row.Status = "time-exit"
				row.ExitIndex = &i
				closed = true
				break
			}
			stopTouched := (s == sideLong && l <= stop) || (s == sideShort && h >= stop)
			if stopTouched {
				b.closePosition(stop, i, ReasonRule, "scheduled-stop")
				b.trades[len(b.trades)-1].PnL -= request.Costs.FeePerUnit
				row.Status = "stopped"
				row.ExitIndex = &i
				closed = true
				break
			}
		}
		if !closed {
			row.Status = "open-terminal"
			row.Detail = "no observed stop or time-exit open; position intentionally left unliquidated"
			audit.OpenPosition = &ScheduledOpenPosition{Date: date, Side: s.String(), Entry: b.position.Entry, Stop: stop, EntryIndex: ei, LastIndex: request.Series.Len() - 1}
			b.hasPosition = false
		}
		if closed {
			lastExitIndex = *row.ExitIndex
		}
		audit.Rows = append(audit.Rows, row)
	}
	fixture := fixtureFromRequest(request)
	result, err := checkedResultEnvelope(fixture, b.trades)
	if err != nil {
		return RunResult{}, err
	}
	result.ScheduledAudit = audit
	return result, nil
}

func validateScheduledCosts(c Costs) error {
	if c.FillOn != "nextOpen" {
		return fmt.Errorf("scheduled trade requires explicit costs.fillOn=nextOpen")
	}
	if !isFinite(c.FeePerUnit) || !isFinite(c.Slippage) || !isFinite(c.SlippageBps) || c.FeePerUnit < 0 || c.Slippage < 0 || c.SlippageBps < 0 || !isFinite(c.StartEquity) || c.StartEquity < 0 {
		return fmt.Errorf("invalid scheduled execution costs")
	}
	return nil
}
