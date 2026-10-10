package dsl

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
)

// Reserved intent selects strict parsing, never a permissive fallback.
func sequentialFullTypeIntent(tokens []frozenProbeToken, i int) bool {
	if tokens[i].quoted || !sequentialFullTypeHead(tokens[i].text) {
		return false
	}
	if sequentialFullSelectorStem(tokens[i].text) {
		return true
	}
	i++
	for i < len(tokens) && (tokens[i].text == "\n" || tokens[i].text == ":") {
		i++
	}
	start := i
	for ; i < len(tokens); i++ {
		value := strings.ToLower(tokens[i].text)
		if !tokens[i].quoted {
			switch value {
			case "\n", "{", "}", "type", "strategy", "name", "description", "symbols", "timeframes", "slices", "sessions", "windows":
				return false
			}
			// A legacy type followed by its valid sequential profile is an
			// authored directive boundary, not part of the type phrase.
			if i > start && value == "sequential" && i+1 < len(tokens) && strings.EqualFold(tokens[i+1].text, "profile") {
				return false
			}
		}
		if sequentialFullSelectorStem(value) {
			return true
		}
	}
	return false
}

func sequentialFullDirectiveIntent(tokens []frozenProbeToken, i int) bool {
	if tokens[i].quoted {
		return false
	}
	if sequentialFullMarker(tokens[i].text) {
		return true
	}
	if sequentialFullIdentity(tokens[i].text) != "sequential" {
		return false
	}
	// `sequential profile seq.legacy.*` remains the archived family's syntax.
	// The other full-family directives reserve this parser even when misplaced.
	j := i + 1
	for j < len(tokens) && (tokens[j].text == "\n" || tokens[j].text == ":") {
		j++
	}
	if j == len(tokens) {
		return false
	}
	key := sequentialFullIdentity(sequentialFullProbeText(tokens[j].text))
	if key == "profile" {
		j++
		for j < len(tokens) && (tokens[j].text == "\n" || tokens[j].text == ":") {
			j++
		}
		return j < len(tokens) && sequentialFullMarker(tokens[j].text)
	}
	switch key {
	case "full", "policy", "symbol", "timeframe", "riskusd", "maxnotionalusd":
		return true
	}
	return false
}

// sequentialFullSelectionProjection makes quoted data opaque while retaining whether a
// quoted selector contains reserved intent. It is used only to select the strict
// parser; it never produces a config or changes the source passed to either
// parser. Damaged quotes recover as code at the same physical line boundary as
// the legacy parser, rather than hiding later authored clauses.
func sequentialFullSelectionProjection(source string) string {
	var out strings.Builder
	for _, token := range frozenProbeTokens(source) {
		if token.text == "\n" && !token.quoted {
			out.WriteByte('\n')
			continue
		}
		if token.quoted && !token.damaged {
			text := sequentialFullProbeText(token.text)
			if sequentialFullIdentity(text) == "sequential" || sequentialFullTypeHead(text) {
				// Keep quoted heads available to the context-aware body probe.
				// They remain quoted, so metadata and list values stay opaque.
				raw, _ := json.Marshal(text)
				out.Write(raw)
			} else if sequentialFullSelectorStem(token.text) {
				out.WriteString(`"__sequentialFull_quoted_intent__"`)
			} else if sequentialFullDirectiveKey(token.text) {
				// Preserve a quoted directive key only for intent detection.
				// Strict parsing still rejects quoted keys in the original source.
				raw, _ := json.Marshal(sequentialFullProbeText(token.text))
				out.Write(raw)
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

func sequentialFullSourceCandidate(source string) bool {
	projected := sequentialFullSelectionProjection(source)
	// Reuse the exact clause expansion and token normalization that the legacy
	// parser consumes. Inspect every authored selector, not only the last one:
	// a later legacy type cannot erase an earlier sequential-full request. Quoted metadata
	// cannot manufacture synthetic clauses because its projection is opaque.
	for _, line := range expandPhysicalLines(projected) {
		tokens := frozenProbeTokens(normalizeLine(line.text))
		if len(tokens) > 0 && (sequentialFullTypeIntent(tokens, 0) || sequentialFullDirectiveIntent(tokens, 0)) {
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
			section := ""
			if len(blocks) > 0 {
				section = blocks[len(blocks)-1]
			}
			if !opaqueTail && (len(blocks) == 0 || section == "setup" || section == "market" || section == "risk") && sequentialFullQuotedHeadIntent(tokens, i) {
				return true
			}
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
		if section == "setup" && sequentialFullTypeIntent(tokens, i) {
			return true
		}
		if sequentialFullDirectiveIntent(tokens, i) {
			return true
		}
	}
	return false
}

// A quoted directive head is malformed code only at an exposed clause position.
// The caller keeps strategy metadata and named-list arguments opaque; this probe
// merely removes the head's quote flag for reservation, never for actual parsing.
func sequentialFullQuotedHeadIntent(tokens []frozenProbeToken, i int) bool {
	head := sequentialFullProbeText(tokens[i].text)
	if sequentialFullIdentity(head) != "sequential" && !sequentialFullTypeHead(head) {
		return false
	}
	probe := make([]frozenProbeToken, 1, len(tokens)-i)
	probe[0].text = head
	probe = append(probe, tokens[i+1:]...)
	return sequentialFullTypeIntent(probe, 0) || sequentialFullDirectiveIntent(probe, 0)
}

// Reservation accepts damaged selector punctuation only to route the original
// text to strict rejection. It never makes those spellings valid grammar.
func sequentialFullTypeHead(text string) bool {
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

// The full type stem is reserved, including malformed punctuation. Legacy
// selectors use `legacy setup 9`, never `sequential`, and remain separate.
func sequentialFullSelectorStem(text string) bool {
	text = sequentialFullProbeText(text)
	if sequentialFullMarker(text) {
		return true
	}
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if word == "sequential" {
			return true
		}
	}
	return false
}

// A malformed JSON string may still contain an escaped reserved selector. A
// broken later escape must not hide earlier valid escapes. This recovery only
// selects strict rejection; the parser always receives the original bytes.
func sequentialFullProbeText(text string) string {
	if !strings.Contains(text, `\`) {
		return text
	}
	var decoded string
	if json.Unmarshal([]byte(`"`+text+`"`), &decoded) == nil {
		return decoded
	}
	var out strings.Builder
	for i := 0; i < len(text); {
		if text[i] == '\\' && i+1 < len(text) {
			if text[i+1] == 'u' && i+6 <= len(text) {
				if value, err := strconv.ParseUint(text[i+2:i+6], 16, 16); err == nil && (value < 0xd800 || value > 0xdfff) {
					out.WriteRune(rune(value))
					i += 6
					continue
				}
			}
			if escaped, ok := map[byte]byte{'"': '"', '\\': '\\', '/': '/', 'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t'}[text[i+1]]; ok {
				out.WriteByte(escaped)
				i += 2
				continue
			}
		}
		out.WriteByte(text[i])
		i++
	}
	return out.String()
}

func sequentialFullDirectiveKey(text string) bool {
	switch sequentialFullIdentity(sequentialFullProbeText(text)) {
	case "profile", "policy", "symbol", "timeframe", "riskusd", "maxnotionalusd":
		return true
	}
	return false
}
