package dsl

import (
	"strings"
	"unicode"
)

// Reserved intent selects strict parsing, never a permissive fallback.
func rangeReversionTypeIntent(tokens []frozenProbeToken, i int) bool {
	if tokens[i].quoted || !rangeReversionTypeHead(tokens[i].text) {
		return false
	}
	if rangeReversionSelectorStem(tokens[i].text) {
		return true
	}
	i++
	for i < len(tokens) && (tokens[i].text == "\n" || tokens[i].text == ":") {
		i++
	}
	previous := ""
	for ; i < len(tokens); i++ {
		value := strings.ToLower(tokens[i].text)
		if !tokens[i].quoted {
			switch value {
			case "\n", "{", "}", "type", "strategy", "name", "description", "symbols", "timeframes", "slices", "sessions", "windows":
				return false
			}
		}
		for _, word := range rangeReversionIntentWords(value) {
			if word == "rangereversion" || previous == "range" && word == "reversion" {
				return true
			}
			previous = word
		}
	}
	return false
}

func rangeReversionDirectiveIntent(tokens []frozenProbeToken, i int) bool {
	if tokens[i].quoted {
		return false
	}
	if rangeReversionSelectorStem(tokens[i].text) {
		return true
	}
	// Tokenization separates colon/comma/parentheses. Rejoin punctuation only,
	// never intervening directives or a physical newline, to reserve a damaged
	// namespace without capturing unrelated uses of the ordinary word "range".
	if !strings.EqualFold(tokens[i].text, "range") {
		return false
	}
	for j := i + 1; j < len(tokens); j++ {
		if tokens[j].quoted || tokens[j].text == "\n" {
			return false
		}
		if strings.EqualFold(tokens[j].text, "reversion") {
			return true
		}
		if strings.TrimFunc(tokens[j].text, func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSymbol(r) }) != "" {
			return false
		}
		if strings.ContainsAny(tokens[j].text, "{}") {
			return false
		}
	}
	return false
}

// rangeReversionSelectionProjection makes quoted data opaque while retaining whether a
// quoted selector contains reserved intent. It is used only to select the strict
// parser; it never produces a config or changes the source passed to either
// parser. Damaged quotes recover as code at the same physical line boundary as
// the legacy parser, rather than hiding later authored clauses.
func rangeReversionSelectionProjection(source string) string {
	var out strings.Builder
	for _, token := range frozenProbeTokens(source) {
		if token.text == "\n" && !token.quoted {
			out.WriteByte('\n')
			continue
		}
		if token.quoted && !token.damaged {
			if rangeReversionSelectorStem(token.text) {
				out.WriteString(`"__range_reversion_quoted_intent__"`)
			} else if strings.EqualFold(token.text, "range") || strings.EqualFold(token.text, "reversion") {
				// Preserve a quoted single component only for selector matching; the
				// surrounding quote still keeps metadata and named-list data opaque.
				out.WriteByte('"')
				out.WriteString(strings.ToLower(token.text))
				out.WriteByte('"')
			} else {
				out.WriteString(`"__quoted_data__"`)
			}
		} else {
			// Match the legacy strings.Fields whitespace vocabulary for intent
			// detection only. Keep the original bytes for strict parsing, and
			// never reinterpret genuine quoted metadata or list contents.
			out.WriteString(strings.Join(strings.Fields(token.text), " "))
		}
		out.WriteByte(' ')
	}
	return out.String()
}

func rangeReversionSourceCandidate(source string) bool {
	projected := rangeReversionSelectionProjection(source)
	// Reuse the exact clause expansion and token normalization that the legacy
	// parser consumes. Inspect every authored selector, not only the last one:
	// a later legacy type cannot erase an earlier range-reversion request.
	// Quoted metadata cannot manufacture clauses because its projection is opaque.
	for _, line := range expandPhysicalLines(projected) {
		tokens := frozenProbeTokens(normalizeLine(line.text))
		if len(tokens) > 0 && (rangeReversionTypeIntent(tokens, 0) || rangeReversionDirectiveIntent(tokens, 0)) {
			return true
		}
	}
	// Legacy inline expansion can discard malformed leading fragments. Audit
	// explicit selectors throughout setup bodies, including anonymous groups,
	// instead of assuming those dropped tokens are inert. Recognized, balanced
	// list arguments and genuine quoted values remain opaque.
	tokens := frozenProbeTokens(projected)
	var blocks []string
	opaqueTail := false
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
		case "\n":
			opaqueTail = false
			continue
		case "{":
			section := ""
			if len(blocks) > 0 {
				section = blocks[len(blocks)-1]
			}
			if i > 0 {
				if name, ok := generatedSectionAliases[strings.ToLower(tokens[i-1].text)]; ok && !tokens[i-1].quoted {
					section = name
				}
			}
			if i > 1 && tokens[i-1].quoted && strings.EqualFold(tokens[i-2].text, "strategy") {
				section = "strategy"
			}
			blocks = append(blocks, section)
			opaqueTail = false
			continue
		case "}":
			if len(blocks) > 0 {
				blocks = blocks[:len(blocks)-1]
			}
			opaqueTail = false
			continue
		}
		section := ""
		if len(blocks) > 0 {
			section = blocks[len(blocks)-1]
		}
		// Free metadata and bare-list values extend to their legacy clause boundary.
		// In a brace body only a real section-specific inline directive ends that
		// span; at top level it runs to the physical newline. Quoted and balanced
		// named-list values were already made opaque, so they cannot manufacture a
		// clause. Inspect every remaining token, including legacy-discarded tails.
		if opaqueTail {
			if len(blocks) == 0 || !masterLegacyClauseStart(tokens, i, section) {
				continue
			}
			opaqueTail = false
		}
		switch strings.ToLower(t.text) {
		case "strategy", "name", "description", "symbols", "timeframes", "slices", "sessions", "windows":
			opaqueTail = true
			continue
		}
		if section == "setup" && rangeReversionTypeIntent(tokens, i) {
			return true
		}
		if rangeReversionDirectiveIntent(tokens, i) {
			return true
		}
	}
	return false
}

// Reservation accepts damaged selector punctuation only to route the original
// text to strict rejection. It never makes those spellings valid grammar.
func rangeReversionTypeHead(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "type" {
		return true
	}
	if !strings.HasPrefix(text, "type") {
		return false
	}
	for _, r := range text[len("type"):] {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	}
	return false
}

// Reservation is limited to the complete family name. "range" alone is used
// by valid legacy families. Punctuation/spacing variants select strict rejection
// of the original bytes; they are not new accepted syntax.
func rangeReversionSelectorStem(text string) bool {
	words := rangeReversionIntentWords(text)
	for i, word := range words {
		if word == "rangereversion" || word == "range" && i+1 < len(words) && words[i+1] == "reversion" {
			return true
		}
	}
	return false
}

func rangeReversionIntentWords(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}
