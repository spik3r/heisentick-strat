package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/spik3r/heisentick-strat/dsl"
	native "github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const (
	r11FixtureSchema = "strat-r11-fixture-v1"
	r11ResultSchema  = "strat-r11-wasm-result-v1"
	r11MaxDSLBytes   = 8 << 20
	r11MaxFixture    = 64 << 20
	r11MaxOutput     = 64 << 20
	r11MaxEntryBars  = 500_000
	r11MaxSourceBars = 100_000
	r11MaxSignals    = 500_000
	r11MaxTrades     = 250_000
	r11MaxJSONTokens = 5_000_000
)

const (
	r11Target3M30SHA256 = "c99f0695cc47fb263a93e4d9bc58970d089cb62e23ffb714482368318cb9ef5a"
	r11Chop45H1SHA256   = "5b0572545543910ea69abc04037d9b5675baca69be44a8ad034dea6efe7c9fd8"
)

type r11Execution struct {
	SlippagePerFill       float64 `json:"slippagePerFill"`
	CommissionPerUnitSide float64 `json:"commissionPerUnitSide"`
	Units                 float64 `json:"units"`
}

type r11Fixture struct {
	Schema          string       `json:"schema"`
	ContractVersion int          `json:"contractVersion"`
	StrategyID      string       `json:"strategyId"`
	Symbol          string       `json:"symbol"`
	Timeframe       string       `json:"timeframe"`
	SourceTimeframe string       `json:"sourceTimeframe"`
	TradeFromMS     int64        `json:"tradeFromMs"`
	TradeToMS       int64        `json:"tradeToMs"`
	Execution       r11Execution `json:"execution"`
	EntryBars       [][]float64  `json:"entryBars"`
	SourceBars      [][]float64  `json:"sourceBars"`
}

type r11Identity struct {
	FixtureSHA256   string                 `json:"fixtureSha256"`
	DSLSHA256       string                 `json:"dslSha256"`
	StrategyID      string                 `json:"strategyId"`
	Symbol          string                 `json:"symbol"`
	Timeframe       string                 `json:"timeframe"`
	SourceTimeframe string                 `json:"sourceTimeframe"`
	TradeFromMS     int64                  `json:"tradeFromMs"`
	TradeToMS       int64                  `json:"tradeToMs"`
	Execution       r11Execution           `json:"execution"`
	EffectiveConfig dsl.RangeReversionSpec `json:"effectiveConfig"`
}

type r11Envelope struct {
	Schema          string                       `json:"schema"`
	ContractVersion int                          `json:"contractVersion"`
	Identity        *r11Identity                 `json:"identity,omitempty"`
	Run             *native.RangeReversionResult `json:"run,omitempty"`
	Error           *r11Error                    `json:"error,omitempty"`
}

type r11Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type r11LimitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *r11LimitedBuffer) Write(value []byte) (int, error) {
	if len(value) > b.limit-b.Len() {
		return 0, errors.New("serialized output exceeds 64 MiB")
	}
	return b.Buffer.Write(value)
}

func r11Failure(code string, err error) []byte {
	message := "R11 fixture rejected"
	if err != nil {
		message = err.Error()
	}
	raw, _ := json.Marshal(r11Envelope{Schema: r11ResultSchema, ContractVersion: 1, Error: &r11Error{Code: code, Message: message}})
	return raw
}

// runR11Fixture is a closed browser execution route for the two source-pinned
// R11 research candidates. It deliberately bypasses the generic fixture path.
func runR11Fixture(rawFixture, source string) []byte {
	if len(rawFixture) > r11MaxFixture {
		return r11Failure("R11_FIXTURE_INVALID", errors.New("fixture JSON exceeds 64 MiB"))
	}
	if len(source) == 0 || len(source) > r11MaxDSLBytes {
		return r11Failure("R11_DSL_INVALID", errors.New("DSL must contain 1..8 MiB"))
	}
	if err := validateR11FixtureJSON(rawFixture); err != nil {
		return r11Failure("R11_FIXTURE_INVALID", err)
	}
	var fixture r11Fixture
	decoder := json.NewDecoder(strings.NewReader(rawFixture))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		return r11Failure("R11_FIXTURE_INVALID", fmt.Errorf("decode fixture: %w", err))
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return r11Failure("R11_FIXTURE_INVALID", err)
	}
	if err := validateR11Fixture(fixture); err != nil {
		return r11Failure("R11_FIXTURE_INVALID", err)
	}

	dslDigest := sha256.Sum256([]byte(source))
	dslHash := hex.EncodeToString(dslDigest[:])
	if want := r11DSLHash(fixture.StrategyID); want == "" || dslHash != want {
		return r11Failure("R11_IDENTITY_MISMATCH", errors.New("DSL bytes do not match the pinned strategy source for strategyId"))
	}
	parsed, err := dsl.Parse(source)
	if err != nil {
		return r11Failure("R11_DSL_INVALID", err)
	}
	if len(parsed.Errors) != 0 {
		return r11Failure("R11_DSL_INVALID", fmt.Errorf("DSL parse errors: %v", parsed.Errors))
	}
	spec, err := dsl.DecodeRangeReversion(parsed.Config)
	if err != nil {
		return r11Failure("R11_DSL_INVALID", err)
	}
	if err := validateR11Strategy(fixture, spec); err != nil {
		return r11Failure("R11_IDENTITY_MISMATCH", err)
	}

	entry, err := r11RowsToSeries(fixture.EntryBars, fixture.Timeframe)
	if err != nil {
		return r11Failure("R11_FIXTURE_INVALID", fmt.Errorf("entryBars: %w", err))
	}
	sourceBars, err := r11RowsToSeries(fixture.SourceBars, "4h")
	if err != nil {
		return r11Failure("R11_FIXTURE_INVALID", fmt.Errorf("sourceBars: %w", err))
	}
	request := native.RangeReversionRequest{
		Config:       parsed.Config,
		EntrySeries:  entry,
		SourceSeries: sourceBars,
		Window:       native.RangeReversionWindow{TradeFromMS: fixture.TradeFromMS, TradeToMS: fixture.TradeToMS},
		Execution: native.RangeReversionExecution{
			SlippagePerFill:       fixture.Execution.SlippagePerFill,
			CommissionPerUnitSide: fixture.Execution.CommissionPerUnitSide,
			Units:                 fixture.Execution.Units,
		},
	}
	run, err := native.RunRangeReversion(request)
	if err != nil {
		return r11Failure("R11_ENGINE_REJECTED", err)
	}
	if len(run.Signals) > r11MaxSignals || len(run.Trades) > r11MaxTrades {
		return r11Failure("R11_ENGINE_REJECTED", errors.New("native output exceeds signal or trade limit"))
	}
	if r11EstimatedOutputBytes(len(run.Signals), len(run.Trades)) > r11MaxOutput {
		return r11Failure("R11_ENGINE_REJECTED", errors.New("native output exceeds conservative serialized-output budget"))
	}
	fixtureDigest := sha256.Sum256([]byte(rawFixture))
	envelope := r11Envelope{
		Schema: r11ResultSchema, ContractVersion: 1,
		Identity: &r11Identity{
			FixtureSHA256: hex.EncodeToString(fixtureDigest[:]), DSLSHA256: dslHash,
			StrategyID: fixture.StrategyID, Symbol: fixture.Symbol, Timeframe: fixture.Timeframe,
			SourceTimeframe: fixture.SourceTimeframe, TradeFromMS: fixture.TradeFromMS,
			TradeToMS: fixture.TradeToMS, Execution: fixture.Execution, EffectiveConfig: spec,
		},
		Run: &run,
	}
	encoded := &r11LimitedBuffer{limit: r11MaxOutput}
	if err := json.NewEncoder(encoded).Encode(envelope); err != nil {
		return r11Failure("R11_ENGINE_REJECTED", fmt.Errorf("encode native result: %w", err))
	}
	return encoded.Bytes()
}

func r11DSLHash(strategyID string) string {
	switch strategyID {
	case "r11-management-target-3r-30m":
		return r11Target3M30SHA256
	case "r11-entry-chop-45-1h":
		return r11Chop45H1SHA256
	default:
		return ""
	}
}

func validateR11Fixture(f r11Fixture) error {
	if f.Schema != r11FixtureSchema || f.ContractVersion != 1 {
		return errors.New("unsupported R11 fixture schema or contractVersion")
	}
	if f.Symbol != "XAUUSD" || f.SourceTimeframe != "4h" {
		return errors.New("R11 requires XAUUSD and original 4h source bars")
	}
	step := int64(30 * 60 * 1000)
	if f.Timeframe == "1h" {
		step = 60 * 60 * 1000
	} else if f.Timeframe != "30m" {
		return errors.New("R11 chart timeframe must be 30m or 1h")
	}
	if f.TradeFromMS <= 0 || f.TradeToMS <= f.TradeFromMS || f.TradeToMS > 9007199254740991 || f.TradeFromMS%step != 0 || f.TradeToMS%step != 0 {
		return errors.New("trade window must be a positive half-open UTC timeframe-aligned window")
	}
	if !finite(f.Execution.SlippagePerFill) || f.Execution.SlippagePerFill < 0 || !finite(f.Execution.CommissionPerUnitSide) || f.Execution.CommissionPerUnitSide < 0 || f.Execution.Units != 1 {
		return errors.New("execution requires finite nonnegative per-fill costs and exactly one unit")
	}
	if len(f.EntryBars) == 0 || len(f.EntryBars) > r11MaxEntryBars || len(f.SourceBars) == 0 || len(f.SourceBars) > r11MaxSourceBars {
		return errors.New("entry/source bar count is empty or exceeds its limit")
	}
	if err := validateR11Rows(f.EntryBars, step); err != nil {
		return fmt.Errorf("entryBars: %w", err)
	}
	if err := validateR11Rows(f.SourceBars, 4*60*60*1000); err != nil {
		return fmt.Errorf("sourceBars: %w", err)
	}
	usableSource := 0
	for _, row := range f.SourceBars {
		if int64(row[0]) < f.TradeToMS {
			usableSource++
		}
	}
	if usableSource < 2 {
		return errors.New("at least two source H4 rows are required before tradeToMs")
	}
	return nil
}

func validateR11Strategy(f r11Fixture, spec dsl.RangeReversionSpec) error {
	wantID, wantTF, wantCHOP, wantTarget := "r11-management-target-3r-30m", "M30", 48.0, 3.0
	if f.StrategyID == "r11-entry-chop-45-1h" {
		wantID, wantTF, wantCHOP, wantTarget = "r11-entry-chop-45-1h", "H1", 45, 2
	}
	if f.StrategyID != wantID {
		return errors.New("strategyId is not one of the pinned R11 candidates")
	}
	if (f.Timeframe == "30m" && wantTF != "M30") || (f.Timeframe == "1h" && wantTF != "H1") {
		return errors.New("strategyId and fixture timeframe differ")
	}
	want := dsl.RangeReversionRules{
		Policy: dsl.RangeReversionDelayedPinePolicy, Timeframe: wantTF,
		SourceTimeframe: "H4", Bounds: "CHART", BoundsLookback: 20,
		SourceAvailability: dsl.RangeReversionSourceAvailability,
		RequireCandleColor: true, UseHTFEMA: true, HTFEMALength: 200,
		UseVectorGates: true, VectorLength: 14, MaxADX: 30, MinCHOP: wantCHOP,
		UseRangeExpansion: true, RangeATRMultiple: 1.1, ATRLength: 14,
		StopATRMultiple: 0.6, TargetR: wantTarget, CooldownBars: 2,
		BreakEvenEnabled: true, BreakEvenTriggerR: 1, BreakEvenOffsetTicks: 10,
		TickSize: 0.01,
	}
	if spec.Rules != want {
		return errors.New("effective R11 configuration differs from the fixed strategy definition")
	}
	return nil
}

func validateR11Rows(rows [][]float64, step int64) error {
	var previous int64 = -1
	for i, row := range rows {
		if len(row) != 6 {
			return fmt.Errorf("row %d must contain exactly six binary64 values", i)
		}
		for _, value := range row {
			if !finite(value) {
				return fmt.Errorf("row %d contains a nonfinite value", i)
			}
		}
		timestamp := row[0]
		if timestamp <= 0 || math.Trunc(timestamp) != timestamp || timestamp > 9007199254740991 || int64(timestamp)%step != 0 || int64(timestamp) <= previous {
			return fmt.Errorf("row %d timestamp must be a strictly increasing safe UTC millisecond on the timeframe grid", i)
		}
		o, h, l, c, v := row[1], row[2], row[3], row[4], row[5]
		if l <= 0 || o < l || o > h || c < l || c > h || v < 0 {
			return fmt.Errorf("row %d has invalid OHLCV bounds", i)
		}
		previous = int64(timestamp)
	}
	return nil
}

func r11RowsToSeries(rows [][]float64, timeframe string) (marketdata.Series, error) {
	step := int64(30 * 60 * 1000)
	switch timeframe {
	case "1h":
		step = 60 * 60 * 1000
	case "4h":
		step = 4 * 60 * 60 * 1000
	}
	if err := validateR11Rows(rows, step); err != nil {
		return marketdata.Series{}, err
	}
	s := marketdata.Series{T: make([]float64, len(rows)), O: make([]float64, len(rows)), H: make([]float64, len(rows)), L: make([]float64, len(rows)), C: make([]float64, len(rows)), V: make([]float64, len(rows))}
	for i, row := range rows {
		s.T[i], s.O[i], s.H[i], s.L[i], s.C[i], s.V[i] = row[0], row[1], row[2], row[3], row[4], row[5]
	}
	return s, nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func r11EstimatedOutputBytes(signals, trades int) int64 {
	// The per-item allowances exceed the JSON width of these fixed Go structs,
	// including worst-case float64 and safe timestamp spellings.
	return int64(signals)*512 + int64(trades)*1024 + 65536
}

type r11JSONReader struct {
	decoder *json.Decoder
	tokens  int
}

func (r *r11JSONReader) token() (any, error) {
	value, err := r.decoder.Token()
	if err != nil {
		return nil, err
	}
	r.tokens++
	if r.tokens > r11MaxJSONTokens {
		return nil, errors.New("fixture JSON exceeds token limit")
	}
	return value, nil
}

func (r *r11JSONReader) delimiter(want byte) error {
	value, err := r.token()
	if err != nil {
		return err
	}
	if value != json.Delim(want) {
		return fmt.Errorf("expected JSON delimiter %q", want)
	}
	return nil
}

func (r *r11JSONReader) number(label string) error {
	value, err := r.token()
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	number, ok := value.(json.Number)
	if !ok {
		return fmt.Errorf("%s must be a non-null JSON number", label)
	}
	parsed, err := number.Float64()
	if err != nil || !finite(parsed) {
		return fmt.Errorf("%s must be finite", label)
	}
	return nil
}

func (r *r11JSONReader) scalar(key string) error {
	value, err := r.token()
	if err != nil {
		return fmt.Errorf("field %q: %w", key, err)
	}
	switch key {
	case "schema", "strategyId", "symbol", "timeframe", "sourceTimeframe":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("field %q must be a non-null string", key)
		}
	default:
		if number, ok := value.(json.Number); !ok {
			return fmt.Errorf("field %q must be a non-null JSON number", key)
		} else if parsed, err := number.Float64(); err != nil || !finite(parsed) {
			return fmt.Errorf("field %q must be finite", key)
		}
	}
	return nil
}

func (r *r11JSONReader) execution() error {
	if err := r.delimiter('{'); err != nil {
		return errors.New("execution must be an object")
	}
	seen := make(map[string]bool, 3)
	for r.decoder.More() {
		token, err := r.token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok || key != "slippagePerFill" && key != "commissionPerUnitSide" && key != "units" {
			return errors.New("execution has an unknown field")
		}
		if seen[key] {
			return fmt.Errorf("duplicate JSON object key %q", key)
		}
		seen[key] = true
		if err := r.number("execution." + key); err != nil {
			return err
		}
	}
	if err := r.delimiter('}'); err != nil {
		return err
	}
	if len(seen) != 3 {
		return errors.New("execution has missing fields")
	}
	return nil
}

func (r *r11JSONReader) rows(label string) error {
	if err := r.delimiter('['); err != nil {
		return fmt.Errorf("%s must be an array of six-number rows", label)
	}
	rowIndex := 0
	for r.decoder.More() {
		if err := r.delimiter('['); err != nil {
			return fmt.Errorf("%s row %d must be an array", label, rowIndex)
		}
		for column := 0; column < 6; column++ {
			if err := r.number(fmt.Sprintf("%s row %d field %d", label, rowIndex, column)); err != nil {
				return err
			}
		}
		if r.decoder.More() {
			return fmt.Errorf("%s row %d has more than six values", label, rowIndex)
		}
		if err := r.delimiter(']'); err != nil {
			return fmt.Errorf("%s row %d is malformed", label, rowIndex)
		}
		rowIndex++
	}
	if err := r.delimiter(']'); err != nil {
		return fmt.Errorf("%s array is malformed", label)
	}
	return nil
}

// validateR11FixtureJSON checks schema, required fields, duplicate keys and
// numeric row cells in one bounded token walk. A second typed decode then
// materializes the validated rows for the native engine.
func validateR11FixtureJSON(raw string) error {
	reader := &r11JSONReader{decoder: json.NewDecoder(strings.NewReader(raw))}
	reader.decoder.UseNumber()
	if err := reader.delimiter('{'); err != nil {
		return errors.New("fixture must be a JSON object")
	}
	required := map[string]bool{
		"schema": false, "contractVersion": false, "strategyId": false, "symbol": false,
		"timeframe": false, "sourceTimeframe": false, "tradeFromMs": false,
		"tradeToMs": false, "execution": false, "entryBars": false, "sourceBars": false,
	}
	for reader.decoder.More() {
		token, err := reader.token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return errors.New("fixture has an unknown field")
		}
		if _, exists := required[key]; !exists {
			return errors.New("fixture has an unknown field")
		}
		if required[key] {
			return fmt.Errorf("duplicate JSON object key %q", key)
		}
		required[key] = true
		switch key {
		case "execution":
			err = reader.execution()
		case "entryBars", "sourceBars":
			err = reader.rows(key)
		default:
			err = reader.scalar(key)
		}
		if err != nil {
			return err
		}
	}
	if err := reader.delimiter('}'); err != nil {
		return errors.New("fixture object is malformed")
	}
	for key, present := range required {
		if !present {
			return fmt.Errorf("fixture field %q is missing", key)
		}
	}
	if _, err := reader.decoder.Token(); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return fmt.Errorf("trailing JSON data: %w", err)
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return fmt.Errorf("trailing JSON data: %w", err)
	}
	return nil
}
