package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/testsupport"
)

func TestAdaptiveResearchCLIClosedFlagAndPairedSchemas(t *testing.T) {
	sp, bp, _, _ := adaptiveCLIInputs(t)
	raw, err := os.ReadFile(filepath.Join(testsupport.StratConformanceRoot(), "parse", "family-adaptive-volume-flag-snapshot_c.strat"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(sp, raw, 0600); err != nil {
		t.Fatal(err)
	}
	base := []string{"--dsl-file=" + sp, "--bars-file=" + bp}
	modes := []engine.AdaptiveFlagResearchAblation{engine.AdaptiveFlagResearchRetraceCapOff, engine.AdaptiveFlagResearchWidthCapOff, engine.AdaptiveFlagResearchBothCapsOff}
	for _, mode := range modes {
		var out bytes.Buffer
		if err = runAdaptiveFlagReport(append(append([]string{}, base...), "--research-ablation="+string(mode)), &out); err != nil {
			t.Fatal(err)
		}
		var got struct {
			Schema string                    `json:"schema"`
			Run    engine.AdaptiveFlagResult `json:"run"`
		}
		if err = json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Schema != engine.AdaptiveFlagResearchCLISchema || got.Run.Schema != engine.AdaptiveFlagResearchSchema || got.Run.ResearchPolicy.Request != mode {
			t.Fatal("research identity missing")
		}
	}
	var legacy bytes.Buffer
	if err = runAdaptiveFlagReport(base, &legacy); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(legacy.String(), "research") {
		t.Fatal("legacy metadata changed")
	}
	for _, extra := range [][]string{{"--research-ablation="}, {"--research-ablation=G1"}, {"--research-ablation=C_EMA_OFF"}, {"--research-ablation=" + string(modes[0]), "--research-ablation=" + string(modes[1])}, {"--research-ablation=" + string(modes[0]) + "," + string(modes[1])}, {"--research-ablation=" + string(modes[0]), "--use-volume-filter=false"}} {
		var out bytes.Buffer
		if err = runAdaptiveFlagReport(append(append([]string{}, base...), extra...), &out); err == nil || out.Len() != 0 {
			t.Fatal("invalid options emitted success", extra, err)
		}
	}
	for _, bundle := range []string{"INITIAL", "TWEAKED", "CUSTOM"} {
		mutated := strings.Replace(string(raw), "adaptiveflag bundle SNAPSHOT_C", "adaptiveflag bundle "+bundle, 1)
		if err = os.WriteFile(sp, []byte(mutated), 0600); err != nil {
			t.Fatal(err)
		}
		for _, mode := range modes {
			var out bytes.Buffer
			if err = runAdaptiveFlagReport(append(append([]string{}, base...), "--research-ablation="+string(mode)), &out); err == nil || out.Len() != 0 {
				t.Fatal("wrong bundle succeeded", bundle)
			}
		}
	}
}
