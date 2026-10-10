package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/dsl"
	native "github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/report"
)

const (
	sequentialResultSchema       = "strat-sequential-backtest-result-v1"
	sequentialMaxFixture         = 64 << 20
	sequentialMaxSource          = 8 << 20
	sequentialMaxBars            = 500_000
	sequentialSourceLimitMessage = "DSL must be nonempty, valid UTF-8 and at most 8 MiB"
)

type sequentialIdentity struct {
	FixtureSHA256 string       `json:"fixtureSha256"`
	DSLSHA256     string       `json:"dslSha256"`
	StrategyID    string       `json:"strategyId"`
	Symbol        string       `json:"symbol"`
	Timeframe     string       `json:"timeframe"`
	BarCount      int          `json:"barCount"`
	FirstBarMS    float64      `json:"firstBarMs"`
	LastBarMS     float64      `json:"lastBarMs"`
	Costs         native.Costs `json:"costs"`
	Profile       string       `json:"profile"`
	Policy        string       `json:"policy,omitempty"`
}

type sequentialCapabilities struct {
	Headline      bool `json:"headline"`
	Trades        bool `json:"trades"`
	Equity        bool `json:"equity"`
	Groupings     bool `json:"groupings"`
	Monthly       bool `json:"monthly"`
	RDistribution bool `json:"rDistribution"`
	Portfolio     bool `json:"portfolio"`
}

type sequentialEnvelope struct {
	Schema          string                    `json:"schema"`
	ContractVersion int                       `json:"contractVersion"`
	Identity        *sequentialIdentity       `json:"identity,omitempty"`
	Run             *native.RunResult         `json:"run,omitempty"`
	Metrics         *report.SequentialMetrics `json:"metrics,omitempty"`
	Capabilities    *sequentialCapabilities   `json:"capabilities,omitempty"`
	Error           *sequentialError          `json:"error,omitempty"`
}

type sequentialError struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	Field         string `json:"field,omitempty"`
	BarIndex      *int   `json:"barIndex,omitempty"`
	OpportunityID string `json:"opportunityId,omitempty"`
}

type sequentialInputError struct {
	code, field, message string
}

func (e *sequentialInputError) Error() string { return e.message }

func sequentialInvalid(field, message string) error {
	return &sequentialInputError{"SEQUENTIAL_FIXTURE_INVALID", field, message}
}

// sequentialFailure deliberately emits no identity, run, metrics or capabilities.
// Native test dispatch and the actual WASM export share exactly this envelope.
func sequentialFailure(err error) []byte {
	failure := &sequentialError{Code: "SEQUENTIAL_ENGINE_REJECTED", Message: err.Error()}
	var input *sequentialInputError
	var config *dsl.SequentialFullConfigError
	var execution *native.SequentialFullExecutionError
	var accounting *native.SequentialAccountingError
	var metrics *report.SequentialMetricsError
	switch {
	case errors.As(err, &input):
		failure.Code, failure.Field = input.code, input.field
	case errors.As(err, &config):
		failure.Code, failure.Field = config.Code, config.Field
	case errors.As(err, &execution):
		failure.Code, failure.Field = execution.Kind, execution.Field
		if execution.BarIndex >= 0 {
			index := execution.BarIndex
			failure.BarIndex = &index
		}
		failure.OpportunityID = execution.OpportunityID
	case errors.As(err, &accounting):
		failure.Code, failure.Field = accounting.Kind, accounting.Field
		if accounting.BarIndex >= 0 {
			index := accounting.BarIndex
			failure.BarIndex = &index
		}
	case errors.As(err, &metrics):
		failure.Code, failure.Field = metrics.Kind, metrics.Field
	}
	raw, _ := json.Marshal(sequentialEnvelope{Schema: sequentialResultSchema, ContractVersion: 1, Error: failure})
	return raw
}

// runSequentialFixture is an additive, source-frozen companion export. It does
// one engine execution and projects only the captured native accounting. The
// retained RunResult keeps its existing serialization and rounding contract.
func runSequentialFixture(raw, source string) ([]byte, error) {
	if len(raw) > sequentialMaxFixture {
		return nil, sequentialInvalid("fixture", "fixture JSON exceeds 64 MiB")
	}
	if len(source) == 0 || len(source) > sequentialMaxSource || !utf8.ValidString(source) {
		return nil, &sequentialInputError{"SEQUENTIAL_DSL_INVALID", "source", sequentialSourceLimitMessage}
	}
	fixture, err := decodeSequentialFixture(raw)
	if err != nil {
		return nil, err
	}
	run, accounting, err := native.RunSequentialFixtureWithAccounting(fixture, source)
	if err != nil {
		return nil, err
	}
	metrics, err := report.ProjectSequentialMetrics(run, accounting)
	if err != nil {
		return nil, err
	}
	fixtureHash, sourceHash := sha256.Sum256([]byte(raw)), sha256.Sum256([]byte(source))
	return json.Marshal(sequentialEnvelope{
		Schema: sequentialResultSchema, ContractVersion: 1,
		Identity: &sequentialIdentity{
			FixtureSHA256: hex.EncodeToString(fixtureHash[:]), DSLSHA256: hex.EncodeToString(sourceHash[:]),
			StrategyID: run.StrategyID, Symbol: run.Symbol, Timeframe: run.Timeframe,
			BarCount: len(fixture.Bars), FirstBarMS: fixture.Bars[0].T, LastBarMS: fixture.Bars[len(fixture.Bars)-1].T,
			Costs: run.Costs, Profile: accounting.Profile, Policy: accounting.Policy,
		},
		Run: &run, Metrics: &metrics,
		Capabilities: &sequentialCapabilities{Headline: true, Trades: true, Equity: true},
	})
}

func decodeSequentialFixture(raw string) (native.RunFixture, error) {
	var fixture native.RunFixture
	if !utf8.ValidString(raw) {
		return fixture, sequentialInvalid("fixture", "fixture must be valid UTF-8")
	}
	// Reuse the family's existing bounded-depth JSON token admission, including
	// exact decimal timestamp validation before binary64 decoding, duplicate
	// keys, unknown fields, null numeric cells and trailing-value rejection.
	// Its provenance/context rules also prevent silent auxiliary inputs.
	if err := native.ValidateSequentialFullJSONEnvelope([]byte(raw), false); err != nil {
		return fixture, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return fixture, sequentialInvalid("fixture", err.Error())
	}
	for _, key := range []string{"schema", "case", "strategyId", "symbol", "timeframe", "rangeMethod"} {
		var value string
		if err := json.Unmarshal(fields[key], &value); err != nil || strings.TrimSpace(value) == "" {
			return fixture, sequentialInvalid(key, "required nonempty string: "+key)
		}
	}
	var costs map[string]json.RawMessage
	if err := json.Unmarshal(fields["costs"], &costs); err != nil || costs == nil {
		return fixture, sequentialInvalid("costs", "required costs object")
	}
	for _, key := range []string{"feePerUnit", "fillOn", "slippage"} {
		if _, ok := costs[key]; !ok {
			return fixture, sequentialInvalid("costs."+key, "required cost: "+key)
		}
	}
	// Count original rows before allocating engine slices. The byte cap bounds
	// this raw-message pass; no row is repaired, removed or downsampled.
	var rows []json.RawMessage
	if err := json.Unmarshal(fields["bars"], &rows); err != nil || len(rows) == 0 || len(rows) > sequentialMaxBars {
		return fixture, sequentialInvalid("bars", "bars must contain 1..500000 original six-number rows")
	}
	if err := json.Unmarshal([]byte(raw), &fixture); err != nil {
		return fixture, sequentialInvalid("fixture", fmt.Sprintf("decode fixture: %v", err))
	}
	if fixture.Schema != "dsl-conformance-run-fixture-v1" {
		return fixture, sequentialInvalid("schema", "unsupported fixture schema")
	}
	for i, row := range fixture.RawBars {
		if len(row) != 6 {
			return fixture, &native.SequentialFullExecutionError{Kind: "invalid-sequential-input", Field: "bars.shape", BarIndex: i}
		}
		for _, value := range row {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return fixture, &native.SequentialFullExecutionError{Kind: "invalid-sequential-input", Field: "bars", BarIndex: i}
			}
		}
	}
	fixture.ScanRawBarRows([]byte(raw))
	fixture.Bars = rowsToBars(fixture.RawBars)
	fixture.SourceBars = rowsToBars(fixture.RawSourceBars)
	fixture.HTFBars = rowsToBars(fixture.RawHTFBars)
	fixture.SourceHTFBars = rowsToBars(fixture.RawSourceHTFBars)
	return fixture, nil
}
