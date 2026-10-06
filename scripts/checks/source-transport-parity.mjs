// Production JS-call transport tests. Parsing and execution stay in Go.
// Usage: node scripts/checks/source-transport-parity.mjs \
// <native enginewasm> <enginewasm.wasm> <native dslwasm> <dslwasm.wasm> <wasm_exec.js>
import assert from 'node:assert/strict';
import { webcrypto } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { createRequire } from 'node:module';

const [nativeEngine, engineWasm, nativeDsl, dslWasm, shim] = process.argv.slice(2);
assert(nativeEngine && engineWasm && nativeDsl && dslWasm && shim, 'five paths required');
globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(resolve(shim));
for (const path of [engineWasm, dslWasm]) {
  const go = new globalThis.Go();
  const { instance } = await WebAssembly.instantiate(readFileSync(path), go.importObject);
  void go.run(instance);
}

const scratch = mkdtempSync(join(tmpdir(), 'source-transport-'));
const sourcePath = join(scratch, 'source.strat');
const fixturePath = join(scratch, 'fixture.json');
const syntheticBar = [1700000000000, 100, 101, 99, 100, 1];
const fixture = JSON.stringify({ schema: 'dsl-conformance-run-fixture-v1', case: 'synthetic-source-transport', symbol: 'XAUUSD', timeframe: '1m', bars: [syntheticBar] });
const metadata = JSON.stringify({ schema: 'enginewasm-columnar-v1', case: 'synthetic-source-transport', symbol: 'XAUUSD', timeframe: '1m' });
const columns = syntheticBar.map((value) => new Float64Array([value]));
const frozen = readFileSync(new URL('../../dsl/testdata/frozen_level/example.strat', import.meta.url), 'utf8');
const legacy = 'dsl v7\nstrategy "Transport Ω" { description "Synthetic transport test" }\nsetup { type: failed breakout }\n';
const unicodeError = /DSL source is not valid UTF-16: unpaired surrogate at code unit \d+/;
let parseComparisons = 0, originalStringRefusals = 0, validUnicodeControls = 0, nativeByteRefusals = 0;

function nativeParse(source) {
  return execFileSync(resolve(nativeDsl), [], { input: source, encoding: 'utf8' }).trim();
}

function compareParse(source, label) {
  const native = nativeParse(source);
  const wasm = globalThis.dslParse(source);
  assert.equal(wasm, native, `${label}: parse envelope differs across source transport`);
  parseComparisons++;
  return JSON.parse(wasm);
}

function rejectOriginalString(source, label) {
  // Do not pass malformed JS strings through a Buffer or JSON round trip:
  // either can hide the original transport defect this test must exercise.
  const parsed = JSON.parse(globalThis.dslParse(source));
  assert.equal(parsed.ok, false, `${label}: dslParse accepted malformed UTF-16`);
  assert.match(parsed.error ?? '', unicodeError, `${label}: missing source-transport error`);
  assert.equal(parsed.result, undefined, `${label}: malformed source produced a config`);
  const run = JSON.parse(globalThis.engineRunFixture(fixture, source));
  assert.match(run.error ?? '', unicodeError, `${label}: fixture bridge accepted malformed UTF-16`);
  assert.equal(run.tradeCount, undefined);
  const columnRun = globalThis.engineRunColumns(metadata, source, ...columns);
  assert.equal(columnRun.ok, false, `${label}: column bridge accepted malformed UTF-16`);
  assert.match(columnRun.error ?? '', unicodeError);
  assert.equal(columnRun.trades, undefined);
  originalStringRefusals += 3;
}

try {
  writeFileSync(fixturePath, fixture);

  // Existing valid and invalid corpus sources must retain exact native/WASM
  // parity after the new transport validation, across all existing families.
  const corpus = new URL('../../conformance/parse/', import.meta.url);
  for (const file of readdirSync(corpus).filter((name) => name.endsWith('.strat')).sort()) {
    const bytes = readFileSync(new URL(file, corpus));
    const source = new TextDecoder('utf-8', { fatal: true }).decode(bytes);
    compareParse(source, file);
  }

  const invalidUnits = [
    [0xd800], [0xdbff], [0xdc00], [0xdfff],
    [0xd800, 0x41], [0xd800, 0xd800, 0xdc00], [0xdc00, 0xd800],
    [0xd800, 0xdc00, 0xd800], [0xd800, 0xdc00, 0xdc00],
    [0xd800, 0xfffd, 0xdc00],
  ];
  for (const [family, template] of [['frozen', frozen], ['legacy', legacy]]) {
    for (const units of invalidUnits) {
      const invalid = String.fromCharCode(...units);
      const label = `${family}/${units.map((unit) => unit.toString(16)).join('-')}`;
      rejectOriginalString(template.replace('Ω', invalid), `${label}/name`);
      rejectOriginalString(`# ${invalid}\n${template}`, `${label}/leading-comment`);
      rejectOriginalString(`${template}\n# ${invalid}`, `${label}/trailing-comment`);
    }
  }

  const validText = [
    'Ω', '\ufffd', String.fromCodePoint(0x10000), String.fromCodePoint(0x10ffff),
    String.fromCodePoint(0x1d11e), 'a\ufffdb', 'e\u0301', 'é',
    `${String.fromCodePoint(0x1f600)}\ufffd${String.fromCodePoint(0x1d11e)}`,
  ];
  for (const [family, template] of [['frozen', frozen], ['legacy', legacy]]) {
    const baselineName = compareParse(template, `${family}/baseline`).result.cfg.name;
    for (const text of validText) {
      const source = template.replace('Ω', text);
      const parsed = compareParse(source, `${family}/valid-unicode`);
      assert.equal(parsed.ok, true);
      assert.deepEqual(parsed.result.errors, []);
      assert.equal(parsed.result.cfg.name, baselineName.replace('Ω', text), 'metadata was repaired or normalized');
      writeFileSync(sourcePath, source);
      const runJSON = globalThis.engineRunFixture(fixture, source);
      const run = JSON.parse(runJSON);
      const columnRun = globalThis.engineRunColumns(metadata, source, ...columns);
      if (family === 'frozen') {
        assert.match(run.error ?? '', /not implemented/);
        assert.equal(columnRun.ok, false);
        assert.match(columnRun.error, /frozen-level-quote-execution-unimplemented/);
        assert.throws(() => execFileSync(resolve(nativeEngine), [fixturePath, sourcePath], { stdio: 'pipe' }), (error) => /not implemented/.test(String(error.stderr)));
      } else {
        const nativeRun = execFileSync(resolve(nativeEngine), [fixturePath, sourcePath], { encoding: 'utf8' }).trim();
        assert.equal(runJSON, nativeRun, 'legacy fixture output changed through source transport');
        assert.equal(run.error, undefined);
        assert.equal(run.tradeCount, 0);
        assert.equal(columnRun.ok, true, `valid legacy column run failed: ${columnRun.error}`);
        assert.equal(JSON.parse(columnRun.summaryJSON).tradeCount, 0);
        assert.equal(columnRun.trades.length, 0);
      }
      validUnicodeControls += 3;
    }
  }

  // ASCII escape syntax is distinct from ill-formed original UTF-16 and must
  // still reach the strict DSL scanner, which rejects an unpaired escape.
  for (const escape of ['\\ud800', '\\udbff', '\\udc00', '\\udfff']) {
    const source = frozen.replace('Ω', escape);
    const parsed = compareParse(source, `escaped-${escape}`);
    assert.equal(parsed.ok, true);
    assert(parsed.result.errors.length > 0);
    assert.deepEqual(parsed.result.cfg, {});
    assert.match(JSON.parse(globalThis.engineRunFixture(fixture, source)).error, /DSL parse errors/);
    const columnRun = globalThis.engineRunColumns(metadata, source, ...columns);
    assert.equal(columnRun.ok, false);
    assert.match(columnRun.error, /DSL parse errors/);
  }

  // Native source uses raw bytes, not JSON string decoding. Verify malformed
  // UTF-8 reaches the strict source validator without replacement there too.
  const [before, after] = frozen.split('Ω');
  for (const invalid of [[0xed, 0xa0, 0x80], [0xed, 0xb0, 0x80], [0xc0, 0xaf], [0xff], [0xf0, 0x90]]) {
    const bytes = Buffer.concat([Buffer.from(before), Buffer.from(invalid), Buffer.from(after)]);
    const parsed = JSON.parse(nativeParse(bytes));
    assert.equal(parsed.ok, true);
    assert(parsed.result.errors.some((message) => /UTF-8/.test(message)));
    assert.deepEqual(parsed.result.cfg, {});
    writeFileSync(sourcePath, bytes);
    assert.throws(() => execFileSync(resolve(nativeEngine), [fixturePath, sourcePath], { stdio: 'pipe' }), (error) => /DSL parse errors/.test(String(error.stderr)));
    nativeByteRefusals += 2;
  }

  console.log(JSON.stringify({ parseComparisons, originalStringRefusals, validUnicodeControls, nativeByteRefusals, status: 'PASS' }));
} finally {
  rmSync(scratch, { recursive: true, force: true });
}
process.exit(0);
