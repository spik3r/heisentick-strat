package dsl

import (
	"strings"
	"unicode"
)

// Reserved intent selects strict parsing, never a permissive fallback.
func masterTypeIntent(tokens []frozenProbeToken, i int) bool {
	if tokens[i].quoted || !masterTypeHead(tokens[i].text) {
		return false
	}
	if strings.Contains(strings.ToLower(tokens[i].text), "master") {
		return true
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
	// ordinary values (such as symbols MASTER) are not setup selectors.
	for ; i < len(tokens); i++ {
		value := strings.ToLower(tokens[i].text)
		if !tokens[i].quoted {
			switch value {
			case "\n", "{", "}", "type", "strategy", "name", "description", "symbols", "timeframes", "slices", "sessions", "windows":
				return false
			}
		}
		if strings.Contains(value, "master") {
			return true
		}
	}
	return false
}

func masterDirectiveIntent(tokens []frozenProbeToken, i int) bool {
	if !tokens[i].quoted && masterIdentity(tokens[i].text) == "masterstructural" {
		return true
	}
	if tokens[i].quoted || !strings.EqualFold(tokens[i].text, "master") {
		return false
	}
	j := i + 1
	for j < len(tokens) && tokens[j].text == "\n" && !tokens[j].quoted {
		j++
	}
	if j == len(tokens) || tokens[j].quoted {
		return false
	}
	switch strings.ToLower(tokens[j].text) {
	case "profile", "mode", "timeframe", "structural":
		return true
	}
	return false
}

// masterSelectionProjection makes quoted data opaque while retaining whether a
// quoted selector contains reserved intent. It is used only to select the strict
// parser; it never produces a config or changes the source passed to either
// parser. Damaged quotes recover as code at the same physical line boundary as
// the legacy parser, rather than hiding later authored clauses.
func masterSelectionProjection(source string) string {
	var out strings.Builder
	for _, token := range frozenProbeTokens(source) {
		if token.text == "\n" && !token.quoted {
			out.WriteByte('\n')
			continue
		}
		if token.quoted && !token.damaged {
			if strings.Contains(strings.ToLower(token.text), "master") {
				out.WriteString(`"__master_quoted_intent__"`)
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

func masterSourceCandidate(source string) bool {
	projected := masterSelectionProjection(source)
	// Reuse the exact clause expansion and token normalization that the legacy
	// parser consumes. Inspect every authored selector, not only the last one:
	// a later legacy type cannot erase an earlier master request. Quoted metadata
	// cannot manufacture synthetic clauses because its projection is opaque.
	for _, line := range expandPhysicalLines(projected) {
		tokens := frozenProbeTokens(normalizeLine(line.text))
		if len(tokens) > 0 && (masterTypeIntent(tokens, 0) || masterDirectiveIntent(tokens, 0)) {
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
		if section == "setup" && masterTypeIntent(tokens, i) {
			return true
		}
		if masterDirectiveIntent(tokens, i) {
			return true
		}
	}
	return false
}

func masterLegacyClauseStart(tokens []frozenProbeToken, i int, section string) bool {
	for _, head := range directiveStartsForSection(section) {
		words := strings.Fields(head)
		if i+len(words) > len(tokens) {
			continue
		}
		match := true
		for j, word := range words {
			if tokens[i+j].quoted || !strings.EqualFold(tokens[i+j].text, word) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// Reservation accepts damaged selector punctuation only to route the original
// text to strict rejection. It never makes those spellings valid grammar.
func masterTypeHead(text string) bool {
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
