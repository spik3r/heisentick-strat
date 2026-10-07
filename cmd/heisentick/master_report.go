package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/spik3r/heisentick-strat/report/masterstructural"
)

func runMasterReport(args []string, out io.Writer) error {
	flags, err := parseTimedReportFlags(args)
	if err != nil {
		return err
	}
	allowed := map[string]bool{"dsl-file": true, "m5-file": true, "warmup-from": true, "trade-from": true, "trade-to": true, "spread": true}
	for key, values := range flags {
		if !allowed[key] || len(values) != 1 {
			return fmt.Errorf("master-report rejects unknown/repeated --%s", key)
		}
	}
	for key := range allowed {
		if _, err = flags.required(key); err != nil {
			return err
		}
	}
	source, err := os.ReadFile(flags.one("dsl-file", ""))
	if err != nil {
		return err
	}
	endpoints := make([]int64, 3)
	for i, key := range []string{"warmup-from", "trade-from", "trade-to"} {
		raw := flags.one(key, "")
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil || t.Location() != time.UTC || t.Format(time.RFC3339) != raw {
			return fmt.Errorf("master --%s requires exact UTC RFC3339 ending Z", key)
		}
		endpoints[i] = t.UnixMilli()
	}
	spread, err := strconv.ParseFloat(flags.one("spread", ""), 64)
	if err != nil {
		return fmt.Errorf("invalid master spread")
	}
	data, err := os.ReadFile(flags.one("m5-file", ""))
	if err != nil {
		return err
	}
	raw, err := masterstructural.Build(source, data, masterstructural.Options{WarmupFromT: endpoints[0], TradeFromT: endpoints[1], TradeToT: endpoints[2], Spread: spread})
	if err != nil {
		return err
	}
	_, err = out.Write(raw)
	return err
}
