package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// This repo has no third-party dependencies (see go.mod), so rather than
// pulling in a full JSON Schema validator (e.g. santhosh-tekuri/jsonschema,
// as heisentick-contracts itself uses) just for this test, a minimal
// recursive evaluator is vendored here, supporting exactly the draft-2020-12
// keywords schemas/trade-export.v1.schema.json and common.v1.schema.json use
// (type, const, enum, pattern, minimum/exclusiveMinimum, required,
// properties, additionalProperties, items, minItems, anyOf, $ref). The
// schema files themselves are copied verbatim into testdata/ from
// heisentick-contracts (not a module dependency or a `replace` directive:
// see agents.md's cross-repo release-then-pin rule) and re-copied whenever
// that repo's schema changes. This proves the documents this package builds
// validate against the real, committed contract shape without adding a
// dependency to a zero-dependency module.
type miniSchema = map[string]any

func loadTradeExportSchemas(t *testing.T) (miniSchema, map[string]any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "trade-export.v1.schema.json"))
	if err != nil {
		t.Fatalf("read vendored schema: %v", err)
	}
	var doc miniSchema
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse vendored schema: %v", err)
	}
	commonRaw, err := os.ReadFile(filepath.Join("testdata", "common.v1.schema.json"))
	if err != nil {
		t.Fatalf("read vendored common schema: %v", err)
	}
	var common miniSchema
	if err := json.Unmarshal(commonRaw, &common); err != nil {
		t.Fatalf("parse vendored common schema: %v", err)
	}
	return doc, common
}

// validator resolves $refs against the trade-export document's own $defs and
// common.v1.schema.json's $defs (the only two files this schema references).
type validator struct {
	root   miniSchema
	common miniSchema
}

func (v validator) resolve(ref string) miniSchema {
	if strings.HasPrefix(ref, "common.v1.schema.json#/$defs/") {
		name := strings.TrimPrefix(ref, "common.v1.schema.json#/$defs/")
		defs, _ := v.common["$defs"].(map[string]any)
		schema, _ := defs[name].(map[string]any)
		return schema
	}
	if strings.HasPrefix(ref, "#/$defs/") {
		name := strings.TrimPrefix(ref, "#/$defs/")
		defs, _ := v.root["$defs"].(map[string]any)
		schema, _ := defs[name].(map[string]any)
		return schema
	}
	panic(fmt.Sprintf("unsupported $ref in test validator: %s", ref))
}

func (v validator) validate(schema miniSchema, value any, path string) []string {
	if ref, ok := schema["$ref"].(string); ok {
		return v.validate(v.resolve(ref), value, path)
	}
	var issues []string
	if anyOf, ok := schema["anyOf"].([]any); ok {
		var branchIssues [][]string
		for _, branch := range anyOf {
			branchSchema, _ := branch.(map[string]any)
			branchIssues = append(branchIssues, v.validate(branchSchema, value, path))
		}
		for _, bi := range branchIssues {
			if len(bi) == 0 {
				return nil
			}
		}
		return []string{fmt.Sprintf("%s: value %#v matched no anyOf branch (%v)", path, value, branchIssues)}
	}
	if constVal, ok := schema["const"]; ok {
		if fmt.Sprint(constVal) != fmt.Sprint(value) {
			issues = append(issues, fmt.Sprintf("%s: %#v != const %#v", path, value, constVal))
		}
	}
	if enum, ok := schema["enum"].([]any); ok {
		match := false
		for _, e := range enum {
			if fmt.Sprint(e) == fmt.Sprint(value) {
				match = true
			}
		}
		if !match {
			issues = append(issues, fmt.Sprintf("%s: %#v not in enum %v", path, value, enum))
		}
	}
	if typ, ok := schema["type"].(string); ok {
		if !matchesType(typ, value) {
			issues = append(issues, fmt.Sprintf("%s: %#v is not type %s", path, value, typ))
		}
	}
	if pattern, ok := schema["pattern"].(string); ok {
		s, isStr := value.(string)
		if !isStr {
			issues = append(issues, fmt.Sprintf("%s: pattern check on non-string %#v", path, value))
		} else if !regexp.MustCompile(pattern).MatchString(s) {
			issues = append(issues, fmt.Sprintf("%s: %q does not match pattern %s", path, s, pattern))
		}
	}
	if minimum, ok := schema["minimum"]; ok {
		if n, isNum := value.(float64); isNum && n < minimum.(float64) {
			issues = append(issues, fmt.Sprintf("%s: %v < minimum %v", path, n, minimum))
		}
	}
	if excl, ok := schema["exclusiveMinimum"]; ok {
		if n, isNum := value.(float64); isNum && n <= excl.(float64) {
			issues = append(issues, fmt.Sprintf("%s: %v <= exclusiveMinimum %v", path, n, excl))
		}
	}
	if obj, isObj := value.(map[string]any); isObj {
		if required, ok := schema["required"].([]any); ok {
			for _, r := range required {
				key := r.(string)
				if _, present := obj[key]; !present {
					issues = append(issues, fmt.Sprintf("%s: missing required field %q", path, key))
				}
			}
		}
		if props, ok := schema["properties"].(map[string]any); ok {
			for key, val := range obj {
				propSchema, known := props[key]
				if !known {
					if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
						issues = append(issues, fmt.Sprintf("%s: unknown field %q (additionalProperties: false)", path, key))
					}
					continue
				}
				issues = append(issues, v.validate(propSchema.(map[string]any), val, path+"."+key)...)
			}
		}
	}
	if arr, isArr := value.([]any); isArr {
		if minItems, ok := schema["minItems"]; ok {
			if n, isNum := minItems.(float64); isNum && float64(len(arr)) < n {
				issues = append(issues, fmt.Sprintf("%s: %d items < minItems %v", path, len(arr), minItems))
			}
		}
		if items, ok := schema["items"].(map[string]any); ok {
			for i, item := range arr {
				issues = append(issues, v.validate(items, item, fmt.Sprintf("%s[%d]", path, i))...)
			}
		}
	}
	return issues
}

func matchesType(typ string, value any) bool {
	switch typ {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "integer":
		n, ok := value.(float64)
		return ok && n == float64(int64(n))
	case "number":
		_, ok := value.(float64)
		return ok
	case "null":
		return value == nil
	case "boolean":
		_, ok := value.(bool)
		return ok
	default:
		return true
	}
}

// assertValidTradeExport marshals doc to JSON, decodes it generically (as
// any producer/consumer sees it on the wire) and validates it against the
// vendored trade-export.v1 schema.
func assertValidTradeExport(t *testing.T, doc TradeExportDocument) {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal trade export: %v", err)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode trade export: %v", err)
	}
	root, common := loadTradeExportSchemas(t)
	v := validator{root: root, common: common}
	if issues := v.validate(root, value, "$"); len(issues) > 0 {
		t.Fatalf("trade-export.v1 schema violations:\n%s\n\ndocument:\n%s", strings.Join(issues, "\n"), raw)
	}
}

func TestTradeExportValidatesAgainstSchema(t *testing.T) {
	riskUsd := 200.0
	doc := TradeExportDocument{
		Schema:  TradeExportSchema,
		Version: TradeExportVersion,
		Strategy: TradeExportStrategy{
			ID:           "dslDualEmaResumptionXauusdFourHour",
			SourceCommit: strings.Repeat("a", 64),
			StratDigest:  "v0.9.0",
		},
		Engine:      TradeExportEngineInfo{Repo: "heisentick-strat", Release: "v0.9.0"},
		DataSha256:  []TradeExportDataFile{{File: "XAUUSD/4h.bin", Sha256: strings.Repeat("b", 64)}},
		RunConfig:   TradeExportRunConfig{CostMode: "realistic", Slippage: 0.06, RiskUsd: &riskUsd},
		Routes:      []TradeExportRoute{{Symbol: "XAUUSD", TF: "4h"}},
		GeneratedAt: 1700000000000,
		Trades: []TradeExportTrade{
			{
				SignalID:   ComputeSignalID("dslDualEmaResumptionXauusdFourHour", strings.Repeat("a", 64), "XAUUSD", "4h", 1700000000000, "long"),
				Symbol:     "XAUUSD",
				TF:         "4h",
				Side:       "long",
				EntryTs:    1700000000000,
				EntryPrice: 2000.5,
				ExitTs:     1700014400000,
				ExitPrice:  2010.2,
				ExitReason: "tp",
			},
		},
	}
	assertValidTradeExport(t, doc)
}

func TestTradeExportSchemaRejectsUnknownField(t *testing.T) {
	riskUsd := 200.0
	doc := TradeExportDocument{
		Schema:      TradeExportSchema,
		Version:     TradeExportVersion,
		Strategy:    TradeExportStrategy{ID: "x", SourceCommit: strings.Repeat("a", 64)},
		Engine:      TradeExportEngineInfo{Repo: "heisentick-strat", Release: "v0.9.0"},
		DataSha256:  []TradeExportDataFile{{File: "XAUUSD/4h.bin", Sha256: strings.Repeat("b", 64)}},
		RunConfig:   TradeExportRunConfig{CostMode: "realistic", Slippage: 0.06, RiskUsd: &riskUsd},
		Routes:      []TradeExportRoute{{Symbol: "XAUUSD", TF: "4h"}},
		GeneratedAt: 1700000000000,
		Trades: []TradeExportTrade{
			{SignalID: strings.Repeat("0", 32), Symbol: "XAUUSD", TF: "4h", Side: "long", EntryTs: 1, ExitTs: 2, ExitReason: "tp"},
		},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	value["extra"] = "nope"
	root, common := loadTradeExportSchemas(t)
	v := validator{root: root, common: common}
	if issues := v.validate(root, value, "$"); len(issues) == 0 {
		t.Fatal("expected an unknown-field schema violation, got none")
	}
}
