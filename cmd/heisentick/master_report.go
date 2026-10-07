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
	"github.com/spik3r/heisentick-strat/engine/master"
	"github.com/spik3r/heisentick-strat/marketdata"
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
	parsed, err := dsl.Parse(string(source))
	if err != nil {
		return err
	}
	if len(parsed.Errors) > 0 {
		return fmt.Errorf("master DSL parse errors: %s", strings.Join(parsed.Errors, "; "))
	}
	if _, err = dsl.DecodeMasterStructural(parsed.Config); err != nil {
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
	// The ordinary decoder permits absent volume and extra bytes. This study
	// requires exactly six columns with no silently defaulted volume or suffix.
	if len(data) < 16 || binary.LittleEndian.Uint32(data[12:16]) != 6 {
		return fmt.Errorf("master requires exactly six BTB1 OHLCV columns")
	}
	count := uint64(binary.LittleEndian.Uint32(data[8:12]))
	if uint64(len(data)) != 16+count*6*8 {
		return fmt.Errorf("master BTB1 byte length mismatch")
	}
	series, err := marketdata.DecodeBTB1(data)
	if err != nil {
		return err
	}
	result, err := master.Run(master.Request{Config: parsed.Config, M5: series, WarmupFromT: endpoints[0], TradeFromT: endpoints[1], TradeToT: endpoints[2], Costs: master.Costs{Spread: spread, FeePerUnitSide: .5, InitialEquity: 10000}})
	if err != nil {
		return err
	}
	cfg, err := json.Marshal(parsed.Config)
	if err != nil {
		return err
	}
	hash := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	envelope := struct {
		Schema     string        `json:"schema"`
		DSLHash    string        `json:"dslSha256"`
		ConfigHash string        `json:"configSha256"`
		DataHash   string        `json:"dataSha256"`
		Config     dsl.Config    `json:"config"`
		Run        master.Result `json:"run"`
	}{"strat-master-structural-cli-v1", hash(source), hash(cfg), hash(data), parsed.Config, result}
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
