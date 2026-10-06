package frozeninput

import (
	"fmt"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine/frozenlevels"
)

// Decode is the sole public admission constructor. It revalidates the received
// mutable config, then decodes original bytes. Caller-created DTOs cannot bypass
// the exact-number and raw-price boundary. Failure always returns nil.
func Decode(cfg dsl.Config, documents Documents, artifacts []Artifact) (*Input, error) {
	normalized, err := dsl.NormalizeFrozenLevelConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("input-config: %w", err)
	}
	id, err := dsl.FrozenLevelIdentity(normalized)
	if err != nil {
		return nil, fmt.Errorf("input-config: %w", err)
	}
	if err = checkBudget(id.ConfigBytes, documents, artifacts); err != nil {
		return nil, err
	}
	documents = copyDocuments(documents)
	snapshot, err := dsl.DecodeFrozenQuoteSnapshotJSON(documents.Snapshot)
	if err != nil {
		return nil, fmt.Errorf("input-snapshot: %w", err)
	}
	source, err := dsl.DecodeFrozenQuoteSourceJSON(documents.Source)
	if err != nil {
		return nil, fmt.Errorf("input-source: %w", err)
	}
	ordering, err := dsl.DecodeFrozenQuoteOrderingJSON(documents.Ordering)
	if err != nil {
		return nil, fmt.Errorf("input-ordering: %w", err)
	}
	if configRef(normalized, "source") != documentRef(source.ID, source.Version, documents.Source) || configRef(normalized, "ordering") != documentRef(ordering.ID, ordering.Version, documents.Ordering) {
		return nil, fmt.Errorf("input-identity: compiled source or ordering reference mismatch")
	}
	if source.SnapshotRef != documentRef(snapshot.ID, snapshot.Version, documents.Snapshot) || source.NormalizedSnapshotSHA256 != digest(snapshot.NormalizedBytes) {
		return nil, fmt.Errorf("input-identity: raw or normalized snapshot reference mismatch")
	}
	if ordering.SourceRef != configRef(normalized, "source") || ordering.SnapshotRef != source.SnapshotRef || ordering.CalendarRef != configRef(normalized, "calendar") {
		return nil, fmt.Errorf("input-identity: ordering source/snapshot/calendar reference mismatch")
	}
	if source.Dataset != snapshot.Dataset || source.Symbol != snapshot.Symbol || source.PriceUnits != snapshot.PriceUnits {
		return nil, fmt.Errorf("input-identity: snapshot/source dataset or units mismatch")
	}
	combined := dsl.FrozenQuoteInterval{StartMS: source.Windows.Warmup.StartMS, EndMS: source.Windows.Evaluation.EndMS}
	if source.Windows.Warmup.StartMS > source.Windows.Warmup.EndMS || source.Windows.Warmup.EndMS != source.Windows.Evaluation.StartMS || source.Windows.Evaluation.StartMS >= source.Windows.Evaluation.EndMS {
		return nil, fmt.Errorf("input-windows: warmup/evaluation intervals do not join")
	}
	if int64(len(snapshot.Rows)) != source.RawArtifact.RecordCount {
		return nil, fmt.Errorf("input-artifact: declared record count mismatch")
	}
	leafs, err := bindArtifacts(source, combined, artifacts)
	if err != nil {
		return nil, err
	}
	if err = checkQuotes(snapshot, source, ordering, combined); err != nil {
		return nil, err
	}
	if err = checkCoverage(source, combined, snapshot.Rows, leafs); err != nil {
		return nil, err
	}
	if err = checkOrdering(ordering, combined, leafs); err != nil {
		return nil, err
	}
	// Nothing supplied by the caller is retained: decoder values are fresh;
	// all raw leaves are copied, and the config is an independent normalized map.
	out := &Input{config: normalized, identity: id, documents: documents, snapshot: snapshot, source: source, ordering: ordering, admitted: true}
	out.artifacts = make([]Artifact, len(artifacts))
	for i, a := range artifacts {
		out.artifacts[i] = Artifact{a.Ref, copyBytes(a.Bytes)}
	}
	out.primitiveSource = frozenlevels.Source{Provider: source.Provider, Dataset: source.Dataset, SHA256: configRef(normalized, "source").SHA256, Side: frozenlevels.Bid}
	return out, nil
}

type declaredLeaf struct {
	ref   dsl.FrozenQuoteRef
	kind  string
	scope dsl.FrozenQuoteInterval
}

func contains(outer, inner dsl.FrozenQuoteInterval) bool {
	return outer.StartMS <= inner.StartMS && inner.StartMS < inner.EndMS && inner.EndMS <= outer.EndMS
}
func bindArtifacts(source dsl.FrozenQuoteSource, combined dsl.FrozenQuoteInterval, artifacts []Artifact) (map[string]declaredLeaf, error) {
	if len(artifacts) != 1+len(source.SupportingArtifacts) {
		return nil, fmt.Errorf("input-artifact: missing or extra artifact")
	}
	declared := make(map[string]declaredLeaf, 1+len(source.SupportingArtifacts))
	declared[artifactKey(source.RawArtifact.Ref)] = declaredLeaf{source.RawArtifact.Ref, "raw", combined}
	for _, a := range source.SupportingArtifacts {
		if a.Scope.StartMS >= a.Scope.EndMS {
			return nil, fmt.Errorf("input-artifact: supporting scope must have positive width")
		}
		k := artifactKey(a.Ref)
		if _, exists := declared[k]; exists {
			return nil, fmt.Errorf("input-artifact: duplicate declared ID/version")
		}
		declared[k] = declaredLeaf{a.Ref, a.Kind, a.Scope}
	}
	seen := make(map[string]bool, len(artifacts))
	for _, a := range artifacts {
		k := artifactKey(a.Ref)
		d, ok := declared[k]
		if !ok || d.ref != a.Ref || seen[k] {
			return nil, fmt.Errorf("input-artifact: undeclared, mismatched or duplicate leaf")
		}
		seen[k] = true
		if digest(a.Bytes) != a.Ref.SHA256 {
			return nil, fmt.Errorf("input-artifact: raw byte digest mismatch")
		}
		if d.kind == "raw" && int64(len(a.Bytes)) != source.RawArtifact.ByteCount {
			return nil, fmt.Errorf("input-artifact: declared byte count mismatch")
		}
	}
	return declared, nil
}
func checkQuotes(s dsl.FrozenQuoteSnapshot, source dsl.FrozenQuoteSource, o dsl.FrozenQuoteOrdering, combined dsl.FrozenQuoteInterval) error {
	if o.Sequence.Method != s.SequenceMethod || o.Sequence.RawArtifactRef != source.RawArtifact.Ref || o.Sequence.Decoder != source.Decoder {
		return fmt.Errorf("input-sequence: method, raw artifact or decoder mismatch")
	}
	for i, row := range s.Rows {
		if row.AvailableMS != row.EventMS {
			return fmt.Errorf("input-availability: event and availability must agree")
		}
		if row.EventMS < combined.StartMS || row.EventMS >= combined.EndMS {
			return fmt.Errorf("input-windows: quote outside half-open scope")
		}
		if i > 0 {
			prev := s.Rows[i-1]
			if row.EventMS < prev.EventMS || row.EventMS == prev.EventMS && row.Sequence <= prev.Sequence {
				return fmt.Errorf("input-sequence: duplicate or out-of-order event key")
			}
			if o.Sequence.CounterPolicy == "global-strict" && row.Sequence <= prev.Sequence {
				return fmt.Errorf("input-sequence: provider counter is not globally increasing")
			}
		}
		if s.SequenceMethod == "artifact-record-ordinal" && (row.Sequence != int64(i) || row.SourceRecord == nil || row.SourceRecord.RecordIndex != int64(i) || row.SourceRecord.ArtifactID != source.RawArtifact.Ref.ID || row.SourceRecord.ArtifactVersion != source.RawArtifact.Ref.Version) {
			return fmt.Errorf("input-sequence: ordinal or locator mismatch")
		}
	}
	first, last := s.Rows[0], s.Rows[len(s.Rows)-1]
	if o.Scope.StartMS != combined.StartMS || o.Scope.EndMS != combined.EndMS || o.Scope.RowCount != int64(len(s.Rows)) || o.Scope.FirstEvent != (dsl.FrozenQuoteEvent{EventMS: first.EventMS, Sequence: first.Sequence}) || o.Scope.LastEvent != (dsl.FrozenQuoteEvent{EventMS: last.EventMS, Sequence: last.Sequence}) {
		return fmt.Errorf("input-sequence: ordering scope/count/event bounds mismatch")
	}
	return nil
}
func checkEvidence(refs []dsl.FrozenQuoteRef, scope dsl.FrozenQuoteInterval, leafs map[string]declaredLeaf, kinds ...string) error {
	for _, ref := range refs {
		leaf, ok := leafs[artifactKey(ref)]
		if !ok || leaf.ref != ref || !contains(leaf.scope, scope) {
			return fmt.Errorf("input-evidence: unresolved reference or insufficient scope")
		}
		if len(kinds) > 0 {
			allowed := false
			for _, kind := range kinds {
				if leaf.kind == kind {
					allowed = true
				}
			}
			if !allowed {
				return fmt.Errorf("input-evidence: incompatible artifact kind")
			}
		}
	}
	return nil
}
func checkCoverage(source dsl.FrozenQuoteSource, combined dsl.FrozenQuoteInterval, rows []dsl.FrozenQuoteRow, leafs map[string]declaredLeaf) error {
	next := combined.StartMS
	rowIndex := 0
	for _, segment := range source.CoverageSegments {
		interval := dsl.FrozenQuoteInterval{StartMS: segment.StartMS, EndMS: segment.EndMS}
		if segment.StartMS != next || !contains(combined, interval) {
			return fmt.Errorf("input-coverage: segments must partition the combined scope")
		}
		next = segment.EndMS
		var kinds []string
		switch segment.Status {
		case "complete-claimed":
			kinds = []string{"raw", "coverage"}
		case "gap":
			kinds = []string{"coverage"}
		case "closure":
			kinds = []string{"closure"}
		}
		if err := checkEvidence(segment.EvidenceRefs, interval, leafs, kinds...); err != nil {
			return err
		}
		for rowIndex < len(rows) && rows[rowIndex].EventMS < segment.EndMS {
			if segment.Status == "gap" || segment.Status == "closure" {
				return fmt.Errorf("input-coverage: quote inside declared gap or closure")
			}
			rowIndex++
		}
	}
	if next != combined.EndMS {
		return fmt.Errorf("input-coverage: incomplete partition")
	}
	return nil
}
func checkOrdering(o dsl.FrozenQuoteOrdering, combined dsl.FrozenQuoteInterval, leafs map[string]declaredLeaf) error {
	if err := checkEvidence(o.Sequence.EvidenceRefs, combined, leafs, "raw", "ordering"); err != nil {
		return err
	}
	seen := make(map[string]bool, len(o.ContextBindings))
	for _, context := range o.ContextBindings {
		if seen[context.WindowID] || !contains(combined, context.Interval) || context.SourceRef != o.SourceRef || context.CalendarRef != o.CalendarRef {
			return fmt.Errorf("input-context: duplicate, out-of-scope or mismatched context")
		}
		seen[context.WindowID] = true
		if err := checkEvidence(context.EvidenceRefs, context.Interval, leafs); err != nil {
			return err
		}
	}
	return nil
}
