package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/spik3r/heisentick-strat/dsl"
)

func validatePrefixOutput(costs Costs, trades []Trade, positions []OpenPositionSnapshot) error {
	if err := validateDerivedOutput(costs, trades); err != nil {
		return err
	}
	for i, position := range positions {
		for _, field := range []struct {
			name  string
			value float64
		}{
			{name: "entry", value: position.Entry},
			{name: "entryT", value: position.EntryT},
			{name: "initialSl", value: position.InitialSL},
			{name: "initialTp", value: position.InitialTP},
			{name: "sl", value: position.SL},
			{name: "tp", value: position.TP},
			{name: "size", value: position.Size},
		} {
			if !isFiniteDerivedOutput(field.value) {
				return fmt.Errorf("prefix result open position %d %s contains non-finite value", i, field.name)
			}
		}
		if err := validateDerivedOutput(Costs{}, []Trade{{Meta: position.Meta}}); err != nil {
			return fmt.Errorf("prefix result open position %d metadata is invalid: %w", i, err)
		}
	}
	return nil
}

// PrefixUnsupportedError reports an engine path that does not yet implement
// preserved-open prefix execution. Callers must not fall back to report
// liquidation when this error is returned.
type PrefixUnsupportedError struct {
	Path string
}

func (e *PrefixUnsupportedError) Error() string {
	return fmt.Sprintf("preserved-open prefix execution is unsupported for %s", e.Path)
}

// OpenPositionSnapshot is a read-only copy of the position left open at the
// end of an explicitly preserved prefix, with an identity stable when that
// prefix is extended.
type OpenPositionSnapshot struct {
	PositionID   string    `json:"positionId"`
	Side         string    `json:"side"`
	Entry        float64   `json:"entry"`
	EntryIndex   int       `json:"entryIndex"`
	EntryT       float64   `json:"entryT"`
	InitialSL    float64   `json:"initialSl"`
	InitialTP    float64   `json:"initialTp"`
	SL           float64   `json:"sl"`
	TP           float64   `json:"tp"`
	Size         float64   `json:"size"`
	Tag          string    `json:"tag"`
	Meta         TradeMeta `json:"meta"`
	PartialTaken bool      `json:"partialTaken,omitempty"`
	NoStop       bool      `json:"noStop,omitempty"`
	NoTarget     bool      `json:"noTarget,omitempty"`
}

// PrefixClosedTrade binds a closed trade to the same deterministic identity
// used while the position was open. Ordinary report Trade JSON is unchanged.
type PrefixClosedTrade struct {
	PositionID string `json:"positionId"`
	Trade      Trade  `json:"trade"`
}

// PrefixResult reports closed trades plus the position preserved after the
// last supplied bar. Existing Run and report results keep liquidating at the
// final close and are unchanged.
type PrefixResult struct {
	Trades        []PrefixClosedTrade    `json:"trades"`
	OpenPositions []OpenPositionSnapshot `json:"openPositions"`
	// CheckpointDigest binds the complete replay input. It is not a resumable
	// engine-state checkpoint; that is a later HT-037 slice.
	CheckpointDigest string `json:"checkpointDigest"`
}

const prefixIdentityDomain = "heisentick-forward-position"
const prefixInputDomain = "heisentick-forward-prefix-input"
const prefixContractVersion = 1

func canonicalDigest(domain string, value any) (string, error) {
	payload, err := json.Marshal(struct {
		Domain  string `json:"domain"`
		Version int    `json:"version"`
		Value   any    `json:"value"`
	}{Domain: domain, Version: prefixContractVersion, Value: value})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func prefixPositionID(strategyID, symbol, timeframe string, entryT float64, entryIndex int, side string) (string, error) {
	// The broker admits at most one active or pending entry, so two positions
	// cannot open on the same route/bar. No synthetic ordinal is encoded.
	digest, err := canonicalDigest(prefixIdentityDomain, struct {
		StrategyID string  `json:"strategyId"`
		Symbol     string  `json:"symbol"`
		Timeframe  string  `json:"timeframe"`
		EntryT     float64 `json:"entryT"`
		EntryIndex int     `json:"entryIndex"`
		Side       string  `json:"side"`
	}{strategyID, symbol, timeframe, entryT, entryIndex, side})
	if err != nil {
		return "", err
	}
	return "fp_" + digest, nil
}

func prefixCheckpointDigest(request RunRequest) (string, error) {
	rangeMethod := request.RangeMethod
	if rangeMethod == "" {
		rangeMethod = "zone"
	}
	digest, err := canonicalDigest(prefixInputDomain, struct {
		StrategyID      string           `json:"strategyId"`
		Symbol          string           `json:"symbol"`
		Timeframe       string           `json:"timeframe"`
		SourceTimeframe string           `json:"sourceTimeframe"`
		HigherTimeframe string           `json:"higherTimeframe"`
		RangeMethod     string           `json:"rangeMethod"`
		Costs           Costs            `json:"costs"`
		Config          dsl.Config       `json:"config"`
		Series          any              `json:"series"`
		SourceSeries    any              `json:"sourceSeries"`
		HTFSeries       any              `json:"htfSeries"`
		SourceHTFSeries any              `json:"sourceHtfSeries"`
		ExecutionWindow *ExecutionWindow `json:"executionWindow"`
	}{
		request.StrategyID, request.Symbol, request.Timeframe, request.SourceTimeframe, request.HigherTimeframe, rangeMethod,
		request.Costs.normalized(), request.Config, request.Series, request.SourceSeries, request.HTFSeries, request.SourceHTFSeries,
		request.ExecutionWindow,
	})
	if err != nil {
		return "", fmt.Errorf("prefix input binding: %w", err)
	}
	return "sha256:" + digest, nil
}

func copyTradeMeta(source TradeMeta) TradeMeta {
	if source == nil {
		return nil
	}
	result := make(TradeMeta, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func (b *broker) openPositionSnapshot() []OpenPositionSnapshot {
	if !b.hasPosition {
		return []OpenPositionSnapshot{}
	}
	position := b.position
	return []OpenPositionSnapshot{{
		Side: position.Side.String(), Entry: position.Entry, EntryIndex: position.EntryIndex, EntryT: position.EntryT,
		InitialSL: position.InitialSL, InitialTP: position.InitialTP, SL: position.SL, TP: position.TP,
		Size: position.Size, Tag: position.Tag, Meta: copyTradeMeta(position.Meta), PartialTaken: position.PartialTaken,
		NoStop: position.NoStop, NoTarget: position.NoTarget,
	}}
}

func prefixUnsupportedPath(prepared *PreparedRun) string {
	if prepared.c5 {
		return "source-entry C5"
	}
	switch prepared.params.SetupType {
	case string(dsl.FamilyDailyFlushFailure), string(dsl.FamilyWeekendExtremeFade), string(dsl.FamilyIntraHourRunExhaustion):
		return "special family " + prepared.params.SetupType
	default:
		return ""
	}
}

// RunPrefix replays the complete supplied prefix without synthesizing an
// end-of-test close. It is replay-only in this slice: it does not accept or
// emit a checkpoint and it cannot place an order outside the engine.
func RunPrefix(request RunRequest) (PrefixResult, error) {
	prepared, err := PrepareRun(request)
	if err != nil {
		return PrefixResult{}, err
	}
	if path := prefixUnsupportedPath(prepared); path != "" {
		return PrefixResult{}, &PrefixUnsupportedError{Path: path}
	}
	prepared.fixture.Costs = request.Costs.normalized()
	if err := validateDerivedOutput(prepared.fixture.Costs, nil); err != nil {
		return PrefixResult{}, err
	}
	checkpointDigest, err := prefixCheckpointDigest(request)
	if err != nil {
		return PrefixResult{}, err
	}
	if prepared.offRoute {
		return PrefixResult{Trades: []PrefixClosedTrade{}, OpenPositions: []OpenPositionSnapshot{}, CheckpointDigest: checkpointDigest}, nil
	}
	prepared.broker.reset(prepared.series, prepared.cols, prepared.htfTrend, prepared.ema, prepared.emaSlope, prepared.params, prepared.fixture, prepared.trades)
	if prepared.windowed {
		prepared.broker.setExecutionWindow(prepared.execution)
	}
	trades := prepared.broker.runWithFinalization(false)
	positions := prepared.broker.openPositionSnapshot()
	if err := validatePrefixOutput(prepared.fixture.Costs, trades, positions); err != nil {
		return PrefixResult{}, err
	}
	closed := make([]PrefixClosedTrade, len(trades))
	for i, trade := range trades {
		positionID, err := prefixPositionID(request.StrategyID, request.Symbol, request.Timeframe, trade.EntryT, trade.EntryIndex, trade.Side)
		if err != nil {
			return PrefixResult{}, err
		}
		closed[i] = PrefixClosedTrade{PositionID: positionID, Trade: trade}
	}
	for i := range positions {
		positionID, err := prefixPositionID(request.StrategyID, request.Symbol, request.Timeframe, positions[i].EntryT, positions[i].EntryIndex, positions[i].Side)
		if err != nil {
			return PrefixResult{}, err
		}
		positions[i].PositionID = positionID
	}
	return PrefixResult{Trades: closed, OpenPositions: positions, CheckpointDigest: checkpointDigest}, nil
}
