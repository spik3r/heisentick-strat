package engine

import (
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
)

func TestTradeWindowUnrestrictedBypassesSessionWindows(t *testing.T) {
	parsed, err := dsl.Parse(`dsl v7
strategy "unrestricted" {
}
market conditions {
  trade window unrestricted
}
setup {
  type: opening range breakout
}
`)
	if err != nil {
		t.Fatalf("parse DSL: %v", err)
	}
	if len(parsed.Errors) != 0 {
		t.Fatalf("parse DSL diagnostics: %v", parsed.Errors)
	}

	params := paramsFromConfig(parsed.Config)
	if !params.TradeWindowUnrestricted {
		t.Fatal("unrestricted trade-window mode was not mapped into engine params")
	}

	// 19:00 UTC is 05:00 in the engine's UTC+10 session clock, outside every
	// normal setup and admission window.
	const outsideSessionWindows = float64(19 * 60 * 60 * 1000)
	if !inSetupTradeWindow(outsideSessionWindows, params, 0) {
		t.Fatal("setup window rejected unrestricted trade-window mode")
	}
	if !inFlagTradeWindow(outsideSessionWindows, params, 0) {
		t.Fatal("market window rejected unrestricted trade-window mode")
	}
	if !inAdmittedTradeWindow(outsideSessionWindows, params, 0) {
		t.Fatal("entry admission rejected unrestricted trade-window mode")
	}
}

func TestTradeWindowDefaultStillUsesSessionWindows(t *testing.T) {
	params := paramsFromConfig(dsl.Config{})
	const outsideSessionWindows = float64(19 * 60 * 60 * 1000)
	if inFlagTradeWindow(outsideSessionWindows, params, 0) {
		t.Fatal("default market window accepted an out-of-session timestamp")
	}
	if inAdmittedTradeWindow(outsideSessionWindows, params, 0) {
		t.Fatal("default entry admission accepted an out-of-session timestamp")
	}
}

func TestSetupMidWindowRequiresEnabledSession(t *testing.T) {
	// 03:00 UTC is 13:00 in the engine's UTC+10 session clock.
	const midSession = float64(3 * 60 * 60 * 1000)
	if inSetupTradeWindow(midSession, flagParams{}, 0) {
		t.Fatal("setup window accepted disabled mid session")
	}
	if !inSetupTradeWindow(midSession, flagParams{UseMidWindow: true}, 0) {
		t.Fatal("setup window rejected enabled mid session")
	}
	if !inSetupTradeWindow(midSession, flagParams{TradeWindowUnrestricted: true}, 0) {
		t.Fatal("setup window rejected unrestricted session")
	}
}
