# Strat conformance envelopes

These versioned JSON schemas describe the language-neutral envelopes committed
under `strat/conformance/`. They are conformance records for parser and run
parity, not generic runtime API schemas and not a replacement for the
implementation-owned parser config types.

The schemas intentionally leave `cfg`, `contextOptions`, and setup-specific
trade `meta` as opaque objects. Add a field only when the current JS and Go
outputs already expose it at this boundary. A contract change needs a new
schema version and fixture review; runtime parser code must not depend on a
generic JSON-schema loader.

The focused test validators implement only this checked subset: `$ref`,
`anyOf`, `const`, `enum`, `type`, `minimum`, `minLength`, `minItems`,
`maxItems`, `minProperties`, `required`, `properties`, `items`, and boolean
`additionalProperties`. The annotation keywords `$schema`, `$id`, `title`,
`description`, and `$defs` are allowed. Unsupported keywords fail closed;
`$ref` and `anyOf` cannot carry validation siblings, and object-valued
`additionalProperties` is not supported by these test helpers.
