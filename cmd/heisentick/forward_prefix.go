package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/report"
)

func runForwardPrefix(args []string, out io.Writer) error {
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
	slippage, err := parseFloatFlag("slippage", flags.one("slippage", "0"))
	if err != nil {
		return err
	}
	slippageBps, err := report.ParseSlippageBps(flags.one("slippage-bps", "0"))
	if err != nil {
		return err
	}
	strategyID := report.StrategyID(parsed.Config, fileBaseName(dslFile), flags.one("dsl-id", ""))
	request := engine.RunRequest{
		Config: parsed.Config, Series: route.Series, SourceSeries: route.SourceSeries,
		HTFSeries: route.HTFSeries, SourceHTFSeries: route.SourceHTFSeries,
		StrategyID: strategyID, Symbol: route.Symbol, Timeframe: route.TF,
		SourceTimeframe: route.SourceTimeframe, HigherTimeframe: route.HigherTimeframe,
		RangeMethod: route.Range,
		Costs:       engine.Costs{FillOn: "close", Slippage: slippage, SlippageBps: *slippageBps, StartEquity: 10_000},
	}
	checkpointIn := flags.one("checkpoint-in", "")
	checkpointOut := flags.one("checkpoint-out", "")
	if checkpointIn == "" && checkpointOut == "" {
		result, err := engine.RunPrefix(request)
		if err != nil {
			return err
		}
		return report.WriteJSON(out, result)
	}
	if checkpointOut == "" {
		return fmt.Errorf("--checkpoint-in requires --checkpoint-out")
	}
	if checkpointIn != "" {
		inPath, err := filepath.Abs(checkpointIn)
		if err != nil {
			return fmt.Errorf("resolve --checkpoint-in: %w", err)
		}
		outPath, err := filepath.Abs(checkpointOut)
		if err != nil {
			return fmt.Errorf("resolve --checkpoint-out: %w", err)
		}
		if inPath == outPath {
			return fmt.Errorf("--checkpoint-in and --checkpoint-out must name different files")
		}
	}
	if _, err := os.Lstat(checkpointOut); err == nil {
		return fmt.Errorf("--checkpoint-out already exists: %s", checkpointOut)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect --checkpoint-out: %w", err)
	}
	var checkpoint []byte
	if checkpointIn != "" {
		checkpoint, err = os.ReadFile(checkpointIn)
		if err != nil {
			return fmt.Errorf("read --checkpoint-in: %w", err)
		}
	}
	checkpointRequest := request
	if checkpointRequest.SourceTimeframe == checkpointRequest.Timeframe {
		// loadRoute aliases ordinary chart/HTF inputs into the source fields for
		// report compatibility. The resumable API accepts the canonical ordinary
		// route; distinct source-entry routes remain typed unsupported.
		checkpointRequest.SourceSeries = marketdata.Series{}
		checkpointRequest.SourceHTFSeries = marketdata.Series{}
	}
	result, nextCheckpoint, err := engine.RunPrefixResumable(checkpointRequest, checkpoint)
	if err != nil {
		return err
	}
	var encoded bytes.Buffer
	if err := report.WriteJSON(&encoded, result); err != nil {
		return err
	}
	if err := writeNewCheckpoint(checkpointOut, nextCheckpoint); err != nil {
		return err
	}
	_, err = io.Copy(out, &encoded)
	return err
}

func writeNewCheckpoint(path string, checkpoint []byte) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".heisentick-checkpoint-*")
	if err != nil {
		return fmt.Errorf("create checkpoint candidate: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(checkpoint); err != nil {
		temp.Close()
		return fmt.Errorf("write checkpoint candidate: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync checkpoint candidate: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close checkpoint candidate: %w", err)
	}
	// Link publishes a new candidate atomically and fails if the destination
	// appeared after the preflight check; it never replaces a prior checkpoint.
	if err := os.Link(tempPath, path); err != nil {
		return fmt.Errorf("publish new checkpoint candidate: %w", err)
	}
	directory, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open checkpoint directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync checkpoint directory: %w", err)
	}
	return nil
}
