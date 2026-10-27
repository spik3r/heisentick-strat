package dsl

// These probes model a proposed consumer contract, not production parser output.
import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

type sourceContractProbe struct {
	Name           string
	Source         string
	CurrentID      string  `json:"currentId"`
	CurrentVersion *string `json:"currentVersion"`
	Envelope       map[string]any
	SchemaValid    bool
	Reason         string
	Positions      [][2]int
	Result         string
	WireNewText    *string `json:"wireNewText"`
	WireNewTextHex *string `json:"wireNewTextHex"`
	NewTextHex     *string `json:"newTextHex"`
}

func TestDiagnosticSourceContractProposal(t *testing.T) {
	schema, path := loadContractSchema(t, "diagnostic-v2.schema.json")
	legacy, legacyPath := loadContractSchema(t, "diagnostic-v1.schema.json")
	data, err := os.ReadFile("testdata/diagnostic-source-contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var probes []sourceContractProbe
	if err := json.Unmarshal(data, &probes); err != nil {
		t.Fatal(err)
	}
	if len(probes) == 0 {
		t.Fatal("empty probe corpus")
	}
	names := map[string]bool{}
	for _, p := range probes {
		t.Run(p.Name, func(t *testing.T) {
			if names[p.Name] {
				t.Fatal("duplicate probe name")
			}
			names[p.Name] = true
			wire, err := json.Marshal(p.Envelope)
			if err != nil {
				t.Fatal(err)
			}
			if p.WireNewText != nil {
				wire = bytes.Replace(wire, []byte(`"newText":"target"`), []byte(`"newText":`+*p.WireNewText), 1)
			}
			if p.WireNewTextHex != nil {
				raw, err := hex.DecodeString(*p.WireNewTextHex)
				if err != nil {
					t.Fatal(err)
				}
				wire = bytes.Replace(wire, []byte(`"newText":"target"`), append([]byte(`"newText":`), raw...), 1)
			}
			decoded, err := probeDecodeEnvelope(wire)
			if err != nil {
				if p.Reason != "wire" {
					t.Fatal(err)
				}
				return
			}
			if p.Reason == "wire" {
				t.Fatal("accepted malformed wire text")
			}
			p.Envelope = decoded
			if p.NewTextHex != nil {
				raw, err := hex.DecodeString(*p.NewTextHex)
				if err != nil {
					t.Fatal(err)
				}
				p.Envelope["edit"].(map[string]any)["newText"] = string(raw)
			}
			err = validateContract(p.Envelope, schema, path, schema, p.Name)
			if (err == nil) != p.SchemaValid {
				t.Fatalf("schema valid=%v: %v", p.SchemaValid, err)
			}
			reason, result := "schema", ""
			if err == nil {
				reason, result = probeSourceEdit(p)
			}
			if reason != p.Reason {
				t.Fatalf("reason=%s, want %s", reason, p.Reason)
			}
			if reason == "ok" && p.Envelope["edit"] != nil && result != p.Result {
				t.Fatalf("replacement=%q, want %q", result, p.Result)
			}
			if reason == "ok" && p.Envelope["edit"] != nil {
				// Preview does not authorize later application against changed text.
				changed := p
				changed.Source += " "
				if why, _ := probeSourceEdit(changed); why != "digest" {
					t.Fatalf("preview/apply text change accepted: %s", why)
				}
				// Even a matching digest cannot authorize malformed source bytes.
				changed.Source = string([]byte{0xff})
				changed.Envelope = cloneContractObject(p.Envelope)
				hash := sha256.Sum256([]byte(changed.Source))
				changed.Envelope["source"].(map[string]any)["sha256"] = hex.EncodeToString(hash[:])
				if why, _ := probeSourceEdit(changed); why != "digest" {
					t.Fatalf("invalid UTF-8 accepted: %s", why)
				}
			}
			if reason == "ok" && p.Envelope["range"] != nil {
				r := p.Envelope["range"].(map[string]any)
				got := [][2]int{probeUTF16Position(p.Source, int(r["startByte"].(float64))), probeUTF16Position(p.Source, int(r["endByte"].(float64)))}
				if !reflect.DeepEqual(got, p.Positions) {
					t.Fatalf("positions=%v, want %v", got, p.Positions)
				}
			}
			// Explicit projection is required; a strict legacy consumer rejects v2.
			if err := validateContract(p.Envelope["diagnostic"], legacy, legacyPath, legacy, p.Name); err != nil {
				t.Fatal(err)
			}
			if err := validateContract(p.Envelope, legacy, legacyPath, legacy, p.Name); err == nil {
				t.Fatal("v1 accepted envelope")
			}
		})
	}
	// Old payloads without a code remain valid and Go's current shape is unchanged.
	old := map[string]any{"severity": "error", "message": "Readable", "line": nil, "col": nil}
	if err := validateContract(old, legacy, legacyPath, legacy, "old"); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(Diagnostic{Severity: DiagnosticError, Message: "Readable"})
	if err != nil {
		t.Fatal(err)
	}
	var current map[string]any
	if err := json.Unmarshal(encoded, &current); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(current, old) {
		t.Fatalf("legacy Go shape changed: %s", encoded)
	}
}

// Test-only semantic gate. Schema validation must precede this function.
func probeSourceEdit(p sourceContractProbe) (string, string) {
	e := p.Envelope
	identity := e["source"].(map[string]any)
	if p.CurrentVersion == nil || identity["version"] == nil || identity["version"] != *p.CurrentVersion || identity["id"] != p.CurrentID {
		return "identity", ""
	}
	digest := sha256.Sum256([]byte(p.Source))
	if !utf8.ValidString(p.Source) || identity["sha256"] != hex.EncodeToString(digest[:]) {
		return "digest", ""
	}
	var start, end int
	if e["range"] != nil {
		r := e["range"].(map[string]any)
		// Bounds in float64 before conversion keep this safe on 32-bit systems too.
		a, b := r["startByte"].(float64), r["endByte"].(float64)
		if a > b || b > float64(len(p.Source)) {
			return "range", ""
		}
		start, end = int(a), int(b)
		for _, n := range []int{start, end} {
			if !utf8.ValidString(p.Source[:n]) || (n > 0 && n < len(p.Source) && p.Source[n-1] == '\r' && p.Source[n] == '\n') {
				return "range", ""
			}
		}
	}
	if e["edit"] == nil {
		if e["range"] == nil {
			return "no-edit", ""
		}
		return "ok", ""
	}
	edit := e["edit"].(map[string]any)
	if !utf8.ValidString(edit["expectedText"].(string)) || !utf8.ValidString(edit["newText"].(string)) {
		return "replacement", ""
	}
	if e["range"] == nil || start == end || !reflect.DeepEqual(edit["range"], e["range"]) || edit["expectedText"] != p.Source[start:end] {
		return "edit", ""
	}
	return "ok", p.Source[:start] + edit["newText"].(string) + p.Source[end:]
}

func probeUTF16Position(source string, offset int) [2]int {
	line, start := 0, 0
	for i := 0; i < offset; i++ {
		if source[i] == '\r' || source[i] == '\n' {
			if source[i] == '\r' && i+1 < offset && source[i+1] == '\n' {
				i++
			}
			line++
			start = i + 1
		}
	}
	return [2]int{line, len(utf16.Encode([]rune(source[start:offset])))}
}

// Demonstrate why decoded-string validation cannot establish wire validity.
func TestDiagnosticSourceWireRepairHazard(t *testing.T) {
	for _, wire := range [][]byte{
		[]byte(`{"newText":"\ud800"}`),
		append(append([]byte(`{"newText":"`), 0xff), []byte(`"}`)...),
	} {
		var lossy map[string]any
		if err := json.Unmarshal(wire, &lossy); err != nil || lossy["newText"] != "\ufffd" {
			t.Fatalf("expected standard decoder repair witness: %v, %v", lossy, err)
		}
		if _, err := probeDecodeEnvelope(wire); err == nil {
			t.Fatal("raw gate accepted lossy replacement")
		}
	}
}

// Validate the raw wire before encoding/json can silently replace malformed text.
// This is a test-only reference gate, not a production transport implementation.
func probeDecodeEnvelope(wire []byte) (map[string]any, error) {
	if !utf8.Valid(wire) || !json.Valid(wire) {
		return nil, fmt.Errorf("invalid JSON/UTF-8")
	}
	inString := false
	for i := 0; i < len(wire); i++ {
		if wire[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || wire[i] != '\\' {
			continue
		}
		i++ // json.Valid guarantees an escape and its full payload are present.
		if wire[i] != 'u' {
			continue
		}
		u, _ := strconv.ParseUint(string(wire[i+1:i+5]), 16, 16)
		i += 4
		if u >= 0xdc00 && u <= 0xdfff {
			return nil, fmt.Errorf("unpaired low surrogate")
		}
		if u < 0xd800 || u > 0xdbff {
			continue
		}
		if i+6 >= len(wire) || wire[i+1] != '\\' || wire[i+2] != 'u' {
			return nil, fmt.Errorf("unpaired high surrogate")
		}
		low, err := strconv.ParseUint(string(wire[i+3:i+7]), 16, 16)
		if err != nil || low < 0xdc00 || low > 0xdfff {
			return nil, fmt.Errorf("invalid surrogate pair")
		}
		i += 6
	}
	var envelope map[string]any
	err := json.Unmarshal(wire, &envelope)
	return envelope, err
}
