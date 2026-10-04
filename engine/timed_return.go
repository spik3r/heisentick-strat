package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
)

func timedAnchorTime(a dsl.TimedAnchor, current, previous TimedReferenceSession, loc *time.Location) (int64, error) {
	row := current
	if a.Session == "previous" {
		row = previous
	}
	clock := a.Clock
	if a.Boundary == "open" {
		clock = row.Open
	}
	if a.Boundary == "close" {
		clock = row.Close
	}
	return timedResolve(row.Date, clock, loc)
}

func compileTimedPlan(request RunRequest, spec dsl.TimedReturnSpec, delta int64) ([]timedPlanRow, string, error) {
	c := request.TimedCalendar
	loc, indices, err := validateTimedCalendar(c, spec.Timezone)
	if err != nil {
		return nil, "", err
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, "", err
	}
	h := sha256.Sum256(raw)
	digest := hex.EncodeToString(h[:])
	rows := []timedPlanRow{}
	needPrevious := spec.Start.Session == "previous" || spec.End.Session == "previous"
	for _, current := range c.Sessions {
		if current.Date < c.TradeFromDate || current.Date >= c.TradeToDateExclusive {
			continue
		}
		row := timedPlanRow{audit: TimedAuditRow{Date: current.Date}, startIndex: -1, endIndex: -1, entryIndex: -1, exitIndex: -1}
		if current.Kind != "full" {
			row.audit.Status = "excluded_reference_session"
			rows = append(rows, row)
			continue
		}
		var previous TimedReferenceSession
		if needPrevious {
			pi, ok := indices[current.PreviousSessionDate]
			if !ok || current.PreviousSessionDate == "" {
				return nil, "", fmt.Errorf("timed calendar %s lacks predecessor context", current.Date)
			}
			previous = c.Sessions[pi]
			if previous.Kind != "full" {
				row.audit.Status = "excluded_predecessor_session"
				rows = append(rows, row)
				continue
			}
		}
		start, err := timedAnchorTime(spec.Start, current, previous, loc)
		if err != nil {
			return nil, "", err
		}
		end, err := timedAnchorTime(spec.End, current, previous, loc)
		if err != nil {
			return nil, "", err
		}
		decision, err := timedResolve(current.Date, spec.Entry, loc)
		if err != nil {
			return nil, "", err
		}
		exitBoundary, err := timedResolve(current.Date, spec.Exit, loc)
		if err != nil {
			return nil, "", err
		}
		if !(start < end && end <= decision && decision < exitBoundary) {
			return nil, "", fmt.Errorf("timed anchor ordering invalid on %s", current.Date)
		}
		entry, exit := decision+delta, exitBoundary+delta
		if time.UnixMilli(entry).In(loc).Format("2006-01-02") != current.Date || time.UnixMilli(exit).In(loc).Format("2006-01-02") != current.Date {
			return nil, "", fmt.Errorf("timed delayed fill crosses local date on %s", current.Date)
		}
		row.audit = TimedAuditRow{Date: current.Date, Status: "scheduled", StartKind: spec.Start.Session + "." + spec.Start.Boundary + ":" + spec.Start.Price,
			EndKind: spec.End.Session + "." + spec.End.Boundary + ":" + spec.End.Price, StartT: start, EndT: end, DecisionT: decision, EntryT: entry, ExitT: exit}
		for _, fill := range []struct {
			role  string
			t     int64
			index *int
		}{{"entry", entry, &row.entryIndex}, {"exit", exit, &row.exitIndex}} {
			if !timedQuoted(c, fill.t) {
				return nil, "", fmt.Errorf("timed dataset inadmissible: %s %s %d outside quoted execution intervals", current.Date, fill.role, fill.t)
			}
			*fill.index = timedIndex(request.Series, fill.t)
			if *fill.index < 0 {
				return nil, "", fmt.Errorf("timed dataset inadmissible: %s missing %s bar open at %d", current.Date, fill.role, fill.t)
			}
		}
		row.startOpen, row.endOpen = spec.Start.Price == "open", spec.End.Price == "open"
		startBar, endBar := start, end
		if !row.startOpen {
			startBar -= delta
		}
		if !row.endOpen {
			endBar -= delta
		}
		row.startIndex, row.endIndex = timedIndex(request.Series, startBar), timedIndex(request.Series, endBar)
		rows = append(rows, row)
	}
	return rows, digest, nil
}

// runTimedReturn is reachable only through checked native APIs. Preparation is
// private so legacy unchecked PreparedRun.Run cannot silently swallow an error.
func runTimedReturn(request RunRequest) (RunResult, error) {
	spec, err := dsl.DecodeTimedReturn(request.Config)
	if err != nil {
		return RunResult{}, err
	}
	if request.SourceSeries.Len() != 0 || request.HTFSeries.Len() != 0 || request.SourceHTFSeries.Len() != 0 || request.SourceTimeframe != "" || request.HigherTimeframe != "" || request.ExecutionWindow != nil || request.ForceRoute || request.ReportTradeContext {
		return RunResult{}, fmt.Errorf("timed return rejects source/HTF, clipping, transfer and generic report context")
	}
	var routes []struct {
		Symbol string `json:"symbol"`
		TF     string `json:"tf"`
	}
	raw, _ := json.Marshal(request.Config["slices"])
	_ = json.Unmarshal(raw, &routes)
	if len(routes) != 1 || routes[0].Symbol != request.Symbol || routes[0].TF != request.Timeframe {
		return RunResult{}, fmt.Errorf("timed request must match its one declared slice")
	}
	delta, err := timedDuration(request.Timeframe)
	if err != nil {
		return RunResult{}, err
	}
	if err := validateTimedSeries(request.Series, delta); err != nil {
		return RunResult{}, err
	}
	if err := validateTimedCosts(request.Costs); err != nil {
		return RunResult{}, err
	}
	plan, digest, err := compileTimedPlan(request, spec, delta)
	if err != nil {
		return RunResult{}, err
	}
	// Cost-domain validation applies to every scheduled row, including future
	// unavailable/zero/side-blocked signals, before any endpoint comparison.
	for _, row := range plan {
		if row.audit.Status == "scheduled" {
			for _, i := range []int{row.entryIndex, row.exitIndex} {
				price := request.Series.O[i]
				slip := request.Costs.Slippage + price*request.Costs.SlippageBps/10000
				if !isFinite(slip) || !isFinite(price+slip) || price-slip <= 0 {
					return RunResult{}, fmt.Errorf("timed execution price/cost domain invalid on %s", row.audit.Date)
				}
			}
		}
	}
	allowLong := numberFromAny(request.Config["allowLong"], 0) == 1
	allowShort := numberFromAny(request.Config["allowShort"], 0) == 1
	b := broker{series: request.Series, costs: request.Costs.normalized(), trades: []Trade{}}
	audit := &TimedReturnAudit{Schema: "strat-timed-return-audit-v1", CalendarSHA256: digest, TimezoneDataSHA256: request.TimedCalendar.TimezoneDataSHA256,
		ExecutionModel: "one-timeframe-delayed-open-ohlc-proxy", QuantityModel: "one-fixed-unit-price-pnl", AdmissionModel: "retrospective-whole-batch-coverage", Rows: []TimedAuditRow{}}
	for _, row := range plan {
		a := row.audit
		if a.Status != "scheduled" {
			audit.Rows = append(audit.Rows, a)
			continue
		}
		if row.startIndex < 0 || row.endIndex < 0 {
			a.Status = "unavailable_endpoint"
			audit.Rows = append(audit.Rows, a)
			continue
		}
		start, end := request.Series.C[row.startIndex], request.Series.C[row.endIndex]
		if row.startOpen {
			start = request.Series.O[row.startIndex]
		}
		if row.endOpen {
			end = request.Series.O[row.endIndex]
		}
		r := math.Log(end) - math.Log(start)
		if !isFinite(r) {
			return RunResult{}, fmt.Errorf("timed return metadata nonfinite on %s", a.Date)
		}
		a.LogReturn = &r
		if start == end {
			a.Status = "zero_return"
			audit.Rows = append(audit.Rows, a)
			continue
		}
		s := sideLong
		if end < start {
			s = sideShort
		}
		if spec.Direction == "against" {
			s = -s
		}
		if s == sideLong && !allowLong || s == sideShort && !allowShort {
			a.Status = "side_blocked"
			audit.Rows = append(audit.Rows, a)
			continue
		}
		if b.hasPosition {
			return RunResult{}, fmt.Errorf("timed position overlap invariant")
		}
		meta := TradeMeta{"setup": "timedReturn", "date": a.Date, "signalStartT": float64(a.StartT), "signalEndT": float64(a.EndT),
			"startKind": a.StartKind, "endKind": a.EndKind, "decisionT": float64(a.DecisionT), "scheduledEntryT": float64(a.EntryT),
			"scheduledExitT": float64(a.ExitT), "endpointLogReturn": r, "calendarSha256": digest, "executionModel": audit.ExecutionModel}
		b.openPosition(s, request.Series.O[row.entryIndex], order{Side: s, Size: 1, HasSize: true, NoStop: true, NoTarget: true, Tag: "DSL-TIMED-RETURN", Meta: meta}, row.entryIndex)
		b.closePosition(request.Series.O[row.exitIndex], row.exitIndex, ReasonRule, "timed-scheduled-exit")
		a.Status = "executed"
		audit.Rows = append(audit.Rows, a)
	}
	if b.hasPosition || len(b.pendingOrders) != 0 || len(b.pendingExits) != 0 {
		return RunResult{}, fmt.Errorf("timed terminal exposure invariant")
	}
	fixture := fixtureFromRequest(request)
	result, err := checkedResultEnvelope(fixture, b.trades)
	if err != nil {
		return RunResult{}, err
	}
	result.TimedAudit = audit
	return result, nil
}
