package dsl

import "testing"

func TestTrendPullbackSessionVWAPPhrases(t *testing.T) {
	source := `dsl v7
setup {
  type: trend pullback
  pullback session VWAP close touch
  pullback first session VWAP touch only
  reclaim session VWAP on trend side
}`
	result, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v", result.Errors)
	}
	tp := copyMap(result.Config["trendPullback"])
	if got := tp["sessionVwapTouch"]; got != "close" {
		t.Fatalf("sessionVwapTouch = %v, want close", got)
	}
	if got := tp["sessionVwapFirstTouchOnly"]; got != 1 {
		t.Fatalf("sessionVwapFirstTouchOnly = %v, want 1", got)
	}
	if got := tp["sessionVwapTrendSideReclaim"]; got != 1 {
		t.Fatalf("sessionVwapTrendSideReclaim = %v, want 1", got)
	}

	partial, err := Parse(`dsl v7
setup {
  type: trend pullback
  pullback session VWAP wick touch
}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(partial.Errors) != 1 || partial.Errors[0] != `trend-pullback session VWAP touch requires both "pullback first session VWAP touch only" and "reclaim session VWAP on trend side".` {
		t.Fatalf("partial errors = %v", partial.Errors)
	}
}
