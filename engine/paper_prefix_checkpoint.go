package engine

import (
	"bytes"
	"encoding/json"
)

const paperCheckpointSchema = "heisentick-paper-replay-checkpoint-v1"

type paperCheckpoint struct {
	Schema         string  `json:"schema"`
	BindingDigest  string  `json:"bindingDigest"`
	InputDigest    string  `json:"inputDigest"`
	ProcessedBars  int     `json:"processedBars"`
	LastBarT       float64 `json:"lastBarT"`
	ClosedThroughT int64   `json:"closedThroughT"`
	EventCount     int     `json:"eventCount"`
	StateDigest    string  `json:"stateDigest"`
}

func newPaperCheckpoint(r PaperPrefixResult, b *broker, c *paperCollector) (paperCheckpoint, error) {
	digest, err := paperDigest(struct {
		Broker             prefixCheckpointState
		Account            PaperAccount
		Statistics         PaperTradeStatistics
		Open               *paperOpenAccount
		LastIndex, Ordinal int
		VWAP               *paperVWAPState `json:",omitempty"`
	}{checkpointStateFromBroker(b), c.account, c.statistics, c.open, c.lastIndex, c.ordinal, paperVWAPStateFromBroker(b)})
	if err != nil {
		return paperCheckpoint{}, err
	}
	return paperCheckpoint{paperCheckpointSchema, r.BindingDigest, r.InputDigest, r.ProcessedBars, r.LastBarT, r.ClosedThroughT, len(c.events), digest}, nil
}

func verifyPaperCheckpoint(request PaperPrefixRequest, binding PaperBinding) (paperCheckpoint, error) {
	var cp paperCheckpoint
	if len(request.Checkpoint) > 4096 || paperJSONUnique(request.Checkpoint) != nil {
		return cp, paperError("CHECKPOINT")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(request.Checkpoint, &fields) != nil || paperFields(fields, []string{"schema", "bindingDigest", "inputDigest", "processedBars", "lastBarT", "closedThroughT", "eventCount", "stateDigest"}, nil) != nil {
		return cp, paperError("CHECKPOINT")
	}
	if !paperExactTimestamp(fields["lastBarT"]) {
		return cp, paperError("CHECKPOINT")
	}
	decoder := json.NewDecoder(bytes.NewReader(request.Checkpoint))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cp) != nil || cp.Schema != paperCheckpointSchema || cp.ProcessedBars < 1 || cp.ProcessedBars > len(request.Bars) || cp.EventCount < 0 || cp.EventCount > PaperPrefixMaxEvents || cp.ClosedThroughT > request.ClosedThroughT {
		return cp, paperError("CHECKPOINT")
	}
	identity, err := paperDigest(binding)
	if err != nil {
		return cp, err
	}
	if cp.BindingDigest != identity {
		return cp, paperError("CHECKPOINT")
	}
	prior := request
	prior.Checkpoint = nil
	prior.Bars = request.Bars[:cp.ProcessedBars]
	prior.ClosedThroughT = cp.ClosedThroughT
	prepared, priorBinding, err := preparePaperPrefix(prior)
	if err != nil {
		return cp, paperError("CHECKPOINT")
	}
	_, expected, err := executePaperPrefix(prior, prepared, priorBinding)
	if err != nil {
		return cp, paperError("CHECKPOINT")
	}
	if cp != expected {
		return cp, paperError("CHECKPOINT")
	}
	return cp, nil
}
