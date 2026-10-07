package dsl

import (
	"encoding/json"
	"fmt"
)

// The family-local reader retains exact tokens and rejects duplicate decoded
// keys and invalid Unicode before these field decoders see any value.
type frozenInputReader struct{ err error }

func (r *frozenInputReader) fail(err error) {
	if r.err == nil {
		r.err = err
	}
}
func (r *frozenInputReader) object(v any, path string, keys ...string) map[string]any {
	m, err := frozenConfigObject(v, path)
	r.fail(err)
	if err == nil {
		r.fail(frozenExactKeys(m, path, keys...))
	}
	return m
}
func (r *frozenInputReader) array(v any, path string, minimum int) []any {
	a, ok := v.([]any)
	if !ok || len(a) < minimum {
		r.fail(fmt.Errorf("%s must be an array with at least %d entries", path, minimum))
	}
	return a
}
func (r *frozenInputReader) text(v any, path string, values ...string) string {
	s, err := frozenConfigString(v, path)
	r.fail(err)
	if err == nil && len(values) > 0 {
		found := false
		for _, want := range values {
			if s == want {
				found = true
			}
		}
		if !found {
			r.fail(fmt.Errorf("%s has an unsupported value", path))
		}
	}
	return s
}
func (r *frozenInputReader) registry(v any, path string, limit int) string {
	s := r.text(v, path)
	if len(s) < 1 || len(s) > limit {
		r.fail(fmt.Errorf("%s has an invalid registry length", path))
	}
	for i, b := range []byte(s) {
		alpha := b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
		if !alpha && (i == 0 || b != '.' && b != '_' && b != '-') {
			r.fail(fmt.Errorf("%s must be an ASCII registry identity", path))
			break
		}
	}
	return s
}
func (r *frozenInputReader) integer(v any, path string, minimum int64) int64 {
	n, err := frozenConfigInteger(v, path, minimum, frozenLevelMaxSafeInteger)
	r.fail(err)
	return n
}
func (r *frozenInputReader) ref(v any, path string) FrozenQuoteRef {
	x, err := frozenDecodeRef(v, path)
	r.fail(err)
	return FrozenQuoteRef{x.ID, x.Version, x.SHA256}
}
func (r *frozenInputReader) refs(v any, path string, minimum int) []FrozenQuoteRef {
	a := r.array(v, path, minimum)
	out := make([]FrozenQuoteRef, len(a))
	for i, v := range a {
		out[i] = r.ref(v, fmt.Sprintf("%s[%d]", path, i))
	}
	return out
}
func (r *frozenInputReader) interval(v any, path string) FrozenQuoteInterval {
	m := r.object(v, path, "startMS", "endMS")
	return FrozenQuoteInterval{r.integer(m["startMS"], path+".startMS", 0), r.integer(m["endMS"], path+".endMS", 0)}
}
func (r *frozenInputReader) event(v any, path string) FrozenQuoteEvent {
	m := r.object(v, path, "eventMS", "sequence")
	return FrozenQuoteEvent{r.integer(m["eventMS"], path+".eventMS", 0), r.integer(m["sequence"], path+".sequence", 0)}
}
func (r *frozenInputReader) decoder(v any, path string) FrozenQuoteDecoder {
	m := r.object(v, path, "id", "version", "method")
	return FrozenQuoteDecoder{r.registry(m["id"], path+".id", 64), r.registry(m["version"], path+".version", 32), r.text(m["method"], path+".method", "preserve-all-records-in-original-order-without-sort-repair-or-drop")}
}

// Compare positive decimal coefficients and exponents before float conversion.
// Work is bounded by token length, with no allocation proportional to exponent.
func frozenInputPriceParts(v any, path string) (frozenDecimalParts, error) {
	n, ok := v.(json.Number)
	if !ok {
		return frozenDecimalParts{}, fmt.Errorf("%s must be a number", path)
	}
	p, err := frozenNumberParts(string(n))
	if err != nil {
		return p, err
	}
	if p.negative || p.digits == "" {
		return p, fmt.Errorf("%s must be positive", path)
	}
	return p, nil
}
func frozenInputDecimalAfter(a, b frozenDecimalParts) bool {
	ad, bd := len(a.digits)+a.scale, len(b.digits)+b.scale
	if ad != bd {
		return ad > bd
	}
	n := len(a.digits)
	if len(b.digits) > n {
		n = len(b.digits)
	}
	for i := 0; i < n; i++ {
		x, y := byte('0'), byte('0')
		if i < len(a.digits) {
			x = a.digits[i]
		}
		if i < len(b.digits) {
			y = b.digits[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// DecodeFrozenQuoteSnapshotJSON validates original JSON and normalizes prices
// only after checking exact raw bid <= ask. Its DTO grants no qualification.
func DecodeFrozenQuoteSnapshotJSON(raw []byte) (FrozenQuoteSnapshot, error) {
	v, err := strictFrozenJSON(raw)
	if err != nil {
		return FrozenQuoteSnapshot{}, err
	}
	r := frozenInputReader{}
	m := r.object(v, "snapshot", "schema", "id", "version", "dataset", "symbol", "priceUnits", "sides", "sequenceMethod", "rows")
	s := FrozenQuoteSnapshot{
		Schema: r.text(m["schema"], "snapshot.schema", "frozen-quote-snapshot-v1"), ID: r.registry(m["id"], "snapshot.id", 64), Version: r.registry(m["version"], "snapshot.version", 32),
		Dataset: r.registry(m["dataset"], "snapshot.dataset", 64), Symbol: r.text(m["symbol"], "snapshot.symbol", "XAUUSD"), PriceUnits: r.text(m["priceUnits"], "snapshot.priceUnits", "USD-per-troy-ounce"),
		Sides: r.text(m["sides"], "snapshot.sides", "paired-bid-ask"), SequenceMethod: r.text(m["sequenceMethod"], "snapshot.sequenceMethod", "artifact-record-ordinal", "provider-value"),
	}
	a := r.array(m["rows"], "snapshot.rows", 1)
	s.Rows = make([]FrozenQuoteRow, len(a))
	rows := make([]any, len(a))
	for i, v := range a {
		p := fmt.Sprintf("snapshot.rows[%d]", i)
		keys := []string{"eventMS", "sequence", "availableMS", "bid", "ask"}
		if s.SequenceMethod == "artifact-record-ordinal" {
			keys = append(keys, "sourceRecord")
		}
		x := r.object(v, p, keys...)
		row := FrozenQuoteRow{EventMS: r.integer(x["eventMS"], p+".eventMS", 0), Sequence: r.integer(x["sequence"], p+".sequence", 0), AvailableMS: r.integer(x["availableMS"], p+".availableMS", 0)}
		bid, e1 := frozenInputPriceParts(x["bid"], p+".bid")
		ask, e2 := frozenInputPriceParts(x["ask"], p+".ask")
		r.fail(e1)
		r.fail(e2)
		if e1 == nil && e2 == nil && frozenInputDecimalAfter(bid, ask) {
			r.fail(fmt.Errorf("%s raw bid exceeds ask", p))
		}
		// Do not let conversion erase the failed original comparison.
		if r.err != nil {
			return FrozenQuoteSnapshot{}, r.err
		}
		row.Bid, err = frozenConfigDecimal(x["bid"], p+".bid")
		r.fail(err)
		row.Ask, err = frozenConfigDecimal(x["ask"], p+".ask")
		r.fail(err)
		projected := map[string]any{"eventMS": row.EventMS, "sequence": row.Sequence, "availableMS": row.AvailableMS, "bid": row.Bid, "ask": row.Ask}
		if s.SequenceMethod == "artifact-record-ordinal" {
			z := r.object(x["sourceRecord"], p+".sourceRecord", "artifactId", "artifactVersion", "recordIndex")
			row.SourceRecord = &FrozenQuoteRecord{r.registry(z["artifactId"], p+".sourceRecord.artifactId", 64), r.registry(z["artifactVersion"], p+".sourceRecord.artifactVersion", 32), r.integer(z["recordIndex"], p+".sourceRecord.recordIndex", 0)}
			projected["sourceRecord"] = map[string]any{"artifactId": row.SourceRecord.ArtifactID, "artifactVersion": row.SourceRecord.ArtifactVersion, "recordIndex": row.SourceRecord.RecordIndex}
		}
		s.Rows[i] = row
		rows[i] = projected
	}
	if r.err != nil {
		return FrozenQuoteSnapshot{}, r.err
	}
	s.NormalizedBytes, err = json.Marshal(map[string]any{"schema": s.Schema, "id": s.ID, "version": s.Version, "dataset": s.Dataset, "symbol": s.Symbol, "priceUnits": s.PriceUnits, "sides": s.Sides, "sequenceMethod": s.SequenceMethod, "rows": rows})
	if err != nil {
		return FrozenQuoteSnapshot{}, err
	}
	return s, nil
}

// DecodeFrozenQuoteSourceJSON validates declaration shape. Cross-document and
// evidence-scope checks belong to the original-byte admission constructor.
func DecodeFrozenQuoteSourceJSON(raw []byte) (FrozenQuoteSource, error) {
	v, err := strictFrozenJSON(raw)
	if err != nil {
		return FrozenQuoteSource{}, err
	}
	r := frozenInputReader{}
	m := r.object(v, "source", "schema", "id", "version", "provider", "dataset", "symbol", "priceUnits", "snapshotRef", "normalizedSnapshotSHA256", "rawArtifact", "supportingArtifacts", "decoder", "normalizationVersion", "windows", "coverageSegments")
	s := FrozenQuoteSource{Schema: r.text(m["schema"], "source.schema", "frozen-quote-source-v1"), ID: r.registry(m["id"], "source.id", 64), Version: r.registry(m["version"], "source.version", 32), Provider: r.registry(m["provider"], "source.provider", 64), Dataset: r.registry(m["dataset"], "source.dataset", 64), Symbol: r.text(m["symbol"], "source.symbol", "XAUUSD"), PriceUnits: r.text(m["priceUnits"], "source.priceUnits", "USD-per-troy-ounce"), SnapshotRef: r.ref(m["snapshotRef"], "source.snapshotRef"), Decoder: r.decoder(m["decoder"], "source.decoder"), NormalizationVersion: r.text(m["normalizationVersion"], "source.normalizationVersion", "frozen-quote-snapshot-normalization-v1")}
	// Reuse the exact digest grammar without adding a second implementation.
	s.NormalizedSnapshotSHA256 = r.ref(map[string]any{"id": "NormalizedSnapshot", "version": "v1", "sha256": m["normalizedSnapshotSHA256"]}, "source.normalizedSnapshotSHA256").SHA256
	x := r.object(m["rawArtifact"], "source.rawArtifact", "ref", "format", "byteCount", "recordCount")
	s.RawArtifact = FrozenQuoteRawArtifact{r.ref(x["ref"], "source.rawArtifact.ref"), r.registry(x["format"], "source.rawArtifact.format", 64), r.integer(x["byteCount"], "source.rawArtifact.byteCount", 1), r.integer(x["recordCount"], "source.rawArtifact.recordCount", 1)}
	a := r.array(m["supportingArtifacts"], "source.supportingArtifacts", 0)
	s.SupportingArtifacts = make([]FrozenQuoteSupportingArtifact, len(a))
	for i, v := range a {
		p := fmt.Sprintf("source.supportingArtifacts[%d]", i)
		x := r.object(v, p, "ref", "kind", "scope")
		s.SupportingArtifacts[i] = FrozenQuoteSupportingArtifact{r.ref(x["ref"], p+".ref"), r.text(x["kind"], p+".kind", "coverage", "closure", "ordering"), r.interval(x["scope"], p+".scope")}
	}
	x = r.object(m["windows"], "source.windows", "warmup", "evaluation")
	s.Windows = FrozenQuoteWindows{r.interval(x["warmup"], "source.windows.warmup"), r.interval(x["evaluation"], "source.windows.evaluation")}
	a = r.array(m["coverageSegments"], "source.coverageSegments", 1)
	s.CoverageSegments = make([]FrozenQuoteCoverage, len(a))
	for i, v := range a {
		p := fmt.Sprintf("source.coverageSegments[%d]", i)
		x := r.object(v, p, "startMS", "endMS", "status", "evidenceRefs")
		status := r.text(x["status"], p+".status", "complete-claimed", "gap", "closure", "unknown")
		minimum := 1
		if status == "unknown" {
			minimum = 0
		}
		s.CoverageSegments[i] = FrozenQuoteCoverage{r.integer(x["startMS"], p+".startMS", 0), r.integer(x["endMS"], p+".endMS", 0), status, r.refs(x["evidenceRefs"], p+".evidenceRefs", minimum)}
	}
	if r.err != nil {
		return FrozenQuoteSource{}, r.err
	}
	return s, nil
}

// DecodeFrozenQuoteOrderingJSON validates explicit ordering declarations. A
// matching label or counter does not establish original provider ordering.
func DecodeFrozenQuoteOrderingJSON(raw []byte) (FrozenQuoteOrdering, error) {
	v, err := strictFrozenJSON(raw)
	if err != nil {
		return FrozenQuoteOrdering{}, err
	}
	r := frozenInputReader{}
	m := r.object(v, "ordering", "schema", "id", "version", "sourceRef", "snapshotRef", "calendarRef", "sequence", "scope", "contextBindings", "availabilityModel")
	o := FrozenQuoteOrdering{Schema: r.text(m["schema"], "ordering.schema", "frozen-quote-ordering-v1"), ID: r.registry(m["id"], "ordering.id", 64), Version: r.registry(m["version"], "ordering.version", 32), SourceRef: r.ref(m["sourceRef"], "ordering.sourceRef"), SnapshotRef: r.ref(m["snapshotRef"], "ordering.snapshotRef"), CalendarRef: r.ref(m["calendarRef"], "ordering.calendarRef"), AvailabilityModel: r.text(m["availabilityModel"], "ordering.availabilityModel", "event-time-reference-assumption")}
	x := r.object(m["sequence"], "ordering.sequence", "method", "counterPolicy", "domainId", "rawArtifactRef", "decoder", "evidenceState", "evidenceRefs")
	q := FrozenQuoteSequence{Method: r.text(x["method"], "ordering.sequence.method", "artifact-record-ordinal", "provider-value"), DomainID: r.registry(x["domainId"], "ordering.sequence.domainId", 64), RawArtifactRef: r.ref(x["rawArtifactRef"], "ordering.sequence.rawArtifactRef"), Decoder: r.decoder(x["decoder"], "ordering.sequence.decoder"), EvidenceState: r.text(x["evidenceState"], "ordering.sequence.evidenceState", "asserted", "unqualified")}
	policies := []string{"global-strict", "per-event-millisecond"}
	if q.Method == "artifact-record-ordinal" {
		policies = []string{"zero-based-data-record-index"}
	}
	q.CounterPolicy = r.text(x["counterPolicy"], "ordering.sequence.counterPolicy", policies...)
	minimum := 0
	if q.EvidenceState == "asserted" {
		minimum = 1
	}
	q.EvidenceRefs = r.refs(x["evidenceRefs"], "ordering.sequence.evidenceRefs", minimum)
	o.Sequence = q
	x = r.object(m["scope"], "ordering.scope", "startMS", "endMS", "rowCount", "firstEvent", "lastEvent")
	o.Scope = FrozenQuoteScope{r.integer(x["startMS"], "ordering.scope.startMS", 0), r.integer(x["endMS"], "ordering.scope.endMS", 0), r.integer(x["rowCount"], "ordering.scope.rowCount", 1), r.event(x["firstEvent"], "ordering.scope.firstEvent"), r.event(x["lastEvent"], "ordering.scope.lastEvent")}
	a := r.array(m["contextBindings"], "ordering.contextBindings", 0)
	o.ContextBindings = make([]FrozenQuoteContext, len(a))
	for i, v := range a {
		p := fmt.Sprintf("ordering.contextBindings[%d]", i)
		x := r.object(v, p, "windowId", "kind", "sourceRef", "calendarRef", "sourceSide", "interval", "evidenceState", "evidenceRefs")
		o.ContextBindings[i] = FrozenQuoteContext{r.registry(x["windowId"], p+".windowId", 64), r.text(x["kind"], p+".kind", "stored-clock-M30", "rolling-six-M5", "completed-clock-M15", "prior-broker-opening"), r.ref(x["sourceRef"], p+".sourceRef"), r.ref(x["calendarRef"], p+".calendarRef"), r.text(x["sourceSide"], p+".sourceSide", "bid"), r.interval(x["interval"], p+".interval"), r.text(x["evidenceState"], p+".evidenceState", "unqualified"), r.refs(x["evidenceRefs"], p+".evidenceRefs", 0)}
	}
	if r.err != nil {
		return FrozenQuoteOrdering{}, r.err
	}
	return o, nil
}
