package engine

// TimedReturnCalendar supplies reference dates separately from quoted execution.
// Identities are caller claims; the engine verifies structure, not source authority.
type TimedReturnCalendar struct {
	Schema                string                  `json:"schema"`
	Timezone              string                  `json:"timezone"`
	ReferenceSourceSHA256 string                  `json:"referenceSourceSha256"`
	ExecutionSourceSHA256 string                  `json:"executionSourceSha256"`
	TimezoneDataSHA256    string                  `json:"timezoneDataSha256"`
	TimezoneData          []byte                  `json:"timezoneData"`
	TradeFromDate         string                  `json:"tradeFromDate"`
	TradeToDateExclusive  string                  `json:"tradeToDateExclusive"`
	Sessions              []TimedReferenceSession `json:"sessions"`
	QuotedIntervals       []TimedQuotedInterval   `json:"quotedIntervals"`
}

type TimedReferenceSession struct {
	Date                string `json:"date"`
	Kind                string `json:"kind"`
	Open                string `json:"open,omitempty"`
	Close               string `json:"close,omitempty"`
	PreviousSessionDate string `json:"previousSessionDate,omitempty"`
}

type TimedQuotedInterval struct {
	FromT int64 `json:"fromT"`
	ToT   int64 `json:"toT"`
}

// TimedAuditRow preserves non-trade dates as well as executed events.
type TimedAuditRow struct {
	Date      string   `json:"date"`
	Status    string   `json:"status"`
	StartKind string   `json:"startKind,omitempty"`
	EndKind   string   `json:"endKind,omitempty"`
	StartT    int64    `json:"startT,omitempty"`
	EndT      int64    `json:"endT,omitempty"`
	DecisionT int64    `json:"decisionT,omitempty"`
	EntryT    int64    `json:"entryT,omitempty"`
	ExitT     int64    `json:"exitT,omitempty"`
	LogReturn *float64 `json:"logReturn,omitempty"`
}

type TimedReturnAudit struct {
	Schema             string          `json:"schema"`
	CalendarSHA256     string          `json:"calendarSha256"`
	TimezoneDataSHA256 string          `json:"timezoneDataSha256"`
	ExecutionModel     string          `json:"executionModel"`
	QuantityModel      string          `json:"quantityModel"`
	AdmissionModel     string          `json:"admissionModel"`
	Rows               []TimedAuditRow `json:"rows"`
}

type timedPlanRow struct {
	audit                                       TimedAuditRow
	startIndex, endIndex, entryIndex, exitIndex int
	startOpen, endOpen                          bool
}
