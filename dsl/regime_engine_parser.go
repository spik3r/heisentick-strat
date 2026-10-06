package dsl

import (
	"fmt"
	"strings"
)

// The scanner and token cursor are shared strict infrastructure, not the
// permissive legacy parser. This grammar consumes every token independently.
type regimeSourceParser struct {
	frozenSourceParser
	mode string
}

func (p *regimeSourceParser) directive(section string) error {
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
		return p.words("type", ":", "regime", "engine")
	}
	if err := p.word("regime"); err != nil {
		return err
	}
	key, err := p.value(false)
	if err != nil {
		return err
	}
	key = strings.ToLower(key)
	expected := map[string]string{"timeframe": "market", "profile": "setup", "mode": "setup"}
	if expected[key] != section {
		return fmt.Errorf("unknown or misplaced regime %s directive in %s", key, section)
	}
	if err := p.once(key); err != nil {
		return err
	}
	switch key {
	case "timeframe":
		return p.words("M30", "from", "M5")
	case "profile":
		value, err := p.value(false)
		if err != nil {
			return err
		}
		if value != "v9-floor-half-v1" {
			return fmt.Errorf("regime profile must equal v9-floor-half-v1")
		}
	case "mode":
		p.mode, err = p.value(false)
		return err
	}
	return nil
}

func (p *regimeSourceParser) parse() (Config, error) {
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
			name, err := p.value(true)
			if err != nil {
				return nil, err
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
		if err := p.word("{"); err != nil {
			return nil, err
		}
		for p.at < len(p.tokens) && (p.tokens[p.at].quoted || p.tokens[p.at].text != "}") {
			if err := p.directive(section); err != nil {
				return nil, err
			}
		}
		if err := p.word("}"); err != nil {
			return nil, err
		}
	}
	for _, section := range []string{"strategy", "market", "setup"} {
		if !blocks[section] {
			return nil, fmt.Errorf("missing block %s", section)
		}
	}
	for _, key := range []string{"type", "profile", "mode", "timeframe"} {
		if !p.seen[key] {
			return nil, fmt.Errorf("missing directive %s", key)
		}
	}
	p.cfg["regimeEngine"] = (RegimeEngineSpec{Mode: p.mode}).projection()
	if _, err := DecodeRegimeEngine(p.cfg); err != nil {
		return nil, err
	}
	return p.cfg, nil
}

func parseRegimeEngineSource(source string) ParseResult {
	result := ParseResult{Config: Config{}, Errors: []string{}, Warnings: []string{}, Diagnostics: []Diagnostic{}}
	tokens, err := scanFrozenSource(source)
	if err == nil {
		p := regimeSourceParser{frozenSourceParser: frozenSourceParser{tokens: tokens, seen: map[string]bool{}, cfg: Config{"dslVersion": int64(7), "setupType": string(FamilyRegimeEngine), "description": ""}}}
		result.Config, err = p.parse()
	}
	if err != nil {
		result.Config = Config{}
		message := "regime engine: " + err.Error()
		result.Errors = append(result.Errors, message)
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: DiagnosticError, Message: message})
		return result
	}
	result.Warnings = append(result.Warnings, RegimeEngineDedicatedRunnerRequired)
	result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: DiagnosticWarning, Message: RegimeEngineDedicatedRunnerRequired})
	return result
}
