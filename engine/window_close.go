package engine

// closeExpiredWindowPosition applies a slot-boundary close before bracket
// resolution. The boundary candle is a completed observation for the next
// slot, so its range must not fill the prior slot's stop or target first.
func (b *broker) closeExpiredWindowPosition(i int) {
	if !b.hasPosition {
		return
	}
	slotEnd, ok := numericMetaValue(b.position.Meta["slotEnd"])
	if ok && b.series.T[i] >= slotEnd {
		b.closePosition(b.series.O[i], i, "window-close")
	}
}

func numericMetaValue(value any) (float64, bool) {
	switch n := value.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
