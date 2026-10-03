package engine

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/spik3r/heisentick-strat/dsl"
)

const CompositionSchema = "heisentick-composition-v1"

//go:embed composition_manifest_v1.json
var builtInCompositionJSON []byte

// CompositionManifest pins the authored wrapper routes and their DSL children.
// Sources stay in the app, so a caller must provide their exact bytes. Child
// params are explicit even when empty; there is no inherited parent param map.
type CompositionManifest struct {
	Schema         string             `json:"schema"`
	SourceRevision string             `json:"sourceRevision"`
	Children       []CompositionChild `json:"children"`
	Composites     []Composite        `json:"composites"`
}

type CompositionChild struct {
	ID     string             `json:"id"`
	Path   string             `json:"path"`
	SHA256 string             `json:"sha256"`
	Params map[string]float64 `json:"params"`
}

type Composite struct {
	ID     string             `json:"id"`
	Routes []CompositionRoute `json:"routes"`
}

type CompositionRoute struct {
	Symbol    string   `json:"symbol"`
	Timeframe string   `json:"timeframe"`
	Children  []string `json:"children"`
}

// BuiltInCompositionManifest returns the versioned Go-owned manifest for the
// five authored JS wrappers. It returns a fresh decode so callers cannot mutate
// the embedded definition.
func BuiltInCompositionManifest() (CompositionManifest, error) {
	var manifest CompositionManifest
	err := json.Unmarshal(builtInCompositionJSON, &manifest)
	return manifest, err
}

// RunBuiltInComposition is the native entry point. Sources is keyed by child
// ID and contains raw .strat text from the pinned app source tree. The wrapper
// strategy ID belongs in request.StrategyID; child params are resolved from the
// manifest and never inherited from a caller's parent strategy.
func RunBuiltInComposition(request RunRequest, sources map[string]string) (RunResult, error) {
	manifest, err := BuiltInCompositionManifest()
	if err != nil {
		return RunResult{}, err
	}
	return RunComposition(manifest, request, sources)
}

// RunComposition validates every declared dependency before selecting a route.
// An undeclared route is a genuine empty result; a missing, stale, or invalid
// child is an error, including when it is only needed on another route.
func RunComposition(manifest CompositionManifest, request RunRequest, sources map[string]string) (RunResult, error) {
	if manifest.Schema != CompositionSchema {
		return RunResult{}, fmt.Errorf("unsupported composition schema %q", manifest.Schema)
	}
	var selected *Composite
	for i := range manifest.Composites {
		if manifest.Composites[i].ID == request.StrategyID {
			if selected != nil {
				return RunResult{}, fmt.Errorf("duplicate composite %q", request.StrategyID)
			}
			selected = &manifest.Composites[i]
		}
	}
	if selected == nil {
		return RunResult{}, fmt.Errorf("unknown composite %q", request.StrategyID)
	}
	children, err := compileCompositionChildren(manifest.Children, selected.Routes, sources)
	if err != nil {
		return RunResult{}, fmt.Errorf("composite %s: %w", request.StrategyID, err)
	}
	var route *CompositionRoute
	for i := range selected.Routes {
		candidate := &selected.Routes[i]
		if candidate.Symbol == request.Symbol && candidate.Timeframe == request.Timeframe {
			if route != nil {
				return RunResult{}, fmt.Errorf("duplicate route %s %s", request.Symbol, request.Timeframe)
			}
			route = candidate
		}
	}
	if route == nil {
		// Keep the standard envelope, including normalized costs and an empty
		// trades array, only after every child dependency has been validated.
		for _, candidate := range selected.Routes {
			request.Config = children[candidate.Children[0]].config
			break
		}
		if err := validateRunRequest(request); err != nil {
			return RunResult{}, err
		}
		if _, err := ResolveExecutionWindow(request.Series, request.ExecutionWindow); err != nil {
			return RunResult{}, err
		}
		return checkedResultEnvelope(fixtureFromRequest(request), []Trade{})
	}
	prepared := make([]*PreparedRun, 0, len(route.Children))
	for _, id := range route.Children {
		child, ok := children[id]
		if !ok {
			return RunResult{}, fmt.Errorf("route %s %s references undeclared child %q", route.Symbol, route.Timeframe, id)
		}
		childRequest := request
		childRequest.Config = child.config
		p, err := PrepareRun(childRequest)
		if err != nil {
			return RunResult{}, fmt.Errorf("child %s: %w", id, err)
		}
		if p.offRoute {
			return RunResult{}, fmt.Errorf("child %s rejects declared composite route %s %s", id, route.Symbol, route.Timeframe)
		}
		emaLen, emaSlopeLen := p.params.TPBEMALen, p.params.TPBEMASlopeLen
		if err := applyCompositionParams(&p.params, child.params); err != nil {
			return RunResult{}, fmt.Errorf("child %s runtime params: %w", id, err)
		}
		if p.params.SetupType == string(dsl.FamilyTrendPullback) && (p.params.TPBEMALen != emaLen || p.params.TPBEMASlopeLen != emaSlopeLen) {
			return RunResult{}, fmt.Errorf("child %s runtime EMA params disagree with pinned DSL source", id)
		}
		prepared = append(prepared, p)
	}
	if len(prepared) == 1 {
		return prepared[0].RunChecked(request.Costs)
	}
	if len(prepared) == 2 {
		return runOrderedSessionBreakHold(prepared, request.Costs)
	}
	return RunResult{}, fmt.Errorf("route %s %s has unsupported child count %d", route.Symbol, route.Timeframe, len(prepared))
}

type compiledCompositionChild struct {
	config dsl.Config
	params map[string]float64
}

func compileCompositionChildren(definitions []CompositionChild, routes []CompositionRoute, sources map[string]string) (map[string]compiledCompositionChild, error) {
	defs := make(map[string]CompositionChild, len(definitions))
	for _, child := range definitions {
		if child.ID == "" || child.Path == "" || len(child.SHA256) != 64 || child.Params == nil {
			return nil, fmt.Errorf("incomplete child declaration %q", child.ID)
		}
		if _, exists := defs[child.ID]; exists {
			return nil, fmt.Errorf("duplicate child %q", child.ID)
		}
		defs[child.ID] = child
	}
	compiled := make(map[string]compiledCompositionChild)
	for _, route := range routes {
		if route.Symbol == "" || route.Timeframe == "" || len(route.Children) == 0 {
			return nil, errors.New("incomplete composition route")
		}
		for _, id := range route.Children {
			if _, done := compiled[id]; done {
				continue
			}
			child, ok := defs[id]
			if !ok {
				return nil, fmt.Errorf("undeclared child %q", id)
			}
			source, ok := sources[id]
			if !ok || strings.TrimSpace(source) == "" {
				return nil, fmt.Errorf("missing source for child %q (%s)", id, child.Path)
			}
			digest := sha256.Sum256([]byte(source))
			if hex.EncodeToString(digest[:]) != child.SHA256 {
				return nil, fmt.Errorf("source digest mismatch for child %q (%s)", id, child.Path)
			}
			parsed, err := dsl.Parse(source)
			if err != nil {
				return nil, fmt.Errorf("child %s: %w", id, err)
			}
			if len(parsed.Errors) != 0 {
				return nil, fmt.Errorf("child %s DSL parse errors: %v", id, parsed.Errors)
			}
			compiled[id] = compiledCompositionChild{config: parsed.Config, params: child.Params}
		}
	}
	return compiled, nil
}

// The JS wrapper replaces api.params with each child's generated numeric
// params. Apply that frozen snapshot after the Go DSL parse. The source digest
// still owns structural gates/context; unsupported numeric knobs fail closed.
func applyCompositionParams(p *flagParams, values map[string]float64) error {
	boolean := func(v float64) bool { return v != 0 }
	if buffer, hasBuffer := values["stopBufferAtr"]; hasBuffer {
		if padding, hasPadding := values["stopPaddingAtr"]; hasPadding && buffer != padding {
			return errors.New("conflicting stopBufferAtr and stopPaddingAtr")
		}
	}
	for key, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%s must be finite", key)
		}
		switch key {
		case "breakevenR":
			p.BreakevenR = v
		case "breakevenOffsetAtr":
			p.BreakevenOffsetATR = v
		case "maxHoldBars":
			p.MaxHoldBars = v
		case "cooldownBars":
			p.CooldownBars = int(v)
		case "useHtfBias":
			p.UseHTFBias = boolean(v)
		case "useAsiaWindow":
			p.UseAsiaWindow = boolean(v)
		case "useMidWindow":
			p.UseMidWindow = boolean(v)
		case "useLondonWindow":
			p.UseLondonWindow = boolean(v)
		case "useNyWindow":
			p.UseNYWindow = boolean(v)
		case "allowLong":
			p.AllowLong = boolean(v)
		case "allowShort":
			p.AllowShort = boolean(v)
		case "riskUsd":
			p.RiskUSD = v
		case "partialEnabled":
			p.Partial.Enabled = boolean(v)
		case "partialFraction":
			p.Partial.Fraction = v
		case "partialTriggerR":
			p.Partial.TriggerR = v
		case "partialMoveBreakeven":
			p.Partial.MoveBreakeven = boolean(v)
		case "trailAtr":
			p.Trail.ATR = v
		case "trailTriggerR":
			p.Trail.TriggerR = v
		case "minStopAtr":
			p.MinStopATR, p.SBHMinStopATR, p.TPBMinStopATR = v, v, v
		case "maxStopAtr":
			p.MaxStopATR, p.SBHMaxStopATR, p.TPBMaxStopATR = v, v, v
		case "stopBufferAtr", "stopPaddingAtr":
			p.StopBufferATR, p.TPBStopBufferATR = v, v
		case "targetR":
			switch p.SetupType {
			case string(dsl.FamilyBreakRetest):
				p.BRTargetR = v
			case string(dsl.FamilySessionBreakHold):
				p.SBHTargetR = v
			case string(dsl.FamilyTrendPullback):
				p.TPBTargetR = v
			default:
				return fmt.Errorf("targetR unsupported for %s", p.SetupType)
			}
		case "minEr":
			p.BRMinER, p.TPBMinER = v, v
		case "maxEr":
			p.BRMaxER, p.TPBMaxER = v, v
		case "holdCandles":
			p.SBHHoldCandles = int(v)
		case "stopInsideAtr":
			p.SBHStopInsideATR = v
		case "freshBreakBars":
			p.BRFreshBreakBars = int(v)
		case "retestBars":
			p.BRRetestBars = int(v)
		case "levelTolerance":
			p.BRLevelTolerance = v
		case "minBreakAtr":
			p.BRMinBreakATR = v
		case "displacementAtr":
			p.BRDisplacementATR = v
		case "fibExtension":
			p.BRFibExtension = v
		case "pullbackAtr":
			p.TPBPullbackATR = v
		case "pullbackWithin":
			p.TPBPullbackWithin = int(v)
		case "maxPullbackDepth":
			p.TPBMaxPullbackDepth = v
		case "impulseLookbackBars":
			p.TPBImpulseLookbackBars = int(v)
		case "attemptCount":
			p.TPBAttemptCount = int(v)
		case "useTrigger":
			if p.SetupType == string(dsl.FamilySessionBreakHold) && v != 1 {
				return errors.New("session break hold requires trigger")
			}
			p.TPBUseTrigger = boolean(v)
		case "emaLen":
			if p.SetupType == string(dsl.FamilyBreakRetest) && v != 0 {
				return errors.New("break retest EMA context is unsupported")
			}
			p.TPBEMALen = int(v)
		case "emaSlopeLen":
			p.TPBEMASlopeLen = int(v)
		case "sessionVwapFirstTouchOnly":
			p.TPBVWAPFirst = boolean(v)
		case "sessionVwapTrendSideReclaim":
			p.TPBVWAPReclaim = boolean(v)
		case "sessionVwapTouchMode", "swingLookbackBars", "swingPivotK", "swingLevelCount", "emaConfluenceAtr":
			// These active snapshots use no VWAP touch or EMA confluence,
			// and key-level break retest does not use swing-only knobs.
			if (key == "sessionVwapTouchMode" || key == "emaConfluenceAtr") && v != 0 ||
				key == "swingLookbackBars" && v != 60 || key == "swingPivotK" && v != 2 || key == "swingLevelCount" && v != 4 {
				return fmt.Errorf("unsupported %s=%g", key, v)
			}
		default:
			return fmt.Errorf("unsupported param %q", key)
		}
	}
	return nil
}

type sessionBreakHoldBranchState struct {
	lastEntry int
	hasEntry  bool
	seen      dailySeenSet
}

// The balanced wrapper calls long then short on each bar and calls short only
// if the shared API has no position after long. Keep one broker for occupancy,
// pending orders, fills, exits and P&L, while swapping each child DSL runtime's
// independent session-break-hold seen/cooldown state and compiled context.
func runOrderedSessionBreakHold(children []*PreparedRun, costs Costs) (RunResult, error) {
	for _, child := range children {
		if child.c5 || child.params.SetupType != string(dsl.FamilySessionBreakHold) || child.series.Len() != children[0].series.Len() {
			return RunResult{}, errors.New("ordered composition supports only same-series session-break-hold children")
		}
	}
	first := children[0]
	fixture := first.fixture
	fixture.Costs = costs
	var b broker
	b.reset(first.series, first.cols, first.htfTrend, first.ema, first.emaSlope, first.params, fixture, nil)
	if first.windowed {
		b.setExecutionWindow(first.execution)
	}
	states := []sessionBreakHoldBranchState{{seen: dailySeenSet{day: -1}}, {seen: dailySeenSet{day: -1}}}
	end := b.executionEnd()
	if end >= b.series.Len() {
		end = b.series.Len() - 1
	}
	for i := 0; i <= end; i++ {
		if b.windowed && i < b.executionStart() {
			b.clearExecutionOrders()
		}
		b.fillPendingExits(i)
		b.fillPending(i)
		b.fillLimits(i)
		b.closeExpiredWindowPosition(i)
		b.resolveIntrabarExit(i)
		for branch, child := range children {
			if branch > 0 && b.hasPosition {
				break
			}
			b.params, b.cols, b.htfTrend = child.params, child.cols, child.htfTrend
			b.ema, b.emaSlope = child.ema, child.emaSlope
			b.sbhLastEntry, b.hasSBHEntry, b.seen.sbh = states[branch].lastEntry, states[branch].hasEntry, states[branch].seen
			b.onBar(i)
			states[branch] = sessionBreakHoldBranchState{b.sbhLastEntry, b.hasSBHEntry, b.seen.sbh}
		}
		b.markToMarket(i)
		if b.windowed && i < b.executionStart() {
			b.clearExecutionOrders()
		}
	}
	if end >= 0 && b.hasPosition {
		b.closePosition(b.series.C[end], end, ReasonEndOfTest, "")
	}
	return checkedResultEnvelope(fixture, b.trades)
}
