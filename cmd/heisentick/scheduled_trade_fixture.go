//go:build !js || !wasm

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
)

func init() {
	nativeExtraCommands["scheduled-trade-fixture"] = runScheduledTradeFixture
	nativeExtraUsage += `

  heisentick scheduled-trade-fixture --fixture-path=<fixture.json> --strategy-path=<strategy.strat>

scheduled-trade-fixture executes one supplied OHLCV fixture through the native scheduled-trade family and prints its run/audit JSON. It validates six-value bar rows and uses the engine's embedded, bounded Japan settlement calendar.`
}

func runScheduledTradeFixture(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("scheduled-trade-fixture", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	fixturePath := flags.String("fixture-path", "", "run fixture JSON")
	strategyPath := flags.String("strategy-path", "", "DSL strategy source")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *fixturePath == "" || *strategyPath == "" {
		return fmt.Errorf("usage: heisentick scheduled-trade-fixture --fixture-path=<fixture.json> --strategy-path=<strategy.strat>")
	}
	fixture, err := engine.LoadRunFixture(*fixturePath)
	if err != nil {
		return err
	}
	source, err := os.ReadFile(*strategyPath)
	if err != nil {
		return err
	}
	parsed, err := dsl.Parse(string(source))
	if err != nil {
		return err
	}
	if len(parsed.Errors) != 0 {
		return fmt.Errorf("DSL parse errors: %v", parsed.Errors)
	}
	if parsed.Config["setupType"] != string(dsl.FamilyScheduledTrade) {
		return fmt.Errorf("scheduled-trade-fixture requires the scheduled trade family")
	}
	result, err := engine.RunFixtureCase(fixture, string(source))
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(encoded))
	return err
}
