package dsl

import (
	"fmt"
	"strings"
)

// The full family consumes every token through the shared strict scanner. It
// never inherits legacy parser risk, routing, management or strategy defaults.
type sequentialFullSourceParser struct {
	frozenSourceParser
	full map[string]any
}

func (p *sequentialFullSourceParser) directive(section string) error {
	if section == "strategy" {
		if err := p.words("description"); err != nil {
			return err
		}
		if err := p.once("description"); err != nil {
			return err
		}
		value, err := p.value(true)
		p.cfg["description"] = value
		return err
	}
	if section == "setup" && p.at < len(p.tokens) && !p.tokens[p.at].quoted && strings.EqualFold(p.tokens[p.at].text, "type") {
		if err := p.once("type"); err != nil {
			return err
		}
		return p.words("type", ":", "sequential", "full")
	}
	if err := p.word("sequential"); err != nil {
		return err
	}
	key, err := p.value(false)
	if err != nil {
		return err
	}
	key = strings.ToLower(key)
	expected := map[string]string{"symbol": "market", "timeframe": "market", "profile": "setup", "policy": "setup", "riskusd": "risk", "maxnotionalusd": "risk"}
	if expected[key] != section {
		return fmt.Errorf("unknown or misplaced sequential %s directive in %s", key, section)
	}
	if section == "setup" && !p.seen["type"] {
		return fmt.Errorf("type: sequential full must precede sequential %s", key)
	}
	if err := p.once(key); err != nil {
		return err
	}
	if key == "riskusd" || key == "maxnotionalusd" {
		value, err := p.number(false)
		field := "riskUsd"
		if key == "maxnotionalusd" {
			field = "maxNotionalUsd"
		}
		p.full[field] = value
		return err
	}
	value, err := p.value(false)
	p.full[key] = value
	return err
}

func (p *sequentialFullSourceParser) parse() (Config, error) {
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
		case "market", "setup", "risk":
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
	for _, section := range []string{"strategy", "market", "setup", "risk"} {
		if !blocks[section] {
			return nil, fmt.Errorf("missing block %s", section)
		}
	}
	for _, key := range []string{"type", "profile", "policy", "symbol", "timeframe", "riskusd", "maxnotionalusd"} {
		if !p.seen[key] {
			return nil, fmt.Errorf("missing directive %s", key)
		}
	}
	p.cfg["sequentialFull"] = p.full
	if _, err := DecodeSequentialFullConfig(p.cfg); err != nil {
		return nil, err
	}
	return p.cfg, nil
}

func parseSequentialFullSource(source string) ParseResult {
	result := ParseResult{Config: Config{}, Errors: []string{}, Warnings: []string{}, Diagnostics: []Diagnostic{}}
	tokens, err := scanFrozenSource(source)
	if err == nil {
		p := sequentialFullSourceParser{
			frozenSourceParser: frozenSourceParser{tokens: tokens, seen: map[string]bool{}, cfg: Config{"dslVersion": int64(7), "setupType": string(FamilySequentialFull), "description": ""}},
			full:               map[string]any{"contractVersion": SequentialFullContractVersion},
		}
		result.Config, err = p.parse()
	}
	if err != nil {
		result.Config = Config{}
		message := "sequential full: " + err.Error()
		// Preserve the archived unknown-profile diagnostic wording when a
		// full profile is explicitly attached to the legacy family. It is still
		// a typed strict refusal with no legacy configuration or execution.
		for i := 0; i+4 < len(tokens); i++ {
			if !tokens[i].quoted && strings.EqualFold(tokens[i].text, "type") && tokens[i+1].text == ":" &&
				!tokens[i+2].quoted && strings.EqualFold(tokens[i+2].text, "legacy") &&
				!tokens[i+3].quoted && strings.EqualFold(tokens[i+3].text, "setup") && tokens[i+4].text == "9" {
				message = "unknown sequential profile for legacy setup 9; " + message
				break
			}
		}
		result.Errors = append(result.Errors, message)
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: DiagnosticError, Code: SequentialFullUnsupportedConfigCode, Message: message})
	}
	return result
}
