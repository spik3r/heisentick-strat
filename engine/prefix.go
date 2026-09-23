package engine

import (
	"fmt"

	"github.com/spik3r/heisentick-strat/dsl"
)

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
// end of an explicitly preserved prefix. It deliberately has no public stable
// lifecycle identifier; HT-037 assigns that contract in a later slice.
type OpenPositionSnapshot struct {
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

// PrefixResult reports closed trades plus the position preserved after the
// last supplied bar. Existing Run and report results keep liquidating at the
// final close and are unchanged.
type PrefixResult struct {
	Trades        []Trade                `json:"trades"`
	OpenPositions []OpenPositionSnapshot `json:"openPositions"`
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
	if prepared.offRoute {
		return PrefixResult{Trades: []Trade{}, OpenPositions: []OpenPositionSnapshot{}}, nil
	}
	prepared.broker.reset(prepared.series, prepared.cols, prepared.htfTrend, prepared.ema, prepared.emaSlope, prepared.params, prepared.fixture, prepared.trades)
	if prepared.windowed {
		prepared.broker.setExecutionWindow(prepared.execution)
	}
	trades := prepared.broker.runWithFinalization(false)
	return PrefixResult{Trades: append([]Trade(nil), trades...), OpenPositions: prepared.broker.openPositionSnapshot()}, nil
}
