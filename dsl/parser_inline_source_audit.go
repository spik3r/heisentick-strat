package dsl

import "strings"

// InlineSourcePrefixPreserved lets draft admission audit the historical inline
// splitter without changing Parse or ParseStrict. A recognized directive must
// not cause authored text before its first hit to be discarded. The section
// aliases, directive heads and setup-type filtering remain lexer-owned.
func InlineSourcePrefixPreserved(header, body string) bool {
	section, ok := sectionOpener(strings.TrimSpace(header) + " {")
	if !ok && isNamedStrategyHeader(header) {
		section, ok = "strategy", true
	}
	if !ok {
		return false
	}
	body = strings.TrimSpace(body)
	parts := splitInlineBody(body, directiveStartsForSection(section), section)
	return len(parts) == 0 || strings.HasPrefix(body, parts[0])
}
