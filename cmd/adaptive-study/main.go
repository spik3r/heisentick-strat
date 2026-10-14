// adaptive-study is an offline assumed-model CLI; no production/broker registration.
package main

import (
	"encoding/json"
	"fmt"
	"github.com/spik3r/heisentick-strat/engine/adaptivestudy"
	"io"
	"os"
)

func run(args []string, out io.Writer) error {
	if len(args) != 5 {
		return fmt.Errorf("usage: adaptive-study <pinned-M30.btb1> <development|2024|2025|2026|train2024|train2025|train2026> <B0|B1|B2> <GROSS|BASIC|HARSH|SOURCE_FEE> <none|one-tick|target-first|skip-entry-gap>")
	}
	start, end, e := adaptivestudy.StudyWindow(args[1])
	if e != nil {
		return e
	}
	bars, e := adaptivestudy.LoadM30(args[0])
	if e != nil {
		return e
	}
	r, e := adaptivestudy.Run(bars, adaptivestudy.Config{StartT: start, EndT: end, Candidate: args[2], Cost: args[3], Stress: args[4]})
	if e != nil {
		return e
	}
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	_, e = out.Write(append(b, '\n'))
	return e
}
func main() {
	if e := run(os.Args[1:], os.Stdout); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
}
