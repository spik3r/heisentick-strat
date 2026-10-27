# Diagnostic source envelope v2 (proposal)

This contract is a design candidate. No parser, CLI, WASM or consumer emits or
accepts it yet. The synthetic probes exercise a test-only reference consumer;
they do not demonstrate parser span production or editor undo support.

## Version and compatibility

Opt in to `strat-diagnostic-v2` through a future explicitly negotiated output
mode. Do not insert v2 fields into v1 diagnostics or change parse-result-v1.
The envelope's `diagnostic` is an unchanged diagnostic-v1 object, including its
optional stable `code`. Down-conversion is exactly this nested object; legacy
consumers continue receiving v1 from their existing entry points. A v1-only
schema intentionally rejects the whole envelope. Never derive edits from
`message`, `suggestion`, `line`, `col`, a code, or fuzzy matching.

Codes identify diagnostic categories and remain independent of translated
messages and source coordinates. Preserve existing codes. Future code registry
and exact typo/alias eligibility require producer-owner review; the synthetic
probe code is not a new production code or grammar rule.

## Coordinates

`range` is null when the producer cannot prove an exact physical source span.
Otherwise it is `[startByte,endByte)`, zero-based UTF-8 byte offsets in the
original valid UTF-8 source, including comments, indentation, BOM and original
line endings. Both endpoints must be Unicode scalar boundaries, within source
length, and ordered. Empty spans are valid diagnostic carets. Do not map spans
back from normalized, joined or expanded logical lines by text search.

For editor/LSP positions derive zero-based line and UTF-16 code-unit character
from the same source. Astral scalars count as two UTF-16 units; combining marks
remain separate scalars. Tabs count as one code unit, not display width. CR and LF
end a line; CRLF is one indivisible line break (both bytes retained); an
endpoint between CR and LF is invalid. This follows Microsoft
[text-document line offsets](https://github.com/microsoft/vscode-languageserver-node/blob/main/textDocument/src/main.ts)
(`computeLineOffsets` and `isEOL`). EOF and the empty line after a trailing
CR, LF or CRLF are valid endpoints. Multiline
spans retain intervening raw bytes. No newline or Unicode normalization occurs.

Existing v1 `line`/`col` retain their existing producer semantics, including
nullable locations and the current one-based byte-column helpers. They are
legacy display hints, not UTF-16 coordinates. Null/partial legacy coordinates
must not be invented from ambiguous errors; a future exact span need not equal
a legacy directive-head hint. New consumers use a validated v2 range, falling
back to the readable legacy diagnostic if unavailable.

## Wire text validity

All wire strings must encode Unicode scalar sequences. Validate raw message
UTF-8 and escaped UTF-16 surrogate pairing before JSON decoding: reject malformed
UTF-8, lone high/low surrogates, reversed pairs and a high surrogate without an
immediately adjacent low-surrogate escape. Valid paired escapes and literal
astral characters are accepted. Do not repair, normalize or replace malformed
text with U+FFFD. Go encoding/json can silently repair these inputs; checking
decoded strings alone is insufficient. A raw validation failure rejects the
whole v2 envelope before schema or edit handling. The same checks apply to
source/identity, expectedText and newText, not just readable message text.

In-memory Go producers/consumers must also check utf8.ValidString for both edit
strings before encoding/applying. Reject an invalid replacement without changing
source, even when identity, digest and expectedText match. A literal U+FFFD is
valid text; it must not be used as evidence that decoding was lossless.

## Source identity and edits

`source.id` is an opaque caller-provided document identity, scoped to the
document/session; it is not a path to open or fetch. `source.version` is an
opaque nonempty revision string, or null when unavailable. `sha256` is lowercase
hex SHA-256 over the exact UTF-8 source bytes (no trimming, decoding escapes,
line-ending conversion or normalization). Reopening a document requires a new
session identity or a revision that cannot collide with an earlier session.

Before offering an edit, validate schema and all semantic invariants against
the currently bound source: valid UTF-8, exact identity, nonnull equal revision,
equal digest, ordered in-bounds scalar/CRLF boundaries, and expected source
slice. A version mismatch rejects even identical text; a digest mismatch rejects
even identical versions. Unknown versions permit readable diagnostics but no
edits. Reject the complete action if any check fails. Revalidate atomically at
apply time to close the preview/apply race; never mutate a file named by id.

`edit` is null unless the producer has proven a unique intended replacement
for the exact original token. This initial contract permits one nonempty-token
replacement only; deletion is represented by empty `newText`. Its range must
equal the nonnull diagnostic range and `expectedText` must equal that exact raw
slice. Insertions, multiple edits, overlaps and alternative fixes need a future
reviewed extension. Repeated text elsewhere is not ambiguous when the producer
retains the exact original token offset; repeated text plus only a directive
hint is ambiguous and yields no edit. Unknown directives with multiple plausible
meanings and document-level errors remain explanation-only.

The consumer previews the exact replacement and applies only on explicit user
action as one editor transaction/undo step, preserving all unrelated bytes.
An invalid or stale edit may remain a readable message but cannot be applied.
JSON Schema checks shape and individual bounds; offset ordering, byte length,
source hashes, revision equality and edit/range relationships are semantic
checks, exercised separately by the reference probes.

## Integration decisions still required

The producer and authoring owners must agree on negotiation, document identity
and revision plumbing, stable code ownership and the first uniquely safe alias
case. Raw token provenance must be designed before modifying parser/emitter
code. Preserve existing v1 output and conformance fixtures during that work.
Release/adoption and actual protocol/browser preview, atomic apply, stale-result
and undo tests follow separately. Do not advertise LSP code actions from these
schema tests alone.
