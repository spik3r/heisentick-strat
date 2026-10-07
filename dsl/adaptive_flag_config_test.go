package dsl

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func adaptiveFlagParseOK(t *testing.T, source string) Config {
	t.Helper()
	r, e := Parse(source)
	if e != nil || len(r.Errors) != 0 {
		t.Fatalf("parse: %v %v", e, r.Errors)
	}
	if !reflect.DeepEqual(r.Warnings, []string{AdaptiveFlagDedicatedRunnerRequired}) || len(r.Diagnostics) != 1 {
		t.Fatalf("missing explicit native boundary: %+v", r)
	}
	if _, e = DecodeAdaptiveVolumeFlag(r.Config); e != nil {
		t.Fatal(e)
	}
	return r.Config
}
func adaptiveFlagReject(t *testing.T, source string) {
	t.Helper()
	r, e := Parse(source)
	if e != nil || len(r.Errors) != 1 || len(r.Config) != 0 || !strings.HasPrefix(r.Errors[0], "adaptive volume flag: ") {
		t.Fatalf("reserved source escaped strict parser: %q\n%+v %v", source, r, e)
	}
}
func adaptiveFlagObject(c Config) map[string]any { return c["adaptiveVolumeFlag"].(map[string]any) }
func adaptiveFlagRuleObject(c Config) map[string]any {
	return adaptiveFlagObject(c)["rules"].(map[string]any)
}

func TestAdaptiveFlagExplicitPresetIdentity(t *testing.T) {
	a, _ := AdaptiveFlagPreset("INITIAL")
	b, _ := AdaptiveFlagPreset("TWEAKED")
	c, _ := AdaptiveFlagPreset("SNAPSHOT_C")
	changed := []string{}
	for k, v := range a.projection() {
		if !reflect.DeepEqual(v, b.projection()[k]) {
			changed = append(changed, k)
		}
	}
	if len(changed) != 6 {
		t.Fatalf("A/B differences:%v", changed)
	}
	b.ATRStopMult = 2
	if b != c {
		t.Fatal("snapshot C changed more than stop buffer")
	}
	if a.ATRStopMult != 1.2 {
		t.Fatal("preset mutation escaped")
	}
	for _, name := range []string{"INITIAL", "TWEAKED", "SNAPSHOT_C"} {
		rules, _ := AdaptiveFlagPreset(name)
		cfg := AdaptiveFlagConfig("Fixture", "", "M30", name, rules)
		spec, e := DecodeAdaptiveVolumeFlag(cfg)
		if e != nil || spec.Rules != rules {
			t.Fatalf("%s:%v", name, e)
		}
		raw, _ := json.Marshal(cfg)
		round, e := DecodeAdaptiveVolumeFlagConfigJSON(raw)
		if e != nil || !reflect.DeepEqual(cfg, round) {
			t.Fatalf("roundtrip:%v", e)
		}
		adaptiveFlagRuleObject(cfg)["atrStopMult"] = 9.0
		if _, e = DecodeAdaptiveVolumeFlag(cfg); e == nil {
			t.Fatal("silently changed named preset")
		}
	}
}

func TestAdaptiveFlagStrictConfigMutations(t *testing.T) {
	cases := map[string]func(Config){
		"root unknown":   func(c Config) { c["riskUSD"] = 1 },
		"wrong family":   func(c Config) { c["setupType"] = "flagContinuation" },
		"invalid name":   func(c Config) { c["name"] = string([]byte{255}) },
		"blank name":     func(c Config) { c["name"] = " " },
		"null object":    func(c Config) { c["adaptiveVolumeFlag"] = nil },
		"unknown spec":   func(c Config) { adaptiveFlagObject(c)["cost"] = 0 },
		"policy":         func(c Config) { adaptiveFlagObject(c)["policy"] = "immediate" },
		"unknown preset": func(c Config) { adaptiveFlagObject(c)["bundle"] = "C" },
		"tf":             func(c Config) { adaptiveFlagObject(c)["timeframe"] = "M5" },
		"unknown rule":   func(c Config) { adaptiveFlagRuleObject(c)["leverage"] = 1 },
		"nan":            func(c Config) { adaptiveFlagRuleObject(c)["targetR"] = math.NaN() },
		"fractional int": func(c Config) { adaptiveFlagRuleObject(c)["pivotSensitivity"] = json.Number("3.0000000000000000001") },
		"rounded preset": func(c Config) { adaptiveFlagRuleObject(c)["entryBufferATR"] = json.Number("0.050000000000000000001") },
	}
	cfg := adaptiveFlagParseOK(t, adaptiveFlagTestSource)
	for key := range cfg {
		k := key
		cases["missing root "+k] = func(c Config) { delete(c, k) }
		cases["null root "+k] = func(c Config) { c[k] = nil }
	}
	for key := range adaptiveFlagObject(cfg) {
		k := key
		cases["missing spec "+k] = func(c Config) { delete(adaptiveFlagObject(c), k) }
		cases["null spec "+k] = func(c Config) { adaptiveFlagObject(c)[k] = nil }
	}
	for key := range adaptiveFlagRuleObject(cfg) {
		k := key
		cases["missing rule "+k] = func(c Config) { delete(adaptiveFlagRuleObject(c), k) }
		cases["null rule "+k] = func(c Config) { adaptiveFlagRuleObject(c)[k] = nil }
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := adaptiveFlagParseOK(t, adaptiveFlagTestSource)
			mutate(c)
			if _, e := DecodeAdaptiveVolumeFlag(c); e == nil {
				t.Fatal("accepted malformed config")
			}
		})
	}
}

func TestAdaptiveFlagCustomAndRawNumbers(t *testing.T) {
	c := adaptiveFlagParseOK(t, adaptiveFlagTestSource)
	adaptiveFlagObject(c)["bundle"] = "CUSTOM"
	rules := adaptiveFlagRuleObject(c)
	for _, k := range []string{"entryBufferATR", "maxFlagRetrace", "volumeSMAMult", "flagWidthPoleMult"} {
		rules[k] = 0.0
	}
	if _, e := DecodeAdaptiveVolumeFlag(c); e != nil {
		t.Fatal("explicit zero custom threshold", e)
	}
	cases := map[string]any{"negative": -1., "negative zero": math.Copysign(0, -1), "tiny underflow": json.Number("1e-500"), "infinite": math.Inf(1), "bool": true, "quoted": "1.2"}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			m := adaptiveFlagParseOK(t, adaptiveFlagTestSource)
			adaptiveFlagObject(m)["bundle"] = "CUSTOM"
			adaptiveFlagRuleObject(m)["atrStopMult"] = value
			if _, e := DecodeAdaptiveVolumeFlag(m); e == nil {
				t.Fatal("accepted invalid decimal")
			}
		})
	}
	for _, value := range []any{0, 1000001, json.Number("1.00000000001"), float64(3)} {
		rules["minFlagBars"] = value
		if _, e := DecodeAdaptiveVolumeFlag(c); e == nil {
			t.Fatal("invalid integer", value)
		}
	}
	rules["minFlagBars"] = int64(100)
	if _, e := DecodeAdaptiveVolumeFlag(c); e == nil {
		t.Fatal("min/max reversed")
	}
	raw, _ := json.Marshal(adaptiveFlagParseOK(t, adaptiveFlagTestSource))
	bad := [][]byte{
		bytes.Replace(raw, []byte(`"dslVersion":7`), []byte(`"dslVersion":7,"dslVersion":7`), 1),
		bytes.Replace(raw, []byte(`"targetR":2.5`), []byte(`"targetR":2.5,"targetR":2.5`), 1),
		bytes.Replace(raw, []byte(`"targetR":2.5`), []byte(`"targ\u0065tR":2.5,"targetR":2.5`), 1),
		bytes.Replace(raw, []byte(`"name":`), []byte(`"unknown":1,"name":`), 1),
		bytes.Replace(raw, []byte(`"targetR":2.5`), []byte(`"targetR":2.50000000000000000001`), 1),
		append(append([]byte{}, raw...), []byte(` {}`)...), []byte(`null`), []byte(`[]`), append(append([]byte{}, raw...), 255),
		bytes.Replace(raw, []byte(`"description":`), []byte(`"description":"\ud800","oldDescription":`), 1),
	}
	for i, b := range bad {
		if _, e := DecodeAdaptiveVolumeFlagConfigJSON(b); e == nil {
			t.Fatalf("raw malformed case %d accepted", i)
		}
	}
	equivalent := bytes.Replace(raw, []byte(`"pivotSensitivity":3`), []byte(`"pivotSensitivity":3.0`), 1)
	if _, e := DecodeAdaptiveVolumeFlagConfigJSON(equivalent); e != nil {
		t.Fatal("exact equivalent integer", e)
	}
}

func TestAdaptiveFlagReservedConfigAliases(t *testing.T) {
	for _, key := range []string{"adaptiveVolumeFlag", "ADAPTIVEVOLUMEFLAG", "adaptive volume flag", "adaptive_volume_flag", "adaptive-volume-flag", "adaptive\u2003volumeFlag"} {
		if !IsAdaptiveVolumeFlagReserved(Config{key: nil, "setupType": "flagContinuation"}) {
			t.Fatal("unreserved object", key)
		}
		if !IsAdaptiveVolumeFlagReserved(Config{"Setup_Type": key}) {
			t.Fatal("unreserved selector", key)
		}
	}
	if !IsAdaptiveVolumeFlagReserved(Config{"setupType": FamilyAdaptiveVolumeFlag}) {
		t.Fatal("typed family")
	}
	for _, c := range []Config{nil, {}, {"description": "adaptiveVolumeFlag"}, {"setupType": "goldFlagReference"}} {
		if IsAdaptiveVolumeFlagReserved(c) {
			t.Fatal("false reservation", c)
		}
	}
}

func TestAdaptiveFlagConfigSchema(t *testing.T) {
	schema, path := loadContractSchema(t, "adaptive-volume-flag-config-v1.schema.json")
	for _, bundle := range []string{"INITIAL", "TWEAKED", "SNAPSHOT_C", "CUSTOM"} {
		rules, _ := AdaptiveFlagPreset("INITIAL")
		if bundle != "CUSTOM" {
			rules, _ = AdaptiveFlagPreset(bundle)
		}
		c := cloneContractObject(AdaptiveFlagConfig("Fixture", "", "H1", bundle, rules))
		if e := validateContract(c, schema, path, schema, "config"); e != nil {
			t.Fatal(e)
		}
		c["unknown"] = 1.
		assertInvalidContract(t, c, schema, path, "unknown root")
	}
}

// The generic helper extension is test-only; numeric boundary semantics are
// pinned separately rather than weakening the new family's published schema.
func TestAdaptiveFlagSchemaNumericBoundSupport(t *testing.T) {
	for _, key := range []string{"maximum", "exclusiveMinimum"} {
		schema := contractSchema{"type": "number", key: float64(2)}
		good := float64(2)
		if key == "exclusiveMinimum" {
			good = math.Nextafter(2, math.Inf(1))
		}
		if e := validateContract(good, schema, "synthetic.schema.json", schema, "value"); e != nil {
			t.Fatal(e)
		}
		bad := math.Nextafter(2, math.Inf(1))
		if key == "exclusiveMinimum" {
			bad = 2
		}
		for _, value := range []any{bad, "2", true, nil, math.NaN(), math.Inf(1), math.Inf(-1)} {
			if e := validateContract(value, schema, "synthetic.schema.json", schema, "value"); e == nil {
				t.Fatalf("%s accepted %v", key, value)
			}
		}
		for _, bound := range []any{"2", true, nil, math.NaN(), math.Inf(1)} {
			malformed := contractSchema{"type": "number", key: bound}
			if e := validateContract(2., malformed, "synthetic.schema.json", malformed, "value"); e == nil {
				t.Fatalf("%s accepted malformed bound %v", key, bound)
			}
		}
	}
}

func TestAdaptiveFlagPunctuatedDiscriminatorKeysRemainReserved(t *testing.T) {
	for _, key := range []string{"setup.type", "setup:type", "setup/type", "setup|type", "setup[type]", "Setup+Type", "setup\u200bType"} {
		if !IsAdaptiveVolumeFlagReserved(Config{"setupType": "flagContinuation", key: "adaptiveVolumeFlag"}) {
			t.Fatal("unreserved damaged discriminator", key)
		}
		if IsAdaptiveVolumeFlagReserved(Config{"setupType": "flagContinuation", key: "ordinary-metadata"}) {
			t.Fatal("changed legacy config without adaptive intent", key)
		}
	}
}
