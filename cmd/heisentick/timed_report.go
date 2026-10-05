package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/report"
)

func runTimedReport(args []string, out io.Writer) error {
	flags, err := parseTimedReportFlags(args)
	if err != nil {
		return err
	}
	allowed := map[string]bool{"dsl-file": true, "calendar-file": true, "symbol": true, "tf": true, "data-root": true, "slippage": true, "slippage-bps": true}
	for key, values := range flags {
		if !allowed[key] || len(values) != 1 {
			return fmt.Errorf("timed-report rejects unknown/repeated flag --%s", key)
		}
	}
	sourcePath, err := flags.required("dsl-file")
	if err != nil {
		return err
	}
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	parsed, err := dsl.Parse(string(source))
	if err != nil {
		return err
	}
	if len(parsed.Errors) != 0 {
		return fmt.Errorf("DSL parse errors: %s", strings.Join(parsed.Errors, "; "))
	}
	if parsed.Config["setupType"] != string(dsl.FamilyTimedReturn) {
		return fmt.Errorf("timed-report requires type: timed return")
	}
	if _, err := dsl.DecodeTimedReturn(parsed.Config); err != nil {
		return err
	}
	calendarPath, err := flags.required("calendar-file")
	if err != nil {
		return err
	}
	file, err := os.Open(calendarPath)
	if err != nil {
		return err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 8*1024*1024+1))
	if err != nil {
		return err
	}
	if len(raw) > 8*1024*1024 {
		return fmt.Errorf("timed calendar exceeds 8 MiB")
	}
	var calendar engine.TimedReturnCalendar
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&calendar); err != nil {
		return fmt.Errorf("timed calendar JSON: %w", err)
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return fmt.Errorf("timed calendar has trailing JSON")
	}
	slipRaw, err := flags.required("slippage")
	if err != nil {
		return err
	}
	slip, err := strconv.ParseFloat(slipRaw, 64)
	if err != nil {
		return fmt.Errorf("invalid timed slippage")
	}
	bps, err := strconv.ParseFloat(flags.one("slippage-bps", "0"), 64)
	if err != nil {
		return fmt.Errorf("invalid timed slippage-bps")
	}
	root, err := flags.required("data-root")
	if err != nil {
		return err
	}
	symbol, err := flags.required("symbol")
	if err != nil {
		return err
	}
	tf, err := flags.required("tf")
	if err != nil {
		return err
	}
	var routes []struct {
		Symbol string `json:"symbol"`
		TF     string `json:"tf"`
	}
	routeJSON, _ := json.Marshal(parsed.Config["slices"])
	_ = json.Unmarshal(routeJSON, &routes)
	if len(routes) != 1 || symbol != routes[0].Symbol || tf != routes[0].TF {
		return fmt.Errorf("timed CLI route must match its declared slice")
	}
	// Hash the exact byte snapshot decoded, including any ignored BTB columns.
	dataBytes, err := os.ReadFile(filepath.Join(root, symbol, tf+".bin"))
	if err != nil {
		return err
	}
	dataHash := sha256.Sum256(dataBytes)
	series, err := marketdata.DecodeBTB1(dataBytes)
	if err != nil {
		return err
	}
	result, err := engine.Run(engine.RunRequest{Config: parsed.Config, Series: series, Symbol: symbol, Timeframe: tf,
		StrategyID: report.StrategyID(parsed.Config, "timed-return", ""), TimedCalendar: &calendar,
		Costs: engine.Costs{FillOn: "nextOpen", Slippage: slip, SlippageBps: bps, FeePerUnit: 0, StartEquity: 10000}})
	if err != nil {
		return err
	}
	var net float64
	for _, trade := range result.Trades {
		net += trade.PnL
	}
	if math.IsInf(net, 0) || math.IsNaN(net) {
		return fmt.Errorf("timed native net overflow")
	}
	sourceHash := sha256.Sum256(source)
	calendarHash := sha256.Sum256(raw)
	return report.WriteJSON(out, struct {
		Schema           string           `json:"schema"`
		DSLHash          string           `json:"dslSha256"`
		CalendarFileHash string           `json:"calendarFileSha256"`
		DataHash         string           `json:"dataSha256"`
		Net              float64          `json:"netPriceUnits"`
		CostComplete     bool             `json:"costComplete"`
		Run              engine.RunResult `json:"run"`
	}{"strat-timed-return-report-v1", hex.EncodeToString(sourceHash[:]), hex.EncodeToString(calendarHash[:]), hex.EncodeToString(dataHash[:]), net, false, result})
}

// All timed-report flags carry explicit values. The generic CLI's boolean
// shorthand would silently turn a missing cost value into one price unit/bp.
func parseTimedReportFlags(args []string) (flagSet, error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			return nil, fmt.Errorf("unexpected timed-report positional argument %q", arg)
		}
		key, value, inline := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if key == "" {
			return nil, fmt.Errorf("empty timed-report flag name")
		}
		if !inline {
			if i+1 == len(args) || strings.HasPrefix(args[i+1], "--") {
				return nil, fmt.Errorf("timed-report --%s requires an explicit value", key)
			}
			i++
			value = args[i]
		}
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("timed-report --%s requires an explicit nonempty value", key)
		}
	}
	return parseFlags(args)
}
