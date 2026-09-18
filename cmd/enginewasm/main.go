//go:build !js || !wasm

package main

import (
	"fmt"
	"os"
)

func main() {
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
