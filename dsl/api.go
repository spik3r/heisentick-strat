// Package serverruntime contains the native Go implementation of the Strat
// parser and language-facing configuration types.
package dsl

// Parse compiles Strat source into the conformance-checked configuration and
// diagnostic envelope.
func Parse(source string) (ParseResult, error) {
	return parse(source)
}

// HigherTimeframe returns the automatic higher timeframe used by the Strat
// language routing rules.
func HigherTimeframe(tf string) string {
	return higherTimeframe(tf)
}

// ResolveHigherTimeframe mirrors the Strat language's higher-timeframe
// selection rules.
func ResolveHigherTimeframe(tf string, requested string) string {
	return resolveHigherTimeframe(tf, requested)
}
