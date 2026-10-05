package marketdata

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
)

func TestWindowPayloadPreservesOriginalAndSelectedIdentities(t *testing.T) {
	request := []byte(`{"schema":"market-data-window-request-v1","format":"json","fromMs":2,"toMs":2}`)
	payload := []byte(`{"bars":[[1,10,12,9,11,0],[2,11,13,10,12,1]],"opaque":{"provider":"invented"}}`)
	result, err := WindowPayload(request, payload)
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceBars != 2 || len(result.Bars) != 1 || result.Bars[0][0] != 2 || result.PayloadSHA256 != windowDigest(payload) || result.RequestSHA256 != windowDigest(request) {
		t.Fatalf("identity/window: %+v", result)
	}
	changed, err := WindowPayload(request, append(payload, ' '))
	if err != nil {
		t.Fatal(err)
	}
	if changed.PayloadSHA256 == result.PayloadSHA256 || changed.RowsSHA256 != result.RowsSHA256 {
		t.Fatal("original bytes were confused with selected rows")
	}
}

func TestWindowPayloadRejectsUnrepairedJSON(t *testing.T) {
	request := []byte(`{"schema":"market-data-window-request-v1","format":"json","fromMs":2,"toMs":null}`)
	for _, payload := range []string{
		`{"bars":[[1,10,12,9,11]]}`, `{"bars":[[1,10,12,9,11,1,99]]}`,
		`{"bars":[[1,10,12,9,11,null]]}`, `{"bars":[[1,10,12,9,11,"1"]]}`,
		`{"bars":[[1,10,12,9,11,false]]}`, `{"bars":[[1,10,12,9,11,1e999]]}`,
		`{"bars":[[1,10,12,9,11,-1]]}`, `{"bars":[[1,10,9,9,11,1]]}`,
		`{"bars":[[2,10,12,9,11,1],[1,10,12,9,11,1]]}`,
		`{"bars":[[1,10,12,9,11,1],[1,10,12,9,11,1]]}`,
		`{"bars":[[1.5,10,12,9,11,1]]}`, `{"bars":[{"t":1}]}`,
		`{"bars":null}`, `{"bars":[] ,"bars":[]}`, `{"bars":[]} {}`,
	} {
		t.Run(payload, func(t *testing.T) {
			if _, err := WindowPayload(request, []byte(payload)); err == nil {
				t.Fatal("malformed original accepted, including outside selected window")
			}
		})
	}
}

func TestWindowPayloadStrictBinaryNoPaddingOrTrailingBytes(t *testing.T) {
	request := []byte(`{"schema":"market-data-window-request-v1","format":"btb1","fromMs":null,"toMs":null}`)
	payload := EncodeBTB1(SeriesFromBars([]Bar{{T: 1, O: 10, H: 12, L: 9, C: 11, V: 0}}))
	result, err := WindowPayload(request, payload)
	if err != nil || len(result.Bars) != 1 || result.Bars[0][5] != 0 {
		t.Fatalf("valid binary: %+v %v", result, err)
	}
	for _, columnCount := range []uint32{5, 7} {
		changed := append([]byte{}, payload...)
		binary.LittleEndian.PutUint32(changed[12:16], columnCount)
		if _, err := WindowPayload(request, changed); err == nil {
			t.Fatal("non-six-column binary accepted")
		}
	}
	for _, changed := range [][]byte{payload[:len(payload)-1], append(append([]byte{}, payload...), 0)} {
		if _, err := WindowPayload(request, changed); err == nil {
			t.Fatal("nonexact binary length accepted")
		}
	}
}

func TestWindowPayloadRequestAdmission(t *testing.T) {
	payload := []byte(`{"bars":[]}`)
	for _, request := range []string{`{}`, `{"schema":"market-data-window-request-v1","format":"json","fromMs":null,"toMs":null,"extra":1}`, `{"schema":"market-data-window-request-v1","format":"json","fromMs":null,"toMs":null,"fromMs":2}`, `{"schema":"market-data-window-request-v1","format":"json","fromMs":3,"toMs":2}`, `{"schema":"market-data-window-request-v1","format":"json","fromMs":"2","toMs":null}`} {
		if _, err := WindowPayload([]byte(request), payload); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
	request, _ := json.Marshal(WindowPayloadRequest{Schema: WindowPayloadRequestSchema, Format: "json"})
	if _, err := WindowPayload(request, []byte(strings.Repeat(" ", MaxWindowPayloadBytes+1))); err == nil {
		t.Fatal("unbounded payload accepted")
	}
}
