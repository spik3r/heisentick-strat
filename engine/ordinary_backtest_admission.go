package engine

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode"

	"github.com/spik3r/heisentick-strat/dsl"
)

// OrdinaryBacktestClassification is source/profile information. It never grants
// execution, replaces a dedicated contract, or approves a fixed source hash.
type OrdinaryBacktestClassification struct{ Kind, Family, Reason string }

// OrdinaryBacktestPreparation captures the reusable initial admission work.
// P1 deliberately has no admitted/execution-ready state: the later shared run
// validators and qualified accounting capture must still run. The parsed config
// remains private to engine so a caller cannot mutate it into an admission grant.
type OrdinaryBacktestPreparation struct {
	config         dsl.Config
	configSHA256   string
	classification OrdinaryBacktestClassification
	dataValidated  bool
}

func (p *OrdinaryBacktestPreparation) ConfigIdentity() string { return p.configSHA256 }
func (p *OrdinaryBacktestPreparation) Classification() OrdinaryBacktestClassification {
	return p.classification
}
func (p *OrdinaryBacktestPreparation) DataValidated() bool { return p.dataValidated }
func (p *OrdinaryBacktestPreparation) Preflight() string   { return "not-evaluated" }

// PrepareOrdinaryBacktestAdmission does not execute a strategy or assert full
// run admission. A non-nil preparation accompanying a later refusal preserves
// the actual parsed config digest for the future bound error envelope.
func PrepareOrdinaryBacktestAdmission(c *OrdinaryBacktestCapture) (*OrdinaryBacktestPreparation, error) {
	if c == nil {
		return nil, ordinaryFault("INTERNAL", "transport", "")
	}
	parsed, e := dsl.Parse(c.source)
	if e != nil || len(parsed.Errors) != 0 || dsl.SequentialFullParseError(parsed) != nil {
		return nil, ordinaryFault("PARSE_REJECTED", "parse", "")
	}
	raw, e := json.Marshal(parsed.Config)
	if e != nil {
		return nil, ordinaryFault("INTERNAL", "parse", "")
	}
	p := &OrdinaryBacktestPreparation{config: parsed.Config, configSHA256: ordinaryIdentity(raw).SHA256, classification: ordinaryClassifyConfig(parsed.Config)}
	if p.classification.Kind == "dedicated" {
		return p, ordinaryFault("DEDICATED_REQUIRED", "admission", "")
	}
	if p.classification.Kind != "ordinary" {
		return p, ordinaryFault("ACCOUNTING_UNAVAILABLE", "admission", "")
	}
	// Original-bar data are an explicit request profile, never inferred from a
	// symbol, calendar, gaps or a last-bar truncation. Specialized intent above is
	// checked even for text-only and off-route requests.
	if c.request.Data != nil {
		if e := ordinaryValidateData(c.request.Route.Timeframe, c.request.Data); e != nil {
			return p, e
		}
		p.dataValidated = true
	}
	return p, nil
}

func ordinaryClassifyConfig(cfg dsl.Config) OrdinaryBacktestClassification {
	family := setupTypeFromAny(cfg["setupType"])
	reserved := []struct {
		family string
		yes    bool
	}{
		{string(dsl.FamilySequentialFull), dsl.IsSequentialFullReserved(cfg)},
		{"goldFlagReference", dsl.IsGoldFlagReferenceReserved(cfg)},
		{string(dsl.FamilyAdaptiveVolumeFlag), dsl.IsAdaptiveVolumeFlagReserved(cfg)},
		{string(dsl.FamilyMasterStructural), dsl.IsMasterStructuralReserved(cfg)},
		{"regimeEngine", dsl.IsRegimeEngineReserved(cfg)},
		{string(dsl.FamilyRangeReversion), dsl.IsRangeReversionReserved(cfg)},
	}
	for _, r := range reserved {
		if r.yes {
			return OrdinaryBacktestClassification{"dedicated", r.family, "dedicated-contract-required"}
		}
	}
	if marker := ordinaryDedicatedLoopMarker(cfg, 0); marker != "" {
		return OrdinaryBacktestClassification{"dedicated", marker, "dedicated-contract-required"}
	}
	if sourceTimeframeFromConfig(cfg) != "" {
		return OrdinaryBacktestClassification{"unsupported", family, "original-profile-source-series-required"}
	}
	if htf := mapValue(cfg, "htf"); stringValue(htf, "mode", "off") != "off" {
		return OrdinaryBacktestClassification{"unsupported", family, "original-profile-higher-series-required"}
	}
	if !implementedFamily(family) {
		return OrdinaryBacktestClassification{"unsupported", family, "family-not-implemented"}
	}
	return OrdinaryBacktestClassification{"ordinary", family, "run-admission-and-accounting-not-evaluated"}
}

// These existing specialized loops do not share the ordinary observation
// lifecycle. Preserve their intent in nested maps, including case/punctuation
// aliases and hidden object keys. Arrays are not claimed as arbitrary config
// traversal; the parser and dedicated family validators retain their own rules.
func ordinaryDedicatedLoopMarker(value any, depth int) string {
	if depth > 32 {
		return "reserved-config-depth"
	}
	var m map[string]any
	switch v := value.(type) {
	case dsl.Config:
		m = map[string]any(v)
	case map[string]any:
		m = v
	default:
		return ""
	}
	normalize := func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return unicode.ToLower(r)
			}
			return -1
		}, s)
	}
	marker := func(s string) string {
		switch normalize(s) {
		case "clockrangebreakout":
			return "clockRangeBreakout"
		case "timedreturn":
			return "timedReturn"
		case "frozenlevelbreakout":
			return "frozenLevelBreakout"
		case "r11":
			return "r11"
		}
		return ""
	}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		child := m[key]
		if found := marker(key); found != "" {
			return found
		}
		if normalize(key) == "setuptype" {
			if found := marker(setupTypeFromAny(child)); found != "" {
				return found
			}
		}
		if found := ordinaryDedicatedLoopMarker(child, depth+1); found != "" {
			return found
		}
	}
	return ""
}

func ordinaryValidateData(timeframe string, d *OrdinaryBacktestData) error {
	// This is the producer's existing finite native-close duration table, reused
	// only as a duration definition, not as Sequential execution admission.
	duration, ok := dsl.SequentialFullTimeframeMS(timeframe)
	if !ok {
		return ordinaryFault("ROUTE_UNSUPPORTED", "admission", "route.timeframe")
	}
	if d == nil || len(d.Bars) == 0 {
		return ordinaryFault("DATA_INVALID", "admission", "data")
	}
	if d.FirstT != d.Bars[0][0] || d.LastT != d.Bars[len(d.Bars)-1][0] {
		return ordinaryFault("DATA_INVALID", "admission", "data")
	}
	for i, row := range d.Bars {
		if i > 0 && row[0] <= d.Bars[i-1][0] || row[2] < row[1] || row[2] < row[3] || row[2] < row[4] || row[3] > row[1] || row[3] > row[4] {
			e := ordinaryFault("DATA_INVALID", "admission", "data.bars")
			e.BarIndex = &i
			return e
		}
		if row[0] > ordinaryBacktestMaxInteger-float64(duration) || row[0]+float64(duration) > d.ClosedThroughT {
			e := ordinaryFault("FORMING_BAR", "admission", "data.bars")
			e.BarIndex = &i
			return e
		}
	}
	return nil
}
