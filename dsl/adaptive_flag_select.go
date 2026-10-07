package dsl

import (
	"strings"
	"unicode"
)

// Reserved intent selects strict parsing, never a permissive fallback.
func adaptiveFlagTypeIntent(tokens []frozenProbeToken, i int) bool {
	if tokens[i].quoted || !adaptiveFlagTypeHead(tokens[i].text) {
		return false
	}
	if adaptiveFlagSelectorStem(tokens[i].text) {
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
	// ordinary values (such as symbols GOLD) are not setup selectors.
	for ; i < len(tokens); i++ {
		value := strings.ToLower(tokens[i].text)
		if !tokens[i].quoted {
			switch value {
			case "\n", "{", "}", "type", "strategy", "name", "description", "symbols", "timeframes", "slices", "sessions", "windows":
				return false
			}
		}
		if adaptiveFlagSelectorStem(value) {
			return true
		}
	}
	return false
}

func adaptiveFlagDirectiveIntent(tokens []frozenProbeToken, i int) bool {
	if tokens[i].quoted {
		return false
	}
	// The unquoted adaptive namespace is reserved, including punctuation-split
	// and damaged directive heads. Metadata and lists are made opaque by the
	// surrounding selector before this check; reservation never admits an alias.
	return adaptiveFlagSelectorStem(tokens[i].text)
}

// adaptiveFlagSelectionProjection makes quoted data opaque while retaining whether a
// quoted selector contains reserved intent. It is used only to select the strict
// parser; it never produces a config or changes the source passed to either
// parser. Damaged quotes recover as code at the same physical line boundary as
// the legacy parser, rather than hiding later authored clauses.
func adaptiveFlagSelectionProjection(source string) string {
	var out strings.Builder
	for _, token := range frozenProbeTokens(source) {
		if token.text == "\n" && !token.quoted {
			out.WriteByte('\n')
			continue
		}
		if token.quoted && !token.damaged {
			if adaptiveFlagSelectorStem(token.text) {
				out.WriteString(`"__adaptiveflag_quoted_intent__"`)
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

func adaptiveFlagSourceCandidate(source string) bool {
	projected := adaptiveFlagSelectionProjection(source)
	// Reuse the exact clause expansion and token normalization that the legacy
	// parser consumes. Inspect every authored selector, not only the last one:
	// a later legacy type cannot erase an earlier adaptive-volume-flag request. Quoted metadata
	// cannot manufacture synthetic clauses because its projection is opaque.
	for _, line := range expandPhysicalLines(projected) {
		tokens := frozenProbeTokens(normalizeLine(line.text))
		if len(tokens) > 0 && (adaptiveFlagTypeIntent(tokens, 0) || adaptiveFlagDirectiveIntent(tokens, 0)) {
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
		if section == "setup" && adaptiveFlagTypeIntent(tokens, i) {
			return true
		}
		if adaptiveFlagDirectiveIntent(tokens, i) {
			return true
		}
	}
	return false
}

// Reservation accepts damaged selector punctuation only to route the original
// text to strict rejection. It never makes those spellings valid grammar.
func adaptiveFlagTypeHead(text string) bool {
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

// No existing family uses the adaptive namespace. Punctuation and spacing
// variants remain reserved without becoming valid syntax.
func adaptiveFlagSelectorStem(text string) bool {
	text = strings.ToLower(text)
	if strings.Contains(goldFlagIdentity(text), "adaptiveflag") || strings.Contains(goldFlagIdentity(text), "adaptivevolumeflag") {
		return true
	}
	for _, word := range strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if word == "adaptive" {
			return true
		}
	}
	return false
}
