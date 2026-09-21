package engine

import "github.com/spik3r/heisentick-strat/marketdata"

// flatHTFForUnrelatedTest neutralizes the HTF direction gate in tests whose
// assertion is about setup discovery, trigger parsing, or execution plumbing.
// The bar timestamps remain intact, so source alignment and availability are
// still exercised without making D19 direction part of the assertion.
func flatHTFForUnrelatedTest(fixture RunFixture) RunFixture {
	fixture.HTFBars = append([]marketdata.Bar(nil), fixture.HTFBars...)
	for i := range fixture.HTFBars {
		fixture.HTFBars[i].O = 100
		fixture.HTFBars[i].H = 101
		fixture.HTFBars[i].L = 99
		fixture.HTFBars[i].C = 100
	}
	return fixture
}
