package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	native "github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/report"
)

// Sequential market companion v3: v2 plus an optional hashed closure calendar
// and open-terminal reporting. v1 and v2 are untouched. See
// docs/sequential-market-contract.md, "Version 3".
const (
	sequentialMarketV3ResultSchema    = "strat-sequential-market-result-v3"
	sequentialMarketV3AdmissionSchema = "strat-sequential-market-admission-v3"
	sequentialMarketV3Version         = 3
)

type sequentialMarketV3Identity struct {
	sequentialMarketIdentity
	Calendar *native.SequentialMarketCalendarUse `json:"calendar,omitempty"`
}

type sequentialMarketV3Terminal struct {
	Position             string                            `json:"position"`
	PendingOrderRejected bool                              `json:"pendingOrderRejected"`
	OpenTrade            *native.SequentialMarketOpenTrade `json:"openTrade,omitempty"`
}

type sequentialMarketV3Limits struct {
	ClosureCalendar      bool   `json:"closureCalendar"`
	UndeclaredGaps       string `json:"undeclaredGaps"`
	TerminalOpenPosition bool   `json:"terminalOpenPosition"`
	MaxBars              int    `json:"maxBars"`
}

func marketV3Limits() *sequentialMarketV3Limits {
	return &sequentialMarketV3Limits{ClosureCalendar: true, UndeclaredGaps: "refused", TerminalOpenPosition: true, MaxBars: native.SequentialMarketMaxBars}
}

type sequentialMarketV3Envelope struct {
	Schema          string                          `json:"schema"`
	ContractVersion int                             `json:"contractVersion"`
	Identity        *sequentialMarketV3Identity     `json:"identity,omitempty"`
	Run             *native.RunResult               `json:"run,omitempty"`
	Metrics         *report.SequentialMarketMetrics `json:"metrics,omitempty"`
	Terminal        *sequentialMarketV3Terminal     `json:"terminal,omitempty"`
	Capabilities    *sequentialCapabilities         `json:"capabilities,omitempty"`
	Limits          *sequentialMarketV3Limits       `json:"limits,omitempty"`
	Error           *sequentialError                `json:"error,omitempty"`
}

type sequentialMarketV3Calendar struct {
	CalendarID       string `json:"calendarId"`
	Provider         string `json:"provider"`
	Symbol           string `json:"symbol"`
	Timeframe        string `json:"timeframe"`
	Version          string `json:"version"`
	SHA256           string `json:"sha256"`
	DeclaredClosures int    `json:"declaredClosures"`
}

type sequentialMarketV3Admission struct {
	Schema          string                             `json:"schema"`
	ContractVersion int                                `json:"contractVersion"`
	Runnable        bool                               `json:"runnable"`
	Disposition     string                             `json:"disposition"`
	Route           *native.SequentialMarketRoute      `json:"route,omitempty"`
	Calendar        *sequentialMarketV3Calendar        `json:"calendar,omitempty"`
	Timeline        *native.SequentialMarketTimelineV3 `json:"timeline,omitempty"`
	Limits          *sequentialMarketV3Limits          `json:"limits"`
	Error           *sequentialError                   `json:"error,omitempty"`
}

// sequentialMarketV3Error extends the shared mapping with the calendar error.
func sequentialMarketV3Error(err error) *sequentialError {
	var calendar *native.SequentialCalendarError
	if errors.As(err, &calendar) {
		out := &sequentialError{Code: calendar.Kind, Message: err.Error(), Field: calendar.Field}
		if calendar.Index >= 0 {
			index := calendar.Index
			out.BarIndex = &index
		}
		return out
	}
	return sequentialMarketError(err)
}

func sequentialMarketV3Failure(err error) []byte {
	raw, _ := json.Marshal(sequentialMarketV3Envelope{Schema: sequentialMarketV3ResultSchema, ContractVersion: sequentialMarketV3Version, Error: sequentialMarketV3Error(err)})
	return raw
}

func sequentialMarketV3AdmissionFailure(err error) []byte {
	e := sequentialMarketV3Error(err)
	raw, _ := json.Marshal(sequentialMarketV3Admission{Schema: sequentialMarketV3AdmissionSchema, ContractVersion: sequentialMarketV3Version, Disposition: sequentialMarketDisposition(e), Limits: marketV3Limits(), Error: e})
	return raw
}

func decodeSequentialMarketV3Calendar(raw string) (*native.SequentialClosureCalendar, error) {
	if raw == "" {
		return nil, nil
	}
	calendar, err := native.DecodeSequentialClosureCalendar(raw)
	if err != nil {
		return nil, err
	}
	return &calendar, nil
}

func runSequentialMarketV3(raw, source, calendarRaw string) ([]byte, bool) {
	fixture, err := sequentialMarketDecode(raw, source)
	if err != nil {
		return sequentialMarketV3Failure(err), false
	}
	calendar, err := decodeSequentialMarketV3Calendar(calendarRaw)
	if err != nil {
		return sequentialMarketV3Failure(err), false
	}
	executed, err := native.RunSequentialMarketV3(fixture, source, calendar)
	if err != nil {
		return sequentialMarketV3Failure(err), false
	}
	metrics, err := report.ProjectSequentialMarketMetrics(executed.Run, executed.Accounting)
	if err != nil {
		return sequentialMarketV3Failure(err), false
	}
	fixtureHash, sourceHash := sha256.Sum256([]byte(raw)), sha256.Sum256([]byte(source))
	terminal := &sequentialMarketV3Terminal{Position: "flat", PendingOrderRejected: executed.PendingOrderRejected}
	if executed.Open != nil {
		terminal.Position, terminal.OpenTrade = "open", executed.Open
	}
	out, err := json.Marshal(sequentialMarketV3Envelope{
		Schema: sequentialMarketV3ResultSchema, ContractVersion: sequentialMarketV3Version,
		Identity: &sequentialMarketV3Identity{
			sequentialMarketIdentity{
				FixtureSHA256: hex.EncodeToString(fixtureHash[:]), DSLSHA256: hex.EncodeToString(sourceHash[:]),
				StrategyID: executed.Run.StrategyID, Route: executed.Route, BarCount: len(fixture.Bars),
				FirstBarMS: fixture.Bars[0].T, LastBarMS: fixture.Bars[len(fixture.Bars)-1].T, Costs: executed.Run.Costs,
			},
			executed.Calendar,
		},
		Run: &executed.Run, Metrics: &metrics, Terminal: terminal,
		Capabilities: &sequentialCapabilities{Headline: true, Trades: true, Equity: true},
		Limits:       marketV3Limits(),
	})
	if err != nil {
		return sequentialMarketV3Failure(err), false
	}
	return out, true
}

// admitSequentialMarketV3 executes the request through the same stages as run
// and discards the result, so runnable cannot disagree with run.
func admitSequentialMarketV3(raw, source, calendarRaw string) ([]byte, bool) {
	out := sequentialMarketV3Admission{Schema: sequentialMarketV3AdmissionSchema, ContractVersion: sequentialMarketV3Version, Limits: marketV3Limits()}
	fixture, err := sequentialMarketDecode(raw, source)
	var calendar *native.SequentialClosureCalendar
	if err == nil {
		calendar, err = decodeSequentialMarketV3Calendar(calendarRaw)
	}
	if err == nil {
		var route native.SequentialMarketRoute
		route, err = native.ValidateSequentialMarketFixture(fixture, source)
		if route.Symbol != "" {
			out.Route = &route
			shown := calendar
			if calendar != nil {
				if calendar.BindRoute(route) == nil {
					out.Calendar = &sequentialMarketV3Calendar{calendar.CalendarID, calendar.Provider, calendar.Symbol, calendar.Timeframe, calendar.Version, calendar.SHA256, len(calendar.Closures)}
				} else {
					shown = nil
				}
			}
			timeline := native.ScanSequentialMarketTimelineV3(fixture, route.TimeframeMS, shown)
			out.Timeline = &timeline
		}
		if err == nil {
			var executed native.SequentialMarketV3Run
			if executed, err = native.RunSequentialMarketV3(fixture, source, calendar); err == nil {
				_, err = report.ProjectSequentialMarketMetrics(executed.Run, executed.Accounting)
			}
		}
	}
	if err == nil {
		out.Runnable, out.Disposition = true, "runnable"
	} else {
		out.Error = sequentialMarketV3Error(err)
		out.Disposition = sequentialMarketDisposition(out.Error)
	}
	encoded, _ := json.Marshal(out)
	return encoded, out.Runnable
}
