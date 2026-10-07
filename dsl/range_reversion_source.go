package dsl

import (
	"encoding/json"
	"fmt"
	"strings"
)

type rangeReversionSourceParser struct {
	frozenSourceParser
	rules             RangeReversionRules
	name, description string
}

func (p *rangeReversionSourceParser) directive(section string) error {
	if section == "strategy" {
		if err := p.words("description"); err != nil {
			return err
		}
		if err := p.once("description"); err != nil {
			return err
		}
		v, err := p.value(true)
		p.description = v
		return err
	}
	if section == "setup" && p.at < len(p.tokens) && !p.tokens[p.at].quoted && strings.EqualFold(p.tokens[p.at].text, "type") {
		if err := p.once("setup.type"); err != nil {
			return err
		}
		return p.words("type", ":", "range", "reversion")
	}
	if err := p.word("rangereversion"); err != nil {
		return err
	}
	key, err := p.value(false)
	if err != nil {
		return err
	}
	key = strings.ToLower(key)
	if err = p.once(section + "." + key); err != nil {
		return err
	}
	if section == "market" {
		switch key {
		case "timeframe":
			v, e := p.value(false)
			if e != nil {
				return e
			}
			p.rules.Timeframe = strings.ToUpper(v)
			return nil
		case "source":
			v, e := p.value(false)
			if e != nil {
				return e
			}
			p.rules.SourceTimeframe = strings.ToUpper(v)
			if e = p.words("availability", "next-native-row"); e != nil {
				return e
			}
			p.rules.SourceAvailability = RangeReversionSourceAvailability
			return nil
		default:
			return fmt.Errorf("unknown rangereversion market directive %s", key)
		}
	}
	if section != "setup" {
		return fmt.Errorf("rangereversion directive is not allowed in %s", section)
	}
	switch key {
	case "policy":
		p.rules.Policy, err = p.value(false)
		p.rules.Policy = strings.ToUpper(p.rules.Policy)
		return err
	case "bounds":
		p.rules.Bounds, err = p.value(false)
		p.rules.Bounds = strings.ToUpper(p.rules.Bounds)
		if err != nil {
			return err
		}
		n, e := p.integer()
		p.rules.BoundsLookback = n
		return e
	case "candle-color":
		p.rules.RequireCandleColor, err = p.boolean()
		return err
	case "htf-ema":
		p.rules.UseHTFEMA, err = p.boolean()
		if err != nil {
			return err
		}
		p.rules.HTFEMALength, err = p.integer()
		return err
	case "vector-gates":
		p.rules.UseVectorGates, err = p.boolean()
		if err != nil {
			return err
		}
		if p.rules.VectorLength, err = p.integer(); err != nil {
			return err
		}
		if p.rules.MaxADX, err = p.decimal(); err != nil {
			return err
		}
		p.rules.MinCHOP, err = p.decimal()
		return err
	case "range-expansion":
		p.rules.UseRangeExpansion, err = p.boolean()
		if err != nil {
			return err
		}
		p.rules.RangeATRMultiple, err = p.decimal()
		return err
	case "atr":
		p.rules.ATRLength, err = p.integer()
		return err
	case "stop-atr":
		p.rules.StopATRMultiple, err = p.decimal()
		return err
	case "target-r":
		p.rules.TargetR, err = p.decimal()
		return err
	case "cooldown":
		p.rules.CooldownBars, err = p.integer()
		return err
	case "breakeven":
		p.rules.BreakEvenEnabled, err = p.boolean()
		if err != nil {
			return err
		}
		if p.rules.BreakEvenTriggerR, err = p.decimal(); err != nil {
			return err
		}
		p.rules.BreakEvenOffsetTicks, err = p.integer()
		return err
	case "tick-size":
		p.rules.TickSize, err = p.decimal()
		return err
	default:
		return fmt.Errorf("unknown rangereversion setup directive %s", key)
	}
}

func (p *rangeReversionSourceParser) integer() (int, error) {
	v, err := p.number(true)
	if err != nil {
		return 0, err
	}
	return int(v.(int64)), nil
}

func (p *rangeReversionSourceParser) decimal() (float64, error) {
	v, err := p.value(false)
	if err != nil {
		return 0, err
	}
	return rangeReversionNumber(v, false)
}

func (p *rangeReversionSourceParser) boolean() (bool, error) {
	v, err := p.value(false)
	if err != nil {
		return false, err
	}
	if strings.EqualFold(v, "true") {
		return true, nil
	}
	if strings.EqualFold(v, "false") {
		return false, nil
	}
	return false, fmt.Errorf("expected true or false")
}

func (p *rangeReversionSourceParser) parse() (Config, error) {
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
		if section == "strategy" {
			p.name, err = p.value(true)
			if err != nil {
				return nil, err
			}
		} else if section != "market" && section != "setup" {
			return nil, fmt.Errorf("unknown block %q", section)
		}
		if blocks[section] {
			return nil, fmt.Errorf("duplicate block %s", section)
		}
		blocks[section] = true
		if err = p.word("{"); err != nil {
			return nil, err
		}
		for p.at < len(p.tokens) && p.tokens[p.at].text != "}" {
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
			return nil, fmt.Errorf("missing %s block", block)
		}
	}
	if p.name == "" {
		return nil, fmt.Errorf("strategy name must not be empty")
	}
	if !p.seen["description"] || !p.seen["setup.type"] {
		return nil, fmt.Errorf("description and type are required")
	}
	for _, key := range []string{"market.timeframe", "market.source", "setup.policy", "setup.bounds", "setup.candle-color", "setup.htf-ema", "setup.vector-gates", "setup.range-expansion", "setup.atr", "setup.stop-atr", "setup.target-r", "setup.cooldown", "setup.breakeven", "setup.tick-size"} {
		if !p.seen[key] {
			return nil, fmt.Errorf("missing rangereversion %s directive", key)
		}
	}
	if p.rules.SourceTimeframe == "" {
		return nil, fmt.Errorf("missing source timeframe")
	}
	cfg := Config{"dslVersion": int64(7), "setupType": string(FamilyRangeReversion), "name": p.name, "description": p.description, "rangeReversion": p.rules.projection()}
	if _, err := DecodeRangeReversion(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func parseRangeReversionSource(source string) ParseResult {
	result := ParseResult{Config: Config{}, Errors: []string{}, Warnings: []string{}, Diagnostics: []Diagnostic{}}
	tokens, err := scanFrozenSource(source)
	if err == nil {
		p := rangeReversionSourceParser{frozenSourceParser: frozenSourceParser{tokens: tokens, seen: map[string]bool{}, cfg: Config{}}, rules: RangeReversionRules{}}
		result.Config, err = p.parse()
	}
	if err != nil {
		message := "range reversion: " + err.Error()
		result.Errors = append(result.Errors, message)
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: DiagnosticError, Message: message})
		return result
	}
	result.Warnings = append(result.Warnings, RangeReversionDedicatedRunnerRequired)
	result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: DiagnosticWarning, Message: RangeReversionDedicatedRunnerRequired})
	return result
}

func rangeReversionCanonicalJSON(cfg Config) ([]byte, error) { return json.Marshal(cfg) }
