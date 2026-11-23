package engine

import (
	"errors"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
)

// clockFadeParams are the clock-fade family settings. The family fades the
// trailing LookbackBars-candle move at one New York wall-clock minute.
type clockFadeParams struct {
	LookbackBars  int
	NewYorkMinute int
	StopATR       float64
}

var errClockFadeConfig = errors.New("clock fade: lookback N candles, signal at new york HH:MM and stop N ATR are required")

// validateClockFadeConfig refuses a malformed clock-fade config. It reads the
// raw parsed config, so a hand-built config cannot skip the parser's checks.
func validateClockFadeConfig(cfg dsl.Config) error {
	if setupTypeFromAny(cfg["setupType"]) != string(dsl.FamilyClockFade) {
		return nil
	}
	if source, ok := cfg["sourceTimeframe"].(string); ok && source != "" {
		return errors.New("clock fade does not support source timeframe execution")
	}
	family := mapValue(cfg, "clockFade")
	if _, ok := donchianCount(family["lookbackBars"]); !ok {
		return errClockFadeConfig
	}
	minute, ok := donchianNumber(family["newYorkMinute"])
	if !ok || minute < 0 || minute > 1439 || minute != float64(int(minute)) {
		return errClockFadeConfig
	}
	stop := mapValue(cfg, "stop")
	if stop["type"] != "atr" {
		return errClockFadeConfig
	}
	if multiple, ok := donchianNumber(stop["atrMult"]); !ok || !(multiple > 0) {
		return errClockFadeConfig
	}
	hold, ok := donchianCount(cfg["maxHoldCandles"])
	if !ok || hold < 1 {
		return errors.New("clock fade requires maxHoldCandles N with positive integer N")
	}
	return nil
}

// clockFadeDirection returns the side that fades the move from the close
// LookbackBars bars before bar i to the close of bar i. It reports false when
// the lookback window is not LookbackBars equally spaced bars (a session gap
// inside the window), when a price is not finite, or when the move is zero.
// Only bars up to i are read.
func clockFadeDirection(t, closes []float64, i, lookback int) (side, float64, bool) {
	if lookback <= 0 || i < lookback || i >= len(closes) || i >= len(t) {
		return 0, 0, false
	}
	step := t[i] - t[i-1]
	if !(step > 0) {
		return 0, 0, false
	}
	for j := i - lookback + 1; j <= i; j++ {
		if t[j]-t[j-1] != step {
			return 0, 0, false
		}
	}
	current, prior := closes[i], closes[i-lookback]
	if !isFinite(current) || !isFinite(prior) {
		return 0, 0, false
	}
	move := current - prior
	switch {
	case move > 0:
		return sideShort, move, true
	case move < 0:
		return sideLong, move, true
	}
	return 0, 0, false
}

// onClockFadeBar evaluates bar i. The signal bar is the bar that opens at the
// configured New York minute; its close is the decision time. The trade fades
// the trailing move with a fixed ATR stop, no target, and the shared
// maxHoldCandles time exit.
func (b *broker) onClockFadeBar(i int) {
	if b.hasPosition {
		b.exitAfterBars(i)
		return
	}
	p := b.params
	cf := p.ClockFade
	if contextcols.NewYorkMinuteOfDay(int64(b.series.T[i])) != cf.NewYorkMinute {
		return
	}
	s, move, ok := clockFadeDirection(b.series.T, b.series.C, i, cf.LookbackBars)
	if !ok {
		return
	}
	if s == sideLong && !p.AllowLong || s == sideShort && !p.AllowShort {
		return
	}
	atr := finiteOrZero(b.cols.ATR[i])
	if atr <= 0 || !isFinite(cf.StopATR) || !(cf.StopATR > 0) {
		return
	}
	entry := b.series.C[i]
	if !isFinite(entry) {
		return
	}
	sign := float64(s)
	stop := entry - sign*cf.StopATR*atr
	if !isFinite(stop) || sign*(entry-stop) <= 0 {
		return
	}
	setup := setupPlan{
		Side:     s,
		Stop:     stop,
		Tag:      "DSL-CF",
		NoTarget: true,
		Meta: gradeMeta(TradeMeta{
			"setup":        "clockFade",
			"side":         s.String(),
			"lookbackBars": float64(cf.LookbackBars),
			"trailingMove": move,
			"signalIndex":  float64(i),
		}),
	}
	b.enterSetup(i, setup)
}
