package frozencontrol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/spik3r/heisentick-strat/engine/frozeninput"
	"github.com/spik3r/heisentick-strat/engine/frozenlevels"
)

// There is deliberately no production implementation or constructor for this
// package-private interface. Only tests can construct the synthetic authority.
// A hash, label or boolean cannot substitute for verify's raw correspondence
// and finite-path checks. Continuity is queried at each actual transition;
// verification must not retroactively reject an initially qualified prefix.
type fixtureAuthority interface {
	verify(*frozeninput.Input, request) error
	calendar() (frozenlevels.Calendar, error)
	openings(frozenlevels.Source) ([]frozenlevels.Opening, error)
	contextKey(start, end int64) (frozenlevels.EventKey, error)
	transition(previous, current frozenlevels.EventKey) error
}

func hash(raw []byte) string { s := sha256.Sum256(raw); return hex.EncodeToString(s[:]) }
func encoded(v any) []byte {
	b, e := json.Marshal(v)
	if e != nil {
		panic("validated trace value")
	}
	return b
}

type contexts struct {
	Bars              []frozenlevels.CompletedBar `json:"bars"`
	Stored            frozenlevels.Window         `json:"storedM30"`
	Rolling           frozenlevels.Window         `json:"rollingSixM5"`
	PivotWindow       frozenlevels.Window         `json:"pivotWindow"`
	Ladder            frozenlevels.Ladder         `json:"ladder"`
	Opening           *frozenlevels.FrozenOpening `json:"opening"`
	OpeningError      string                      `json:"openingError"`
	Spec              frozenlevels.LevelSpec      `json:"levelSpec"`
	LockLevels        frozenlevels.LockLevels     `json:"lockLevels"`
	SignalKey         frozenlevels.EventKey       `json:"signalKey"`
	EntryCandidateKey frozenlevels.EventKey       `json:"entryCandidateKey"`
	InitialStop       float64                     `json:"initialStop"`
	Target            float64                     `json:"target"`
	RiskBudgetUSD     string                      `json:"riskBudgetUSD"`
	QuantityOz        string                      `json:"quantityOz"`
	CompletionKey     frozenlevels.EventKey       `json:"contextCompletionKey"`
}

func sourceBars(in *frozeninput.Input, w fixtureAuthority, start, end int64) ([]frozenlevels.CompletedBar, error) {
	if start < 0 || end <= start || start%frozenlevels.M5Millis != 0 || end%frozenlevels.M5Millis != 0 || end-start > 9*frozenlevels.M5Millis {
		return nil, fmt.Errorf("unsupported-synthetic-window")
	}
	source, ok := in.Source()
	if !ok {
		return nil, fmt.Errorf("input-not-admitted")
	}
	bars := []frozenlevels.CompletedBar{}
	for at := start; at < end; at += frozenlevels.M5Millis {
		closeAt := at + frozenlevels.M5Millis
		key, err := w.contextKey(at, closeAt)
		if err != nil {
			return nil, err
		}
		if key.AtMillis != closeAt {
			return nil, fmt.Errorf("invalid-context-completion-time")
		}
		bar := frozenlevels.CompletedBar{StartMillis: at, EndMillis: closeAt, KnownAtMillis: key.AtMillis, Complete: true, Source: source}
		count := 0
		var last frozenlevels.EventKey
		for i := 0; i < in.Len(); i++ {
			obs, _ := in.Observation(i)
			q := obs.Quote
			if q.Event.AtMillis < at {
				continue
			}
			if q.Event.AtMillis >= closeAt {
				if q.Event.AtMillis == closeAt && !q.Event.After(key) {
					return nil, fmt.Errorf("unproved-context-boundary-order")
				}
				break
			}
			if count == 0 {
				if q.Event.AtMillis != at {
					return nil, fmt.Errorf("missing-synthetic-bucket-open")
				}
				bar.Open, bar.High, bar.Low = q.Bid, q.Bid, q.Bid
			}
			if q.Bid > bar.High {
				bar.High = q.Bid
			}
			if q.Bid < bar.Low {
				bar.Low = q.Bid
			}
			bar.Close = q.Bid
			last = q.Event
			count++
		}
		if count == 0 || !key.After(last) || key.Sequence != last.Sequence {
			return nil, fmt.Errorf("unproved-context-completion-key")
		}
		bars = append(bars, bar)
	}
	return bars, nil
}
func window(in *frozeninput.Input, w fixtureAuthority, kind frozenlevels.WindowKind, at int64) (frozenlevels.Window, error) {
	alignment, duration := frozenlevels.M5Millis, 6*frozenlevels.M5Millis
	switch kind {
	case frozenlevels.ClockM30:
		alignment = duration
	case frozenlevels.ClockM15:
		alignment = 3 * frozenlevels.M5Millis
		duration = alignment
	case frozenlevels.RollingM30:
	default:
		return frozenlevels.Window{}, fmt.Errorf("unsupported-window")
	}
	end := at / alignment * alignment
	bars, e := sourceBars(in, w, end-duration, end)
	if e != nil {
		return frozenlevels.Window{}, e
	}
	return frozenlevels.FreezeWindow(kind, at, bars)
}
func signalContexts(in *frozeninput.Input, w fixtureAuthority, stored frozenlevels.Window, spec frozenlevels.LevelSpec, q frozenlevels.Quote, side frozenlevels.Direction, grid float64) (contexts, error) {
	c := contexts{Stored: stored, Spec: spec, SignalKey: q.Event}
	var e error
	c.Rolling, e = window(in, w, frozenlevels.RollingM30, q.Event.AtMillis)
	if e != nil {
		return c, e
	}
	if c.Rolling.NetDirection() != int(side) {
		return c, fmt.Errorf("momentum-does-not-support-side")
	}
	c.PivotWindow, e = window(in, w, frozenlevels.ClockM15, q.Event.AtMillis)
	if e != nil {
		return c, e
	}
	c.Ladder, e = frozenlevels.Camarilla(c.PivotWindow)
	if e != nil {
		return c, e
	}
	c.Bars, e = sourceBars(in, w, stored.StartMillis, c.Rolling.EndMillis)
	if e != nil {
		return c, e
	}
	c.CompletionKey, e = w.contextKey(c.Rolling.EndMillis-frozenlevels.M5Millis, c.Rolling.EndMillis)
	if e != nil {
		return c, e
	}
	calendar, e := w.calendar()
	if e != nil {
		return c, e
	}
	source, _ := in.Source()
	openings, e := w.openings(source)
	if e != nil {
		return c, e
	}
	opening, e := frozenlevels.FreezePriorOpening(calendar, q.Event.AtMillis, openings)
	if e != nil {
		c.OpeningError = e.Error()
	} else {
		c.Opening = &opening
	}
	c.LockLevels = frozenlevels.LockLevels{ID: hash(encoded(c.Ladder)), SourceWindowID: c.Ladder.WindowID, KnownAtMillis: c.Ladder.KnownAtMillis, FrozenAtMillis: c.Ladder.FrozenAtMillis, Prices: []float64{}}
	for _, p := range c.Ladder.Levels {
		c.LockLevels.Prices = append(c.LockLevels.Prices, p.Price)
	}
	c.InitialStop, e = structuralStop(c.Rolling, side, grid)
	return c, e
}

func checkContextBindings(in *frozeninput.Input, c contexts) error {
	bindings := in.OrderingDeclaration().ContextBindings
	check := func(id, kind string, start, end int64) error {
		for _, b := range bindings {
			if b.WindowID == id && b.Kind == kind && b.Interval.StartMS == start && b.Interval.EndMS == end {
				return nil
			}
		}
		return fmt.Errorf("derived-context-binding-mismatch: %s", kind)
	}
	for _, x := range []struct {
		kind string
		w    frozenlevels.Window
	}{{"stored-clock-M30", c.Stored}, {"rolling-six-M5", c.Rolling}, {"completed-clock-M15", c.PivotWindow}} {
		if e := check(x.w.ID, x.kind, x.w.StartMillis, x.w.EndMillis); e != nil {
			return e
		}
	}
	if c.Opening != nil {
		return check(c.Opening.ID, "prior-broker-opening", c.Opening.Opening.Event.AtMillis, c.Opening.Opening.Event.AtMillis+1)
	}
	return nil
}
