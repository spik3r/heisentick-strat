package engine_test

// Independent E1/E2 acceptance adapter. Expected values are read-only inputs.
// Canonical locations: engine/sequential_acceptance_test.go and
// engine/testdata/sequential_execution_e1_e2_v1/{SOURCES.json,MANIFEST.json,cases/**,...}.
// No regeneration mode, private scheduler access, new runtime API, or dependencies.
//
// Coverage is deliberately bounded:
//   - all 66 binding cases execute; OQ-2 remains explicitly provisional and
//     the OQ-4 raw-open guard is an excluded proposal, never an acceptance target;
//   - ATR nulls before index 13 express decision eligibility, not ComputeATR's
//     raw output, which contains early averages;
//   - not_placed/no_next_bar is a rejection audit record, not a broker order;
//   - rejected attempted entry_fill is unobservable in the exported audit and
//     is reported as an exclusion, never recreated from expected data or costs;
//   - anchor price/buffer, unconstrained size, notional, R and held bars are
//     arithmetic projections of actual audited/traded fields (see comparisons);
//   - refusal metadata follows binding flags, except the contract-bound terminal
//     kind and existing typed time-gap category; refused outputs must not leak;
//   - injected unreachable guards and the 143 core prefix/restart cases remain
//     covered by their existing independent tests, not by invented OHLC cases.

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const seqAcceptanceSchema = "seq-exec-fixtures.v1"
const seqAcceptanceConvention = "seq-exec-e1e2.v1"
const seqAcceptanceProfile = "seq.full.public_approx.v1"
const seqAcceptanceRelative = 1e-9

const seqAcceptanceSourceCommit = "3e4694ea088703ebf6a696287127b50cdbf8b6f7"
const seqAcceptanceCorpusHash = "40951963bd91f1c542ac598d3ee0b17aa5a758bf2ecc1a7f413900cb1eaf15ae"

var seqAcceptanceProvisional = map[string]string{
	"pv.target_gap_open_beyond": "OQ-2",
}
var seqAcceptanceExcluded = map[string]string{
	"pv.entry_straddle_open_below_stop": "OQ-4",
}

// SOURCES pins the independently authored repository revision and every imported
// byte in the nine claimed files (MANIFEST, README, DERIVATIONS, six case
// files). Author tools stay in the source repository. The importer verifies the commit
// -> path -> blob relationship against Git before committing SOURCES. Offline
// tests recompute both blob-object SHA-1 and SHA-256; a digest alone cannot prove
// that a file belonged to the declared remote commit.
type seqAcceptanceSources struct {
	Schema     string                      `json:"schema"`
	Repository string                      `json:"repository"`
	Commit     string                      `json:"commit"`
	Root       string                      `json:"root"`
	Files      map[string]seqAcceptancePin `json:"files"`
}
type seqAcceptancePin struct {
	SourcePath string `json:"source_path"`
	GitBlob    string `json:"git_blob"`
	SHA256     string `json:"sha256"`
}
type seqAcceptanceManifest struct {
	Revision     string            `json:"revision"`
	Convention   string            `json:"convention_id"`
	Schema       string            `json:"schema"`
	CorpusSHA256 string            `json:"corpus_sha256"`
	HashRule     string            `json:"hash_rule"`
	Files        map[string]string `json:"files"`
	Cases        int               `json:"cases"`
	Binding      int               `json:"binding"`
	Provisional  int               `json:"provisional"`
	Excluded     int               `json:"excluded"`
	History      []struct {
		Revision     string `json:"revision"`
		PR465Head    string `json:"pr465_head"`
		CorpusSHA256 string `json:"corpus_sha256"`
		Cases        int    `json:"cases"`
		Binding      int    `json:"binding"`
		Provisional  int    `json:"provisional"`
	} `json:"history"`
	Authority struct {
		BacklogMain     string            `json:"backlog_main"`
		PR464Head       string            `json:"pr464_head"`
		StratPR109      string            `json:"strat_pr109"`
		StratPR110      string            `json:"strat_pr110"`
		StratPR111      string            `json:"strat_pr111"`
		StratPR112Merge string            `json:"strat_pr112_merge"`
		CoreBlobs       map[string]string `json:"reused_core_fixtures_blobs"`
	} `json:"authority"`
}
type seqAcceptanceFile struct {
	Schema     string            `json:"schema"`
	Convention string            `json:"convention_id"`
	Profile    string            `json:"profile_id"`
	Mirror     string            `json:"mirror"`
	Family     string            `json:"family"`
	Cases      []json.RawMessage `json:"cases"`
}
type seqAcceptanceCase struct {
	ID                     string  `json:"id"`
	PreviousID             *string `json:"previous_id"`
	AddedIn                *string `json:"added_in"`
	AllowNonpositivePrices *bool   `json:"allow_nonpositive_prices"`
	Group                  string  `json:"group"`
	Policy                 string  `json:"policy"`
	Status                 string  `json:"status"`
	OpenQuestion           *string `json:"open_question"`
	Title                  string  `json:"title"`
	Derivation             string  `json:"derivation"`
	PrefixGroup            *struct {
		ID            string `json:"id"`
		SharedThrough int    `json:"shared_through_index"`
	} `json:"prefix_group"`
	Series struct {
		Symbol      string  `json:"symbol"`
		Timeframe   string  `json:"timeframe"`
		TimeframeMS int64   `json:"timeframe_ms"`
		StartOpenMS int64   `json:"start_open_ms"`
		OpenMS      []int64 `json:"open_ms,omitempty"`
	} `json:"series"`
	Config json.RawMessage `json:"config"`
	Mirror struct {
		Constant float64 `json:"constant"`
		Exact    bool    `json:"exact"`
	} `json:"mirror"`
	SourceFixture *struct {
		File string `json:"file"`
		Case string `json:"case"`
		Bars string `json:"bars"`
		Blob string `json:"blob"`
	} `json:"source_fixture"`
	Variants map[string]seqAcceptanceVariant `json:"variants"`
}
type seqAcceptanceATRCase struct {
	ID           string  `json:"id"`
	Group        string  `json:"group"`
	Policy       string  `json:"policy"`
	Status       string  `json:"status"`
	OpenQuestion *string `json:"open_question"`
	Title        string  `json:"title"`
	Derivation   string  `json:"derivation"`
	PrefixGroup  *struct {
		ID            string `json:"id"`
		SharedThrough int    `json:"shared_through_index"`
	} `json:"prefix_group"`
	Series struct {
		Symbol      string  `json:"symbol"`
		Timeframe   string  `json:"timeframe"`
		TimeframeMS int64   `json:"timeframe_ms"`
		StartOpenMS int64   `json:"start_open_ms"`
		OpenMS      []int64 `json:"open_ms,omitempty"`
	} `json:"series"`
	Config json.RawMessage `json:"config"`
	Mirror struct {
		Constant float64 `json:"constant"`
		Exact    bool    `json:"exact"`
	} `json:"mirror"`
	SourceFixture *struct {
		File string `json:"file"`
		Case string `json:"case"`
		Bars string `json:"bars"`
		Blob string `json:"blob"`
	} `json:"source_fixture"`
	Variants map[string]seqAcceptanceVariant `json:"variants"`
}
type seqAcceptanceVariant struct {
	Bars []struct {
		O float64 `json:"o"`
		H float64 `json:"h"`
		L float64 `json:"l"`
		C float64 `json:"c"`
	} `json:"bars"`
	Expected json.RawMessage `json:"expected"`
}
type seqAcceptanceConfig struct {
	Slippage    float64 `json:"slippage"`
	Risk        float64 `json:"risk_amount"`
	MaxNotional float64 `json:"max_notional"`
	ATRLength   int     `json:"atr_length"`
	StopBuffer  float64 `json:"stop_buffer_atr"`
	RewardRisk  float64 `json:"reward_risk"`
	Policy      string  `json:"policy"`
	Hold        int     `json:"time_exit_bars"`
}
type seqAcceptanceOK struct {
	Outcome   string                  `json:"outcome"`
	Decisions []seqAcceptanceDecision `json:"decisions"`
	Orders    []seqAcceptanceOrder    `json:"orders"`
	Trades    []seqAcceptanceTrade    `json:"trades"`
}
type seqAcceptanceDecision struct {
	ID             string  `json:"opportunity_id"`
	EpisodeID      string  `json:"episode_id"`
	Policy         string  `json:"policy"`
	SetupSide      string  `json:"setup_side"`
	TradeSide      string  `json:"trade_side"`
	Trigger        string  `json:"trigger"`
	SetupFirst     int     `json:"setup_bar1_index"`
	SetupNinth     int     `json:"setup_bar9_index"`
	AdmittingSetup int     `json:"admitting_setup_bar9_index"`
	DecisionIndex  int     `json:"decision_bar_index"`
	ATR            float64 `json:"atr14"`
	Anchor         struct {
		From  int     `json:"from_index"`
		To    int     `json:"to_index"`
		Price float64 `json:"price"`
	} `json:"anchor"`
	StopBuffer        float64 `json:"stop_buffer"`
	Stop              float64 `json:"stop_price"`
	Eligible          bool    `json:"eligible"`
	Age               *int    `json:"age,omitempty"`
	FillOutsideWindow *bool   `json:"fill_outside_window,omitempty"`
	IneligibleReason  *string `json:"ineligible_reason,omitempty"`
}
type seqAcceptanceOrder struct {
	ID         string   `json:"opportunity_id"`
	EntryIndex *int     `json:"entry_bar_index,omitempty"`
	EntryOpen  *float64 `json:"entry_open,omitempty"`
	EntryFill  *float64 `json:"entry_fill,omitempty"`
	Status     string   `json:"status"`
	Reason     *string  `json:"reason,omitempty"`
}
type seqAcceptanceTrade struct {
	ID                string  `json:"opportunity_id"`
	EntryIndex        int     `json:"entry_bar_index"`
	EntryOpen         float64 `json:"entry_open"`
	EntryFill         float64 `json:"entry_fill"`
	Side              string  `json:"side"`
	Stop              float64 `json:"stop"`
	Target            float64 `json:"target"`
	RiskDistance      float64 `json:"risk_distance"`
	SizeUnconstrained float64 `json:"size_unconstrained"`
	Size              float64 `json:"size"`
	Notional          float64 `json:"notional"`
	CapBinds          bool    `json:"cap_binds"`
	Exit              struct {
		Index    int     `json:"bar_index"`
		Reason   string  `json:"reason"`
		Price    float64 `json:"price"`
		HeldBars int     `json:"held_bars"`
	} `json:"exit"`
	GrossPnL float64 `json:"gross_pnl"`
	R        float64 `json:"r_multiple"`
}
type seqAcceptanceRefused struct {
	Outcome string `json:"outcome"`
	Refusal struct {
		Kind         string `json:"kind"`
		Index        int    `json:"at_bar_index"`
		KindBinding  bool   `json:"kind_binding"`
		IndexBinding bool   `json:"index_binding"`
	} `json:"refusal"`
}

// Every expected envelope field comes from explicit surface inputs, never from
// the returned result. Direct/prepared calls have no fixture Case identifier.
type seqAcceptanceEnvelope struct {
	Schema          string
	Case            string
	StrategyID      string
	Symbol          string
	Timeframe       string
	RangeMethod     string
	HigherTimeframe string
	Costs           engine.Costs
}

func TestSequentialIndependentAcceptance(t *testing.T) {
	root := filepath.Join("testdata", "sequential_execution_e1_e2_v1")
	cases := seqAcceptanceLoad(t, root)
	for _, c := range cases {
		c := c
		t.Run(c.ID, func(t *testing.T) {
			if c.Status == "provisional" || c.Status == "excluded" {
				t.Skipf("explicit %s record: %s (%s); no producer behavior frozen", c.Status, c.ID, *c.OpenQuestion)
			}
			for _, side := range []string{"long", "short"} {
				v := c.Variants[side]
				t.Run(side, func(t *testing.T) {
					bars := seqAcceptanceBars(t, c, v)
					if c.Policy == "both" {
						seqAcceptanceATR(t, c, v, bars)
						return
					}
					var cfg seqAcceptanceConfig
					seqAcceptanceReadJSON(t, c.Config, &cfg)
					costs, err := seqAcceptanceCosts(c.Policy, cfg)
					if err != nil {
						t.Fatalf("binding corpus uses unsupported configuration: %v", err)
					}
					source := fmt.Sprintf("dsl v7\nstrategy \"Independent E1/E2 acceptance\" { description \"Read-only independent corpus\" }\nmarket { sequential symbol %s sequential timeframe %s }\nsetup { type: sequential full sequential profile %s sequential policy %s }\nrisk { sequential riskUsd %.17g sequential maxNotionalUsd %.17g }\n", c.Series.Symbol, c.Series.Timeframe, seqAcceptanceProfile, c.Policy, cfg.Risk, cfg.MaxNotional)
					parsed, err := dsl.Parse(source)
					if err != nil || len(parsed.Errors) != 0 {
						t.Fatalf("DSL transport failed: %v; %v", err, parsed.Errors)
					}
					if err := dsl.SequentialFullParseError(parsed); err != nil {
						t.Fatal(err)
					}
					request := engine.RunRequest{Config: parsed.Config, Series: marketdata.SeriesFromBars(bars), StrategyID: c.ID, Symbol: c.Series.Symbol, Timeframe: c.Series.Timeframe, RangeMethod: "zone", Costs: costs}
					envelope := seqAcceptanceEnvelope{Schema: "dsl-conformance-trades-v1", Case: "", StrategyID: request.StrategyID, Symbol: request.Symbol, Timeframe: request.Timeframe, RangeMethod: request.RangeMethod, HigherTimeframe: request.HigherTimeframe, Costs: request.Costs}
					t.Run("Run", func(t *testing.T) {
						got, err := engine.Run(request)
						seqAcceptanceResult(t, c, v, cfg, bars, envelope, got, err)
					})
					t.Run("PrepareRun", func(t *testing.T) {
						prepared, err := engine.PrepareRun(request)
						if err != nil {
							seqAcceptanceResult(t, c, v, cfg, bars, envelope, engine.RunResult{}, err)
							return
						}
						// Reuse is exercised without accepting the first run as an oracle.
						for i := 0; i < 2; i++ {
							got, err := prepared.RunChecked(costs)
							seqAcceptanceResult(t, c, v, cfg, bars, envelope, got, err)
						}
					})
					t.Run("FixtureDSL", func(t *testing.T) {
						rows := make([][]float64, len(bars))
						for i, b := range bars {
							rows[i] = []float64{b.T, b.O, b.H, b.L, b.C, b.V}
						}
						fixture := engine.RunFixture{Schema: "dsl-conformance-run-fixture-v1", Case: c.ID + "." + side, StrategyID: c.ID, Symbol: c.Series.Symbol, Timeframe: c.Series.Timeframe, RangeMethod: "zone", Costs: costs, RawBars: rows}
						raw, err := json.Marshal(fixture)
						if err != nil {
							t.Fatal(err)
						}
						path := filepath.Join(t.TempDir(), "input.json")
						if err := os.WriteFile(path, raw, 0600); err != nil {
							t.Fatal(err)
						}
						loaded, err := engine.LoadRunFixture(path)
						if err != nil {
							t.Fatalf("fixture transport: %v", err)
						}
						fixtureEnvelope := seqAcceptanceEnvelope{Schema: "dsl-conformance-trades-v1", Case: fixture.Case, StrategyID: fixture.StrategyID, Symbol: fixture.Symbol, Timeframe: fixture.Timeframe, RangeMethod: fixture.RangeMethod, HigherTimeframe: fixture.HigherTimeframe, Costs: fixture.Costs}
						got, err := engine.RunFixtureCase(loaded, source)
						seqAcceptanceResult(t, c, v, cfg, bars, fixtureEnvelope, got, err)
					})
				})
			}
		})
	}
}

func seqAcceptanceCosts(policy string, c seqAcceptanceConfig) (engine.Costs, error) {
	hold := 4
	if policy == "E2" {
		hold = 12
	}
	if (policy != "E1" && policy != "E2") || c.Policy != policy || c.ATRLength != 14 || c.StopBuffer != 0.1 || c.RewardRisk != 2 || c.Hold != hold {
		return engine.Costs{}, fmt.Errorf("unsupported convention/config: %+v", c)
	}
	if c.Slippage < 0 || c.Risk <= 0 || c.MaxNotional <= 0 {
		return engine.Costs{}, errors.New("invalid costs/risk bounds")
	}
	return engine.Costs{FillOn: "open", Slippage: c.Slippage, FeePerUnit: 0, StartEquity: 10000}, nil
}

func seqAcceptanceBars(t *testing.T, c seqAcceptanceCase, v seqAcceptanceVariant) []marketdata.Bar {
	t.Helper()
	if c.Series.Symbol != "SYN" || c.Series.Timeframe != "5m" || c.Series.TimeframeMS != 300000 {
		t.Fatal("unsupported synthetic route")
	}
	if len(v.Bars) == 0 || (c.Series.OpenMS != nil && len(c.Series.OpenMS) != len(v.Bars)) {
		t.Fatal("invalid bar/time shape")
	}
	bars := make([]marketdata.Bar, len(v.Bars))
	for i, b := range v.Bars {
		ts := c.Series.StartOpenMS + int64(i)*c.Series.TimeframeMS
		if c.Series.OpenMS != nil {
			ts = c.Series.OpenMS[i]
		}
		if ts < 0 || ts > 1<<53 || b.H < math.Max(b.O, b.C) || b.L > math.Min(b.O, b.C) || b.L > b.H {
			t.Fatalf("invalid synthetic bar %d", i)
		}
		bars[i] = marketdata.Bar{T: float64(ts), O: b.O, H: b.H, L: b.L, C: b.C, V: 1}
	}
	return bars
}

func seqAcceptanceATR(t *testing.T, c seqAcceptanceCase, v seqAcceptanceVariant, bars []marketdata.Bar) {
	t.Helper()
	var cfg struct {
		Length int `json:"atr_length"`
	}
	var want struct {
		ATR []*float64 `json:"atr14"`
	}
	seqAcceptanceReadJSON(t, c.Config, &cfg)
	seqAcceptanceReadJSON(t, v.Expected, &want)
	if cfg.Length != 14 || len(want.ATR) != len(bars) {
		t.Fatal("invalid ATR fixture shape")
	}
	got := contextcols.ComputeATR(marketdata.SeriesFromBars(bars), cfg.Length)
	if len(got) != len(want.ATR) {
		t.Fatalf("ATR length %d, want %d", len(got), len(want.ATR))
	}
	for i, w := range want.ATR {
		if i < cfg.Length-1 {
			if w != nil {
				t.Errorf("ATR eligibility[%d] must be null before 14 bars", i)
			}
			continue // Do not pretend ComputeATR returns null or overwrite its output.
		}
		if w == nil {
			t.Fatalf("ATR[%d] unexpectedly unavailable", i)
		}
		seqAcceptanceNear(t, fmt.Sprintf("atr14[%d]", i), got[i], *w)
	}
}

func seqAcceptanceResult(t *testing.T, c seqAcceptanceCase, v seqAcceptanceVariant, cfg seqAcceptanceConfig, bars []marketdata.Bar, envelope seqAcceptanceEnvelope, got engine.RunResult, runErr error) {
	t.Helper()
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(v.Expected, &probe); err != nil {
		t.Fatal(err)
	}
	var outcome string
	if err := json.Unmarshal(probe["outcome"], &outcome); err != nil {
		t.Fatal("missing outcome")
	}
	if outcome == "refused" {
		var want seqAcceptanceRefused
		seqAcceptanceReadJSON(t, v.Expected, &want)
		if runErr == nil {
			t.Fatal("run succeeded, want refusal")
		}
		// Do not let a parser/transport fault masquerade as a run refusal.
		var actual *engine.SequentialFullExecutionError
		if !errors.As(runErr, &actual) {
			t.Fatalf("unexpected error class (not sequential refusal): %T: %v", runErr, runErr)
		}
		for _, problem := range seqAcceptanceEnvelopeProblems(got, envelope, true) {
			t.Error(problem)
		}
		// PR464 already binds terminal refusal despite the older corpus's false
		// kind_binding flag. Missing intervals must retain the actual typed gap
		// category, rather than accepting any unrelated validation failure.
		kind, err := seqAcceptanceRefusalKind(c, bars)
		if err != nil {
			t.Fatal(err)
		}
		seqAcceptanceExact(t, "refusal.required_category", actual.Kind, kind)
		if want.Refusal.KindBinding {
			seqAcceptanceExact(t, "refusal.kind", actual.Kind, want.Refusal.Kind)
		}
		if want.Refusal.IndexBinding {
			seqAcceptanceExact(t, "refusal.at_bar_index", actual.BarIndex, want.Refusal.Index)
		}
		t.Logf("refused: kind=%s index=%d; corpus suggestion kind=%s index=%d (binding flags: kind=%t index=%t)", actual.Kind, actual.BarIndex, want.Refusal.Kind, want.Refusal.Index, want.Refusal.KindBinding, want.Refusal.IndexBinding)
		return
	}
	if outcome != "ok" {
		t.Fatalf("unsupported outcome %q", outcome)
	}
	var want seqAcceptanceOK
	seqAcceptanceReadJSON(t, v.Expected, &want)
	if runErr != nil {
		t.Fatalf("unexpected refusal: %v", runErr)
	}
	for _, problem := range seqAcceptanceEnvelopeProblems(got, envelope, false) {
		t.Error(problem)
	}
	if got.SequentialFull == nil {
		t.Fatal("missing sequential audit")
	}
	a := got.SequentialFull
	seqAcceptanceExact(t, "audit.schema", a.Schema, "sequential-full-execution.v1")
	seqAcceptanceExact(t, "audit.profile", a.Profile, seqAcceptanceProfile)
	seqAcceptanceExact(t, "audit.policy", a.Policy, c.Policy)
	seqAcceptanceExact(t, "audit.symbol", a.Symbol, c.Series.Symbol)
	seqAcceptanceExact(t, "audit.timeframe", a.Timeframe, c.Series.Timeframe)
	if len(a.Opportunities) != len(want.Decisions) {
		t.Fatalf("decisions count %d, want %d", len(a.Opportunities), len(want.Decisions))
	}
	opportunities := map[string]engine.SequentialFullOpportunity{}
	decisions := map[string]seqAcceptanceDecision{}
	var orderAudit []engine.SequentialFullOpportunity
	for i, o := range a.Opportunities {
		w := want.Decisions[i]
		if _, ok := opportunities[o.ID]; ok {
			t.Fatalf("duplicate actual opportunity %q", o.ID)
		}
		opportunities[o.ID], decisions[w.ID] = o, w
		seqAcceptanceDecisionFields(t, c, bars, o, w)
		if o.Reason != engine.SequentialFullExpired && o.Reason != engine.SequentialFullWarmup {
			orderAudit = append(orderAudit, o)
		}
	}
	if len(orderAudit) != len(want.Orders) {
		t.Fatalf("order audit count %d, want %d", len(orderAudit), len(want.Orders))
	}
	for i, o := range orderAudit {
		seqAcceptanceOrderFields(t, bars, o, want.Orders[i])
	}
	seqAcceptanceExact(t, "tradeCount", got.TradeCount, len(want.Trades))
	if len(got.Trades) != len(want.Trades) {
		t.Fatalf("trades count %d, want %d", len(got.Trades), len(want.Trades))
	}
	for i, trade := range got.Trades {
		w := want.Trades[i]
		id, ok := trade.Meta["opportunityId"].(string)
		if !ok {
			t.Fatal("trade has no actual opportunity identity")
		}
		o, ok := opportunities[id]
		if !ok {
			t.Fatalf("trade references unknown opportunity %q", id)
		}
		seqAcceptanceTradeFields(t, c, cfg, bars, trade, o, decisions[w.ID], w)
	}
}

// Returning problems lets the guard test mutation-check this validator without
// deliberately failing child tests or weakening the acceptance comparison.
func seqAcceptanceEnvelopeProblems(got engine.RunResult, want seqAcceptanceEnvelope, refused bool) []string {
	var problems []string
	if got.TimedAudit != nil {
		problems = append(problems, "unrelated TimedAudit in Sequential result")
	}
	if got.ClockRangeDays != nil {
		problems = append(problems, "unrelated ClockRangeDays in Sequential result")
	}
	if refused {
		// Public Run/RunChecked/RunFixtureCase return a zero result on typed refusal.
		// Reject even a metadata-only partial envelope or a non-nil empty result list.
		if !reflect.DeepEqual(got, engine.RunResult{}) {
			problems = append(problems, fmt.Sprintf("typed refusal returned a partial public result: %+v", got))
		}
		return problems
	}
	actual := seqAcceptanceEnvelope{Schema: got.Schema, Case: got.Case, StrategyID: got.StrategyID, Symbol: got.Symbol, Timeframe: got.Timeframe, RangeMethod: got.RangeMethod, HigherTimeframe: got.HigherTimeframe, Costs: got.Costs}
	av, wv := reflect.ValueOf(actual), reflect.ValueOf(want)
	for i := 0; i < av.NumField(); i++ {
		if !reflect.DeepEqual(av.Field(i).Interface(), wv.Field(i).Interface()) {
			problems = append(problems, fmt.Sprintf("result.%s = %#v, want exactly %#v", av.Type().Field(i).Name, av.Field(i).Interface(), wv.Field(i).Interface()))
		}
	}
	return problems
}

func seqAcceptanceNotional(size, entry float64) float64 {
	return size * math.Abs(entry)
}

func seqAcceptanceRefusalKind(c seqAcceptanceCase, bars []marketdata.Bar) (string, error) {
	terminal := map[string]bool{
		"e1.refuse_open_at_end_entry_bar":        true,
		"e1.refuse_open_at_end_last_checked_bar": true,
		"e1.refuse_open_at_end_gap_entry":        true,
		"e2.refuse_open_at_end_last_checked_bar": true,
		"e2.refuse_open_at_end_entry_bar":        true,
	}
	gaps := map[string]bool{
		"e1.refuse_missing_interval_before_decision": true,
		"e1.refuse_missing_interval_during_position": true,
		"e1.refuse_missing_interval_after_exit":      true,
		"e2.refuse_missing_interval_in_countdown":    true,
	}
	hasGap := false
	for i := 1; i < len(bars); i++ {
		if bars[i].T <= bars[i-1].T {
			return "", errors.New("refusal corpus is not strictly increasing")
		}
		if bars[i].T != bars[i-1].T+float64(c.Series.TimeframeMS) {
			hasGap = true
		}
	}
	if c.Group == "refusals" && terminal[c.ID] && !hasGap {
		return "unsupported-incomplete-terminal-run", nil
	}
	if c.Group == "refusals" && gaps[c.ID] && hasGap {
		return "unsupported-sequential-time-gap", nil
	}
	return "", fmt.Errorf("%s: unknown refusal category or changed timestamp input; review adapter", c.ID)
}

func seqAcceptanceDecisionFields(t *testing.T, c seqAcceptanceCase, bars []marketdata.Bar, o engine.SequentialFullOpportunity, w seqAcceptanceDecision) {
	t.Helper()
	seqAcceptanceExact(t, "decision.opportunity_id", o.ID, w.ID)
	seqAcceptanceExact(t, "decision.episode_id", o.EpisodeID, w.EpisodeID)
	seqAcceptanceExact(t, "decision.policy", c.Policy, w.Policy)
	seqAcceptanceExact(t, "decision.trade_side", o.Side, w.TradeSide)
	side, sign := "buy", 1.0
	if o.Side == "short" {
		side, sign = "sell", -1
	} else if o.Side != "long" {
		t.Fatalf("unknown side %q", o.Side)
	}
	seqAcceptanceExact(t, "decision.setup_side", side, w.SetupSide)
	trigger := o.Trigger
	if trigger == "perf" {
		trigger = "perfection"
		if o.DecisionIndex != o.SetupIndex {
			trigger = "delayed_perfection"
		}
	} else if trigger == "cd13" {
		trigger = "countdown_complete"
	} else {
		t.Fatalf("unknown trigger %q", trigger)
	}
	seqAcceptanceExact(t, "decision.trigger", trigger, w.Trigger)
	seqAcceptanceExact(t, "decision.setup_bar1_index", o.SetupFirstIndex, w.SetupFirst)
	seqAcceptanceExact(t, "decision.setup_bar9_index", o.SetupIndex, w.SetupNinth)
	seqAcceptanceExact(t, "decision.admitting_setup_bar9_index", o.SetupIndex, w.AdmittingSetup)
	seqAcceptanceExact(t, "decision.decision_bar_index", o.DecisionIndex, w.DecisionIndex)
	if w.DecisionIndex < 0 || w.DecisionIndex >= len(bars) {
		t.Fatal("expected decision index outside input")
	}
	seqAcceptanceExact(t, "decision.open_ms", o.DecisionOpenMS, int64(bars[w.DecisionIndex].T))
	seqAcceptanceExact(t, "decision.close_ms", o.DecisionMS, int64(bars[w.DecisionIndex].T)+c.Series.TimeframeMS)
	seqAcceptanceExact(t, "decision.next_open_index", o.NextOpenIndex, w.DecisionIndex+1)
	seqAcceptanceNear(t, "decision.atr14", o.SignalATR, w.ATR)
	seqAcceptanceNear(t, "decision.stop_price", o.Stop, w.Stop)
	seqAcceptanceExact(t, "decision.anchor.from", o.SetupFirstIndex, w.Anchor.From)
	end := o.SetupIndex
	if c.Policy == "E2" {
		end = o.DecisionIndex
	}
	seqAcceptanceExact(t, "decision.anchor.to", end, w.Anchor.To)
	// Invert actual audited stop = anchor +/- buffer. No new lifecycle oracle.
	seqAcceptanceNear(t, "decision.stop_buffer (projection)", 0.1*o.SignalATR, w.StopBuffer)
	seqAcceptanceNear(t, "decision.anchor.price (projection)", o.Stop+sign*0.1*o.SignalATR, w.Anchor.Price)
	eligible := o.Reason != engine.SequentialFullExpired && o.Reason != engine.SequentialFullWarmup
	seqAcceptanceExact(t, "decision.eligible", eligible, w.Eligible)
	if c.Policy == "E1" {
		if w.Age == nil {
			t.Fatal("E1 decision missing age")
		}
		seqAcceptanceExact(t, "decision.age", o.DecisionIndex-o.SetupIndex, *w.Age)
		if w.Eligible {
			if w.FillOutsideWindow == nil {
				t.Fatal("eligible E1 decision missing fill_outside_window")
			}
			seqAcceptanceExact(t, "decision.fill_outside_window", o.NextOpenIndex-o.SetupIndex > 4, *w.FillOutsideWindow)
		}
	} else if w.Age != nil || w.FillOutsideWindow != nil {
		t.Fatal("E2 decision has E1-only fields")
	}
	if !w.Eligible {
		if w.IneligibleReason == nil {
			t.Fatal("ineligible decision missing reason")
		}
		seqAcceptanceExact(t, "decision.ineligible_reason", string(o.Reason), *w.IneligibleReason)
		seqAcceptanceExact(t, "decision.status", o.Status, engine.SequentialFullRejected)
		seqAcceptanceNoFill(t, o)
	} else if w.IneligibleReason != nil {
		t.Fatal("eligible decision has ineligible_reason")
	}
}

func seqAcceptanceNoFill(t *testing.T, o engine.SequentialFullOpportunity) {
	t.Helper()
	if o.FillIndex != nil || o.Fill != nil || o.Target != nil || o.RiskDistance != nil || o.Size != nil || o.CapBinds {
		t.Errorf("rejected opportunity carries fill/bracket/size data: %+v", o)
	}
}
func seqAcceptanceOrderFields(t *testing.T, bars []marketdata.Bar, o engine.SequentialFullOpportunity, w seqAcceptanceOrder) {
	t.Helper()
	seqAcceptanceExact(t, "order.opportunity_id", o.ID, w.ID)
	reason := ""
	if w.Reason != nil {
		reason = *w.Reason
	}
	seqAcceptanceExact(t, "order.reason", string(o.Reason), reason)
	if w.Status == "not_placed" {
		if reason != "no_next_bar" || w.EntryIndex != nil || w.EntryOpen != nil || w.EntryFill != nil {
			t.Fatal("not_placed must be a no_next_bar audit record without executable entry data")
		}
		seqAcceptanceExact(t, "order.audit_status", o.Status, engine.SequentialFullRejected)
		seqAcceptanceExact(t, "order.next_open_index", o.NextOpenIndex, len(bars))
		seqAcceptanceNoFill(t, o)
		return
	}
	if w.Status != "filled" && w.Status != "rejected" {
		t.Fatalf("unsupported order status %q", w.Status)
	}
	seqAcceptanceExact(t, "order.status", string(o.Status), w.Status)
	if w.EntryIndex == nil || w.EntryOpen == nil || w.EntryFill == nil {
		t.Fatal("entry audit missing fields")
	}
	seqAcceptanceExact(t, "order.entry_bar_index", o.NextOpenIndex, *w.EntryIndex)
	if o.NextOpenIndex < 0 || o.NextOpenIndex >= len(bars) {
		t.Fatal("actual entry index outside input")
	}
	seqAcceptanceNear(t, "order.entry_open (input lookup)", bars[o.NextOpenIndex].O, *w.EntryOpen)
	if w.Status == "rejected" {
		seqAcceptanceNoFill(t, o)
		t.Logf("projection exclusion: %s rejected attempted entry_fill is not exposed by the actual audit; expected %.17g is not compared or synthesized", o.ID, *w.EntryFill)
		return
	}
	if o.Fill == nil || o.FillIndex == nil {
		t.Fatal("filled audit lacks fill")
	}
	seqAcceptanceExact(t, "order.fill_index", *o.FillIndex, *w.EntryIndex)
	seqAcceptanceNear(t, "order.entry_fill", *o.Fill, *w.EntryFill)
}

func seqAcceptanceTradeFields(t *testing.T, c seqAcceptanceCase, cfg seqAcceptanceConfig, bars []marketdata.Bar, g engine.Trade, o engine.SequentialFullOpportunity, d seqAcceptanceDecision, w seqAcceptanceTrade) {
	t.Helper()
	seqAcceptanceExact(t, "trade.opportunity_id", g.Meta["opportunityId"], w.ID)
	seqAcceptanceExact(t, "trade.episode_id", g.Meta["episodeId"], d.EpisodeID)
	seqAcceptanceExact(t, "trade.profile", g.Meta["profile"], seqAcceptanceProfile)
	seqAcceptanceExact(t, "trade.policy", g.Meta["policy"], c.Policy)
	seqAcceptanceExact(t, "trade.signal_index", g.Meta["signalIndex"], float64(d.DecisionIndex))
	atr, ok := g.Meta["signalAtr"].(float64)
	if !ok {
		t.Fatal("trade missing signalAtr")
	}
	seqAcceptanceNear(t, "trade.signal_atr", atr, d.ATR)
	seqAcceptanceExact(t, "trade.entry_bar_index", g.EntryIndex, w.EntryIndex)
	seqAcceptanceExact(t, "trade.exit.bar_index", g.ExitIndex, w.Exit.Index)
	if w.EntryIndex < 0 || w.EntryIndex >= len(bars) || w.Exit.Index < w.EntryIndex || w.Exit.Index >= len(bars) {
		t.Fatal("expected trade indices outside input")
	}
	seqAcceptanceExact(t, "trade.entry_ms", g.EntryT, bars[w.EntryIndex].T)
	seqAcceptanceExact(t, "trade.exit_ms", g.ExitT, bars[w.Exit.Index].T)
	if g.EntryIndex < 0 || g.EntryIndex >= len(bars) || g.ExitIndex < g.EntryIndex || g.ExitIndex >= len(bars) {
		t.Fatal("actual trade indices outside input")
	}
	seqAcceptanceNear(t, "trade.entry_open (input lookup)", bars[g.EntryIndex].O, w.EntryOpen)
	seqAcceptanceNear(t, "trade.entry_fill", g.Entry, w.EntryFill)
	seqAcceptanceExact(t, "trade.side", g.Side, w.Side)
	seqAcceptanceExact(t, "trade.tag", g.Tag, "SEQUENTIAL-"+c.Policy)
	if g.Partial || g.NoStop || g.NoTarget || g.Rule != "" {
		t.Errorf("unexpected partial/no-bracket/rule trade: %+v", g)
	}
	seqAcceptanceNear(t, "trade.initial_stop", g.InitialSL, w.Stop)
	seqAcceptanceNear(t, "trade.stop", g.SL, w.Stop)
	seqAcceptanceNear(t, "trade.initial_target", g.InitialTP, w.Target)
	seqAcceptanceNear(t, "trade.target", g.TP, w.Target)
	if o.FillIndex == nil || o.Fill == nil || o.Target == nil || o.RiskDistance == nil || o.Size == nil {
		t.Fatal("trade's opportunity lacks filled bracket")
	}
	seqAcceptanceExact(t, "trade.audit_fill_index", *o.FillIndex, w.EntryIndex)
	seqAcceptanceNear(t, "trade.audit_fill", *o.Fill, w.EntryFill)
	seqAcceptanceNear(t, "trade.audit_target", *o.Target, w.Target)
	seqAcceptanceNear(t, "trade.risk_distance", *o.RiskDistance, w.RiskDistance)
	seqAcceptanceNear(t, "trade.size_unconstrained (projection)", cfg.Risk / *o.RiskDistance, w.SizeUnconstrained)
	seqAcceptanceNear(t, "trade.size", g.Size, w.Size)
	seqAcceptanceNear(t, "trade.audit_size", *o.Size, w.Size)
	seqAcceptanceNear(t, "trade.notional (projection)", seqAcceptanceNotional(g.Size, g.Entry), w.Notional)
	seqAcceptanceExact(t, "trade.cap_binds", o.CapBinds, w.CapBinds)
	reasons := map[string]string{"sl": "stop", "tp": "target", "time": "time_exit"}
	reason, ok := reasons[g.Reason]
	if !ok {
		t.Fatalf("unsupported actual exit reason %q", g.Reason)
	}
	seqAcceptanceExact(t, "trade.exit.reason", reason, w.Exit.Reason)
	seqAcceptanceNear(t, "trade.exit.price", g.Exit, w.Exit.Price)
	held := g.ExitIndex - g.EntryIndex
	if g.Reason != "time" {
		held++
	}
	seqAcceptanceExact(t, "trade.exit.held_bars (projection)", held, w.Exit.HeldBars)
	seqAcceptanceNear(t, "trade.gross_pnl (zero fees)", g.PnL, w.GrossPnL)
	seqAcceptanceNear(t, "trade.r_multiple (actual points/risk)", g.Points / *o.RiskDistance, w.R)
}

func seqAcceptanceNear(t *testing.T, name string, got, want float64) {
	t.Helper()
	if !seqAcceptanceClose(got, want) {
		t.Errorf("%s = %.17g, want %.17g (relative %.1g, no absolute floor)", name, got, want, seqAcceptanceRelative)
	}
}
func seqAcceptanceClose(got, want float64) bool {
	if math.IsNaN(got) || math.IsNaN(want) || math.IsInf(got, 0) || math.IsInf(want, 0) {
		return false
	}
	if got == want {
		return true
	}
	return math.Abs(got-want) <= seqAcceptanceRelative*math.Max(math.Abs(got), math.Abs(want))
}
func seqAcceptanceExact(t *testing.T, name string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want exactly %#v", name, got, want)
	}
}

func seqAcceptanceLoad(t *testing.T, root string) []seqAcceptanceCase {
	t.Helper()
	read := func(name string) []byte {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("required independent corpus file %s: %v", name, err)
		}
		return raw
	}
	var sources seqAcceptanceSources
	seqAcceptanceReadJSON(t, read("SOURCES.json"), &sources)
	if sources.Schema != "seq-exec-sources.v1" || sources.Repository != "spik3r/heisentick-backlog" || sources.Root != "fixtures/sequential-execution-e1-e2.v1" || sources.Commit != seqAcceptanceSourceCommit {
		t.Fatal("invalid independent SOURCES identity")
	}
	claimed := map[string]bool{"MANIFEST.json": true, "README.md": true, "DERIVATIONS.md": true, "cases/atr.json": true, "cases/e1.json": true, "cases/e2.json": true, "cases/excluded.json": true, "cases/provisional.json": true, "cases/refusals.json": true}
	if len(sources.Files) != len(claimed) {
		t.Fatal("SOURCES must pin exactly the nine claimed source files; no author tools")
	}
	for name, pin := range sources.Files {
		if !claimed[name] || !seqAcceptanceSafePath(name) || name == "SOURCES.json" || pin.SourcePath != sources.Root+"/"+name || !seqAcceptanceDigest(pin.GitBlob, 20) || !seqAcceptanceDigest(pin.SHA256, 32) {
			t.Fatalf("invalid source pin %q", name)
		}
		raw := read(name)
		seqAcceptanceExact(t, name+" sha256", fmt.Sprintf("%x", sha256.Sum256(raw)), pin.SHA256)
		blob := sha1.New()
		fmt.Fprintf(blob, "blob %d\x00", len(raw))
		_, _ = blob.Write(raw)
		seqAcceptanceExact(t, name+" git blob", fmt.Sprintf("%x", blob.Sum(nil)), pin.GitBlob)
	}
	// Catch unpinned additions, symlinks, and a shortened SOURCES inventory.
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink forbidden: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name = filepath.ToSlash(name)
		if name != "SOURCES.json" {
			if _, ok := sources.Files[name]; !ok {
				return fmt.Errorf("un-pinned imported file %s", name)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := sources.Files["MANIFEST.json"]; !ok {
		t.Fatal("SOURCES must pin MANIFEST.json")
	}
	var manifest seqAcceptanceManifest
	seqAcceptanceReadJSON(t, read("MANIFEST.json"), &manifest)
	seqAcceptanceExact(t, "manifest.schema", manifest.Schema, seqAcceptanceSchema)
	seqAcceptanceExact(t, "manifest.revision", manifest.Revision, "v1.2: owner cap decision, exit costs settled, OQ-4 excluded, two non-positive-fill fixtures")
	seqAcceptanceExact(t, "manifest.convention", manifest.Convention, seqAcceptanceConvention)
	seqAcceptanceExact(t, "manifest.hash_rule", manifest.HashRule, "sha256 of the lines \"<sha256>  <path>\\n\" for cases/*.json (sorted), README.md, DERIVATIONS.md")
	casePaths := []string{"cases/atr.json", "cases/e1.json", "cases/e2.json", "cases/excluded.json", "cases/provisional.json", "cases/refusals.json"}
	manifestPaths := append(append([]string{}, casePaths...), "README.md", "DERIVATIONS.md")
	if len(manifest.Files) != len(manifestPaths) {
		t.Fatalf("manifest inventory changed: got %d, want %d; review the adapter", len(manifest.Files), len(manifestPaths))
	}
	var corpusHash bytes.Buffer
	for _, name := range manifestPaths {
		digest, ok := manifest.Files[name]
		if !ok || !seqAcceptanceDigest(digest, 32) {
			t.Fatalf("missing/invalid manifest digest: %s", name)
		}
		pin, ok := sources.Files[name]
		if !ok {
			t.Fatalf("SOURCES does not pin %s", name)
		}
		seqAcceptanceExact(t, name+" manifest/source agreement", digest, pin.SHA256)
		seqAcceptanceExact(t, name+" manifest digest", fmt.Sprintf("%x", sha256.Sum256(read(name))), digest)
		fmt.Fprintf(&corpusHash, "%s  %s\n", digest, name)
	}
	seqAcceptanceExact(t, "manifest.corpus_sha256", fmt.Sprintf("%x", sha256.Sum256(corpusHash.Bytes())), manifest.CorpusSHA256)
	seqAcceptanceExact(t, "pinned corpus hash", manifest.CorpusSHA256, seqAcceptanceCorpusHash)
	var all []seqAcceptanceCase
	ids := map[string]bool{}
	provisional := map[string]bool{}
	excluded := map[string]bool{}
	binding := 0
	for _, name := range casePaths {
		var f seqAcceptanceFile
		seqAcceptanceReadJSON(t, read(name), &f)
		seqAcceptanceExact(t, name+".schema", f.Schema, seqAcceptanceSchema)
		seqAcceptanceExact(t, name+".convention", f.Convention, seqAcceptanceConvention)
		seqAcceptanceExact(t, name+".profile", f.Profile, seqAcceptanceProfile)
		seqAcceptanceExact(t, name+".family", f.Family, strings.TrimSuffix(filepath.Base(name), ".json"))
		if len(f.Cases) == 0 {
			t.Fatalf("empty case file %s", name)
		}
		for _, raw := range f.Cases {
			var c seqAcceptanceCase
			if name == "cases/atr.json" {
				// ATR metadata is the unchanged v1.1 schema; do not invent authored fields.
				var atr seqAcceptanceATRCase
				seqAcceptanceReadJSON(t, raw, &atr)
				c = seqAcceptanceCase{ID: atr.ID, Group: atr.Group, Policy: atr.Policy, Status: atr.Status, OpenQuestion: atr.OpenQuestion, Title: atr.Title, Derivation: atr.Derivation, PrefixGroup: atr.PrefixGroup, Series: atr.Series, Config: atr.Config, Mirror: atr.Mirror, SourceFixture: atr.SourceFixture, Variants: atr.Variants}
			} else {
				seqAcceptanceReadJSON(t, raw, &c)
			}
			if err := seqAcceptanceMetadata(c); err != nil {
				t.Fatal(err)
			}
			if c.ID == "" || ids[c.ID] {
				t.Fatalf("empty/duplicate case identity %q", c.ID)
			}
			ids[c.ID] = true
			if len(c.Variants) != 2 {
				t.Fatalf("%s: expected exactly long and short variants", c.ID)
			}
			for _, side := range []string{"long", "short"} {
				v, ok := c.Variants[side]
				if !ok {
					t.Fatalf("%s: missing %s variant", c.ID, side)
				}
				// Validate every oracle field before running any exported surface.
				if c.Policy == "both" {
					var cfg struct {
						Length int `json:"atr_length"`
					}
					seqAcceptanceReadJSON(t, c.Config, &cfg)
					var w struct {
						ATR []*float64 `json:"atr14"`
					}
					seqAcceptanceReadJSON(t, v.Expected, &w)
				} else {
					var cfg seqAcceptanceConfig
					seqAcceptanceReadJSON(t, c.Config, &cfg)
					var probe map[string]json.RawMessage
					if err := json.Unmarshal(v.Expected, &probe); err != nil {
						t.Fatal(err)
					}
					var outcome string
					if err := json.Unmarshal(probe["outcome"], &outcome); err != nil {
						t.Fatalf("%s/%s: missing outcome", c.ID, side)
					}
					switch outcome {
					case "ok":
						var w seqAcceptanceOK
						seqAcceptanceReadJSON(t, v.Expected, &w)
					case "refused":
						var w seqAcceptanceRefused
						seqAcceptanceReadJSON(t, v.Expected, &w)
					default:
						t.Fatalf("%s/%s: unsupported outcome %q", c.ID, side, outcome)
					}
				}
			}
			oq, wasProvisional := seqAcceptanceProvisional[c.ID]
			excludedOQ, wasExcluded := seqAcceptanceExcluded[c.ID]
			switch c.Status {
			case "binding":
				if c.OpenQuestion != nil || wasProvisional || wasExcluded {
					t.Fatalf("%s: provisional status promotion requires explicit adapter review", c.ID)
				}
				binding++
			case "provisional":
				if !wasProvisional || c.OpenQuestion == nil || *c.OpenQuestion != oq {
					t.Fatalf("%s: unexpected provisional exclusion", c.ID)
				}
				provisional[c.ID] = true
			case "excluded":
				if !wasExcluded || c.OpenQuestion == nil || *c.OpenQuestion != excludedOQ {
					t.Fatalf("%s: unexpected excluded proposal", c.ID)
				}
				excluded[c.ID] = true
			default:
				t.Fatalf("%s: unsupported status %q", c.ID, c.Status)
			}
			all = append(all, c)
		}
	}
	seqAcceptanceExact(t, "manifest.cases", manifest.Cases, len(all))
	seqAcceptanceExact(t, "manifest.binding", manifest.Binding, binding)
	seqAcceptanceExact(t, "manifest.provisional", manifest.Provisional, len(provisional))
	seqAcceptanceExact(t, "manifest.excluded", manifest.Excluded, len(excluded))
	if len(all) != 68 || binding != 66 || len(provisional) != 1 || len(excluded) != 1 || len(provisional) != len(seqAcceptanceProvisional) || len(excluded) != len(seqAcceptanceExcluded) {
		t.Fatal("reviewed 68/66/1/1 case inventory or named nonbinding records changed")
	}
	if t.Failed() {
		t.Fatal("corpus identity/integrity validation failed; no engine run performed")
	}
	t.Logf("verified independent corpus: commit=%s corpus=%s; %d binding cases, %d provisional and %d excluded records", sources.Commit, manifest.CorpusSHA256, binding, len(provisional), len(excluded))
	return all
}
func seqAcceptanceDigest(s string, n int) bool {
	raw, err := hex.DecodeString(s)
	return err == nil && len(raw) == n && s == strings.ToLower(s)
}
func seqAcceptanceSafePath(s string) bool {
	return s != "" && !strings.Contains(s, "\\") && !strings.HasPrefix(s, "/") && filepath.ToSlash(filepath.Clean(s)) == s && s != "." && !strings.HasPrefix(s, "../")
}
func seqAcceptanceReadJSON(t *testing.T, raw []byte, dst any) {
	t.Helper()
	if err := seqAcceptanceDecode(raw, dst); err != nil {
		t.Fatalf("strict corpus schema: %v", err)
	}
}

// DisallowUnknownFields alone accepts duplicate keys, missing fields, and null
// numeric values. Check all three explicitly, recursively. Optional fields are
// marked omitempty; required nullable fields have pointer types without it.
func seqAcceptanceDecode(raw []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := seqAcceptanceJSONValue(d); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing JSON data")
	}
	typ := reflect.TypeOf(dst)
	if typ == nil || typ.Kind() != reflect.Pointer {
		return errors.New("decode destination must be pointer")
	}
	if err := seqAcceptanceRequired(raw, typ.Elem(), "$"); err != nil {
		return err
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(dst)
}
func seqAcceptanceJSONValue(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return errors.New("object key is not string")
			}
			if seen[name] {
				return fmt.Errorf("duplicate JSON key %q", name)
			}
			seen[name] = true
			if err := seqAcceptanceJSONValue(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := seqAcceptanceJSONValue(d); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected delimiter %q", delim)
	}
	_, err = d.Token()
	return err
}
func seqAcceptanceRequired(raw []byte, typ reflect.Type, path string) error {
	if typ == reflect.TypeOf(json.RawMessage{}) {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("%s: null object", path)
		}
		return nil
	}
	if typ.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil
		}
		return seqAcceptanceRequired(raw, typ.Elem(), path)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("%s: null is not allowed", path)
	}
	switch typ.Kind() {
	case reflect.Struct:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
		allowed := map[string]bool{}
		for i := 0; i < typ.NumField(); i++ {
			allowed[strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]] = true
		}
		for key := range fields {
			if !allowed[key] {
				return fmt.Errorf("%s.%s: unknown field", path, key)
			}
		}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			parts := strings.Split(f.Tag.Get("json"), ",")
			name := parts[0]
			value, ok := fields[name]
			if !ok {
				if len(parts) > 1 && parts[1] == "omitempty" {
					continue
				}
				return fmt.Errorf("%s.%s: missing required field", path, name)
			}
			if len(parts) > 1 && parts[1] == "omitempty" && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return fmt.Errorf("%s.%s: optional field must be omitted rather than null", path, name)
			}
			if err := seqAcceptanceRequired(value, f.Type, path+"."+name); err != nil {
				return err
			}
		}
	case reflect.Slice:
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return err
		}
		for i, v := range values {
			if err := seqAcceptanceRequired(v, typ.Elem(), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case reflect.Map:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
		keys := make([]string, 0, len(fields))
		for k := range fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := seqAcceptanceRequired(fields[k], typ.Elem(), path+"."+k); err != nil {
				return err
			}
		}
	}
	return nil
}
func TestSequentialAcceptanceAdapterGuards(t *testing.T) {
	guardEnvelope := seqAcceptanceEnvelope{Schema: "dsl-conformance-trades-v1", Case: "fixture.long", StrategyID: "fixture", Symbol: "SYN", Timeframe: "5m", RangeMethod: "zone", HigherTimeframe: "", Costs: engine.Costs{FillOn: "open", StartEquity: 10000}}
	guardResult := engine.RunResult{Schema: guardEnvelope.Schema, Case: guardEnvelope.Case, StrategyID: guardEnvelope.StrategyID, Symbol: guardEnvelope.Symbol, Timeframe: guardEnvelope.Timeframe, RangeMethod: guardEnvelope.RangeMethod, HigherTimeframe: guardEnvelope.HigherTimeframe, Costs: guardEnvelope.Costs}
	if problems := seqAcceptanceEnvelopeProblems(guardResult, guardEnvelope, false); len(problems) != 0 {
		t.Fatalf("valid result envelope rejected: %v", problems)
	}
	if problems := seqAcceptanceEnvelopeProblems(engine.RunResult{}, guardEnvelope, true); len(problems) != 0 {
		t.Fatalf("empty typed-refusal result rejected: %v", problems)
	}
	mutations := []struct {
		name  string
		apply func(*engine.RunResult)
	}{
		{"Schema", func(r *engine.RunResult) { r.Schema = "wrong" }},
		{"Case", func(r *engine.RunResult) { r.Case = "wrong" }},
		{"StrategyID", func(r *engine.RunResult) { r.StrategyID = "wrong" }},
		{"Symbol", func(r *engine.RunResult) { r.Symbol = "wrong" }},
		{"Timeframe", func(r *engine.RunResult) { r.Timeframe = "wrong" }},
		{"RangeMethod", func(r *engine.RunResult) { r.RangeMethod = "wrong" }},
		{"HigherTimeframe", func(r *engine.RunResult) { r.HigherTimeframe = "wrong" }},
		{"Costs.FeePerUnit", func(r *engine.RunResult) { r.Costs.FeePerUnit = 1 }},
		{"Costs.FillOn", func(r *engine.RunResult) { r.Costs.FillOn = "close" }},
		{"Costs.Slippage", func(r *engine.RunResult) { r.Costs.Slippage = 1 }},
		{"Costs.SlippageBps", func(r *engine.RunResult) { r.Costs.SlippageBps = 1 }},
		{"Costs.StartEquity", func(r *engine.RunResult) { r.Costs.StartEquity = 1 }},
		{"TimedAudit", func(r *engine.RunResult) { r.TimedAudit = &engine.TimedReturnAudit{} }},
		{"ClockRangeDays", func(r *engine.RunResult) { r.ClockRangeDays = []engine.ClockRangeDay{} }},
	}
	for _, mutation := range mutations {
		actual := guardResult
		mutation.apply(&actual)
		if len(seqAcceptanceEnvelopeProblems(actual, guardEnvelope, false)) == 0 {
			t.Errorf("result envelope mutation passed: %s", mutation.name)
		}
		partial := engine.RunResult{}
		mutation.apply(&partial)
		if len(seqAcceptanceEnvelopeProblems(partial, guardEnvelope, true)) == 0 {
			t.Errorf("typed refusal leaked partial result: %s", mutation.name)
		}
	}
	for _, partial := range []engine.RunResult{{Trades: []engine.Trade{}}, {TradeCount: 1}, {SequentialFull: &engine.SequentialFullAudit{}}} {
		if len(seqAcceptanceEnvelopeProblems(partial, guardEnvelope, true)) == 0 {
			t.Errorf("typed refusal leaked result: %+v", partial)
		}
	}
	for _, input := range []struct{ size, entry, want float64 }{{2, -3, 6}, {2, 3, 6}, {2, 0, 0}} {
		if got := seqAcceptanceNotional(input.size, input.entry); got != input.want {
			t.Errorf("absolute notional(%g,%g)=%g, want %g", input.size, input.entry, got, input.want)
		}
	}
	if seqAcceptanceClose(0, 1e-20) || seqAcceptanceClose(1e-12, 1.01e-12) || seqAcceptanceClose(math.NaN(), 0) || seqAcceptanceClose(math.Inf(1), math.Inf(1)) {
		t.Fatal("relative comparison must reject nonfinite values and must not have an absolute floor")
	}
	if !seqAcceptanceClose(0, 0) || !seqAcceptanceClose(1e-12, 1e-12*(1+0.5e-9)) {
		t.Fatal("relative comparison rejects valid agreement")
	}
	for _, raw := range []string{`{}`, `{"value":null}`, `{"value":1,"value":1}`, `{"value":1,"unexpected":0}`, `{"value":1,"Value":2}`, `{"value":1} {"value":1}`} {
		var dst struct {
			Value float64 `json:"value"`
		}
		if err := seqAcceptanceDecode([]byte(raw), &dst); err == nil {
			t.Errorf("malformed schema accepted: %s", raw)
		}
	}
	cfg := seqAcceptanceConfig{Policy: "E1", Slippage: 0.5, Risk: 90, MaxNotional: 1e6, ATRLength: 14, StopBuffer: 0.1, RewardRisk: 2, Hold: 4}
	costs, err := seqAcceptanceCosts("E1", cfg)
	if err != nil || costs.Slippage != cfg.Slippage {
		t.Fatalf("shared slippage was rejected or changed: %+v, %v", costs, err)
	}
	cfg.Slippage = -0.5
	if _, err := seqAcceptanceCosts("E1", cfg); err == nil {
		t.Fatal("negative shared slippage silently admitted")
	}
	boolPointer := func(value bool) *bool { return &value }
	stringPointer := func(value string) *string { return &value }
	for _, c := range []seqAcceptanceCase{
		{ID: "atr.warmup_boundary", Policy: "both"},
		{ID: "e1.target_basic", Policy: "E1", AllowNonpositivePrices: boolPointer(false)},
		{ID: "e1.cap_fill_zero", Policy: "E1", AddedIn: stringPointer("v1.2"), AllowNonpositivePrices: boolPointer(true)},
		{ID: "e1.cap_fill_negative", Policy: "E1", AddedIn: stringPointer("v1.2"), AllowNonpositivePrices: boolPointer(true)},
		{ID: "e1.cap_binds_reduces_size", Policy: "E1", PreviousID: stringPointer("pv.cap_binds_reduces_size"), AddedIn: stringPointer("v1.2 (promoted)"), AllowNonpositivePrices: boolPointer(false)},
		{ID: "e1.cap_equal_not_binding", Policy: "E1", PreviousID: stringPointer("pv.cap_equal_not_binding"), AddedIn: stringPointer("v1.2 (promoted)"), AllowNonpositivePrices: boolPointer(false)},
	} {
		if err := seqAcceptanceMetadata(c); err != nil {
			t.Errorf("reviewed metadata rejected: %v", err)
		}
		bad := c
		bad.AddedIn = stringPointer("unreviewed revision")
		if seqAcceptanceMetadata(bad) == nil {
			t.Errorf("changed revision metadata accepted: %s", c.ID)
		}
		bad = c
		bad.AllowNonpositivePrices = boolPointer(c.AllowNonpositivePrices == nil || !*c.AllowNonpositivePrices)
		if seqAcceptanceMetadata(bad) == nil {
			t.Errorf("changed price assumption accepted: %s", c.ID)
		}
	}
	legacy := []byte(`{"entry_slippage":0.5,"exit_slippage":0,"risk_amount":90,"max_notional":1000000,"atr_length":14,"stop_buffer_atr":0.1,"reward_risk":2,"policy":"E1","time_exit_bars":4}`)
	if err := seqAcceptanceDecode(legacy, &cfg); err == nil {
		t.Fatal("legacy split-cost schema silently admitted")
	}
}

func seqAcceptanceMetadata(c seqAcceptanceCase) error {
	previous, added, nonpositive := "", "", false
	switch c.ID {
	case "e1.cap_binds_reduces_size":
		previous, added = "pv.cap_binds_reduces_size", "v1.2 (promoted)"
	case "e1.cap_equal_not_binding":
		previous, added = "pv.cap_equal_not_binding", "v1.2 (promoted)"
	case "e1.cap_fill_zero", "e1.cap_fill_negative":
		added, nonpositive = "v1.2", true
	}
	if (c.PreviousID != nil) != (previous != "") || (c.PreviousID != nil && *c.PreviousID != previous) || (c.AddedIn != nil) != (added != "") || (c.AddedIn != nil && *c.AddedIn != added) {
		return fmt.Errorf("%s: unreviewed revision metadata", c.ID)
	}
	if c.Policy == "both" {
		if c.AllowNonpositivePrices != nil {
			return fmt.Errorf("%s: unexpected ATR metadata", c.ID)
		}
	} else if c.AllowNonpositivePrices == nil || *c.AllowNonpositivePrices != nonpositive {
		return fmt.Errorf("%s: unreviewed nonpositive-price metadata", c.ID)
	}
	return nil
}
