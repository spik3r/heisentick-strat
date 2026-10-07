package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
)

func runAdaptiveFlagReport(args []string, out io.Writer) error {
	flags, err := parseTimedReportFlags(args)
	if err != nil {
		return err
	}
	allowed := map[string]bool{"dsl-file": true, "bars-file": true, "trade-from": true, "trade-to": true, "research-ablation": true}
	for key, values := range flags {
		if !allowed[key] || len(values) != 1 {
			return fmt.Errorf("adaptive-flag-report rejects unknown/repeated --%s", key)
		}
	}
	for _, key := range []string{"dsl-file", "bars-file"} {
		if _, err = flags.required(key); err != nil {
			return err
		}
	}
	ablation := engine.AdaptiveFlagResearchAblation(flags.one("research-ablation", ""))
	if _, present := flags["research-ablation"]; present && ablation == "" {
		return fmt.Errorf("adaptive --research-ablation requires a nonempty exact enum")
	}
	_, hasFrom := flags["trade-from"]
	_, hasTo := flags["trade-to"]
	if hasFrom != hasTo {
		return fmt.Errorf("adaptive evaluation window requires both --trade-from and --trade-to")
	}
	var window *engine.AdaptiveFlagExecutionWindow
	if hasFrom {
		limits := make([]int64, 2)
		for i, key := range []string{"trade-from", "trade-to"} {
			raw := flags.one(key, "")
			stamp, e := time.Parse(time.RFC3339, raw)
			if e != nil || stamp.Location() != time.UTC || stamp.Format(time.RFC3339) != raw {
				return fmt.Errorf("adaptive --%s requires exact UTC RFC3339 ending Z", key)
			}
			limits[i] = stamp.UnixMilli()
		}
		window = &engine.AdaptiveFlagExecutionWindow{TradeFromMS: limits[0], TradeToMS: limits[1]}
	}
	source, err := os.ReadFile(flags.one("dsl-file", ""))
	if err != nil {
		return err
	}
	prepared, err := adaptiveflag.PrepareLegacy(source, adaptiveflag.Options{Window: window, ResearchAblation: ablation})
	if err != nil {
		return err
	}
	data, err := os.ReadFile(flags.one("bars-file", ""))
	if err != nil {
		return err
	}
	raw, err := prepared.Build(data)
	if err != nil {
		return err
	}
	_, err = out.Write(raw)
	return err
}
