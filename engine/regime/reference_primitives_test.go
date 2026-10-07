package regime

import (
	"reflect"
	"testing"
)

func TestReferenceFacadePreservesQualifiedPrimitives(t *testing.T) {
	s := waveM5(100)
	before := s.Bars()
	bars, used, e := aggregate(s, 0, 100*M30MS)
	if e != nil {
		t.Fatal(e)
	}
	want, e := calculate(bars)
	if e != nil {
		t.Fatal(e)
	}
	got, n, e := ReferenceIndicators(s, 0, 100*M30MS)
	if e != nil || n != used || !reflect.DeepEqual(got, want) {
		t.Fatal("aggregation facade changed v9 arithmetic", e)
	}
	got, e = ReferenceBarIndicators(bars)
	if e != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(s.Bars(), before) {
		t.Fatal("bar facade changed inputs/arithmetic", e)
	}
	for _, d := range []int{1, -1} {
		p, r, tm := barrier(bars[90], 99, 103, d, 1)
		gp, gr, gt := ReferenceBarrier(bars[90], 99, 103, d, 1)
		if p != gp || r != gr || tm != gt {
			t.Fatal("barrier facade changed chronology")
		}
	}
}
