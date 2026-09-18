package main

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
)

type gridVariantSpec struct {
	index  int
	params map[string]float64
	cfg    dsl.Config
	key    string
}

func runGrid(args []string, out io.Writer) error {
	flags, err := parseFlags(args)
	if err != nil {
		return err
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
	sets, err := parseSets(flags.all("set"))
	if err != nil {
		return err
	}
	if len(sets) == 0 {
		return fmt.Errorf("missing --set <param>=<v1,v2,...>")
	}
	modes, _, err := costModes(route.Symbol, flags.one("slippage", ""))
	if err != nil {
		return err
	}
	id := strategyID(parsed, dslFile)
	variants := expandVariants(sets)
	specs, groups, groupOrder, err := buildGridVariantSpecs(parsed.Config, route, id, variants)
	if err != nil {
		return err
	}
	sharedContexts := make(map[string]*engine.SharedRunContext, len(groups))
	for _, key := range groupOrder {
		spec := specs[groups[key][0]]
		shared, err := engine.PrepareSharedRunContext(engine.RunRequest{
			Config:          spec.cfg,
			Series:          route.Series,
			SourceSeries:    route.SourceSeries,
			HTFSeries:       route.HTFSeries,
			SourceHTFSeries: route.SourceHTFSeries,
			StrategyID:      id,
			Symbol:          route.Symbol,
			Timeframe:       route.TF,
			SourceTimeframe: route.SourceTimeframe,
			HigherTimeframe: route.HigherTimeframe,
			RangeMethod:     route.Range,
			Costs:           engine.Costs{FillOn: "close", StartEquity: 10000},
		})
		if err != nil {
			return err
		}
		sharedContexts[key] = shared
	}
	outVariants := make([]gridVariant, len(variants))
	variantErrors := make([]error, len(variants))
	var wg sync.WaitGroup
	workerLimit := runtime.GOMAXPROCS(0)
	if workerLimit < 1 {
		workerLimit = 1
	}
	if workerLimit > len(specs) {
		workerLimit = len(specs)
	}
	workers := make(chan struct{}, workerLimit)
	for _, spec := range specs {
		spec := spec
		wg.Add(1)
		workers <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-workers }()
			rows, err := runCostRowsFromShared(sharedContexts[spec.key], spec.cfg, modes)
			if err != nil {
				variantErrors[spec.index] = fmt.Errorf("variant %d: %w", spec.index, err)
				return
			}
			outVariants[spec.index] = gridVariant{Index: spec.index, Params: spec.params, Costs: rows}
		}()
	}
	wg.Wait()
	if err := firstGridVariantError(variantErrors); err != nil {
		return err
	}
	var htf *string
	if route.HigherTimeframe != "" {
		htf = &route.HigherTimeframe
	}
	payload := gridPayload{
		Symbols:  []string{route.Symbol},
		TFs:      []string{route.TF},
		Strategy: id,
		Range:    route.Range,
		Bars:     route.Series.Len(),
		HTF:      htf,
		Sets:     sets,
		Variants: outVariants,
		Warnings: routeWarnings(parsed.Config, route),
	}
	if boolFlag(flags.one("json-only", "0")) {
		return writeJSON(out, payload)
	}
	for _, variant := range payload.Variants {
		fmt.Fprintf(out, "variant %d %v\n", variant.Index, variant.Params)
		printCostTable(out, variant.Costs)
	}
	return writeJSON(out, payload)
}

func firstGridVariantError(variantErrors []error) error {
	for _, err := range variantErrors {
		if err != nil {
			return err
		}
	}
	return nil
}

func buildGridVariantSpecs(base dsl.Config, route loadedRoute, strategy string, variants []map[string]float64) ([]gridVariantSpec, map[string][]int, []string, error) {
	specs := make([]gridVariantSpec, len(variants))
	groups := make(map[string][]int)
	groupOrder := make([]string, 0, len(variants))
	for i, params := range variants {
		cfg, err := configWithParams(base, params)
		if err != nil {
			return nil, nil, nil, err
		}
		key, err := engine.SharedContextKey(engine.RunRequest{
			Config:          cfg,
			Series:          route.Series,
			SourceSeries:    route.SourceSeries,
			HTFSeries:       route.HTFSeries,
			SourceHTFSeries: route.SourceHTFSeries,
			StrategyID:      strategy,
			Symbol:          route.Symbol,
			Timeframe:       route.TF,
			SourceTimeframe: route.SourceTimeframe,
			HigherTimeframe: route.HigherTimeframe,
			RangeMethod:     route.Range,
			Costs:           engine.Costs{FillOn: "close", StartEquity: 10000},
		})
		if err != nil {
			return nil, nil, nil, fmt.Errorf("variant %d: %w", i, err)
		}
		if _, ok := groups[key]; !ok {
			groupOrder = append(groupOrder, key)
		}
		specs[i] = gridVariantSpec{index: i, params: params, cfg: cfg, key: key}
		groups[key] = append(groups[key], i)
	}
	return specs, groups, groupOrder, nil
}

func configWithParams(base dsl.Config, params map[string]float64) (dsl.Config, error) {
	cfg, err := cloneConfig(base)
	if err != nil {
		return nil, err
	}
	for key, value := range params {
		cfg[key] = value
	}
	return cfg, nil
}

func parseSets(raw []string) (map[string][]float64, error) {
	sets := make(map[string][]float64)
	for _, item := range raw {
		key, valueList, ok := strings.Cut(item, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --set %q: expected <param>=<v1,v2,...>", item)
		}
		if strings.Contains(key, ".") {
			return nil, fmt.Errorf("invalid --set %q: dotted params are not supported", item)
		}
		parts := strings.Split(valueList, ",")
		values := make([]float64, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			value, err := parseFloatFlag("set", part)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		if len(values) == 0 {
			return nil, fmt.Errorf("invalid --set %q: no values", item)
		}
		sets[key] = values
	}
	return sets, nil
}

func expandVariants(sets map[string][]float64) []map[string]float64 {
	keys := make([]string, 0, len(sets))
	for key := range sets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out []map[string]float64
	var walk func(int, map[string]float64)
	walk = func(index int, params map[string]float64) {
		if index == len(keys) {
			row := make(map[string]float64, len(params))
			for key, value := range params {
				row[key] = value
			}
			out = append(out, row)
			return
		}
		key := keys[index]
		for _, value := range sets[key] {
			params[key] = value
			walk(index+1, params)
		}
		delete(params, key)
	}
	walk(0, make(map[string]float64, len(keys)))
	return out
}

func cloneConfig(cfg dsl.Config) (dsl.Config, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var out dsl.Config
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
