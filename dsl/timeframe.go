package dsl

import "strings"

func higherTimeframe(tf string) string {
	switch strings.ToLower(strings.TrimSpace(tf)) {
	case "1m", "5m", "15m":
		return "1h"
	case "1h":
		return "4h"
	case "4h":
		return "1d"
	default:
		return ""
	}
}

func resolveHigherTimeframe(tf string, requested string) string {
	chartTF := strings.ToLower(strings.TrimSpace(tf))
	value := strings.ToLower(strings.TrimSpace(requested))
	if value == "" {
		value = "auto"
	}
	if value == "auto" {
		return higherTimeframe(chartTF)
	}
	if value == "none" || value == chartTF {
		return ""
	}
	return value
}
