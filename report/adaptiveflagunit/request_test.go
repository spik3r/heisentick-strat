package adaptiveflagunit

import (
	"encoding/json"
	"strings"
	"testing"
)

func runtimeProjectionRequestFixture(t *testing.T) (Request, string) {
	t.Helper()
	want := Request{RequestSchema, "UNIT_POINT_VALUE_1", "BINARY64_ORDERED_V1", "RAW", DataSource{"invented:dataset/revision-1", strings.Repeat("a", 64)}}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	return want, string(raw)
}

func replaceProjectionField(t *testing.T, raw, key, value string) string {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		t.Fatal(err)
	}
	if value == "" {
		delete(fields, key)
	} else {
		fields[key] = json.RawMessage(value)
	}
	out, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func rejectProjectionRequest(t *testing.T, raw, code string) {
	t.Helper()
	got, err := DecodeRuntimeRequest(raw)
	if err == nil || got != (Request{}) {
		t.Fatalf("rejected request returned partial/success data: %+v, %v", got, err)
	}
	wire := decodeCoreError(t, err)
	loc := wire.Error.Location
	if wire.Error.Code != code || wire.Error.Phase != errorPhase(code) || loc.Operation == nil || *loc.Operation != "projection_request" || loc.RowIndex != nil || loc.OrderID != nil || loc.EventID != nil {
		t.Fatalf("wrong projection admission error: %+v", wire)
	}
}

func TestDecodeRuntimeRequestAcceptsClosedValues(t *testing.T) {
	want, raw := runtimeProjectionRequestFixture(t)
	for _, policy := range []string{"RAW", "RAZOR_PROXY_BASIC", "RAZOR_PROXY_HARSH_AGGREGATE"} {
		want.CostPolicy = policy
		got, err := DecodeRuntimeRequest(replaceProjectionField(t, raw, "costPolicy", `"`+policy+`"`))
		if err != nil || got != want {
			t.Fatalf("policy %s: %+v, %v", policy, got, err)
		}
	}
	want, _ = runtimeProjectionRequestFixture(t)
	// Escaped ASCII retains the decoded contract, including field names and the
	// slash in an identity. The caller retains raw bytes for request hashing.
	escaped := strings.NewReplacer(`"schema"`, `"\u0073chema"`, `"RAW"`, `"R\u0041W"`, "/revision", `\/revision`).Replace(raw)
	got, err := DecodeRuntimeRequest(" \n\t" + escaped + "\r\n")
	if err != nil || got != want {
		t.Fatalf("valid escaped ASCII/whitespace: %+v, %v", got, err)
	}
	reordered := `{"dataSource":{"sourceSha256":"` + want.DataSource.SourceSHA256 + `","id":"` + want.DataSource.ID + `"},"costPolicy":"RAW","numericalPolicy":"BINARY64_ORDERED_V1","scenario":"UNIT_POINT_VALUE_1","schema":"` + RequestSchema + `"}`
	got, err = DecodeRuntimeRequest(reordered)
	if err != nil || got != want {
		t.Fatalf("valid key order: %+v, %v", got, err)
	}
}

func TestDecodeRuntimeRequestExactByteBoundary(t *testing.T) {
	want, raw := runtimeProjectionRequestFixture(t)
	for _, at := range []string{"before", "after", "inside"} {
		t.Run(at, func(t *testing.T) {
			padding := strings.Repeat(" ", 1024-len(raw))
			padded := padding + raw
			if at == "after" {
				padded = raw + padding
			} else if at == "inside" {
				padded = "{" + padding + raw[1:]
			}
			if len(padded) != 1024 {
				t.Fatalf("boundary fixture has %d bytes", len(padded))
			}
			got, err := DecodeRuntimeRequest(padded)
			if err != nil || got != want {
				t.Fatalf("1024-byte metadata refused: %+v, %v", got, err)
			}
			rejectProjectionRequest(t, padded+" ", "resource_limit")
		})
	}
	// The resource bound applies to original bytes before parsing or repair.
	rejectProjectionRequest(t, strings.Repeat("\xff", 1025), "resource_limit")
	rejectProjectionRequest(t, strings.Repeat("é", 513), "resource_limit")
	maxID := strings.Repeat("a", 128)
	maxRaw := strings.Replace(raw, want.DataSource.ID, maxID, 1)
	if got, err := DecodeRuntimeRequest(maxRaw); err != nil || got.DataSource.ID != maxID {
		t.Fatalf("128-byte identity refused: %+v, %v", got, err)
	}
}

func TestDecodeRuntimeRequestMandatoryFieldsAndTypes(t *testing.T) {
	want, raw := runtimeProjectionRequestFixture(t)
	for _, key := range []string{"schema", "scenario", "numericalPolicy", "costPolicy", "dataSource"} {
		for _, value := range []string{"", "null", "false", "0", "1.5", "[]", `""`} {
			t.Run(key+"/"+value, func(t *testing.T) {
				rejectProjectionRequest(t, replaceProjectionField(t, raw, key, value), "projection_request_rejected")
			})
		}
		if key != "dataSource" {
			rejectProjectionRequest(t, replaceProjectionField(t, raw, key, "{}"), "projection_request_rejected")
		}
	}
	source := `{"id":"` + want.DataSource.ID + `","sourceSha256":"` + want.DataSource.SourceSHA256 + `"}`
	for _, key := range []string{"id", "sourceSha256"} {
		for _, value := range []string{"", "null", "true", "0", "[]", "{}", `""`} {
			t.Run("dataSource/"+key+"/"+value, func(t *testing.T) {
				bad := replaceProjectionField(t, source, key, value)
				rejectProjectionRequest(t, replaceProjectionField(t, raw, "dataSource", bad), "projection_request_rejected")
			})
		}
	}
}

func TestDecodeRuntimeRequestRejectsSyntaxDuplicatesAndTrailing(t *testing.T) {
	_, raw := runtimeProjectionRequestFixture(t)
	for name, bad := range map[string]string{
		"empty": "", "null": "null", "array": "[]", "string": `"request"`, "number": "1", "boolean": "true",
		"empty object": "{}", "truncated": raw[:len(raw)-1], "trailing comma": raw[:len(raw)-1] + ",}",
		"trailing object": raw + "{}", "trailing null": raw + " null", "trailing junk": raw + "x",
		"comment": raw + "//comment", "bom": "\ufeff" + raw, "non JSON whitespace": raw + "\v",
		"duplicate root":                  `{"schema":"` + RequestSchema + `",` + raw[1:],
		"duplicate escaped root":          `{"\u0073chema":"` + RequestSchema + `",` + raw[1:],
		"duplicate source object":         `{"dataSource":{"id":"x","sourceSha256":"` + strings.Repeat("a", 64) + `"},` + raw[1:],
		"duplicate nested id":             strings.Replace(raw, `"id":`, `"id":"other","id":`, 1),
		"duplicate escaped nested id":     strings.Replace(raw, `"id":`, `"\u0069d":"other","id":`, 1),
		"duplicate nested digest":         strings.Replace(raw, `"sourceSha256":`, `"sourceSha256":"`+strings.Repeat("b", 64)+`","sourceSha256":`, 1),
		"duplicate escaped nested digest": strings.Replace(raw, `"sourceSha256":`, `"sourceSha\u003256":"`+strings.Repeat("b", 64)+`","sourceSha256":`, 1),
		"case alias":                      strings.Replace(raw, `"costPolicy"`, `"CostPolicy"`, 1),
		"nested case alias":               strings.Replace(raw, `"sourceSha256"`, `"sourceSHA256"`, 1),
		"nonstring key":                   strings.Replace(raw, `"schema":`, `1:`, 1),
	} {
		t.Run(name, func(t *testing.T) { rejectProjectionRequest(t, bad, "projection_request_rejected") })
	}
}

func TestDecodeRuntimeRequestRejectsUnicodeAndInvalidUTF8(t *testing.T) {
	want, raw := runtimeProjectionRequestFixture(t)
	// Run every invalid encoding through root values, nested values, root keys
	// and nested keys; lone surrogates must not be silently repaired into success.
	for _, invalid := range []string{`\ud800`, `\udbff`, `\udc00`, `\udfff`, `\ud800x`, `\ud800\ud800`, `\udc00\ud800`, `\ud83d\ude00`, `\ufffd`, "é", "🙂", "\xff", "\xc0\xaf", "\xed\xa0\x80", "\xe2\x82", `\u0000`, `\n`, `\t`} {
		for _, target := range []string{RequestSchema, "UNIT_POINT_VALUE_1", "BINARY64_ORDERED_V1", "RAW", want.DataSource.ID, want.DataSource.SourceSHA256, "schema", "id"} {
			t.Run(target+"/"+invalid, func(t *testing.T) {
				bad := strings.Replace(raw, `"`+target+`"`, `"`+target+invalid+`"`, 1)
				rejectProjectionRequest(t, bad, "projection_request_rejected")
			})
		}
	}
}

func TestDecodeRuntimeRequestRejectsIdentityAndEnumVariants(t *testing.T) {
	want, raw := runtimeProjectionRequestFixture(t)
	for _, id := range []string{"a", "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:/-"} {
		if got, err := DecodeRuntimeRequest(strings.Replace(raw, want.DataSource.ID, id, 1)); err != nil || got.DataSource.ID != id {
			t.Fatalf("allowed identity %q refused: %+v, %v", id, got, err)
		}
	}
	for _, id := range []string{strings.Repeat("a", 129), " a", "a ", "a@b", "a?b", "a#b", "a+b", "a=b", "a%b", "a,b", "a\\b"} {
		encoded, _ := json.Marshal(id)
		bad := strings.Replace(raw, `"`+want.DataSource.ID+`"`, string(encoded), 1)
		rejectProjectionRequest(t, bad, "projection_request_rejected")
	}
	for _, hash := range []string{strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64), "0x" + strings.Repeat("a", 62)} {
		rejectProjectionRequest(t, strings.Replace(raw, want.DataSource.SourceSHA256, hash, 1), "projection_request_rejected")
	}
	for key, values := range map[string][]string{
		"schema":          {"adaptive-flag-unit-request-v2", "ADAPTIVE-FLAG-UNIT-REQUEST-V1"},
		"scenario":        {"UNIT_POINT_VALUE_2", "ACCOUNT", "G", "CUSTOM"},
		"numericalPolicy": {"BINARY64_ORDERED_V2", "binary64_ordered_v1", "DECIMAL"},
		"costPolicy":      {"raw", " RAW", "RAW ", "CUSTOM", "G", "RAZOR_PROXY_HARSH", "RAZOR_PROXY_BASIC_CUSTOM"},
	} {
		for _, value := range values {
			rejectProjectionRequest(t, replaceProjectionField(t, raw, key, `"`+value+`"`), "projection_request_rejected")
		}
	}
}

func TestDecodeRuntimeRequestRejectsAccountFeeAndRegistryOverrides(t *testing.T) {
	want, raw := runtimeProjectionRequestFixture(t)
	for _, field := range []string{
		"capital", "startingCapital", "initialCapital", "dollars", "leverage", "lotSize", "quantity", "pointValue", "plannedRiskU", "risk", "customRisk", "riskPercent", "percentageReturn",
		"fee", "fees", "feeOverrides", "costs", "perFill", "spread", "commission", "aggregate", "funding", "fundingCashU", "fundingAssumption", "account", "accountSettings", "fillPolicy",
		"registry", "include_registry", "includeRegistry", "registryEnabled", "candidateRegistry", "gPolicy", "G", "filters", "caps", "producer", "buildIdentity", "raw", "rawLedger", "executionWindow",
	} {
		t.Run(field, func(t *testing.T) {
			rejectProjectionRequest(t, replaceProjectionField(t, raw, field, "false"), "projection_request_rejected")
			source := `{"id":"` + want.DataSource.ID + `","sourceSha256":"` + want.DataSource.SourceSHA256 + `","` + field + `":false}`
			rejectProjectionRequest(t, replaceProjectionField(t, raw, "dataSource", source), "projection_request_rejected")
			got, err := DecodeRuntimeRequest(raw)
			if err != nil || got != want {
				t.Fatalf("valid-call recovery failed: %+v, %v", got, err)
			}
		})
	}
}
