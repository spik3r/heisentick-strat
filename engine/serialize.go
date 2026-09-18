package engine

import (
	"fmt"
	"math"
	"sort"
	"strconv"
)

func checkedResultEnvelope(fixture RunFixture, trades []Trade) (RunResult, error) {
	fixture.Costs = fixture.Costs.normalized()
	if err := validateDerivedOutput(fixture.Costs, trades); err != nil {
		return RunResult{}, err
	}
	return resultEnvelope(fixture, trades), nil
}

func validateDerivedOutput(costs Costs, trades []Trade) error {
	for _, field := range []struct {
		name  string
		value float64
	}{
		{name: "feePerUnit", value: costs.FeePerUnit},
		{name: "slippage", value: costs.Slippage},
		{name: "startEquity", value: costs.StartEquity},
	} {
		if !isFiniteDerivedOutput(field.value) {
			return fmt.Errorf("run result costs.%s contains non-finite value", field.name)
		}
	}

	for i, trade := range trades {
		for _, field := range []struct {
			name  string
			value float64
		}{
			{name: "entry", value: trade.Entry},
			{name: "exit", value: trade.Exit},
			{name: "entryT", value: trade.EntryT},
			{name: "exitT", value: trade.ExitT},
			{name: "initialSl", value: trade.InitialSL},
			{name: "initialTp", value: trade.InitialTP},
			{name: "pnl", value: trade.PnL},
			{name: "points", value: trade.Points},
			{name: "size", value: trade.Size},
			{name: "sl", value: trade.SL},
			{name: "tp", value: trade.TP},
		} {
			if !isFiniteDerivedOutput(field.value) {
				return fmt.Errorf("run result trade %d %s contains non-finite value", i, field.name)
			}
		}

		floatKeys := make([]string, 0, len(trade.Meta))
		for key, value := range trade.Meta {
			if _, ok := value.(float64); ok {
				floatKeys = append(floatKeys, key)
			}
		}
		sort.Strings(floatKeys)
		for _, key := range floatKeys {
			if value := trade.Meta[key].(float64); !isFiniteDerivedOutput(value) {
				return fmt.Errorf("run result trade %d meta %q contains non-finite float64 value", i, key)
			}
		}
	}
	return nil
}

func isFiniteDerivedOutput(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func serializeTrades(trades []Trade) []Trade {
	out := make([]Trade, len(trades))
	for i, trade := range trades {
		out[i] = serializeTrade(trade)
	}
	return out
}

func serializeTrade(trade Trade) Trade {
	trade.Entry = numberForJSON(trade.Entry)
	trade.Exit = numberForJSON(trade.Exit)
	trade.EntryT = numberForJSON(trade.EntryT)
	trade.ExitT = numberForJSON(trade.ExitT)
	trade.InitialSL = numberForJSON(trade.InitialSL)
	trade.InitialTP = numberForJSON(trade.InitialTP)
	trade.PnL = numberForJSON(trade.PnL)
	trade.Points = numberForJSON(trade.Points)
	trade.Size = numberForJSON(trade.Size)
	trade.SL = numberForJSON(trade.SL)
	trade.TP = numberForJSON(trade.TP)
	if trade.Meta != nil {
		trade.Meta = serializeMeta(trade.Meta)
	}
	return trade
}

func serializeMeta(meta TradeMeta) TradeMeta {
	out := make(TradeMeta, len(meta))
	for key, value := range meta {
		switch v := value.(type) {
		case float64:
			out[key] = numberForJSON(v)
		default:
			out[key] = v
		}
	}
	return out
}

func numberForJSON(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if value == 0 {
		return 0
	}
	text := strconv.FormatFloat(value, 'g', 15, 64)
	rounded, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return value
	}
	if rounded == 0 {
		return 0
	}
	return rounded
}
