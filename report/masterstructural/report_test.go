package masterstructural

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine/master"
	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/testsupport"
)

func sourceFor(mode string) string {
	return `dsl v7
strategy "Invented runtime fixture" { description "Synthetic OHLCV only" }
market { master timeframe M30 from M5 }
setup { type: master structural
master profile v10-phase0-floor-half-reference-v1
master mode ` + mode + `
}`
}

func inventedRows(native int) marketdata.Series {
	s := marketdata.NewSeries(native * 6)
	previous := 100.
	for i := 0; i < native; i++ {
		close := 100 + .015*float64(i) + 4*math.Sin(float64(i)*.13) + .8*math.Sin(float64(i)*.71)
		for j := 0; j < 6; j++ {
			k := i*6 + j
			s.T[k] = float64(k) * float64(master.M5MS)
			s.O[k] = previous + (close-previous)*float64(j)/6
			s.C[k] = previous + (close-previous)*float64(j+1)/6
			s.H[k] = math.Max(s.O[k], s.C[k]) + .08
			s.L[k] = math.Min(s.O[k], s.C[k]) - .08
			s.V[k] = 10 + float64((i*17)%23)
		}
		previous = close
	}
	return s
}

func requestJSON(options Options) string {
	raw, _ := json.Marshal(map[string]any{"schema": RuntimeSchema, "warmupFromT": options.WarmupFromT, "tradeFromT": options.TradeFromT, "tradeToT": options.TradeToT, "spread": options.Spread})
	return string(raw)
}

func TestSharedReportPreservesOriginalEnvelope(t *testing.T) {
	for _, mode := range []string{master.SourceMode, master.ProtectedMode} {
		for _, spread := range []float64{0, 1} {
			source := sourceFor(mode)
			series := inventedRows(500)
			data := marketdata.EncodeBTB1(series)
			options := Options{TradeFromT: 40 * master.M30MS, TradeToT: 500 * master.M30MS, Spread: spread}
			parsed, err := dsl.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			run, err := master.Run(master.Request{Config: parsed.Config, M5: series, TradeFromT: options.TradeFromT, TradeToT: options.TradeToT, Costs: master.Costs{Spread: spread, FeePerUnitSide: .5, InitialEquity: 10000}})
			if err != nil {
				t.Fatal(err)
			}
			if len(run.Trades) == 0 {
				t.Fatal("missing synthetic execution control")
			}
			cfg, _ := json.Marshal(parsed.Config)
			// This is the original CLI envelope, independently reassembled.
			original := struct {
				Schema     string        `json:"schema"`
				DSLHash    string        `json:"dslSha256"`
				ConfigHash string        `json:"configSha256"`
				DataHash   string        `json:"dataSha256"`
				Config     dsl.Config    `json:"config"`
				Run        master.Result `json:"run"`
			}{"strat-master-structural-cli-v1", hash([]byte(source)), hash(cfg), hash(data), parsed.Config, run}
			want, _ := json.MarshalIndent(original, "", "  ")
			want = append(want, '\n')
			for _, runtime := range []bool{false, true} {
				var got []byte
				if runtime {
					got, err = BuildRuntime(requestJSON(options), source, data)
				} else {
					got, err = Build([]byte(source), data, options)
				}
				if err != nil || !bytes.Equal(got, want) {
					t.Fatalf("mode %s spread %v runtime %v: %v byte identity=%v", mode, spread, runtime, err, bytes.Equal(got, want))
				}
			}
		}
	}
}

func TestNativeSourceLimitUnchanged(t *testing.T) {
	source := sourceFor(master.SourceMode) + "\n# " + strings.Repeat("x", MaxSourceBytes)
	data := marketdata.EncodeBTB1(inventedRows(50))
	options := Options{TradeFromT: 40 * master.M30MS, TradeToT: 50 * master.M30MS}
	if raw, err := Build([]byte(source), data, options); err != nil || len(raw) == 0 {
		t.Fatalf("native contract changed: %v", err)
	}
	if raw, err := BuildRuntime(requestJSON(options), source, data); err == nil || raw != nil {
		t.Fatal("runtime source limit absent")
	}
}

// The cardinality/string proof depends on the frozen output constructors, not
// merely the current shape of a few happy-path reports. Any source change here
// requires explicit re-review of that proof, rather than silently retaining it.
func TestOutputBoundFrozenConstructorContracts(t *testing.T) {
	files := map[string]string{
		"engine/master/execute.go":              "77fda0c3049e6f77fc82d23646b7626b602879c602f791d139e959220a5e29cf",
		"engine/master/run.go":                  "b0751ddde971d9f2a33ab5efe19947f801877aabcb79a6317cda5b2f542f7de6",
		"engine/master/types.go":                "0cf67ba2d7fe3fb3f97bc56cec09d6ee7185f61a3a0d28807a34c090f6a9cc7f",
		"engine/master/indicators.go":           "75b71330c471cb0e3f3937a9146e15392c826c858e9fee882873be4d8b55c624",
		"engine/regime/indicators.go":           "7df6d3d050900cd61a0296f39719e51cccbc70db7adb683a5349a336339ffa11",
		"engine/regime/types.go":                "a5009107434e7d0ce7ebd364b6c068b368a017cfc00aebd5e9074f9d40c22fd1",
		"engine/regime/reference_primitives.go": "2b4cfa732413442e51b07761fe7e6e1758faf2f15c5e05ec9172a66592cdb551",
		"engine/regime/run.go":                  "a20c91f0d11c0f7ded4208bfbfb1bdc9b0acd03df2ed99fcc70106365d324712",
	}
	for path, want := range files {
		raw, err := os.ReadFile(filepath.Join(testsupport.MustRepoRoot(), path))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != want {
			t.Errorf("%s changed: re-review runtime output cardinality, string and allocation bounds", path)
		}
	}
}

func TestMaximumJSONCoversWorstEscapingAndShape(t *testing.T) {
	slices := map[reflect.Type]uint64{}
	var fill func(reflect.Value)
	fill = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer:
			v.Set(reflect.New(v.Type().Elem()))
			fill(v.Elem())
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				fill(v.Field(i))
			}
		case reflect.Float64:
			v.SetFloat(-1.2345678901234567e+308)
		case reflect.Int, reflect.Int64:
			v.SetInt(-9223372036854775807)
		case reflect.Bool:
			v.SetBool(false)
		case reflect.String:
			v.SetString(strings.Repeat("<", 256))
		case reflect.Slice:
			slices[v.Type()] = 2
			v.Set(reflect.MakeSlice(v.Type(), 2, 2))
			for i := 0; i < 2; i++ {
				fill(v.Index(i))
			}
		default:
			t.Fatalf("unreviewed type %s", v.Type())
		}
	}
	value := reflect.New(reflect.TypeOf(master.Result{})).Elem()
	fill(value)
	bound, err := maximumJSON(value.Type(), 1, slices)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(value.Interface(), "  ", "  ")
	if err != nil || uint64(len(raw)) > bound {
		t.Fatalf("bound %d actual %d: %v", bound, len(raw), err)
	}
	if _, err = maximumJSON(reflect.TypeOf(map[string]string{}), 0, slices); err == nil {
		t.Fatal("dynamic map silently bounded")
	}
	if _, err = maximumJSON(reflect.TypeOf([]int{}), 0, slices); err == nil {
		t.Fatal("new slice silently bounded")
	}
}
