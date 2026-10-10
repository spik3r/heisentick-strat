package seqcore

// ResponseWindowPolicy is a read-only eligibility helper, not a trade or fill policy.
type ResponseWindowPolicy struct {
	SetupBars     int64
	CountdownBars int64
}

func DefaultResponseWindowPolicy() ResponseWindowPolicy { return ResponseWindowPolicy{4, 12} }
func (p ResponseWindowPolicy) Eligible(kind string, age int64) bool {
	if age < 0 {
		return false
	}
	switch kind {
	case "setup", "setup_complete":
		return age <= p.SetupBars
	case "countdown", "countdown_complete":
		return age <= p.CountdownBars
	default:
		return false
	}
}
