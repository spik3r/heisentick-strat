package main

import (
	"encoding/json"
	"fmt"
	"github.com/spik3r/heisentick-strat/engine/regimeadaptive"
	"io"
	"os"
)

func run(args []string, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: regime-adaptive-inspect <synthetic-request.json>; execution is unsupported")
	}
	raw, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	req, err := regimeadaptive.Decode(raw)
	if err != nil {
		return err
	}
	result, err := regimeadaptive.Inspect(req)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = out.Write(append(encoded, '\n'))
	return err
}
func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
