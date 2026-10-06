package dsl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func frozenParserSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/frozen_level/example.strat")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func frozenParseOK(t *testing.T, source string) Config {
	t.Helper()
	r, err := Parse(source)
	if err != nil || len(r.Errors) != 0 {
		t.Fatalf("parse: %v %v", err, r.Errors)
	}
	if !reflect.DeepEqual(r.Warnings, []string{FrozenLevelExecutionUnimplemented}) || len(r.Diagnostics) != 1 || r.Diagnostics[0].Severity != DiagnosticWarning {
		t.Fatalf("missing compile-only warning: %+v", r)
	}
	return r.Config
}

func TestFrozenParserHandDerivedIdentity(t *testing.T) {
	cfg := frozenParseOK(t, frozenParserSource(t))
	id, err := FrozenLevelIdentity(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct {
		file string
		got  []byte
	}{{"config.canonical.json", id.ConfigBytes}, {"policy.canonical.json", id.PolicyBytes}} {
		want, err := os.ReadFile("testdata/frozen_level/" + pair.file)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(want, pair.got) {
			t.Fatalf("%s differs from reviewed hand-derived bytes", pair.file)
		}
	}
	if id.ConfigDigest != "55c56c3b9d50bdd3a026a77117d36033a276e4bfd15b49c43bd59a1f3194df8e" || id.PolicyDigest != "9b670f1a12f6e317234a5b3a0f8c77e4d00d3a9ba46bac618f2ccb3b74b6170d" {
		t.Fatalf("wrong identities: %+v", id)
	}
	// A stable trace from the real parser and helper is compared byte-for-byte
	// by the native/WASM test runner, including standard Go JSON escaping.
	out, _ := json.Marshal(struct{ Config, Policy string }{string(id.ConfigBytes), string(id.PolicyBytes)})
	fmt.Println("FROZEN_CONFIG_TRACE=" + string(out))
}

func TestFrozenParserEquivalentSyntaxAndExplicitArms(t *testing.T) {
	source := frozenParserSource(t)
	want := frozenParseOK(t, source)
	starts := []int{len("dsl v7\n")}
	for _, name := range []string{"market conditions", "setup", "risk", "management", "execution"} {
		starts = append(starts, strings.Index(source, "\n"+name+" {")+1)
	}
	starts = append(starts, len(source))
	reordered := "dsl v7\n"
	for _, i := range []int{5, 3, 1, 4, 2, 0} {
		reordered += source[starts[i]:starts[i+1]]
	}
	for name, s := range map[string]string{
		"block order":   reordered,
		"inline":        strings.ReplaceAll(source, "\n", " "),
		"comments":      "# frozen lock none\n" + strings.ReplaceAll(source, "\n", " # ignored } { type: frozen level breakout\n"),
		"decimal alias": strings.Replace(source, "grid .01", "grid 0.01", 1),
		"keyword case":  strings.ReplaceAll(strings.ReplaceAll(source, "\n  frozen ", "\n  FROZEN "), "type:", "TYPE:"),
		"type first":    strings.Replace(source, "frozen control stored-m30-lock-isolation-v1\n  type: frozen level breakout", "type: frozen level breakout frozen control stored-m30-lock-isolation-v1", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if got := frozenParseOK(t, s); !reflect.DeepEqual(got, want) {
				t.Fatal("equivalent syntax changed config")
			}
		})
	}
	for _, arm := range []string{"none", "scale", "pivot"} {
		cfg := frozenParseOK(t, strings.Replace(source, "frozen lock scale", "frozen lock "+arm, 1))
		if cfg["frozenLevelBreakout"].(map[string]any)["lockMode"] != arm {
			t.Fatal(arm)
		}
	}
	without := strings.Replace(source, `  description "Synthetic # { } \\ frozen lock pivot \u2028 \u2029"`+"\n", "", 1)
	empty := strings.Replace(source, `description "Synthetic # { } \\ frozen lock pivot \u2028 \u2029"`, `description ""`, 1)
	if without == source || empty == source {
		t.Fatal("description mutation missed source")
	}
	if !reflect.DeepEqual(frozenParseOK(t, without), frozenParseOK(t, empty)) {
		t.Fatal("omitted description differs from explicit empty")
	}
	limits := strings.NewReplacer("timeout 60", "timeout 150119987579", "predecessor 5000", "predecessor 9007199254740991", "receipt 5000", "receipt 9007199254740991", "entry 0 ms", "entry 9007199254740991 ms", "amendment 0 ms", "amendment 9007199254740991 ms").Replace(source)
	spec := frozenParseOK(t, limits)["frozenLevelBreakout"].(map[string]any)
	if spec["entryLatencyMS"] != int64(9007199254740991) || spec["timeoutMinutes"] != int64(150119987579) {
		t.Fatal("parser lost maximum exact integer bounds")
	}
}

func TestFrozenParserRejectsEveryUnaccountedFragment(t *testing.T) {
	source := frozenParserSource(t)
	cases := map[string]string{
		"old version": strings.Replace(source, "dsl v7", "dsl v6", 1), "double header": "dsl v7\n" + source,
		"missing header": strings.TrimPrefix(source, "dsl v7\n"), "header last": strings.TrimPrefix(source, "dsl v7\n") + "dsl v7",
		"unknown outside": source + "what", "extra block": source + "setup {}", "unknown block": source + "filters {}",
		"nested": strings.Replace(source, "risk {", "risk { risk { }", 1), "unclosed": strings.TrimSuffix(source, "}\n"),
		"empty name":   strings.Replace(source, `"Gold \"\u003c\u0026\u003e\" Ω"`, `""`, 1),
		"single quote": strings.Replace(source, `"Gold \"\u003c\u0026\u003e\" Ω"`, `'name'`, 1),
		"backtick":     strings.Replace(source, `"Gold \"\u003c\u0026\u003e\" Ω"`, "`name`", 1),
		"surrogate":    strings.Replace(source, `\u003c`, `\ud800`, 1), "utf8": source + string([]byte{0xff}),
		"json comment": source + "// comment", "block comment": source + "/* comment */",
		"unknown profile":                      strings.Replace(source, "stored-m30-lock-isolation-v1", "stored-m30-lock-isolation-v2", 1),
		"old management":                       strings.Replace(source, "frozen lock scale", "partial 0.5 at 1 R frozen lock scale", 1),
		"old setup leading":                    strings.Replace(source, "type: frozen", "side long type: frozen", 1),
		"old setup trailing":                   strings.Replace(source, "type: frozen level breakout", "type: frozen level breakout stop 1 ATR", 1),
		"unknown arm":                          strings.Replace(source, "frozen lock scale", "frozen lock target", 1),
		"unquoted inline metadata and relabel": `dsl v7 strategy "x" { description free prose } setup { type: frozen level breakout type: flag continuation }`,
		"misplaced":                            strings.Replace(source, "frozen grid .01 price units", "frozen lock scale", 1),
		"zero age":                             strings.Replace(source, "predecessor 5000", "predecessor 0", 1),
		"negative latency":                     strings.Replace(source, "entry 0 ms", "entry -1 ms", 1),
		"timeout overflow":                     strings.Replace(source, "timeout 60", "timeout 150119987580", 1),
		"age overflow":                         strings.Replace(source, "receipt 5000", "receipt 9007199254740992", 1),
		"reference path":                       strings.Replace(source, "FixtureSource", "../FixtureSource", 1),
		"reference escaped newline":            strings.Replace(source, "FixtureSource", `FixtureSource\n`, 1),
		"digest upper":                         strings.Replace(source, "70f1259596", "70F1259596", 1),
		"string newline":                       strings.Replace(source, `Synthetic #`, "Synthetic\n#", 1),
		"too large":                            source + strings.Repeat(" ", 1<<20),
	}
	for _, value := range []string{"0", "01", "1.", "+1", "-0", "1e0", `"1"`, "１", "NaN", "Inf", "1" + strings.Repeat("0", 400), "." + strings.Repeat("0", 400) + "1"} {
		cases["decimal "+value] = strings.Replace(source, "activation 1 R", "activation "+value+" R", 1)
	}
	for _, line := range strings.Split(source, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "frozen ") || strings.HasPrefix(trim, "type:") {
			cases["missing "+trim] = strings.Replace(source, line+"\n", "", 1)
			cases["duplicate "+trim] = strings.Replace(source, line, line+" "+trim, 1)
		}
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			if s == source {
				t.Fatal("mutation ineffective")
			}
			r, err := Parse(s)
			if err != nil || len(r.Errors) == 0 || len(r.Config) != 0 || len(r.Warnings) != 0 {
				t.Fatalf("invalid source returned usable config: %v %+v", err, r)
			}
			if _, err := FrozenLevelIdentity(r.Config); err == nil {
				t.Fatal("invalid source got identity")
			}
		})
	}
}

func TestFrozenProbePreservesLegacyQuotedMetadata(t *testing.T) {
	for _, quote := range []string{`"`, "'", "`"} {
		source := "dsl v7\nstrategy " + quote + "frozen level breakout" + quote + "\nsetup { type: clock range breakout range 11:05 to 14:05 orders expire 03:00 buffer 0 pips }\ndescription " + quote + "frozen lock pivot # { type: frozen level breakout }" + quote
		if frozenSourceCandidate(source) {
			t.Fatal("quoted old-family metadata selected new scanner")
		}
		old := newParser(source)
		old.parse()
		want := old.result()
		got, err := Parse(source)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("changed legacy %q metadata", quote)
		}
	}
	for _, metadata := range []string{"strategy frozen", "strategy frozen-control", "description frozen lock pivot"} {
		source := "dsl v7\n" + metadata + "\nsetup { type: flag continuation }"
		if frozenSourceCandidate(source) {
			t.Fatalf("legacy free metadata selected frozen grammar: %q", metadata)
		}
		old := newParser(source)
		old.parse()
		want := old.result()
		got, err := Parse(source)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("changed legacy free metadata: %q", metadata)
		}
	}
}

func TestFrozenGrammarManifestBinding(t *testing.T) {
	raw, err := os.ReadFile("../spec/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if GrammarManifestSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("generated manifest hash differs")
	}
	if generatedCanonicalSetupFamilies["frozen level breakout"] != string(FamilyFrozenLevelBreakout) {
		t.Fatal("missing generated family")
	}
	var manifest struct {
		Aliases        []struct{ Source, Spelling, FamilyID string }
		DirectiveHeads []struct{ Spelling, Classification string }
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	canonical := map[string]string{}
	var inline, heads []string
	for _, a := range manifest.Aliases {
		switch a.Source {
		case "go-canonical-setup-type":
			canonical[strings.ToLower(a.Spelling)] = a.FamilyID
		case "go-inline-setup-type":
			inline = append(inline, a.Spelling)
		}
	}
	for _, h := range manifest.DirectiveHeads {
		if h.Classification != "js-only" {
			heads = append(heads, h.Spelling)
		}
	}
	if !reflect.DeepEqual(canonical, generatedCanonicalSetupFamilies) || !reflect.DeepEqual(inline, inlineSetupTypePhrases) || !reflect.DeepEqual(heads, generatedDirectiveHeads) {
		t.Fatal("generated family/head tables differ from manifest")
	}
}
