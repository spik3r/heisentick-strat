package dsl

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/testsupport"
)

// These checks consume the static contract and exercise the real parser. They
// neither implement an authoring parser nor regenerate protected expectations.
func authoringMetadata(t *testing.T) map[string]any {
	t.Helper()
	return loadContractJSON(t, filepath.Join(testsupport.MustRepoRoot(), "spec", "authoring-v1.json")).(map[string]any)
}

func authoringObjects(value any) []map[string]any {
	var out []map[string]any
	for _, item := range value.([]any) {
		out = append(out, item.(map[string]any))
	}
	return out
}

func authoringValidate(t *testing.T, doc map[string]any) error {
	t.Helper()
	schema, schemaPath := loadContractSchema(t, "authoring-v1.schema.json")
	if err := validateContract(doc, schema, schemaPath, schema, "authoring"); err != nil {
		return err
	}
	root := testsupport.MustRepoRoot()
	manifest := loadContractJSON(t, filepath.Join(root, "spec", "manifest.json")).(map[string]any)
	// Inventory membership and canonical spelling come from the producer manifest.
	for _, key := range []string{"sections", "families"} {
		want := map[string]map[string]any{}
		for _, item := range authoringObjects(manifest[key]) {
			id := item["id"].(string)
			coverage := "unknown"
			if id == "smaGoldenCross" || id == "section.setup" {
				coverage = "partial"
			}
			row := map[string]any{"id": id, "coverage": coverage}
			if key == "sections" {
				canonical := item["canonical"]
				if override, ok := item["goCanonical"]; ok {
					canonical = override
				}
				row["spelling"], row["canonical"] = item["spelling"], canonical
			}
			want[id] = row
		}
		last := ""
		for _, row := range authoringObjects(doc[key]) {
			id := row["id"].(string)
			if id <= last || !reflect.DeepEqual(row, want[id]) {
				return fmt.Errorf("%s: duplicate, unsorted or manifest drift: %s", key, id)
			}
			last = id
			delete(want, id)
		}
		if len(want) != 0 {
			return fmt.Errorf("%s: missing manifest entries", key)
		}
	}
	sources := map[string]bool{}
	for _, row := range authoringObjects(doc["grammarSource"].(map[string]any)["files"]) {
		path := row["path"].(string)
		if sources[path] || filepath.IsAbs(path) || filepath.Clean(path) != path || strings.HasPrefix(path, "../") {
			return fmt.Errorf("invalid source path %s", path)
		}
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != row["sha256"] {
			return fmt.Errorf("grammar source drift: %s", path)
		}
		sources[path] = true
	}
	requiredSources := []string{"spec/manifest.json", "dsl/grammar_tables_generated.go", "dsl/parser.go", "dsl/parser_lexing.go", "dsl/parser_values.go", "dsl/parser_setups.go", "dsl/parser_sma_golden_cross.go", "dsl/parser_sma_golden_cross_validation.go"}
	if len(sources) != len(requiredSources) {
		return fmt.Errorf("source inventory changed")
	}
	for _, path := range requiredSources {
		if !sources[path] {
			return fmt.Errorf("missing source %s", path)
		}
	}
	aliases := []string{}
	for _, row := range authoringObjects(manifest["aliases"]) {
		if row["familyId"] == "smaGoldenCross" && row["source"] == "go-canonical-setup-type" {
			aliases = append(aliases, row["spelling"].(string))
		}
	}
	sort.Strings(aliases)
	ids := map[string]bool{}
	last := ""
	defaults, err := Parse("dsl v7\nsetup { type: sma golden cross }\n")
	if err != nil || len(defaults.Errors) != 0 {
		return fmt.Errorf("default parser failed: %v", err)
	}
	fields := map[string]string{"type": "setupType", "fast": "fastSmaLen", "slow": "slowSmaLen", "atr": "atrLen", "stop": "stopAtr", "target": "targetAtr"}
	helpBytes, err := os.ReadFile(filepath.Join(root, "spec/authoring-metadata.md"))
	if err != nil {
		return err
	}
	for _, d := range authoringObjects(doc["directives"]) {
		id := d["id"].(string)
		key := strings.TrimPrefix(id, "setup.smaGoldenCross.")
		field, ok := fields[key]
		if !ok || id <= last || ids[id] || d["family"] != "smaGoldenCross" || d["section"] != "section.setup" {
			return fmt.Errorf("bad directive identity/reference %s", id)
		}
		last, ids[id] = id, true
		if d["help"] != "spec/authoring-metadata.md#"+key || !bytes.Contains(helpBytes, []byte("\n## "+key+"\n")) {
			return fmt.Errorf("bad help reference %s", id)
		}
		for _, path := range schemaStringSlice(d["sourcePaths"]) {
			if !sources[path] {
				return fmt.Errorf("bad source reference %s", path)
			}
		}
		args := authoringObjects(d["arguments"])
		if len(args) != 1 {
			return fmt.Errorf("%s: one argument expected", id)
		}
		a := args[0]
		prefix, after, arity := []string{"sma", key}, []string{"setup.smaGoldenCross.type"}, []any{float64(3)}
		presence, kind, lexical, unit, bound := "optional-bundle", "integer", "finite-decimal", "bars", "no-parser-maximum"
		var unitToken, minimum any = nil, float64(0)
		var def any
		path := "smaGoldenCross." + field
		switch key {
		case "type":
			prefix, after, arity = []string{"type"}, []string{}, []any{float64(2), float64(4)}
			presence, kind, lexical, unit, bound = "family-selector", "enum", "family-alias", "none", "not-applicable"
			minimum, path = nil, "setupType"
			if !reflect.DeepEqual(schemaStringSlice(a["enum"]), aliases) {
				return fmt.Errorf("family aliases drift")
			}
		case "fast", "slow":
			presence, lexical, bound = "optional-default", "legacy-number", "go-int-roundtrip"
			def = defaults.Config["smaGoldenCross"].(map[string]any)[field]
		case "stop", "target":
			kind, unit, unitToken, arity = "number", "ATR", "ATR", []any{float64(4)}
		}
		if key != "type" && len(a["enum"].([]any)) != 0 {
			return fmt.Errorf("unexpected numeric enum")
		}
		if !reflect.DeepEqual(schemaStringSlice(d["prefix"]), prefix) || !reflect.DeepEqual(schemaStringSlice(d["after"]), after) || !reflect.DeepEqual(d["arity"], arity) || d["presence"] != presence || d["configPath"] != path || !reflect.DeepEqual(d["default"], def) {
			return fmt.Errorf("signature/default drift: %s", id)
		}
		if a["type"] != kind || a["lexical"] != lexical || a["unit"] != unit || a["unitToken"] != unitToken || a["exclusiveMinimum"] != minimum || a["upperBound"] != bound || a["maximum"] != nil || a["position"] != float64(0) {
			return fmt.Errorf("argument contract drift: %s", id)
		}
		if key != "type" && def == nil {
			if _, exists := defaults.Config["smaGoldenCross"].(map[string]any)[field]; exists {
				return fmt.Errorf("optional field acquired default: %s", key)
			}
		}
	}
	if len(ids) != len(fields) {
		return fmt.Errorf("missing directive")
	}
	wantConstraints := []any{
		map[string]any{"id": "smaGoldenCross.fast-less-than-slow", "kind": "less-than", "directives": []any{"setup.smaGoldenCross.fast", "setup.smaGoldenCross.slow"}},
		map[string]any{"id": "smaGoldenCross.protected-bundle", "kind": "all-or-none", "directives": []any{"setup.smaGoldenCross.atr", "setup.smaGoldenCross.stop", "setup.smaGoldenCross.target"}},
	}
	if !reflect.DeepEqual(doc["constraints"], wantConstraints) {
		return fmt.Errorf("constraint identity/reference drift")
	}
	exampleIDs := map[string]bool{}
	for _, e := range authoringObjects(doc["examples"]) {
		id := e["id"].(string)
		if exampleIDs[id] {
			return fmt.Errorf("duplicate example %s", id)
		}
		exampleIDs[id] = true
		result, err := Parse(e["source"].(string))
		if err != nil || len(result.Errors) != 0 || result.Config["setupType"] != "smaGoldenCross" {
			return fmt.Errorf("example %s failed: %v %v", id, err, result.Errors)
		}
		params := result.Config["smaGoldenCross"].(map[string]any)
		for key, want := range e["expected"].(map[string]any) {
			if !reflect.DeepEqual(params[key], want) {
				return fmt.Errorf("example %s: %s mismatch", id, key)
			}
		}
	}
	return nil
}

func TestAuthoringMetadataContract(t *testing.T) {
	if err := authoringValidate(t, authoringMetadata(t)); err != nil {
		t.Fatal(err)
	}
}

// Reject duplicate object members too; encoding/json alone keeps the last one.
func authoringUniqueJSON(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delim != '{' && delim != '[' {
		return fmt.Errorf("unexpected delimiter")
	}
	keys := map[string]bool{}
	for dec.More() {
		if delim == '{' {
			key, err := dec.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || keys[name] {
				return fmt.Errorf("duplicate/non-string JSON key %v", key)
			}
			keys[name] = true
		}
		if err := authoringUniqueJSON(dec); err != nil {
			return err
		}
	}
	_, err = dec.Token()
	return err
}

func TestAuthoringMetadataDeterministicJSON(t *testing.T) {
	for _, path := range []string{"spec/authoring-v1.json", "spec/schemas/authoring-v1.schema.json"} {
		data, err := os.ReadFile(filepath.Join(testsupport.MustRepoRoot(), path))
		if err != nil {
			t.Fatal(err)
		}
		dec := json.NewDecoder(bytes.NewReader(data))
		if err := authoringUniqueJSON(dec); err != nil {
			t.Fatal(err)
		}
		if _, err := dec.Token(); err != io.EOF {
			t.Fatalf("%s: trailing data", path)
		}
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		var canonical bytes.Buffer
		enc := json.NewEncoder(&canonical)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		if err := enc.Encode(value); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, canonical.Bytes()) {
			t.Fatalf("%s: use sorted keys, two-space indentation and final newline", path)
		}
	}
	if err := authoringUniqueJSON(json.NewDecoder(strings.NewReader(`{"x":1,"x":2}`))); err == nil {
		t.Fatal("accepted duplicate JSON member")
	}
}

func TestAuthoringMetadataRejectsMutations(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"version", func(d map[string]any) { d["schemaVersion"] = float64(2) }},
		{"unknown-field", func(d map[string]any) { d["runtimeSupported"] = true }},
		{"missing-field", func(d map[string]any) { delete(d, "unlisted") }},
		{"nested-field", func(d map[string]any) { authoringObjects(d["directives"])[0]["extra"] = true }},
		{"duplicate-id", func(d map[string]any) { rows := d["directives"].([]any); d["directives"] = append(rows, rows[0]) }},
		{"bad-section", func(d map[string]any) { authoringObjects(d["directives"])[0]["section"] = "section.execution" }},
		{"bad-family", func(d map[string]any) { authoringObjects(d["directives"])[0]["family"] = "missing" }},
		{"bad-source", func(d map[string]any) { authoringObjects(d["directives"])[0]["sourcePaths"] = []any{"missing.go"} }},
		{"bad-help", func(d map[string]any) {
			authoringObjects(d["directives"])[0]["help"] = "spec/authoring-metadata.md#missing"
		}},
		{"bad-after", func(d map[string]any) { authoringObjects(d["directives"])[0]["after"] = []any{"missing"} }},
		{"missing-constraint", func(d map[string]any) { d["constraints"] = d["constraints"].([]any)[:1] }},
		{"missing-inventory", func(d map[string]any) { d["families"] = d["families"].([]any)[1:] }},
		{"invented-default", func(d map[string]any) { authoringObjects(d["directives"])[0]["default"] = float64(14) }},
		{"wrong-unit", func(d map[string]any) {
			authoringObjects(authoringObjects(d["directives"])[0]["arguments"])[0]["unit"] = "ATR"
		}},
		{"wrong-arity", func(d map[string]any) { authoringObjects(d["directives"])[0]["arity"] = []any{float64(4)} }},
		{"wrong-bound", func(d map[string]any) {
			authoringObjects(authoringObjects(d["directives"])[0]["arguments"])[0]["exclusiveMinimum"] = float64(-1)
		}},
		{"source-drift", func(d map[string]any) {
			authoringObjects(d["grammarSource"].(map[string]any)["files"])[0]["sha256"] = strings.Repeat("0", 64)
		}},
		{"example-drift", func(d map[string]any) {
			authoringObjects(d["examples"])[0]["expected"].(map[string]any)["fastSmaLen"] = float64(51)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := authoringMetadata(t)
			tc.mutate(d)
			if err := authoringValidate(t, d); err == nil {
				t.Fatal("accepted malformed metadata")
			}
		})
	}
}

func TestAuthoringMetadataSchemaRejectsRelaxations(t *testing.T) {
	schema, path := loadContractSchema(t, "authoring-v1.schema.json")
	if err := validateContract(authoringMetadata(t), schema, path, schema, "authoring"); err != nil {
		t.Fatalf("unmodified descriptor: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(doc, argument map[string]any)
	}{
		{"unknown-argument-property", func(doc, argument map[string]any) { argument["unexpected"] = true }},
		{"optional-argument", func(doc, argument map[string]any) { argument["required"] = false }},
		{"invalid-semantics", func(doc, argument map[string]any) { doc["semantics"] = "runtime-execution" }},
		{"invalid-argument-type", func(doc, argument map[string]any) { argument["type"] = "boolean" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := authoringMetadata(t)
			var argument map[string]any
			for _, directive := range authoringObjects(doc["directives"]) {
				if directive["id"] == "setup.smaGoldenCross.fast" {
					argument = authoringObjects(directive["arguments"])[0]
				}
			}
			if argument == nil {
				t.Fatal("missing fast argument")
			}
			tc.mutate(doc, argument)
			// Check the schema alone: authoringValidate's semantic comparisons
			// must not conceal a relaxed const, enum or additionalProperties.
			if err := validateContract(doc, schema, path, schema, "authoring"); err == nil {
				t.Fatal("schema accepted malformed metadata")
			}
		})
	}
}

func authoringParse(t *testing.T, source string, valid bool) map[string]any {
	t.Helper()
	result, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if valid != (len(result.Errors) == 0) {
		t.Fatalf("valid=%v; errors=%v\n%s", valid, result.Errors, source)
	}
	if valid && result.Config["setupType"] != "smaGoldenCross" {
		t.Fatalf("unexpected family: %v", result.Config["setupType"])
	}
	return result.Config
}

func authoringProgram(body string) string { return "dsl v7\nsetup {\n" + body + "\n}\n" }

func TestAuthoringMetadataExamples(t *testing.T) {
	doc := authoringMetadata(t)
	complete := authoringObjects(doc["examples"])[2]["source"].(string)
	// Every offered directive snippet is actually in a complete checked example.
	for _, d := range authoringObjects(doc["directives"]) {
		line := d["example"].(string)
		if !strings.Contains(complete, "\n "+line+"\n") {
			t.Fatalf("unchecked directive example: %s", line)
		}
		found := false
		for _, n := range d["arity"].([]any) {
			if int(n.(float64)) == len(tokenize(normalizeLine(line))) {
				found = true
			}
		}
		if !found {
			t.Fatalf("example arity: %s", line)
		}
	}
	data, err := os.ReadFile(filepath.Join(testsupport.MustRepoRoot(), "spec/authoring-metadata.md"))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(data), "```dsl\n")
	if len(parts) < 2 {
		t.Fatal("no documentation example")
	}
	for _, part := range parts[1:] {
		source, _, ok := strings.Cut(part, "```")
		if !ok {
			t.Fatal("unclosed DSL fence")
		}
		authoringParse(t, source, true)
	}
	// All manifest-backed family spellings and case variants select the family.
	for _, d := range authoringObjects(doc["directives"]) {
		if d["id"] == "setup.smaGoldenCross.type" {
			for _, alias := range schemaStringSlice(authoringObjects(d["arguments"])[0]["enum"]) {
				authoringParse(t, authoringProgram("TYPE: "+strings.ToUpper(alias)), true)
			}
		}
	}
}

func TestAuthoringMetadataParserBoundaries(t *testing.T) {
	const selector = "type: sma golden cross\n"
	const bundle = "sma atr 14\nsma stop 2 ATR\nsma target 3 ATR"
	for _, section := range []string{"strategy", "market", "context", "levels", "filters", "triggers", "entry", "risk", "target", "management", "execution", "grade", "volume profile"} {
		t.Run("type/section/"+section, func(t *testing.T) {
			authoringParse(t, authoringProgram(selector)+section+" { "+strings.TrimSpace(selector)+" }\n", false)
		})
	}
	for _, key := range []string{"fast", "slow", "atr", "stop", "target"} {
		line := map[string]string{"fast": "sma fast 20", "slow": "sma slow 80", "atr": "sma atr 14", "stop": "sma stop 2 ATR", "target": "sma target 3 ATR"}[key]
		for _, section := range []string{"strategy", "market", "context", "levels", "filters", "triggers", "entry", "risk", "target", "management", "execution", "grade", "volume profile"} {
			t.Run(key+"/section/"+section, func(t *testing.T) {
				authoringParse(t, authoringProgram(selector+bundle)+section+" { "+line+" }\n", false)
			})
		}
		t.Run(key+"/before-type", func(t *testing.T) { authoringParse(t, authoringProgram(line+"\n"+selector+bundle), false) })
		for _, bad := range []string{strings.Join(strings.Fields(line)[:len(strings.Fields(line))-1], " "), line + " extra"} {
			t.Run(key+"/arity/"+bad, func(t *testing.T) {
				authoringParse(t, authoringProgram(selector+strings.ReplaceAll(bundle, line, "")+"\n"+bad), false)
			})
		}
	}
	for _, key := range []string{"fast", "slow", "atr"} {
		for _, value := range []string{"0", "-1", "1.5", "NaN", "Infinity", "1e999", "word", "+20", "2e+1"} {
			t.Run(key+"/value/"+value, func(t *testing.T) {
				body := selector + bundle + "\nsma " + key + " " + value
				authoringParse(t, authoringProgram(body), false)
			})
		}
	}
	for _, key := range []string{"stop", "target"} {
		for _, value := range []string{"0", "-1", "NaN", "Infinity", "1e999", "0x1p1", "1_0", "2R", "2%", "+2"} {
			t.Run(key+"/value/"+value, func(t *testing.T) {
				authoringParse(t, authoringProgram(selector+bundle+"\nsma "+key+" "+value+" ATR"), false)
			})
		}
		for _, unit := range []string{"R", "bars", "candles", "percent"} {
			t.Run(key+"/unit/"+unit, func(t *testing.T) {
				authoringParse(t, authoringProgram(selector+bundle+"\nsma "+key+" 2 "+unit), false)
			})
		}
	}
	for _, value := range []string{"0x1p4", "1_0", "14%", "14R"} {
		authoringParse(t, authoringProgram(selector+bundle+"\nsma atr "+value), false)
	}
	for _, key := range []string{"fast", "slow", "atr"} {
		authoringParse(t, authoringProgram(selector+bundle+"\nsma "+key+" 60 bars"), false)
	}
	for _, pair := range [][2]int{{50, 50}, {80, 20}, {200, 200}} {
		authoringParse(t, authoringProgram(selector+fmt.Sprintf("sma fast %d\nsma slow %d", pair[0], pair[1])), false)
	}
	// Every proper nonempty subset of the optional bundle must fail.
	lines := strings.Split(bundle, "\n")
	for mask := 1; mask < 7; mask++ {
		body := selector
		for i, line := range lines {
			if mask&(1<<i) != 0 {
				body += line + "\n"
			}
		}
		t.Run(fmt.Sprintf("bundle/%d", mask), func(t *testing.T) { authoringParse(t, authoringProgram(body), false) })
	}
	// Scalar and bundle order after selection are unrestricted.
	authoringParse(t, authoringProgram(selector+"sma target .5 atr\nsma slow 80\nsma stop 2e-1 AtR\nsma atr 14.0\nsma fast 20"), true)
	authoringParse(t, authoringProgram(selector+"sma atr 1e100\nsma stop 1e100 ATR\nsma target 1e100 ATR"), true)
	for _, tc := range []struct {
		token string
		want  float64
	}{{"20", 20}, {"20.0", 20}, {"2e1", 20}, {"20R", 20}, {"20r", 20}, {"20%", 20}, {"0x1p4", 16}, {"1_0", 10}} {
		cfg := authoringParse(t, authoringProgram(selector+"sma fast "+tc.token), true)
		if got := cfg["smaGoldenCross"].(map[string]any)["fastSmaLen"]; got != tc.want {
			t.Fatalf("legacy token %s = %v, want %v", tc.token, got, tc.want)
		}
	}
	cfg := authoringParse(t, authoringProgram(selector+"sma fast 30\nsma fast 20"), true)
	if cfg["smaGoldenCross"].(map[string]any)["fastSmaLen"] != 20.0 {
		t.Fatal("last scalar no longer wins")
	}
	cfg = authoringParse(t, authoringProgram(selector+bundle+"\nsma fast 20\n"+selector), true)
	params := cfg["smaGoldenCross"].(map[string]any)
	if params["fastSmaLen"] != 50.0 || params["atrLen"] != nil {
		t.Fatal("type no longer resets defaults/bundle")
	}
}
