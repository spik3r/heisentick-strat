package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const MaxBasketJSONBytes = 32 << 20
const MaxBasketLegs = 128
const MaxBasketTrades = 100000
const maxBasketMetadataBytes = 64 << 10
const maxBasketEpochMS int64 = 253402300799999 // Last millisecond in UTC year 9999.

// DecodeBasketReport is the shared native/WASM boundary. All calculation fields
// use exact allowlists, reject duplicate keys and wrong casing, and admit one
// bounded document. Only designated opaque metadata objects permit arbitrary
// keys; their entire contents are echoed and are never executed.
func DecodeBasketReport(raw []byte) (BasketResult, error) {
	if len(raw) == 0 || len(raw) > MaxBasketJSONBytes {
		return BasketResult{}, fmt.Errorf("basket request must contain 1..%d bytes", MaxBasketJSONBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := checkBasketJSON(decoder, 0); err != nil {
		return BasketResult{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return BasketResult{}, fmt.Errorf("basket request has trailing JSON")
	}
	if err := basketKeys(raw, []string{"schema", "nowMs", "identity", "legs", "bar", "sourceIsDsl"}, []string{"schema", "nowMs", "identity", "legs"}); err != nil {
		return BasketResult{}, err
	}
	var root map[string]json.RawMessage
	_ = json.Unmarshal(raw, &root)
	if bar := root["bar"]; len(bar) > 0 && !bytes.Equal(bar, []byte("null")) {
		if err := basketKeys(bar, basketBarKeys, basketBarKeys); err != nil {
			return BasketResult{}, err
		}
	}
	var legs []json.RawMessage
	if err := json.Unmarshal(root["legs"], &legs); err != nil || bytes.Equal(root["legs"], []byte("null")) {
		return BasketResult{}, fmt.Errorf("basket legs must be an array")
	}
	if len(legs) > MaxBasketLegs {
		return BasketResult{}, fmt.Errorf("too many basket legs")
	}
	for _, rawLeg := range legs {
		if err := basketKeys(rawLeg, []string{"symbol", "tf", "bars", "dataSpan", "identity", "provenance", "trades", "harshTrades", "harshIdentity"}, []string{"symbol", "tf", "bars", "identity", "trades"}); err != nil {
			return BasketResult{}, err
		}
		var leg map[string]json.RawMessage
		_ = json.Unmarshal(rawLeg, &leg)
		if span := leg["dataSpan"]; len(span) > 0 && !bytes.Equal(span, []byte("null")) {
			if err := basketKeys(span, []string{"firstBarT", "lastBarT"}, nil); err != nil {
				return BasketResult{}, err
			}
		}
		for _, name := range []string{"trades", "harshTrades"} {
			value := leg[name]
			if name == "harshTrades" && (len(value) == 0 || bytes.Equal(value, []byte("null"))) {
				continue
			}
			var trades []json.RawMessage
			if err := json.Unmarshal(value, &trades); err != nil || bytes.Equal(value, []byte("null")) {
				return BasketResult{}, fmt.Errorf("%s must be an array", name)
			}
			for _, trade := range trades {
				if err := basketKeys(trade, []string{"entryT", "exitT", "entryIndex", "exitIndex", "side", "tag", "entry", "size", "pnl", "initialSl", "sl", "partial", "status"}, []string{"entryT", "entry", "size", "pnl"}); err != nil {
					return BasketResult{}, err
				}
				var fields map[string]json.RawMessage
				_ = json.Unmarshal(trade, &fields)
				for _, required := range []string{"entryT", "entry", "size", "pnl"} {
					if bytes.Equal(fields[required], []byte("null")) {
						return BasketResult{}, fmt.Errorf("trade %s cannot be null", required)
					}
				}
			}
		}
	}
	var request BasketRequest
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return BasketResult{}, fmt.Errorf("basket request: %w", err)
	}
	result, err := BuildBasketReport(request)
	if err != nil {
		return BasketResult{}, err
	}
	result.RequestSHA256 = fmt.Sprintf("%x", sha256.Sum256(raw))
	return result, nil
}

func basketKeys(raw json.RawMessage, allowed, required []string) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return fmt.Errorf("basket field must be an object")
	}
	for key, value := range object {
		if bytes.Equal(value, []byte("null")) {
			switch key {
			case "bar", "sourceIsDsl", "dataSpan", "provenance", "harshTrades", "harshIdentity", "initialSl", "sl", "entryIndex", "firstBarT", "lastBarT":
			default:
				return fmt.Errorf("basket field %q cannot be null", key)
			}
		}
		found := false
		for _, name := range allowed {
			found = found || key == name
		}
		if !found {
			return fmt.Errorf("unknown basket field %q", key)
		}
	}
	for _, name := range required {
		if value, ok := object[name]; !ok || bytes.Equal(value, []byte("null")) {
			return fmt.Errorf("missing basket field %q", name)
		}
	}
	return nil
}

func checkBasketJSON(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return fmt.Errorf("basket JSON exceeds maximum depth")
	}
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("invalid basket JSON: %w", err)
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := map[string]bool{}
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := token.(string)
			if !ok || keys[key] {
				return fmt.Errorf("invalid or duplicate basket JSON key %q", key)
			}
			keys[key] = true
			if err := checkBasketJSON(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := checkBasketJSON(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("invalid basket JSON delimiter")
	}
	_, err = decoder.Token()
	return err
}

func basketMetadata(raw json.RawMessage, required bool) error {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		if required {
			return fmt.Errorf("basket identity object is required")
		}
		return nil
	}
	if len(raw) > maxBasketMetadataBytes {
		return fmt.Errorf("basket metadata exceeds bound")
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return fmt.Errorf("basket metadata must be an object")
	}
	return nil
}

func validateBasketRequest(request BasketRequest) error {
	if request.Schema != BasketRequestSchema {
		return fmt.Errorf("unsupported basket schema %q", request.Schema)
	}
	if request.NowMS < 0 || request.NowMS > maxBasketEpochMS {
		return fmt.Errorf("invalid basket nowMs")
	}
	if len(request.Legs) > MaxBasketLegs {
		return fmt.Errorf("too many basket legs")
	}
	if err := basketMetadata(request.Identity, true); err != nil {
		return err
	}
	if request.Bar != nil {
		if err := validateBasketBar(*request.Bar); err != nil {
			return err
		}
	}
	count := 0
	for _, leg := range request.Legs {
		if strings.TrimSpace(leg.Symbol) == "" || len(leg.Symbol) > 128 || strings.TrimSpace(leg.TF) == "" || len(leg.TF) > 64 || leg.Bars < 0 {
			return fmt.Errorf("invalid basket route or bars")
		}
		for _, item := range []struct {
			raw      json.RawMessage
			required bool
		}{{leg.Identity, true}, {leg.Provenance, false}, {leg.HarshIdentity, false}} {
			if err := basketMetadata(item.raw, item.required); err != nil {
				return err
			}
		}
		if leg.DataSpan != nil {
			for _, v := range []*int64{leg.DataSpan.FirstBarT, leg.DataSpan.LastBarT} {
				if v != nil && (*v < 0 || *v > maxBasketEpochMS) {
					return fmt.Errorf("invalid basket data span timestamp")
				}
			}
		}
		lists := [][]BasketTrade{leg.Trades}
		if leg.HarshTrades != nil {
			lists = append(lists, *leg.HarshTrades)
		}
		for _, trades := range lists {
			count += len(trades)
			if count > MaxBasketTrades {
				return fmt.Errorf("too many basket trades")
			}
			if err := validateBasketPartials(trades); err != nil {
				return err
			}
			for _, t := range trades {
				if t.EntryT < 0 || t.EntryT > maxBasketEpochMS || t.ExitT < 0 || t.ExitT > maxBasketEpochMS {
					return fmt.Errorf("invalid basket trade timestamp")
				}
				if !basketFinite(t.Entry) || !basketFinite(t.Size) || !basketFinite(t.PnL) {
					return fmt.Errorf("basket trades must be finite")
				}
				if t.InitialSL != nil && !basketFinite(*t.InitialSL) || t.SL != nil && !basketFinite(*t.SL) {
					return fmt.Errorf("basket stops must be finite or null")
				}
				if t.Status != "" && t.Status != "closed" && t.Status != "open" && t.Status != "prefix" {
					return fmt.Errorf("invalid basket trade status")
				}
				if len(t.Side) > 32 || len(t.Tag) > 1024 {
					return fmt.Errorf("basket trade identity exceeds bound")
				}
			}
		}
	}
	return nil
}

// A closed partial entry must have a final exit and invariant initial-stop
// metadata (or invariant fallback stops when initialSl is absent). Otherwise
// the original entry risk cannot be reconstructed safely.
func validateBasketPartials(trades []BasketTrade) error {
	anyPartial := false
	for _, trade := range trades {
		if trade.Status != "open" && trade.Status != "prefix" {
			anyPartial = anyPartial || trade.Partial
		}
	}
	if !anyPartial {
		return nil
	}

	type key struct {
		Index     int
		T         int64
		Side, Tag string
		Entry     float64
	}
	type group struct {
		stop                                   *float64
		fallback                               *float64
		partial                                bool
		terminals                              int
		inconsistentStop, inconsistentFallback bool
		count                                  int
	}
	groups := map[key]*group{}
	for _, trade := range trades {
		if trade.Status == "open" || trade.Status == "prefix" {
			continue
		}
		index := -1
		if trade.EntryIndex != nil {
			index = *trade.EntryIndex
		}
		k := key{index, trade.EntryT, trade.Side, trade.Tag, trade.Entry}
		g := groups[k]
		if g == nil {
			g = &group{stop: trade.InitialSL, fallback: trade.SL}
			groups[k] = g
		}
		if (g.stop == nil) != (trade.InitialSL == nil) || g.stop != nil && *g.stop != *trade.InitialSL {
			g.inconsistentStop = true
		}
		if g.stop == nil && ((g.fallback == nil) != (trade.SL == nil) || g.fallback != nil && *g.fallback != *trade.SL) {
			g.inconsistentFallback = true
		}
		g.partial = g.partial || trade.Partial
		if !trade.Partial {
			g.terminals++
		}
		g.count++
	}
	for _, g := range groups {
		if !g.partial {
			continue
		}
		if g.terminals != 1 || g.count < 2 {
			return fmt.Errorf("partial basket entry requires exactly one terminal exit")
		}
		if g.inconsistentStop {
			return fmt.Errorf("inconsistent initial stop for partial basket entry")
		}
		if g.inconsistentFallback {
			return fmt.Errorf("inconsistent fallback stop for partial basket entry")
		}
	}
	return nil
}
