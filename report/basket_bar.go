package report

import (
	"fmt"
	"strings"
)

// BasketBar has no policy defaults. Every field must be explicitly supplied
// when JSON callers ask for a verdict; omitting bar returns statistics only.
type BasketBar struct {
	Basket                            []string `json:"basket"`
	MinAggregateTrades                int      `json:"minAggregateTrades"`
	MinAggregatePFR                   float64  `json:"minAggregatePfR"`
	MinPerSymbolPFR                   float64  `json:"minPerSymbolPfR"`
	MinTradesPerYearPerSymbol         float64  `json:"minTradesPerYearPerSymbol"`
	MinAggregateHarshExpectancyR      float64  `json:"minAggregateHarshExpectancyR"`
	MinPerSymbolHarshPFR              float64  `json:"minPerSymbolHarshPfR"`
	RequireAggregateYearStability     bool     `json:"requireAggregateYearStability"`
	MaxTrailingNegativeYearsPerSymbol int      `json:"maxTrailingNegativeYearsPerSymbol"`
	RequireDSLSource                  bool     `json:"requireDslSource"`
}

var basketBarKeys = []string{"basket", "minAggregateTrades", "minAggregatePfR", "minPerSymbolPfR", "minTradesPerYearPerSymbol", "minAggregateHarshExpectancyR", "minPerSymbolHarshPfR", "requireAggregateYearStability", "maxTrailingNegativeYearsPerSymbol", "requireDslSource"}

type BasketComponent struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail"`
}
type BasketVerdict struct {
	Pass       bool              `json:"pass"`
	Components []BasketComponent `json:"components"`
	Reasons    []string          `json:"reasons"`
}

func validateBasketBar(bar BasketBar) error {
	if len(bar.Basket) > MaxBasketLegs || bar.MinAggregateTrades < 0 || bar.MaxTrailingNegativeYearsPerSymbol < 0 {
		return fmt.Errorf("invalid basket bar count")
	}
	for _, s := range bar.Basket {
		if strings.TrimSpace(s) == "" || len(s) > 128 {
			return fmt.Errorf("invalid basket bar symbol")
		}
	}
	for _, v := range []float64{bar.MinAggregatePFR, bar.MinPerSymbolPFR, bar.MinTradesPerYearPerSymbol, bar.MinPerSymbolHarshPFR} {
		if !basketFinite(v) || v < 0 {
			return fmt.Errorf("invalid basket bar threshold")
		}
	}
	if !basketFinite(bar.MinAggregateHarshExpectancyR) {
		return fmt.Errorf("invalid basket bar expectancy")
	}
	return nil
}

func basketPFAtLeast(value *float64, reason string, floor float64) bool {
	return reason == ProfitFactorNoLosses || value != nil && *value >= floor
}

func evaluateBasketBar(result BasketResult, bar BasketBar, source *bool) BasketVerdict {
	verdict := BasketVerdict{Pass: true, Components: []BasketComponent{}, Reasons: []string{}}
	add := func(name string, pass bool, detail string) {
		verdict.Components = append(verdict.Components, BasketComponent{name, pass, detail})
		if !pass {
			verdict.Pass = false
			verdict.Reasons = append(verdict.Reasons, name+": "+detail)
		}
	}
	required, measured, routes := map[string]bool{}, map[string]bool{}, map[string]int{}
	for _, s := range bar.Basket {
		required[strings.ToUpper(s)] = true
	}
	for _, s := range result.Symbols {
		measured[strings.ToUpper(s.Symbol)] = true
	}
	coverage := []string{}
	for _, s := range bar.Basket {
		if !measured[strings.ToUpper(s)] {
			coverage = append(coverage, "missing symbol "+s)
		}
	}
	for _, s := range result.Symbols {
		if !required[strings.ToUpper(s.Symbol)] {
			coverage = append(coverage, "unexpected symbol "+s.Symbol)
		}
	}
	for _, l := range result.Legs {
		key := strings.ToUpper(l.Symbol) + ":" + l.TF
		routes[key]++
		if routes[key] == 2 {
			coverage = append(coverage, "duplicate route "+key)
		}
	}
	detail := "all configured symbols measured"
	if len(coverage) > 0 {
		detail = strings.Join(coverage, "; ")
	}
	a := result.Aggregate
	add("basket-coverage", len(coverage) == 0, detail)
	add("risk-denominators", a.UnmeasurableTrades == 0, fmt.Sprintf("%d trades without a usable initial-risk denominator", a.UnmeasurableTrades))
	add("aggregate-sample", a.Trades >= bar.MinAggregateTrades, fmt.Sprintf("%d trades; require at least %d", a.Trades, bar.MinAggregateTrades))
	add("aggregate-profit-factor", basketPFAtLeast(a.PFR, a.PFRReason, bar.MinAggregatePFR), fmt.Sprintf("pooled PF(R) must be at least %g", bar.MinAggregatePFR))
	weak, thin, unknown, missing, harshWeak, streak := []string{}, []string{}, []string{}, []string{}, []string{}, []string{}
	for _, s := range result.Symbols {
		if !basketPFAtLeast(s.R.PFR, s.R.PFRReason, bar.MinPerSymbolPFR) {
			weak = append(weak, s.Symbol)
		}
		if s.TradesPerYear == nil {
			unknown = append(unknown, s.Symbol)
		} else if *s.TradesPerYear < bar.MinTradesPerYearPerSymbol {
			thin = append(thin, s.Symbol)
		}
		if s.MissingHarsh {
			missing = append(missing, s.Symbol)
		}
		if s.Harsh != nil && !basketPFAtLeast(s.Harsh.PFR, s.Harsh.PFRReason, bar.MinPerSymbolHarshPFR) {
			harshWeak = append(harshWeak, s.Symbol)
		}
		if s.RecentNegativeYearRun > bar.MaxTrailingNegativeYearsPerSymbol {
			streak = append(streak, s.Symbol)
		}
	}
	add("per-symbol-profit-factor", len(weak) == 0, fmt.Sprintf("PF(R) floor %g; below floor: %s", bar.MinPerSymbolPFR, strings.Join(weak, ", ")))
	add("frequency", len(thin) == 0 && len(unknown) == 0, fmt.Sprintf("frequency floor %g/year; below floor: %s; unknown: %s", bar.MinTradesPerYearPerSymbol, strings.Join(thin, ", "), strings.Join(unknown, ", ")))
	if a.Harsh != nil && len(missing) == 0 {
		add("harsh-costs", a.Harsh.ExpectancyR > bar.MinAggregateHarshExpectancyR, fmt.Sprintf("pooled harsh expectancy %gR must exceed %gR", a.Harsh.ExpectancyR, bar.MinAggregateHarshExpectancyR))
		add("per-symbol-harsh", len(harshWeak) == 0, fmt.Sprintf("harsh PF(R) floor %g; below floor: %s", bar.MinPerSymbolHarshPFR, strings.Join(harshWeak, ", ")))
	} else {
		detail := "harsh-cost evidence is missing or incomplete"
		if len(missing) > 0 {
			detail += " for " + strings.Join(missing, ", ")
		}
		add("harsh-costs", false, detail)
		add("per-symbol-harsh", false, detail)
	}
	if bar.RequireAggregateYearStability {
		add("year-stability", a.YearStable, fmt.Sprintf("negative complete years: %v", a.NegativeCompleteYears))
	}
	add("per-symbol-recent-years", len(streak) == 0, fmt.Sprintf("allowed trailing negative observed complete years %d (last two checked); exceeding: %s", bar.MaxTrailingNegativeYearsPerSymbol, strings.Join(streak, ", ")))
	if bar.RequireDSLSource {
		add("dsl-source", source != nil && *source, "caller must assert a DSL source is present")
	}
	return verdict
}
