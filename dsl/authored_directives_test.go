package dsl

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOrdinarySourceDirectivesMatchActualParser(t *testing.T) {
	paths, err := filepath.Glob("../conformance/parse/*.strat")
	if err != nil || len(paths) == 0 {
		t.Fatal("missing parser fixtures", err)
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		testOrdinaryDirectiveWitness(t, string(raw))
	}
	for _, source := range []string{
		"dsl v7\nstrategy \"target partial\" { description \"stop trail\" }\nmarket conditions { symbols(XAUUSD) timeframes(15m) }\nsetup { type: vwap extension fade }\n",
		"# comment\ndsl\tv7\nstrategy 'alias'\nfilters:\n side both # comment\n}\nsetup { type: vwap extension fade }\ntarget { take profit at opposite range edge }\n",
		"dsl v7\nsetup { type: vwap extension fade }\nwhen (close >\n open) then enter long\nmanagement { partial 0.5 at 1R trail off }\n",
	} {
		testOrdinaryDirectiveWitness(t, source)
	}
}

func testOrdinaryDirectiveWitness(t *testing.T, source string) {
	t.Helper()
	parser := newParser(source)
	parser.parse()
	var got []string
	view := OrdinarySourceDirectives(source)
	audit := AuditOrdinarySource(source)
	if !reflect.DeepEqual(view, audit.Directives) {
		t.Fatal("audited view changed ordinary directives")
	}
	withAudit := expandPhysicalLinesWithCoverage(source, func(string, string, int, int) {})
	if !reflect.DeepEqual(expandPhysicalLines(source), withAudit) {
		t.Fatal("audit changed ordinary expansion")
	}
	for _, d := range view {
		if d.Line < 1 || d.Column < 1 || len(d.Tokens) == 0 {
			t.Fatal("invalid source location", d)
		}
		got = append(got, d.Section+"|"+strings.ToLower(d.Tokens[0]))
	}
	if !reflect.DeepEqual(got, parser.legacySetup9Audit.authored) {
		t.Fatalf("directive view differs from actual apply calls: %v != %v", got, parser.legacySetup9Audit.authored)
	}
	before := parser.result()
	if len(view) > 0 {
		view[0].Tokens[0] = "mutated"
	}
	again := OrdinarySourceDirectives(source)
	if len(again) > 0 && again[0].Tokens[0] == "mutated" {
		t.Fatal("view storage leaked")
	}
	if !reflect.DeepEqual(before, parser.result()) {
		t.Fatal("inspection changed parser result")
	}
}

func TestOrdinarySourceDirectivesRetainErasedIntent(t *testing.T) {
	base := "dsl v7\nsetup { type: vwap extension fade }\n"
	mutated := base + "target { take profit at opposite range edge }\n"
	a, _ := Parse(base)
	b, _ := Parse(mutated)
	if len(a.Errors) != 0 || len(b.Errors) != 0 || !reflect.DeepEqual(a.Config, b.Config) {
		t.Fatal("erased-intent regression premise changed")
	}
	if reflect.DeepEqual(OrdinarySourceDirectives(base), OrdinarySourceDirectives(mutated)) {
		t.Fatal("authored intent erased")
	}
}

func TestAuditOrdinarySourceCoverage(t *testing.T) {
	for _, test := range []struct{ source, kind, text string }{
		{"filters { approach 0 ATR side both }", "inline-leading-content", "approach 0 ATR"},
		{"management { breakeven after 1R trail off }", "inline-leading-content", "breakeven after 1R"},
		{"management { breakeven off } partial 0.5 at 1R", "inline-trailing-content", "partial 0.5 at 1R"},
		{"market conditions { weekday monday sessions london }", "inline-leading-content", "weekday monday"},
		{"\tfilters {  ?! side both } # comment", "inline-leading-content", "?!"},
		{"\tfilters { side both }   ()", "inline-trailing-content", "()"},
		{"{}", "residual-brace", "{"},
		{"management { trail off } }", "residual-brace", "}"},
		{"filters { side both } { side long }", "residual-brace", "}"},
	} {
		audit := AuditOrdinarySource(test.source)
		if len(audit.Issues) == 0 || audit.Issues[0].Kind != test.kind || audit.Issues[0].Text != test.text {
			t.Fatalf("missing coverage: %q %+v", test.source, audit.Issues)
		}
		if audit.Issues[0].Line != 1 || audit.Issues[0].Column != strings.Index(test.source, test.text)+1 {
			t.Fatalf("wrong coverage location: %q %+v", test.source, audit.Issues[0])
		}
		testOrdinaryDirectiveWitness(t, test.source)
	}
	for _, source := range []string{
		"dsl v7\r\n# comment\r\nfilters { side both } # suffix comment\r\n",
		"strategy \"sample\" { description \"plain metadata\" }\nsetup { type: vwap extension fade }",
		"filters { \t side both \t } \t ",
		"filters { }", "filters { unlisted 7 }", "filters {\n side both\n}\n",
		"strategy \"sample\"\n description \"literal { braces }\"\n",
	} {
		if audit := AuditOrdinarySource(source); len(audit.Issues) != 0 {
			t.Fatalf("false coverage issue: %q %+v", source, audit.Issues)
		}
		testOrdinaryDirectiveWitness(t, source)
	}
	copyAudit := AuditOrdinarySource("filters { discarded side both }")
	copyAudit.Issues[0].Text = "changed"
	copyAudit.Directives[0].Tokens[0] = "changed"
	fresh := AuditOrdinarySource("filters { discarded side both }")
	if fresh.Issues[0].Text != "discarded" || fresh.Directives[0].Tokens[0] != "side" {
		t.Fatal("coverage storage leaked")
	}
	// Generalized coverage across the lexer's own recognized heads, not a
	// second list of directives maintained by the admission layer.
	for _, section := range []string{"strategy", "market", "setup", "filters", "risk", "target", "management", "execution"} {
		for _, head := range directiveStartsForSection(section) {
			tail := "value"
			if section == "filters" && head == "source" {
				tail = "timeframe 15m"
			}
			source := section + " { __unconsumed__ " + head + " " + tail + " }"
			audit := AuditOrdinarySource(source)
			if len(audit.Issues) == 0 || audit.Issues[0].Text != "__unconsumed__" {
				t.Fatalf("lost prefix: %q %+v", source, audit.Issues)
			}
			testOrdinaryDirectiveWitness(t, source)
		}
	}
}

func TestOrdinaryQuoteTransitionCompatibility(t *testing.T) {
	for _, source := range []string{"plain", `"quoted { braces }"`, `'single " double'`, `"backslash \" unchanged"`, "é🌱'{}'é", `"unclosed {`, "''\"\"{}"} {
		for index := -1; index <= len(source)+1; index++ {
			var old byte
			for i := 0; i < index && i < len(source); i++ {
				switch {
				case old == 0 && (source[i] == '"' || source[i] == '\''):
					old = source[i]
				case old != 0 && source[i] == old:
					old = 0
				}
			}
			if inQuotedText(source, index) != (old != 0) {
				t.Fatal("changed ordinary quote behavior", source, index)
			}
		}
	}
}

func TestAuditOrdinarySourceBoundedLargeQuotedMetadata(t *testing.T) {
	// Before the linear walk, this valid 256KiB metadata body needed roughly
	// a minute. Keep a generous wall bound so that a quadratic regression fails
	// in bounded time without imposing a tight platform-specific timing oracle.
	source := "strategy \"large metadata\"\n description \"literal " + strings.Repeat("{}", 128*1024) + "\"\n"
	done := make(chan OrdinarySourceAudit, 1)
	go func() { done <- AuditOrdinarySource(source) }()
	select {
	case audit := <-done:
		if len(audit.Issues) != 0 || len(audit.Directives) != 2 {
			t.Fatal("valid quoted metadata lost", len(audit.Issues))
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ordinary coverage audit exceeded bounded large-metadata regression budget")
	}
}
