# Strat server runtime

This package contains the native Go Strat parser, configuration types,
diagnostics, generated grammar tables, and parser tests. It has no dependency
on the generic backtest engine or market-data packages.

Native callers that still import `backtester/go/dsl` use its compatibility
facade, which aliases this package's public types and forwards its entry
points. Keep parser-visible changes and conformance checks in this package;
engine execution lives under `go/native`, with `go/engine`
retained as a compatibility facade for existing native callers.
