package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
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
	parsed, err := dsl.Parse(string(source))
	if err != nil {
		return err
	}
	if len(parsed.Errors) > 0 {
		return fmt.Errorf("adaptive flag DSL parse errors: %s", strings.Join(parsed.Errors, "; "))
	}
	if _, err = dsl.DecodeAdaptiveVolumeFlag(parsed.Config); err != nil {
		return err
	}
	data, err := os.ReadFile(flags.one("bars-file", ""))
	if err != nil {
		return err
	}
	if len(data) < 16 || binary.LittleEndian.Uint32(data[12:16]) != 6 {
		return fmt.Errorf("adaptive flag requires exactly six BTB1 OHLCV columns")
	}
	count := uint64(binary.LittleEndian.Uint32(data[8:12]))
	if uint64(len(data)) != 16+count*6*8 {
		return fmt.Errorf("adaptive flag BTB1 exact byte length mismatch")
	}
	series, err := marketdata.DecodeBTB1(data)
	if err != nil {
		return err
	}
	result, err := engine.RunAdaptiveVolumeFlag(engine.AdaptiveFlagRequest{Config: parsed.Config, Series: series, Window: window, ResearchAblation: ablation})
	if err != nil {
		return err
	}
	config, err := json.Marshal(parsed.Config)
	if err != nil {
		return err
	}
	hash := func(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
	envelope := struct {
		Schema       string                    `json:"schema"`
		DSLSHA256    string                    `json:"dslSha256"`
		ConfigSHA256 string                    `json:"configSha256"`
		BTB1SHA256   string                    `json:"btb1Sha256"`
		Config       dsl.Config                `json:"config"`
		Run          engine.AdaptiveFlagResult `json:"run"`
	}{"strat-adaptive-volume-flag-cli-v1", hash(source), hash(config), hash(data), parsed.Config, result}
	if result.ResearchPolicy != nil {
		envelope.Schema = engine.AdaptiveFlagResearchCLISchema
	}
	// Never emit partial success-looking JSON after validation or encoding failure.
	raw, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return err
	}
	_, err = out.Write(append(raw, '\n'))
	return err
}
