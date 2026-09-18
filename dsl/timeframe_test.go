package dsl

import "testing"

func TestResolveHigherTimeframeMatchesJSFixtureSemantics(t *testing.T) {
	tests := []struct {
		name      string
		tf        string
		requested string
		want      string
	}{
		{name: "empty requested defaults to auto", tf: "15m", requested: "", want: "1h"},
		{name: "auto lower intraday", tf: "5m", requested: "auto", want: "1h"},
		{name: "auto one hour", tf: "1h", requested: "auto", want: "4h"},
		{name: "auto four hour", tf: "4h", requested: "auto", want: "1d"},
		{name: "auto unsupported", tf: "1d", requested: "auto", want: ""},
		{name: "explicit value lowercased", tf: "15m", requested: "4H", want: "4h"},
		{name: "none disables htf", tf: "15m", requested: "none", want: ""},
		{name: "same timeframe disables htf", tf: "15m", requested: "15m", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveHigherTimeframe(tt.tf, tt.requested); got != tt.want {
				t.Fatalf("ResolveHigherTimeframe(%q, %q) = %q, want %q", tt.tf, tt.requested, got, tt.want)
			}
		})
	}
}
