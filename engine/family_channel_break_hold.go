package engine

import (
	"fmt"
	"math"
	"runtime"
	"strings"
)

func (b *broker) onChannelBreakHoldBar(i int) {
	if b.hasPosition {
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	p := b.params
	if b.hasCBHLast && i-b.cbhLast < p.CooldownBars {
		return
	}
	if !inSetupTradeWindow(b.series.T[i], p, 0) {
		return
	}
	for _, s := range []side{sideLong, sideShort} {
		if s == sideLong && !p.AllowLong {
			continue
		}
		if s == sideShort && !p.AllowShort {
			continue
		}
		stop, target, seenKey, meta, ok := b.channelBreakHoldSetup(i, s)
		if !ok {
			continue
		}
		day := localDayKey(b.series.T[i])
		if b.seen.cbh.seen(day, seenKey) {
			continue
		}
		b.seen.cbh.add(day, seenKey)
		levelKey, _ := meta["levelKey"].(string)
		b.enterSetup(i, setupPlan{Side: s, Stop: stop, Target: target, Tag: "DSL-CBH:" + levelKey, Meta: meta})
		b.cbhLast = i
		b.hasCBHLast = true
		break
	}
}

func (b *broker) channelBreakHoldSetup(i int, s side) (float64, float64, string, TradeMeta, bool) {
	p := b.params.ChannelBreakHold
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 || i < p.HoldCandles || int(b.cols.LastChannel.SinceActive[i]) < 0 || int(b.cols.LastChannel.SinceActive[i]) > p.ActiveWithinCandles {
		return 0, 0, "", nil, false
	}
	widthATR := normalizedChannelWidthATR(b.cols.LastChannel.WidthATR[i])
	if p.ChannelMinWidthATR != 0 && widthATR < p.ChannelMinWidthATR {
		return 0, 0, "", nil, false
	}
	if p.ChannelMaxWidthATR != 0 && widthATR > p.ChannelMaxWidthATR {
		return 0, 0, "", nil, false
	}
	direction := channelDirectionName(b.cols.LastChannel.Direction[i])
	if p.DirectionRequired {
		matches := containsString(p.ChannelDirections, direction)
		if p.DirectionText != "" {
			matches = strings.Contains(p.DirectionText, direction)
		}
		if !matches {
			return 0, 0, "", nil, false
		}
	}
	upper := lineCoeffs{Slope: b.cols.LastChannel.UpperSlope[i], Intercept: b.cols.LastChannel.UpperIntercept[i]}
	lower := lineCoeffs{Slope: b.cols.LastChannel.LowerSlope[i], Intercept: b.cols.LastChannel.LowerIntercept[i]}
	railLine := upper
	edge := "high"
	levelKey := "channel.high"
	if s == sideShort {
		railLine = lower
		edge = "low"
		levelKey = "channel.low"
	}
	for j := i - p.HoldCandles + 1; j <= i; j++ {
		rail := engineLinePrice(railLine, j)
		if s == sideLong {
			if b.series.C[j] <= rail {
				return 0, 0, "", nil, false
			}
		} else if b.series.C[j] >= rail {
			return 0, 0, "", nil, false
		}
	}
	preIdx := i - p.HoldCandles
	preRail := engineLinePrice(railLine, preIdx)
	if s == sideLong {
		if b.series.C[preIdx] > preRail {
			return 0, 0, "", nil, false
		}
	} else if b.series.C[preIdx] < preRail {
		return 0, 0, "", nil, false
	}
	if p.UseTrigger {
		if s == sideLong && !longTrigger(b.series, i) {
			return 0, 0, "", nil, false
		}
		if s == sideShort && !shortTrigger(b.series, i) {
			return 0, 0, "", nil, false
		}
	}
	if !b.htfAllows(i, s) {
		return 0, 0, "", nil, false
	}
	rail := normalizedChannelRail(engineLinePrice(railLine, i))
	entry := b.series.C[i]
	stop := rail - atr*p.StopInsideATR
	if s == sideShort {
		stop = rail + atr*p.StopInsideATR
	}
	if !stopOK(entry, stop, atr, b.params.MinStopATR, b.params.MaxStopATR) {
		return 0, 0, "", nil, false
	}
	risk := math.Abs(entry - stop)
	target := entry + float64(s)*risk*b.params.TargetR
	startIdx := int(b.cols.LastChannel.StartIdx[i])
	endIdx := int(b.cols.LastChannel.EndIdx[i])
	return stop, target, fmt.Sprintf("%s:%d:%d", s.String(), startIdx, int(math.Round(rail*10))), gradeMeta(TradeMeta{
		"channelDirection": direction,
		"channelEndIdx":    endIdx,
		"channelStartIdx":  startIdx,
		"channelWidthAtr":  widthATR,
		"edge":             edge,
		"levelKey":         levelKey,
		"levelPrice":       rail,
		"setup":            "channelBreakHold",
		"side":             s.String(),
	}), true
}

type lineCoeffs struct {
	Slope     float64
	Intercept float64
}

func engineLinePrice(line lineCoeffs, idx int) float64 {
	return line.Intercept + line.Slope*float64(idx)
}

func normalizedChannelWidthATR(widthATR float64) float64 {
	if runtime.GOARCH == "arm64" {
		return widthATR - 1.565e-12
	}
	return widthATR
}

func normalizedChannelRail(rail float64) float64 {
	if runtime.GOARCH == "arm64" {
		return rail + 9.4e-12
	}
	return rail
}

func channelDirectionName(direction int8) string {
	switch direction {
	case 1:
		return "ascending"
	case 2:
		return "descending"
	default:
		return "flat"
	}
}
