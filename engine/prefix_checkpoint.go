package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const prefixCheckpointSchema = "heisentick-prefix-checkpoint-v1"
const prefixCheckpointBindingDomain = "heisentick-forward-checkpoint-binding"
const prefixCheckpointBarsDomain = "heisentick-forward-checkpoint-bars"
const prefixCheckpointStateDomain = "heisentick-forward-checkpoint-state"

// PrefixCheckpointUnsupportedError reports a path whose mutable state is not
// yet represented by the resumable checkpoint schema.
type PrefixCheckpointUnsupportedError struct {
	Path string
}

func (e *PrefixCheckpointUnsupportedError) Error() string {
	return fmt.Sprintf("resumable prefix checkpoint is unsupported for %s", e.Path)
}

// PrefixCheckpointError reports a malformed checkpoint or one that does not
// bind to the replay request supplied by the caller.
type PrefixCheckpointError struct {
	Reason string
}

func (e *PrefixCheckpointError) Error() string {
	return "invalid prefix checkpoint: " + e.Reason
}

type prefixCheckpointState struct {
	Position      position      `json:"position"`
	HasPosition   bool          `json:"hasPosition"`
	PendingOrders []order       `json:"pendingOrders"`
	PendingExits  []pendingExit `json:"pendingExits"`
	LimitOrders   []order       `json:"limitOrders"`
	Realized      float64       `json:"realized"`
	ORBLastEntry  int           `json:"orbLastEntry"`
	HasORBEntry   bool          `json:"hasOrbEntry"`
	ORBSeenDay    int64         `json:"orbSeenDay"`
	ORBSeenKeys   []string      `json:"orbSeenKeys"`
	LastExitIndex int           `json:"lastExitIndex"`
}

type prefixCheckpointEnvelope struct {
	Schema               string                `json:"schema"`
	BindingDigest        string                `json:"bindingDigest"`
	AdmittedPrefixDigest string                `json:"admittedPrefixDigest"`
	StateDigest          string                `json:"stateDigest"`
	ProcessedBars        int                   `json:"processedBars"`
	LastBarT             float64               `json:"lastBarT"`
	State                prefixCheckpointState `json:"state"`
}

func checkpointUnsupportedPath(prepared *PreparedRun, request RunRequest) string {
	if path := prefixUnsupportedPath(prepared); path != "" {
		return path
	}
	if prepared.offRoute {
		return "off-route request"
	}
	if prepared.params.SetupType != string(dsl.FamilyOpeningRangeBreakout) {
		return "ordinary family " + prepared.params.SetupType
	}
	if request.SourceSeries.Len() != 0 || request.SourceHTFSeries.Len() != 0 {
		return "source-series replay"
	}
	if request.ExecutionWindow != nil {
		return "bounded execution window"
	}
	return ""
}

func prefixCheckpointBindingDigest(request RunRequest) (string, error) {
	rangeMethod := request.RangeMethod
	if rangeMethod == "" {
		rangeMethod = "zone"
	}
	digest, err := canonicalDigest(prefixCheckpointBindingDomain, struct {
		StrategyID      string     `json:"strategyId"`
		Symbol          string     `json:"symbol"`
		Timeframe       string     `json:"timeframe"`
		SourceTimeframe string     `json:"sourceTimeframe"`
		HigherTimeframe string     `json:"higherTimeframe"`
		RangeMethod     string     `json:"rangeMethod"`
		Costs           Costs      `json:"costs"`
		Config          dsl.Config `json:"config"`
	}{request.StrategyID, request.Symbol, request.Timeframe, request.SourceTimeframe, request.HigherTimeframe, rangeMethod, request.Costs.normalized(), request.Config})
	if err != nil {
		return "", fmt.Errorf("checkpoint binding: %w", err)
	}
	return "sha256:" + digest, nil
}

func seriesPrefix(series marketdata.Series, count int) marketdata.Series {
	return marketdata.Series{
		T: series.T[:count], O: series.O[:count], H: series.H[:count],
		L: series.L[:count], C: series.C[:count], V: series.V[:count],
	}
}

func seriesThrough(series marketdata.Series, lastT float64) marketdata.Series {
	count := 0
	for count < series.Len() && series.T[count] <= lastT {
		count++
	}
	return seriesPrefix(series, count)
}

func prefixCheckpointBarsDigest(request RunRequest, count int) (string, error) {
	if count < 0 || count > request.Series.Len() {
		return "", &PrefixCheckpointError{Reason: "processed bar count is outside the supplied prefix"}
	}
	lastT := float64(0)
	if count > 0 {
		lastT = request.Series.T[count-1]
	}
	digest, err := canonicalDigest(prefixCheckpointBarsDomain, struct {
		Series    marketdata.Series `json:"series"`
		HTFSeries marketdata.Series `json:"htfSeries"`
	}{seriesPrefix(request.Series, count), seriesThrough(request.HTFSeries, lastT)})
	if err != nil {
		return "", fmt.Errorf("checkpoint prefix binding: %w", err)
	}
	return "sha256:" + digest, nil
}

func checkpointStateFromBroker(b *broker) prefixCheckpointState {
	return prefixCheckpointState{
		Position: b.position, HasPosition: b.hasPosition,
		PendingOrders: append([]order(nil), b.pendingOrders...), PendingExits: append([]pendingExit(nil), b.pendingExits...),
		LimitOrders: append([]order(nil), b.limitOrders...), Realized: b.realized,
		ORBLastEntry: b.orbLastEntry, HasORBEntry: b.hasORBEntry,
		ORBSeenDay: b.seen.orb.day, ORBSeenKeys: append([]string(nil), b.seen.orb.keys...), LastExitIndex: b.lastExitIndex,
	}
}

func restoreCheckpointState(b *broker, state prefixCheckpointState) {
	b.position, b.hasPosition = state.Position, state.HasPosition
	b.pendingOrders = append(b.pendingOrders[:0], state.PendingOrders...)
	b.pendingExits = append(b.pendingExits[:0], state.PendingExits...)
	b.limitOrders = append(b.limitOrders[:0], state.LimitOrders...)
	b.realized = state.Realized
	b.orbLastEntry, b.hasORBEntry = state.ORBLastEntry, state.HasORBEntry
	b.seen.orb.day = state.ORBSeenDay
	b.seen.orb.keys = append(b.seen.orb.keys[:0], state.ORBSeenKeys...)
	b.lastExitIndex = state.LastExitIndex
}

func encodePrefixCheckpoint(request RunRequest, b *broker) (json.RawMessage, error) {
	count := request.Series.Len()
	binding, err := prefixCheckpointBindingDigest(request)
	if err != nil {
		return nil, err
	}
	bars, err := prefixCheckpointBarsDigest(request, count)
	if err != nil {
		return nil, err
	}
	lastT := float64(0)
	if count > 0 {
		lastT = request.Series.T[count-1]
	}
	state := checkpointStateFromBroker(b)
	stateDigest, err := canonicalDigest(prefixCheckpointStateDomain, state)
	if err != nil {
		return nil, fmt.Errorf("checkpoint state binding: %w", err)
	}
	payload, err := json.Marshal(prefixCheckpointEnvelope{
		Schema: prefixCheckpointSchema, BindingDigest: binding, AdmittedPrefixDigest: bars,
		StateDigest: "sha256:" + stateDigest, ProcessedBars: count, LastBarT: lastT, State: state,
	})
	if err != nil {
		return nil, fmt.Errorf("encode prefix checkpoint: %w", err)
	}
	return payload, nil
}

func decodePrefixCheckpoint(request RunRequest, raw json.RawMessage) (prefixCheckpointEnvelope, error) {
	var checkpoint prefixCheckpointEnvelope
	if len(raw) == 0 {
		return checkpoint, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&checkpoint); err != nil {
		return checkpoint, &PrefixCheckpointError{Reason: "malformed envelope"}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return checkpoint, &PrefixCheckpointError{Reason: "malformed envelope"}
	}
	if checkpoint.Schema != prefixCheckpointSchema {
		return checkpoint, &PrefixCheckpointError{Reason: "unsupported schema"}
	}
	binding, err := prefixCheckpointBindingDigest(request)
	if err != nil {
		return checkpoint, err
	}
	if checkpoint.BindingDigest != binding {
		return checkpoint, &PrefixCheckpointError{Reason: "binding digest mismatch"}
	}
	if checkpoint.ProcessedBars < 0 || checkpoint.ProcessedBars > request.Series.Len() {
		return checkpoint, &PrefixCheckpointError{Reason: "processed bar count is outside the supplied prefix"}
	}
	if checkpoint.ProcessedBars > 0 && checkpoint.LastBarT != request.Series.T[checkpoint.ProcessedBars-1] {
		return checkpoint, &PrefixCheckpointError{Reason: "last admitted bar timestamp mismatch"}
	}
	bars, err := prefixCheckpointBarsDigest(request, checkpoint.ProcessedBars)
	if err != nil {
		return checkpoint, err
	}
	if checkpoint.AdmittedPrefixDigest != bars {
		return checkpoint, &PrefixCheckpointError{Reason: "admitted prefix digest mismatch"}
	}
	stateDigest, err := canonicalDigest(prefixCheckpointStateDomain, checkpoint.State)
	if err != nil {
		return checkpoint, &PrefixCheckpointError{Reason: "state is not encodable"}
	}
	if checkpoint.StateDigest != "sha256:"+stateDigest {
		return checkpoint, &PrefixCheckpointError{Reason: "state digest mismatch"}
	}
	return checkpoint, nil
}

// RunPrefixResumable replays an opening-range-breakout prefix from an opaque
// engine checkpoint. Callers must still supply the complete extended bar
// prefix; the checkpoint skips already-admitted bars but does not replace them.
// The returned trades are deltas since the supplied checkpoint.
func RunPrefixResumable(request RunRequest, raw json.RawMessage) (PrefixResult, json.RawMessage, error) {
	prepared, err := PrepareRun(request)
	if err != nil {
		return PrefixResult{}, nil, err
	}
	if path := checkpointUnsupportedPath(prepared, request); path != "" {
		return PrefixResult{}, nil, &PrefixCheckpointUnsupportedError{Path: path}
	}
	prepared.fixture.Costs = request.Costs.normalized()
	if err := validateDerivedOutput(prepared.fixture.Costs, nil); err != nil {
		return PrefixResult{}, nil, err
	}
	checkpoint, err := decodePrefixCheckpoint(request, raw)
	if err != nil {
		return PrefixResult{}, nil, err
	}
	prepared.broker.reset(prepared.series, prepared.cols, prepared.htfTrend, prepared.ema, prepared.emaSlope, prepared.params, prepared.fixture, prepared.trades)
	start := 0
	if len(raw) != 0 {
		restoreCheckpointState(&prepared.broker, checkpoint.State)
		start = checkpoint.ProcessedBars
	}
	trades := prepared.broker.runRangeWithFinalization(start, false)
	positions := prepared.broker.openPositionSnapshot()
	if err := validatePrefixOutput(prepared.fixture.Costs, trades, positions); err != nil {
		return PrefixResult{}, nil, err
	}
	closed := make([]PrefixClosedTrade, len(trades))
	for i, trade := range trades {
		positionID, err := prefixPositionID(request.StrategyID, request.Symbol, request.Timeframe, trade.EntryT, trade.EntryIndex, trade.Side)
		if err != nil {
			return PrefixResult{}, nil, err
		}
		closed[i] = PrefixClosedTrade{PositionID: positionID, Trade: trade}
	}
	for i := range positions {
		positionID, err := prefixPositionID(request.StrategyID, request.Symbol, request.Timeframe, positions[i].EntryT, positions[i].EntryIndex, positions[i].Side)
		if err != nil {
			return PrefixResult{}, nil, err
		}
		positions[i].PositionID = positionID
	}
	digest, err := prefixCheckpointDigest(request)
	if err != nil {
		return PrefixResult{}, nil, err
	}
	next, err := encodePrefixCheckpoint(request, &prepared.broker)
	if err != nil {
		return PrefixResult{}, nil, err
	}
	return PrefixResult{Trades: closed, OpenPositions: positions, CheckpointDigest: digest}, next, nil
}
