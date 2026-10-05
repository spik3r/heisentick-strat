package frozenlevels

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

func ms(s string) int64 {
	t, e := time.Parse(time.RFC3339, s)
	if e != nil {
		panic(e)
	}
	return t.UnixMilli()
}
func source() Source {
	return Source{Provider: "invented-fixture", Dataset: "synthetic-m5-v1", SHA256: strings.Repeat("a", 64), Side: Bid}
}
func syntheticBars() []CompletedBar {
	start := ms("2026-02-04T23:30:00Z")
	var out []CompletedBar
	for i := 0; i < 12; i++ {
		p := float64(100 + i)
		at := start + int64(i)*M5Millis
		out = append(out, CompletedBar{StartMillis: at, EndMillis: at + M5Millis, KnownAtMillis: at + M5Millis, Complete: true, Open: p, High: p + 2, Low: p - 1, Close: p + 1, Source: source()})
	}
	return out
}
func TestRollingClockAndCompletedWindows(t *testing.T) {
	bars := syntheticBars()
	at := ms("2026-02-05T00:15:00Z")
	cases := []struct {
		kind       WindowKind
		start, end string
		count      int
		o, h, l, c float64
	}{
		{RollingM30, "2026-02-04T23:45:00Z", "2026-02-05T00:15:00Z", 6, 103, 110, 102, 109},
		{ClockM30, "2026-02-04T23:30:00Z", "2026-02-05T00:00:00Z", 6, 100, 107, 99, 106},
		{ClockM15, "2026-02-05T00:00:00Z", "2026-02-05T00:15:00Z", 3, 106, 110, 105, 109},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			w, e := FreezeWindow(tc.kind, at, bars)
			if e != nil {
				t.Fatal(e)
			}
			if w.StartMillis != ms(tc.start) || w.EndMillis != ms(tc.end) || w.BarCount != tc.count || w.Open != tc.o || w.High != tc.h || w.Low != tc.l || w.Close != tc.c || w.KnownAtMillis != ms(tc.end) || w.FrozenAtMillis != at || w.NetDirection() != 1 {
				t.Fatalf("wrong window: %+v", w)
			}
			ladder, e := Camarilla(w)
			if e != nil {
				t.Fatal(e)
			}
			if len(ladder.Levels) != 9 || ladder.Levels[4] != (Pivot{"C", tc.c}) || math.Abs(ladder.Levels[5].Price-(tc.c+1.1*(tc.h-tc.l)/12)) > 1e-12 {
				t.Fatalf("wrong ladder: %+v", ladder)
			}
		})
	}
}
func TestNoUnfinishedOrFutureBarLeakage(t *testing.T) {
	bars := syntheticBars()
	at := ms("2026-02-05T00:17:00Z")
	base, e := FreezeWindow(RollingM30, at, bars)
	if e != nil {
		t.Fatal(e)
	}
	// The unfinished 00:15 bucket and all later bars are intentionally absurd.
	for i := range bars {
		if bars[i].StartMillis >= base.EndMillis {
			bars[i].High = math.Inf(1)
			bars[i].KnownAtMillis = 0
			bars[i].Complete = false
		}
	}
	got, e := FreezeWindow(RollingM30, at, bars)
	if e != nil || !reflect.DeepEqual(got, base) {
		t.Fatalf("future leakage: %+v %v", got, e)
	}
	// Source rows can be unordered, but their timestamps determine slot order.
	for i, j := 0, len(bars)-1; i < j; i, j = i+1, j-1 {
		bars[i], bars[j] = bars[j], bars[i]
	}
	got, e = FreezeWindow(RollingM30, at, bars)
	if e != nil || !reflect.DeepEqual(got, base) {
		t.Fatalf("input order affects slots: %+v %v", got, e)
	}
}
func TestWindowFailClosed(t *testing.T) {
	mutations := map[string]func([]CompletedBar) []CompletedBar{
		"missing":         func(b []CompletedBar) []CompletedBar { return append(b[:4], b[5:]...) },
		"duplicate":       func(b []CompletedBar) []CompletedBar { return append(b, b[4]) },
		"unfinished":      func(b []CompletedBar) []CompletedBar { b[4].Complete = false; return b },
		"late":            func(b []CompletedBar) []CompletedBar { b[4].KnownAtMillis = ms("2026-02-05T00:15:00.001Z"); return b },
		"early knowledge": func(b []CompletedBar) []CompletedBar { b[4].KnownAtMillis = b[4].EndMillis - 1; return b },
		"off grid":        func(b []CompletedBar) []CompletedBar { b[4].StartMillis++; return b },
		"wrong period":    func(b []CompletedBar) []CompletedBar { b[4].EndMillis++; return b },
		"mixed side":      func(b []CompletedBar) []CompletedBar { b[4].Source.Side = Ask; return b },
		"missing source":  func(b []CompletedBar) []CompletedBar { b[4].Source.SHA256 = ""; return b },
		"wrong source":    func(b []CompletedBar) []CompletedBar { b[4].Source.Dataset = "another"; return b },
		"bad OHLC":        func(b []CompletedBar) []CompletedBar { b[4].Low = b[4].High + 1; return b },
		"infinite":        func(b []CompletedBar) []CompletedBar { b[4].High = math.Inf(1); return b },
		"nan":             func(b []CompletedBar) []CompletedBar { b[4].Close = math.NaN(); return b },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			if _, e := FreezeWindow(RollingM30, ms("2026-02-05T00:15:00Z"), mutate(syntheticBars())); e == nil {
				t.Fatal("accepted invalid window")
			}
		})
	}
	for _, at := range []int64{-1, 0, maxExactMillis + 1} {
		if _, e := FreezeWindow(RollingM30, at, nil); e == nil {
			t.Fatal("accepted invalid time")
		}
	}
	if _, e := FreezeWindow("prior-day", ms("2026-02-05T00:15:00Z"), syntheticBars()); e == nil {
		t.Fatal("substituted unsupported window")
	}
}
func TestSixBarOpenCloseIsNotCloseROC(t *testing.T) {
	bars := syntheticBars()[:6]
	bars[0].Open = 90
	bars[0].Low = 89
	bars[0].Close = 101
	for i := 1; i < len(bars); i++ {
		bars[i].Open = 95
		bars[i].High = 101
		bars[i].Low = 89
		bars[i].Close = 95
	}
	w, e := FreezeWindow(RollingM30, bars[5].EndMillis, bars)
	if e != nil {
		t.Fatal(e)
	}
	if w.NetDirection() != 1 || bars[5].Close >= bars[0].Close {
		t.Fatal("open-to-close direction was replaced by close ROC")
	}
	for _, close := range []float64{90, 89} {
		bars[5].Close = close
		w, e = FreezeWindow(RollingM30, bars[5].EndMillis, bars)
		if e != nil {
			t.Fatal(e)
		}
		want := 0
		if close == 89 {
			want = -1
		}
		if w.NetDirection() != want {
			t.Fatal("direction", w)
		}
	}
}
func TestFrozenWindowIdentityAndInputIsolation(t *testing.T) {
	bars := syntheticBars()
	w, e := FreezeWindow(ClockM15, ms("2026-02-05T00:15:00Z"), bars)
	if e != nil {
		t.Fatal(e)
	}
	bars[6].High = 999
	if w.High == 999 {
		t.Fatal("mutable source retained")
	}
	w.High++
	if _, e = Camarilla(w); e == nil {
		t.Fatal("accepted mutated frozen window")
	}
}
