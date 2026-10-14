package main

import (
	"encoding/json"
	"fmt"
	"github.com/spik3r/heisentick-strat/internal/adaptivemc"
	"os"
	"path/filepath"
	"runtime"
)

func run(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: adaptive-study-bootstrap <12-development-receipts-request.json> <new-output-directory>")
	}
	if runtime.Version() != "go1.22.12" {
		return fmt.Errorf("pinned go1.22.12 runtime required")
	}
	r, e := adaptivemc.ReadRequest(args[0])
	if e != nil {
		return e
	}
	streams, e := adaptivemc.ReadStreams(r)
	if e != nil {
		return e
	}
	report, matrices, e := adaptivemc.Analyze(streams)
	if e != nil {
		return e
	}
	if e = os.Mkdir(args[1], 0700); e != nil {
		return e
	}
	for _, m := range matrices {
		raw, _, e := adaptivemc.MatrixBytes(m)
		if e != nil {
			return e
		}
		if e = os.WriteFile(filepath.Join(args[1], fmt.Sprintf("indices-block-%d.json", m.Block)), raw, 0600); e != nil {
			return e
		}
	}
	raw, e := json.Marshal(report)
	if e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(args[1], "REPORT.json"), append(raw, '\n'), 0600)
}
func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
}
