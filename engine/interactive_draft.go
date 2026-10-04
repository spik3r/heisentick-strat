package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const InteractiveSourceProfileSchema = "dsl-interactive-source-profile-v1"
const InteractiveDraftSchema = "dsl-interactive-draft-v1"
const InteractiveDraftStrategyID = "dslDraftStrategy"
const interactiveDraftMaxBars = 30_000

type InteractiveSourceProfile struct {
	SourceAssertions *InteractiveSourceAssertions `json:"sourceAssertions,omitempty"`
	Schema           string                       `json:"schema"`
	Provenance       InteractiveProvenance        `json:"provenance"`
	Parse            dsl.ParseResult              `json:"parse"`
	Profile          struct {
		Family            string `json:"family"`
		Symbol            string `json:"symbol"`
		Timeframe         string `json:"timeframe"`
		RangeMethod       string `json:"rangeMethod"`
		MaxBars           int    `json:"maxBars"`
		CalculationSource string `json:"calculationSource"`
	} `json:"profile"`
}

// draftObject refuses aliases, duplicate keys and trailing JSON before typed
// decoding can discard or overwrite a caller's requested meaning.
func draftObject(raw []byte, allowed map[string]bool) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, fmt.Errorf("draft request must be an object")
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok || !allowed[key] {
			return nil, fmt.Errorf("%w: draft field %v", ErrInteractiveUnsupported, token)
		}
		if _, exists := fields[key]; exists {
			return nil, fmt.Errorf("duplicate draft field %s", key)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("draft request has trailing JSON")
	}
	return fields, nil
}

func draftString(fields map[string]json.RawMessage, key string) (string, error) {
	var value string
	if json.Unmarshal(fields[key], &value) != nil || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("draft %s must be a nonempty string", key)
	}
	return value, nil
}

func draftCalculationSource(fields map[string]json.RawMessage) (string, error) {
	if _, exists := fields["calculationSource"]; !exists {
		return "raw", nil
	}
	value, err := draftString(fields, "calculationSource")
	if err != nil || (value != "raw" && value != "heikinAshi") {
		return "", fmt.Errorf("%w: draft calculationSource must be raw or heikinAshi", ErrInteractiveUnsupported)
	}
	return value, nil
}

func draftSourceProfile(source, symbol, timeframe string) (dsl.ParseResult, string, error) {
	var zero dsl.ParseResult
	if len(source) == 0 || len(source) > 256_000 {
		return zero, "", fmt.Errorf("draft source requires 1–256000 bytes")
	}
	if err := validateDraftPhysicalSections(source); err != nil {
		return zero, "", err
	}
	if _, ok := interactiveFixedDuration(timeframe); !ok {
		return zero, "", fmt.Errorf("%w: draft timeframe %q", ErrInteractiveUnsupported, timeframe)
	}
	parsed, err := dsl.ParseStrict(source)
	if err != nil {
		return zero, "", err
	}
	if len(parsed.Errors) != 0 {
		return zero, "", fmt.Errorf("DSL parse errors: %v", parsed.Errors)
	}
	if len(parsed.Warnings) != 0 {
		return zero, "", fmt.Errorf("%w: draft source has unresolved warnings: %v", ErrInteractiveUnsupported, parsed.Warnings)
	}
	cfg := parsed.Config
	if rawLengthNonempty(cfg["microstructureFilters"]) {
		return zero, "", fmt.Errorf("%w: draft microstructure filters are not implemented", ErrInteractiveUnsupported)
	}
	family := setupTypeFromAny(cfg["setupType"])
	if (family != string(dsl.FamilySMAGoldenCross) && family != string(dsl.FamilyDualEMAResumption) && family != string(dsl.FamilyFailedBreakout)) ||
		stringValue(mapValue(cfg, "htf"), "mode", "off") != "off" || sourceTimeframeFromConfig(cfg) != "" ||
		stringValue(cfg, "entryTf", "current") != "current" {
		return zero, "", fmt.Errorf("%w: draft v1 admits audited SMA, Dual EMA or fixed Failed Breakout chart-only syntax without HTF or source/entry timeframe", ErrInteractiveUnsupported)
	}
	if !compiledRouteAllowed(cfg, symbol, timeframe) {
		return zero, "", fmt.Errorf("%w: draft is off-route", ErrInteractiveUnsupported)
	}
	rangeMethod := stringValue(mapValue(cfg, "range"), "method", "pivot")
	if rangeMethod != "pivot" && rangeMethod != "zone" {
		return zero, "", fmt.Errorf("%w: draft range method %q", ErrInteractiveUnsupported, rangeMethod)
	}
	// Reuse the executor's full config admission before requesting market data.
	probe := marketdata.SeriesFromBars([]marketdata.Bar{{T: 0, O: 1, H: 1, L: 1, C: 1, V: 1}})
	if err := validateRunRequest(RunRequest{interactiveDraft: true, Config: cfg, Series: probe, StrategyID: InteractiveDraftStrategyID,
		Symbol: symbol, Timeframe: timeframe, RangeMethod: rangeMethod}); err != nil {
		return zero, "", fmt.Errorf("%w: %v", ErrInteractiveUnsupported, err)
	}
	return parsed, rangeMethod, nil
}

// The historical inline lexer may drop content after a closing section brace.
// Local draft admission must not discard authored intent. Inspect original
// physical lines, honoring quoted metadata and trailing comments; leave the
// historical parser and its corpus unchanged.
func validateDraftPhysicalSections(source string) error {
	depth := 0
	for lineIndex, line := range strings.Split(source, "\n") {
		var quote rune
		escaped, closed := false, false
		open, closeAt := -1, -1
		for i, char := range line {
			if quote != 0 {
				if escaped {
					escaped = false
				} else if char == '\\' {
					escaped = true
				} else if char == quote {
					quote = 0
				}
				continue
			}
			if char == '#' {
				break
			}
			if closed && !strings.ContainsRune(" \t\r", char) {
				return fmt.Errorf("%w: draft line %d has trailing section content", ErrInteractiveUnsupported, lineIndex+1)
			}
			if char == '"' || char == '\'' {
				quote = char
			} else if char == '{' {
				if depth != 0 {
					return fmt.Errorf("%w: draft line %d has nested sections", ErrInteractiveUnsupported, lineIndex+1)
				}
				depth = 1
				open = i
			} else if char == '}' {
				if depth != 1 {
					return fmt.Errorf("%w: draft line %d has unmatched closing brace", ErrInteractiveUnsupported, lineIndex+1)
				}
				depth = 0
				closed = true
				closeAt = i
			}
		}
		if quote != 0 {
			return fmt.Errorf("%w: draft line %d has an unclosed quote", ErrInteractiveUnsupported, lineIndex+1)
		}
		if open >= 0 && !dsl.InlineSourcePrefixPreserved(line[:open], "") {
			return fmt.Errorf("%w: draft line %d has an unsupported section header", ErrInteractiveUnsupported, lineIndex+1)
		}
		if open >= 0 && closeAt < 0 {
			body := strings.TrimSpace(line[open+1:])
			if body != "" && !strings.HasPrefix(body, "#") {
				return fmt.Errorf("%w: draft line %d has discarded opening-line content", ErrInteractiveUnsupported, lineIndex+1)
			}
		}
		if open >= 0 && closeAt > open {
			body := strings.TrimSpace(line[open+1 : closeAt])
			if body == "" {
				continue
			}
			// Share the actual historical splitter and its heads. A separate
			// approximate vocabulary would miss within-section prefix loss.
			if !dsl.InlineSourcePrefixPreserved(line[:open], body) {
				return fmt.Errorf("%w: draft line %d has discarded inline prefix", ErrInteractiveUnsupported, lineIndex+1)
			}
		}
	}
	if depth != 0 {
		return fmt.Errorf("%w: draft has an unclosed section", ErrInteractiveUnsupported)
	}
	return nil
}

func draftFamilyCalculation(parsed dsl.ParseResult, calculationSource string) error {
	if setupTypeFromAny(parsed.Config["setupType"]) == string(dsl.FamilyFailedBreakout) && calculationSource != "raw" {
		return fmt.Errorf("%w: fixed Failed Breakout draft requires raw calculation source", ErrInteractiveUnsupported)
	}
	return nil
}

// InspectInteractiveSource supplies Go-owned parsing and data requirements for
// mutable local DSL. It admits no catalog wrappers or implicit runtime options.
func InspectInteractiveSource(raw []byte, source string) (InteractiveSourceProfile, error) {
	var zero InteractiveSourceProfile
	fields, err := draftObject(raw, map[string]bool{"schema": true, "symbol": true, "timeframe": true, "calculationSource": true, "sourceAssertions": true})
	if err != nil {
		return zero, err
	}
	calculationSource, err := draftCalculationSource(fields)
	if err != nil {
		return zero, err
	}
	schema, err := draftString(fields, "schema")
	if err != nil {
		return zero, err
	}
	if schema != InteractiveSourceProfileSchema {
		return zero, fmt.Errorf("draft source profile schema is unsupported")
	}
	symbol, err := draftString(fields, "symbol")
	if err != nil {
		return zero, err
	}
	timeframe, err := draftString(fields, "timeframe")
	if err != nil {
		return zero, err
	}
	parsed, rangeMethod, err := draftSourceProfile(source, symbol, timeframe)
	if err != nil {
		return zero, err
	}
	if err := draftFamilyCalculation(parsed, calculationSource); err != nil {
		return zero, err
	}
	assertions, err := draftSourceAssertions(fields, parsed, symbol, timeframe)
	if err != nil {
		return zero, err
	}
	requestSum, sourceSum := sha256.Sum256(raw), sha256.Sum256([]byte(source))
	out := InteractiveSourceProfile{Schema: InteractiveSourceProfileSchema, Parse: parsed, SourceAssertions: assertions,
		Provenance: InteractiveProvenance{FixtureSHA256: hex.EncodeToString(requestSum[:]), SourceSHA256: hex.EncodeToString(sourceSum[:])}}
	out.Profile.Family = setupTypeFromAny(parsed.Config["setupType"])
	out.Profile.Symbol, out.Profile.Timeframe, out.Profile.RangeMethod = symbol, timeframe, rangeMethod
	out.Profile.MaxBars, out.Profile.CalculationSource = interactiveDraftMaxBars, calculationSource
	return out, nil
}

type preparedInteractiveDraft struct {
	fixture    RunFixture
	parsed     dsl.ParseResult
	assertions *InteractiveSourceAssertions
}

// Both finalized and prefix operations recheck the exact input in Go.
func prepareInteractiveDraftFixture(raw []byte, source string) (preparedInteractiveDraft, error) {
	var zero preparedInteractiveDraft
	fields, err := draftObject(raw, map[string]bool{"schema": true, "case": true, "strategyId": true,
		"symbol": true, "timeframe": true, "rangeMethod": true, "costs": true, "bars": true, "calculationSource": true, "sourceAssertions": true})
	if err != nil {
		return zero, err
	}
	calculationSource, err := draftCalculationSource(fields)
	if err != nil {
		return zero, err
	}
	for _, key := range []string{"schema", "case", "strategyId", "symbol", "timeframe", "rangeMethod"} {
		if _, err := draftString(fields, key); err != nil {
			return zero, err
		}
	}
	costs, err := draftObject(fields["costs"], map[string]bool{"fillOn": true, "feePerUnit": true, "slippage": true, "slippageBps": true, "startEquity": true})
	if err != nil {
		return zero, err
	}
	if len(costs) != 5 {
		return zero, fmt.Errorf("draft requires all five explicit costs")
	}
	if err := validateInteractiveInput(raw, true); err != nil {
		return zero, err
	}
	fixture, err := DecodeRunFixture(raw)
	if err != nil {
		return zero, err
	}
	if fixture.StrategyID != InteractiveDraftStrategyID {
		return zero, fmt.Errorf("%w: draft requires its reserved strategy identity", ErrInteractiveUnsupported)
	}
	parsed, rangeMethod, err := draftSourceProfile(source, fixture.Symbol, fixture.Timeframe)
	if err != nil {
		return zero, err
	}
	if err := draftFamilyCalculation(parsed, calculationSource); err != nil {
		return zero, err
	}
	assertions, err := draftSourceAssertions(fields, parsed, fixture.Symbol, fixture.Timeframe)
	if err != nil {
		return zero, err
	}
	if fixture.RangeMethod != rangeMethod || len(fixture.Bars) == 0 || len(fixture.Bars) > interactiveDraftMaxBars {
		return zero, fmt.Errorf("%w: draft requires its source range method and 1–30000 bars", ErrInteractiveUnsupported)
	}
	duration, _ := interactiveFixedDuration(fixture.Timeframe)
	adjacent := len(fixture.Bars) == 1
	for i, bar := range fixture.Bars {
		if i > 0 && bar.T-fixture.Bars[i-1].T == duration {
			adjacent = true
		}
		if bar.T < 0 || bar.T > 9007199254740991 || math.Trunc(bar.T) != bar.T || (i > 0 && bar.T <= fixture.Bars[i-1].T) ||
			math.Mod(bar.T, duration) != 0 ||
			bar.V < 0 || bar.H < math.Max(bar.O, bar.C) || bar.L > math.Min(bar.O, bar.C) || bar.H < bar.L {
			return zero, fmt.Errorf("draft bars[%d] has invalid ordered integer timestamps or OHLCV bounds", i)
		}
	}
	if !adjacent {
		return zero, fmt.Errorf("%w: draft cadence cannot be verified from only gapped bars", ErrInteractiveUnsupported)
	}
	return preparedInteractiveDraft{fixture: fixture, parsed: parsed, assertions: assertions}, nil
}

// RunInteractiveDraftFixture reparses exact source and rechecks capability at
// execution. No preflight token can authorize a different source or route.
func RunInteractiveDraftFixture(raw []byte, source string) (InteractiveRunResult, error) {
	var zero InteractiveRunResult
	prepared, err := prepareInteractiveDraftFixture(raw, source)
	if err != nil {
		return zero, err
	}
	result, err := runInteractiveFixture(raw, source, false, true)
	if err != nil {
		return zero, err
	}
	result.Schema = InteractiveDraftSchema
	result.SourceAssertions = prepared.assertions
	return result, nil
}
