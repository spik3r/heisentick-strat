package dsl

// These DTOs describe raw input declarations, never run-qualified evidence.
// Decode them from original bytes. They are mutable inspection values and
// cannot be supplied to the admitted-input constructor.
type FrozenQuoteRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

func (r FrozenQuoteRef) ConfigRef() FrozenLevelInputRef {
	return FrozenLevelInputRef{ID: r.ID, Version: r.Version, SHA256: r.SHA256}
}

type FrozenQuoteInterval struct {
	StartMS int64 `json:"startMS"`
	EndMS   int64 `json:"endMS"`
}
type FrozenQuoteEvent struct {
	EventMS  int64 `json:"eventMS"`
	Sequence int64 `json:"sequence"`
}
type FrozenQuoteRecord struct {
	ArtifactID      string `json:"artifactId"`
	ArtifactVersion string `json:"artifactVersion"`
	RecordIndex     int64  `json:"recordIndex"`
}
type FrozenQuoteRow struct {
	EventMS      int64              `json:"eventMS"`
	Sequence     int64              `json:"sequence"`
	AvailableMS  int64              `json:"availableMS"`
	Bid          float64            `json:"bid"`
	Ask          float64            `json:"ask"`
	SourceRecord *FrozenQuoteRecord `json:"sourceRecord,omitempty"`
}
type FrozenQuoteSnapshot struct {
	Schema          string           `json:"schema"`
	ID              string           `json:"id"`
	Version         string           `json:"version"`
	Dataset         string           `json:"dataset"`
	Symbol          string           `json:"symbol"`
	PriceUnits      string           `json:"priceUnits"`
	Sides           string           `json:"sides"`
	SequenceMethod  string           `json:"sequenceMethod"`
	Rows            []FrozenQuoteRow `json:"rows"`
	NormalizedBytes []byte           `json:"-"`
}
type FrozenQuoteDecoder struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Method  string `json:"method"`
}
type FrozenQuoteRawArtifact struct {
	Ref         FrozenQuoteRef `json:"ref"`
	Format      string         `json:"format"`
	ByteCount   int64          `json:"byteCount"`
	RecordCount int64          `json:"recordCount"`
}
type FrozenQuoteSupportingArtifact struct {
	Ref   FrozenQuoteRef      `json:"ref"`
	Kind  string              `json:"kind"`
	Scope FrozenQuoteInterval `json:"scope"`
}
type FrozenQuoteWindows struct {
	Warmup     FrozenQuoteInterval `json:"warmup"`
	Evaluation FrozenQuoteInterval `json:"evaluation"`
}
type FrozenQuoteCoverage struct {
	StartMS      int64            `json:"startMS"`
	EndMS        int64            `json:"endMS"`
	Status       string           `json:"status"`
	EvidenceRefs []FrozenQuoteRef `json:"evidenceRefs"`
}
type FrozenQuoteSource struct {
	Schema                   string                          `json:"schema"`
	ID                       string                          `json:"id"`
	Version                  string                          `json:"version"`
	Provider                 string                          `json:"provider"`
	Dataset                  string                          `json:"dataset"`
	Symbol                   string                          `json:"symbol"`
	PriceUnits               string                          `json:"priceUnits"`
	SnapshotRef              FrozenQuoteRef                  `json:"snapshotRef"`
	NormalizedSnapshotSHA256 string                          `json:"normalizedSnapshotSHA256"`
	RawArtifact              FrozenQuoteRawArtifact          `json:"rawArtifact"`
	SupportingArtifacts      []FrozenQuoteSupportingArtifact `json:"supportingArtifacts"`
	Decoder                  FrozenQuoteDecoder              `json:"decoder"`
	NormalizationVersion     string                          `json:"normalizationVersion"`
	Windows                  FrozenQuoteWindows              `json:"windows"`
	CoverageSegments         []FrozenQuoteCoverage           `json:"coverageSegments"`
}
type FrozenQuoteSequence struct {
	Method         string             `json:"method"`
	CounterPolicy  string             `json:"counterPolicy"`
	DomainID       string             `json:"domainId"`
	RawArtifactRef FrozenQuoteRef     `json:"rawArtifactRef"`
	Decoder        FrozenQuoteDecoder `json:"decoder"`
	EvidenceState  string             `json:"evidenceState"`
	EvidenceRefs   []FrozenQuoteRef   `json:"evidenceRefs"`
}
type FrozenQuoteScope struct {
	StartMS    int64            `json:"startMS"`
	EndMS      int64            `json:"endMS"`
	RowCount   int64            `json:"rowCount"`
	FirstEvent FrozenQuoteEvent `json:"firstEvent"`
	LastEvent  FrozenQuoteEvent `json:"lastEvent"`
}
type FrozenQuoteContext struct {
	WindowID      string              `json:"windowId"`
	Kind          string              `json:"kind"`
	SourceRef     FrozenQuoteRef      `json:"sourceRef"`
	CalendarRef   FrozenQuoteRef      `json:"calendarRef"`
	SourceSide    string              `json:"sourceSide"`
	Interval      FrozenQuoteInterval `json:"interval"`
	EvidenceState string              `json:"evidenceState"`
	EvidenceRefs  []FrozenQuoteRef    `json:"evidenceRefs"`
}
type FrozenQuoteOrdering struct {
	Schema            string               `json:"schema"`
	ID                string               `json:"id"`
	Version           string               `json:"version"`
	SourceRef         FrozenQuoteRef       `json:"sourceRef"`
	SnapshotRef       FrozenQuoteRef       `json:"snapshotRef"`
	CalendarRef       FrozenQuoteRef       `json:"calendarRef"`
	Sequence          FrozenQuoteSequence  `json:"sequence"`
	Scope             FrozenQuoteScope     `json:"scope"`
	ContextBindings   []FrozenQuoteContext `json:"contextBindings"`
	AvailabilityModel string               `json:"availabilityModel"`
}
