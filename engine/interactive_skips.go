package engine

// InteractiveSkipReasonSchema version-binds the code vocabulary and event
// definition in spec/interactive-run-v1.md. Counts are Go entry-gate events,
// not the legacy browser's display strings or every bar without a setup.
const InteractiveSkipReasonSchema = "dsl-skip-reasons-v1"

const (
	skipRMVUnavailable        = "gate.rmv_unavailable"
	skipRMVThreshold          = "gate.rmv_threshold"
	skipUTCWindow             = "gate.utc_window"
	skipSessionWindow         = "gate.session_window"
	skipLocalWeekdayAllow     = "gate.local_weekday_allow"
	skipLocalWeekdayBlock     = "gate.local_weekday_block"
	skipLocalHourAllow        = "gate.local_hour_allow"
	skipLocalHourBlock        = "gate.local_hour_block"
	skipOpenLocation          = "gate.open_location"
	skipSessionPhase          = "gate.session_phase"
	skipPriorDayTypeAllow     = "gate.prior_day_type_allow"
	skipPriorDayTypeBlock     = "gate.prior_day_type_block"
	skipDayRegime             = "gate.day_regime"
	skipMovementER            = "gate.movement_er"
	skipPriorDayRangeMissing  = "gate.prior_day_range_missing"
	skipPriorDayRangeTooSmall = "gate.prior_day_range_too_small"
)

func (b *broker) recordInteractiveSkip(i int, code string) {
	if b.skipCounts == nil || code == "" || b.lastSkipIndex == i {
		return
	}
	b.skipCounts[code]++
	b.lastSkipIndex = i
}
