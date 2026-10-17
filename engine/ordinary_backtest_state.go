package engine

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

func ordinaryStateEqual(a, b any) bool {
	return ordinaryStateEqualValue(reflect.ValueOf(a), reflect.ValueOf(b))
}

func ordinaryStateEqualValue(a, b reflect.Value) bool {
	if !a.IsValid() || !b.IsValid() {
		return a.IsValid() == b.IsValid()
	}
	if a.Type() != b.Type() {
		return false
	}
	switch a.Kind() {
	case reflect.Float64, reflect.Float32:
		return math.Float64bits(a.Float()) == math.Float64bits(b.Float())
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			if !ordinaryStateEqualValue(a.Field(i), b.Field(i)) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		if a.Len() != b.Len() || (a.Kind() == reflect.Slice && a.IsNil() != b.IsNil()) {
			return false
		}
		for i := 0; i < a.Len(); i++ {
			if !ordinaryStateEqualValue(a.Index(i), b.Index(i)) {
				return false
			}
		}
	case reflect.Map:
		if a.IsNil() != b.IsNil() || a.Len() != b.Len() {
			return false
		}
		for _, key := range a.MapKeys() {
			if !ordinaryStateEqualValue(a.MapIndex(key), b.MapIndex(key)) {
				return false
			}
		}
	case reflect.Interface, reflect.Pointer:
		if a.IsNil() != b.IsNil() {
			return false
		}
		return a.IsNil() || ordinaryStateEqualValue(a.Elem(), b.Elem())
	case reflect.String:
		return a.String() == b.String()
	case reflect.Bool:
		return a.Bool() == b.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return a.Int() == b.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return a.Uint() == b.Uint()
	default:
		return false
	}
	return true
}

func ordinaryStateSameContents(a, b ordinaryBacktestState) bool {
	a.Boundary, b.Boundary = "", ""
	a.Index, b.Index, a.T, b.T, a.EventCount, b.EventCount = 0, 0, 0, 0, 0, 0
	return ordinaryStateEqual(a, b)
}

func ordinaryStateSameQueues(a, b ordinaryBacktestState) bool {
	return ordinaryStateEqual(a.PendingOrders, b.PendingOrders) && ordinaryStateEqual(a.PendingExits, b.PendingExits) && ordinaryStateEqual(a.LimitOrders, b.LimitOrders)
}

func ordinaryStateFixedPosition(p position) position {
	// Stop/target/metadata may change inside strategy management. Size and the
	// remaining identity/flags may change only at observed financial boundaries.
	p.SL, p.TP, p.Meta = 0, 0, nil
	return p
}

// ordinaryBacktestState is an actual observation, never a reconstructed state.
// Position retains the stored position even when HasPosition is false; in that
// case its fields are inactive, possibly left over from a previous lifecycle.
// Queue-stage boundaries and immediate financial boundaries are intentionally
// distinct: a pending order can already be removed before before-entry/exit.
type ordinaryBacktestState struct {
	Boundary      string
	Index         int
	T             float64
	HasPosition   bool
	Position      position
	PendingOrders []order
	PendingExits  []pendingExit
	LimitOrders   []order
	Realized      float64
	TradeCount    int
	EventCount    int
}

type ordinaryBacktestStateCapture struct {
	financial *ordinaryBacktestFinancialCapture
	states    []ordinaryBacktestState
	seals     [][32]byte
	err       error
	finished  bool
	nodes     int
	textBytes int
}

// These are evidence budgets, not execution truncation. Crossing one refuses
// the complete capture; the observer never changes broker execution.
const ordinaryStateMaxSnapshots = 32*OrdinaryBacktestMaxBars + 2
const ordinaryStateMaxNodes = 32 * OrdinaryBacktestMaxNodes
const ordinaryStateMaxTextBytes = 64 * 1024 * 1024

func ordinaryStateError(field string, index int) error {
	return fmt.Errorf("ordinary state capture: %s at bar %d", field, index)
}

// Attach after reset, before any execution. This private wrapper does not admit
// a route or execute it. In particular, special loops without original marks
// and queue-stage witnesses cannot produce a successful result.
func attachOrdinaryBacktestStateCapture(b *broker) (*ordinaryBacktestStateCapture, error) {
	f, err := attachOrdinaryBacktestFinancialCapture(b)
	if err != nil {
		return nil, err
	}
	x := &ordinaryBacktestStateCapture{financial: f}
	f.collector.ordinaryState = x
	x.observe(b, "initial", -1)
	if x.err != nil {
		// Restore the formerly empty observer slot on an attachment refusal.
		b.paperAccounting = nil
		return nil, x.err
	}
	return x, nil
}

// Nil receivers are deliberate: capture-off calls do no work or allocation.
func (c *paperCollector) ordinaryObserve(b *broker, boundary string, index int) {
	if c != nil && c.ordinaryState != nil {
		c.ordinaryState.observe(b, boundary, index)
	}
}

func (x *ordinaryBacktestStateCapture) observe(b *broker, boundary string, index int) {
	if x.err != nil {
		return
	}
	if x.finished || x.financial == nil || b != x.financial.broker ||
		b.paperAccounting != x.financial.collector || b.paperAccounting.ordinaryState != x ||
		len(x.states) >= ordinaryStateMaxSnapshots || index < -1 || index >= len(x.financial.bars) ||
		(index >= 0 && index >= len(b.series.T)) {
		x.err = ordinaryStateError("observation lifecycle", index)
		return
	}
	s := ordinaryBacktestState{Boundary: boundary, Index: index, HasPosition: b.hasPosition,
		Position: b.position, PendingOrders: b.pendingOrders, PendingExits: b.pendingExits,
		LimitOrders: b.limitOrders, Realized: b.realized, TradeCount: len(b.trades), EventCount: len(b.paperAccounting.events)}
	if index >= 0 {
		s.T = b.series.T[index]
	}
	value, err := x.copyValue(reflect.ValueOf(s), 0)
	if err != nil {
		x.err = ordinaryStateError("uncopyable or nonfinite state", index)
		return
	}
	copied := value.Interface().(ordinaryBacktestState)
	seal, err := ordinaryStateSeal(copied)
	if err != nil {
		x.err = err
		return
	}
	x.states = append(x.states, copied)
	x.seals = append(x.seals, seal)
}

// A seal detects later alteration of retained evidence. It does not establish
// hook coverage, capture timing, financial validity or external provenance.
// Types, nil/empty, slice order and float bits are retained, unlike normalized JSON.
func ordinaryStateSeal(s ordinaryBacktestState) ([32]byte, error) {
	h := sha256.New()
	writeNumber := func(n uint64) { var b [8]byte; binary.BigEndian.PutUint64(b[:], n); _, _ = h.Write(b[:]) }
	writeText := func(s string) { writeNumber(uint64(len(s))); _, _ = h.Write([]byte(s)) }
	nodes, textBytes := 0, 0
	var visit func(reflect.Value, int) error
	visit = func(v reflect.Value, depth int) error {
		nodes++
		if depth > OrdinaryBacktestMaxDepth || nodes > ordinaryStateMaxNodes {
			return ordinaryStateError("seal budget", s.Index)
		}
		if !v.IsValid() {
			writeText("invalid")
			return nil
		}
		writeText(v.Type().PkgPath())
		writeText(v.Type().String())
		writeNumber(uint64(v.Kind()))
		switch v.Kind() {
		case reflect.Bool:
			if v.Bool() {
				writeNumber(1)
			} else {
				writeNumber(0)
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			writeNumber(uint64(v.Int()))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			writeNumber(v.Uint())
		case reflect.Float32, reflect.Float64:
			if !isFinite(v.Float()) {
				return ordinaryStateError("nonfinite seal", s.Index)
			}
			if v.Kind() == reflect.Float32 {
				writeNumber(uint64(math.Float32bits(float32(v.Float()))))
			} else {
				writeNumber(math.Float64bits(v.Float()))
			}
		case reflect.String:
			textBytes += len(v.String())
			if textBytes > ordinaryStateMaxTextBytes {
				return ordinaryStateError("seal text budget", s.Index)
			}
			writeText(v.String())
		case reflect.Interface, reflect.Pointer:
			if v.IsNil() {
				writeNumber(0)
				return nil
			}
			writeNumber(1)
			return visit(v.Elem(), depth+1)
		case reflect.Struct:
			writeNumber(uint64(v.NumField()))
			for i := 0; i < v.NumField(); i++ {
				if !v.Field(i).CanInterface() {
					return ordinaryStateError("private seal field", s.Index)
				}
				if err := visit(v.Field(i), depth+1); err != nil {
					return err
				}
			}
		case reflect.Array, reflect.Slice:
			if v.Kind() == reflect.Slice {
				if v.IsNil() {
					writeNumber(0)
					return nil
				}
				writeNumber(1)
			}
			writeNumber(uint64(v.Len()))
			if v.Len() > ordinaryStateMaxNodes-nodes {
				return ordinaryStateError("seal slice budget", s.Index)
			}
			for i := 0; i < v.Len(); i++ {
				if err := visit(v.Index(i), depth+1); err != nil {
					return err
				}
			}
		case reflect.Map:
			if v.Type().Key().Kind() != reflect.String {
				return ordinaryStateError("seal map key", s.Index)
			}
			if v.IsNil() {
				writeNumber(0)
				return nil
			}
			writeNumber(1)
			writeNumber(uint64(v.Len()))
			if v.Len() > ordinaryStateMaxNodes-nodes {
				return ordinaryStateError("seal map budget", s.Index)
			}
			keys := v.MapKeys()
			sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
			for _, key := range keys {
				if err := visit(key, depth+1); err != nil {
					return err
				}
				if err := visit(v.MapIndex(key), depth+1); err != nil {
					return err
				}
			}
		default:
			return ordinaryStateError("seal type", s.Index)
		}
		return nil
	}
	if err := visit(reflect.ValueOf(s), 0); err != nil {
		return [32]byte{}, err
	}
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result, nil
}

// Reflection preserves named container types, nil/empty distinctions, integer
// types and every float bit. JSON round-tripping would lose that evidence.
// Depth/node/text budgets also bound cyclic metadata without following it forever.
func (x *ordinaryBacktestStateCapture) copyValue(v reflect.Value, depth int) (reflect.Value, error) {
	x.nodes++
	if depth > OrdinaryBacktestMaxDepth || x.nodes > ordinaryStateMaxNodes {
		return reflect.Value{}, ordinaryStateError("copy budget", -1)
	}
	if !v.IsValid() {
		return v, nil
	}
	out := reflect.New(v.Type()).Elem()
	switch v.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		out.Set(v)
	case reflect.String:
		x.textBytes += len(v.String())
		if x.textBytes > ordinaryStateMaxTextBytes {
			return reflect.Value{}, ordinaryStateError("text budget", -1)
		}
		out.Set(v)
	case reflect.Float32, reflect.Float64:
		if !isFinite(v.Float()) {
			return reflect.Value{}, ordinaryStateError("nonfinite", -1)
		}
		out.Set(v)
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			return out, nil
		}
		child, err := x.copyValue(v.Elem(), depth+1)
		if err != nil {
			return reflect.Value{}, err
		}
		if v.Kind() == reflect.Pointer {
			out.Set(reflect.New(v.Type().Elem()))
			out.Elem().Set(child)
		} else {
			out.Set(child)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !v.Field(i).CanInterface() {
				return reflect.Value{}, ordinaryStateError("private metadata field", -1)
			}
			child, err := x.copyValue(v.Field(i), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			out.Field(i).Set(child)
		}
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice {
			if v.IsNil() {
				return out, nil
			}
			if v.Len() > ordinaryStateMaxNodes-x.nodes {
				return reflect.Value{}, ordinaryStateError("slice budget", -1)
			}
			out.Set(reflect.MakeSlice(v.Type(), v.Len(), v.Len()))
		}
		for i := 0; i < v.Len(); i++ {
			child, err := x.copyValue(v.Index(i), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			out.Index(i).Set(child)
		}
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return reflect.Value{}, ordinaryStateError("map key", -1)
		}
		if v.IsNil() {
			return out, nil
		}
		if v.Len() > ordinaryStateMaxNodes-x.nodes {
			return reflect.Value{}, ordinaryStateError("map budget", -1)
		}
		out.Set(reflect.MakeMapWithSize(v.Type(), v.Len()))
		iter := v.MapRange()
		for iter.Next() {
			key, err := x.copyValue(iter.Key(), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			child, err := x.copyValue(iter.Value(), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			out.SetMapIndex(key, child)
		}
	default:
		return reflect.Value{}, ordinaryStateError("metadata type", -1)
	}
	return out, nil
}

// finish is one-shot and all-or-nothing. No public report may use financials
// from this wrapper without the successfully validated corresponding witness.
func (x *ordinaryBacktestStateCapture) finish() (OrdinaryBacktestFinancials, []ordinaryBacktestState, error) {
	if x == nil || x.finished || x.financial == nil {
		return OrdinaryBacktestFinancials{}, nil, ordinaryStateError("finalization lifecycle", -1)
	}
	x.observe(x.financial.broker, "terminal", len(x.financial.bars)-1)
	x.finished = true
	if x.err != nil {
		return OrdinaryBacktestFinancials{}, nil, x.err
	}
	if err := x.validate(); err != nil {
		return OrdinaryBacktestFinancials{}, nil, err
	}
	f, err := x.financial.finish()
	if err != nil {
		return OrdinaryBacktestFinancials{}, nil, err
	}
	// Copy once more so returned evidence cannot mutate the retained witness.
	copyBudget := &ordinaryBacktestStateCapture{}
	v, err := copyBudget.copyValue(reflect.ValueOf(x.states), 0)
	if err != nil {
		return OrdinaryBacktestFinancials{}, nil, err
	}
	return f, v.Interface().([]ordinaryBacktestState), nil
}

func (x *ordinaryBacktestStateCapture) validate() error {
	f := x.financial
	b, c := f.broker, f.collector
	if b.paperAccounting != c || c.ordinaryState != x || c.err != nil || len(x.states) < 2 {
		return ordinaryStateError("detached or incomplete", -1)
	}
	if len(x.states) != len(x.seals) {
		return ordinaryStateError("seal cardinality", -1)
	}
	// Check before recursive comparisons: malformed cyclic retained metadata is
	// rejected by the seal traversal's budgets rather than recursing unboundedly.
	for i, s := range x.states {
		seal, err := ordinaryStateSeal(s)
		if err != nil || seal != x.seals[i] {
			return ordinaryStateError("retained witness changed", s.Index)
		}
	}
	first, last := x.states[0], x.states[len(x.states)-1]
	if first.Boundary != "initial" || first.Index != -1 || first.HasPosition || first.TradeCount != 0 ||
		first.EventCount != 0 || !paperSame(first.Realized, 0) || len(first.PendingOrders)+len(first.PendingExits)+len(first.LimitOrders) != 0 ||
		last.Boundary != "terminal" || last.Index != len(f.bars)-1 || last.HasPosition || last.TradeCount != len(b.trades) ||
		last.EventCount != len(c.events) || !paperSame(last.Realized, b.realized) {
		return ordinaryStateError("initial or terminal", -1)
	}
	if !ordinaryStateSameContents(x.states[len(x.states)-2], last) {
		return ordinaryStateError("terminal continuity", last.Index)
	}
	stages := []string{"before-pending-exits", "after-pending-exits", "before-pending-entries", "after-pending-entries", "before-limit-orders", "after-limit-orders", "before-on-bar", "after-on-bar"}
	nextStage := make([]int, len(f.bars))
	marks, events := 0, 0
	stack := []ordinaryBacktestState{}
	lastIndex := -1
	previous, live := first, first
	for _, s := range x.states[1 : len(x.states)-1] {
		if s.Index < 0 || s.Index >= len(f.bars) || s.Index < lastIndex || !paperSame(s.T, f.bars[s.Index].T) {
			return ordinaryStateError("index continuity", s.Index)
		}
		lastIndex = s.Index
		if (previous.Boundary == "after-pending-exits" && s.Boundary == "before-pending-entries") ||
			(previous.Boundary == "after-pending-entries" && s.Boundary == "before-limit-orders") ||
			(previous.Boundary == "after-limit-orders" && s.Boundary == "before-on-bar") ||
			(previous.Boundary == "after-on-bar" && s.Boundary == "original-close") ||
			(previous.Boundary == "original-close" && (s.Boundary == "before-pending-exits" || s.Boundary == "before-exit")) {
			if !ordinaryStateSameContents(previous, s) {
				return ordinaryStateError("unmutated boundary continuity", s.Index)
			}
		}
		previous = s
		financialAfter := s.Boundary == "after-entry" || s.Boundary == "after-partial" || s.Boundary == "after-exit"
		if !financialAfter && (s.HasPosition != live.HasPosition || s.TradeCount != live.TradeCount || !paperSame(s.Realized, live.Realized) ||
			(s.HasPosition && !ordinaryStateEqual(ordinaryStateFixedPosition(s.Position), ordinaryStateFixedPosition(live.Position)))) {
			return ordinaryStateError("live state continuity", s.Index)
		}
		stage := false
		for _, name := range stages {
			if s.Boundary == name {
				stage = true
				if nextStage[s.Index] >= len(stages) || stages[nextStage[s.Index]] != name {
					return ordinaryStateError("stage ordering", s.Index)
				}
				nextStage[s.Index]++
				break
			}
		}
		if strings.HasPrefix(s.Boundary, "before-") {
			if !stage && s.Boundary != "before-entry" && s.Boundary != "before-exit" && s.Boundary != "before-partial" {
				return ordinaryStateError("unknown before boundary", s.Index)
			}
			if s.EventCount != events {
				return ordinaryStateError("before event continuity", s.Index)
			}
			stack = append(stack, s)
			continue
		}
		if stage || strings.HasPrefix(s.Boundary, "after-") {
			if len(stack) == 0 {
				return ordinaryStateError("missing before", s.Index)
			}
			before := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if before.Boundary != "before-"+strings.TrimPrefix(s.Boundary, "after-") || before.Index != s.Index {
				return ordinaryStateError("boundary pairing", s.Index)
			}
			if stage {
				if s.EventCount != events {
					return ordinaryStateError("stage event continuity", s.Index)
				}
				switch s.Boundary {
				case "after-pending-exits":
					if !ordinaryStateEqual(before.PendingOrders, s.PendingOrders) || !ordinaryStateEqual(before.LimitOrders, s.LimitOrders) {
						return ordinaryStateError("pending exit stage scope", s.Index)
					}
				case "after-pending-entries":
					if !ordinaryStateEqual(before.PendingExits, s.PendingExits) || !ordinaryStateEqual(before.LimitOrders, s.LimitOrders) {
						return ordinaryStateError("pending entry stage scope", s.Index)
					}
				case "after-limit-orders":
					if !ordinaryStateEqual(before.PendingOrders, s.PendingOrders) || !ordinaryStateEqual(before.PendingExits, s.PendingExits) {
						return ordinaryStateError("limit stage scope", s.Index)
					}
				}
				continue
			}
			if events >= len(c.events) || s.EventCount != events+1 {
				return ordinaryStateError("after event continuity", s.Index)
			}
			e := c.events[events]
			if e.Kind != strings.TrimPrefix(s.Boundary, "after-") || e.Index != s.Index ||
				!paperSame(before.Realized, e.RealizedBefore) || !paperSame(s.Realized, e.RealizedAfter) || e.Effect == nil {
				return ordinaryStateError("financial linkage", s.Index)
			}
			if e.Kind == "entry" {
				if before.HasPosition || !s.HasPosition || s.TradeCount != before.TradeCount || !paperSame(s.Position.Size, e.Effect.Size) ||
					!paperSame(s.Position.Entry, e.Effect.Price) || s.Position.Side.String() != e.Effect.Side ||
					s.Position.EntryIndex != e.Index || !paperSame(s.Position.EntryT, e.T) || !ordinaryStateSameQueues(before, s) ||
					!ordinaryStateEqual(paperOptional(s.Position.SL, s.Position.NoStop), e.Effect.Stop) ||
					!ordinaryStateEqual(paperOptional(s.Position.TP, s.Position.NoTarget), e.Effect.Target) {
					return ordinaryStateError("entry state", s.Index)
				}
			} else {
				if !before.HasPosition || s.TradeCount != before.TradeCount+1 || s.HasPosition != (e.Kind == "partial") ||
					before.Position.Side.String() != e.Effect.Side || before.Position.Size < e.Effect.Size {
					return ordinaryStateError("exit state", s.Index)
				}
				if e.Kind == "exit" && !paperSame(before.Position.Size, e.Effect.Size) {
					return ordinaryStateError("actual final size", s.Index)
				}
				if e.Kind == "partial" && (s.Position.Size <= 0 || s.Position.Size > before.Position.Size || !s.Position.PartialTaken) {
					return ordinaryStateError("actual partial remainder", s.Index)
				}
				if e.Kind == "partial" {
					allowed := before.Position
					allowed.Size, allowed.PartialTaken, allowed.SL = s.Position.Size, s.Position.PartialTaken, s.Position.SL
					if !ordinaryStateEqual(allowed, s.Position) || !ordinaryStateSameQueues(before, s) {
						return ordinaryStateError("partial mutation scope", s.Index)
					}
				} else {
					var retained []pendingExit
					if before.PendingExits != nil {
						retained = make([]pendingExit, 0, len(before.PendingExits))
					}
					for _, pending := range before.PendingExits {
						if pending.PositionEntryIndex != before.Position.EntryIndex {
							retained = append(retained, pending)
						}
					}
					if !ordinaryStateEqual(before.Position, s.Position) || !ordinaryStateEqual(before.PendingOrders, s.PendingOrders) ||
						!ordinaryStateEqual(before.LimitOrders, s.LimitOrders) || !ordinaryStateEqual(retained, s.PendingExits) {
						return ordinaryStateError("exit mutation scope", s.Index)
					}
				}
			}
			live = s
			events++
			continue
		}
		if s.Boundary != "original-close" || len(stack) != 0 || s.Index != marks || nextStage[s.Index] != len(stages) ||
			events >= len(c.events) || c.events[events].Kind != "mark" || c.events[events].Index != s.Index ||
			s.EventCount != events+1 || !paperSame(s.Realized, c.events[events].RealizedAfter) {
			return ordinaryStateError("original close coverage", s.Index)
		}
		marks++
		events++
	}
	if len(stack) != 0 || marks != len(f.bars) || events != len(c.events) {
		return ordinaryStateError("incomplete witness", -1)
	}
	return nil
}
