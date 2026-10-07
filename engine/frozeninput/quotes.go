package frozeninput

import (
	"fmt"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine/frozenlevels"
)

// Input is immutable admitted structural evidence. The zero value is not
// admitted. There is no run method and no evidence-upgrade setter.
type Input struct {
	admitted        bool
	config          dsl.Config
	identity        dsl.FrozenLevelIdentityResult
	documents       Documents
	artifacts       []Artifact
	snapshot        dsl.FrozenQuoteSnapshot
	source          dsl.FrozenQuoteSource
	ordering        dsl.FrozenQuoteOrdering
	primitiveSource frozenlevels.Source
}

type Observation struct {
	Quote  frozenlevels.Quote  `json:"quote"`
	Source frozenlevels.Source `json:"source"`
}
type Evidence struct {
	Status                string `json:"status"`
	RunReady              bool   `json:"runReady"`
	ContinuityVerified    bool   `json:"continuityVerified"`
	SourceOrderVerified   bool   `json:"sourceOrderVerified"`
	SequenceEvidenceState string `json:"sequenceEvidenceState"`
	CoverageEvidenceState string `json:"coverageEvidenceState"`
	RawDecoderFidelity    string `json:"rawDecoderFidelity"`
	Calendar              string `json:"calendar"`
	PriorOpening          string `json:"priorOpening"`
	CompletedContext      string `json:"completedContext"`
	Schedule              string `json:"schedule"`
	Costs                 string `json:"costs"`
}
type View struct {
	Source           frozenlevels.Source       `json:"source"`
	OrderingProof    string                    `json:"orderingProof"`
	Observations     []Observation             `json:"observations"`
	Evidence         Evidence                  `json:"evidence"`
	CoverageSegments []dsl.FrozenQuoteCoverage `json:"coverageSegments"`
	ContextBindings  []dsl.FrozenQuoteContext  `json:"contextBindings"`
}

func (in *Input) valid() bool { return in != nil && in.admitted }
func (in *Input) Len() int {
	if !in.valid() {
		return 0
	}
	return len(in.snapshot.Rows)
}
func (in *Input) Source() (frozenlevels.Source, bool) {
	if !in.valid() {
		return frozenlevels.Source{}, false
	}
	return in.primitiveSource, true
}
func (in *Input) Observation(index int) (Observation, bool) {
	if !in.valid() || index < 0 || index >= len(in.snapshot.Rows) {
		return Observation{}, false
	}
	r := in.snapshot.Rows[index]
	return Observation{Source: in.primitiveSource, Quote: frozenlevels.Quote{Event: frozenlevels.EventKey{AtMillis: r.EventMS, Sequence: r.Sequence}, AvailableAtMillis: r.AvailableMS, Bid: r.Bid, Ask: r.Ask}}, true
}
func (in *Input) Evidence() Evidence {
	if !in.valid() {
		return Evidence{}
	}
	return Evidence{Status: "structurally-valid-input", SequenceEvidenceState: in.ordering.Sequence.EvidenceState, CoverageEvidenceState: "assertions-preserved", RawDecoderFidelity: "unqualified", Calendar: "unqualified", PriorOpening: "unqualified", CompletedContext: "unqualified", Schedule: "unqualified", Costs: "unqualified"}
}
func (in *Input) OrderingProof() string {
	if !in.valid() {
		return ""
	}
	return "frozen-ordering:" + configRef(in.config, "ordering").SHA256
}
func (in *Input) Identity() dsl.FrozenLevelIdentityResult {
	if !in.valid() {
		return dsl.FrozenLevelIdentityResult{}
	}
	id := in.identity
	id.ConfigBytes = copyBytes(id.ConfigBytes)
	id.PolicyBytes = copyBytes(id.PolicyBytes)
	return id
}

// Config returns a fresh normalized copy rather than a retained map alias.
func (in *Input) Config() (dsl.Config, error) {
	if !in.valid() {
		return nil, fmt.Errorf("input-not-admitted")
	}
	return dsl.NormalizeFrozenLevelConfig(in.config)
}
func (in *Input) Documents() Documents {
	if !in.valid() {
		return Documents{}
	}
	return copyDocuments(in.documents)
}
func (in *Input) Artifacts() []Artifact {
	if !in.valid() {
		return nil
	}
	out := make([]Artifact, len(in.artifacts))
	for i, a := range in.artifacts {
		out[i] = Artifact{Ref: a.Ref, Bytes: copyBytes(a.Bytes)}
	}
	return out
}
func copyRefs(refs []dsl.FrozenQuoteRef) []dsl.FrozenQuoteRef {
	out := make([]dsl.FrozenQuoteRef, len(refs))
	copy(out, refs)
	return out
}
func (in *Input) SnapshotDeclaration() dsl.FrozenQuoteSnapshot {
	if !in.valid() {
		return dsl.FrozenQuoteSnapshot{}
	}
	s := in.snapshot
	s.Rows = make([]dsl.FrozenQuoteRow, len(in.snapshot.Rows))
	copy(s.Rows, in.snapshot.Rows)
	for i, r := range s.Rows {
		if r.SourceRecord != nil {
			record := *r.SourceRecord
			s.Rows[i].SourceRecord = &record
		}
	}
	s.NormalizedBytes = copyBytes(in.snapshot.NormalizedBytes)
	return s
}
func (in *Input) SourceDeclaration() dsl.FrozenQuoteSource {
	if !in.valid() {
		return dsl.FrozenQuoteSource{}
	}
	s := in.source
	s.SupportingArtifacts = make([]dsl.FrozenQuoteSupportingArtifact, len(in.source.SupportingArtifacts))
	copy(s.SupportingArtifacts, in.source.SupportingArtifacts)
	s.CoverageSegments = make([]dsl.FrozenQuoteCoverage, len(in.source.CoverageSegments))
	copy(s.CoverageSegments, in.source.CoverageSegments)
	for i := range s.CoverageSegments {
		s.CoverageSegments[i].EvidenceRefs = copyRefs(s.CoverageSegments[i].EvidenceRefs)
	}
	return s
}
func (in *Input) OrderingDeclaration() dsl.FrozenQuoteOrdering {
	if !in.valid() {
		return dsl.FrozenQuoteOrdering{}
	}
	o := in.ordering
	o.Sequence.EvidenceRefs = copyRefs(o.Sequence.EvidenceRefs)
	o.ContextBindings = make([]dsl.FrozenQuoteContext, len(in.ordering.ContextBindings))
	copy(o.ContextBindings, in.ordering.ContextBindings)
	for i := range o.ContextBindings {
		o.ContextBindings[i].EvidenceRefs = copyRefs(o.ContextBindings[i].EvidenceRefs)
	}
	return o
}

// View returns independent observation/evidence arrays. False qualification
// flags remain false even when all declarations and raw hashes are consistent.
func (in *Input) View() (View, error) {
	if !in.valid() {
		return View{}, fmt.Errorf("input-not-admitted")
	}
	v := View{Source: in.primitiveSource, OrderingProof: in.OrderingProof(), Evidence: in.Evidence(), CoverageSegments: in.SourceDeclaration().CoverageSegments, ContextBindings: in.OrderingDeclaration().ContextBindings, Observations: make([]Observation, in.Len())}
	for i := range v.Observations {
		v.Observations[i], _ = in.Observation(i)
	}
	return v, nil
}
