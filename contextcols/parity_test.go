package contextcols

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

type parityFixture struct {
	Schema  string            `json:"schema"`
	Source  fixtureMeta       `json:"source"`
	Columns []string          `json:"columns"`
	Rows    [][]nullableFloat `json:"rows"`
}

type fixtureMeta struct {
	Fixture string `json:"fixture"`
	Limit   int    `json:"limit"`
}

type parityFeatureGroup struct {
	Name    string
	Columns []string
}

var moneyLogicParityGroups = map[string][]parityFeatureGroup{
	"session-expansion-core.json": {
		{Name: "indicators", Columns: []string{"atr", "er", "vwap", "vwapDistanceAtr"}},
		{Name: "session", Columns: []string{"hourUtc", "sessionPhase", "inSessionAsia", "inSessionLondon", "inSessionNy"}},
		{Name: "day", Columns: []string{"dayHigh", "dayLow", "dayOpen", "isNewDay", "priorDayO", "priorDayH", "priorDayL", "priorDayC"}},
	},
	"session-expansion-levels.json": {
		{Name: "camarilla", Columns: []string{"camarillaR3", "camarillaR4", "camarillaS3", "camarillaS4"}},
		{Name: "prior-session", Columns: prefixedFields([]string{"priorSessionAsia", "priorSessionLondon", "priorSessionNy"}, []string{"O", "H", "L", "C"})},
		{Name: "overnight-range", Columns: prefixedNames("overnightRange", "High", "Low", "Mid", "Width", "WidthAtr")},
		{Name: "day-range", Columns: []string{"dayRangeProgress"}},
		{Name: "range-stats-day", Columns: prefixedNames("rangeStatsDay", "Range", "Avg", "UsedPct", "Hi", "Lo", "State")},
		{Name: "range-stats-week", Columns: prefixedNames("rangeStatsWeek", "Range", "Avg", "UsedPct", "Hi", "Lo", "State")},
		{Name: "range-stats-session", Columns: prefixedFields([]string{"rangeStatsSessionAsia", "rangeStatsSessionLondon", "rangeStatsSessionNy"}, []string{"Range", "Avg", "UsedPct", "Hi", "Lo", "State"})},
		{Name: "range-stats-window", Columns: prefixedFields([]string{"rangeStatsWindowAsia", "rangeStatsWindowLondon", "rangeStatsWindowNy"}, []string{"Range", "Avg", "UsedPct", "Hi", "Lo", "State"})},
	},
	"session-expansion-ranges.json": {
		{Name: "regime-trend", Columns: []string{"er", "regime", "trendDir"}},
		{Name: "swings", Columns: []string{"swingHigh", "swingLow"}},
		{Name: "active-range", Columns: []string{"rangeHigh", "rangeLow", "rangeActive"}},
		{Name: "last-range", Columns: []string{"lastRangeHigh", "lastRangeLow", "lastRangeSinceActive"}},
		{Name: "volume", Columns: []string{"volumeSma20", "volumeRatio20", "volumeZ50", "spreadAtr", "bodyAtr", "effortResultRatio"}},
		{Name: "ema", Columns: []string{"emaFast", "emaFastSlope", "emaFastDistanceAtr"}},
		{Name: "channel", Columns: channelFixtureColumns("channel")},
		{Name: "last-channel", Columns: channelFixtureColumns("lastChannel")},
	},
	"session-expansion-session-bias.json": {
		{Name: "session-day-context", Columns: []string{"atr", "hourUtc", "sessionPhase", "openLocation", "priorDayType"}},
		{Name: "session-bias-session", Columns: prefixedFields([]string{"sessionBiasSessionAsia", "sessionBiasSessionLondon", "sessionBiasSessionNy"}, []string{"Mode", "Score", "DisplacementAtr", "BodyNetAtr", "VwapSlopeAtr", "State"})},
		{Name: "session-bias-window", Columns: prefixedFields([]string{"sessionBiasWindowAsia", "sessionBiasWindowLondon", "sessionBiasWindowNy"}, []string{"Mode", "Score", "DisplacementAtr", "BodyNetAtr", "VwapSlopeAtr", "State"})},
	},
}

func TestMoneyLogicParityFixtureCoverage(t *testing.T) {
	fixtures := []struct {
		name   string
		schema string
	}{
		{name: "session-expansion-core.json", schema: "go-context-core-v1"},
		{name: "session-expansion-levels.json", schema: "go-context-levels-v1"},
		{name: "session-expansion-ranges.json", schema: "go-context-ranges-v1"},
		{name: "session-expansion-session-bias.json", schema: "go-context-session-bias-v1"},
	}

	totalFeatures := 0
	totalColumns := 0
	for _, item := range fixtures {
		t.Run(item.name, func(t *testing.T) {
			fixture := loadParityFixture(t, item.name, item.schema)
			features, ok := moneyLogicParityGroups[item.name]
			if !ok {
				t.Fatalf("fixture has no money-logic assertion groups")
			}
			assigned := make(map[string]string)
			for _, feature := range features {
				if len(feature.Columns) == 0 {
					t.Errorf("feature %q has no columns", feature.Name)
				}
				for _, column := range feature.Columns {
					if previous, duplicate := assigned[column]; duplicate {
						t.Errorf("column %q belongs to both %q and %q", column, previous, feature.Name)
					}
					assigned[column] = feature.Name
				}
			}

			fixtureColumns := columnIndex(t, fixture.Columns)
			covered := 0
			for _, column := range fixture.Columns {
				if isFixtureInputColumn(column) {
					continue
				}
				if _, ok := assigned[column]; !ok {
					t.Errorf("money-logic reference column %q has no Go assertion group", column)
				}
				covered++
			}
			for column, feature := range assigned {
				if _, ok := fixtureColumns[column]; !ok {
					t.Errorf("feature %q assigns unknown fixture column %q", feature, column)
				}
			}
			t.Logf("%d feature groups cover %d money-logic reference columns", len(features), covered)
			totalFeatures += len(features)
			totalColumns += covered
		})
	}
	if totalFeatures != 22 || totalColumns != 178 {
		t.Fatalf("coverage totals = %d features/%d columns, want 22 features/178 columns", totalFeatures, totalColumns)
	}
}

func prefixedNames(prefix string, names ...string) []string {
	out := make([]string, len(names))
	for i, name := range names {
		out[i] = prefix + name
	}
	return out
}

func prefixedFields(prefixes []string, fields []string) []string {
	out := make([]string, 0, len(prefixes)*len(fields))
	for _, prefix := range prefixes {
		out = append(out, prefixedNames(prefix, fields...)...)
	}
	return out
}

func channelFixtureColumns(prefix string) []string {
	return prefixedNames(prefix,
		"Active", "Direction", "Upper", "Lower", "WidthAtr", "StartIdx", "EndIdx",
		"TouchesHigh", "TouchesLow", "Crossings", "SinceActive", "UpperSlope",
		"UpperIntercept", "LowerSlope", "LowerIntercept",
	)
}

func isFixtureInputColumn(column string) bool {
	switch column {
	case "i", "t", "o", "h", "l", "c", "v":
		return true
	default:
		return false
	}
}

type nullableFloat struct {
	Value float64
	Valid bool
}

func TestChannelDirectionCodesMatchJSReference(t *testing.T) {
	if channelNone != 0 || channelAscending != 1 || channelDescending != 2 || channelFlat != 3 {
		t.Fatalf("channel direction codes = none:%d ascending:%d descending:%d flat:%d, want 0/1/2/3",
			channelNone, channelAscending, channelDescending, channelFlat)
	}
}

func (n *nullableFloat) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		n.Value = math.NaN()
		n.Valid = false
		return nil
	}
	n.Valid = true
	return json.Unmarshal(data, &n.Value)
}

func TestCoreColumnsMatchJSDump(t *testing.T) {
	fixture := loadParityFixture(t, "session-expansion-core.json", "go-context-core-v1")
	col := columnIndex(t, fixture.Columns)
	series := seriesFromFixture(t, fixture, col)
	cols := Build(series, Options{})

	if cols.Length != len(fixture.Rows) {
		t.Fatalf("Length = %d, want %d", cols.Length, len(fixture.Rows))
	}
	for i, row := range fixture.Rows {
		assertFloat(t, "atr", i, cols.ATR[i], valueAt(t, row, col, "atr"))
		assertFloat(t, "er", i, cols.ER[i], valueAt(t, row, col, "er"))
		assertFloat(t, "hourUtc", i, cols.HourUTC[i], valueAt(t, row, col, "hourUtc"))
		assertInt8(t, "sessionPhase", i, cols.SessionPhase[i], valueAt(t, row, col, "sessionPhase"))
		assertInt8(t, "inSessionAsia", i, cols.InSession.Asia[i], valueAt(t, row, col, "inSessionAsia"))
		assertInt8(t, "inSessionLondon", i, cols.InSession.London[i], valueAt(t, row, col, "inSessionLondon"))
		assertInt8(t, "inSessionNy", i, cols.InSession.NY[i], valueAt(t, row, col, "inSessionNy"))
		assertFloat(t, "dayHigh", i, cols.DayHigh[i], valueAt(t, row, col, "dayHigh"))
		assertFloat(t, "dayLow", i, cols.DayLow[i], valueAt(t, row, col, "dayLow"))
		assertFloat(t, "dayOpen", i, cols.DayOpen[i], valueAt(t, row, col, "dayOpen"))
		assertInt8(t, "isNewDay", i, cols.IsNewDay[i], valueAt(t, row, col, "isNewDay"))
		assertFloat(t, "priorDayO", i, cols.PriorDayO[i], valueAt(t, row, col, "priorDayO"))
		assertFloat(t, "priorDayH", i, cols.PriorDayH[i], valueAt(t, row, col, "priorDayH"))
		assertFloat(t, "priorDayL", i, cols.PriorDayL[i], valueAt(t, row, col, "priorDayL"))
		assertFloat(t, "priorDayC", i, cols.PriorDayC[i], valueAt(t, row, col, "priorDayC"))
		assertFloat(t, "vwap", i, cols.VWAP[i], valueAt(t, row, col, "vwap"))
		assertFloat(t, "vwapDistanceAtr", i, cols.VWAPDistanceATR[i], valueAt(t, row, col, "vwapDistanceAtr"))
	}
}

func TestLevelColumnsMatchJSDump(t *testing.T) {
	fixture := loadParityFixture(t, "session-expansion-levels.json", "go-context-levels-v1")
	col := columnIndex(t, fixture.Columns)
	series := seriesFromFixture(t, fixture, col)
	cols := Build(series, Options{})

	for i, row := range fixture.Rows {
		assertFloat(t, "camarillaR3", i, cols.Camarilla.R3[i], valueAt(t, row, col, "camarillaR3"))
		assertFloat(t, "camarillaR4", i, cols.Camarilla.R4[i], valueAt(t, row, col, "camarillaR4"))
		assertFloat(t, "camarillaS3", i, cols.Camarilla.S3[i], valueAt(t, row, col, "camarillaS3"))
		assertFloat(t, "camarillaS4", i, cols.Camarilla.S4[i], valueAt(t, row, col, "camarillaS4"))
		assertOHLC(t, "priorSessionAsia", i, cols.PriorSession.Asia, row, col)
		assertOHLC(t, "priorSessionLondon", i, cols.PriorSession.London, row, col)
		assertOHLC(t, "priorSessionNy", i, cols.PriorSession.NY, row, col)
		assertFloat(t, "overnightRangeHigh", i, cols.OvernightRange.High[i], valueAt(t, row, col, "overnightRangeHigh"))
		assertFloat(t, "overnightRangeLow", i, cols.OvernightRange.Low[i], valueAt(t, row, col, "overnightRangeLow"))
		assertFloat(t, "overnightRangeMid", i, cols.OvernightRange.Mid[i], valueAt(t, row, col, "overnightRangeMid"))
		assertFloat(t, "overnightRangeWidth", i, cols.OvernightRange.Width[i], valueAt(t, row, col, "overnightRangeWidth"))
		assertFloat(t, "overnightRangeWidthAtr", i, cols.OvernightRange.WidthATR[i], valueAt(t, row, col, "overnightRangeWidthAtr"))
		assertFloat(t, "dayRangeProgress", i, cols.DayRangeProgress[i], valueAt(t, row, col, "dayRangeProgress"))
		assertRangeStat(t, "rangeStatsDay", i, cols.RangeStats.Day, row, col)
		assertRangeStat(t, "rangeStatsWeek", i, cols.RangeStats.Week, row, col)
		assertRangeStat(t, "rangeStatsSessionAsia", i, cols.RangeStats.Session.Asia, row, col)
		assertRangeStat(t, "rangeStatsSessionLondon", i, cols.RangeStats.Session.London, row, col)
		assertRangeStat(t, "rangeStatsSessionNy", i, cols.RangeStats.Session.NY, row, col)
		assertRangeStat(t, "rangeStatsWindowAsia", i, cols.RangeStats.Window.Asia, row, col)
		assertRangeStat(t, "rangeStatsWindowLondon", i, cols.RangeStats.Window.London, row, col)
		assertRangeStat(t, "rangeStatsWindowNy", i, cols.RangeStats.Window.NY, row, col)
	}
}

func TestRangeColumnsMatchJSDump(t *testing.T) {
	fixture := loadParityFixture(t, "session-expansion-ranges.json", "go-context-ranges-v1")
	col := columnIndex(t, fixture.Columns)
	series := seriesFromFixture(t, fixture, col)
	cols := Build(series, Options{
		EMAFastLen:      20,
		EMAFastSlopeLen: 5,
		Channel: ChannelOptions{
			Enabled:              true,
			MinSpan:              20,
			MinMidCrossings:      1,
			MinWidthATR:          0.1,
			MaxWidthATR:          20,
			BodyTolATR:           2,
			MaxBodyViolationRate: 1,
			MaxSlopeGapATR:       1,
		},
	})

	activeCount := 0
	activeChannelCount := 0
	lastChannelCount := 0
	for i, row := range fixture.Rows {
		assertFloat(t, "er", i, cols.ER[i], valueAt(t, row, col, "er"))
		assertInt8(t, "regime", i, cols.Regime[i], valueAt(t, row, col, "regime"))
		assertInt8(t, "trendDir", i, cols.TrendDir[i], valueAt(t, row, col, "trendDir"))
		assertFloat(t, "swingHigh", i, cols.SwingHigh[i], valueAt(t, row, col, "swingHigh"))
		assertFloat(t, "swingLow", i, cols.SwingLow[i], valueAt(t, row, col, "swingLow"))
		assertFloat(t, "rangeHigh", i, cols.Range.High[i], valueAt(t, row, col, "rangeHigh"))
		assertFloat(t, "rangeLow", i, cols.Range.Low[i], valueAt(t, row, col, "rangeLow"))
		assertInt8(t, "rangeActive", i, cols.Range.Active[i], valueAt(t, row, col, "rangeActive"))
		assertFloat(t, "lastRangeHigh", i, cols.LastRange.High[i], valueAt(t, row, col, "lastRangeHigh"))
		assertFloat(t, "lastRangeLow", i, cols.LastRange.Low[i], valueAt(t, row, col, "lastRangeLow"))
		assertInt32(t, "lastRangeSinceActive", i, cols.LastRange.SinceActive[i], valueAt(t, row, col, "lastRangeSinceActive"))
		assertFloat(t, "volumeSma20", i, cols.VolumeSMA20[i], valueAt(t, row, col, "volumeSma20"))
		assertFloat(t, "volumeRatio20", i, cols.VolumeRatio20[i], valueAt(t, row, col, "volumeRatio20"))
		assertFloat(t, "volumeZ50", i, cols.VolumeZ50[i], valueAt(t, row, col, "volumeZ50"))
		assertFloat(t, "spreadAtr", i, cols.SpreadATR[i], valueAt(t, row, col, "spreadAtr"))
		assertFloat(t, "bodyAtr", i, cols.BodyATR[i], valueAt(t, row, col, "bodyAtr"))
		assertFloat(t, "effortResultRatio", i, cols.EffortResultRatio[i], valueAt(t, row, col, "effortResultRatio"))
		assertFloat(t, "emaFast", i, cols.EMAFast[i], valueAt(t, row, col, "emaFast"))
		assertFloat(t, "emaFastSlope", i, cols.EMAFastSlope[i], valueAt(t, row, col, "emaFastSlope"))
		assertFloat(t, "emaFastDistanceAtr", i, cols.EMAFastDistanceATR[i], valueAt(t, row, col, "emaFastDistanceAtr"))
		assertChannel(t, "channel", i, cols.Channel, row, col)
		assertChannel(t, "lastChannel", i, cols.LastChannel, row, col)
		if cols.Range.Active[i] == 1 {
			activeCount++
		}
		if cols.Channel.Active[i] == 1 {
			activeChannelCount++
		}
		if cols.LastChannel.SinceActive[i] >= 0 {
			lastChannelCount++
		}
	}
	if activeCount == 0 {
		t.Fatalf("range fixture did not exercise an active range")
	}
	if activeChannelCount == 0 || lastChannelCount == 0 {
		t.Fatalf("range fixture channel coverage = %d active/%d last, want both non-zero", activeChannelCount, lastChannelCount)
	}
}

func TestSessionBiasColumnsMatchJSDump(t *testing.T) {
	fixture := loadParityFixture(t, "session-expansion-session-bias.json", "go-context-session-bias-v1")
	col := columnIndex(t, fixture.Columns)
	series := seriesFromFixture(t, fixture, col)
	cols := Build(series, Options{})

	nonOtherOpenLocations := 0
	nonZeroPriorDayTypes := 0
	liveWindowEntries := 0
	for i, row := range fixture.Rows {
		assertFloat(t, "atr", i, cols.ATR[i], valueAt(t, row, col, "atr"))
		assertFloat(t, "hourUtc", i, cols.HourUTC[i], valueAt(t, row, col, "hourUtc"))
		assertInt8(t, "sessionPhase", i, cols.SessionPhase[i], valueAt(t, row, col, "sessionPhase"))
		assertInt8(t, "openLocation", i, cols.OpenLocation[i], valueAt(t, row, col, "openLocation"))
		assertInt8(t, "priorDayType", i, cols.PriorDayType[i], valueAt(t, row, col, "priorDayType"))
		assertSessionBias(t, "sessionBiasSessionAsia", i, cols.SessionBias.Session.Asia, row, col)
		assertSessionBias(t, "sessionBiasSessionLondon", i, cols.SessionBias.Session.London, row, col)
		assertSessionBias(t, "sessionBiasSessionNy", i, cols.SessionBias.Session.NY, row, col)
		assertSessionBias(t, "sessionBiasWindowAsia", i, cols.SessionBias.Window.Asia, row, col)
		assertSessionBias(t, "sessionBiasWindowLondon", i, cols.SessionBias.Window.London, row, col)
		assertSessionBias(t, "sessionBiasWindowNy", i, cols.SessionBias.Window.NY, row, col)

		if cols.OpenLocation[i] != openLocationOther {
			nonOtherOpenLocations++
		}
		if cols.PriorDayType[i] != 0 {
			nonZeroPriorDayTypes++
		}
		if cols.SessionBias.Window.Asia.State[i] == rangeStatLive ||
			cols.SessionBias.Window.London.State[i] == rangeStatLive ||
			cols.SessionBias.Window.NY.State[i] == rangeStatLive {
			liveWindowEntries++
		}
	}
	if nonOtherOpenLocations == 0 {
		t.Fatalf("session-bias fixture did not exercise open-location classification")
	}
	if nonZeroPriorDayTypes == 0 {
		t.Fatalf("session-bias fixture did not exercise prior-day type classification")
	}
	if liveWindowEntries == 0 {
		t.Fatalf("session-bias fixture did not exercise live trade-window bias")
	}
}

func BenchmarkBuildCoreColumns(b *testing.B) {
	fixture := loadParityFixture(b, "session-expansion-core.json", "go-context-core-v1")
	col := columnIndex(b, fixture.Columns)
	series := seriesFromFixture(b, fixture, col)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Build(series, Options{})
	}
	b.ReportMetric(float64(series.Len()), "bars/op")
	b.ReportMetric(float64(series.Len()*b.N)/b.Elapsed().Seconds(), "bars/s")
}

func loadParityFixture(t testing.TB, name string, schema string) parityFixture {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var fixture parityFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if fixture.Schema != schema {
		t.Fatalf("schema = %q, want %q", fixture.Schema, schema)
	}
	return fixture
}

func columnIndex(t testing.TB, columns []string) map[string]int {
	t.Helper()
	index := make(map[string]int, len(columns))
	for i, column := range columns {
		index[column] = i
	}
	return index
}

func valueAt(t testing.TB, row []nullableFloat, columns map[string]int, name string) nullableFloat {
	t.Helper()
	index, ok := columns[name]
	if !ok {
		t.Fatalf("missing column %q", name)
	}
	if index >= len(row) {
		t.Fatalf("row has %d columns, need %q at %d", len(row), name, index)
	}
	return row[index]
}

func seriesFromFixture(t testing.TB, fixture parityFixture, col map[string]int) marketdata.Series {
	t.Helper()
	bars := make([]marketdata.Bar, len(fixture.Rows))
	for i, row := range fixture.Rows {
		bars[i] = marketdata.Bar{
			T: valueAt(t, row, col, "t").Value,
			O: valueAt(t, row, col, "o").Value,
			H: valueAt(t, row, col, "h").Value,
			L: valueAt(t, row, col, "l").Value,
			C: valueAt(t, row, col, "c").Value,
			V: valueAt(t, row, col, "v").Value,
		}
	}
	return marketdata.SeriesFromBars(bars)
}

func assertFloat(t testing.TB, name string, index int, got float64, want nullableFloat) {
	t.Helper()
	if !want.Valid {
		if !math.IsNaN(got) {
			t.Fatalf("%s[%d] = %v, want NaN", name, index, got)
		}
		return
	}
	if math.IsNaN(got) || math.Abs(got-want.Value) > 1e-9 {
		t.Fatalf("%s[%d] = %.15g, want %.15g", name, index, got, want.Value)
	}
}

func assertInt8(t testing.TB, name string, index int, got int8, want nullableFloat) {
	t.Helper()
	if !want.Valid || got != int8(want.Value) {
		t.Fatalf("%s[%d] = %d, want %v", name, index, got, want.Value)
	}
}

func assertInt32(t testing.TB, name string, index int, got int32, want nullableFloat) {
	t.Helper()
	if !want.Valid || got != int32(want.Value) {
		t.Fatalf("%s[%d] = %d, want %v", name, index, got, want.Value)
	}
}

func assertOHLC(t testing.TB, prefix string, index int, got OHLCColumns, row []nullableFloat, columns map[string]int) {
	t.Helper()
	assertFloat(t, prefix+"O", index, got.O[index], valueAt(t, row, columns, prefix+"O"))
	assertFloat(t, prefix+"H", index, got.H[index], valueAt(t, row, columns, prefix+"H"))
	assertFloat(t, prefix+"L", index, got.L[index], valueAt(t, row, columns, prefix+"L"))
	assertFloat(t, prefix+"C", index, got.C[index], valueAt(t, row, columns, prefix+"C"))
}

func assertRangeStat(t testing.TB, prefix string, index int, got RangeStatEntryColumns, row []nullableFloat, columns map[string]int) {
	t.Helper()
	assertFloat(t, prefix+"Range", index, got.Range[index], valueAt(t, row, columns, prefix+"Range"))
	assertFloat(t, prefix+"Avg", index, got.Avg[index], valueAt(t, row, columns, prefix+"Avg"))
	assertFloat(t, prefix+"UsedPct", index, got.UsedPct[index], valueAt(t, row, columns, prefix+"UsedPct"))
	assertFloat(t, prefix+"Hi", index, got.Hi[index], valueAt(t, row, columns, prefix+"Hi"))
	assertFloat(t, prefix+"Lo", index, got.Lo[index], valueAt(t, row, columns, prefix+"Lo"))
	assertInt8(t, prefix+"State", index, got.State[index], valueAt(t, row, columns, prefix+"State"))
}

func assertSessionBias(t testing.TB, prefix string, index int, got SessionBiasEntryColumns, row []nullableFloat, columns map[string]int) {
	t.Helper()
	assertInt8(t, prefix+"Mode", index, got.Mode[index], valueAt(t, row, columns, prefix+"Mode"))
	assertInt8(t, prefix+"Score", index, got.Score[index], valueAt(t, row, columns, prefix+"Score"))
	assertFloat(t, prefix+"DisplacementAtr", index, got.DisplacementATR[index], valueAt(t, row, columns, prefix+"DisplacementAtr"))
	assertFloat(t, prefix+"BodyNetAtr", index, got.BodyNetATR[index], valueAt(t, row, columns, prefix+"BodyNetAtr"))
	assertFloat(t, prefix+"VwapSlopeAtr", index, got.VWAPSlopeATR[index], valueAt(t, row, columns, prefix+"VwapSlopeAtr"))
	assertInt8(t, prefix+"State", index, got.State[index], valueAt(t, row, columns, prefix+"State"))
}

func assertChannel(t testing.TB, prefix string, index int, got ChannelColumns, row []nullableFloat, columns map[string]int) {
	t.Helper()
	assertInt8(t, prefix+"Active", index, got.Active[index], valueAt(t, row, columns, prefix+"Active"))
	assertInt8(t, prefix+"Direction", index, got.Direction[index], valueAt(t, row, columns, prefix+"Direction"))
	assertFloat(t, prefix+"Upper", index, got.Upper[index], valueAt(t, row, columns, prefix+"Upper"))
	assertFloat(t, prefix+"Lower", index, got.Lower[index], valueAt(t, row, columns, prefix+"Lower"))
	assertFloat(t, prefix+"WidthAtr", index, got.WidthATR[index], valueAt(t, row, columns, prefix+"WidthAtr"))
	assertInt32(t, prefix+"StartIdx", index, got.StartIdx[index], valueAt(t, row, columns, prefix+"StartIdx"))
	assertInt32(t, prefix+"EndIdx", index, got.EndIdx[index], valueAt(t, row, columns, prefix+"EndIdx"))
	assertInt32(t, prefix+"TouchesHigh", index, got.TouchesHigh[index], valueAt(t, row, columns, prefix+"TouchesHigh"))
	assertInt32(t, prefix+"TouchesLow", index, got.TouchesLow[index], valueAt(t, row, columns, prefix+"TouchesLow"))
	assertInt32(t, prefix+"Crossings", index, got.Crossings[index], valueAt(t, row, columns, prefix+"Crossings"))
	assertInt32(t, prefix+"SinceActive", index, got.SinceActive[index], valueAt(t, row, columns, prefix+"SinceActive"))
	assertFloat(t, prefix+"UpperSlope", index, got.UpperSlope[index], valueAt(t, row, columns, prefix+"UpperSlope"))
	assertFloat(t, prefix+"UpperIntercept", index, got.UpperIntercept[index], valueAt(t, row, columns, prefix+"UpperIntercept"))
	assertFloat(t, prefix+"LowerSlope", index, got.LowerSlope[index], valueAt(t, row, columns, prefix+"LowerSlope"))
	assertFloat(t, prefix+"LowerIntercept", index, got.LowerIntercept[index], valueAt(t, row, columns, prefix+"LowerIntercept"))
}
