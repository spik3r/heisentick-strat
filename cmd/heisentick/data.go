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
	"github.com/spik3r/heisentick-strat/report"
)

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

func loadRoute(flags flagSet, cfg dsl.Config) (report.Route, error) {
	symbol, err := flags.required("symbol")
	if err != nil {
		return report.Route{}, err
	}
	tf, err := flags.required("tf")
	if err != nil {
		return report.Route{}, err
	}
	rangeMethod := flags.one("range", "zone")
	if err := validateRangeMethod(rangeMethod); err != nil {
		return report.Route{}, err
	}
	root, err := dataRoot(flags)
	if err != nil {
		return report.Route{}, err
	}
	series, err := data.Load(root, symbol, tf)
	if err != nil {
		return report.Route{}, err
	}
	sourceTf := report.ResolveSourceTimeframe(tf, cfg)
	sourceSeries := series
	if sourceTf != tf {
		sourceSeries, err = data.Load(root, symbol, sourceTf)
		if err != nil {
			return report.Route{}, fmt.Errorf("load source timeframe %s for %s %s: %w", sourceTf, symbol, tf, err)
		}
	}
	htf := report.ResolveHigherTimeframe(sourceTf, cfg)
	var htfSeries marketdata.Series
	if htf != "" {
		htfSeries, err = data.Load(root, symbol, htf)
		if err != nil {
			return report.Route{}, fmt.Errorf("load higher timeframe %s for %s %s: %w", htf, symbol, tf, err)
		}
	}
	return report.Route{
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

func fileBaseName(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
