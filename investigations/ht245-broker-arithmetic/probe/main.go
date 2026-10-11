// Command probe builds context columns for one frozen fixture with channels on
// and prints, for every float64 column, a hash of the bit patterns and the
// first non-zero index. Run it on two targets and diff the output to find the
// first column where they disagree. Read-only use of the exported API.
//
//	probe -fixture <file> [-scale 1.7]
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"os"
	"reflect"
	"strconv"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func main() {
	path := flag.String("fixture", "", "run fixture")
	scale := flag.String("scale", "1", "exact decimal multiplier")
	flag.Parse()
	fx, err := engine.LoadRunFixture(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	k, _ := strconv.ParseFloat(*scale, 64)
	for i := range fx.Bars {
		fx.Bars[i].O *= k
		fx.Bars[i].H *= k
		fx.Bars[i].L *= k
		fx.Bars[i].C *= k
	}
	cols := contextcols.Build(marketdata.SeriesFromBars(fx.Bars), contextcols.Options{
		Channel: contextcols.ChannelOptions{Enabled: true}, Range: contextcols.RangeOptions{Method: fx.RangeMethod}})
	walk("", reflect.ValueOf(cols))
}

func walk(prefix string, v reflect.Value) {
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				walk(prefix+"."+v.Type().Field(i).Name, v.Field(i))
			}
		}
	case reflect.Slice:
		if v.Type().Elem().Kind() != reflect.Float64 {
			return
		}
		h := sha256.New()
		first := -1
		for i := 0; i < v.Len(); i++ {
			b := math.Float64bits(v.Index(i).Float())
			var buf [8]byte
			binary.BigEndian.PutUint64(buf[:], b)
			h.Write(buf[:])
			_ = first
		}
		fmt.Printf("%-40s n=%d %x\n", prefix, v.Len(), h.Sum(nil)[:6])
	}
}
