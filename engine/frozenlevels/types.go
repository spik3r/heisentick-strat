// Package frozenlevels contains causal, source-qualified research primitives.
// It is not a registered strategy family or a quote execution/reporting engine.
package frozenlevels

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

const ContractVersion = "frozen-level-primitives-v1"
const M5Millis int64 = 5 * 60 * 1000
const maxExactMillis int64 = 9007199254740991

type QuoteSide string

const (
	Bid QuoteSide = "bid"
	Ask QuoteSide = "ask"
	Mid QuoteSide = "mid"
)

// Source identifies the actual input, not the instrument's broker contract.
// A digest is mandatory even for synthetic fixtures. Calendar qualification is
// separate: a source's date envelope does not establish continuous coverage.
type Source struct {
	Provider string    `json:"provider"`
	Dataset  string    `json:"dataset"`
	SHA256   string    `json:"sha256"`
	Side     QuoteSide `json:"side"`
}

func (s Source) validate() error {
	b, err := hex.DecodeString(s.SHA256)
	if strings.TrimSpace(s.Provider) == "" || strings.TrimSpace(s.Dataset) == "" || err != nil || len(b) != 32 {
		return fmt.Errorf("invalid-source-identity")
	}
	if s.Side != Bid && s.Side != Ask && s.Side != Mid {
		return fmt.Errorf("invalid-quote-side")
	}
	return nil
}
func finitePositive(x float64) bool { return x > 0 && !math.IsNaN(x) && !math.IsInf(x, 0) }
func validMillis(t int64) bool      { return t >= 0 && t <= maxExactMillis }
func identity(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Observation order is lexicographic (timestamp, source sequence). Equal
// timestamps with increasing sequence are distinct, strictly later events.
type EventKey struct {
	AtMillis int64 `json:"atMillis"`
	Sequence int64 `json:"sequence"`
}

func (k EventKey) valid() bool {
	return validMillis(k.AtMillis) && k.Sequence >= 0 && k.Sequence <= maxExactMillis
}
func (k EventKey) After(other EventKey) bool {
	return k.AtMillis > other.AtMillis || (k.AtMillis == other.AtMillis && k.Sequence > other.Sequence)
}
