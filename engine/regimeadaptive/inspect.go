package regimeadaptive

import (
	"encoding/json"
	"fmt"
	fp "github.com/spik3r/heisentick-strat/internal/float64contract"
)

func sensitive(tf Timeframe, profile string) bool {
	return tf.Multiplier <= 30 && (tf.Unit == "minute" || profile == SourceProfile && tf.Unit == "second")
}
func cross(a, b, prevA, prevB *float64, up bool) bool {
	if a == nil || b == nil || prevA == nil || prevB == nil {
		return false
	}
	if up {
		return *a > *b && *prevA <= *prevB
	}
	return *a < *b && *prevA >= *prevB
}
func classify(chop, rsi, sar *float64, close float64, small bool) int {
	bull, bear := 48., 52.
	if small {
		bull, bear = 46, 54
	}
	if chop != nil && *chop > 58 {
		return 0
	}
	if rsi != nil && sar != nil && *rsi > bull && close > *sar {
		return 1
	}
	if rsi != nil && sar != nil && *rsi < bear && close < *sar {
		return 2
	}
	return 3
}
func signals(row Row, previous *Row, b Bar, prevBar *Bar, small bool, profile string) Signals {
	s := Signals{}
	if previous == nil {
		return s
	}
	s.InitialLong = row.Regime == 1 && cross(row.RSI, row.RSIMA, previous.RSI, previous.RSIMA, true)
	s.InitialShort = row.Regime == 2 && cross(row.RSI, row.RSIMA, previous.RSI, previous.RSIMA, false)
	fifty := ptr(50)
	if profile == SourceProfile {
		s.ScaleLong = row.Regime == 1 && cross(row.RSI, fifty, previous.RSI, fifty, true) && row.PSAR != nil && b.Close > *row.PSAR
		s.ScaleShort = row.Regime == 2 && cross(row.RSI, fifty, previous.RSI, fifty, false) && row.PSAR != nil && b.Close < *row.PSAR
	}
	close, prevClose := ptr(b.Close), ptr(prevBar.Close)
	s.RangeLong = row.Regime == 3 && (cross(close, row.BBLower, prevClose, previous.BBLower, false) || small && row.BBLower != nil && b.Low <= *row.BBLower)
	s.RangeShort = row.Regime == 3 && (cross(close, row.BBUpper, prevClose, previous.BBUpper, true) || small && row.BBUpper != nil && b.High >= *row.BBUpper)
	s.BothBands = s.RangeLong && s.RangeShort
	s.FlatEligible = profile == FlatProfile && !s.BothBands && row.CHOP != nil && row.ATR != nil && row.RSI != nil && row.RSIMA != nil && row.PSAR != nil && row.BBBasis != nil && row.BBUpper != nil && row.BBLower != nil && (s.InitialLong || s.InitialShort || s.RangeLong || s.RangeShort)
	// FlatEligible is indicator/signal eligibility only. No equity, position,
	// quantity, margin, rounding or fill admission is evaluated here.
	return s
}
func Inspect(req Request) (result Result, err error) {
	defer func() {
		if err != nil {
			result = Result{}
		}
	}()
	defer fp.Recover(&err)
	if req.Schema != Schema || req.Mode != "indicators-signals-only" || req.DataKind != "synthetic" || req.FixtureID == "" {
		return result, fmt.Errorf("only identified synthetic indicators-signals-only requests supported; execution/historical refused")
	}
	expected := SourceHash
	if req.Profile == FlatProfile {
		expected = FlatHash
	} else if req.Profile != SourceProfile {
		return result, fmt.Errorf("unsupported profile")
	}
	if req.SourceSHA256 != expected {
		return result, fmt.Errorf("profile/source identity mismatch")
	}
	tf := req.Timeframe
	if (tf.Unit != "second" && tf.Unit != "minute" && tf.Unit != "day") || tf.Multiplier <= 0 || tf.Multiplier > 1440 {
		return result, fmt.Errorf("unsupported timeframe")
	}
	if len(req.Bars) == 0 || len(req.Bars) > 10000 {
		return result, fmt.Errorf("synthetic fixture requires 1..10000 bars")
	}
	duration := int64(tf.Multiplier) * 1000
	if tf.Unit == "minute" {
		duration *= 60
	}
	if tf.Unit == "day" {
		duration *= 86400
	}
	for i, b := range req.Bars {
		if b.T < 0 || b.T > 9007199254740991 || b.CloseT < 0 || b.CloseT > 9007199254740991 || b.CloseT <= b.T || b.CloseT-b.T != duration || i > 0 && b.T < req.Bars[i-1].CloseT {
			return result, fmt.Errorf("invalid/overlapping closed-bar timestamps at %d", i)
		}
		for _, v := range []float64{b.Open, b.High, b.Low, b.Close} {
			if !finite(v) {
				return result, fmt.Errorf("nonfinite OHLC at %d", i)
			}
			fp.Check(v, "bar price")
		}
		if b.Low <= 0 || b.High < b.Low || b.Open < b.Low || b.Open > b.High || b.Close < b.Low || b.Close > b.High {
			return result, fmt.Errorf("invalid OHLC at %d", i)
		}
	}
	rows := calculate(req.Bars)
	small := sensitive(tf, req.Profile)
	for i := range rows {
		rows[i].Regime = classify(rows[i].CHOP, rows[i].RSI, rows[i].PSAR, req.Bars[i].Close, small)
		if i > 0 {
			rows[i].Signals = signals(rows[i], &rows[i-1], req.Bars[i], &req.Bars[i-1], small, req.Profile)
		}
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return result, err
	}
	return Result{Schema: Schema, Profile: req.Profile, SourceSHA256: expected, RequestSHA256: hash(raw), FixtureID: req.FixtureID, Timeframe: tf,
		Inputs: map[string]any{"chopLen": 14, "chopThreshold": 58, "rsiLen": 14, "rsiMaLen": 14, "psarStart": .018, "psarIncrement": .018, "psarMaximum": .20, "bbLen": 20, "bbMultiplier": 2, "atrLen": 14, "scaleIn": req.Profile == SourceProfile}, Rows: rows, ExecutionStatus: "unsupported-not-run",
		Limitations: []string{"synthetic indicator/signal candidate only; no Pine witness qualification", "both-zero RSI is unavailable pending Pine flat-series witness", "native synthetic bars only; no MTF/security/session resampling", "no orders/fills/fees/sizing/margin/terminal accounting; null means unavailable, never empty-equivalent", "default Inputs only; no generic DSL/browser/live admission"}}, nil
}
