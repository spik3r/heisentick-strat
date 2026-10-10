package dsl

import (
	"math"
	"strings"
)

// Profile identifiers for the frozen legacy Setup-9 controls (HT-227). The
// profiles port two archived JavaScript strategies; a corrected variant is a
// new profile ID, never an edit of these.
const (
	LegacySetup9Profile             = "seq.legacy.setup9.v1"
	LegacySetup9PerfSeasonalProfile = "seq.legacy.setup9_perf_seasonal.v1"
)

// LegacySetup9Profiles lists the accepted profile IDs.
var LegacySetup9Profiles = []string{LegacySetup9Profile, LegacySetup9PerfSeasonalProfile}

type legacySetup9ParseAudit struct {
	authored      []string
	riskUsdTokens [][]string
	profileLines  int
}

func (p *parser) recordLegacySetup9Authored(line logicalLine, head string, tokens []string) {
	audit := &p.legacySetup9Audit
	audit.authored = append(audit.authored, line.section+"|"+head)
	if head == "riskusd" {
		audit.riskUsdTokens = append(audit.riskUsdTokens, append([]string(nil), tokens...))
	}
}

// parseLegacySetup9 handles `sequential profile <id>`. The profile closes every
// strategy knob, so this is the only family phrase.
func (p *parser) parseLegacySetup9(line logicalLine, tokens []string) {
	if p.config["setupType"] != string(FamilyLegacySetup9) {
		p.err(line, "sequential profile requires `type: legacy setup 9` before it", "")
		return
	}
	if line.section != "setup" || len(tokens) != 3 || !strings.EqualFold(tokens[1], "profile") {
		p.err(line, "unrecognized sequential line — use `sequential profile <profile id>`", "")
		return
	}
	p.legacySetup9Audit.profileLines++
	if p.legacySetup9Audit.profileLines > 1 {
		p.err(line, "sequential profile may be written only once", "")
		return
	}
	for _, id := range LegacySetup9Profiles {
		if strings.EqualFold(tokens[2], id) {
			cfg := copyMap(p.config["legacySetup9"])
			cfg["profile"] = id
			p.config["legacySetup9"] = cfg
			return
		}
	}
	p.err(line, "unknown sequential profile "+tokens[2]+" — known: "+strings.Join(LegacySetup9Profiles, ", "), "")
}

func (p *parser) validateLegacySetup9() {
	if p.config["setupType"] != string(FamilyLegacySetup9) {
		return
	}
	audit := p.legacySetup9Audit
	allowed := map[string]bool{
		"|dsl": true, "|strategy": true, "strategy|strategy": true, "strategy|description": true,
		"market|slices": true, "setup|type": true, "setup|sequential": true,
		"risk|riskusd": true, "setup|riskusd": true,
	}
	seen := map[string]bool{}
	for _, authored := range audit.authored {
		if allowed[authored] || seen[authored] {
			continue
		}
		seen[authored] = true
		p.errorAt(nil, nil, "legacy setup 9 does not support authored directive: "+strings.ReplaceAll(authored, "|", ":")+".", "")
	}
	if profile, _ := copyMap(p.config["legacySetup9"])["profile"].(string); profile == "" {
		p.errorAt(nil, nil, "legacy setup 9 requires `sequential profile "+strings.Join(LegacySetup9Profiles, "` or `sequential profile ")+"`.", "")
	}
	for _, tokens := range audit.riskUsdTokens {
		value, ok := 0.0, len(tokens) == 2
		if ok {
			value, ok = parseSMAFiniteNumber(tokens[1])
		}
		if !ok || value <= 0 || math.IsInf(value, 0) {
			p.errorAt(nil, nil, "legacy setup 9 riskUsd must be one finite positive number.", "")
		}
	}
}
