package frozencontrol

import (
	"fmt"
	"math"
	"math/big"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine/frozeninput"
	"github.com/spik3r/heisentick-strat/engine/frozenlevels"
)

type request struct {
	Sources                   [3]string
	Documents                 frozeninput.Documents
	Artifacts                 []frozeninput.Artifact
	Calendar, Schedule, Costs []byte
	Assumptions               [3][]byte
	RiskBudgetUSD             string
	Side                      frozenlevels.Direction
}
type identity struct {
	Mode         string `json:"mode"`
	ConfigDigest string `json:"configDigest"`
	PolicyDigest string `json:"policyDigest"`
}
type managerTrace struct {
	Mode     string                    `json:"mode"`
	Events   []frozenlevels.LockEvent  `json:"events"`
	Counters frozenlevels.LockCounters `json:"counters"`
	Trade    trade                     `json:"trade"`
}
type trace struct {
	Version       string                    `json:"version"`
	Scope         string                    `json:"scope"`
	Status        string                    `json:"status"`
	Reason        string                    `json:"reason,omitempty"`
	InputEvidence frozeninput.Evidence      `json:"inputEvidence"`
	Identities    []identity                `json:"identities"`
	Source        frozenlevels.Source       `json:"source"`
	OrderingProof string                    `json:"orderingProof"`
	CostsSHA256   string                    `json:"costsSHA256"`
	Context       contexts                  `json:"context"`
	Lifecycle     []frozenlevels.LevelEvent `json:"lifecycle"`
	Managers      []managerTrace            `json:"managers"`
}

func inputRef(cfg dsl.Config, key string) dsl.FrozenQuoteRef {
	m := cfg["frozenLevelBreakout"].(map[string]any)["inputRefs"].(map[string]any)[key].(map[string]any)
	return dsl.FrozenQuoteRef{ID: m["id"].(string), Version: m["version"].(string), SHA256: m["sha256"].(string)}
}
func budget(r request) error {
	remaining := frozeninput.MaxInputBytes
	take := func(n int) bool {
		if n > remaining {
			return false
		}
		remaining -= n
		return true
	}
	for _, s := range r.Sources {
		if !take(len(s)) {
			return fmt.Errorf("synthetic-pilot-size-limit")
		}
	}
	for _, b := range [][]byte{r.Documents.Snapshot, r.Documents.Source, r.Documents.Ordering, r.Calendar, r.Schedule, r.Costs, r.Assumptions[0], r.Assumptions[1], r.Assumptions[2]} {
		if !take(len(b)) {
			return fmt.Errorf("synthetic-pilot-size-limit")
		}
	}
	for _, a := range r.Artifacts {
		if !take(len(a.Bytes)) {
			return fmt.Errorf("synthetic-pilot-size-limit")
		}
	}
	if !take(len(r.RiskBudgetUSD)) {
		return fmt.Errorf("synthetic-pilot-size-limit")
	}
	return nil
}

// replay is intentionally unexported and unregistered. The sole qualification
// implementation lives in _test.go and proves the finite invented source path.
// Runtime entrypoints cannot call this by supplying a label, digest or boolean.
func replay(r request, w fixtureAuthority) (*trace, error) {
	if w == nil {
		return nil, fmt.Errorf("missing-test-only-synthetic-authority")
	}
	if r.Side != frozenlevels.Long && r.Side != frozenlevels.Short {
		return nil, fmt.Errorf("invalid-side")
	}
	if e := budget(r); e != nil {
		return nil, e
	}
	if r.RiskBudgetUSD != "1" {
		return nil, fmt.Errorf("pilot-requires-explicit-USD1-risk-budget")
	}
	riskBudget := big.NewRat(1, 1)
	var e error
	out := &trace{Version: "frozen-synthetic-trace-v1", Scope: "test-only-finite-synthetic-witness", Identities: []identity{}, Lifecycle: []frozenlevels.LevelEvent{}, Managers: []managerTrace{}}
	var input *frozeninput.Input
	var cfg dsl.Config
	commonPolicy := ""
	for i, mode := range []string{"none", "scale", "pivot"} {
		parsed, e := dsl.Parse(r.Sources[i])
		if e != nil || len(parsed.Errors) != 0 {
			return nil, fmt.Errorf("invalid-synthetic-DSL: %v %v", e, parsed.Errors)
		}
		in, e := frozeninput.Decode(parsed.Config, r.Documents, r.Artifacts)
		if e != nil {
			return nil, e
		}
		copy, e := in.Config()
		if e != nil {
			return nil, e
		}
		spec := copy["frozenLevelBreakout"].(map[string]any)
		if spec["lockMode"] != mode {
			return nil, fmt.Errorf("manager-controls-must-be-none-scale-pivot")
		}
		for _, leaf := range []struct {
			name string
			raw  []byte
		}{{"calendar", r.Calendar}, {"schedule", r.Schedule}, {"costs", r.Costs}, {"assumptions", r.Assumptions[i]}} {
			if hash(leaf.raw) != inputRef(copy, leaf.name).SHA256 {
				return nil, fmt.Errorf("control-payload-hash-mismatch: %s", leaf.name)
			}
		}
		id := in.Identity()
		out.Identities = append(out.Identities, identity{mode, id.ConfigDigest, id.PolicyDigest})
		// Mode and the intentionally excluded assumptions/metadata are the only
		// differences permitted across the three common-opportunity controls.
		spec["lockMode"] = "none"
		same, e := dsl.FrozenLevelIdentity(copy)
		if e != nil {
			return nil, e
		}
		if i == 0 {
			commonPolicy = same.PolicyDigest
			input = in
			cfg, e = in.Config()
			if e != nil {
				return nil, e
			}
		} else if same.PolicyDigest != commonPolicy {
			return nil, fmt.Errorf("manager-common-policy-mismatch")
		}
	}
	if e = w.verify(input, r); e != nil {
		return nil, e
	}
	costRaw := append([]byte(nil), r.Costs...)
	cost, e := dsl.DecodeFrozenTraceCostsJSON(costRaw)
	if e != nil {
		return nil, e
	}
	if cost.SourceRef != inputRef(cfg, "source") || inputRef(cfg, "costs") != (dsl.FrozenQuoteRef{ID: cost.ID, Version: cost.Version, SHA256: hash(costRaw)}) {
		return nil, fmt.Errorf("cost-identity-mismatch")
	}
	fee, e := rational(cost.CommissionPerOzPerFillUSD)
	if e != nil {
		return nil, e
	}
	spec := cfg["frozenLevelBreakout"].(map[string]any)
	grid := spec["priceGrid"].(float64)
	activation := spec["activationR"].(float64)
	entryLatency := spec["entryLatencyMS"].(int64)
	amendmentLatency := spec["amendmentLatencyMS"].(int64)
	windows := input.SourceDeclaration().Windows
	start, end := windows.Evaluation.StartMS, windows.Evaluation.EndMS
	if end-start >= spec["timeoutMinutes"].(int64)*60000 {
		return nil, fmt.Errorf("pilot-evaluation-reaches-unimplemented-timeout-model")
	}
	creation := start / (6 * frozenlevels.M5Millis) * (6 * frozenlevels.M5Millis)
	stored, e := window(input, w, frozenlevels.ClockM30, creation)
	if e != nil {
		return nil, e
	}
	key, e := w.contextKey(creation-frozenlevels.M5Millis, creation)
	if e != nil {
		return nil, e
	}
	source, _ := input.Source()
	price := stored.High
	if r.Side == frozenlevels.Short {
		price = stored.Low
	}
	level := frozenlevels.LevelSpec{Version: frozenlevels.LifecycleVersion, ProviderID: "synthetic-common-control-v1", SourceWindowID: stored.ID, ContextSource: source, QuoteSource: source, OrderingProof: input.OrderingProof(), KnownAt: key, CreatedAt: key, ExpiresAtMillis: creation + 6*frozenlevels.M5Millis, Side: r.Side, Price: price, Policy: frozenlevels.LevelPolicy{AssumptionID: "synthetic-common-opportunity-v1", MaxPredecessorAgeMillis: spec["maxPredecessorAgeMS"].(int64), MaxObservationAgeMillis: spec["maxReceiptAgeMS"].(int64), SubmissionLatencyMillis: entryLatency, UsePolicy: "one-success-no-retry"}}
	life, created, e := frozenlevels.NewLevelLifecycle(level, nil)
	if e != nil {
		return nil, e
	}
	out.Lifecycle = append(out.Lifecycle, created)
	out.InputEvidence = input.Evidence()
	out.Source = source
	out.OrderingProof = input.OrderingProof()
	out.CostsSHA256 = hash(costRaw)
	var ctx contexts
	var ctxErr error
	entryIndex := -1
	var entry fill
	var quantity *big.Rat
	var previous frozenlevels.EventKey
	for i := 0; i < input.Len(); i++ {
		obs, _ := input.Observation(i)
		q := obs.Quote
		if q.Event.AtMillis < start {
			continue
		}
		if q.Event.AtMillis >= end {
			break
		}
		if previous == (frozenlevels.EventKey{}) {
			previous = level.CreatedAt
		}
		if e = w.transition(previous, q.Event); e != nil {
			return nil, fmt.Errorf("unqualified-preentry-transition: %w", e)
		}
		previous = q.Event
		events, e := life.Observe(frozenlevels.LevelQuote{Quote: q, Source: source}, true, false)
		if e != nil {
			return nil, e
		}
		out.Lifecycle = append(out.Lifecycle, events...)
		for _, event := range events {
			if event.Kind == "crossed" {
				ctx, ctxErr = signalContexts(input, w, stored, level, q, r.Side, grid)
				if ctxErr == nil {
					ctxErr = checkContextBindings(input, ctx)
				}
			}
			if event.Kind != "admission-ready" {
				continue
			}
			ctx.EntryCandidateKey = q.Event
			ctx.RiskBudgetUSD = amount(riskBudget)
			candidate := entryFill(q, r.Side)
			if ctxErr == nil {
				quantity, e = quantityFor(candidate.Price, ctx.InitialStop, r.Side, riskBudget)
				if e != nil {
					ctxErr = e
				} else {
					ctx.QuantityOz = amount(quantity)
				}
			}
			if ctxErr == nil {
				target := minus(times(exactPrice(candidate.Price), big.NewRat(3, 1)), times(exactPrice(ctx.InitialStop), big.NewRat(2, 1)))
				ctx.Target, _ = target.Float64()
				if ctx.Target <= 0 || math.IsInf(ctx.Target, 0) || math.IsNaN(ctx.Target) || exactPrice(ctx.Target).Cmp(target) != 0 {
					ctxErr = fmt.Errorf("unrepresentable-exact-target")
				}
			}
			reason := ""
			if ctxErr != nil {
				reason = ctxErr.Error()
			} else if ctx.Opening == nil {
				reason = ctx.OpeningError
			} else if ctx.Rolling.FrozenAtMillis >= q.Event.AtMillis || ctx.Opening.FrozenAtMillis >= q.Event.AtMillis || ctx.Ladder.FrozenAtMillis >= q.Event.AtMillis {
				reason = "context-not-strictly-before-fill"
			}
			out.Context = ctx
			if reason != "" {
				resolved, e := life.ResolveAdmission(q.Event, "rejected")
				if e != nil {
					return nil, e
				}
				out.Lifecycle = append(out.Lifecycle, resolved)
				out.Status = "common-entry-rejected"
				out.Reason = reason
				for _, mode := range []string{"none", "scale", "pivot"} {
					out.Managers = append(out.Managers, managerTrace{Mode: mode, Events: []frozenlevels.LockEvent{}, Trade: trade{Status: "not-entered", EntryFeeUSD: "0", ExitFeeUSD: "0", CashDeltaUSD: "0", Ledger: []ledgerEntry{}}})
				}
				return out, nil
			}
			entry, entryIndex = candidate, i
		}
		if entryIndex >= 0 {
			break
		}
		if life.Terminal() {
			return nil, fmt.Errorf("synthetic-opportunity-not-established")
		}
	}
	if entryIndex < 0 {
		return nil, fmt.Errorf("synthetic-opportunity-not-established")
	}
	// Prepare all managers before committing the shared execution. An invalid
	// seed cannot create a partially filled no-lock/scale/pivot comparison.
	managers := make([]*frozenlevels.LockTracker, 3)
	ledgers := make([]*positionLedger, 3)
	seedQuote, _ := input.Observation(entryIndex)
	for i, mode := range []string{"none", "scale", "pivot"} {
		kind := frozenlevels.NoLock
		var opening *frozenlevels.FrozenOpening
		var levels *frozenlevels.LockLevels
		if mode == "scale" {
			kind, opening = frozenlevels.ScaleLock, ctx.Opening
		}
		if mode == "pivot" {
			kind, levels = frozenlevels.PivotLock, &ctx.LockLevels
		}
		managers[i], e = frozenlevels.NewLockTracker(frozenlevels.PositionSeed{Side: r.Side, Entry: entry.Price, InitialStop: ctx.InitialStop, EntryQuote: seedQuote.Quote}, frozenlevels.LockPolicy{Mode: kind, ActivationR: activation, Grid: grid, LatencyMillis: amendmentLatency, AssumptionID: "synthetic-manager-control-v1"}, opening, levels)
		if e != nil {
			return nil, e
		}
		if managers[i].CommonTarget() != ctx.Target {
			return nil, fmt.Errorf("common-target-mismatch")
		}
	}
	resolved, e := life.ResolveAdmission(entry.At, "consumed")
	if e != nil {
		return nil, e
	}
	out.Lifecycle = append(out.Lifecycle, resolved)
	for i, mode := range []string{"none", "scale", "pivot"} {
		ledgers[i] = newLedger(entry, quantity, riskBudget, fee, r.Side)
		out.Managers = append(out.Managers, managerTrace{Mode: mode, Events: []frozenlevels.LockEvent{}})
	}
	previous = entry.At
	for idx := entryIndex + 1; idx < input.Len(); idx++ {
		obs, _ := input.Observation(idx)
		q := obs.Quote
		if q.Event.AtMillis >= end {
			break
		}
		continuous := w.transition(previous, q.Event) == nil
		previous = q.Event
		for i, manager := range managers {
			if manager.Terminal() {
				continue
			}
			events, e := manager.Observe(q, continuous)
			if e != nil {
				return nil, e
			}
			out.Managers[i].Events = append(out.Managers[i].Events, events...)
			for _, event := range events {
				switch event.Kind {
				case "unresolved":
					ledgers[i].unresolved()
				case "exit-due":
					exit, e := exitFill(q, r.Side, event.Reason, manager.CommonTarget())
					if e != nil {
						return nil, e
					}
					ledgers[i].close(exit)
				}
			}
		}
	}
	out.Status = "closed"
	for i, manager := range managers {
		if !manager.Terminal() {
			ledgers[i].unresolved()
		}
		out.Managers[i].Counters = manager.Counters()
		out.Managers[i].Trade = ledgers[i].result
		if ledgers[i].result.Status == "unresolved" {
			out.Status = "unresolved"
		}
	}
	return out, nil
}
