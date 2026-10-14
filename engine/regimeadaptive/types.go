// Package regimeadaptive is an offline synthetic indicator/signal inspector.
// It is not an execution engine and cannot admit orders to any broker route.
package regimeadaptive

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
)

const SourceProfile = "regime-adaptive-v4.2-source-parity-v1"
const FlatProfile = "regime-adaptive-v4.2-flat-risk-v1"
const SourceHash = "bf40faa4f320f64c555fb38689128449c9967b4f97bc42de9af23e11516b7da7"
const FlatHash = "be7da27aa7224a9f092a9b40bc88462ba7c6fa0865220aa9eb900f4b9284f3ee"
const Schema = "regime-adaptive-synthetic-inspect-v1"

type Bar struct {
	T      int64   `json:"t"`
	CloseT int64   `json:"closeT"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
}
type Timeframe struct {
	Unit       string `json:"unit"`
	Multiplier int    `json:"multiplier"`
}

// Request is closed: only fixed default Inputs, native closed bars and synthetic
// inspection are supported. No implied session construction or execution defaults.
type Request struct {
	Schema       string    `json:"schema"`
	Mode         string    `json:"mode"`
	Profile      string    `json:"profile"`
	SourceSHA256 string    `json:"sourceSha256"`
	DataKind     string    `json:"dataKind"`
	FixtureID    string    `json:"fixtureId"`
	Timeframe    Timeframe `json:"timeframe"`
	Bars         []Bar     `json:"bars"`
}
type Signals struct {
	InitialLong  bool `json:"initialLong"`
	InitialShort bool `json:"initialShort"`
	ScaleLong    bool `json:"scaleLong"`
	ScaleShort   bool `json:"scaleShort"`
	RangeLong    bool `json:"rangeLong"`
	RangeShort   bool `json:"rangeShort"`
	BothBands    bool `json:"bothBands"`
	FlatEligible bool `json:"flatEligible"`
}
type Row struct {
	T       int64    `json:"t"`
	CloseT  int64    `json:"closeT"`
	TR      *float64 `json:"tr"`
	ATR     *float64 `json:"atr"`
	CHOP    *float64 `json:"chop"`
	RSI     *float64 `json:"rsi"`
	RSIMA   *float64 `json:"rsiMa"`
	PSAR    *float64 `json:"psar"`
	BBBasis *float64 `json:"bbBasis"`
	BBUpper *float64 `json:"bbUpper"`
	BBLower *float64 `json:"bbLower"`
	Regime  int      `json:"regime"`
	Signals Signals  `json:"signals"`
}
type Result struct {
	Schema            string         `json:"schema"`
	Profile           string         `json:"profile"`
	SourceSHA256      string         `json:"sourceSha256"`
	RequestSHA256     string         `json:"requestSha256"`
	FixtureID         string         `json:"fixtureId"`
	Timeframe         Timeframe      `json:"timeframe"`
	Inputs            map[string]any `json:"inputs"`
	Rows              []Row          `json:"rows"`
	ExecutionStatus   string         `json:"executionStatus"`
	OrderLifecycle    any            `json:"orderLifecycle"`
	Trades            any            `json:"trades"`
	PendingOrders     any            `json:"pendingOrders"`
	TerminalPositions any            `json:"terminalPositions"`
	Limitations       []string       `json:"limitations"`
}

func Decode(raw []byte) (Request, error) {
	// encoding/json normally overwrites duplicate object keys; reject them first.
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return err
				}
				key, ok := k.(string)
				if !ok || seen[key] {
					return fmt.Errorf("duplicate/invalid JSON key %v", k)
				}
				seen[key] = true
				if err := walk(); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := walk(); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unexpected delimiter")
		}
		_, err = d.Token()
		return err
	}
	if err := walk(); err != nil {
		return Request{}, err
	}
	if _, err := d.Token(); err != io.EOF {
		return Request{}, fmt.Errorf("exactly one JSON document required")
	}
	// Require exact keys, including zero-valued members. Case aliases and null
	// cannot silently inherit Go struct zero values.
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return Request{}, err
	}
	if err := exact(root, "schema", "mode", "profile", "sourceSha256", "dataKind", "fixtureId", "timeframe", "bars"); err != nil {
		return Request{}, err
	}
	var tf map[string]json.RawMessage
	if err := json.Unmarshal(root["timeframe"], &tf); err != nil {
		return Request{}, err
	}
	if err := exact(tf, "unit", "multiplier"); err != nil {
		return Request{}, err
	}
	var bars []map[string]json.RawMessage
	if err := json.Unmarshal(root["bars"], &bars); err != nil {
		return Request{}, err
	}
	for _, b := range bars {
		if err := exact(b, "t", "closeT", "open", "high", "low", "close"); err != nil {
			return Request{}, err
		}
	}
	var req Request
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return Request{}, err
	}
	return req, nil
}
func exact(m map[string]json.RawMessage, fields ...string) error {
	if len(m) != len(fields) {
		return fmt.Errorf("exact JSON fields required")
	}
	for _, f := range fields {
		v, ok := m[f]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return fmt.Errorf("missing/null field %s", f)
		}
	}
	return nil
}
func hash(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func finite(x float64) bool  { return !math.IsNaN(x) && !math.IsInf(x, 0) }
func ptr(x float64) *float64 {
	if !finite(x) {
		return nil
	}
	return &x
}
func missing(n int) []float64 {
	a := make([]float64, n)
	for i := range a {
		a[i] = math.NaN()
	}
	return a
}
