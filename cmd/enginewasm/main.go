//go:build !js || !wasm

package main

import (
	"encoding/json"
	"fmt"
	"os"

	native "github.com/spik3r/heisentick-strat/engine"
)

func main() {
	if len(os.Args) == 3 && os.Args[1] == "--authored-orb" {
		raw, err := os.ReadFile(os.Args[2])
		if err != nil {
			panic(err)
		}
		out, err := runAuthoredOrb(string(raw))
		if err != nil {
			panic(err)
		}
		fmt.Println(string(out))
		return
	}
	if len(os.Args) == 3 && (os.Args[1] == "--forward-smoke" || os.Args[1] == "--forward-smoke-prefix") {
		raw, err := os.ReadFile(os.Args[2])
		if err != nil {
			panic(err)
		}
		mode := "whole"
		if os.Args[1] == "--forward-smoke-prefix" {
			mode = "prefix"
		}
		var input map[string]any
		if err := json.Unmarshal(raw, &input); err != nil {
			panic(err)
		}
		input["mode"] = mode
		raw, err = json.Marshal(input)
		if err != nil {
			panic(err)
		}
		out, err := runForwardSmokeJSON(string(raw))
		if err != nil {
			panic(err)
		}
		fmt.Println(string(out))
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "--authored-vp-asia-london-wide" {
		raw, err := os.ReadFile(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		result, err := native.RunAuthoredVPAsiaLondonWideInteractive(raw)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		out, err := json.Marshal(result)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(string(out))
		return
	}
	interactive := len(os.Args) == 4 && (os.Args[1] == "--interactive" || os.Args[1] == "--interactive-vp-ny-handoff")
	composition := len(os.Args) == 4 && os.Args[1] == "--composition"
	interactiveComposition := len(os.Args) == 4 && os.Args[1] == "--interactive-composition"
	if !interactive && !composition && !interactiveComposition && len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: enginewasm [--interactive|--interactive-vp-ny-handoff fixture.json source.strat | --composition|--interactive-composition fixture.json child-sources.json | fixture.json source.strat]")
		os.Exit(2)
	}
	offset := 1
	if interactive || composition || interactiveComposition {
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
		if os.Args[1] == "--interactive-vp-ny-handoff" {
			out = runInteractiveVPNYHandoff(string(raw), string(source))
		}
		fmt.Println(string(out))
		return
	}
	if interactiveComposition {
		fmt.Println(string(runInteractiveComposition(string(raw), string(source))))
		return
	}
	if composition {
		out, err := runCompositionFixture(string(raw), string(source))
		if err != nil {
			fmt.Fprintln(os.Stderr, "enginewasm composition:", err)
			os.Exit(1)
		}
		fmt.Println(string(out))
		return
	}
	out, err := runFixture(string(raw), string(source))
	if err != nil {
		panic(err)
	}
	fmt.Println(string(out))
}
