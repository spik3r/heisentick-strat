package adaptivemc

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
)

var hashRE = regexp.MustCompile(`^[a-f0-9]{64}$`)

func uniqueJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 24 {
			return fmt.Errorf("JSON depth")
		}
		v, e := d.Token()
		if e != nil {
			return e
		}
		delim, ok := v.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				if e != nil {
					return e
				}
				k, ok := key.(string)
				if !ok || seen[k] {
					return fmt.Errorf("duplicate JSON key")
				}
				seen[k] = true
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
		default:
			return fmt.Errorf("JSON delimiter")
		}
		_, e = d.Token()
		return e
	}
	if e := walk(0); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return fmt.Errorf("one JSON document required")
	}
	return nil
}
func exactKeys(raw []byte, keys ...string) error {
	var m map[string]json.RawMessage
	if e := json.Unmarshal(raw, &m); e != nil {
		return e
	}
	if len(m) != len(keys) {
		return fmt.Errorf("exact JSON keys required")
	}
	for _, k := range keys {
		v, ok := m[k]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return fmt.Errorf("missing/null exact field")
		}
	}
	return nil
}

func ReadRequest(path string) (Request, error) {
	var r Request
	raw, e := os.ReadFile(path)
	if e != nil {
		return r, e
	}
	if len(raw) > 65536 {
		return r, fmt.Errorf("request size")
	}
	if e = uniqueJSON(raw); e != nil {
		return r, e
	}
	if e = exactKeys(raw, "schema", "studyHead", "inputs"); e != nil {
		return r, e
	}
	var requestFields map[string]json.RawMessage
	json.Unmarshal(raw, &requestFields)
	var inputFields []json.RawMessage
	json.Unmarshal(requestFields["inputs"], &inputFields)
	for _, in := range inputFields {
		if e = exactKeys(in, "id", "path", "sha256"); e != nil {
			return r, e
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e = d.Decode(&r); e != nil {
		return r, e
	}
	if e = d.Decode(new(any)); e != io.EOF {
		return r, fmt.Errorf("one request document required")
	}
	if r.Schema != "adaptive-bootstrap-request-v1" || r.StudyHead != StudyHead || len(r.Inputs) != 12 {
		return r, fmt.Errorf("exact reviewed development input set required")
	}
	return r, nil
}

type receipt struct {
	Schema     string `json:"schema"`
	Profile    string `json:"profile"`
	Status     string `json:"status"`
	SourceHash string `json:"sourceHash"`
	DataHash   string `json:"dataHash"`
	Warmup     int    `json:"warmup"`
	Bars       int    `json:"bars"`
	Config     struct {
		Candidate string `json:"candidate"`
		Cost      string `json:"cost"`
		Stress    string `json:"stress"`
		StartT    int64  `json:"startT"`
		EndT      int64  `json:"endT"`
	} `json:"config"`
	Equity []Mark `json:"equity"`
}

func ReadStreams(r Request) ([]Stream, error) {
	if r.Schema != "adaptive-bootstrap-request-v1" || r.StudyHead != StudyHead || len(r.Inputs) != 12 {
		return nil, fmt.Errorf("request identity")
	}
	seen := map[string]bool{}
	streams := []Stream{}
	var times []int64
	for _, in := range r.Inputs {
		if seen[in.ID] || !hashRE.MatchString(in.SHA256) {
			return nil, fmt.Errorf("duplicate/invalid input identity")
		}
		seen[in.ID] = true
		f, e := os.Open(in.Path)
		if e != nil {
			return nil, e
		}
		raw, e := io.ReadAll(io.LimitReader(f, 16<<20))
		f.Close()
		if e != nil {
			return nil, e
		}
		h := sha256.Sum256(raw)
		if hex.EncodeToString(h[:]) != in.SHA256 {
			return nil, fmt.Errorf("input hash mismatch %s", in.ID)
		}
		var x receipt
		if e = uniqueJSON(raw); e != nil {
			return nil, e
		}
		if e = exactKeys(raw, "schema", "profile", "status", "config", "sourceHash", "dataHash", "warmup", "bars", "trades", "events", "equity", "terminal", "diagnostics", "metrics", "limitations"); e != nil {
			return nil, e
		}
		var fields map[string]json.RawMessage
		json.Unmarshal(raw, &fields)
		if e = exactKeys(fields["config"], "candidate", "cost", "stress", "startT", "endT"); e != nil {
			return nil, e
		}
		var points []json.RawMessage
		if e = json.Unmarshal(fields["equity"], &points); e != nil {
			return nil, e
		}
		for _, point := range points {
			if e = exactKeys(point, "t", "balance", "unrealized", "equity", "open", "gap"); e != nil {
				return nil, e
			}
		}
		d := json.NewDecoder(bytes.NewReader(raw))
		if e = d.Decode(&x); e != nil {
			return nil, e
		}
		if e = d.Decode(new(any)); e != io.EOF {
			return nil, fmt.Errorf("one receipt document required")
		}
		c := x.Config
		if x.Schema != "adaptive-study-result-v1" || x.Profile != Profile || x.Status != "retrospective-assumed-model-not-Pine-parity" || x.SourceHash != SourceHash || x.DataHash != DataHash || x.Warmup != 500 || x.Bars != len(x.Equity) || c.StartT != 1617235200000 || c.EndT != 1704067200000 || c.Stress != "none" || in.ID != c.Candidate+":"+c.Cost {
			return nil, fmt.Errorf("wrong source/profile/window input %s", in.ID)
		}
		if c.Candidate != "B0" && c.Candidate != "B1" && c.Candidate != "B2" {
			return nil, fmt.Errorf("candidate")
		}
		if c.Cost != "GROSS" && c.Cost != "BASIC" && c.Cost != "HARSH" && c.Cost != "SOURCE_FEE" {
			return nil, fmt.Errorf("cost")
		}
		if len(times) == 0 {
			times = make([]int64, len(x.Equity))
			for n, m := range x.Equity {
				times[n] = m.T
			}
		}
		if len(x.Equity) != len(times) {
			return nil, fmt.Errorf("different mark membership")
		}
		for n, m := range x.Equity {
			if m.T != times[n] || m.T <= c.StartT || m.T > c.EndT || m.T%1800000 != 0 {
				return nil, fmt.Errorf("different/out-of-window marks")
			}
		}
		days, e := Group(x.Equity)
		if e != nil {
			return nil, e
		}
		streams = append(streams, Stream{ID: in.ID, InputHash: in.SHA256, Days: days})
	}
	sort.Slice(streams, func(i, j int) bool { return streams[i].ID < streams[j].ID })
	return streams, nil
}
