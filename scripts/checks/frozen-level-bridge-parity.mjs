// Harness only: all parsing, validation and refusal comes from the Go builds.
// Usage: node scripts/checks/frozen-level-bridge-parity.mjs \
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
const corpus = new URL('../../conformance/parse/', import.meta.url);
const scratch = mkdtempSync(join(tmpdir(), 'frozen-level-bridge-'));
let parses = 0, fixtureRefusals = 0, columnRefusals = 0;
let selectorRegressions = 0, executableBypassesRefused = 0, legacyValues = 0, inlineRegressions = 0;
try {
  for (const file of readdirSync(corpus).filter((n) => n.startsWith('family-frozen-level-breakout-') && n.endsWith('.strat')).sort()) {
    const source = readFileSync(new URL(file, corpus), 'utf8');
    const expected = JSON.parse(readFileSync(new URL(file.replace('.strat', '.cfg.json'), corpus), 'utf8'));
    writeFileSync(join(scratch, 'source.strat'), source);
    const native = execFileSync(resolve(nativeDsl), [join(scratch, 'source.strat')], { encoding: 'utf8' }).trim();
    assert.equal(globalThis.dslParse(source), native, `${file}: parse runtime mismatch`);
    const envelope = JSON.parse(native);
    assert.equal(envelope.ok, true);
    assert.deepEqual(envelope.result, { cfg: expected.cfg, errors: expected.errors, warnings: expected.warnings, diagnostics: expected.diagnostics });
    parses++;
    for (const bars of [[], [[0, 100, 101, 99, 100, 1]], [[null, 100, 101, 99, 100, 1]], null]) {
      for (const symbol of ['XAUUSD', 'WRONG']) {
        const raw = JSON.stringify({ schema: 'dsl-conformance-run-fixture-v1', case: file, symbol, timeframe: '1m', bars });
        const wasm = JSON.parse(globalThis.engineRunFixture(raw, source));
        const pattern = expected.errors.length ? /DSL parse errors/ : /not implemented/;
        assert.match(wasm.error ?? '', pattern, `${file}: fixture was not refused`);
        writeFileSync(join(scratch, 'fixture.json'), raw);
        assert.throws(() => execFileSync(resolve(nativeEngine), [join(scratch, 'fixture.json'), join(scratch, 'source.strat')], { stdio: 'pipe' }), (err) => pattern.test(String(err.stderr)), `${file}: native fixture was not refused`);
        fixtureRefusals++;
      }
    }
    for (const n of [0, 1]) {
      const columns = Array.from({ length: 6 }, () => new Float64Array(n));
      const output = globalThis.engineRunColumns(JSON.stringify({ schema: 'enginewasm-columnar-v1', symbol: 'XAUUSD', timeframe: '1m' }), source, ...columns);
      assert.equal(output.ok, false);
      assert.match(output.error, expected.errors.length ? /DSL parse errors/ : /frozen-level-quote-execution-unimplemented/);
      columnRefusals++;
    }
  }
  const inlineProbes = JSON.parse(readFileSync(new URL('../../dsl/testdata/frozen_level/same-line-reproductions.json', import.meta.url), 'utf8'));
  for (const [name, source] of Object.entries(inlineProbes)) {
    writeFileSync(join(scratch, 'source.strat'), source);
    const native = execFileSync(resolve(nativeDsl), [join(scratch, 'source.strat')], { encoding: 'utf8' }).trim();
    assert.equal(globalThis.dslParse(source), native, `${name}: inline selection runtime mismatch`);
    const result = JSON.parse(native).result;
    assert(result.errors.length > 0, `${name}: inline frozen request defaulted`);
    assert.deepEqual(result.cfg, {});
    inlineRegressions++;
  }
  assert.equal(parses, 12);
  const probes = JSON.parse(readFileSync(new URL('../../dsl/testdata/frozen_level/selection_reproductions.json', import.meta.url), 'utf8'));
  for (const [name, source] of Object.entries(probes)) {
    writeFileSync(join(scratch, 'source.strat'), source);
    const native = execFileSync(resolve(nativeDsl), [join(scratch, 'source.strat')], { encoding: 'utf8' }).trim();
    assert.equal(globalThis.dslParse(source), native, `${name}: selection runtime mismatch`);
    const result = JSON.parse(native).result;
    if (name === 'old_symbols') {
      assert.equal(result.errors.length, 0);
      assert.equal(result.cfg.setupType, 'flagContinuation');
      assert.deepEqual(result.cfg.symbols, ['FROZEN']);
    } else {
      assert(result.errors.length > 0, `${name}: malformed frozen request defaulted`);
      assert.deepEqual(result.cfg, {});
    }
    selectorRegressions++;
  }
  const runBase = new URL('../../conformance/run/research-dsl-failed-breakout-five-minute-early-breakeven', import.meta.url);
  const legacySource = readFileSync(`${runBase.pathname}.strat`, 'utf8');
  const legacyFixture = readFileSync(`${runBase.pathname}.fixture.json`, 'utf8');
  assert.equal(JSON.parse(globalThis.engineRunFixture(legacyFixture, legacySource)).tradeCount, 5, 'legacy executable positive control');
  const fixtureObject = JSON.parse(legacyFixture);
  const actualColumns = Array.from({length:6}, (_,j) => Float64Array.from(fixtureObject.bars, row => row[j]));
  const columnMetadata = {schema:'enginewasm-columnar-v1'};
  for (const key of ['case','strategyId','symbol','timeframe','rangeMethod','costs']) columnMetadata[key]=fixtureObject[key];
  const actualMeta = JSON.stringify(columnMetadata);
  const columnPositive = globalThis.engineRunColumns(actualMeta, legacySource, ...actualColumns);
  assert.equal(columnPositive.ok, true);
  assert.equal(JSON.parse(columnPositive.summaryJSON).tradeCount, 5, 'legacy column five-trade positive control');
  for (const selector of ['"frozen level breakout"', "'frozen level breakout'", '`frozen level breakout`', 'frozenLevelBreakout']) {
    const source = `dsl v7\nsetup { type: ${selector} }\n${legacySource}`;
    writeFileSync(join(scratch, 'source.strat'), source);
    writeFileSync(join(scratch, 'fixture.json'), legacyFixture);
    assert.match(JSON.parse(globalThis.engineRunFixture(legacyFixture, source)).error ?? '', /DSL parse errors/);
    assert.throws(() => execFileSync(resolve(nativeEngine), [join(scratch, 'fixture.json'), join(scratch, 'source.strat')], { stdio: 'pipe' }), (err) => /DSL parse errors/.test(String(err.stderr)));
    const output = globalThis.engineRunColumns(JSON.stringify({ schema: 'enginewasm-columnar-v1', symbol: 'XAUUSD', timeframe: '5m' }), source, ...Array.from({ length: 6 }, () => new Float64Array(0)));
    assert.equal(output.ok, false);
    assert.match(output.error, /DSL parse errors/);
    executableBypassesRefused++;
  }
  for (const body of ['symbols XAUUSD type: frozen level breakout type: failed breakout', 'symbols XAUUSD type: "frozen level breakout" type: failed breakout', 'description free type: frozen level breakout type: failed breakout', '(type: frozen level breakout) type: failed breakout']) {
    const source = `dsl v7\nsetup { ${body} }\n${legacySource}`;
    writeFileSync(join(scratch, 'source.strat'), source);
    writeFileSync(join(scratch, 'fixture.json'), legacyFixture);
    assert.match(JSON.parse(globalThis.engineRunFixture(legacyFixture, source)).error ?? '', /DSL parse errors/);
    assert.throws(() => execFileSync(resolve(nativeEngine), [join(scratch, 'fixture.json'), join(scratch, 'source.strat')], {stdio:'pipe'}), error => /DSL parse errors/.test(String(error.stderr)));
    const column = globalThis.engineRunColumns(actualMeta, source, ...actualColumns);
    assert.equal(column.ok, false);
    assert.match(column.error, /DSL parse errors/);
    executableBypassesRefused++;
  }
  const clauses = ['symbols FROZEN', 'symbols FROZEN CONTROL', 'symbols(FROZEN)', 'symbols(FROZEN, BTCUSDT)', 'slices(FROZEN 1h, BTCUSDT 1m)', 'description frozen lock pivot'];
  for (const q of ['"', "'", '`']) clauses.push(`symbols ${q}FROZEN${q}`, `symbols(${q}FROZEN${q}, BTCUSDT)`, `description ${q}type: frozen level breakout # { frozen lock pivot }${q}`);
  for (const clause of clauses) {
    const source = `dsl v7\nstrategy "Legacy"\n${clause}\nsetup { type: flag continuation }`;
    writeFileSync(join(scratch, 'source.strat'), source);
    const native = execFileSync(resolve(nativeDsl), [join(scratch, 'source.strat')], { encoding: 'utf8' }).trim();
    assert.equal(globalThis.dslParse(source), native);
    const result = JSON.parse(native).result;
    assert.equal(result.errors.length, 0);
    assert.equal(result.cfg.setupType, 'flagContinuation');
    legacyValues++;
  }
  console.log(JSON.stringify({ parses, fixtureRefusals, columnRefusals, selectorRegressions, inlineRegressions, executableBypassesRefused, legacyValues, status: 'PASS' }));
} finally {
  rmSync(scratch, { recursive: true, force: true });
}
process.exit(0);
