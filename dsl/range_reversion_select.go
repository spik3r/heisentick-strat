package dsl

import "strings"

// Selection recognizes reserved family intent even when its source is malformed,
// so the legacy parser cannot silently fall back to a different family.
func rangeReversionSourceCandidate(source string) bool {
	tokens := frozenProbeTokens(source)
	section := ""
	depth := 0
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if t.quoted {
			continue
		}
		if t.text == "{" {
			depth++
			if i > 0 && strings.EqualFold(tokens[i-1].text, "setup") {
				section = "setup"
			}
			if i > 0 && strings.EqualFold(tokens[i-1].text, "market") {
				section = "market"
			}
			continue
		}
		if t.text == "}" {
			depth--
			if depth <= 0 {
				section = ""
				depth = 0
			}
			continue
		}
		if strings.EqualFold(t.text, "rangereversion") {
			return true
		}
		if section != "setup" || !strings.EqualFold(t.text, "type") {
			continue
		}
		j := i + 1
		if j < len(tokens) && tokens[j].text == ":" {
			j++
		}
		if j+1 < len(tokens) && !tokens[j].quoted && !tokens[j+1].quoted && strings.EqualFold(tokens[j].text, "range") && strings.EqualFold(tokens[j+1].text, "reversion") {
			return true
		}
	}
	return false
}
