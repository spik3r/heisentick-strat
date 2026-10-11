package dsl

// AuthoredDirective is a copied view of a logical statement consumed by the
// ordinary parser. It is not a replacement config or a dedicated-family AST.
type AuthoredDirective struct {
	Section string
	Tokens  []string
	Line    int
	Column  int
}

// OrdinarySourceDirectives uses the exact lexical pipeline in parser.parse.
// Callers first use Parse for language validation and semantic configuration;
// this view retains authored intent which a final config can erase. Dedicated
// family scanners are outside this API. The caller owns source admission limits.
func OrdinarySourceDirectives(source string) []AuthoredDirective {
	return ordinaryDirectives(expandPhysicalLines(source))
}

// AuthoredCoverageIssue records noncomment source discarded by the ordinary
// lexical pipeline, or a residual structural brace that normalization erases.
// Locations use the ordinary lexer's one-based byte columns.
type AuthoredCoverageIssue struct {
	Kind   string
	Text   string
	Line   int
	Column int
}

// OrdinarySourceAudit keeps the parser's logical view and its source-coverage
// limitations together. A caller requiring full authored coverage rejects any
// Issues before checking its supported directives. This does not validate DSL.
type OrdinarySourceAudit struct {
	Directives []AuthoredDirective
	Issues     []AuthoredCoverageIssue
}

// AuditOrdinarySource instruments the existing ordinary lexer. Parse behavior,
// configuration and diagnostics are unchanged; dedicated scanners are excluded.
func AuditOrdinarySource(source string) OrdinarySourceAudit {
	audit := OrdinarySourceAudit{Issues: []AuthoredCoverageIssue{}}
	lines := expandPhysicalLinesWithCoverage(source, func(kind, text string, line, column int) {
		audit.Issues = append(audit.Issues, AuthoredCoverageIssue{kind, text, line, column})
	})
	audit.Directives = ordinaryDirectives(lines)
	return audit
}

func ordinaryDirectives(lines []logicalLine) []AuthoredDirective {
	out := []AuthoredDirective{}
	for _, line := range joinWhenLines(lines) {
		tokens := tokenize(normalizeLine(line.text))
		if len(tokens) == 0 {
			continue
		}
		out = append(out, AuthoredDirective{line.section, append([]string(nil), tokens...), line.line, line.headCol})
	}
	return out
}
