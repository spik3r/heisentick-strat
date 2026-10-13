package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/report"
)

// The dedicated command accepts only exact request/source files. It writes one
// complete bounded JSON line on success or refusal, never a partial success.
func runSupplyDemandRetestReport(args []string, out io.Writer) error {
	send := func(raw []byte) error {
		raw = append(raw, '\n')
		n, err := out.Write(raw)
		if err != nil {
			return err
		}
		if n != len(raw) {
			return io.ErrShortWrite
		}
		return nil
	}
	refusal := func(code, field string) error { return send(report.SupplyDemandRetestFailure(code, field)) }
	paths := map[string]string{}
	if len(args) != 2 {
		return refusal("invalid-arguments", "arguments")
	}
	for _, arg := range args {
		name, path, ok := strings.Cut(arg, "=")
		if !ok || path == "" || (name != "--request-file" && name != "--dsl-file") || paths[name] != "" {
			return refusal("invalid-arguments", "arguments")
		}
		paths[name] = path
	}
	if len(paths) != 2 {
		return refusal("invalid-arguments", "arguments")
	}
	raw, err := readSupplyDemandRetestFile(paths["--request-file"], engine.SupplyDemandRetestMaxRequestBytes)
	if err != nil {
		return refusal("input-unavailable", "request")
	}
	source, err := readSupplyDemandRetestFile(paths["--dsl-file"], engine.SupplyDemandRetestMaxSourceBytes)
	if err != nil {
		return refusal("input-unavailable", "source")
	}
	return send(report.RunSupplyDemandRetestReport(string(raw), string(source)))
}

func readSupplyDemandRetestFile(path string, max int) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > int64(max) {
		return nil, fmt.Errorf("invalid bounded input file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > int64(max) {
		return nil, fmt.Errorf("invalid bounded input file")
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(max)+1))
	if err != nil || len(data) > max {
		return nil, fmt.Errorf("invalid bounded input file")
	}
	return data, nil
}
