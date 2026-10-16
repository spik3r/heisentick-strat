package engine

import (
	"bytes"
	"encoding/json"
	"math/big"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const PaperBidAskMaxBytes = 64 << 20
const PaperBidAskMaxRecords = 65536
const PaperBidAskMaxQuoteIdentities = PaperBidAskMaxRecords
const PaperBidAskMaxBars = 4096
const PaperBidAskMaxEvents = 262144
const PaperBidAskMaxOutputBytes = 128 << 20
const PaperBidAskMaxSourceBytes = 64 << 10

// Decimal transport is exact; family binary64 operands cross into the financial
// model by their shortest round-trip base-ten representation, never an epsilon.
type BidAskDecimal string

var bidAskDecimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,11})(\.[0-9]{1,12})?$`)
var bidAskTimestampPattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?Z$`)

func bidAskNumber(s BidAskDecimal) (*big.Rat, error) {
	if !bidAskDecimalPattern.MatchString(string(s)) {
		return nil, bidAskError("DECIMAL")
	}
	r, ok := new(big.Rat).SetString(string(s))
	if !ok {
		return nil, bidAskError("DECIMAL")
	}
	return r, nil
}
func bidAskFloat(s BidAskDecimal) (float64, error) {
	r, e := bidAskNumber(s)
	if e != nil {
		return 0, e
	}
	f, _ := r.Float64()
	return f, nil
}
func bidAskTime(s string) (int64, error) {
	// Go time.Parse accepts a comma separator, an unpadded hour and arbitrarily
	// many fractional digits (truncating after nanoseconds). None belongs to this
	// exact transport grammar: reject before parsing so grid ties cannot move.
	if len(s) < 20 || len(s) > 30 || !bidAskTimestampPattern.MatchString(s) {
		return 0, bidAskError("TIMESTAMP")
	}
	t, e := time.Parse(time.RFC3339Nano, s)
	if e != nil || t.Year() < 1970 || t.Year() > 2200 {
		return 0, bidAskError("TIMESTAMP")
	}
	return t.UnixNano(), nil
}
func bidAskStamp(ns int64) string { return time.Unix(0, ns).UTC().Format(time.RFC3339Nano) }
func bidAskSequence(s string) (int, error) {
	n, e := strconv.Atoi(s)
	if e != nil || n < 1 || strconv.Itoa(n) != s {
		return 0, bidAskError("SEQUENCE")
	}
	return n, nil
}

type BidAskOHLC struct {
	Open  BidAskDecimal `json:"open"`
	High  BidAskDecimal `json:"high"`
	Low   BidAskDecimal `json:"low"`
	Close BidAskDecimal `json:"close"`
}
type BidAskBar struct {
	OpenT            string     `json:"openT"`
	CloseT           string     `json:"closeT"`
	ReceivedT        string     `json:"receivedT"`
	ProviderID       string     `json:"providerId"`
	ProvenanceDigest string     `json:"provenanceDigest"`
	Mid              BidAskOHLC `json:"mid"`
	Bid              BidAskOHLC `json:"bid"`
	Ask              BidAskOHLC `json:"ask"`
	PriceCount       string     `json:"priceCount"`
	Complete         bool       `json:"complete"`
}
type BidAskTier struct {
	Price     BidAskDecimal `json:"price"`
	Liquidity BidAskDecimal `json:"liquidity"`
}
type BidAskQuote struct {
	ProviderT             string       `json:"providerT"`
	RequestStartT         string       `json:"requestStartT"`
	ReceivedT             string       `json:"receivedT"`
	QualifiedT            string       `json:"qualifiedT"`
	ProviderID            string       `json:"providerId"`
	ProvenanceDigest      string       `json:"provenanceDigest"`
	AccountDigest         string       `json:"accountDigest"`
	Instrument            string       `json:"instrument"`
	Tradeable             bool         `json:"tradeable"`
	Bids                  []BidAskTier `json:"bids"`
	Asks                  []BidAskTier `json:"asks"`
	EpisodeSignalSequence string       `json:"episodeSignalSequence"`
}
type BidAskEpisode struct {
	SignalSequence string `json:"signalSequence"`
	AdmittedT      string `json:"admittedT"`
	DeadlineT      string `json:"deadlineT"`
}
type BidAskSeal struct {
	SignalSequence string `json:"signalSequence"`
	Reason         string `json:"reason"`
	EffectiveT     string `json:"effectiveT"`
}
type BidAskGap struct {
	FromT    string `json:"fromT"`
	ThroughT string `json:"throughT"`
	Reason   string `json:"reason"`
}
type BidAskRecord struct {
	ExperimentClock *BidAskExperimentClock `json:"experimentClock,omitempty"`
	Closure         *BidAskClosure         `json:"closure,omitempty"`
	Sequence        string                 `json:"sequence"`
	Epoch           string                 `json:"epoch"`
	CommitT         string                 `json:"commitT"`
	Kind            string                 `json:"kind"`
	Bar             *BidAskBar             `json:"bar,omitempty"`
	Episode         *BidAskEpisode         `json:"episode,omitempty"`
	Quote           *BidAskQuote           `json:"quote,omitempty"`
	Seal            *BidAskSeal            `json:"seal,omitempty"`
	Gap             *BidAskGap             `json:"gap,omitempty"`
}

// Strict shape checks cover absent zero-valued fields, case aliases, nulls,
// duplicate keys, strings and nesting before decoding. Optional means omitted,
// never null. Decimal and nanosecond timestamp values are JSON strings.
func bidAskShape(raw json.RawMessage, t reflect.Type) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return bidAskError("INPUT")
	}
	if t.Kind() == reflect.Pointer {
		return bidAskShape(raw, t.Elem())
	}
	if t == reflect.TypeOf(json.RawMessage{}) {
		return nil
	}
	switch t.Kind() {
	case reflect.Struct:
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return bidAskError("INPUT")
		}
		allowed := map[string]bool{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := strings.Split(f.Tag.Get("json"), ",")
			key := tag[0]
			allowed[key] = true
			v, ok := fields[key]
			if !ok {
				if len(tag) > 1 && tag[1] == "omitempty" {
					continue
				}
				return bidAskError("INPUT")
			}
			if e := bidAskShape(v, f.Type); e != nil {
				return e
			}
		}
		for k := range fields {
			if !allowed[k] {
				return bidAskError("INPUT")
			}
		}
	case reflect.Slice:
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil {
			return bidAskError("INPUT")
		}
		for _, v := range values {
			if e := bidAskShape(v, t.Elem()); e != nil {
				return e
			}
		}
	}
	return nil
}
func DecodePaperBidAskRequest(raw []byte) (PaperBidAskRequest, error) {
	var r PaperBidAskRequest
	if len(raw) > PaperBidAskMaxBytes || !utf8.Valid(raw) || paperJSONStrings(raw) != nil || paperJSONUnique(raw) != nil {
		return r, bidAskError("INPUT")
	}
	if e := bidAskShape(raw, reflect.TypeOf(r)); e != nil {
		return r, e
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil {
		return PaperBidAskRequest{}, bidAskError("INPUT")
	}
	return r, nil
}
func bidAskValidOHLC(v BidAskOHLC) error {
	o, e := bidAskNumber(v.Open)
	if e != nil {
		return e
	}
	h, e := bidAskNumber(v.High)
	if e != nil {
		return e
	}
	l, e := bidAskNumber(v.Low)
	if e != nil {
		return e
	}
	c, e := bidAskNumber(v.Close)
	if e != nil {
		return e
	}
	if l.Sign() <= 0 || h.Cmp(l) < 0 || h.Cmp(o) < 0 || h.Cmp(c) < 0 || l.Cmp(o) > 0 || l.Cmp(c) > 0 {
		return bidAskError("BAR")
	}
	return nil
}
func bidAskDigestValid(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func validateBidAskRecords(r PaperBidAskRequest, duration int64) ([]BidAskBar, error) {
	bars := []BidAskBar{}
	var lastCommit, lastOpen int64 = -1, -1
	for i, rec := range r.Records {
		seq, e := bidAskSequence(rec.Sequence)
		if e != nil || seq != i+1 || rec.Epoch != r.Identity.Epoch {
			return nil, bidAskError("SEQUENCE")
		}
		commit, e := bidAskTime(rec.CommitT)
		if e != nil || commit < lastCommit {
			return nil, bidAskError("COMMIT_ORDER")
		}
		if err := validateBidAskExperimentRecord(r, rec); err != nil {
			return nil, err
		}
		lastCommit = commit
		count := 0
		for _, present := range []bool{rec.Bar != nil, rec.Episode != nil, rec.Quote != nil, rec.Seal != nil, rec.Gap != nil, rec.Closure != nil, rec.ExperimentClock != nil} {
			if present {
				count++
			}
		}
		if count != 1 {
			return nil, bidAskError("RECORD")
		}
		switch rec.Kind {
		case "bar":
			b := rec.Bar
			if b == nil {
				return nil, bidAskError("RECORD")
			}
			open, e := bidAskTime(b.OpenT)
			if e != nil {
				return nil, e
			}
			close, e := bidAskTime(b.CloseT)
			if e != nil {
				return nil, e
			}
			received, e := bidAskTime(b.ReceivedT)
			if e != nil {
				return nil, e
			}
			if !b.Complete || open%duration != 0 || close-open != duration || open <= lastOpen || received < close || commit < received || !bidAskDigestValid(b.ProviderID) || !bidAskDigestValid(b.ProvenanceDigest) {
				return nil, bidAskError("BAR")
			}
			lastOpen = open
			for _, v := range []BidAskOHLC{b.Mid, b.Bid, b.Ask} {
				if e := bidAskValidOHLC(v); e != nil {
					return nil, e
				}
			}
			for _, v := range [][3]BidAskDecimal{{b.Bid.Open, b.Mid.Open, b.Ask.Open}, {b.Bid.High, b.Mid.High, b.Ask.High}, {b.Bid.Low, b.Mid.Low, b.Ask.Low}, {b.Bid.Close, b.Mid.Close, b.Ask.Close}} {
				lo, _ := bidAskNumber(v[0])
				mid, _ := bidAskNumber(v[1])
				hi, _ := bidAskNumber(v[2])
				if lo.Cmp(mid) > 0 || mid.Cmp(hi) > 0 {
					return nil, bidAskError("BAR_COMPONENTS")
				}
			}
			v, e := strconv.ParseUint(b.PriceCount, 10, 53)
			if e != nil || strconv.FormatUint(v, 10) != b.PriceCount {
				return nil, bidAskError("VOLUME")
			}
			bars = append(bars, *b)
			if len(bars) > PaperBidAskMaxBars {
				return nil, bidAskError("BAR_LIMIT")
			}
		case "quote":
			q := rec.Quote
			if q == nil || q.AccountDigest != r.Identity.AccountDigest || q.Instrument != r.Product.Instrument || !bidAskDigestValid(q.ProviderID) || !bidAskDigestValid(q.ProvenanceDigest) || len(q.Bids) < 1 || len(q.Asks) < 1 || len(q.Bids) > 8 || len(q.Asks) > 8 {
				return nil, bidAskError("QUOTE")
			}
			provider, e := bidAskTime(q.ProviderT)
			if e != nil {
				return nil, e
			}
			start, e := bidAskTime(q.RequestStartT)
			if e != nil {
				return nil, e
			}
			received, e := bidAskTime(q.ReceivedT)
			if e != nil {
				return nil, e
			}
			qualified, e := bidAskTime(q.QualifiedT)
			if e != nil {
				return nil, e
			}
			if provider > received || start > received || received > qualified || qualified > commit {
				return nil, bidAskError("QUOTE_CLOCK")
			}
			for side, tiers := range [][]BidAskTier{q.Bids, q.Asks} {
				var previous *big.Rat
				for _, tier := range tiers {
					p, e := bidAskNumber(tier.Price)
					if e != nil {
						return nil, e
					}
					if p.Sign() <= 0 {
						return nil, bidAskError("QUOTE")
					}
					if _, e := bidAskNumber(tier.Liquidity); e != nil {
						return nil, e
					}
					if previous != nil && ((side == 0 && previous.Cmp(p) <= 0) || (side == 1 && previous.Cmp(p) >= 0)) {
						return nil, bidAskError("QUOTE_DEPTH")
					}
					previous = p
				}
			}
			bid, _ := bidAskNumber(q.Bids[0].Price)
			ask, _ := bidAskNumber(q.Asks[0].Price)
			if bid.Cmp(ask) > 0 {
				return nil, bidAskError("QUOTE_SPREAD")
			}
			if q.EpisodeSignalSequence != "" {
				if _, e := bidAskSequence(q.EpisodeSignalSequence); e != nil {
					return nil, e
				}
			}
		case "episode":
			e := rec.Episode
			if e == nil {
				return nil, bidAskError("RECORD")
			}
			if _, err := bidAskSequence(e.SignalSequence); err != nil {
				return nil, err
			}
			admit, err := bidAskTime(e.AdmittedT)
			if err != nil {
				return nil, err
			}
			deadline, err := bidAskTime(e.DeadlineT)
			if err != nil {
				return nil, err
			}
			if admit >= deadline || commit < admit || commit > deadline {
				return nil, bidAskError("EPISODE_CLOCK")
			}
		case "seal":
			s := rec.Seal
			if s == nil || (s.Reason != "recovery" && s.Reason != "deadline" && !(s.Reason == "signal-expired" && r.Experiment != nil)) {
				return nil, bidAskError("SEAL")
			}
			if _, e := bidAskSequence(s.SignalSequence); e != nil {
				return nil, e
			}
			at, e := bidAskTime(s.EffectiveT)
			if e != nil || at > commit {
				return nil, bidAskError("SEAL")
			}
		case "experiment-clock":
			c := rec.ExperimentClock
			if c == nil || r.Experiment == nil || i != len(r.Records)-1 {
				return nil, bidAskError("EXPERIMENT_CLOCK")
			}
			at, err := bidAskTime(c.EffectiveT)
			if err != nil {
				return nil, err
			}
			until, err := bidAskTime(r.Experiment.ObservationUntilT)
			if err != nil {
				return nil, err
			}
			activation, err := bidAskTime(r.ActivationFromT)
			if err != nil {
				return nil, err
			}
			calendarThrough, err := bidAskTime(r.Experiment.CalendarThroughT)
			if err != nil {
				return nil, err
			}
			if (c.Reason == "calendar-coverage-expired" && at != calendarThrough) || at > calendarThrough {
				return nil, bidAskError("EXPERIMENT_CLOCK")
			}
			if at > commit || at < activation || at > until || (c.Reason != "experiment-observation-horizon" && c.Reason != "experiment-capacity-exhausted" && c.Reason != "calendar-coverage-expired") || (c.Reason == "experiment-observation-horizon" && at != until) {
				return nil, bidAskError("EXPERIMENT_CLOCK")
			}
		case "market-closure":
			c := rec.Closure
			if c == nil || r.Experiment == nil {
				return nil, bidAskError("CLOSURE")
			}
			index, err := bidAskSequence(c.WindowIndex)
			if err != nil || index > len(r.Experiment.Closures) {
				return nil, bidAskError("CLOSURE")
			}
			from, err := bidAskTime(c.FromT)
			if err != nil {
				return nil, err
			}
			through, err := bidAskTime(c.ThroughT)
			if err != nil {
				return nil, err
			}
			window := r.Experiment.Closures[index-1]
			windowFrom, err := bidAskTime(window.FromT)
			if err != nil {
				return nil, err
			}
			windowThrough, err := bidAskTime(window.ThroughT)
			if err != nil {
				return nil, err
			}
			if from%duration != 0 || through%duration != 0 || from >= through || from < windowFrom || through > windowThrough || commit < through {
				return nil, bidAskError("CLOSURE")
			}
		case "coverage-gap":
			g := rec.Gap
			if g == nil || (g.Reason != "disconnect" && g.Reason != "missing-samples" && g.Reason != "restart") {
				return nil, bidAskError("GAP")
			}
			from, e := bidAskTime(g.FromT)
			if e != nil {
				return nil, e
			}
			through, e := bidAskTime(g.ThroughT)
			if e != nil || from > through || through > commit {
				return nil, bidAskError("GAP")
			}
		default:
			return nil, bidAskError("RECORD")
		}
	}
	return bars, nil
}
