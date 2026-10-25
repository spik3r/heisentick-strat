# Passive authoring metadata v1

`authoring-v1.json` is a static, partial description of the current Go grammar.
It helps an editor offer signatures, defaults and help. **Syntax metadata never
establishes runtime execution support.** A successful parse, a protected bundle
or an example is not an engine capability, warmup, data or admission guarantee.
Use the actual pinned parser for validation and the engine's own admission path
for execution.

Only the six `smaGoldenCross` setup directives below have typed descriptions.
`coverage: "partial"` and `unlisted: "unknown"` are mandatory. The section and
family inventories mirror the manifest; `unknown` means no typed metadata, not
invalid syntax or unsupported execution. Shared directives in the complete
examples (strategy, slices, side) remain outside this typed slice. Only DSL v7
is described and checked here; other versions are unknown to this descriptor.

## Reading the contract

Validate against `schemas/authoring-v1.schema.json` (JSON Schema 2020-12).
Unknown fields and versions are rejected. Stable directive IDs are distinct
from the manifest's section/family IDs. `section` and `family` reference those
inventories; `after` and `constraints[].directives` reference directive IDs.
`sourcePaths` resolve from the producer repository root and `help` adds a
Markdown heading anchor. `grammarSource` pins the reviewed grammar baseline,
not the commit that eventually packages this new file. Its file hashes are
checked against current source; drift requires review, not automatic refresh.

Argument array order and zero-based `position` describe arguments after the
literal `prefix`. `required` applies when that directive is written. `enum`
lists family spellings, including a spelling spanning multiple words. `arity`
is the exact allowed total token count **after the existing Go normalizer**,
including prefix, argument words and any `unitToken`. `unitToken: null` means
no separate unit word. `default: null` means absent, not zero. `presence`
distinguishes selection of the family, an omitted value with a parser default,
and an optional member of an all-or-none bundle. `deprecated: false` applies
to these six entries only.

The normalizer accepts the optional colon in `type:`, collapses whitespace,
and removes some punctuation. Keywords, family spellings and the ATR unit are
case insensitive. The metadata is not a replacement lexer or parser: its
lexical names describe normalized tokens, not regular expressions to run over
raw lines. In particular, the normalizer splits `+`, so a leading plus or
positive exponent sign does not survive as one numeric token.

Numeric lexical modes deliberately differ:

- `legacy-number`: fast/slow use the existing Go `strconv.ParseFloat` reader,
  which also strips attached `R`, `r` or `%` suffixes without scaling. Thus
  `20%` is length 20, not 0.2. Integral decimal/exponent values and accepted Go
  hexadecimal/underscore forms are legacy syntax. Values must be positive
  and survive the parser's `float64(int(value))` roundtrip. There is no portable
  declared maximum; `upperBound: "go-int-roundtrip"` exposes this limitation.
- `finite-decimal`: ATR length and multiples use the producer's decimal regex
  and finite binary64 parsing. Hexadecimal, underscores, NaN, infinity,
  overflow and attached unit suffixes are rejected. ATR length must be
  integral; multiples may be fractional. `maximum: null` and
  `upperBound: "no-parser-maximum"` mean the parser sets no explicit maximum,
  not that the engine supports arbitrarily large values.
- `family-alias`: use only the listed producer family aliases. `type` is a
  selector; omitting it does not select this family. This contract does not
  assert that every malformed selector produces a parser error.

## type

`type: sma golden cross` (also `type smaGoldenCross`) selects the family in
`setup` and must precede its SMA directives. It resets the family parameters:
fast 50, slow 200, and no ATR/stop/target fields. Repeated scalar directives use
the last value; a later type selector resets the family defaults again. These
are current parser behaviors, not recommended authoring patterns. Fast/slow
and bundle constraints apply to the final effective configuration.

## fast

`sma fast 20`: one positive integral bar count, default 50. No separate `bars`
or `candles` word. The effective fast length must be less than slow.

## slow

`sma slow 80`: one positive integral bar count, default 200. No separate unit
word. Fast and slow can appear in either order after `type`; the constraint
compares their final values, not their line order.

## atr

`sma atr 14`: one positive finite integral bar count, absent by default. It
requires both `sma stop` and `sma target`. No separate unit word.

## stop

`sma stop 2 ATR`: one positive finite multiple followed by the mandatory `ATR`
word, absent by default. It requires both `sma atr` and `sma target`.

## target

`sma target 3 ATR`: one positive finite multiple followed by the mandatory
`ATR` word, absent by default. It requires both `sma atr` and `sma stop`.
The bundle members can appear in any order after `type`. All three absent
is valid syntax and carries no protected execution assertion.

## Complete example

```dsl
dsl v7
strategy "Authoring example" { description "Static syntax example" }
market conditions { slices(XAUUSD 1d) }
setup {
 type: sma golden cross
 sma fast 20
 sma slow 80
 sma atr 14
 sma stop 2 ATR
 sma target 3 ATR
}
filters { side long only }
```

The descriptor contains complete default-only, custom-length and protected
examples with expected parsed values. Tests check every example and every
DSL fence in this document against the actual Go parser.

## Deterministic checks and distribution

Run `go test ./dsl -run AuthoringMetadata -count=1` for strict schema,
reference, source-hash, manifest-inventory, default, example, lexical and
boundary checks. The check is read-only: it never regenerates metadata or
conformance expectations. JSON uses sorted object keys, two-space indentation
and a final newline; inventories and directive IDs are sorted. No dependency
or alternate parser is introduced. Semantic checks supplement JSON Schema
for unique IDs, references and producer agreement.

The existing release workflow packages `conformance/ spec/ examples/` in
`conformance.tar.gz`, so these three spec files are included without a new
asset or workflow change. This source handoff creates no release. `tag: null`
is intentional; `releaseIdentity: "external-verified-archive"` tells consumers
to bind these bytes to the verified archive digest, tag and resolved source
commit from their existing release pin. Do not interpret the grammar baseline
as that eventual release identity. There is no self-referential commit stamp.

Older consumers may ignore the new file and continue using their pinned
manifest/parser. New consumers must treat a missing file, unknown version or
uncovered directive as unavailable metadata; they must not infer syntax or
execution permissions. Consumer adoption and release qualification are
separate work.
