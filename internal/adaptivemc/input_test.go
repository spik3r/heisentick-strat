package adaptivemc

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
