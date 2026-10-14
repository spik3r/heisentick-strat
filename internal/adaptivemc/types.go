// Package adaptivemc provides post-lock descriptive observed-date resampling.
// It never selects strategies, changes engine parameters or forecasts returns.
package adaptivemc

const Seed int64 = 2872026
const Replicates = 2000
const StudyHead = "fe267e72b6194adbfc4af69264b7df087fb2950f"
const Profile = "regime-adaptive-v4.2-go-flat-risk-study-v2"
const DataHash = "a04fad3f6daa2b53addcd4b8b6fe6ba9c49afd329b7785d167724763d628d92f"
const SourceHash = "bf40faa4f320f64c555fb38689128449c9967b4f97bc42de9af23e11516b7da7"

type Mark struct {
	T      int64   `json:"t"`
	Equity float64 `json:"equity"`
	Gap    bool    `json:"gap"`
}
type Day struct {
	Date                string  `json:"date"`
	Marks               int     `json:"marks"`
	Gap                 bool    `json:"gap"`
	End                 float64 `json:"endFactor"`
	Max                 float64 `json:"maxFactor"`
	Min                 float64 `json:"minFactor"`
	DD                  float64 `json:"withinDayMaxDrawdown"`
	CapitalLoss         bool    `json:"capitalLoss"`
	UnclippedLossFactor float64 `json:"unclippedLossFactor"`
	ActualEnd           float64 `json:"actualEndFactor"`
}
type Stream struct {
	ID        string `json:"id"`
	InputHash string `json:"inputSha256"`
	Days      []Day  `json:"days"`
}
type Input struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type Request struct {
	Schema    string  `json:"schema"`
	StudyHead string  `json:"studyHead"`
	Inputs    []Input `json:"inputs"`
}
type Matrix struct {
	Schema     string  `json:"schema"`
	Days       int     `json:"days"`
	Block      int     `json:"block"`
	Replicates int     `json:"replicates"`
	Seed       int64   `json:"seed"`
	Indices    [][]int `json:"indices"`
}
type Outcome struct {
	Return              float64 `json:"return"`
	DD                  float64 `json:"m30MarkedMaxDrawdown"`
	CapitalLoss         bool    `json:"capitalLoss"`
	UnclippedLossFactor float64 `json:"unclippedLossFactor"`
	UsedDays            int     `json:"usedDays"`
}
type Quantiles struct {
	P05 float64 `json:"p05"`
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
}
type Frequency struct {
	Count    int     `json:"count"`
	N        int     `json:"n"`
	Fraction float64 `json:"fraction"`
	Low      float64 `json:"wilson95Low"`
	High     float64 `json:"wilson95High"`
	Meaning  string  `json:"meaning"`
}
type Stats struct {
	Return      Quantiles `json:"return"`
	DD          Quantiles `json:"m30MarkedMaxDrawdown"`
	Loss        Frequency `json:"loss"`
	DD10        Frequency `json:"drawdownAbove10Percent"`
	DD20        Frequency `json:"drawdownAbove20Percent"`
	CapitalLoss Frequency `json:"capitalLoss"`
}
type Paired struct {
	ID               string    `json:"id"`
	ReturnDifference Quantiles `json:"returnDifference"`
	DDDifference     Quantiles `json:"drawdownDifference"`
}
type BlockReport struct {
	Block       int              `json:"block"`
	IndexSHA256 string           `json:"indexSha256"`
	Stats       map[string]Stats `json:"streams"`
	Paired      []Paired         `json:"pairedDifferences"`
}
type Report struct {
	Schema      string             `json:"schema"`
	Status      string             `json:"status"`
	StudyHead   string             `json:"studyHead"`
	GoVersion   string             `json:"goVersion"`
	Days        int                `json:"observedUtcDates"`
	Replicates  int                `json:"replicates"`
	Seed        int64              `json:"seed"`
	Original    map[string]Outcome `json:"originalPaths"`
	Streams     []Stream           `json:"sourceDayTuples"`
	Blocks      []BlockReport      `json:"blocks"`
	Limitations []string           `json:"limitations"`
}
