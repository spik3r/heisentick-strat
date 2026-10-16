package engine

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// Includes the admitted record ceiling and unique quote identities, rather
// than repeatedly replaying an unrealistically tiny duplicate observation.
func BenchmarkPaperBidAskBoundedPrefix(b *testing.B) {
	r := dualPaperRequest(b)
	at, _ := bidAskTime(r.ActivationFromT)
	for len(r.Records) < PaperBidAskMaxRecords {
		q := bidAskTestQuote(r, at+int64(len(r.Records))*60000000000, "119", "119.2", "")
		q.RequestStartT = q.ProviderT
		q.ReceivedT = q.ProviderT
		q.QualifiedT = q.ProviderT
		r.Records = append(r.Records, BidAskRecord{Sequence: strconv.Itoa(len(r.Records) + 1), Epoch: r.Identity.Epoch, CommitT: q.ProviderT, Kind: "quote", Quote: &q})
	}
	raw, err := json.Marshal(r)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportMetric(float64(len(raw)), "request-bytes")
	for i := 0; i < b.N; i++ {
		request, err := DecodePaperBidAskRequest(raw)
		if err != nil {
			b.Fatal(err)
		}
		result, err := RunPaperBidAsk(request)
		if err != nil {
			b.Fatal(err)
		}
		encoded, err := EncodePaperBidAskResult(result)
		if err != nil {
			b.Fatal(err)
		}
		if result.ProcessedRecords != PaperBidAskMaxRecords {
			b.Fatal("incomplete replay")
		}
		b.ReportMetric(float64(len(encoded)), "output-bytes")
	}
}

func TestPaperBidAskExpandedOutputAndEventBounds(t *testing.T) {
	x := &bidAskRunner{events: make([]BidAskEvent, PaperBidAskMaxEvents)}
	x.emit(BidAskRecord{Sequence: "1"}, BidAskEvent{Kind: "mark"})
	if x.err == nil || x.err.Error() != "PAPER_BIDASK_EVENT_LIMIT" {
		t.Fatal("event ceiling not enforced")
	}
	x.events = nil
	raw := PaperBidAskResult{Coverage: strings.Repeat("x", PaperBidAskMaxOutputBytes)}
	if _, err := EncodePaperBidAskResult(raw); err == nil || err.Error() != "PAPER_BIDASK_OUTPUT_LIMIT" {
		t.Fatal("output ceiling not enforced")
	}
}
