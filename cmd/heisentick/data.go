package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spik3r/heisentick-strat/data"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

type loadedRoute struct {
	Symbol          string
	TF              string
	SourceTimeframe string
	Range           string
	Series          marketdata.Series
	SourceSeries    marketdata.Series
	HigherTimeframe string
	HTFSeries       marketdata.Series
	SourceHTFSeries marketdata.Series
}

func loadDSLFile(path string) (dsl.ParseResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return dsl.ParseResult{}, fmt.Errorf("read DSL file %s: %w", path, err)
	}
	parsed, err := dsl.Parse(string(raw))
	if err != nil {
		return dsl.ParseResult{}, err
	}
	if len(parsed.Errors) > 0 {
		return dsl.ParseResult{}, fmt.Errorf("DSL parse errors: %s", strings.Join(parsed.Errors, "; "))
	}
	return parsed, nil
}

func loadRoute(flags flagSet, cfg dsl.Config) (loadedRoute, error) {
	symbol, err := flags.required("symbol")
	if err != nil {
		return loadedRoute{}, err
	}
	tf, err := flags.required("tf")
	if err != nil {
		return loadedRoute{}, err
	}
	rangeMethod := flags.one("range", "zone")
	if err := validateRangeMethod(rangeMethod); err != nil {
		return loadedRoute{}, err
	}
	root, err := dataRoot(flags)
	if err != nil {
		return loadedRoute{}, err
	}
	series, err := data.Load(root, symbol, tf)
	if err != nil {
		return loadedRoute{}, err
	}
	sourceTf := resolvedSourceTimeframe(tf, cfg)
	sourceSeries := series
	if sourceTf != tf {
		sourceSeries, err = data.Load(root, symbol, sourceTf)
		if err != nil {
			return loadedRoute{}, fmt.Errorf("load source timeframe %s for %s %s: %w", sourceTf, symbol, tf, err)
		}
	}
	htf := resolvedHigherTimeframe(sourceTf, cfg)
	var htfSeries marketdata.Series
	if htf != "" {
		htfSeries, err = data.Load(root, symbol, htf)
		if err != nil {
			return loadedRoute{}, fmt.Errorf("load higher timeframe %s for %s %s: %w", htf, symbol, tf, err)
		}
	}
	return loadedRoute{
		Symbol:          symbol,
		TF:              tf,
		SourceTimeframe: sourceTf,
		Range:           rangeMethod,
		Series:          series,
		SourceSeries:    sourceSeries,
		HigherTimeframe: htf,
		HTFSeries:       htfSeries,
		SourceHTFSeries: htfSeries,
	}, nil
}

func resolvedHigherTimeframe(tf string, cfg dsl.Config) string {
	htf, ok := cfg["htf"].(map[string]any)
	if !ok {
		return ""
	}
	if mode, _ := htf["mode"].(string); mode != "notAgainst" {
		return ""
	}
	requested, _ := htf["timeframe"].(string)
	return dsl.ResolveHigherTimeframe(tf, requested)
}

func resolvedSourceTimeframe(tf string, cfg dsl.Config) string {
	if source, _ := cfg["sourceTimeframe"].(string); source != "" {
		return source
	}
	return tf
}

func dataRoot(flags flagSet) (string, error) {
	if root := strings.TrimSpace(flags.one("data-root", "")); root != "" {
		return root, nil
	}
	repo, err := findRepoRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(repo, "data"), nil
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		next := filepath.Dir(dir)
		if next == dir {
			return "", errors.New("could not find repository root containing go.mod")
		}
		dir = next
	}
}

func strategyID(parsed dsl.ParseResult, dslFile string, requested ...string) string {
	requestedID := ""
	if len(requested) > 0 {
		requestedID = requested[0]
	}
	if requestedID = strings.TrimSpace(requestedID); requestedID != "" {
		return requestedID
	}
	if name, _ := parsed.Config["name"].(string); strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	base := filepath.Base(dslFile)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func strategyDisplayName(parsed dsl.ParseResult, fallback, requested string) string {
	if requested = strings.TrimSpace(requested); requested != "" {
		return requested
	}
	if name, _ := parsed.Config["name"].(string); strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return fallback
}
