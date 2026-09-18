package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"time"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func runReport(args []string, out io.Writer) error {
	flags, err := parseFlags(args)
	if err != nil {
		return err
	}
	if profilePath := flags.one("memprofile", ""); profilePath != "" {
		defer func() {
			runtime.GC()
			file, err := os.Create(profilePath)
			if err == nil {
				_ = pprof.WriteHeapProfile(file)
				_ = file.Close()
			}
		}()
	}
	dslFile, err := flags.required("dsl-file")
	if err != nil {
		return err
	}
	parsed, err := loadDSLFile(dslFile)
	if err != nil {
		return err
	}
	route, err := loadRoute(flags, parsed.Config)
	if err != nil {
		return err
	}
	modes, primary, err := costModes(route.Symbol, flags.one("slippage", ""))
	if err != nil {
		return err
	}
	id := strategyID(parsed, dslFile, flags.one("dsl-id", ""))
	displayName := strategyDisplayName(parsed, id, flags.one("dsl-name", ""))
	includeTrades := boolFlag(flags.one("include-trades", "0"))
	captureTradesAt := -1
	if includeTrades {
		captureTradesAt = primary.Index
	}
	slippageBps, err := parseSlippageBps(flags.one("slippage-bps", ""))
	if err != nil {
		return err
	}
	rows, trades, prepared, err := runCostRows(parsed.Config, route, id, modes, captureTradesAt, slippageBps)
	if err != nil {
		return err
	}
	var htf *string
	if route.HigherTimeframe != "" {
		htf = &route.HigherTimeframe
	}
	provenance := reportSourceEntryProvenance(parsed.Config, route)
	slice := sliceRow{
		Symbol:                      route.Symbol,
		TF:                          route.TF,
		Range:                       route.Range,
		Bars:                        route.Series.Len(),
		HTF:                         htf,
		sourceEntryReportProvenance: provenance,
		Costs:                       rows,
	}
	payload := reportPayload{
		Symbols:          []string{route.Symbol},
		TFs:              []string{route.TF},
		Strategy:         id,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		Range:            route.Range,
		Bars:             route.Series.Len(),
		PrimaryCost:      primary,
		Slices:           []sliceRow{slice},
		Costs:            rows,
		Warnings:         routeWarnings(parsed.Config, route),
		ActiveSessions:   activeSessions(parsed.Config),
		StrategyTypeTags: strategyTypeTags(parsed.Config),
	}
	if includeTrades {
		payload.TradeSchema = reportTradesSchema
		reportTrades, err := annotateReportTrades(trades, route.Series, prepared, reportTradeRoute{
			Symbol: route.Symbol,
			TF:     route.TF,
			Range:  route.Range,
		})
		if err != nil {
			return err
		}
		payload.Slices[0].Trades = &reportTrades
	}
	payload.EvidenceEnvelope = buildReportEvidenceEnvelope(payload, id, displayName, payload.GeneratedAt)
	if boolFlag(flags.one("json-only", "0")) {
		return writeJSON(out, payload)
	}
	printCostTable(out, payload.Costs)
	return writeJSON(out, payload)
}

// reportSourceEntryProvenance keeps report JSON explicit about the separately
// loaded source stream. It deliberately derives only presentation metadata;
// route validation and scheduling stay inside the existing DSL/engine paths.
func reportSourceEntryProvenance(cfg dsl.Config, route loadedRoute) sourceEntryReportProvenance {
	sourceTf, _ := cfg["sourceTimeframe"].(string)
	entryTf, _ := cfg["entryTf"].(string)
	if sourceTf == "" && (entryTf == "" || entryTf == "current") {
		return sourceEntryReportProvenance{}
	}
	provenance := sourceEntryReportProvenance{
		SourceTimeframe: &route.SourceTimeframe,
		EntryTimeframe:  &entryTf,
	}
	sourceBars := route.SourceSeries.Len()
	provenance.SourceBars = &sourceBars
	if route.HigherTimeframe != "" {
		provenance.SourceHTF = &route.HigherTimeframe
		sourceHtfBars := route.SourceHTFSeries.Len()
		provenance.SourceHTFBars = &sourceHtfBars
	}
	return provenance
}

func routeWarnings(cfg dsl.Config, route loadedRoute) []string {
	warnings := []string{}
	if !engine.RouteAllowed(cfg, route.Symbol, route.TF, route.Series) {
		warnings = append(warnings, fmt.Sprintf(
			"route %s %s is excluded by the strategy's slices()/symbols()/timeframes() market conditions; zero trades (matches JS gating)",
			route.Symbol, route.TF))
	}
	return warnings
}

func buildReportEvidenceEnvelope(payload reportPayload, strategy, displayName, generatedAt string) *reportEvidenceEnvelope {
	primary := payload.Costs[payload.PrimaryCost.Index]
	slice := payload.Slices[0]
	labels := make([]string, len(payload.Costs))
	for index, cost := range payload.Costs {
		labels[index] = cost.Label
	}
	return &reportEvidenceEnvelope{
		Schema:        "xauusd-backtester/evidence-envelope",
		SchemaVersion: 1,
		Artifact:      "strategy-report",
		Source:        "cli",
		GeneratedAt:   generatedAt,
		Strategy: reportEvidenceStrategy{
			ID:   strategy,
			Name: displayName,
		},
		Route: reportEvidenceRoute{
			Symbol:       slice.Symbol,
			TF:           slice.TF,
			Symbols:      append([]string(nil), payload.Symbols...),
			TFs:          append([]string(nil), payload.TFs...),
			RangeMethod:  slice.Range,
			RangeRequest: payload.Range,
			RouteMode:    "explicit",
			ForceRoute:   false,
		},
		Costs: reportEvidenceCosts{
			Label:            primary.Label,
			Slippage:         primary.Slippage,
			SlippageBps:      primary.SlippageBps,
			Commission:       0,
			PrimaryCostIndex: payload.PrimaryCost.Index,
			PrimaryCostLabel: payload.PrimaryCost.Label,
			CostLabels:       labels,
		},
		Summary: reportEvidenceSummaryFromCost(primary),
		Diagnostics: reportEvidenceDiagnostics{
			Warnings:         append([]string{}, payload.Warnings...),
			StrategyTypeTags: payload.StrategyTypeTags,
		},
		Aggregate: reportEvidenceAggregate{
			Kind:                  "strategy-report",
			RouteSliceCount:       1,
			ActiveRouteSliceCount: nonZeroTradeSliceCount(primary),
			Bars:                  payload.Bars,
			Symbols:               append([]string(nil), payload.Symbols...),
			TFs:                   append([]string(nil), payload.TFs...),
			RangeMethods:          []string{slice.Range},
			CostLabels:            labels,
			PrimaryCostIndex:      payload.PrimaryCost.Index,
			PrimaryCostLabel:      payload.PrimaryCost.Label,
		},
		Rows: []reportEvidenceRow{{
			Symbol:                      slice.Symbol,
			TF:                          slice.TF,
			RangeMethod:                 slice.Range,
			Bars:                        slice.Bars,
			HTF:                         slice.HTF,
			sourceEntryReportProvenance: slice.sourceEntryReportProvenance,
			Status:                      "ok",
			Costs: reportEvidenceRowCosts{
				Label:       primary.Label,
				Slippage:    primary.Slippage,
				SlippageBps: primary.SlippageBps,
				Commission:  0,
			},
			Summary: reportEvidenceSummaryFromCost(primary),
		}},
	}
}

func strategyTypeTags(cfg dsl.Config) reportStrategyTypeTags {
	return reportStrategyTypeTags{
		SessionBiasFilterCount: strategyTypeTagCount(cfg["sessionBiasFilters"]),
		SeasonalityFilterCount: strategyTypeTagCount(cfg["seasonalityFilters"]),
		VPAFilterCount:         strategyTypeTagCount(cfg["vp"]),
	}
}

func strategyTypeTagCount(value any) int {
	switch values := value.(type) {
	case []any:
		return len(values)
	case []map[string]any:
		return len(values)
	case []dsl.Config:
		return len(values)
	default:
		return 0
	}
}

func reportEvidenceSummaryFromCost(cost costRow) reportEvidenceSummary {
	return reportEvidenceSummary{
		Trades:         cost.Trades,
		WinRatePct:     cost.WinRate,
		ProfitFactor:   cost.PF,
		NetPnl:         cost.Net,
		ExpectancyPnl:  cost.Expectancy,
		MaxDrawdownPct: cost.DD,
	}
}

func nonZeroTradeSliceCount(cost costRow) int {
	if cost.Trades > 0 {
		return 1
	}
	return 0
}

func activeSessions(cfg dsl.Config) []string {
	var sessions map[string]any
	switch values := cfg["sessions"].(type) {
	case map[string]any:
		sessions = values
	case dsl.Config:
		sessions = map[string]any(values)
	}
	out := make([]string, 0, 3)
	for _, name := range []string{"asia", "london", "ny"} {
		if activeSessionEnabled(sessions[name]) {
			out = append(out, name)
		}
	}
	return out
}

func activeSessionEnabled(value any) bool {
	switch value := value.(type) {
	case bool:
		return value
	case int:
		return value != 0
	case int64:
		return value != 0
	case float64:
		return value != 0
	default:
		return false
	}
}

type reportTradeRoute struct {
	Symbol string
	TF     string
	Range  string
}

func annotateReportTrades(trades []engine.Trade, series marketdata.Series, prepared *engine.PreparedRun, route reportTradeRoute) ([]reportTrade, error) {
	out := make([]reportTrade, len(trades))
	for i, trade := range trades {
		phase := "other"
		if trade.EntryT >= 0 {
			phase = contextcols.SessionPhaseAtHour(contextcols.LocalHour(int64(trade.EntryT)))
		}
		localHour := 0
		if trade.EntryT >= 0 {
			localHour = int(contextcols.LocalHour(int64(trade.EntryT)))
		}
		weekday := "Sun"
		month := ""
		if trade.EntryT >= 0 {
			weekday = time.UnixMilli(int64(trade.EntryT)).UTC().Add(10 * time.Hour).Format("Mon")
			month = time.UnixMilli(int64(trade.EntryT)).UTC().Format("2006-01")
		}
		openLocation, priorDayType := "other", "none"
		if prepared != nil {
			if context, ok := prepared.ReportTradeContext(trade.EntryIndex); ok {
				openLocation = context.OpenLocation
				if context.PriorDayType != "" {
					priorDayType = context.PriorDayType
				}
			}
		}
		report := reportTrade{
			Trade:              trade,
			ReportSymbol:       route.Symbol,
			ReportTF:           route.TF,
			ReportRange:        route.Range,
			ReportSlice:        fmt.Sprintf("%s %s", route.Symbol, route.TF),
			ReportSessionPhase: phase,
			ReportOpenLocation: openLocation,
			ReportPriorDayType: priorDayType,
			ReportLocalHour:    localHour,
			ReportLocalWeekday: weekday,
			ReportEntryMonth:   month,
		}
		if excursion := reportTradeExcursion(trade, series); excursion != nil {
			if err := validateReportTradeExcursion(i, excursion); err != nil {
				return nil, err
			}
			report.ReportMfeR = &excursion.mfeR
			report.ReportMaeR = &excursion.maeR
			report.ReportTimeToMfeBars = &excursion.timeToMfeBars
			report.ReportPostExitMfeR = excursion.postExitMfeR
			report.ReportPostExitMaeR = excursion.postExitMaeR
		}
		out[i] = report
	}
	annotateReportPreviousOutcomes(out)
	return out, nil
}

// annotateReportPreviousOutcomes mirrors strategyReport's chronological
// sequence grouping. It runs only for the opt-in report trade payload and
// leaves engine trade execution and default report JSON untouched.
func annotateReportPreviousOutcomes(trades []reportTrade) {
	order := make([]int, len(trades))
	for i := range trades {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return trades[order[i]].EntryT < trades[order[j]].EntryT
	})
	for position, index := range order {
		outcome := "first trade"
		if position > 0 {
			if trades[order[position-1]].PnL > 0 {
				outcome = "after win"
			} else {
				outcome = "after loss"
			}
		}
		trades[index].ReportPreviousOutcome = outcome
	}
}

type tradeExcursionSummary struct {
	mfeR, maeR                 float64
	timeToMfeBars              int
	postExitMfeR, postExitMaeR *float64
}

func validateReportTradeExcursion(tradeIndex int, excursion *tradeExcursionSummary) error {
	for _, field := range []struct {
		name  string
		value *float64
	}{
		{name: "reportMfeR", value: &excursion.mfeR},
		{name: "reportMaeR", value: &excursion.maeR},
		{name: "reportPostExitMfeR", value: excursion.postExitMfeR},
		{name: "reportPostExitMaeR", value: excursion.postExitMaeR},
	} {
		if field.value != nil && (math.IsNaN(*field.value) || math.IsInf(*field.value, 0)) {
			return fmt.Errorf("report trade %d %s contains non-finite value", tradeIndex, field.name)
		}
	}
	return nil
}

func reportTradeExcursion(trade engine.Trade, series marketdata.Series) *tradeExcursionSummary {
	risk := math.Abs(trade.Entry - trade.InitialSL)
	if risk <= 0 || trade.EntryIndex < 0 || trade.ExitIndex < trade.EntryIndex || trade.EntryIndex >= series.Len() {
		return nil
	}
	end := trade.ExitIndex
	if end >= series.Len() {
		end = series.Len() - 1
	}
	best, worst, bestAt := math.Inf(-1), math.Inf(-1), 0
	for index := trade.EntryIndex; index <= end; index++ {
		bar := series.Bar(index)
		favorable, adverse := bar.H-trade.Entry, trade.Entry-bar.L
		if trade.Side == "short" {
			favorable, adverse = trade.Entry-bar.L, bar.H-trade.Entry
		}
		if favorable > best {
			best, bestAt = favorable, index-trade.EntryIndex
		}
		if adverse > worst {
			worst = adverse
		}
	}
	summary := &tradeExcursionSummary{mfeR: best / risk, maeR: worst / risk, timeToMfeBars: bestAt}
	postStart, postEnd := trade.ExitIndex+1, trade.ExitIndex+12
	last := series.Len() - 1
	if postStart > last {
		postStart = last
	}
	if postEnd > last {
		postEnd = last
	}
	if postEnd >= postStart {
		postBest, postWorst := 0.0, 0.0
		for index := postStart; index <= postEnd; index++ {
			bar := series.Bar(index)
			favorable, adverse := bar.H-trade.Exit, trade.Exit-bar.L
			if trade.Side == "short" {
				favorable, adverse = trade.Exit-bar.L, bar.H-trade.Exit
			}
			postBest = math.Max(postBest, favorable)
			postWorst = math.Max(postWorst, adverse)
		}
		mfe, mae := postBest/risk, postWorst/risk
		summary.postExitMfeR, summary.postExitMaeR = &mfe, &mae
	}
	return summary
}

func writeJSON(out io.Writer, payload any) error {
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", encoded)
	return err
}

func printCostTable(out io.Writer, rows []costRow) {
	fmt.Fprintln(out, "label,trades,winRate,pf,net,expectancy,dd")
	for _, row := range rows {
		fmt.Fprintf(out, "%s,%d,%.2f,%s,%.2f,%.2f,%.2f\n", row.Label, row.Trades, row.WinRate, formatPF(row.PF), row.Net, row.Expectancy, row.DD)
	}
}

func formatPF(value *float64) string {
	if value == nil {
		return "inf"
	}
	return strconvFormatFloat(*value, 'f', 4)
}
