package dsl

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

func expandPhysicalLines(source string) []logicalLine {
	rawLines := strings.Split(source, "\n")
	lines := []logicalLine{}
	section := ""
	for i, raw := range rawLines {
		lineNo := i + 1
		stripped := strings.TrimSpace(stripComment(raw))
		if stripped == "" {
			continue
		}
		if stripped == "}" {
			section = ""
			continue
		}
		if expanded, ok := expandInlineLine(raw, stripped, lineNo, section); ok {
			for _, line := range expanded {
				if line.section != "" {
					section = line.section
				}
				lines = append(lines, line)
			}
			if strings.Contains(stripped, "}") {
				section = ""
			}
			continue
		}
		if opener, ok := sectionOpener(stripped); ok {
			section = opener
			if opener == "strategy" && strings.Contains(stripped, "{") {
				lines = append(lines, logicalLine{
					text:    strings.TrimSpace(strings.TrimSuffix(stripped, "{")),
					line:    lineNo,
					headCol: firstNonSpaceCol(raw),
					section: section,
				})
			}
			continue
		}
		lines = append(lines, logicalLine{
			text:    stripped,
			line:    lineNo,
			headCol: firstNonSpaceCol(raw),
			section: section,
		})
	}
	return lines
}

func expandInlineLine(raw string, stripped string, lineNo int, currentSection string) ([]logicalLine, bool) {
	open := strings.Index(stripped, "{")
	close := strings.LastIndex(stripped, "}")
	if open < 0 || close < open {
		return nil, false
	}
	prefix := strings.TrimSpace(stripped[:open])
	body := strings.TrimSpace(stripped[open+1 : close])
	section, ok := sectionOpener(prefix + " {")
	if !ok {
		return nil, false
	}
	lines := []logicalLine{}
	if strings.HasPrefix(strings.ToLower(prefix), "strategy") {
		lines = append(lines, logicalLine{text: prefix, line: lineNo, headCol: columnOf(raw, "strategy"), section: section})
	}
	starts := directiveStartsForSection(section)
	for _, part := range splitInlineBody(body, starts, section) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lines = append(lines, logicalLine{
			text:    part,
			line:    lineNo,
			headCol: columnOf(raw, strings.Fields(part)[0]),
			section: section,
		})
	}
	if len(lines) == 0 && currentSection != "" {
		return nil, false
	}
	return lines, true
}

func joinWhenLines(lines []logicalLine) []logicalLine {
	joined := []logicalLine{}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(line.text)), "when ") {
			joined = append(joined, line)
			continue
		}
		text := line.text
		balance := parenBalance(text)
		for i+1 < len(lines) && (balance != 0 || !strings.Contains(strings.ToLower(text), " then ")) {
			i++
			text += " " + strings.TrimSpace(lines[i].text)
			balance = parenBalance(text)
		}
		line.text = text
		joined = append(joined, line)
	}
	return joined
}

func stripComment(line string) string {
	if idx := strings.Index(line, "#"); idx >= 0 {
		return line[:idx]
	}
	return line
}

func normalizeLine(line string) string {
	replacer := strings.NewReplacer("->", " enter ")
	line = replacer.Replace(line)
	line = regexp.MustCompile(`\b([A-Za-z][A-Za-z0-9_]*)\s*:`).ReplaceAllString(line, "$1 ")
	for _, ch := range []string{"(", ")", ",", "{", "}"} {
		line = strings.ReplaceAll(line, ch, " ")
	}
	line = strings.ReplaceAll(line, "<=", " <= ")
	line = strings.ReplaceAll(line, "+", " + ")
	return strings.Join(strings.Fields(line), " ")
}

func tokenize(line string) []string {
	return strings.Fields(line)
}

func sectionOpener(line string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(line))
	lower = strings.TrimSuffix(lower, "{")
	lower = strings.TrimSuffix(lower, ":")
	lower = strings.TrimSpace(lower)
	if strings.HasPrefix(lower, "strategy") {
		return "strategy", true
	}
	if canonical, ok := generatedSectionAliases[lower]; ok {
		return canonical, true
	}
	return "", false
}

func directiveStartsForSection(section string) []string {
	starts := []string{"description", "slices", "symbols", "timeframes", "sessions", "windows", "day", "prior", "movement", "maxMovementEr", "trendiness", "priority", "near", "nearKeyLevel", "range", "channel", "higher", "side", "direction", "trigger", "rejection", "candle", "tail", "close", "entry", "when", "enter", "stop", "target", "take", "minimum", "fallback", "move", "partial", "trail", "wait", "maxHoldCandles", "maxHoldBars", "risk", "riskUsd", "context", "location", "require", "size"}
	if section == "filters" {
		starts = append(starts, "source")
	}
	if section == "strategy" {
		return []string{"description", "name"}
	}
	if section == "setup" {
		starts = append(starts, "type", "departure", "consolidation", "zone", "structure", "patterns", "pattern", "base", "source", "impulse", "displacement minimum", "gap minimum", "retest", "choch", "lookback", "neutral", "swing", "ema")
	}
	return starts
}

type inlineHit struct {
	index int
	word  string
}

func splitInlineBody(body string, starts []string, section string) []string {
	hits := []inlineHit{}
	depth := 0
	lower := strings.ToLower(body)
	for i, r := range body {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
		if depth != 0 || (i > 0 && !unicode.IsSpace(rune(body[i-1]))) {
			continue
		}
		for _, start := range starts {
			startLower := strings.ToLower(start)
			if strings.HasPrefix(lower[i:], startLower) && isBoundary(body, i+len(start)) {
				if section == "filters" && startLower == "source" && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(body[i+len(start):])), "timeframe") {
					continue
				}
				hits = append(hits, inlineHit{index: i, word: start})
				break
			}
		}
	}
	hits = filterInlineSetupTypeHits(body, hits)
	if len(hits) == 0 {
		return []string{body}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].index < hits[j].index })
	parts := []string{}
	for i, hit := range hits {
		end := len(body)
		if i+1 < len(hits) {
			end = hits[i+1].index
		}
		parts = append(parts, body[hit.index:end])
	}
	return parts
}

func filterInlineSetupTypeHits(body string, hits []inlineHit) []inlineHit {
	type span struct {
		start int
		end   int
	}
	spans := []span{}
	for _, hit := range hits {
		if strings.EqualFold(hit.word, "type") {
			if end := setupTypeValueEnd(body, hit.index); end > hit.index {
				spans = append(spans, span{start: hit.index, end: end})
			}
		}
		if strings.EqualFold(hit.word, "gap minimum") || strings.EqualFold(hit.word, "displacement minimum") {
			spans = append(spans, span{start: hit.index, end: hit.index + len(hit.word)})
		}
	}
	if len(spans) == 0 {
		return hits
	}
	filtered := hits[:0]
	for _, hit := range hits {
		insideTypeValue := false
		for _, span := range spans {
			if hit.index > span.start && hit.index < span.end {
				insideTypeValue = true
				break
			}
		}
		if !insideTypeValue {
			filtered = append(filtered, hit)
		}
	}
	return filtered
}

func setupTypeValueEnd(body string, pos int) int {
	rest := body[pos:]
	lowerRest := strings.ToLower(rest)
	if !strings.HasPrefix(lowerRest, "type") || !isBoundary(body, pos+len("type")) {
		return pos
	}
	i := pos + len("type")
	for i < len(body) && unicode.IsSpace(rune(body[i])) {
		i++
	}
	if i < len(body) && body[i] == ':' {
		i++
	}
	for i < len(body) && unicode.IsSpace(rune(body[i])) {
		i++
	}
	tail := strings.ToLower(body[i:])
	for _, phrase := range fairValueGapSetupTypePhrases {
		if strings.HasPrefix(tail, phrase) && isBoundary(body, i+len(phrase)) {
			return i + len(phrase)
		}
	}
	for _, phrase := range inlineSetupTypePhrases {
		if strings.HasPrefix(tail, phrase) && isBoundary(body, i+len(phrase)) {
			return i + len(phrase)
		}
	}
	return pos
}

func isBoundary(text string, index int) bool {
	if index >= len(text) {
		return true
	}
	r := rune(text[index])
	return unicode.IsSpace(r) || r == '(' || r == ':'
}

func parenBalance(text string) int {
	balance := 0
	for _, r := range text {
		switch r {
		case '(':
			balance++
		case ')':
			balance--
		}
	}
	return balance
}

func firstNonSpaceCol(text string) int {
	for i, r := range text {
		if !unicode.IsSpace(r) {
			return i + 1
		}
	}
	return 1
}

func columnOf(text string, needle string) int {
	idx := strings.Index(strings.ToLower(text), strings.ToLower(needle))
	if idx < 0 {
		return firstNonSpaceCol(text)
	}
	return idx + 1
}
