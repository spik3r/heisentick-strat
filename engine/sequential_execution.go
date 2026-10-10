package engine

import (
	"math"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine/seqcore"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// This adapter owns only Sequential scheduling. The unchanged Go broker owns
// position creation, entry/exit costs, stop-first bracket resolution and trades.
// All public callers must first perform the family-specific input admission.
func runSequentialFull(spec dsl.SequentialFullSpec, series marketdata.Series, fixture RunFixture) ([]Trade, SequentialFullAudit, error) {
	return runSequentialFullMode(spec, series, fixture, true)
}

// Non-finalizing mode exists only for internal causality tests. It is not a
// public execution-prefix/checkpoint capability and does not liquidate a trade.
func runSequentialFullMode(spec dsl.SequentialFullSpec, series marketdata.Series, fixture RunFixture, finalize bool) ([]Trade, SequentialFullAudit, error) {
	s, err := newSequentialFullScheduler(spec, series, fixture)
	if err != nil {
		return nil, SequentialFullAudit{}, err
	}
	for i := 0; i < series.Len(); i++ {
		if err := s.step(i); err != nil {
			return nil, s.audit, err
		}
	}
	if finalize {
		if s.pending >= 0 {
			s.reject(s.pending, SequentialFullNoNextBar)
			s.pending = -1
		}
		if s.broker.hasPosition {
			id, _ := s.broker.position.Meta["opportunityId"].(string)
			return nil, s.audit, &SequentialFullExecutionError{"unsupported-incomplete-terminal-run", "held position", series.Len() - 1, id}
		}
	}
	return append([]Trade{}, s.broker.trades...), s.audit, nil
}

type sequentialFullAnchor struct {
	completion int
	first      int
	low, high  float64
}

type sequentialFullScheduler struct {
	spec    dsl.SequentialFullSpec
	series  marketdata.Series
	broker  broker
	core    *seqcore.Engine
	atr     []float64
	setups  map[string]sequentialFullAnchor
	seen    map[string]bool
	audit   SequentialFullAudit
	pending int
	hold    int
}

func newSequentialFullScheduler(spec dsl.SequentialFullSpec, series marketdata.Series, fixture RunFixture) (*sequentialFullScheduler, error) {
	step, ok := dsl.SequentialFullTimeframeMS(spec.Timeframe)
	if !ok || spec.Profile != seqcore.ProfileID || (spec.Policy != "E1" && spec.Policy != "E2") {
		return nil, &SequentialFullExecutionError{"invalid-sequential-input", "profile, policy or timeframe", -1, ""}
	}
	identity := seqcore.DefaultIdentity(seqcore.Source{Symbol: spec.Symbol, Timeframe: spec.Timeframe}, "sequential-full-execution.v1", step)
	core, err := seqcore.New(seqcore.DefaultConfig(), identity)
	if err != nil {
		return nil, err
	}
	s := &sequentialFullScheduler{
		spec: spec, series: series, core: core, atr: contextcols.ComputeATR(series, 14),
		setups: map[string]sequentialFullAnchor{}, seen: map[string]bool{}, pending: -1, hold: 4,
		audit: SequentialFullAudit{sequentialFullAuditSchema, spec.Profile, spec.Policy, spec.Symbol, spec.Timeframe, []SequentialFullOpportunity{}},
	}
	if spec.Policy == "E2" {
		s.hold = 12
	}
	// No generic family defaults, gates, trails, partial exits or EOT behavior.
	s.broker.reset(series, contextcols.Columns{}, nil, nil, nil, flagParams{}, fixture, []Trade{})
	for i, atr := range s.atr {
		if !isFinite(atr) || atr < 0 {
			return nil, s.derivedError("ATR14", i, "")
		}
	}
	return s, nil
}

func (s *sequentialFullScheduler) step(i int) error {
	b := &s.broker
	// The entry bar is held bar one. Time exit precedes every bracket check
	// at e+N, including a stop/target opening gap on that bar.
	if b.hasPosition && i-b.position.EntryIndex >= s.hold {
		b.closePosition(s.series.O[i], i, "time", "")
	}
	if s.pending >= 0 {
		pending := s.pending
		s.pending = -1
		if err := s.fill(pending, i); err != nil {
			return err
		}
	}
	b.resolveIntrabarExit(i)
	if err := s.checkBrokerNumbers(i); err != nil {
		return err
	}
	_, events, err := s.core.Step(seqcore.Bar{OpenMS: int64(s.series.T[i]), O: s.series.O[i], H: s.series.H[i], L: s.series.L[i], C: s.series.C[i]})
	if err != nil {
		return &SequentialFullExecutionError{"invalid-sequential-input", err.Error(), i, ""}
	}
	return s.acceptEvents(i, events)
}

func (s *sequentialFullScheduler) acceptEvents(i int, events []seqcore.Event) error {
	candidates := []int{}
	for _, event := range events {
		if event.Type == "setup_complete" {
			first := i - 8
			if first < 0 {
				return &SequentialFullExecutionError{"invalid-sequential-input", "setup anchor", i, event.EpisodeID}
			}
			s.setups[event.EpisodeID] = sequentialFullAnchor{i, first, recentExtreme(s.series, i, 9, sideLong), recentExtreme(s.series, i, 9, sideShort)}
		}
		trigger := ""
		if s.spec.Policy == "E1" && (event.Type == "perfection" || event.Type == "delayed_perfection") {
			trigger = "perf"
		}
		if s.spec.Policy == "E2" && event.Type == "countdown_complete" {
			trigger = "cd13"
		}
		if trigger == "" {
			continue
		}
		id := event.EpisodeID + ":" + trigger
		if s.seen[id] {
			continue
		}
		s.seen[id] = true
		anchor, found := s.setups[event.EpisodeID]
		if !found {
			return &SequentialFullExecutionError{"invalid-sequential-input", "missing admitting setup", i, id}
		}
		direction, extreme := sideLong, anchor.low
		if event.Side == "sell" {
			direction, extreme = sideShort, anchor.high
		} else if event.Side != "buy" {
			return &SequentialFullExecutionError{"invalid-sequential-input", "event side", i, id}
		}
		if s.spec.Policy == "E2" {
			extreme = recentExtreme(s.series, i, i-anchor.first+1, direction)
		}
		stop := extreme - float64(float64(direction)*float64(0.10*s.atr[i]))
		if !isFinite(stop) {
			return s.derivedError("stop", i, id)
		}
		opportunity := SequentialFullOpportunity{
			ID: id, EpisodeID: event.EpisodeID, Trigger: trigger, Side: direction.String(),
			SetupIndex: anchor.completion, SetupFirstIndex: anchor.first, DecisionIndex: i,
			DecisionOpenMS: event.BarOpenMS, DecisionMS: event.DecisionMS, NextOpenIndex: i + 1,
			Status: SequentialFullPending, Stop: stop, SignalATR: s.atr[i],
		}
		s.audit.Opportunities = append(s.audit.Opportunities, opportunity)
		index := len(s.audit.Opportunities) - 1
		if i < 13 {
			s.reject(index, SequentialFullWarmup)
		} else if s.spec.Policy == "E1" && i-anchor.completion > 4 {
			s.reject(index, SequentialFullExpired)
		} else {
			candidates = append(candidates, index)
		}
	}
	// Decide on the set of executable sides, never on seqcore event ordering.
	long, short := false, false
	for _, index := range candidates {
		long = long || s.audit.Opportunities[index].Side == "long"
		short = short || s.audit.Opportunities[index].Side == "short"
	}
	for _, index := range candidates {
		switch {
		case s.broker.hasPosition || s.pending >= 0:
			s.reject(index, SequentialFullOccupied)
		case long && short:
			s.reject(index, SequentialFullSimultaneous)
		default:
			s.pending = index
		}
	}
	return nil
}

func (s *sequentialFullScheduler) fill(index, i int) error {
	o := &s.audit.Opportunities[index]
	direction := sideLong
	if o.Side == "short" {
		direction = sideShort
	}
	base := s.series.O[i]
	fill := base + float64(float64(direction)*s.broker.slippageAt(base))
	if !isFinite(fill) {
		return s.derivedError("fill", i, o.ID)
	}
	// The frozen rule is evaluated against the actual slipped fill.
	if (direction == sideLong && o.Stop >= fill) || (direction == sideShort && o.Stop <= fill) {
		s.reject(index, SequentialFullWrongSide)
		return nil
	}
	distance := math.Abs(fill - o.Stop)
	if !isFinite(distance) || distance <= 0 {
		return s.derivedError("fill bracket", i, o.ID)
	}
	twiceRisk := float64(2 * distance)
	target := fill + float64(float64(direction)*twiceRisk)
	if math.IsInf(twiceRisk, 1) {
		// Preserve ordinary ordered binary64 arithmetic. Only when doubling
		// overflows, distribute the two equal directional distances across
		// two rounded additions: opposite-signed fill may make final 2R finite.
		// If either partial sum overflows in this monotone direction, the
		// final target is not representable and the finite guard rejects it.
		oneRisk := float64(float64(direction) * distance)
		target = float64(fill+oneRisk) + oneRisk
	}
	if !isFinite(target) {
		return s.derivedError("fill bracket", i, o.ID)
	}
	riskSize := s.spec.RiskUSD / distance
	// The cap is absolute entry notional, including for negative prices. At
	// zero fill every finite quantity has zero entry notional. A positive
	// quotient may overflow without invalidating the other, binding bound;
	// only their minimum must be representable as a positive finite quantity.
	capSize := math.Inf(1)
	if fill != 0 {
		capSize = s.spec.MaxNotionalUSD / math.Abs(fill)
	}
	if math.IsNaN(riskSize) || riskSize <= 0 || math.IsNaN(capSize) || capSize <= 0 {
		return s.derivedError("size bound", i, o.ID)
	}
	size := math.Min(riskSize, capSize)
	if !isFinite(size) || size <= 0 {
		return s.derivedError("size", i, o.ID)
	}
	capBinds := capSize < riskSize
	if !isFinite(size*math.Abs(fill)) || !isFinite(s.broker.costs.FeePerUnit*size) {
		return s.derivedError("notional or entry fee", i, o.ID)
	}
	s.broker.openPosition(direction, fill, order{
		Side: direction, SL: o.Stop, TP: target, Size: size, HasSize: true,
		NoSlip: true, GapAwareStop: true, Tag: "SEQUENTIAL-" + s.spec.Policy,
		Meta: TradeMeta{"opportunityId": o.ID, "episodeId": o.EpisodeID, "profile": s.spec.Profile, "policy": s.spec.Policy, "signalIndex": float64(o.DecisionIndex), "signalAtr": o.SignalATR},
	}, i)
	o.Status = SequentialFullFilled
	o.FillIndex, o.Fill, o.Target = sequentialFullPointer(i), sequentialFullPointer(fill), sequentialFullPointer(target)
	o.RiskDistance, o.Size, o.CapBinds = sequentialFullPointer(distance), sequentialFullPointer(size), capBinds
	return nil
}

func (s *sequentialFullScheduler) reject(index int, reason SequentialFullReason) {
	s.audit.Opportunities[index].Status = SequentialFullRejected
	s.audit.Opportunities[index].Reason = reason
}

func (s *sequentialFullScheduler) derivedError(field string, i int, id string) error {
	return &SequentialFullExecutionError{"invalid-sequential-derived-value", field, i, id}
}

func (s *sequentialFullScheduler) checkBrokerNumbers(i int) error {
	if !isFinite(s.broker.realized) {
		return s.derivedError("realized", i, "")
	}
	if n := len(s.broker.trades); n != 0 {
		t := s.broker.trades[n-1]
		if !isFinite(t.Exit) || !isFinite(t.Points) || !isFinite(t.PnL) {
			id, _ := t.Meta["opportunityId"].(string)
			return s.derivedError("exit or trade PnL", i, id)
		}
	}
	return nil
}

func sequentialFullPointer[T any](value T) *T { return &value }
