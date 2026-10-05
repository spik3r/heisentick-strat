package frozenlevels

import "fmt"

type WindowKind string

const (
	RollingM30 WindowKind = "rolling-six-completed-m5"
	ClockM30   WindowKind = "previous-clock-m30"
	ClockM15   WindowKind = "previous-clock-m15"
)

// CompletedBar represents a half-open M5 source bucket. Complete is a caller's
// source-quality assertion; this package still proves contiguous bucket slots.
type CompletedBar struct {
	StartMillis   int64   `json:"startMillis"`
	EndMillis     int64   `json:"endMillis"`
	KnownAtMillis int64   `json:"knownAtMillis"`
	Complete      bool    `json:"complete"`
	Open          float64 `json:"open"`
	High          float64 `json:"high"`
	Low           float64 `json:"low"`
	Close         float64 `json:"close"`
	Source        Source  `json:"source"`
}

type Window struct {
	ID             string     `json:"id"`
	Version        string     `json:"version"`
	Kind           WindowKind `json:"kind"`
	StartMillis    int64      `json:"startMillis"`
	EndMillis      int64      `json:"endMillis"`
	KnownAtMillis  int64      `json:"knownAtMillis"`
	FrozenAtMillis int64      `json:"frozenAtMillis"`
	BarCount       int        `json:"barCount"`
	Source         Source     `json:"source"`
	Open           float64    `json:"open"`
	High           float64    `json:"high"`
	Low            float64    `json:"low"`
	Close          float64    `json:"close"`
}

// FreezeWindow never falls back to another anchor or skips a missing bucket.
// Future and unrelated rows do not affect the result. Duplicate relevant slots,
// late/uncompleted relevant bars and mixed source identities fail closed.
func FreezeWindow(kind WindowKind, at int64, bars []CompletedBar) (Window, error) {
	w := Window{Version: ContractVersion, Kind: kind, FrozenAtMillis: at}
	if !validMillis(at) {
		return Window{}, fmt.Errorf("invalid-decision-time")
	}
	duration := 6 * M5Millis
	alignment := M5Millis
	switch kind {
	case RollingM30:
	case ClockM30:
		alignment = duration
	case ClockM15:
		duration = 3 * M5Millis
		alignment = duration
	default:
		return Window{}, fmt.Errorf("unsupported-window")
	}
	w.EndMillis = at / alignment * alignment
	w.StartMillis = w.EndMillis - duration
	if w.StartMillis < 0 {
		return Window{}, fmt.Errorf("missing-completed-window")
	}
	want := int(duration / M5Millis)
	slots := make([]*CompletedBar, want)
	for i := range bars {
		b := &bars[i]
		if b.StartMillis < w.StartMillis || b.StartMillis >= w.EndMillis {
			continue
		}
		if b.StartMillis%M5Millis != 0 || b.EndMillis != b.StartMillis+M5Millis || !validMillis(b.KnownAtMillis) || b.KnownAtMillis < b.EndMillis || b.KnownAtMillis > at || !b.Complete {
			return Window{}, fmt.Errorf("invalid-or-unavailable-window-bar")
		}
		if err := b.Source.validate(); err != nil {
			return Window{}, err
		}
		if !finitePositive(b.Open) || !finitePositive(b.High) || !finitePositive(b.Low) || !finitePositive(b.Close) || b.Low > b.Open || b.Low > b.Close || b.High < b.Open || b.High < b.Close {
			return Window{}, fmt.Errorf("invalid-window-ohlc")
		}
		slot := int((b.StartMillis - w.StartMillis) / M5Millis)
		if slots[slot] != nil {
			return Window{}, fmt.Errorf("duplicate-window-slot")
		}
		slots[slot] = b
	}
	for i, b := range slots {
		if b == nil {
			return Window{}, fmt.Errorf("missing-completed-window-slot")
		}
		if i == 0 {
			w.Source = b.Source
			w.Open = b.Open
			w.High = b.High
			w.Low = b.Low
		} else if b.Source != w.Source {
			return Window{}, fmt.Errorf("mixed-window-source")
		}
		if b.High > w.High {
			w.High = b.High
		}
		if b.Low < w.Low {
			w.Low = b.Low
		}
		w.Close = b.Close
		if b.KnownAtMillis > w.KnownAtMillis {
			w.KnownAtMillis = b.KnownAtMillis
		}
	}
	w.BarCount = want
	w.ID = identity(w)
	return w, nil
}

// NetDirection uses the first completed source bar's OPEN and last CLOSE. It
// must not be replaced by the existing close-to-close ROC strategy family.
func (w Window) NetDirection() int {
	if w.Close > w.Open {
		return 1
	}
	if w.Close < w.Open {
		return -1
	}
	return 0
}

type Pivot struct {
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}
type Ladder struct {
	Version        string  `json:"version"`
	WindowID       string  `json:"windowId"`
	KnownAtMillis  int64   `json:"knownAtMillis"`
	FrozenAtMillis int64   `json:"frozenAtMillis"`
	Levels         []Pivot `json:"levels"`
}

// Camarilla emits C and R/S1..4 from the supplied frozen, qualified source.
// No label is chosen from a later fill and no prior UTC-day context is used.
func Camarilla(w Window) (Ladder, error) {
	check := w
	check.ID = ""
	if w.Version != ContractVersion || w.ID == "" || identity(check) != w.ID {
		return Ladder{}, fmt.Errorf("invalid-window-identity")
	}
	ladder := Ladder{Version: ContractVersion, WindowID: w.ID, KnownAtMillis: w.KnownAtMillis, FrozenAtMillis: w.FrozenAtMillis}
	spread := 1.1 * (w.High - w.Low)
	for i, k := range []float64{2, 4, 6, 12} {
		ladder.Levels = append(ladder.Levels, Pivot{Name: fmt.Sprintf("S%d", 4-i), Price: w.Close - spread/k})
	}
	ladder.Levels = append(ladder.Levels, Pivot{Name: "C", Price: w.Close})
	for i, k := range []float64{12, 6, 4, 2} {
		ladder.Levels = append(ladder.Levels, Pivot{Name: fmt.Sprintf("R%d", i+1), Price: w.Close + spread/k})
	}
	for _, p := range ladder.Levels {
		if !finitePositive(p.Price) {
			return Ladder{}, fmt.Errorf("invalid-pivot-price")
		}
	}
	return ladder, nil
}
