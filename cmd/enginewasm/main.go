//go:build !js || !wasm

package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--sequential-backtest" {
		os.Exit(sequentialMain(os.Args[2:]))
	}
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: enginewasm fixture.json source.strat")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	source, err := os.ReadFile(os.Args[2])
	if err != nil {
		panic(err)
	}
	out, err := runFixture(string(raw), string(source))
	if err != nil {
		panic(err)
	}
	fmt.Println(string(out))
}

// Explicit native parity dispatch only; the ordinary two-file CLI is unchanged.
func sequentialMain(args []string) int {
	if len(args) != 2 {
		fmt.Println(string(sequentialFailure(sequentialInvalid("arguments", "expected --sequential-backtest fixture.json source.strat"))))
		return 2
	}
	raw, err := readSequentialInput(args[0], sequentialMaxFixture)
	if err != nil {
		fmt.Println(string(sequentialFailure(sequentialInvalid("fixture", err.Error()))))
		return 1
	}
	source, err := readSequentialInput(args[1], sequentialMaxSource)
	if err != nil {
		fmt.Println(string(sequentialFailure(&sequentialInputError{"SEQUENTIAL_DSL_INVALID", "source", err.Error()})))
		return 1
	}
	out, err := runSequentialFixture(string(raw), string(source))
	if err != nil {
		fmt.Println(string(sequentialFailure(err)))
		return 1
	}
	fmt.Println(string(out))
	return 0
}

func readSequentialInput(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, limit+1))
}
