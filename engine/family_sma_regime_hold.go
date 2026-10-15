package engine

func (b *broker) onSMARegimeHoldBar(i int) {
	period := b.params.SMARegimeHold.Period
	average, ready := simpleMovingAverage(b.series, i, period)
	if !ready {
		return
	}
	if b.hasPosition {
		if b.series.C[i] <= average {
			b.pendingExits = append(b.pendingExits, pendingExit{
				PositionEntryIndex: b.position.EntryIndex,
				Index:              i + 1,
				Rule:               "sma-regime-below-or-equal",
			})
		}
		return
	}
	if b.series.C[i] <= average {
		return
	}
	entry := order{
		Side:     sideLong,
		Size:     1,
		HasSize:  true,
		NoStop:   true,
		NoTarget: true,
		Tag:      "DSL-SMA-REGIME",
		Index:    i + 1,
		Meta: TradeMeta{
			"setup":       "smaRegimeHold",
			"signalIndex": float64(i),
			"smaPeriod":   float64(period),
			"signalSMA":   average,
		},
	}
	if b.captureEntries {
		b.capturedEntries = append(b.capturedEntries, entry)
		return
	}
	b.pendingOrders = append(b.pendingOrders, entry)
}
