package dsl

import (
	"encoding/json"
	"fmt"
	"strings"
)

type adaptiveFlagSourceParser struct {
	frozenSourceParser
	rules                     map[string]any
	policy, timeframe, bundle string
}

var adaptiveFlagRuleTypes = map[string]string{
	"pivotSensitivity": "int", "minPoleATR": "decimal", "minFlagBars": "int", "maxFlagBars": "int", "maxFlagRetrace": "decimal",
	"useVolumeFilter": "bool", "useEMATrend": "bool", "fastEMALen": "int", "slowEMALen": "int", "targetR": "decimal",
	"atrStopMult": "decimal", "validBars": "int", "maxHold": "int", "atrLen": "int", "volumeSMALen": "int", "volumeSMAMult": "decimal", "flagWidthPoleMult": "decimal", "entryBufferATR": "decimal",
}

func (p *adaptiveFlagSourceParser) directive(section string) error {
	if section == "strategy" {
		if err := p.word("description"); err != nil {
			return err
		}
		if err := p.once("description"); err != nil {
			return err
		}
		v, err := p.value(true)
		p.cfg["description"] = v
		return err
	}
	if section == "setup" && p.at < len(p.tokens) && !p.tokens[p.at].quoted && strings.EqualFold(p.tokens[p.at].text, "type") {
		if err := p.once("type"); err != nil {
			return err
		}
		return p.words("type", ":", "adaptive", "volume", "flag")
	}
	if err := p.word("adaptiveflag"); err != nil {
		return err
	}
	key, err := p.value(false)
	if err != nil {
		return err
	}
	key = strings.ToLower(key)
	if err = p.once(key); err != nil {
		return err
	}
	if key == "timeframe" && section == "market" {
		p.timeframe, err = p.value(false)
		p.timeframe = strings.ToUpper(p.timeframe)
		return err
	}
	if section != "setup" {
		return fmt.Errorf("unknown or misplaced adaptiveflag %s in %s", key, section)
	}
	switch key {
	case "policy":
		p.policy, err = p.value(false)
		return err
	case "bundle":
		p.bundle, err = p.value(false)
		return err
	}
	for field, kind := range adaptiveFlagRuleTypes {
		if !strings.EqualFold(field, key) {
			continue
		}
		token, e := p.value(false)
		if e != nil {
			return e
		}
		if kind == "bool" {
			if !strings.EqualFold(token, "true") && !strings.EqualFold(token, "false") {
				return fmt.Errorf("%s requires true or false", field)
			}
			p.rules[field] = strings.EqualFold(token, "true")
			return nil
		}
		if (kind == "int" && !frozenDSLInteger.MatchString(token)) || (kind == "decimal" && !frozenDSLDecimal.MatchString(token)) {
			return fmt.Errorf("invalid %s for %s", kind, field)
		}
		if strings.HasPrefix(token, ".") {
			token = "0" + token
		}
		p.rules[field] = json.Number(token)
		return nil
	}
	return fmt.Errorf("unknown adaptiveflag directive %s", key)
}

func (p *adaptiveFlagSourceParser) parse() (Config, error) {
	if err := p.words("dsl", "v7"); err != nil {
		return nil, err
	}
	blocks := map[string]bool{}
	for p.at < len(p.tokens) {
		section, err := p.value(false)
		if err != nil {
			return nil, err
		}
		section = strings.ToLower(section)
		switch section {
		case "strategy":
			name, e := p.value(true)
			if e != nil {
				return nil, e
			}
			p.cfg["name"] = name
		case "market", "setup":
		default:
			return nil, fmt.Errorf("unknown block %q", section)
		}
		if blocks[section] {
			return nil, fmt.Errorf("duplicate block %s", section)
		}
		blocks[section] = true
		if err = p.word("{"); err != nil {
			return nil, err
		}
		for p.at < len(p.tokens) && (p.tokens[p.at].quoted || p.tokens[p.at].text != "}") {
			if err = p.directive(section); err != nil {
				return nil, err
			}
		}
		if err = p.word("}"); err != nil {
			return nil, err
		}
	}
	for _, block := range []string{"strategy", "market", "setup"} {
		if !blocks[block] {
			return nil, fmt.Errorf("missing block %s", block)
		}
	}
	for _, key := range []string{"type", "policy", "timeframe", "bundle"} {
		if !p.seen[key] {
			return nil, fmt.Errorf("missing directive %s", key)
		}
	}
	p.cfg["adaptiveVolumeFlag"] = map[string]any{"contractVersion": adaptiveFlagContractVersion, "policy": p.policy, "numericalPolicy": AdaptiveFlagNumericalPolicy, "timeframe": p.timeframe, "bundle": p.bundle, "rules": p.rules}
	spec, err := DecodeAdaptiveVolumeFlag(p.cfg)
	if err != nil {
		return nil, err
	}
	return AdaptiveFlagConfig(p.cfg["name"].(string), p.cfg["description"].(string), spec.Timeframe, spec.Bundle, spec.Rules), nil
}

func parseAdaptiveVolumeFlagSource(source string) ParseResult {
	result := ParseResult{Config: Config{}, Errors: []string{}, Warnings: []string{}, Diagnostics: []Diagnostic{}}
	tokens, err := scanFrozenSource(source)
	if err == nil {
		p := adaptiveFlagSourceParser{frozenSourceParser: frozenSourceParser{tokens: tokens, seen: map[string]bool{}, cfg: Config{"dslVersion": int64(7), "setupType": string(FamilyAdaptiveVolumeFlag), "description": ""}}, rules: map[string]any{}}
		result.Config, err = p.parse()
	}
	if err != nil {
		result.Config = Config{}
		message := "adaptive volume flag: " + err.Error()
		result.Errors = append(result.Errors, message)
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: DiagnosticError, Message: message})
		return result
	}
	result.Warnings = append(result.Warnings, AdaptiveFlagDedicatedRunnerRequired)
	result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: DiagnosticWarning, Message: AdaptiveFlagDedicatedRunnerRequired})
	return result
}
