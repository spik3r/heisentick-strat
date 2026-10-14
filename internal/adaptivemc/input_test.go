package adaptivemc

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONDuplicatesAliasesAndNullRefused(t *testing.T) {
	for _, raw := range []string{`{"schema":"x","schema":"y","studyHead":"x","inputs":[]}`, `{"Schema":"x","studyHead":"x","inputs":[]}`, `{"schema":null,"studyHead":"x","inputs":[]}`} {
		p := filepath.Join(t.TempDir(), "request.json")
		os.WriteFile(p, []byte(raw), 0600)
		if _, e := ReadRequest(p); e == nil {
			t.Fatal("invalid JSON admitted")
		}
	}
	if e := uniqueJSON(bytes.Repeat([]byte("["), 30)); e == nil {
		t.Fatal("depth")
	}
}

func TestRejectWrongRequestAndMissingReceiptBeforeAnalysis(t *testing.T) {
	p := filepath.Join(t.TempDir(), "request.json")
	if e := os.WriteFile(p, []byte(`{"schema":"wrong","studyHead":"wrong","inputs":[]}`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := ReadRequest(p); e == nil {
		t.Fatal("wrong source/schema")
	}
	if _, e := ReadStreams(Request{Schema: "adaptive-bootstrap-request-v1", StudyHead: "wrong"}); e == nil {
		t.Fatal("wrong head")
	}
	r := Request{Schema: "adaptive-bootstrap-request-v1", StudyHead: StudyHead}
	for i := 0; i < 12; i++ {
		r.Inputs = append(r.Inputs, Input{ID: "B0:GROSS", Path: "missing", SHA256: DataHash})
	}
	raw, _ := json.Marshal(r)
	os.WriteFile(p, raw, 0600)
	parsed, e := ReadRequest(p)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ReadStreams(parsed); e == nil {
		t.Fatal("missing/duplicate inputs")
	}
}

func TestRejectUnhashedOversizedSuffix(t *testing.T) {
	prefix := append([]byte(`{}`), bytes.Repeat([]byte(" "), (16<<20)-2)...)
	h := sha256.Sum256(prefix)
	p := filepath.Join(t.TempDir(), "oversized.json")
	if e := os.WriteFile(p, append(prefix, []byte("INVALID UNHASHED SUFFIX")...), 0600); e != nil {
		t.Fatal(e)
	}
	r := Request{Schema: "adaptive-bootstrap-request-v1", StudyHead: StudyHead, Inputs: make([]Input, 12)}
	r.Inputs[0] = Input{ID: "B0:GROSS", Path: p, SHA256: fmt.Sprintf("%x", h)}
	if _, e := ReadStreams(r); e == nil || !strings.Contains(e.Error(), "input size") {
		t.Fatalf("expected oversized refusal, got %v", e)
	}
}
