package dsl

import "strings"

// Extra draft syntax checks prevent permissive historical normalization from
// silently discarding user intent. Family-specific audits still own vocabulary.
func (p *parser) validateStrictSourceLine(line logicalLine, tokens []string, head string) {
	switch head {
	case "dsl":
		p.strictVersions++
		if len(tokens) != 2 || !strings.EqualFold(tokens[1], "v7") {
			p.err(line, "strict source requires exactly dsl v7", "")
		}
	case "type":
		p.strictTypes++
	case "slices":
		p.strictSlices++
		if len(tokens) < 3 || len(tokens)%2 != 1 {
			p.err(line, "strict slices requires complete nonempty symbol/timeframe pairs", "")
		}
		for i := 2; i < len(tokens); i += 2 {
			switch strings.ToLower(tokens[i]) {
			case "1m", "5m", "15m", "30m", "1h", "4h", "1d":
			default:
				p.err(line, "strict slices has an unsupported timeframe", "")
			}
		}
	case "side":
		if len(tokens) < 2 || len(tokens) > 3 || !strings.EqualFold(tokens[1], "long") ||
			(len(tokens) == 3 && !strings.EqualFold(tokens[2], "only")) {
			p.err(line, "strict source supports exactly side long or side long only", "")
		}
	}
	if head == "sma" || head == "ema" || head == "stop" || head == "trail" || head == "fallback" || head == "risk" || head == "riskusd" {
		for _, token := range tokens[1:] {
			if isNumberToken(token) {
				if _, ok := parseSMAFiniteNumber(token); !ok {
					p.err(line, "strict numeric tokens require finite decimal numbers without suffixes", "")
				}
			}
		}
	}
}
