package dsl

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type frozenToken struct {
	text      string
	quoted    bool
	line, col int
}

func scanFrozenSource(source string) ([]frozenToken, error) {
	if len(source) > 1<<20 || !utf8.ValidString(source) {
		return nil, fmt.Errorf("source exceeds 1 MiB or is not valid UTF-8")
	}
	var tokens []frozenToken
	line, col := 1, 1
	advance := func(c byte) {
		if c == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	for i := 0; i < len(source); {
		c := source[i]
		if strings.ContainsRune(" \t\r\n", rune(c)) {
			advance(c)
			i++
			continue
		}
		if c == '#' {
			for i < len(source) && source[i] != '\n' {
				advance(source[i])
				i++
			}
			continue
		}
		t := frozenToken{line: line, col: col}
		start := i
		if c == '"' {
			i++
			advance(c)
			closed := false
			for i < len(source) {
				c = source[i]
				i++
				advance(c)
				if c == '\\' && i < len(source) {
					advance(source[i])
					i++
					continue
				}
				if c == '"' {
					closed = true
					break
				}
			}
			if !closed {
				return nil, fmt.Errorf("unterminated quoted string at line %d", t.line)
			}
			value, err := strictFrozenJSONString([]byte(source[start:i]))
			if err != nil {
				return nil, err
			}
			t.text, t.quoted = value, true
		} else if strings.ContainsRune("{}:", rune(c)) {
			t.text = string(c)
			advance(c)
			i++
		} else {
			for i < len(source) && !strings.ContainsRune(" \t\r\n{}:\"#", rune(source[i])) {
				advance(source[i])
				i++
			}
			t.text = source[start:i]
			if len(t.text) > 1024 {
				return nil, fmt.Errorf("unquoted token exceeds 1024 bytes")
			}
		}
		tokens = append(tokens, t)
	}
	return tokens, nil
}

type frozenSourceParser struct {
	tokens     []frozenToken
	at         int
	seen       map[string]bool
	cfg        Config
	spec, refs map[string]any
}

func (p *frozenSourceParser) word(w string) error {
	if p.at == len(p.tokens) || p.tokens[p.at].quoted || !strings.EqualFold(p.tokens[p.at].text, w) {
		return fmt.Errorf("expected %q at token %d", w, p.at+1)
	}
	p.at++
	return nil
}
func (p *frozenSourceParser) value(quoted bool) (string, error) {
	if p.at == len(p.tokens) || p.tokens[p.at].quoted != quoted || (!quoted && strings.ContainsAny(p.tokens[p.at].text, "{}:")) {
		return "", fmt.Errorf("expected value at token %d", p.at+1)
	}
	v := p.tokens[p.at].text
	p.at++
	return v, nil
}
func (p *frozenSourceParser) once(key string) error {
	if p.seen[key] {
		return fmt.Errorf("duplicate %s", key)
	}
	p.seen[key] = true
	return nil
}
func (p *frozenSourceParser) words(words ...string) error {
	for _, w := range words {
		if err := p.word(w); err != nil {
			return err
		}
	}
	return nil
}

var frozenDSLInteger = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
var frozenDSLDecimal = regexp.MustCompile(`^((0|[1-9][0-9]*)(\.[0-9]+)?|\.[0-9]+)$`)

func (p *frozenSourceParser) number(integer bool) (any, error) {
	v, err := p.value(false)
	if err != nil {
		return nil, err
	}
	if integer {
		if !frozenDSLInteger.MatchString(v) {
			return nil, fmt.Errorf("invalid unsigned integer %q", v)
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n > 9007199254740991 {
			return nil, fmt.Errorf("integer out of exact range")
		}
		return n, nil
	}
	if !frozenDSLDecimal.MatchString(v) {
		return nil, fmt.Errorf("invalid unsigned decimal %q", v)
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || n <= 0 || math.IsInf(n, 0) || math.IsNaN(n) {
		return nil, fmt.Errorf("decimal must be positive and finite")
	}
	return n, nil
}

func (p *frozenSourceParser) directive(section string) error {
	if p.at == len(p.tokens) {
		return fmt.Errorf("missing block close")
	}
	if section == "strategy" {
		if err := p.words("description"); err != nil {
			return err
		}
		if err := p.once("description"); err != nil {
			return err
		}
		v, err := p.value(true)
		p.cfg["description"] = v
		return err
	}
	if section == "setup" && !p.tokens[p.at].quoted && strings.EqualFold(p.tokens[p.at].text, "type") {
		if err := p.once("type"); err != nil {
			return err
		}
		return p.words("type", ":", "frozen", "level", "breakout")
	}
	if err := p.word("frozen"); err != nil {
		return err
	}
	key, err := p.value(false)
	if err != nil {
		return err
	}
	key = strings.ToLower(key)
	expected := map[string]string{"control": "setup", "source": "market", "calendar": "market", "ordering": "market", "schedule": "market", "grid": "risk", "lock": "management", "activation": "management", "timeout": "management", "freshness": "execution", "latency": "execution", "costs": "execution", "assumptions": "execution"}
	if expected[key] != section {
		return fmt.Errorf("unknown or misplaced frozen %s directive in %s", key, section)
	}
	if err := p.once(key); err != nil {
		return err
	}
	switch key {
	case "control":
		return p.word("stored-m30-lock-isolation-v1")
	case "source", "calendar", "ordering", "schedule", "costs", "assumptions":
		id, err := p.value(true)
		if err != nil {
			return err
		}
		if err = p.word("version"); err != nil {
			return err
		}
		version, err := p.value(true)
		if err != nil {
			return err
		}
		if err = p.word("sha256"); err != nil {
			return err
		}
		digest, err := p.value(true)
		if err != nil {
			return err
		}
		p.refs[key] = map[string]any{"id": id, "version": version, "sha256": digest}
		return nil
	case "lock":
		v, err := p.value(false)
		p.spec["lockMode"] = strings.ToLower(v)
		return err
	case "activation", "grid", "timeout":
		v, err := p.number(key == "timeout")
		if err != nil {
			return err
		}
		field, units := "activationR", []string{"R"}
		if key == "grid" {
			field, units = "priceGrid", []string{"price", "units"}
		} else if key == "timeout" {
			field, units = "timeoutMinutes", []string{"minutes"}
		}
		p.spec[field] = v
		return p.words(units...)
	case "freshness", "latency":
		first, second, field1, field2 := "predecessor", "receipt", "maxPredecessorAgeMS", "maxReceiptAgeMS"
		if key == "latency" {
			first, second, field1, field2 = "entry", "amendment", "entryLatencyMS", "amendmentLatencyMS"
		}
		if err := p.word(first); err != nil {
			return err
		}
		v, err := p.number(true)
		if err != nil {
			return err
		}
		p.spec[field1] = v
		if err := p.words("ms", second); err != nil {
			return err
		}
		v, err = p.number(true)
		if err != nil {
			return err
		}
		p.spec[field2] = v
		return p.word("ms")
	}
	return fmt.Errorf("unknown frozen directive")
}

func (p *frozenSourceParser) parse() (Config, error) {
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
		case "market":
			if err := p.word("conditions"); err != nil {
				return nil, err
			}
		case "setup", "risk", "management", "execution":
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
	for _, section := range []string{"strategy", "market", "setup", "risk", "management", "execution"} {
		if !blocks[section] {
			return nil, fmt.Errorf("missing block %s", section)
		}
	}
	for _, key := range []string{"type", "control", "source", "calendar", "ordering", "schedule", "grid", "lock", "activation", "timeout", "freshness", "latency", "costs", "assumptions"} {
		if !p.seen[key] {
			return nil, fmt.Errorf("missing directive %s", key)
		}
	}
	return NormalizeFrozenLevelConfig(p.cfg)
}

func parseFrozenLevelSource(source string) ParseResult {
	result := ParseResult{Config: Config{}, Errors: []string{}, Warnings: []string{}, Diagnostics: []Diagnostic{}}
	tokens, err := scanFrozenSource(source)
	if err == nil {
		refs := map[string]any{}
		spec := map[string]any{"contractVersion": "frozen-level-compiler-v1", "controlProfile": "stored-m30-lock-isolation-v1", "profile": FrozenLevelProfile(), "inputRefs": refs}
		p := frozenSourceParser{tokens: tokens, seen: map[string]bool{}, refs: refs, spec: spec, cfg: Config{"dslVersion": int64(7), "setupType": string(FamilyFrozenLevelBreakout), "description": "", "frozenLevelBreakout": spec}}
		result.Config, err = p.parse()
	}
	if err != nil {
		result.Config = Config{}
		message := "frozen level breakout: " + err.Error()
		result.Errors = append(result.Errors, message)
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: DiagnosticError, Message: message})
		return result
	}
	result.Warnings = append(result.Warnings, FrozenLevelExecutionUnimplemented)
	result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: DiagnosticWarning, Message: FrozenLevelExecutionUnimplemented})
	return result
}
