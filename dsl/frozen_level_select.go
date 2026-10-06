package dsl

import "strings"

// Selection recognizes reserved intent, not valid syntax. In particular a
// quoted or compact frozen type must reach the strict scanner and fail there;
// the permissive legacy parser must never silently replace it with its default.
// Values in legacy metadata and lists are opaque. Recovery at physical newlines
// matches the legacy parser's line boundary, so a damaged quote cannot hide a
// later selector that the legacy parser would still execute.
type frozenProbeToken struct {
	text            string
	quoted, damaged bool
}

func frozenProbeTokens(source string) []frozenProbeToken {
	var out []frozenProbeToken
	for i := 0; i < len(source); {
		c := source[i]
		switch {
		case c == '\n':
			out = append(out, frozenProbeToken{text: "\n"})
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '#':
			for i < len(source) && source[i] != '\n' {
				i++
			}
		case strings.ContainsRune("{}():,", rune(c)):
			out = append(out, frozenProbeToken{text: string(c)})
			i++
		case c == '"' || c == '\'' || c == '`':
			start, quote := i, c
			i++
			closed := false
			for i < len(source) && source[i] != '\n' {
				c = source[i]
				i++
				if c == '\\' && i < len(source) && source[i] != '\n' {
					i++
					continue
				}
				if c == quote {
					closed = true
					break
				}
			}
			end := i
			if closed {
				end--
			}
			value := source[start+1 : end]
			if closed && quote == '"' {
				if decoded, err := strictFrozenJSONString([]byte(source[start:i])); err == nil {
					value = decoded
				}
			}
			out = append(out, frozenProbeToken{text: value, quoted: true, damaged: !closed})
		default:
			start := i
			for i < len(source) && !strings.ContainsRune(" \t\r\n{}():,\"'`#", rune(source[i])) {
				i++
			}
			out = append(out, frozenProbeToken{text: source[start:i]})
		}
	}
	return out
}

func frozenTypeIntent(tokens []frozenProbeToken, i int) bool {
	if tokens[i].quoted || !strings.EqualFold(tokens[i].text, "type") {
		return false
	}
	i++
	for i < len(tokens) && tokens[i].text == "\n" {
		i++
	}
	if i < len(tokens) && tokens[i].text == ":" {
		i++
	}
	for i < len(tokens) && tokens[i].text == "\n" {
		i++
	}
	// No supported legacy setup contains this reserved family stem. Inspect
	// the selector clause even when malformed punctuation precedes it, without
	// accepting that text as an alias. Stop at metadata/list clauses: their
	// ordinary values (such as symbols FROZEN) are not setup selectors.
	for ; i < len(tokens); i++ {
		value := strings.ToLower(tokens[i].text)
		if !tokens[i].quoted {
			switch value {
			case "\n", "{", "}", "type", "strategy", "name", "description", "symbols", "timeframes", "slices", "sessions", "windows":
				return false
			}
		}
		if strings.Contains(value, "frozen") {
			return true
		}
	}
	return false
}

func frozenDirectiveIntent(tokens []frozenProbeToken, i int) bool {
	if tokens[i].quoted || !strings.EqualFold(tokens[i].text, "frozen") || i+1 == len(tokens) || tokens[i+1].quoted {
		return false
	}
	switch strings.ToLower(tokens[i+1].text) {
	case "control", "source", "calendar", "ordering", "schedule", "grid", "lock", "activation", "timeout", "freshness", "latency", "costs", "assumptions":
		return true
	}
	return false
}

// frozenSelectionProjection makes quoted data opaque while retaining whether a
// quoted selector contains reserved intent. It is used only to select the strict
// parser; it never produces a config or changes the source passed to either
// parser. Damaged quotes recover as code at the same physical line boundary as
// the legacy parser, rather than hiding later authored clauses.
func frozenSelectionProjection(source string) string {
	var out strings.Builder
	for _, token := range frozenProbeTokens(source) {
		if token.text == "\n" && !token.quoted {
			out.WriteByte('\n')
			continue
		}
		if token.quoted && !token.damaged {
			if strings.Contains(strings.ToLower(token.text), "frozen") {
				out.WriteString(`"__frozen_quoted_intent__"`)
			} else {
				out.WriteString(`"__quoted_data__"`)
			}
		} else {
			out.WriteString(token.text)
		}
		out.WriteByte(' ')
	}
	return out.String()
}

// Only balanced, named list arguments are opaque groups. An anonymous group
// such as (type: frozen ...) is not a list, and an unclosed group provides no
// safe boundary. Bare list and free metadata clauses are deliberately not
// skipped: expandPhysicalLines below decides where their inline clauses end.
func frozenNamedListEnd(tokens []frozenProbeToken, i int) int {
	if tokens[i].quoted {
		return i
	}
	switch strings.ToLower(tokens[i].text) {
	case "symbols", "timeframes", "slices", "sessions", "windows":
	default:
		return i
	}
	if i+1 == len(tokens) || tokens[i+1].text != "(" {
		return i
	}
	depth := 0
	for end := i + 1; end < len(tokens); end++ {
		if tokens[end].quoted {
			continue
		}
		switch tokens[end].text {
		case "\n", "{", "}":
			return i
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return end
			}
		}
	}
	return i
}

func frozenSourceCandidate(source string) bool {
	projected := frozenSelectionProjection(source)
	// Reuse the exact clause expansion and token normalization that the legacy
	// parser consumes. Inspect every authored selector, not only the last one:
	// a later legacy type cannot erase an earlier frozen request. Quoted metadata
	// cannot manufacture synthetic clauses because its projection is opaque.
	for _, line := range expandPhysicalLines(projected) {
		tokens := frozenProbeTokens(normalizeLine(line.text))
		if len(tokens) > 0 && (frozenTypeIntent(tokens, 0) || frozenDirectiveIntent(tokens, 0)) {
			return true
		}
	}
	// Legacy inline expansion can discard malformed leading fragments. Audit
	// explicit selectors throughout setup bodies, including anonymous groups,
	// instead of assuming those dropped tokens are inert. Recognized, balanced
	// list arguments and genuine quoted values remain opaque.
	tokens := frozenProbeTokens(projected)
	var blocks []bool
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if t.quoted {
			continue
		}
		if end := frozenNamedListEnd(tokens, i); end > i {
			i = end
			continue
		}
		switch t.text {
		case "{":
			isSetup := i > 0 && !tokens[i-1].quoted && strings.EqualFold(tokens[i-1].text, "setup")
			if len(blocks) > 0 && blocks[len(blocks)-1] {
				isSetup = true
			}
			blocks = append(blocks, isSetup)
			continue
		case "}":
			if len(blocks) > 0 {
				blocks = blocks[:len(blocks)-1]
			}
			continue
		}
		if len(blocks) > 0 && blocks[len(blocks)-1] && frozenTypeIntent(tokens, i) {
			return true
		}
		if (i == 0 || tokens[i-1].text == "{" || tokens[i-1].text == "\n") && frozenDirectiveIntent(tokens, i) {
			return true
		}
	}
	return false
}
