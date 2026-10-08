package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func runRangeReversionReport(args []string, out io.Writer) error {
	flags, err := parseTimedReportFlags(args)
	if err != nil {
		return err
	}
	allowed := map[string]bool{"dsl-file": true, "entry-bars-file": true, "source-bars-file": true, "daily-bars-file": true, "trade-from": true, "trade-to": true, "slippage-per-fill": true, "commission-per-unit-side": true, "units": true}
	for key, values := range flags {
		if !allowed[key] || len(values) != 1 {
			return fmt.Errorf("range-reversion-report rejects unknown or repeated --%s", key)
		}
	}
	for _, key := range []string{"dsl-file", "entry-bars-file", "source-bars-file", "trade-from", "trade-to", "slippage-per-fill", "commission-per-unit-side", "units"} {
		if _, err = flags.required(key); err != nil {
			return err
		}
	}
	parseUTC := func(key string) (int64, error) {
		raw := flags.one(key, "")
		stamp, e := time.Parse(time.RFC3339, raw)
		if e != nil || stamp.Location() != time.UTC || stamp.Format(time.RFC3339) != raw {
			return 0, fmt.Errorf("--%s requires exact UTC RFC3339 ending Z", key)
		}
		return stamp.UnixMilli(), nil
	}
	from, err := parseUTC("trade-from")
	if err != nil {
		return err
	}
	to, err := parseUTC("trade-to")
	if err != nil {
		return err
	}
	if from >= to {
		return fmt.Errorf("trade window requires trade-from < trade-to")
	}
	parseNonnegative := func(key string) (float64, error) {
		v, e := parseFloatFlag(key, flags.one(key, ""))
		if e != nil {
			return 0, e
		}
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, fmt.Errorf("--%s requires a finite nonnegative number", key)
		}
		return v, nil
	}
	slippage, err := parseNonnegative("slippage-per-fill")
	if err != nil {
		return err
	}
	commission, err := parseNonnegative("commission-per-unit-side")
	if err != nil {
		return err
	}
	units, err := parseFloatFlag("units", flags.one("units", ""))
	if err != nil || units <= 0 || math.IsInf(units, 0) || math.IsNaN(units) {
		return fmt.Errorf("--units requires a finite positive fixed quantity")
	}
	dslBytes, err := os.ReadFile(flags.one("dsl-file", ""))
	if err != nil {
		return err
	}
	parsed, err := dsl.Parse(string(dslBytes))
	if err != nil {
		return err
	}
	if len(parsed.Errors) > 0 {
		return fmt.Errorf("range reversion DSL errors: %s", strings.Join(parsed.Errors, "; "))
	}
	spec, err := dsl.DecodeRangeReversion(parsed.Config)
	if err != nil {
		return err
	}
	dailyPathValues, hasDailyPath := flags["daily-bars-file"]
	if spec.Rules.DailyCHOP != nil {
		if _, err = flags.required("daily-bars-file"); err != nil {
			return fmt.Errorf("daily-chop requires --daily-bars-file: %w", err)
		}
	} else if hasDailyPath {
		return fmt.Errorf("--daily-bars-file is only accepted when rangereversion daily-chop is configured")
	}
	readBars := func(path string) (marketdata.Series, string, error) {
		raw, e := os.ReadFile(path)
		if e != nil {
			return marketdata.Series{}, "", e
		}
		if len(raw) < 16 || binary.LittleEndian.Uint32(raw[12:16]) != 6 {
			return marketdata.Series{}, "", fmt.Errorf("%s requires six-column BTB1 OHLCV", path)
		}
		count := uint64(binary.LittleEndian.Uint32(raw[8:12]))
		if uint64(len(raw)) != 16+count*6*8 {
			return marketdata.Series{}, "", fmt.Errorf("%s BTB1 exact byte length mismatch", path)
		}
		series, e := marketdata.DecodeBTB1(raw)
		if e != nil {
			return marketdata.Series{}, "", e
		}
		sum := sha256.Sum256(raw)
		return series, hex.EncodeToString(sum[:]), nil
	}
	entry, entryHash, err := readBars(flags.one("entry-bars-file", ""))
	if err != nil {
		return err
	}
	source, sourceHash, err := readBars(flags.one("source-bars-file", ""))
	if err != nil {
		return err
	}
	daily := marketdata.Series{}
	dailyHash := ""
	if spec.Rules.DailyCHOP != nil {
		daily, dailyHash, err = readBars(dailyPathValues[0])
		if err != nil {
			return err
		}
	}
	run, err := engine.RunRangeReversion(engine.RangeReversionRequest{Config: parsed.Config, EntrySeries: entry, SourceSeries: source, DailySeries: daily, Window: engine.RangeReversionWindow{TradeFromMS: from, TradeToMS: to}, Execution: engine.RangeReversionExecution{SlippagePerFill: slippage, CommissionPerUnitSide: commission, Units: units}})
	if err != nil {
		return err
	}
	dslHash := sha256.Sum256(dslBytes)
	cfgBytes, err := json.Marshal(parsed.Config)
	if err != nil {
		return err
	}
	cfgHash := sha256.Sum256(cfgBytes)
	envelope := struct {
		Schema           string                      `json:"schema"`
		DSLSHA256        string                      `json:"dslSha256"`
		ConfigSHA256     string                      `json:"configSha256"`
		EntryBTB1SHA256  string                      `json:"entryBtb1Sha256"`
		SourceBTB1SHA256 string                      `json:"sourceBtb1Sha256"`
		DailyBTB1SHA256  string                      `json:"dailyBtb1Sha256,omitempty"`
		Config           dsl.Config                  `json:"config"`
		Run              engine.RangeReversionResult `json:"run"`
	}{"strat-range-reversion-report-v1", hex.EncodeToString(dslHash[:]), hex.EncodeToString(cfgHash[:]), entryHash, sourceHash, dailyHash, parsed.Config, run}
	raw, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return err
	}
	_, err = out.Write(append(raw, '\n'))
	return err
}
