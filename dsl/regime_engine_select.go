package dsl

import "strings"

// Reserved intent selects strict parsing, never a permissive fallback.
func regimeTypeIntent(tokens []frozenProbeToken, i int) bool {
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
	// ordinary values (such as symbols REGIME) are not setup selectors.
	for ; i < len(tokens); i++ {
		value := strings.ToLower(tokens[i].text)
		if !tokens[i].quoted {
			switch value {
			case "\n", "{", "}", "type", "strategy", "name", "description", "symbols", "timeframes", "slices", "sessions", "windows":
				return false
			}
		}
		if strings.Contains(value, "regime") {
			return true
		}
	}
	return false
}

func regimeDirectiveIntent(tokens []frozenProbeToken, i int) bool {
	if !tokens[i].quoted && regimeIdentity(tokens[i].text) == "regimeengine" {
		return true
	}
	if tokens[i].quoted || !strings.EqualFold(tokens[i].text, "regime") || i+1 == len(tokens) || tokens[i+1].quoted {
		return false
	}
	switch strings.ToLower(tokens[i+1].text) {
	case "profile", "mode", "timeframe":
		return true
	}
	return false
}

// regimeSelectionProjection makes quoted data opaque while retaining whether a
// quoted selector contains reserved intent. It is used only to select the strict
// parser; it never produces a config or changes the source passed to either
// parser. Damaged quotes recover as code at the same physical line boundary as
// the legacy parser, rather than hiding later authored clauses.
func regimeSelectionProjection(source string) string {
	var out strings.Builder
	for _, token := range frozenProbeTokens(source) {
		if token.text == "\n" && !token.quoted {
			out.WriteByte('\n')
			continue
		}
		if token.quoted && !token.damaged {
			if strings.Contains(strings.ToLower(token.text), "regime") {
				out.WriteString(`"__regime_quoted_intent__"`)
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

func regimeSourceCandidate(source string) bool {
	projected := regimeSelectionProjection(source)
	// Reuse the exact clause expansion and token normalization that the legacy
	// parser consumes. Inspect every authored selector, not only the last one:
	// a later legacy type cannot erase an earlier regime request. Quoted metadata
	// cannot manufacture synthetic clauses because its projection is opaque.
	for _, line := range expandPhysicalLines(projected) {
		tokens := frozenProbeTokens(normalizeLine(line.text))
		if len(tokens) > 0 && (regimeTypeIntent(tokens, 0) || regimeDirectiveIntent(tokens, 0)) {
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
		if len(blocks) > 0 && blocks[len(blocks)-1] && regimeTypeIntent(tokens, i) {
			return true
		}
		if (i == 0 || tokens[i-1].text == "{" || tokens[i-1].text == "\n") && regimeDirectiveIntent(tokens, i) {
			return true
		}
	}
	return false
}
