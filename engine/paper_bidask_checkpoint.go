package engine

import (
	"bytes"
	"encoding/json"
	"reflect"
)

// A replay receipt authenticates nothing. The consumer must atomically fence
// and commit the candidate, unique effects, account/protection, episode and input
// cursor. Native verification recomputes both prefixes; no host state is loaded.
type bidAskCheckpoint struct {
	Schema        string `json:"schema"`
	BindingDigest string `json:"bindingDigest"`
	InputDigest   string `json:"inputDigest"`
	RecordCount   int    `json:"recordCount"`
	EventCount    int    `json:"eventCount"`
	EventDigest   string `json:"eventDigest"`
	StateDigest   string `json:"stateDigest"`
}

func jsonMarshalBidAsk(v any) ([]byte, error) { return json.Marshal(v) }
func bidAskCheckpointFor(r PaperBidAskResult) bidAskCheckpoint {
	return bidAskCheckpoint{Schema: "heisentick-paper-bidask-replay-v1", BindingDigest: r.BindingDigest, InputDigest: r.InputDigest, RecordCount: r.ProcessedRecords, EventCount: r.EventRange.To, EventDigest: bidAskHash(r.Events), StateDigest: r.StateDigest}
}
func replayBidAsk(r PaperBidAskRequest, p *PreparedRun, b BidAskBinding) (PaperBidAskResult, error) {
	from := 0
	if len(r.Checkpoint) > 0 {
		var cp bidAskCheckpoint
		if len(r.Checkpoint) > 4096 || paperJSONUnique(r.Checkpoint) != nil || paperJSONStrings(r.Checkpoint) != nil || bidAskShape(r.Checkpoint, reflect.TypeOf(cp)) != nil {
			return PaperBidAskResult{}, bidAskError("CHECKPOINT")
		}
		decoder := json.NewDecoder(bytes.NewReader(r.Checkpoint))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&cp) != nil || cp.RecordCount < 0 || cp.RecordCount > len(r.Records) || cp.EventCount < 0 || cp.EventCount > PaperBidAskMaxEvents || cp.BindingDigest != bidAskHash(b) {
			return PaperBidAskResult{}, bidAskError("CHECKPOINT")
		}
		prior := r
		prior.Records = r.Records[:cp.RecordCount]
		prior.Checkpoint = nil
		pp, pb, e := prepareBidAsk(prior, false)
		if e != nil {
			return PaperBidAskResult{}, bidAskError("CHECKPOINT")
		}
		expected, e := executeBidAsk(prior, pp, pb)
		if e != nil || bidAskCheckpointFor(expected) != cp {
			return PaperBidAskResult{}, bidAskError("CHECKPOINT")
		}
		from = cp.EventCount
	}
	result, e := executeBidAsk(r, p, b)
	if e != nil {
		return PaperBidAskResult{}, e
	}
	if from > len(result.Events) {
		return PaperBidAskResult{}, bidAskError("CHECKPOINT")
	}
	result.EventRange.From = from
	result.Events = append([]BidAskEvent{}, result.Events[from:]...)
	if _, e = EncodePaperBidAskResult(result); e != nil {
		return PaperBidAskResult{}, e
	}
	return result, nil
}
