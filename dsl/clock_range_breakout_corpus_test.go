package dsl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/testsupport"
)

// Parse conformance inputs for the clock range breakout family. The programs
// are invented; each diagnostic case names the first error it must produce.
// Run with CRB_WRITE_CORPUS=1 to rewrite the inputs and headers; goldens are
// generated from them by `go run ./cmd/conformance regen`.

type clockParseCase struct {
	name        string
	category    string
	description string
	source      string
	// wantError is empty for an accepted program.
	wantError string
}

func clockParseCases() []clockParseCase {
	diagnostic := func(name, description, wantError string, mutate func(string, string) string) clockParseCase {
		return clockParseCase{name: "diagnostic-clock-range-breakout-" + name, category: "diagnostic", description: description, wantError: wantError, source: clockExample(mutate)}
	}
	replace := replaceInBlock
	reordered := clockProgram([]string{"strategy", "management", "execution", "risk", "setup", "market"}, replace("management", "{\n", "{\n  target 2R\n"))
	return []clockParseCase{
		{name: "setup-clock-range-breakout", category: "setup-family", description: "Clock range breakout: UTC+10 range 11:05-14:05, expiry and close 03:00, 1 percent stop (spec example).", source: clockExample(nil)},
		{name: "setup-clock-range-breakout-utc2", category: "setup-family", description: "Same instants as the UTC+10 example written on a UTC+2 clock; expiry defaults to the close.", source: clockExample(func(name, block string) string {
			switch name {
			case "market":
				return strings.Replace(block, "UTC+10", "UTC+2", 1)
			case "setup":
				return "setup {\n  type: clock range breakout\n  range 03:05 to 06:05\n  buffer 0 pips\n}\n"
			case "management":
				return strings.Replace(block, "03:00", "19:00", 1)
			}
			return block
		})},
		{name: "setup-clock-range-breakout-long-only-fractional", category: "setup-family", description: "Fractional negative offset, implicit symbol/timeframe route, fractional buffer and stop, long side only.", source: "dsl v7\nstrategy \"Clock Range Breakout Long Only\" {\n  description \"synthetic fractional offset example\"\n}\nmarket conditions {\n  symbols XAUUSD\n  timeframes 5m 1m\n  clock UTC-3:30\n}\nsetup {\n  type: clock range breakout\n  range 09:05 to 12:05\n  buffer 2.5 pips\n}\nfilters {\n  side long only\n}\nrisk {\n  stop 0.5 percent\n}\nmanagement {\n  close positions at 20:00\n}\nexecution {\n  risk: 150 USD\n}\n"},
		diagnostic("missing-required", "No clock, range, percent stop or clock close: each is reported.", "requires clock UTC", func(name, block string) string {
			switch name {
			case "market":
				return strings.Replace(block, "  clock UTC+10\n", "", 1)
			case "setup":
				return "setup {\n  type: clock range breakout\n}\n"
			case "risk", "management":
				return ""
			}
			return block
		}),
		diagnostic("offset-out-of-bounds", "UTC+14:01 is beyond the +14:00 limit.", "within UTC-12:00 and UTC+14:00", replace("market", "UTC+10", "UTC+14:01")),
		diagnostic("offset-minutes", "Offset minutes 60 are invalid.", "minutes must be 00-59", replace("market", "UTC+10", "UTC+2:60")),
		diagnostic("malformed-offset", "A space between UTC and the offset is not the clock spelling.", "clock must be written", replace("market", "UTC+10", "UTC +10")),
		diagnostic("range-start-after-end", "The range must start before it ends on the same clock date.", "range start must be before range end", replace("setup", "11:05 to 14:05", "14:05 to 11:05")),
		diagnostic("malformed-time", "24:00 is not a valid wall time.", `range time "24:00"`, replace("setup", "to 14:05", "to 24:00")),
		diagnostic("expiry-equals-range-end", "Expiry equal to the range end leaves no order window.", "must differ from range end", replace("setup", "orders expire 03:00", "orders expire 14:05")),
		diagnostic("expiry-after-close", "Expiry resolved after the close is invalid.", "must not be after close", replace("setup", "orders expire 03:00", "orders expire 10:00")),
		diagnostic("route-15m", "The UTC+10 schedule is not on the 15m UTC grid.", "route XAUUSD 15m", replace("market", "XAUUSD 5m", "XAUUSD 15m")),
		diagnostic("range-start-off-grid", "Range start 11:02 on UTC+10 is off the 5m UTC grid.", "range start 11:02", replace("setup", "range 11:05", "range 11:02")),
		diagnostic("range-end-off-grid", "Range end 14:02 on UTC+10 is off the 5m UTC grid.", "range end 14:02", replace("setup", "to 14:05", "to 14:02")),
		diagnostic("expiry-off-grid", "Expiry 02:58 on UTC+10 is off the 5m UTC grid.", "orders expire 02:58", replace("setup", "orders expire 03:00", "orders expire 02:58")),
		diagnostic("close-off-grid", "Close 03:02 on UTC+10 is off the 5m UTC grid.", "close positions 03:02", replace("management", "03:00", "03:02")),
		diagnostic("implicit-default-route", "Declaring a symbol without timeframes keeps the shared 5m/15m default; 15m is off grid.", "route XAUUSD 15m", replace("market", "slices(XAUUSD 5m)", "symbols XAUUSD")),
		diagnostic("unsupported-pip-route", "A symbol without a reviewed pip size fails closed even with a zero buffer.", "no reviewed pip size for FOOUSD", replace("market", "XAUUSD 5m", "FOOUSD 5m")),
		diagnostic("negative-buffer", "A buffer must be nonnegative.", "buffer must be nonnegative", replace("setup", "buffer 0 pips", "buffer -1 pips")),
		diagnostic("nan-buffer", "A buffer must be finite.", "buffer must be finite", replace("setup", "buffer 0 pips", "buffer NaN pips")),
		diagnostic("percent-zero", "A stop percent must be above zero.", "greater than 0 and at most 100", replace("risk", "stop 1 percent", "stop 0 percent")),
		diagnostic("percent-over-100", "A stop percent above 100 is invalid.", "greater than 0 and at most 100", replace("risk", "stop 1 percent", "stop 100.01 percent")),
		diagnostic("forbidden-sessions-before-sections", "A sessions line before any section is an error, never ignored.", "does not support directive \"sessions\"", func(name, block string) string {
			if name == "strategy" {
				return "sessions london\n" + block
			}
			return block
		}),
		{name: "diagnostic-clock-range-breakout-forbidden-target-reordered", category: "diagnostic", description: "A target in a management block placed before the setup is still an error.", wantError: "does not support directive \"target\"", source: reordered},
		diagnostic("forbidden-local-hour", "A local-hour gate is not part of the v1 surface.", "does not support directive \"local\"", replace("setup", "buffer 0 pips", "buffer 0 pips\n  local hour not in (18)")),
		diagnostic("forbidden-atr-stop", "Only a percent stop is supported.", "does not support stop", replace("risk", "stop 1 percent", "stop 2 ATR")),
		diagnostic("forbidden-neutral-hold-limit", "A neutral-looking maxHoldCandles value is still unsupported.", "does not support directive \"maxholdcandles\"", replace("management", "{\n", "{\n  maxHoldCandles 0\n")),
	}
}

func TestClockRangeBreakoutParseCorpusInputs(t *testing.T) {
	dir := filepath.Join(testsupport.StratConformanceRoot(), "parse")
	write := os.Getenv("CRB_WRITE_CORPUS") != ""
	for _, c := range clockParseCases() {
		result := parseClock(t, c.source)
		if c.wantError == "" && len(result.Errors) != 0 {
			t.Errorf("%s: unexpected errors %v", c.name, result.Errors)
		}
		if c.wantError != "" && !hasError(result, c.wantError) {
			t.Errorf("%s: errors %q lack %q", c.name, result.Errors, c.wantError)
		}
		header, err := json.MarshalIndent(map[string]string{
			"case": c.name, "category": c.category, "description": c.description,
			"source": "Invented clock range breakout fixture (HT-167); no market data.",
		}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		for suffix, want := range map[string][]byte{".strat": []byte(c.source), ".cfg.json": append(header, '\n')} {
			path := filepath.Join(dir, c.name+suffix)
			switch {
			case write && (suffix == ".strat" || !fileExists(path)):
				if err := os.WriteFile(path, want, 0o644); err != nil {
					t.Fatal(err)
				}
			case suffix == ".strat":
				if got, err := os.ReadFile(path); err != nil || string(got) != string(want) {
					t.Errorf("%s is missing or differs from the case table (rewrite with CRB_WRITE_CORPUS=1): %v", path, err)
				}
			}
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

var _ = fmt.Sprint
