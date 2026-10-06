package dsl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestFrozenLevelJSONIntegerAliasesAndBounds(t *testing.T) {
	raw := string(frozenConfigTestFile(t, "config.canonical.json"))
	for _, token := range []string{"0", "0.0", "0e10000", "0e-10000", "1", "1.0", "1e0", "1E+0", "10e-1", "0.1e1", "9007199254740991", "9007199254740991.0", "90071992547409910e-1", "900719925474099100e-2"} {
		t.Run("accept/"+token, func(t *testing.T) {
			candidate := strings.Replace(raw, `"entryLatencyMS":0`, `"entryLatencyMS":`+token, 1)
			cfg, err := DecodeFrozenLevelConfigJSON([]byte(candidate))
			if err != nil {
				t.Fatal(err)
			}
			value, err := frozenExactInteger(token)
			if err != nil || frozenConfigTestSpec(cfg)["entryLatencyMS"] != value {
				t.Fatal("integer alias did not normalize exactly")
			}
		})
	}
	for _, token := range []string{"9007199254740991.1", "9007199254740990.9", "9007199254740992", "90071992547409920e-1", "1e10000", "0e10001", "0e-10001", "1e10001", "1e-10001", "1e-10000", "1e-1", "-0", "-0.0", "-0e0", "-0e10000", "-0.000e-1", "-1", "+1", "01", "1.", ".1", "1e", "1e+", "1e--1", "1.e1", `"1"`, "null", "true", "NaN", "Infinity", "١"} {
		t.Run("reject/"+token, func(t *testing.T) {
			candidate := strings.Replace(raw, `"entryLatencyMS":0`, `"entryLatencyMS":`+token, 1)
			if cfg, err := DecodeFrozenLevelConfigJSON([]byte(candidate)); err == nil || cfg != nil {
				t.Fatal("invalid number accepted")
			}
		})
	}
	for _, tt := range []struct{ field, old, min, max, below, above string }{
		{"timeoutMinutes", "60", "1", "150119987579", "0", "150119987580"},
		{"maxPredecessorAgeMS", "5000", "1", "9007199254740991", "0", "9007199254740992"},
		{"maxReceiptAgeMS", "5000", "1", "9007199254740991", "0", "9007199254740992"},
		{"entryLatencyMS", "0", "0", "9007199254740991", "-1", "9007199254740992"},
		{"amendmentLatencyMS", "0", "0", "9007199254740991", "-1", "9007199254740992"},
	} {
		for _, token := range []string{tt.min, tt.max, tt.below, tt.above} {
			candidate := strings.Replace(raw, `"`+tt.field+`":`+tt.old, `"`+tt.field+`":`+token, 1)
			_, err := DecodeFrozenLevelConfigJSON([]byte(candidate))
			if (err == nil) != (token == tt.min || token == tt.max) {
				t.Fatalf("%s boundary %s: %v", tt.field, token, err)
			}
		}
	}
}

func TestFrozenLevelJSONDecimalNormalization(t *testing.T) {
	raw := string(frozenConfigTestFile(t, "config.canonical.json"))
	for _, token := range []string{"1", "1.0", "1e0", "0.1e1", "10e-1", "5e-324", "1.7976931348623157e308", "9007199254740991.1"} {
		candidate := strings.Replace(raw, `"activationR":1`, `"activationR":`+token, 1)
		cfg, err := DecodeFrozenLevelConfigJSON([]byte(candidate))
		if err != nil {
			t.Fatalf("finite positive decimal %s: %v", token, err)
		}
		expected, _ := strconv.ParseFloat(token, 64)
		if frozenConfigTestSpec(cfg)["activationR"] != expected {
			t.Fatal("decimal not normalized to nearest float64")
		}
	}
	for _, token := range []string{"0", "-0", "-0.0", "-0e1", "-1", "1e309", "1e-324", "1e-10000", "1e10001", "0e10001", `"1"`, "null", "false", "NaN", "Inf"} {
		candidate := strings.Replace(raw, `"activationR":1`, `"activationR":`+token, 1)
		if _, err := DecodeFrozenLevelConfigJSON([]byte(candidate)); err == nil {
			t.Fatalf("invalid decimal accepted: %s", token)
		}
	}
	base := frozenConfigTestIdentity(t, frozenConfigTestFixture(t))
	alias := strings.ReplaceAll(raw, `"activationR":1`, `"activationR":0.10e1`)
	alias = strings.ReplaceAll(alias, `"priceGrid":0.01`, `"priceGrid":1e-2`)
	alias = strings.ReplaceAll(alias, `"timeoutMinutes":60`, `"timeoutMinutes":6.00e1`)
	alias = strings.ReplaceAll(alias, `"dslVersion":7`, `"dslVersion":7.0`)
	alias = strings.ReplaceAll(alias, `"max":9007199254740991`, `"max":9007199254740991.000`)
	alias = strings.ReplaceAll(alias, `"name":`, `"\u006eame":`)
	alias = strings.ReplaceAll(alias, `\u003c`, `<`)
	alias = " \n\t" + alias + "\r\n "
	cfg, err := DecodeFrozenLevelConfigJSON([]byte(alias))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(base, frozenConfigTestIdentity(t, cfg)) {
		t.Fatal("lexical aliases changed canonical identity")
	}
	pretty, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = DecodeFrozenLevelConfigJSON(pretty)
	if err != nil || !reflect.DeepEqual(base, frozenConfigTestIdentity(t, cfg)) {
		t.Fatal("whitespace/object ordering changed identity")
	}
}

func TestFrozenLevelStrictJSONUnicodeAndDuplicates(t *testing.T) {
	for _, raw := range []string{
		`{"id":"one","id":"two"}`, `{"id":"one","\u0069d":"two"}`,
		`{"outer":{"id":null,"\u0069d":true}}`, `[{"id":1,"id":1}]`,
		`{"\ud834\udd1e":1,"𝄞":2}`, `{"a":{},"a":{}}`,
	} {
		if _, err := strictFrozenJSON([]byte(raw)); err == nil || !strings.Contains(err.Error(), "duplicate") {
			t.Fatalf("decoded duplicate not rejected: %s: %v", raw, err)
		}
	}
	for _, raw := range []string{
		``, `{} {}`, `{} true`, `{} // comment`, `/*comment*/ {}`, "\ufeff{}", "{}\v",
		`{"a":}`, `{"a":1,}`, `[1,]`, `{"a" 1}`, `{a:1}`, `truefalse`,
		`{"name":"\ud800"}`, `{"name":"\udc00"}`, `{"name":"\ud800\ud800"}`, `{"name":"\ud800\u0041"}`,
		`{"name":"\ud800a"}`, `{"name":"\ud800\\udc00"}`, `{"name":"\u12"}`, `{"name":"\uZZZZ"}`,
		`{"name":"\x20"}`, `{"name":"\v"}`, "{\"name\":\"literal\nnewline\"}", string([]byte{'"', 0xff, '"'}),
		string([]byte{'"', 0xed, 0xa0, 0x80, '"'}), string([]byte{'"', 0xc0, 0xaf, '"'}),
	} {
		if _, err := strictFrozenJSON([]byte(raw)); err == nil {
			t.Fatalf("malformed raw JSON accepted: %q", raw)
		}
	}
	for _, tt := range []struct{ raw, expected string }{
		{`"\ud834\udd1e"`, "𝄞"}, {`"\uD800\uDC00"`, "\U00010000"}, {`"\uDBFF\uDFFF"`, "\U0010ffff"},
		{`"\ufffd"`, "\ufffd"}, {`"�"`, "\ufffd"}, {`"\u0000"`, "\x00"},
		{`"\"\\\/\b\f\n\r\t"`, "\"\\/\b\f\n\r\t"}, {`"Ω # { }"`, "Ω # { }"}, {`""`, ""},
	} {
		got, err := strictFrozenJSONString([]byte(tt.raw))
		if err != nil || got != tt.expected {
			t.Fatalf("valid string %s: got %q, %v", tt.raw, got, err)
		}
	}
	for _, raw := range []string{` "a"`, `"a" `, `"a" "b"`, `'a'`, "`a`", `"\ud800"`, `"\udc00"`, `"\q"`} {
		if _, err := strictFrozenJSONString([]byte(raw)); err == nil {
			t.Fatalf("invalid standalone string accepted: %s", raw)
		}
	}
	valid := frozenConfigTestFile(t, "config.canonical.json")
	for _, insert := range []string{`"description":"extra",`, `"\u0064escription":"extra",`} {
		candidate := append([]byte("{"+insert), valid[1:]...)
		if _, err := DecodeFrozenLevelConfigJSON(candidate); err == nil {
			t.Fatal("duplicate root field accepted by full decoder")
		}
	}
}

func TestFrozenLevelJSONResourceLimits(t *testing.T) {
	valid := frozenConfigTestFile(t, "config.canonical.json")
	atLimit := append(append([]byte{}, valid...), bytes.Repeat([]byte{' '}, frozenLevelMaxBytes-len(valid))...)
	if _, err := DecodeFrozenLevelConfigJSON(atLimit); err != nil {
		t.Fatalf("exact 1 MiB valid input rejected: %v", err)
	}
	if _, err := DecodeFrozenLevelConfigJSON(append(atLimit, ' ')); err == nil {
		t.Fatal("input over 1 MiB accepted")
	}
	for _, depth := range []int{63, 64, 65} {
		raw := strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth)
		_, err := strictFrozenJSON([]byte(raw))
		if (err == nil) != (depth <= 64) {
			t.Fatalf("depth %d: %v", depth, err)
		}
		raw = strings.Repeat(`{"x":`, depth) + "0" + strings.Repeat("}", depth)
		_, err = strictFrozenJSON([]byte(raw))
		if (err == nil) != (depth <= 64) {
			t.Fatalf("object depth %d: %v", depth, err)
		}
	}
	for _, length := range []int{1023, 1024, 1025} {
		// An exact integer alias for zero exercises the lexical cap without
		// also failing a magnitude bound or allocating a huge power of ten.
		token := "0." + strings.Repeat("0", length-2)
		_, err := strictFrozenJSON([]byte(token))
		if (err == nil) != (length <= 1024) {
			t.Fatalf("numeric length %d: %v", length, err)
		}
		cfg := frozenConfigTestFixture(t)
		frozenConfigTestSpec(cfg)["entryLatencyMS"] = json.Number(token)
		_, err = NormalizeFrozenLevelConfig(cfg)
		if (err == nil) != (length <= 1024) {
			t.Fatalf("Go json.Number length %d: %v", length, err)
		}
	}
	for _, exponent := range []int{-10001, -10000, 10000, 10001} {
		_, err := strictFrozenJSON([]byte(fmt.Sprintf("0e%d", exponent)))
		if (err == nil) != (exponent >= -10000 && exponent <= 10000) {
			t.Fatalf("exponent %d: %v", exponent, err)
		}
	}
}

func FuzzFrozenLevelStrictJSON(f *testing.F) {
	for _, raw := range []string{`{}`, `{"a":1,"\u0061":2}`, `"\ud834\udd1e"`, `"\ud800"`, `9007199254740991.1`, `-0e0`, `1e10001`, `[true,false,null]`} {
		f.Add([]byte(raw))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		value, err := strictFrozenJSON(raw)
		if err != nil {
			return
		}
		if !json.Valid(raw) {
			t.Fatalf("strict decoder accepted invalid JSON: %q", raw)
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var expected any
		if err := decoder.Decode(&expected); err != nil || !reflect.DeepEqual(value, expected) {
			t.Fatalf("strict accepted value differs from JSON semantics: %q", raw)
		}
	})
}
