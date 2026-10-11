package main

import (
	"encoding/json"
	"fmt"
	"github.com/spik3r/heisentick-strat/engine"
	"io"
	"os"
	"strings"
)

func runSequentialInspection(args []string, out io.Writer) error {
	if len(args) != 1 || !strings.HasPrefix(args[0], "--dsl-file=") || len(args[0]) == len("--dsl-file=") {
		return fmt.Errorf("SEQUENTIAL_INSPECTION_ARGUMENTS: expected exactly --dsl-file=<path>")
	}
	file, err := os.Open(strings.TrimPrefix(args[0], "--dsl-file="))
	if err != nil {
		return writeSequentialInspection(out, engine.SequentialInspectionFailure("SEQUENTIAL_INSPECTION_INPUT"))
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, engine.SequentialInspectionMaxSource+1))
	if err != nil {
		return writeSequentialInspection(out, engine.SequentialInspectionFailure("SEQUENTIAL_INSPECTION_INPUT"))
	}
	return writeSequentialInspection(out, engine.InspectSequentialSource(string(raw)))
}

func writeSequentialInspection(out io.Writer, result engine.SequentialSourceInspection) error {
	raw, err := json.Marshal(result)
	if err != nil {
		result = engine.SequentialInspectionFailure("SEQUENTIAL_INSPECTION_SERIALIZATION")
		raw, _ = json.Marshal(result)
	}
	if _, err := fmt.Fprintln(out, string(raw)); err != nil {
		return fmt.Errorf("SEQUENTIAL_INSPECTION_OUTPUT")
	}
	if result.Error != nil {
		return fmt.Errorf("%s", result.Error.Code)
	}
	return nil
}
