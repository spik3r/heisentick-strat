//go:build !js || !wasm

package main

import (
	"fmt"
	"os"
)

func main() {
	interactive := len(os.Args) == 4 && os.Args[1] == "--interactive"
	if !interactive && len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: enginewasm [--interactive] fixture.json source.strat")
		os.Exit(2)
	}
	offset := 1
	if interactive {
		offset = 2
	}
	raw, err := os.ReadFile(os.Args[offset])
	if err != nil {
		panic(err)
	}
	source, err := os.ReadFile(os.Args[offset+1])
	if err != nil {
		panic(err)
	}
	if interactive {
		out := runInteractive(string(raw), string(source))
		fmt.Println(string(out))
		return
	}
	out, err := runFixture(string(raw), string(source))
	if err != nil {
		panic(err)
	}
	fmt.Println(string(out))
}
