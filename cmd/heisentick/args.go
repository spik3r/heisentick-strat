package main

import (
	"fmt"
	"strconv"
	"strings"
)

type flagSet map[string][]string

func parseFlags(args []string) (flagSet, error) {
	flags := make(flagSet)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			return nil, fmt.Errorf("unexpected positional argument %q", arg)
		}
		body := strings.TrimPrefix(arg, "--")
		if body == "" {
			return nil, fmt.Errorf("empty flag")
		}
		key, value, hasValue := strings.Cut(body, "=")
		if key == "" {
			return nil, fmt.Errorf("empty flag name")
		}
		if !hasValue {
			value = "1"
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
				value = args[i]
			}
		}
		flags[key] = append(flags[key], value)
	}
	return flags, nil
}

func (f flagSet) required(name string) (string, error) {
	value := f.one(name, "")
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("missing --%s", name)
	}
	return value, nil
}

func (f flagSet) one(name string, fallback string) string {
	values := f[name]
	if len(values) == 0 {
		return fallback
	}
	return values[len(values)-1]
}

func (f flagSet) all(name string) []string {
	values := f[name]
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func boolFlag(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseFloatFlag(name string, raw string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid --%s value %q", name, raw)
	}
	return value, nil
}

func validateRangeMethod(value string) error {
	if value != "zone" && value != "pivot" {
		return fmt.Errorf("invalid --range %q: expected zone or pivot", value)
	}
	return nil
}
