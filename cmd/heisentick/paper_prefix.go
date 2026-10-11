package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spik3r/heisentick-strat/engine"
)

func runPaperPrefix(args []string, out io.Writer) error {
	if len(args) != 1 || !strings.HasPrefix(args[0], "--request-file=") || len(args[0]) == len("--request-file=") {
		return fmt.Errorf("PAPER_PREFIX_ARGUMENTS")
	}
	file, err := os.Open(strings.TrimPrefix(args[0], "--request-file="))
	if err != nil {
		return fmt.Errorf("PAPER_PREFIX_INPUT")
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, engine.PaperPrefixMaxBytes+1))
	if err != nil {
		return fmt.Errorf("PAPER_PREFIX_INPUT")
	}
	request, err := engine.DecodePaperPrefixRequest(raw)
	if err != nil {
		return err
	}
	result, err := engine.RunPaperPrefix(request)
	if err != nil {
		return err
	}
	encoded, err := engine.EncodePaperPrefixResult(result)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if _, err := out.Write(encoded); err != nil {
		return fmt.Errorf("PAPER_PREFIX_OUTPUT")
	}
	return nil
}
