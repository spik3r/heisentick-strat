package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode/utf8"

	native "github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/report"
)

// Sequential market companion v2. It shares the v1 transport decoder, error
// classification, scheduler, broker, collector and metrics projection. The v1
// export and its schema are unchanged. See docs/sequential-market-contract.md.
const (
	sequentialMarketResultSchema    = "strat-sequential-market-result-v2"
	sequentialMarketAdmissionSchema = "strat-sequential-market-admission-v2"
	sequentialMarketVersion         = 2
)

type sequentialMarketIdentity struct {
	FixtureSHA256 string                       `json:"fixtureSha256"`
	DSLSHA256     string                       `json:"dslSha256"`
	StrategyID    string                       `json:"strategyId"`
	Route         native.SequentialMarketRoute `json:"route"`
	BarCount      int                          `json:"barCount"`
	FirstBarMS    float64                      `json:"firstBarMs"`
	LastBarMS     float64                      `json:"lastBarMs"`
	Costs         native.Costs                 `json:"costs"`
}

// sequentialMarketTerminal states how the end of the window was treated. A
// success always has a flat book; v2 refuses a held terminal position instead
// of closing or marking it. A decision on the last bar is a pending order that
// has no next bar to fill on, reported here and in the audit as no_next_bar.
type sequentialMarketTerminal struct {
	Position             string `json:"position"`
	PendingOrderRejected bool   `json:"pendingOrderRejected"`
}

type sequentialMarketLimits struct {
	ContiguousBarsOnly bool `json:"contiguousBarsOnly"`
	ClosureCalendar    bool `json:"closureCalendar"`
	TerminalOpen       bool `json:"terminalOpenPosition"`
	MaxBars            int  `json:"maxBars"`
}

type sequentialMarketEnvelope struct {
	Schema          string                    `json:"schema"`
	ContractVersion int                       `json:"contractVersion"`
	Identity        *sequentialMarketIdentity `json:"identity,omitempty"`
	Run             *native.RunResult         `json:"run,omitempty"`
	Metrics         *report.SequentialMetrics `json:"metrics,omitempty"`
	Terminal        *sequentialMarketTerminal `json:"terminal,omitempty"`
	Capabilities    *sequentialCapabilities   `json:"capabilities,omitempty"`
	Limits          *sequentialMarketLimits   `json:"limits,omitempty"`
	Error           *sequentialError          `json:"error,omitempty"`
}

type sequentialMarketAdmission struct {
	Schema          string                           `json:"schema"`
	ContractVersion int                              `json:"contractVersion"`
	Runnable        bool                             `json:"runnable"`
	Disposition     string                           `json:"disposition"`
	Route           *native.SequentialMarketRoute    `json:"route,omitempty"`
	Timeline        *native.SequentialMarketTimeline `json:"timeline,omitempty"`
	Limits          *sequentialMarketLimits          `json:"limits"`
	Error           *sequentialError                 `json:"error,omitempty"`
}

func marketLimits() *sequentialMarketLimits {
	return &sequentialMarketLimits{ContiguousBarsOnly: true, MaxBars: native.SequentialMarketMaxBars}
}

// sequentialMarketError reuses the v1 typed-error mapping so a code means the
// same thing on both surfaces.
func sequentialMarketError(err error) *sequentialError {
	var v1 sequentialEnvelope
	if json.Unmarshal(sequentialFailure(err), &v1) != nil || v1.Error == nil {
		return &sequentialError{Code: "SEQUENTIAL_ENGINE_REJECTED", Message: err.Error()}
	}
	return v1.Error
}

func sequentialMarketFailure(err error) []byte {
	raw, _ := json.Marshal(sequentialMarketEnvelope{Schema: sequentialMarketResultSchema, ContractVersion: sequentialMarketVersion, Error: sequentialMarketError(err)})
	return raw
}

// sequentialMarketDisposition separates an input the engine deliberately does
// not support from one that is malformed. Both are refusals; neither returns
// trades.
func sequentialMarketDisposition(e *sequentialError) string {
	if strings.HasPrefix(e.Code, "unsupported") {
		return "unsupported"
	}
	if e.Code == "invalid-sequential-accounting" {
		switch e.Field {
		case "source.family", "additional source or calendar":
			return "unsupported"
		}
	}
	return "invalid"
}

func sequentialMarketDecode(raw, source string) (native.RunFixture, error) {
	if len(raw) > sequentialMaxFixture {
		return native.RunFixture{}, sequentialInvalid("fixture", "fixture JSON exceeds 64 MiB")
	}
	if len(source) == 0 || len(source) > sequentialMaxSource || !utf8.ValidString(source) {
		return native.RunFixture{}, &sequentialInputError{"SEQUENTIAL_DSL_INVALID", "source", sequentialSourceLimitMessage}
	}
	return decodeSequentialFixture(raw)
}

// runSequentialMarket always returns one envelope: either a success with
// trades, equity and statistics or a typed error with none of them.
func runSequentialMarket(raw, source string) ([]byte, bool) {
	fixture, err := sequentialMarketDecode(raw, source)
	if err != nil {
		return sequentialMarketFailure(err), false
	}
	executed, err := native.RunSequentialMarketFixture(fixture, source)
	if err != nil {
		return sequentialMarketFailure(err), false
	}
	metrics, err := report.ProjectSequentialMetrics(executed.Run, executed.Accounting)
	if err != nil {
		return sequentialMarketFailure(err), false
	}
	fixtureHash, sourceHash := sha256.Sum256([]byte(raw)), sha256.Sum256([]byte(source))
	terminal := &sequentialMarketTerminal{Position: "flat"}
	if audit := executed.Run.SequentialFull; audit != nil {
		for _, o := range audit.Opportunities {
			terminal.PendingOrderRejected = terminal.PendingOrderRejected || o.Reason == native.SequentialFullNoNextBar
		}
	}
	out, err := json.Marshal(sequentialMarketEnvelope{
		Schema: sequentialMarketResultSchema, ContractVersion: sequentialMarketVersion,
		Identity: &sequentialMarketIdentity{
			FixtureSHA256: hex.EncodeToString(fixtureHash[:]), DSLSHA256: hex.EncodeToString(sourceHash[:]),
			StrategyID: executed.Run.StrategyID, Route: executed.Route, BarCount: len(fixture.Bars),
			FirstBarMS: fixture.Bars[0].T, LastBarMS: fixture.Bars[len(fixture.Bars)-1].T, Costs: executed.Run.Costs,
		},
		Run: &executed.Run, Metrics: &metrics, Terminal: terminal,
		Capabilities: &sequentialCapabilities{Headline: true, Trades: true, Equity: true},
		Limits:       marketLimits(),
	})
	if err != nil {
		return sequentialMarketFailure(err), false
	}
	return out, true
}

// admitSequentialMarket answers "would this exact request run?" by performing
// the same validation and execution as runSequentialMarket and discarding the
// result, so the answer cannot drift from the run. A terminal-position refusal
// depends on execution and is therefore only known this way.
func admitSequentialMarket(raw, source string) ([]byte, bool) {
	out := sequentialMarketAdmission{Schema: sequentialMarketAdmissionSchema, ContractVersion: sequentialMarketVersion, Limits: marketLimits()}
	fixture, err := sequentialMarketDecode(raw, source)
	if err == nil {
		var route native.SequentialMarketRoute
		route, err = native.ValidateSequentialMarketFixture(fixture, source)
		if route.Symbol != "" {
			out.Route = &route
			timeline := native.ScanSequentialMarketTimeline(fixture, route.TimeframeMS)
			out.Timeline = &timeline
		}
		if err == nil {
			_, err = native.RunSequentialMarketFixture(fixture, source)
		}
	}
	if err == nil {
		out.Runnable, out.Disposition = true, "runnable"
	} else {
		out.Error = sequentialMarketError(err)
		out.Disposition = sequentialMarketDisposition(out.Error)
	}
	encoded, _ := json.Marshal(out)
	return encoded, out.Runnable
}
