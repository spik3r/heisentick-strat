package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
)

const (
	parseGoldenSchema = "dsl-conformance-parse-v1"
	metadataFile      = "metadata.json"
)

// golden is one file the generator owns: its path and the exact bytes the
// engine produces for it.
type golden struct {
	path    string
	content []byte
}

// runCase is one conformance/run fixture with its scoreboard status.
type runCase struct {
	name       string
	todoReason string
	todo       bool
}

// corpus is everything regen writes and check compares.
type corpus struct {
	goldens    []golden
	parseCases int
	runCases   []runCase
}

func (c corpus) implemented() []string {
	var out []string
	for _, rc := range c.runCases {
		if !rc.todo {
			out = append(out, rc.name)
		}
	}
	return out
}

func (c corpus) todo() map[string]string {
	out := map[string]string{}
	for _, rc := range c.runCases {
		if rc.todo {
			out[rc.name] = rc.todoReason
		}
	}
	return out
}

func regen(dir string, out io.Writer) error {
	c, err := generate(dir)
	if err != nil {
		return err
	}
	for _, g := range c.goldens {
		if err := os.WriteFile(g.path, g.content, 0o644); err != nil {
			return err
		}
	}
	for _, rc := range c.runCases {
		if rc.todo {
			fmt.Fprintf(out, "skipped %s: TODO (%s); no golden written\n", rc.name, rc.todoReason)
		}
	}
	fmt.Fprintf(out, "wrote %d parse goldens, %d run goldens (%d TODO) and %s\n",
		c.parseCases, len(c.implemented()), len(c.todo()), filepath.Join(dir, metadataFile))
	return nil
}

func check(dir string, out io.Writer) error {
	c, err := generate(dir)
	if err != nil {
		return err
	}
	var problems []string
	for _, g := range c.goldens {
		actual, err := os.ReadFile(g.path)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", g.path, err))
			continue
		}
		if !bytes.Equal(actual, g.content) {
			problems = append(problems, fmt.Sprintf("%s differs from the engine's output (%s)", g.path, firstDiff(actual, g.content)))
		}
	}
	for _, rc := range c.runCases {
		if !rc.todo {
			continue
		}
		path := filepath.Join(dir, "run", rc.name+".trades.json")
		if _, err := os.Stat(path); err == nil {
			problems = append(problems, fmt.Sprintf("%s exists but the case is TODO (%s); nothing generates it", path, rc.todoReason))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("corpus differs from the Go engine:\n- %s\n\nclassify the difference (Go bug, semantics change, fixture change), fix it at the source, then `go run ./cmd/conformance regen` in a reviewed PR", strings.Join(problems, "\n- "))
	}
	fmt.Fprintf(out, "conformance OK: %d parse goldens, %d run goldens, %d TODO run cases match the Go engine\n",
		c.parseCases, len(c.implemented()), len(c.todo()))
	return nil
}

func list(dir string, out io.Writer) error {
	c, err := generate(dir)
	if err != nil {
		return err
	}
	for _, rc := range c.runCases {
		if rc.todo {
			fmt.Fprintf(out, "%-60s todo: %s\n", rc.name, rc.todoReason)
		} else {
			fmt.Fprintf(out, "%-60s implemented\n", rc.name)
		}
	}
	fmt.Fprintf(out, "%d run cases: %d implemented, %d TODO; %d parse cases\n",
		len(c.runCases), len(c.implemented()), len(c.todo()), c.parseCases)
	return nil
}

func generate(dir string) (corpus, error) {
	var c corpus
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return c, fmt.Errorf("%s is not a directory; run from the repository root or pass --dir", dir)
	}
	parseGoldens, categories, err := generateParse(filepath.Join(dir, "parse"))
	if err != nil {
		return c, err
	}
	c.goldens = append(c.goldens, parseGoldens...)
	c.parseCases = len(parseGoldens)

	runGoldens, runCases, err := generateRun(filepath.Join(dir, "run"))
	if err != nil {
		return c, err
	}
	c.goldens = append(c.goldens, runGoldens...)
	c.runCases = runCases

	metadata, err := generateMetadata(filepath.Join(dir, metadataFile), c, categories)
	if err != nil {
		return c, err
	}
	c.goldens = append(c.goldens, metadata)
	return c, nil
}

// generateParse produces <case>.cfg.json for every <case>.strat. The header
// fields (case, category, description, source) describe where the input came
// from and are kept from the existing golden; the parser output replaces the
// rest.
func generateParse(dir string) ([]golden, map[string]int, error) {
	stratPaths, err := filepath.Glob(filepath.Join(dir, "*.strat"))
	if err != nil {
		return nil, nil, err
	}
	if len(stratPaths) == 0 {
		return nil, nil, fmt.Errorf("no parse cases found in %s", dir)
	}
	sort.Strings(stratPaths)
	categories := map[string]int{}
	goldens := make([]golden, 0, len(stratPaths))
	for _, stratPath := range stratPaths {
		caseName := strings.TrimSuffix(filepath.Base(stratPath), ".strat")
		goldenPath := filepath.Join(dir, caseName+".cfg.json")
		header, err := readParseHeader(goldenPath, caseName)
		if err != nil {
			return nil, nil, err
		}
		source, err := os.ReadFile(stratPath)
		if err != nil {
			return nil, nil, err
		}
		result, err := dsl.Parse(string(source))
		if err != nil {
			return nil, nil, fmt.Errorf("%s: parse: %w", caseName, err)
		}
		payload := map[string]any{
			"schema":      parseGoldenSchema,
			"case":        caseName,
			"category":    header["category"],
			"description": header["description"],
			"source":      header["source"],
			"cfg":         result.Config,
			"errors":      nonNil(result.Errors),
			"warnings":    nonNil(result.Warnings),
			"diagnostics": result.Diagnostics,
		}
		if result.Diagnostics == nil {
			payload["diagnostics"] = []dsl.Diagnostic{}
		}
		content, err := stableJSON(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: encode: %w", caseName, err)
		}
		var category string
		if err := json.Unmarshal(header["category"], &category); err != nil || category == "" {
			return nil, nil, fmt.Errorf("%s: category must be a non-empty string", goldenPath)
		}
		categories[category]++
		goldens = append(goldens, golden{path: goldenPath, content: content})
	}
	return goldens, categories, nil
}

// readParseHeader returns the descriptive fields of an existing parse golden.
// A new case starts as a header-only file next to its .strat input.
func readParseHeader(path string, caseName string) (map[string]json.RawMessage, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w (a new parse case needs a header file with case, category, description and source; see conformance/README.md)", path, err)
	}
	var header map[string]json.RawMessage
	if err := json.Unmarshal(raw, &header); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for _, key := range []string{"case", "category", "description", "source"} {
		if len(header[key]) == 0 {
			return nil, fmt.Errorf("%s: missing header field %q", path, key)
		}
	}
	var declared string
	if err := json.Unmarshal(header["case"], &declared); err != nil || declared != caseName {
		return nil, fmt.Errorf("%s: case field %s does not match the file name %q", path, header["case"], caseName)
	}
	return header, nil
}

// generateRun produces <case>.trades.json for every <case>.fixture.json the
// engine implements. TODO cases (engine/run_todo.go) get no golden; a case in
// neither list is an error, so a new fixture is classified before it lands.
func generateRun(dir string) ([]golden, []runCase, error) {
	fixturePaths, err := filepath.Glob(filepath.Join(dir, "*.fixture.json"))
	if err != nil {
		return nil, nil, err
	}
	if len(fixturePaths) == 0 {
		return nil, nil, fmt.Errorf("no run fixtures found in %s", dir)
	}
	sort.Strings(fixturePaths)
	implemented := map[string]bool{}
	for _, name := range engine.ImplementedRunCases() {
		implemented[name] = true
	}
	todo := engine.RunConformanceTODO()
	seen := map[string]bool{}
	goldens := make([]golden, 0, len(fixturePaths))
	cases := make([]runCase, 0, len(fixturePaths))
	for _, fixturePath := range fixturePaths {
		caseName := strings.TrimSuffix(filepath.Base(fixturePath), ".fixture.json")
		seen[caseName] = true
		if reason, ok := todo[caseName]; ok {
			if implemented[caseName] {
				return nil, nil, fmt.Errorf("%s is listed as both implemented and TODO", caseName)
			}
			cases = append(cases, runCase{name: caseName, todo: true, todoReason: reason})
			continue
		}
		if !implemented[caseName] {
			return nil, nil, fmt.Errorf("%s is neither in engine.ImplementedRunCases nor in engine/run_todo.go; classify it first", caseName)
		}
		fixture, err := engine.LoadRunFixture(fixturePath)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: load fixture: %w", caseName, err)
		}
		source, err := os.ReadFile(filepath.Join(dir, caseName+".strat"))
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", caseName, err)
		}
		result, err := engine.RunFixtureCase(fixture, string(source))
		if err != nil {
			return nil, nil, fmt.Errorf("%s: run: %w", caseName, err)
		}
		content, err := stableJSON(engine.ConformanceProjection(result))
		if err != nil {
			return nil, nil, fmt.Errorf("%s: encode: %w", caseName, err)
		}
		goldens = append(goldens, golden{path: filepath.Join(dir, caseName+".trades.json"), content: content})
		cases = append(cases, runCase{name: caseName})
	}
	for caseName := range todo {
		if !seen[caseName] {
			return nil, nil, fmt.Errorf("engine/run_todo.go lists %q but %s has no such fixture", caseName, dir)
		}
	}
	for caseName := range implemented {
		if !seen[caseName] {
			return nil, nil, fmt.Errorf("engine.ImplementedRunCases lists %q but %s has no such fixture", caseName, dir)
		}
	}
	return goldens, cases, nil
}

// generateMetadata rewrites metadata.json with the counts and the Go run
// scoreboard. Fields the generator does not own (notes, missingDeployedDsl)
// are kept as they are.
func generateMetadata(path string, c corpus, categories map[string]int) (golden, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return golden{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var metadata map[string]any
	if err := decoder.Decode(&metadata); err != nil {
		return golden{}, fmt.Errorf("%s: %w", path, err)
	}
	implemented := c.implemented()
	sort.Strings(implemented)
	metadata["parseCases"] = c.parseCases
	metadata["parseCategories"] = categories
	metadata["runCases"] = len(c.runCases)
	metadata["goRun"] = map[string]any{
		"implemented": implemented,
		"todo":        c.todo(),
	}
	content, err := stableJSON(metadata)
	if err != nil {
		return golden{}, err
	}
	return golden{path: path, content: content}, nil
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// stableJSON encodes a value the way the corpus has always been written:
// object keys sorted, two-space indent, no HTML escaping, trailing newline.
// Numbers keep the shortest round-trip form encoding/json produces.
func stableJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var generic any
	if err := decoder.Decode(&generic); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(generic); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func firstDiff(got []byte, want []byte) string {
	limit := len(got)
	if len(want) < limit {
		limit = len(want)
	}
	for i := 0; i < limit; i++ {
		if got[i] != want[i] {
			start := i - 40
			if start < 0 {
				start = 0
			}
			end := func(n int) int {
				if i+80 < n {
					return i + 80
				}
				return n
			}
			return fmt.Sprintf("byte %d: on disk %q, engine %q", i, got[start:end(len(got))], want[start:end(len(want))])
		}
	}
	return fmt.Sprintf("length on disk %d, engine %d", len(got), len(want))
}
