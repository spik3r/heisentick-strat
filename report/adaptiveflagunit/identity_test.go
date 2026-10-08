package adaptiveflagunit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
)

func TestCorePolicyHelpersMatchFrozenB0Vectors(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testsupport", "testdata", "adaptive-flag-unit", "policy-identity-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Vectors []struct {
			ID   string `json:"id"`
			JSON string `json:"compactJSON"`
			SHA  string `json:"sha256"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, vector := range fixture.Vectors {
		var value any
		var got string
		if strings.HasPrefix(vector.ID, "economic-") {
			policy, policyErr := policyFor(strings.TrimPrefix(vector.ID, "economic-"))
			if policyErr != nil {
				t.Fatal(policyErr)
			}
			value = economicPolicy(policy)
			got, err = economicPolicySHA256(policy)
		} else {
			value = numericalDefinition()
			got, err = numericalPolicySHA256()
		}
		if err != nil || got != vector.SHA {
			t.Fatalf("%s: hash %q, error %v", vector.ID, got, err)
		}
		encoded, err := json.Marshal(value)
		if err != nil || string(encoded) != vector.JSON {
			t.Fatalf("%s: helper bytes differ from frozen B0 vector: %s", vector.ID, encoded)
		}
	}
	for _, name := range []string{"", "raw", "BASIC", "CUSTOM", "RAW "} {
		if _, err := policyFor(name); err == nil {
			t.Fatalf("unsupported policy %q accepted", name)
		}
	}
	for _, mutate := range []func(*CostPolicy){
		func(p *CostPolicy) { p.PerFill = math.Copysign(0, -1) },
		func(p *CostPolicy) { p.Spread = nil },
		func(p *CostPolicy) { *p.Commission = math.NaN() },
		func(p *CostPolicy) { zero := 0.0; p.Aggregate = &zero },
	} {
		policy, _ := policyFor("RAW")
		mutate(&policy)
		if _, err := economicPolicySHA256(policy); err == nil {
			t.Fatal("mutated fixed policy was hashed")
		}
		fresh, _ := policyFor("RAW")
		if _, err := economicPolicySHA256(fresh); err != nil {
			t.Fatal("policy mutation leaked across calls")
		}
	}
}

func inventedIdentityRun() *engine.AdaptiveFlagResult {
	return &engine.AdaptiveFlagResult{
		SourcePineSHA256: strings.Repeat("a", 64), TimeframeMS: 1800000,
		Snapshots: []engine.AdaptiveFlagSnapshot{
			{AdaptiveFlagBar: engine.AdaptiveFlagBar{Index: 0, OpenT: 0}},
			{AdaptiveFlagBar: engine.AdaptiveFlagBar{Index: 1, OpenT: 1800000}},
			{AdaptiveFlagBar: engine.AdaptiveFlagBar{Index: 2, OpenT: 3600000}, LastHigh: &engine.AdaptiveFlagPivot{Index: 0, ConfirmationIdx: 1}},
		},
	}
}

func literalIdentitySHA256(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func TestCoreOriginIdentityRestrictedCanonicalGrammar(t *testing.T) {
	run := inventedIdentityRun()
	episode, candidate, err := originIdentity(run, "long", 2, "feed:gold/v1")
	if err != nil {
		t.Fatal(err)
	}
	// Independent, hand-written grammar vectors; no Python execution or economic
	// output is involved. Null low pivot and time zero are both significant.
	episodeBytes := `{"data_source_id":"feed:gold/v1","high_pivot":{"confirmation_open_ms":1800000,"occurrence_open_ms":0},"low_pivot":null,"side":"long","source_pine_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","timeframe_ms":1800000}`
	candidateBytes := `{"data_source_id":"feed:gold/v1","decision_bar_open_ms":3600000,"high_pivot":{"confirmation_open_ms":1800000,"occurrence_open_ms":0},"low_pivot":null,"side":"long","source_pine_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","timeframe_ms":1800000}`
	if episode != literalIdentitySHA256(episodeBytes) || candidate != literalIdentitySHA256(candidateBytes) || episode == candidate {
		t.Fatal("causal identity differs from the sorted integer/string/null grammar")
	}
	run.InputSHA256, run.ConfigSHA256 = strings.Repeat("b", 64), strings.Repeat("c", 64)
	run.ExecutionWindow = &engine.AdaptiveFlagExecutionWindow{TradeFromMS: 3600000, TradeToMS: 9000000}
	run.Snapshots = append(run.Snapshots, engine.AdaptiveFlagSnapshot{AdaptiveFlagBar: engine.AdaptiveFlagBar{Index: 3, OpenT: 5400000}, LastHigh: run.Snapshots[2].LastHigh})
	unchangedEpisode, unchangedCandidate, err := originIdentity(run, "long", 2, "feed:gold/v1")
	if err != nil || unchangedEpisode != episode || unchangedCandidate != candidate {
		t.Fatal("prefix extension or noncausal provenance renamed an existing identity")
	}
	nextEpisode, nextCandidate, err := originIdentity(run, "long", 3, "feed:gold/v1")
	if err != nil || nextEpisode != episode || nextCandidate == candidate {
		t.Fatal("decision row must affect only candidate identity")
	}
	for _, input := range []struct{ side, source string }{{"short", "feed:gold/v1"}, {"long", "feed:gold/v2"}} {
		e, c, err := originIdentity(run, input.side, 2, input.source)
		if err != nil || e == episode || c == candidate {
			t.Fatal("side/dataset revision must change both identities")
		}
	}
	run.Snapshots[2].LastHigh, run.Snapshots[2].LastLow = nil, &engine.AdaptiveFlagPivot{Index: 1, ConfirmationIdx: 2}
	e, c, err := originIdentity(run, "long", 2, "feed:gold/v1")
	if err != nil || e == episode || c == candidate {
		t.Fatal("pivot identities or null positions were ignored")
	}
}

func TestCoreOriginIdentityRejectsOutOfDomainInputs(t *testing.T) {
	for _, source := range []string{"", "with space", "é", "a\"b", "a\\b", "a&b", "a\x00b", strings.Repeat("a", 129)} {
		if _, _, err := originIdentity(inventedIdentityRun(), "long", 2, source); err == nil {
			t.Fatalf("escaped/non-ASCII/unbounded source %q accepted", source)
		}
	}
	for _, row := range []int{-1, 3, 1024} {
		if _, _, err := originIdentity(inventedIdentityRun(), "long", row, "ok"); err == nil {
			t.Fatal("invalid row accepted")
		}
	}
	if _, _, err := originIdentity(nil, "long", 0, "ok"); err == nil {
		t.Fatal("nil run accepted")
	}
	if _, _, err := originIdentity(inventedIdentityRun(), "LONG", 2, "ok"); err == nil {
		t.Fatal("unreviewed side accepted")
	}
	for _, mutate := range []func(*engine.AdaptiveFlagResult){
		func(r *engine.AdaptiveFlagResult) { r.SourcePineSHA256 = strings.Repeat("A", 64) },
		func(r *engine.AdaptiveFlagResult) { r.TimeframeMS = 0 },
		func(r *engine.AdaptiveFlagResult) { r.TimeframeMS = MaxCalendarEndpointMS },
		func(r *engine.AdaptiveFlagResult) { r.Snapshots[2].OpenT = -1 },
		func(r *engine.AdaptiveFlagResult) { r.Snapshots[2].OpenT = MaxCalendarEndpointMS },
		func(r *engine.AdaptiveFlagResult) { r.Snapshots[2].OpenT++ },
		func(r *engine.AdaptiveFlagResult) { r.Snapshots[2].LastHigh.Index = -1 },
		func(r *engine.AdaptiveFlagResult) { r.Snapshots[2].LastHigh.Index = 2 },
		func(r *engine.AdaptiveFlagResult) { r.Snapshots[2].LastHigh.ConfirmationIdx = 3 },
		func(r *engine.AdaptiveFlagResult) { r.Snapshots[1].OpenT = 7200000 },
	} {
		run := inventedIdentityRun()
		mutate(run)
		if _, _, err := originIdentity(run, "long", 2, "ok"); err == nil {
			t.Fatal("invalid integer/hash/pivot identity domain accepted")
		}
	}
}

func TestCoreBuildIdentityActualAndMissingStamps(t *testing.T) {
	actual := buildIdentity()
	if actual.GoVersion != runtime.Version() || actual.Compiler != runtime.Compiler || actual.GOOS != runtime.GOOS || actual.GOARCH != runtime.GOARCH {
		t.Fatal("build identity does not describe the running artifact")
	}
	if err := validateBuildIdentity(actual); err != nil {
		t.Fatal(err)
	}
	missing := identityFromBuildInfo("go1.test", "gc", "js", "wasm", nil, false)
	if missing.Verified || missing.VCSRevision != "" || missing.VCSModified != nil || validateBuildIdentity(missing) != nil {
		t.Fatal("missing build data must be explicit, unverified and unknown")
	}
	info := &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: strings.Repeat("b", 40)}, {Key: "vcs.modified", Value: "true"}}}
	stamped := identityFromBuildInfo("go1.test", "gc", "linux", "amd64", info, true)
	if !stamped.Verified || stamped.VCSRevision != strings.Repeat("b", 40) || stamped.VCSModified == nil || !*stamped.VCSModified || validateBuildIdentity(stamped) != nil {
		t.Fatal("artifact VCS stamp not preserved")
	}
	info.Settings[1].Value = "false"
	clean := identityFromBuildInfo("go1.test", "gc", "linux", "amd64", info, true)
	if !clean.Verified || clean.VCSModified == nil || *clean.VCSModified {
		t.Fatal("false VCS modified stamp lost")
	}
	info.Settings[1].Value = "unknown"
	unknown := identityFromBuildInfo("go1.test", "gc", "linux", "amd64", info, true)
	if unknown.Verified || unknown.VCSModified != nil {
		t.Fatal("unknown VCS modified stamp was certified")
	}
	for _, mutate := range []func(*BuildIdentity){
		func(i *BuildIdentity) { i.GoVersion = strings.Repeat("x", 65) },
		func(i *BuildIdentity) { i.Compiler = strings.Repeat("é", 33) },
		func(i *BuildIdentity) { i.GOOS = strings.Repeat("x", 17) },
		func(i *BuildIdentity) { i.GOARCH = strings.Repeat("x", 17) },
		func(i *BuildIdentity) { i.Compiler = "\xff" },
		func(i *BuildIdentity) { i.VCSRevision = strings.Repeat("A", 40) },
		func(i *BuildIdentity) { i.VCSRevision = strings.Repeat("a", 41) },
		func(i *BuildIdentity) { i.Verified = true },
	} {
		bad := missing
		mutate(&bad)
		if err := validateBuildIdentity(bad); err == nil {
			t.Fatalf("invalid/unbounded build identity accepted: %+v", bad)
		}
	}
}
