package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func assertedDualSource(timeframe string) string {
	fast, slow, trail := 10, 40, 2
	if timeframe == "4h" {
		fast, slow, trail = 20, 80, 3
	}
	return fmt.Sprintf(`dsl v7
strategy "Synthetic registered-parameter consistency witness"
market conditions { slices(XAUUSD %s) }
setup {
 type: dual ema resumption
 ema fast %d
 ema slow %d
 ema slow rise 12
 ema wilder atr 20
 stop initial 2.5 ATR
 trail close %d ATR
 fallback below slow ema
}
filters { side long only }
execution { risk 200 USD }`, timeframe, fast, slow, trail)
}

func assertedDualClaims(timeframe string) map[string]any {
	fast, slow, trail, id := 10, 40, 2, "dslDualEmaResumptionXauusdDaily"
	if timeframe == "4h" {
		fast, slow, trail, id = 20, 80, 3, "dslDualEmaResumptionXauusdFourHour"
	}
	return map[string]any{"schema": InteractiveSourceAssertionsSchema, "strategyId": id,
		"params": map[string]any{"fastEmaLen": fast, "slowEmaLen": slow, "slowRiseBars": 12,
			"atrLen": 20, "stopAtr": 2.5, "trailAtr": trail, "allowLong": 1, "allowShort": 0, "riskUsd": 200},
		"contextOptions": map[string]any{}, "contextRequirements": []string{"sessions"}, "preferredRangeMethod": "zone"}
}

func assertedDualRequest(t *testing.T, timeframe string, inspect bool, assertions any) []byte {
	t.Helper()
	fields := map[string]any{"schema": InteractiveSourceProfileSchema, "symbol": "XAUUSD", "timeframe": timeframe}
	if !inspect {
		duration := 86_400_000
		if timeframe == "4h" {
			duration = 14_400_000
		}
		rows := make([][]float64, 400)
		for i := range rows {
			close := 1900 + float64(i)*0.2
			if i%12 == 9 {
				close -= 2.4
			}
			rows[i] = []float64{float64(i * duration), close - 0.1, close + 0.3, close - 0.3, close, 1}
		}
		fields["schema"], fields["strategyId"], fields["case"] = runFixtureSchema, InteractiveDraftStrategyID, "source-assertions"
		fields["rangeMethod"], fields["bars"] = "pivot", rows
		fields["costs"] = map[string]any{"fillOn": "nextOpen", "startEquity": 10000, "feePerUnit": 0.1, "slippage": 0.06, "slippageBps": 0}
	}
	if assertions != nil {
		fields["sourceAssertions"] = assertions
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestInteractiveSourceAssertionsPreserveExecution(t *testing.T) {
	for _, timeframe := range []string{"1d", "4h"} {
		t.Run(timeframe, func(t *testing.T) {
			source, claims := assertedDualSource(timeframe), assertedDualClaims(timeframe)
			profileRaw := assertedDualRequest(t, timeframe, true, claims)
			profile, err := InspectInteractiveSource(profileRaw, source)
			if err != nil || profile.SourceAssertions == nil || profile.Profile.RangeMethod != "pivot" || *profile.SourceAssertions.PreferredRangeMethod != "zone" {
				t.Fatalf("profile: %+v, %v", profile, err)
			}
			assertSourceProvenance(t, profile.Provenance, profileRaw, source)
			want, _ := json.Marshal(claims)
			got, _ := json.Marshal(profile.SourceAssertions)
			if string(got) == "" || !sameJSON(got, want) {
				t.Fatalf("canonical echo: %s != %s", got, want)
			}
			for _, calculation := range []string{"raw", "heikinAshi"} {
				raw := assertedDualRequest(t, timeframe, false, claims)
				var fields map[string]any
				_ = json.Unmarshal(raw, &fields)
				fields["calculationSource"] = calculation
				raw, _ = json.Marshal(fields)
				result, err := RunInteractiveDraftFixture(raw, source)
				if err != nil || result.SourceAssertions == nil || len(result.Run.Trades) == 0 {
					t.Fatalf("%s result: %+v, %v", calculation, result, err)
				}
				assertSourceProvenance(t, result.Provenance, raw, source)
				echo, _ := json.Marshal(result.SourceAssertions)
				if !sameJSON(echo, want) {
					t.Fatal("finalized echo differs from Go canonical claims")
				}
				delete(fields, "sourceAssertions")
				baseRaw, _ := json.Marshal(fields)
				base, err := RunInteractiveDraftFixture(baseRaw, source)
				if err != nil || base.SourceAssertions != nil {
					t.Fatalf("assertion-free run: %v", err)
				}
				result.SourceAssertions, result.Provenance = nil, base.Provenance
				if !reflect.DeepEqual(base, result) {
					t.Fatal("assertions changed execution/accounting instead of checking consistency")
				}
				if calculation == "raw" {
					prefix, err := RunInteractiveDraftPrefixFixture(raw, source)
					if err != nil || prefix.SourceAssertions == nil {
						t.Fatalf("prefix: %v", err)
					}
					assertSourceProvenance(t, prefix.Provenance, raw, source)
					echo, _ := json.Marshal(prefix.SourceAssertions)
					if !sameJSON(echo, want) {
						t.Fatal("prefix echo differs from Go canonical claims")
					}
					basePrefix, err := RunInteractiveDraftPrefixFixture(baseRaw, source)
					if err != nil {
						t.Fatal(err)
					}
					prefix.SourceAssertions, prefix.Provenance = nil, basePrefix.Provenance
					if !reflect.DeepEqual(prefix, basePrefix) {
						t.Fatal("assertions changed preserved exposure or position identity")
					}
				} else if _, err := RunInteractiveDraftPrefixFixture(raw, source); err == nil {
					t.Fatal("assertions admitted unqualified HA prefix")
				}
			}
		})
	}
}

func sameJSON(a, b []byte) bool {
	var aa, bb any
	return json.Unmarshal(a, &aa) == nil && json.Unmarshal(b, &bb) == nil && reflect.DeepEqual(aa, bb)
}

func TestInteractiveSourceAssertionsRecheckedAtEveryEntry(t *testing.T) {
	mutations := map[string]func(map[string]any){
		"schema":                                func(c map[string]any) { c["schema"] = "unknown" },
		"id":                                    func(c map[string]any) { c["strategyId"] = "dslEditorStrategy" },
		"wrong route id":                        func(c map[string]any) { c["strategyId"] = "dslDualEmaResumptionXauusdDaily" },
		"unknown field":                         func(c map[string]any) { c["override"] = true },
		"empty requirements":                    func(c map[string]any) { c["contextRequirements"] = []string{} },
		"null requirements":                     func(c map[string]any) { c["contextRequirements"] = nil },
		"extra requirement":                     func(c map[string]any) { c["contextRequirements"] = []string{"sessions", "vp"} },
		"duplicate requirement":                 func(c map[string]any) { c["contextRequirements"] = []string{"sessions", "sessions"} },
		"requirements object":                   func(c map[string]any) { c["contextRequirements"] = map[string]any{"sessions": true} },
		"nonempty options":                      func(c map[string]any) { c["contextOptions"] = map[string]any{"sessions": true} },
		"null options":                          func(c map[string]any) { c["contextOptions"] = nil },
		"array options":                         func(c map[string]any) { c["contextOptions"] = []any{} },
		"null preference":                       func(c map[string]any) { c["preferredRangeMethod"] = nil },
		"source instead of registry preference": func(c map[string]any) { c["preferredRangeMethod"] = "pivot" },
		"null params":                           func(c map[string]any) { c["params"] = nil },
		"unknown param":                         func(c map[string]any) { c["params"].(map[string]any)["riskPercent"] = 2 },
	}
	for _, key := range []string{"schema", "strategyId", "params", "contextOptions", "contextRequirements", "preferredRangeMethod"} {
		key := key
		mutations["missing "+key] = func(c map[string]any) { delete(c, key) }
	}
	for key := range assertedDualClaims("4h")["params"].(map[string]any) {
		key := key
		for name, value := range map[string]any{"null": nil, "string": "1", "bool": true, "array": []any{}, "mismatch": -10} {
			value := value
			mutations[key+" "+name] = func(c map[string]any) { c["params"].(map[string]any)[key] = value }
		}
		mutations["missing param "+key] = func(c map[string]any) { delete(c["params"].(map[string]any), key) }
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			claims := assertedDualClaims("4h")
			mutate(claims)
			assertClaimsRejected(t, claims)
		})
	}
}

func assertClaimsRejected(t *testing.T, claims any) {
	t.Helper()
	source := assertedDualSource("4h")
	if _, err := InspectInteractiveSource(assertedDualRequest(t, "4h", true, claims), source); err == nil {
		t.Fatal("inspection admitted malformed or inconsistent assertions")
	}
	raw := assertedDualRequest(t, "4h", false, claims)
	if _, err := RunInteractiveDraftFixture(raw, source); err == nil {
		t.Fatal("finalized execution admitted malformed or inconsistent assertions")
	}
	if _, err := RunInteractiveDraftPrefixFixture(raw, source); err == nil {
		t.Fatal("prefix execution admitted malformed or inconsistent assertions")
	}
}

func TestInteractiveSourceAssertionsRejectDuplicateJSONAndSourceDrift(t *testing.T) {
	claims, _ := json.Marshal(assertedDualClaims("4h"))
	for _, bad := range []string{
		"null", `[]`, `true`,
		strings.Replace(string(claims), `"schema":`, `"schema":"ignored","schema":`, 1),
		strings.Replace(string(claims), `"fastEmaLen":`, `"fastEmaLen":20,"fastEmaLen":`, 1),
		strings.Replace(string(claims), `"fastEmaLen":20`, `"fastEmaLen":1e999`, 1),
		strings.Replace(string(claims), `"params":`, `"Params":`, 1),
	} {
		t.Run(bad, func(t *testing.T) { assertClaimsRejected(t, json.RawMessage(bad)) })
	}
	source := assertedDualSource("4h")
	for _, drift := range []string{
		strings.Replace(source, "ema fast 20", "ema fast 21", 1),
		strings.Replace(source, "risk 200 USD", "risk 300 USD", 1),
		strings.Replace(source, "XAUUSD 4h", "XAUUSD 1d", 1),
		draftSMASource,
	} {
		if _, err := InspectInteractiveSource(assertedDualRequest(t, "4h", true, assertedDualClaims("4h")), drift); err == nil {
			t.Fatal("inspection reused stale source claims")
		}
		raw := assertedDualRequest(t, "4h", false, assertedDualClaims("4h"))
		if _, err := RunInteractiveDraftFixture(raw, drift); err == nil {
			t.Fatal("finalized execution reused stale source claims")
		}
		if _, err := RunInteractiveDraftPrefixFixture(raw, drift); err == nil {
			t.Fatal("prefix reused stale source claims")
		}
	}
	raw := assertedDualRequest(t, "4h", false, assertedDualClaims("4h"))
	ordinary := strings.Replace(string(raw), InteractiveDraftStrategyID, "ordinary-dual", 1)
	if _, err := RunInteractiveFixture([]byte(ordinary), source); err == nil {
		t.Fatal("ordinary interactive path silently discarded source assertions")
	}
	zone := strings.Replace(string(raw), `"rangeMethod":"pivot"`, `"rangeMethod":"zone"`, 1)
	if _, err := RunInteractiveDraftFixture([]byte(zone), source); err == nil {
		t.Fatal("registry preference overrode the Go source range method")
	}
	if _, err := RunInteractiveDraftPrefixFixture([]byte(zone), source); err == nil {
		t.Fatal("prefix registry preference overrode the Go source range method")
	}
}

func assertSourceProvenance(t *testing.T, provenance InteractiveProvenance, raw []byte, source string) {
	t.Helper()
	rh, sh := sha256.Sum256(raw), sha256.Sum256([]byte(source))
	if provenance.FixtureSHA256 != hex.EncodeToString(rh[:]) || provenance.SourceSHA256 != hex.EncodeToString(sh[:]) {
		t.Fatal("exact original source/request bytes were not retained in provenance")
	}
}
