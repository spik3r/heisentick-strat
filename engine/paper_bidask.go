package engine

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const PaperBidAskSchema = "heisentick-paper-bidask-v1"
const PaperBidAskRequestSchema = "heisentick-paper-bidask-request-v1"
const PaperBidAskModel = "sampled-straddle-full-bar-stop-first-v1"

type BidAskIdentity struct {
	ArtifactDigest  string `json:"artifactDigest"`
	PrincipalDigest string `json:"principalDigest"`
	AccountDigest   string `json:"accountDigest"`
	RunID           string `json:"runId"`
	Epoch           string `json:"epoch"`
}
type BidAskProduct struct {
	Provider          string        `json:"provider"`
	Instrument        string        `json:"instrument"`
	AttestationDigest string        `json:"attestationDigest"`
	AccountCurrency   string        `json:"accountCurrency"`
	PriceUnit         string        `json:"priceUnit"`
	QuantityUnit      string        `json:"quantityUnit"`
	Multiplier        BidAskDecimal `json:"multiplier"`
}
type BidAskEconomics struct {
	StartEquity     BidAskDecimal `json:"startEquity"`
	EntryFee        BidAskDecimal `json:"entryFee"`
	ExitFee         BidAskDecimal `json:"exitFee"`
	SlippageFixed   BidAskDecimal `json:"slippageFixed"`
	SlippageBPS     BidAskDecimal `json:"slippageBps"`
	QuantityQuantum BidAskDecimal `json:"quantityQuantum"`
	QuantityMinimum BidAskDecimal `json:"quantityMinimum"`
	QuantityMaximum BidAskDecimal `json:"quantityMaximum"`
	NotionalPolicy  string        `json:"notionalPolicy"`
	NotionalMaximum BidAskDecimal `json:"notionalMaximum"`
}
type BidAskPolicy struct {
	Model          string `json:"model"`
	MaxQuoteAgeMS  string `json:"maxQuoteAgeMs"`
	MaxSignalAgeMS string `json:"maxSignalAgeMs"`
	Coverage       string `json:"coverage"`
	Arithmetic     string `json:"arithmetic"`
}
type PaperBidAskRequest struct {
	Schema          string          `json:"schema"`
	Source          string          `json:"source"`
	Route           PaperRoute      `json:"route"`
	Identity        BidAskIdentity  `json:"identity"`
	Product         BidAskProduct   `json:"product"`
	Economics       BidAskEconomics `json:"economics"`
	Policy          BidAskPolicy    `json:"policy"`
	ActivationFromT string          `json:"activationFromT"`
	Records         []BidAskRecord  `json:"records"`
	Checkpoint      json.RawMessage `json:"checkpoint,omitempty"`
}
type BidAskBinding struct {
	Family          string                             `json:"family"`
	Profile         string                             `json:"profile"`
	SignalRecipe    string                             `json:"signalRecipe"`
	SizingPolicy    string                             `json:"sizingPolicy"`
	Contract        string                             `json:"contract"`
	Source          PaperSourceIdentity                `json:"source"`
	ConfigIdentity  SequentialInspectionConfigIdentity `json:"configIdentity"`
	Route           PaperRoute                         `json:"route"`
	Identity        BidAskIdentity                     `json:"identity"`
	Product         BidAskProduct                      `json:"product"`
	Economics       BidAskEconomics                    `json:"economics"`
	Policy          BidAskPolicy                       `json:"policy"`
	ActivationFromT string                             `json:"activationFromT"`
	Producer        SequentialInspectionProducer       `json:"producer"`
}
type BidAskClock struct {
	SignalOpenT     string `json:"signalOpenT"`
	SignalCloseT    string `json:"signalCloseT"`
	ProviderT       string `json:"providerT"`
	RequestStartT   string `json:"requestStartT"`
	ReceivedT       string `json:"receivedT"`
	QualifiedT      string `json:"qualifiedT"`
	CommitT         string `json:"commitT"`
	ModelEffectiveT string `json:"modelEffectiveT"`
	IntervalOpenT   string `json:"intervalOpenT"`
	IntervalCloseT  string `json:"intervalCloseT"`
}
type BidAskEvent struct {
	ID                     string      `json:"id"`
	RecordSequence         string      `json:"recordSequence"`
	Kind                   string      `json:"kind"`
	Outcome                string      `json:"outcome"`
	DecisionID             string      `json:"decisionId"`
	PositionID             string      `json:"positionId"`
	Side                   string      `json:"side"`
	Price                  string      `json:"price"`
	Quantity               string      `json:"quantity"`
	Fee                    string      `json:"fee"`
	NetPnL                 string      `json:"netPnl"`
	Stop                   string      `json:"stop"`
	Target                 string      `json:"target"`
	ProtectionVersion      int         `json:"protectionVersion"`
	PriorProtectionVersion int         `json:"priorProtectionVersion"`
	Basis                  string      `json:"basis"`
	Clock                  BidAskClock `json:"clock"`
}
type BidAskPosition struct {
	Phase                     string `json:"phase"`
	ExpectedFullIntervalOpenT string `json:"expectedFullIntervalOpenT"`
	ID                        string `json:"id"`
	DecisionID                string `json:"decisionId"`
	Side                      string `json:"side"`
	Entry                     string `json:"entry"`
	Quantity                  string `json:"quantity"`
	Stop                      string `json:"stop"`
	Target                    string `json:"target"`
	RiskBudget                string `json:"riskBudget"`
	StopBudgetLoss            string `json:"stopBudgetLoss"`
	EntryFee                  string `json:"entryFee"`
	ProtectionVersion         int    `json:"protectionVersion"`
	SignalIndex               int    `json:"signalIndex"`
	EntryProviderT            string `json:"entryProviderT"`
	FullPhaseFromT            string `json:"fullPhaseFromT"`
}
type BidAskPending struct {
	DecisionID     string `json:"decisionId"`
	SignalSequence string `json:"signalSequence"`
	State          string `json:"state"`
	DeadlineT      string `json:"deadlineT"`
}
type PaperBidAskResult struct {
	Schema           string           `json:"schema"`
	Mode             string           `json:"mode"`
	Binding          BidAskBinding    `json:"binding"`
	BindingDigest    string           `json:"bindingDigest"`
	InputDigest      string           `json:"inputDigest"`
	ProcessedRecords int              `json:"processedRecords"`
	ProcessedBars    int              `json:"processedBars"`
	EventRange       PaperEventRange  `json:"eventRange"`
	Events           []BidAskEvent    `json:"events"`
	Account          BidAskAccount    `json:"account"`
	CompletedTrades  BidAskStatistics `json:"completedTrades"`
	Position         *BidAskPosition  `json:"position,omitempty"`
	Pending          *BidAskPending   `json:"pending,omitempty"`
	Coverage         string           `json:"coverage"`
	StateDigest      string           `json:"stateDigest"`
	Checkpoint       json.RawMessage  `json:"checkpoint"`
}
type PaperBidAskError struct {
	Code string `json:"code"`
}

func (e *PaperBidAskError) Error() string { return e.Code }
func bidAskError(code string) error       { return &PaperBidAskError{Code: "PAPER_BIDASK_" + code} }
func bidAskHash(v any) string {
	s, e := paperDigest(v)
	if e != nil {
		return ""
	}
	return s
}

// The shared guard audits the exact authored source and its unmodified ordinary
// Parse config. This model further narrows its occurrence receipt; a later off
// or family reset must not erase management that this model cannot execute.
func bidAskAdmitSource(source string, cfg dsl.Config, route PaperRoute) error {
	features, err := validatePaperORB(source, cfg, route)
	if err != nil {
		if err.Error() == "PAPER_PREFIX_ROUTE_UNSUPPORTED" {
			return bidAskError("ROUTE_UNSUPPORTED")
		}
		return bidAskError("ADMISSION")
	}
	if features.HTF || features.SourceEntry {
		return bidAskError("CONTEXT_UNSUPPORTED")
	}
	if features.Partial {
		return bidAskError("PARTIAL_UNQUALIFIED")
	}
	if features.Trailing {
		return bidAskError("TRAILING_UNQUALIFIED")
	}
	return nil
}

func prepareBidAsk(r PaperBidAskRequest, admitSource bool) (*PreparedRun, BidAskBinding, error) {
	var binding BidAskBinding
	if r.Schema != PaperBidAskRequestSchema || !utf8.ValidString(r.Source) || len(r.Source) == 0 || len(r.Source) > PaperBidAskMaxSourceBytes || len(r.Records) > PaperBidAskMaxRecords {
		return nil, binding, bidAskError("INPUT")
	}
	if !bidAskDigestValid(r.Identity.ArtifactDigest) || !bidAskDigestValid(r.Identity.PrincipalDigest) || !bidAskDigestValid(r.Identity.AccountDigest) || !bidAskDigestValid(r.Identity.RunID) || !bidAskDigestValid(r.Identity.Epoch) || !bidAskDigestValid(r.Product.AttestationDigest) {
		return nil, binding, bidAskError("IDENTITY")
	}
	if r.Product.Provider != "oanda-v20" || r.Product.Instrument != "XAU_USD" || r.Product.AccountCurrency != "USD" || r.Product.PriceUnit != "USD/troy-ounce" || r.Product.QuantityUnit != "troy-ounce" || r.Product.Multiplier != "1" || r.Route.Symbol != "XAUUSD" {
		return nil, binding, bidAskError("PRODUCT_UNSUPPORTED")
	}
	if r.Route.StrategyID == "" || len(r.Route.StrategyID) > 128 || (r.Route.RangeMethod != "zone" && r.Route.RangeMethod != "pivot") {
		return nil, binding, bidAskError("ROUTE")
	}
	duration := paperTimeframeMS(r.Route.Timeframe) * 1000000
	activation, e := bidAskTime(r.ActivationFromT)
	if e != nil || duration <= 0 || activation%duration != 0 {
		return nil, binding, bidAskError("ACTIVATION")
	}
	maxQuote, e := strconv.ParseInt(r.Policy.MaxQuoteAgeMS, 10, 64)
	if e != nil || maxQuote <= 0 || maxQuote > 60000 || strconv.FormatInt(maxQuote, 10) != r.Policy.MaxQuoteAgeMS {
		return nil, binding, bidAskError("POLICY")
	}
	maxSignal, e := strconv.ParseInt(r.Policy.MaxSignalAgeMS, 10, 64)
	if e != nil || maxSignal <= 0 || maxSignal > duration/1000000 || strconv.FormatInt(maxSignal, 10) != r.Policy.MaxSignalAgeMS {
		return nil, binding, bidAskError("POLICY")
	}
	if r.Policy.Model != PaperBidAskModel || r.Policy.Coverage != "refuse-discontinuous-exposure" || r.Policy.Arithmetic != "decimal-rational-family-shortest-v1" {
		return nil, binding, bidAskError("POLICY")
	}
	ec := r.Economics
	for _, v := range []BidAskDecimal{ec.StartEquity, ec.EntryFee, ec.ExitFee, ec.SlippageFixed, ec.SlippageBPS, ec.QuantityQuantum, ec.QuantityMinimum, ec.QuantityMaximum, ec.NotionalMaximum} {
		if _, e := bidAskNumber(v); e != nil {
			return nil, binding, e
		}
	}
	start, _ := bidAskNumber(ec.StartEquity)
	quantum, _ := bidAskNumber(ec.QuantityQuantum)
	minimum, _ := bidAskNumber(ec.QuantityMinimum)
	maximum, _ := bidAskNumber(ec.QuantityMaximum)
	notional, _ := bidAskNumber(ec.NotionalMaximum)
	if start.Sign() <= 0 || quantum.Sign() <= 0 || minimum.Sign() <= 0 || maximum.Cmp(minimum) < 0 || !newRatQuo(minimum, quantum).IsInt() || !newRatQuo(maximum, quantum).IsInt() || !((ec.NotionalPolicy == "none" && notional.Sign() == 0) || (ec.NotionalPolicy == "cap" && notional.Sign() > 0)) {
		return nil, binding, bidAskError("ECONOMICS")
	}
	bars, e := validateBidAskRecords(r, duration)
	if e != nil {
		return nil, binding, e
	}
	if len(r.Checkpoint) > 4096 {
		return nil, binding, bidAskError("CHECKPOINT")
	}
	rawRequest, marshalErr := json.Marshal(r)
	if marshalErr != nil || len(rawRequest) > PaperBidAskMaxBytes {
		return nil, binding, bidAskError("INPUT")
	}
	parsed, e := dsl.Parse(r.Source)
	if e != nil || len(parsed.Errors) != 0 {
		return nil, binding, bidAskError("SOURCE")
	}
	if setupTypeFromAny(parsed.Config["setupType"]) != string(dsl.FamilyOpeningRangeBreakout) {
		return nil, binding, bidAskError("FAMILY_UNQUALIFIED")
	}
	if admitSource {
		if e := bidAskAdmitSource(r.Source, parsed.Config, r.Route); e != nil {
			return nil, binding, e
		}
	}
	htf := mapValue(parsed.Config, "htf")
	mode, _ := htf["mode"].(string)
	if sourceTimeframeFromConfig(parsed.Config) != "" || (mode != "" && mode != "off") {
		return nil, binding, bidAskError("CONTEXT_UNSUPPORTED")
	}
	params := paramsFromConfig(parsed.Config)
	if params.Partial.Enabled {
		return nil, binding, bidAskError("PARTIAL_UNQUALIFIED")
	}
	if params.Trail.ATR > 0 {
		return nil, binding, bidAskError("TRAILING_UNQUALIFIED")
	}
	if !isFinite(params.RiskUSD) || params.RiskUSD <= 0 {
		return nil, binding, bidAskError("RISK")
	}
	configHash := bidAskHash(parsed.Config)
	if configHash == "" {
		return nil, binding, bidAskError("SOURCE_NUMERIC")
	}
	binding = BidAskBinding{Family: "openingRangeBreakout", Profile: "ordinary", SignalRecipe: "provider-midpoint-ohlc-price-count-v1", SizingPolicy: "source-risk-usd-total-stop-budget-v1", Contract: PaperBidAskSchema, Source: PaperSourceIdentity{SHA256: paperSourceDigest(r.Source), UTF8Bytes: len(r.Source)}, ConfigIdentity: SequentialInspectionConfigIdentity{SHA256: configHash, Encoding: SequentialInspectionConfigEncoding}, Route: r.Route, Identity: r.Identity, Product: r.Product, Economics: r.Economics, Policy: r.Policy, ActivationFromT: r.ActivationFromT, Producer: sequentialInspectionProducer()}
	if len(bars) == 0 {
		return nil, binding, nil
	}
	series := marketdata.NewSeries(len(bars))
	for i, b := range bars {
		ns, _ := bidAskTime(b.OpenT)
		series.T[i] = float64(ns / 1000000)
		series.O[i], _ = bidAskFloat(b.Mid.Open)
		series.H[i], _ = bidAskFloat(b.Mid.High)
		series.L[i], _ = bidAskFloat(b.Mid.Low)
		series.C[i], _ = bidAskFloat(b.Mid.Close)
		series.V[i], _ = strconv.ParseFloat(b.PriceCount, 64)
	}
	p, e := PrepareRun(RunRequest{Config: parsed.Config, Series: series, StrategyID: r.Route.StrategyID, Symbol: r.Route.Symbol, Timeframe: r.Route.Timeframe, RangeMethod: r.Route.RangeMethod, Costs: Costs{StartEquity: 1000, FillOn: "close"}})
	if e != nil || p.offRoute || p.c5 || p.sequentialFull != nil || p.params.UseHTFBias {
		return nil, binding, bidAskError("ADMISSION")
	}
	return p, binding, nil
}
func RunPaperBidAsk(r PaperBidAskRequest) (PaperBidAskResult, error) {
	p, b, e := prepareBidAsk(r, true)
	if e != nil {
		return PaperBidAskResult{}, e
	}
	return replayBidAsk(r, p, b)
}
func EncodePaperBidAskResult(r PaperBidAskResult) ([]byte, error) {
	b, e := json.Marshal(r)
	if e != nil {
		return nil, bidAskError("OUTPUT")
	}
	if len(b) > PaperBidAskMaxBytes {
		return nil, bidAskError("OUTPUT_LIMIT")
	}
	return b, nil
}
func bidAskTrimDecimal(s string) string {
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "-0" {
		return "0"
	}
	return s
}
