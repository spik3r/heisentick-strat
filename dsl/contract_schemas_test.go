package dsl

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/testsupport"
)

type contractSchema map[string]any

func contractSchemaRoot() string {
	return filepath.Join(testsupport.MustRepoRoot(), "spec", "schemas")
}

func loadContractSchema(t *testing.T, name string) (contractSchema, string) {
	t.Helper()
	path := filepath.Join(contractSchemaRoot(), name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var schema contractSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return schema, path
}

func loadContractJSON(t *testing.T, path string) any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return value
}

func TestVersionedStratContractSchemas(t *testing.T) {
	diagnostic, diagnosticPath := loadContractSchema(t, "diagnostic-v1.schema.json")
	parseResult, parseResultPath := loadContractSchema(t, "parse-result-v1.schema.json")
	runFixture, runFixturePath := loadContractSchema(t, "run-fixture-v1.schema.json")
	tradeEnvelope, tradeEnvelopePath := loadContractSchema(t, "trade-envelope-v1.schema.json")

	parseDir := filepath.Join(testsupport.StratConformanceRoot(), "parse")
	parsePaths, err := filepath.Glob(filepath.Join(parseDir, "*.cfg.json"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(parsePaths)
	for _, path := range parsePaths {
		value := loadContractJSON(t, path)
		if err := validateContract(value, parseResult, parseResultPath, parseResult, path); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		fixture := value.(map[string]any)
		for index, diagnosticValue := range fixture["diagnostics"].([]any) {
			if err := validateContract(diagnosticValue, diagnostic, diagnosticPath, diagnostic, fmt.Sprintf("%s.diagnostics[%d]", path, index)); err != nil {
				t.Fatalf("%s: %v", path, err)
			}
		}
	}

	runDir := filepath.Join(testsupport.StratConformanceRoot(), "run")
	runPaths, err := filepath.Glob(filepath.Join(runDir, "*.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(runPaths)
	for _, path := range runPaths {
		if err := validateContract(loadContractJSON(t, path), runFixture, runFixturePath, runFixture, path); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}

	tradePaths, err := filepath.Glob(filepath.Join(runDir, "*.trades.json"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(tradePaths)
	for _, path := range tradePaths {
		value := loadContractJSON(t, path)
		if err := validateContract(value, tradeEnvelope, tradeEnvelopePath, tradeEnvelope, path); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		envelope := value.(map[string]any)
		if got, want := int(envelope["tradeCount"].(float64)), len(envelope["trades"].([]any)); got != want {
			t.Fatalf("%s: tradeCount = %d, trades = %d", path, got, want)
		}
	}
}

func TestGoSchemaValidationRejectsMalformedContractMutations(t *testing.T) {
	diagnostic, diagnosticPath := loadContractSchema(t, "diagnostic-v1.schema.json")
	parseResult, parseResultPath := loadContractSchema(t, "parse-result-v1.schema.json")
	runFixture, runFixturePath := loadContractSchema(t, "run-fixture-v1.schema.json")
	tradeEnvelope, tradeEnvelopePath := loadContractSchema(t, "trade-envelope-v1.schema.json")
	parseFixture := loadContractJSON(t, filepath.Join(testsupport.StratConformanceRoot(), "parse", "diagnostic-malformed-number-v7.cfg.json")).(map[string]any)
	runFixtureValue := loadContractJSON(t, filepath.Join(testsupport.StratConformanceRoot(), "run", "family-price-momentum.fixture.json")).(map[string]any)
	tradeFixture := loadContractJSON(t, filepath.Join(testsupport.StratConformanceRoot(), "run", "family-price-momentum.trades.json")).(map[string]any)

	missingRequired := cloneContractObject(parseFixture)
	delete(missingRequired, "case")
	assertInvalidContract(t, missingRequired, parseResult, parseResultPath, "missing required parse field")

	wrongVersion := cloneContractObject(parseFixture)
	wrongVersion["schema"] = "dsl-conformance-parse-v2"
	assertInvalidContract(t, wrongVersion, parseResult, parseResultPath, "wrong parse schema version")

	wrongType := cloneContractObject(runFixtureValue)
	wrongType["bars"] = "not an array"
	assertInvalidContract(t, wrongType, runFixture, runFixturePath, "wrong run field type")

	extraField := cloneContractObject(tradeFixture)
	extraField["unexpected"] = true
	assertInvalidContract(t, extraField, tradeEnvelope, tradeEnvelopePath, "extra forbidden trade field")

	diagnosticValue := cloneContractObject(parseFixture["diagnostics"].([]any)[0].(map[string]any))
	diagnosticValue["severity"] = "info"
	assertInvalidContract(t, diagnosticValue, diagnostic, diagnosticPath, "invalid diagnostic enum")

	missingDiagnosticField := cloneContractObject(parseFixture["diagnostics"].([]any)[0].(map[string]any))
	delete(missingDiagnosticField, "message")
	assertInvalidContract(t, missingDiagnosticField, diagnostic, diagnosticPath, "missing referenced diagnostic field")

	badCostField := cloneContractObject(runFixtureValue)
	badCostField["costs"].(map[string]any)["fillOn"] = float64(4)
	assertInvalidContract(t, badCostField, runFixture, runFixturePath, "bad referenced cost field")

	unsupportedKeyword := cloneContractObject(map[string]any(parseResult))
	unsupportedKeyword["minContains"] = float64(1)
	assertInvalidContract(t, parseFixture, unsupportedKeyword, parseResultPath, "unsupported schema keyword")
}

func TestGoSchemaValidationRejectsUnsupportedSchemaCombinations(t *testing.T) {
	parseResult, parseResultPath := loadContractSchema(t, "parse-result-v1.schema.json")

	refWithValidationSibling := contractSchema{"$ref": "diagnostic-v1.schema.json", "type": "object"}
	assertInvalidContract(t, map[string]any{}, refWithValidationSibling, parseResultPath, "ref validation sibling")

	anyOfWithValidationSibling := contractSchema{"anyOf": []any{map[string]any{"type": "object"}}, "required": []any{"schema"}}
	assertInvalidContract(t, map[string]any{}, anyOfWithValidationSibling, parseResultPath, "anyOf validation sibling")

	objectAdditionalProperties := contractSchema{"type": "object", "additionalProperties": map[string]any{"type": "string"}}
	assertInvalidContract(t, map[string]any{}, objectAdditionalProperties, parseResultPath, "object additionalProperties")

	_ = parseResult
}

func assertInvalidContract(t *testing.T, value any, schema contractSchema, schemaPath, label string) {
	t.Helper()
	if err := validateContract(value, schema, schemaPath, schema, label); err == nil {
		t.Fatalf("%s: validator accepted malformed value", label)
	}
}

func cloneContractObject(value map[string]any) map[string]any {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(data, &clone); err != nil {
		panic(err)
	}
	return clone
}

var supportedContractSchemaKeywords = map[string]bool{
	"$defs": true, "$id": true, "$ref": true, "$schema": true,
	"additionalProperties": true, "anyOf": true, "const": true,
	"description": true, "enum": true, "items": true, "maxItems": true,
	"minItems": true, "minLength": true, "minProperties": true,
	"minimum": true, "properties": true, "required": true, "title": true,
	"type": true,
}

var contractSchemaAnnotationKeywords = map[string]bool{
	"$defs": true, "$id": true, "$schema": true, "description": true, "title": true,
}

func assertSupportedContractSchemaKeywords(schema contractSchema, location string) error {
	for key := range schema {
		if !supportedContractSchemaKeywords[key] {
			return fmt.Errorf("%s: unsupported schema keyword %s", location, key)
		}
	}
	for _, combinator := range []string{"$ref", "anyOf"} {
		if _, present := schema[combinator]; present {
			for key := range schema {
				if key != combinator && !contractSchemaAnnotationKeywords[key] {
					return fmt.Errorf("%s: %s validation siblings are unsupported", location, combinator)
				}
			}
		}
	}
	if additional, present := schema["additionalProperties"]; present {
		if _, ok := additional.(bool); !ok {
			return fmt.Errorf("%s: object additionalProperties schemas are unsupported", location)
		}
	}
	for key, value := range schema {
		switch key {
		case "$defs", "properties":
			children, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.%s: schema map expected", location, key)
			}
			for name, child := range children {
				childSchema, ok := child.(map[string]any)
				if !ok {
					return fmt.Errorf("%s.%s.%s: schema object expected", location, key, name)
				}
				if err := assertSupportedContractSchemaKeywords(childSchema, location+"."+key+"."+name); err != nil {
					return err
				}
			}
		case "items", "additionalProperties":
			if child, ok := value.(map[string]any); ok {
				if err := assertSupportedContractSchemaKeywords(child, location+"."+key); err != nil {
					return err
				}
			}
		case "anyOf":
			branches, ok := value.([]any)
			if !ok {
				return fmt.Errorf("%s.anyOf: schema list expected", location)
			}
			for index, branch := range branches {
				child, ok := branch.(map[string]any)
				if !ok {
					return fmt.Errorf("%s.anyOf[%d]: schema object expected", location, index)
				}
				if err := assertSupportedContractSchemaKeywords(child, fmt.Sprintf("%s.anyOf[%d]", location, index)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func TestGoParseProjectionSatisfiesParseResultSchema(t *testing.T) {
	schema, schemaPath := loadContractSchema(t, "parse-result-v1.schema.json")
	parseDir := filepath.Join(testsupport.StratConformanceRoot(), "parse")
	paths, err := filepath.Glob(filepath.Join(parseDir, "*.cfg.json"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		fixture := loadContractJSON(t, path).(map[string]any)
		caseName := strings.TrimSuffix(filepath.Base(path), ".cfg.json")
		source, err := os.ReadFile(filepath.Join(parseDir, caseName+".strat"))
		if err != nil {
			t.Fatalf("read %s source: %v", caseName, err)
		}
		result, err := Parse(string(source))
		if err != nil {
			t.Fatalf("parse %s: %v", caseName, err)
		}
		projectedBytes, err := json.Marshal(map[string]any{
			"schema":      fixture["schema"],
			"case":        fixture["case"],
			"category":    fixture["category"],
			"description": fixture["description"],
			"source":      fixture["source"],
			"cfg":         result.Config,
			"errors":      result.Errors,
			"warnings":    result.Warnings,
			"diagnostics": result.Diagnostics,
		})
		if err != nil {
			t.Fatalf("marshal %s projection: %v", caseName, err)
		}
		var projected map[string]any
		if err := json.Unmarshal(projectedBytes, &projected); err != nil {
			t.Fatalf("decode %s projection: %v", caseName, err)
		}
		if err := validateContract(projected, schema, schemaPath, schema, caseName); err != nil {
			t.Fatalf("%s: %v", caseName, err)
		}
	}
}

func validateContract(value any, schema contractSchema, schemaPath string, root contractSchema, location string) error {
	if err := assertSupportedContractSchemaKeywords(schema, schemaPath); err != nil {
		return err
	}
	if ref, ok := schema["$ref"].(string); ok {
		refFile, fragment := ref, ""
		if index := strings.IndexByte(ref, '#'); index >= 0 {
			refFile, fragment = ref[:index], ref[index+1:]
		}
		targetPath := schemaPath
		targetRoot := root
		if refFile != "" {
			targetRoot = make(contractSchema)
			targetPath = filepath.Join(filepath.Dir(schemaPath), refFile)
			data, err := os.ReadFile(targetPath)
			if err != nil {
				return fmt.Errorf("%s: read $ref %s: %w", location, ref, err)
			}
			if err := json.Unmarshal(data, &targetRoot); err != nil {
				return fmt.Errorf("%s: decode $ref %s: %w", location, ref, err)
			}
		}
		target, err := schemaPointer(targetRoot, fragment)
		if err != nil {
			return fmt.Errorf("%s: resolve $ref %s: %w", location, ref, err)
		}
		return validateContract(value, target, targetPath, targetRoot, location)
	}

	if branches, ok := schema["anyOf"].([]any); ok {
		failures := make([]string, 0, len(branches))
		for _, branch := range branches {
			branchSchema, ok := branch.(map[string]any)
			if !ok {
				return fmt.Errorf("%s: anyOf branch is not an object", location)
			}
			if err := validateContract(value, branchSchema, schemaPath, root, location); err == nil {
				return nil
			} else {
				failures = append(failures, err.Error())
			}
		}
		return fmt.Errorf("%s: no anyOf branch matched: %s", location, strings.Join(failures, "; "))
	}

	if expected, ok := schema["const"]; ok && !reflect.DeepEqual(value, expected) {
		return fmt.Errorf("%s: const mismatch", location)
	}
	if enum, ok := schema["enum"].([]any); ok {
		matched := false
		for _, candidate := range enum {
			if reflect.DeepEqual(value, candidate) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s: enum mismatch", location)
		}
	}
	if expected, ok := schema["type"].(string); ok && !contractTypeMatches(value, expected) {
		return fmt.Errorf("%s: expected %s", location, expected)
	}
	if minimum, ok := schema["minimum"].(float64); ok {
		if number, ok := value.(float64); !ok || number < minimum {
			return fmt.Errorf("%s: below minimum", location)
		}
	}

	switch typed := value.(type) {
	case string:
		if minimum, ok := schema["minLength"].(float64); ok && utf8.RuneCountInString(typed) < int(minimum) {
			return fmt.Errorf("%s: too short", location)
		}
	case []any:
		if minimum, ok := schema["minItems"].(float64); ok && len(typed) < int(minimum) {
			return fmt.Errorf("%s: too few items", location)
		}
		if maximum, ok := schema["maxItems"].(float64); ok && len(typed) > int(maximum) {
			return fmt.Errorf("%s: too many items", location)
		}
		if itemSchema, ok := schema["items"].(map[string]any); ok {
			for index, item := range typed {
				if err := validateContract(item, itemSchema, schemaPath, root, fmt.Sprintf("%s[%d]", location, index)); err != nil {
					return err
				}
			}
		}
	case map[string]any:
		if minimum, ok := schema["minProperties"].(float64); ok && len(typed) < int(minimum) {
			return fmt.Errorf("%s: too few properties", location)
		}
		properties, _ := schema["properties"].(map[string]any)
		for _, required := range schemaStringSlice(schema["required"]) {
			if _, ok := typed[required]; !ok {
				return fmt.Errorf("%s: missing %s", location, required)
			}
		}
		if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
			for key := range typed {
				if _, known := properties[key]; !known {
					return fmt.Errorf("%s: unexpected %s", location, key)
				}
			}
		}
		for key, property := range properties {
			if child, ok := typed[key]; ok {
				propertySchema, ok := property.(map[string]any)
				if !ok {
					return fmt.Errorf("%s.%s: property schema is not an object", location, key)
				}
				if err := validateContract(child, propertySchema, schemaPath, root, location+"."+key); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func schemaStringSlice(value any) []string {
	items, _ := value.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func schemaPointer(root contractSchema, fragment string) (contractSchema, error) {
	if fragment == "" {
		return root, nil
	}
	if !strings.HasPrefix(fragment, "/") {
		return nil, fmt.Errorf("unsupported fragment %q", fragment)
	}
	var current any = map[string]any(root)
	for _, raw := range strings.Split(strings.TrimPrefix(fragment, "/"), "/") {
		segment := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("segment %q is not an object", segment)
		}
		current, ok = object[segment]
		if !ok {
			return nil, fmt.Errorf("segment %q is missing", segment)
		}
	}
	result, ok := current.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("fragment %q is not an object", fragment)
	}
	return result, nil
}

func contractTypeMatches(value any, expected string) bool {
	switch expected {
	case "null":
		return value == nil
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		number, ok := value.(float64)
		return ok && !math.IsNaN(number) && !math.IsInf(number, 0)
	case "integer":
		number, ok := value.(float64)
		return ok && !math.IsNaN(number) && !math.IsInf(number, 0) && math.Trunc(number) == number
	case "boolean":
		_, ok := value.(bool)
		return ok
	default:
		return false
	}
}
