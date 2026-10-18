package engine

import (
	"fmt"
	"math"
	"reflect"
	"runtime"
	"testing"
)

func ordinaryStatePair(t *testing.T, prices []float64, fill string) (*ordinaryFinancialPair, *ordinaryBacktestStateCapture) {
	t.Helper()
	p := ordinaryFinancialNewPair(t, ordinaryFinancialBars(prices...), Costs{StartEquity: 1000, FeePerUnit: .1, FillOn: fill})
	p.observed.paperAccounting = nil
	x, err := attachOrdinaryBacktestStateCapture(&p.observed)
	if err != nil {
		t.Fatal(err)
	}
	p.capture = x.financial
	return p, x
}

func ordinaryStateFind(t *testing.T, states []ordinaryBacktestState, boundary string, index int) ordinaryBacktestState {
	t.Helper()
	for _, s := range states {
		if s.Boundary == boundary && s.Index == index {
			return s
		}
	}
	t.Fatalf("missing %s at %d", boundary, index)
	return ordinaryBacktestState{}
}

func ordinaryStateFinish(t *testing.T, x *ordinaryBacktestStateCapture) (OrdinaryBacktestFinancials, []ordinaryBacktestState) {
	t.Helper()
	f, states, err := x.finish()
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Marks) != len(x.financial.bars) || len(states) < 2 || states[0].Boundary != "initial" || states[len(states)-1].Boundary != "terminal" {
		t.Fatal("missing complete witness")
	}
	return f, states
}

func ordinaryStateRefused(t *testing.T, x *ordinaryBacktestStateCapture) {
	t.Helper()
	f, s, err := x.finish()
	if err == nil || !reflect.DeepEqual(f, OrdinaryBacktestFinancials{}) || s != nil {
		t.Fatalf("expected all-or-nothing refusal: %+v %v %v", f, s, err)
	}
}

func TestOrdinaryStateMarketQueueAndFinalization(t *testing.T) {
	for _, fill := range []string{"close", "open", "nextOpen"} {
		for _, direction := range []side{sideLong, sideShort} {
			t.Run(fill+"/"+direction.String(), func(t *testing.T) {
				p, x := ordinaryStatePair(t, []float64{100, 105, 110}, fill)
				p.step(func(b *broker) {
					b.dispatchCaptured(0, order{Side: direction, Size: .7, HasSize: true, NoStop: true, NoTarget: true})
				})
				p.step(func(b *broker) { b.run() })
				f, states := ordinaryStateFinish(t, x)
				entryIndex := 0
				if fill != "close" {
					entryIndex = 1
					beforeQueue := ordinaryStateFind(t, states, "before-pending-entries", 1)
					if len(beforeQueue.PendingOrders) != 1 || beforeQueue.HasPosition {
						t.Fatal("actual pending order not witnessed before removal")
					}
				}
				before := ordinaryStateFind(t, states, "before-entry", entryIndex)
				after := ordinaryStateFind(t, states, "after-entry", entryIndex)
				if before.HasPosition || len(before.PendingOrders) != 0 || !after.HasPosition || !paperSame(after.Position.Size, .7) || !paperSame(after.Realized, f.Events[0].RealizedAfter) {
					t.Fatalf("bad actual entry witness: %+v %+v", before, after)
				}
				lastBefore := ordinaryStateFind(t, states, "before-exit", 2)
				lastAfter := ordinaryStateFind(t, states, "after-exit", 2)
				if !lastBefore.HasPosition || lastAfter.HasPosition || lastAfter.TradeCount != 1 || len(f.Events) != 2 || !paperSame(lastAfter.Realized, f.TerminalRealized) {
					t.Fatal("bad ordinary liquidation witness")
				}
			})
		}
	}
}

func TestOrdinaryStateLimitAndDeferredExitQueues(t *testing.T) {
	p, x := ordinaryStatePair(t, []float64{100, 100, 101, 102}, "close")
	p.step(func(b *broker) {
		b.limitOrders = append(b.limitOrders, order{Side: sideLong, Size: 2, HasSize: true, Limit: 100, HasLimit: true, PlacedAt: 0, ExpireAt: 3, NoStop: true, NoTarget: true})
		b.pendingExits = append(b.pendingExits, pendingExit{PositionEntryIndex: 1, Index: 2, Rule: "synthetic-rule"})
	})
	p.step(func(b *broker) { b.run() })
	f, states := ordinaryStateFinish(t, x)
	if len(ordinaryStateFind(t, states, "before-limit-orders", 1).LimitOrders) != 1 || len(ordinaryStateFind(t, states, "before-entry", 1).LimitOrders) != 0 {
		t.Fatal("limit order queue was reconstructed or dropped")
	}
	queue := ordinaryStateFind(t, states, "before-pending-exits", 2)
	before := ordinaryStateFind(t, states, "before-exit", 2)
	after := ordinaryStateFind(t, states, "after-exit", 2)
	if len(queue.PendingExits) != 1 || len(before.PendingExits) != 0 || len(after.PendingExits) != 0 || !before.HasPosition || after.HasPosition || f.Events[1].Index != 2 {
		t.Fatal("deferred queue and financial boundaries conflated")
	}
}

func TestOrdinaryStatePartialActualRemainder(t *testing.T) {
	for _, fraction := range []float64{.5, .7, 1} {
		for _, direction := range []side{sideLong, sideShort} {
			t.Run(direction.String()+"/"+fmt.Sprint(fraction), func(t *testing.T) {
				prices, stop := []float64{100, 105, 110}, 90.0
				if direction == sideShort {
					prices, stop = []float64{100, 95, 90}, 110
				}
				p, x := ordinaryStatePair(t, prices, "close")
				p.step(func(b *broker) {
					b.params.Partial.Enabled = true
					b.params.Partial.Fraction = fraction
					b.params.Partial.TriggerR = .25
					b.params.Partial.MoveBreakeven = true
					b.openPosition(direction, 100, order{Size: 10, HasSize: true, SL: stop, NoTarget: true}, 0)
				})
				p.step(func(b *broker) { b.run() })
				f, states := ordinaryStateFinish(t, x)
				if fraction == 1 {
					if len(f.Events) != 2 || f.Events[1].Kind != "exit" || ordinaryStateFind(t, states, "after-exit", 1).HasPosition {
						t.Fatal("fraction-one did not delegate to full exit")
					}
					return
				}
				before := ordinaryStateFind(t, states, "before-partial", 1)
				after := ordinaryStateFind(t, states, "after-partial", 1)
				if before.Position.Size != 10 || before.Position.PartialTaken || !after.Position.PartialTaken || after.Position.SL != 100 || len(f.Events) != 3 || !paperSame(after.Position.Size, f.Events[2].Size) {
					t.Fatal("partial state or actual final remainder linkage changed")
				}
				if fraction == .7 {
					want := uint64(0x4008000000000000)
					if runtime.GOARCH == "arm64" {
						want++
					}
					if math.Float64bits(after.Position.Size) != want {
						t.Fatalf("target remainder changed: %016x want %016x", math.Float64bits(after.Position.Size), want)
					}
				}
			})
		}
	}
}

func TestOrdinaryStateMetadataDeepCopies(t *testing.T) {
	p, x := ordinaryStatePair(t, []float64{100, 101}, "close")
	meta := TradeMeta{"nested": map[string]any{"values": []float64{math.Copysign(0, -1), 2}}, "nil": []string(nil), "empty": []string{}}
	p.step(func(b *broker) {
		b.openPosition(sideLong, 100, order{Size: 1, HasSize: true, NoStop: true, NoTarget: true, Meta: meta}, 0)
	})
	meta["nested"].(map[string]any)["values"].([]float64)[1] = 7
	entry := ordinaryStateFind(t, x.states, "after-entry", 0)
	v := entry.Position.Meta["nested"].(map[string]any)["values"].([]float64)
	if v[1] != 2 || math.Float64bits(v[0]) != 1<<63 || entry.Position.Meta["nil"].([]string) != nil || entry.Position.Meta["empty"].([]string) == nil {
		t.Fatal("entry metadata was aliased or normalized")
	}
	p.step(func(b *broker) { b.run() })
	_, states := ordinaryStateFinish(t, x)
	returned := ordinaryStateFind(t, states, "after-entry", 0)
	returned.Position.Meta["nested"].(map[string]any)["values"].([]float64)[1] = 99
	if ordinaryStateFind(t, x.states, "after-entry", 0).Position.Meta["nested"].(map[string]any)["values"].([]float64)[1] != 2 {
		t.Fatal("returned metadata aliases retained evidence")
	}
}

func TestOrdinaryStateWarmupNoTradeAndPendingTerminal(t *testing.T) {
	for _, pending := range []bool{false, true} {
		p, x := ordinaryStatePair(t, []float64{100, 101, 102}, "nextOpen")
		if pending {
			p.step(func(b *broker) {
				b.pendingOrders = []order{{Index: 3, Side: sideLong, Size: 1, HasSize: true, NoStop: true, NoTarget: true}}
			})
		}
		p.step(func(b *broker) { b.run() })
		f, states := ordinaryStateFinish(t, x)
		if len(f.Events) != 0 || len(f.Marks) != 3 || f.TerminalRealized != 0 {
			t.Fatal("warmup or pending orders invented trades")
		}
		terminal := states[len(states)-1]
		if pending != (len(terminal.PendingOrders) == 1) {
			t.Fatal("terminal pending queue lost")
		}
	}
}

func TestOrdinaryStateRefusals(t *testing.T) {
	for _, name := range []string{"missing-before", "missing-stage", "reordered-stage", "detached", "reset", "changed-bars", "repeated-finish", "nonfinite", "function", "cycle", "budget", "missing-loop"} {
		t.Run(name, func(t *testing.T) {
			p, x := ordinaryStatePair(t, []float64{100, 105}, "close")
			if name == "nonfinite" || name == "function" || name == "cycle" {
				m := TradeMeta{"v": math.Inf(1)}
				if name == "function" {
					m["v"] = func() {}
				}
				if name == "cycle" {
					m["v"] = m
				}
				p.observed.position.Meta = m
				x.observe(&p.observed, "before-on-bar", 0)
				ordinaryStateRefused(t, x)
				return
			}
			if name == "budget" {
				x.nodes = ordinaryStateMaxNodes
				x.observe(&p.observed, "before-on-bar", 0)
				ordinaryStateRefused(t, x)
				return
			}
			p.step(func(b *broker) {
				b.openPosition(sideLong, 100, order{Size: 1, HasSize: true, NoStop: true, NoTarget: true}, 0)
			})
			if name == "missing-loop" {
				// Financial marks alone cannot substitute for full loop coverage.
				p.observed.paperAccounting.mark(&p.observed, 0)
				p.observed.closePosition(105, 1, ReasonEndOfTest, "")
				p.observed.paperAccounting.mark(&p.observed, 1)
				ordinaryStateRefused(t, x)
				return
			}
			p.step(func(b *broker) { b.run() })
			switch name {
			case "missing-before":
				x.states = append(x.states[:1], x.states[2:]...)
			case "missing-stage":
				for i, s := range x.states {
					if s.Boundary == "before-limit-orders" {
						x.states = append(x.states[:i], x.states[i+1:]...)
						break
					}
				}
			case "reordered-stage":
				x.states[3], x.states[4] = x.states[4], x.states[3]
			case "detached":
				p.observed.paperAccounting.ordinaryState = nil
			case "reset":
				b := &p.observed
				b.reset(b.series, b.cols, nil, nil, nil, b.params, b.fixture, nil)
			case "changed-bars":
				p.observed.series.T = nil
			case "repeated-finish":
				ordinaryStateFinish(t, x)
			}
			ordinaryStateRefused(t, x)
		})
	}
}

func TestOrdinaryStateRejectsFiniteWitnessCorruption(t *testing.T) {
	cases := []struct {
		name, boundary string
		index          int
		change         func(*ordinaryBacktestState)
	}{
		{"partial-remainder", "after-partial", 1, func(s *ordinaryBacktestState) { s.Position.Size = 2 }},
		{"close-flat", "original-close", 1, func(s *ordinaryBacktestState) { s.HasPosition = false }},
		{"close-size", "original-close", 1, func(s *ordinaryBacktestState) { s.Position.Size = 2 }},
		{"close-entry", "original-close", 1, func(s *ordinaryBacktestState) { s.Position.Entry = 101 }},
		{"close-meta", "original-close", 1, func(s *ordinaryBacktestState) { s.Position.Meta["key"] = "changed" }},
		{"close-queue", "original-close", 1, func(s *ordinaryBacktestState) { s.PendingOrders = []order{{Index: 9}} }},
		{"stage-queue", "after-pending-exits", 1, func(s *ordinaryBacktestState) { s.LimitOrders = []order{{Index: 9}} }},
		{"stage-realized", "before-pending-entries", 1, func(s *ordinaryBacktestState) { s.Realized++ }},
		{"stage-trades", "before-pending-entries", 1, func(s *ordinaryBacktestState) { s.TradeCount++ }},
		{"exit-position", "after-exit", 2, func(s *ordinaryBacktestState) { s.Position.Meta["key"] = "changed" }},
		{"exit-queue", "after-exit", 2, func(s *ordinaryBacktestState) { s.PendingOrders = []order{{Index: 9}} }},
		{"entry-time", "after-entry", 0, func(s *ordinaryBacktestState) { s.Position.EntryT++ }},
		{"entry-stop", "after-entry", 0, func(s *ordinaryBacktestState) { s.Position.SL++ }},
	}
	for _, tc := range cases {
		for _, reseal := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/resealed=%t", tc.name, reseal), func(t *testing.T) {
				p, x := ordinaryStatePair(t, []float64{100, 105, 110}, "close")
				p.step(func(b *broker) {
					b.params.Partial.Enabled = true
					b.params.Partial.Fraction = .7
					b.params.Partial.TriggerR = .25
					b.openPosition(sideLong, 100, order{Size: 10, HasSize: true, SL: 90, NoTarget: true, Meta: TradeMeta{"key": "original"}}, 0)
				})
				p.step(func(b *broker) { b.run() })
				found := false
				for i := range x.states {
					if x.states[i].Boundary == tc.boundary && x.states[i].Index == tc.index {
						tc.change(&x.states[i])
						// Re-seal this deliberately false test state to exercise the
						// independent semantic checks rather than only integrity checks.
						if reseal {
							var err error
							x.seals[i], err = ordinaryStateSeal(x.states[i])
							if err != nil {
								t.Fatal(err)
							}
						}
						found = true
						break
					}
				}
				if !found {
					t.Fatal("fixture did not produce target boundary")
				}
				ordinaryStateRefused(t, x)
			})
		}
	}
}

func TestOrdinaryStateEqualityRetainsSignedZero(t *testing.T) {
	a := ordinaryBacktestState{Position: position{Meta: TradeMeta{"v": float64(0)}}}
	b := ordinaryBacktestState{Position: position{Meta: TradeMeta{"v": math.Copysign(0, -1)}}}
	if ordinaryStateSameContents(a, b) {
		t.Fatal("signed zero collapsed")
	}
}

func TestOrdinaryStateSealsPreserveTypesAndContainers(t *testing.T) {
	base := ordinaryBacktestState{Position: position{Meta: TradeMeta{"a": float64(0), "b": []string(nil)}}}
	want, err := ordinaryStateSeal(base)
	if err != nil {
		t.Fatal(err)
	}
	reordered := ordinaryBacktestState{Position: position{Meta: TradeMeta{"b": []string(nil), "a": float64(0)}}}
	got, err := ordinaryStateSeal(reordered)
	if err != nil || got != want {
		t.Fatal("map iteration order changed seal")
	}
	for _, v := range []any{math.Copysign(0, -1), int(0), float32(0), "0"} {
		s := ordinaryBacktestState{Position: position{Meta: TradeMeta{"a": v, "b": []string(nil)}}}
		got, err := ordinaryStateSeal(s)
		if err != nil || got == want {
			t.Fatal("seal collapsed exact metadata type/bits")
		}
	}
	base.Position.Meta["b"] = []string{}
	got, err = ordinaryStateSeal(base)
	if err != nil || got == want {
		t.Fatal("seal collapsed nil/empty")
	}
	base.Position.Meta["cycle"] = base.Position.Meta
	if _, err = ordinaryStateSeal(base); err == nil {
		t.Fatal("cyclic metadata seal accepted")
	}
}

func TestOrdinaryStateSealRejectsRetainedMutation(t *testing.T) {
	p, x := ordinaryStatePair(t, []float64{100, 101}, "close")
	p.step(func(b *broker) { b.run() })
	x.states[1].LimitOrders = []order{{Limit: 99, HasLimit: true, Meta: TradeMeta{"invented": "expired"}}}
	ordinaryStateRefused(t, x)
}
