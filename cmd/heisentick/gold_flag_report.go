package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func runGoldFlagReport(args []string, out io.Writer) error {
	flags, err := parseTimedReportFlags(args)
	if err != nil {
		return err
	}
	allowed := map[string]bool{"dsl-file": true, "m15-file": true, "from": true, "to": true, "cost": true}
	for key, values := range flags {
		if !allowed[key] || len(values) != 1 {
			return fmt.Errorf("gold-flag-report rejects unknown/repeated --%s", key)
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
	parsed, err := dsl.Parse(string(source))
	if err != nil {
		return err
	}
	if len(parsed.Errors) > 0 {
		return fmt.Errorf("gold flag DSL parse errors: %s", strings.Join(parsed.Errors, "; "))
	}
	if _, err = dsl.DecodeGoldFlagReference(parsed.Config); err != nil {
		return err
	}
	endpoints := make([]int64, 2)
	for i, key := range []string{"from", "to"} {
		raw := flags.one(key, "")
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil || t.Location() != time.UTC || t.Format(time.RFC3339) != raw {
			return fmt.Errorf("gold flag --%s requires exact UTC RFC3339 ending Z", key)
		}
		endpoints[i] = t.UnixMilli()
	}
	cost, err := strconv.ParseFloat(flags.one("cost", ""), 64)
	if err != nil {
		return fmt.Errorf("invalid gold flag cost")
	}
	data, err := os.ReadFile(flags.one("m15-file", ""))
	if err != nil {
		return err
	}
	// The ordinary decoder permits absent volume and extra bytes. This study
	// requires exactly six columns with no silently defaulted volume or suffix.
	if len(data) < 16 || binary.LittleEndian.Uint32(data[12:16]) != 6 {
		return fmt.Errorf("gold flag requires exactly six BTB1 OHLCV columns")
	}
	count := uint64(binary.LittleEndian.Uint32(data[8:12]))
	if uint64(len(data)) != 16+count*6*8 {
		return fmt.Errorf("gold flag BTB1 byte length mismatch")
	}
	series, err := marketdata.DecodeBTB1(data)
	if err != nil {
		return err
	}
	result, err := engine.RunGoldFlagReference(engine.GoldFlagReferenceRequest{Config: parsed.Config, M15: series, FromT: endpoints[0], ToT: endpoints[1], CostPerFill: cost})
	if err != nil {
		return err
	}
	cfg, err := json.Marshal(parsed.Config)
	if err != nil {
		return err
	}
	hash := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	envelope := struct {
		Schema     string                         `json:"schema"`
		DSLHash    string                         `json:"dslSha256"`
		ConfigHash string                         `json:"configSha256"`
		DataHash   string                         `json:"dataSha256"`
		Config     dsl.Config                     `json:"config"`
		Run        engine.GoldFlagReferenceResult `json:"run"`
	}{"strat-gold-flag-reference-cli-v1", hash(source), hash(cfg), hash(data), parsed.Config, result}
	// Marshal fully before writing so a validation/encoding failure produces
	// no partial success-looking JSON.
	raw, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	_, err = out.Write(raw)
	return err
}
