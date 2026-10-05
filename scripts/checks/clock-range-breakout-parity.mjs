// Native and Go/WASM execution of the clock range breakout conformance cases.
// Usage: node scripts/checks/clock-range-breakout-parity.mjs \
//   <native enginewasm> <enginewasm.wasm> <native dslwasm> <dslwasm.wasm> <wasm_exec.js>
// Every case must give byte-identical native and WASM output, and that output
// must equal the committed Go golden. This checks the Go producer in both
// runtimes; it does not exercise the app's JavaScript runtime.
import assert from 'node:assert/strict';
import { createHash, webcrypto } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { createRequire } from 'node:module';

const [nativeEngine, engineWasm, nativeDsl, dslWasm, shimPath] = process.argv.slice(2);
assert(nativeEngine && engineWasm && nativeDsl && dslWasm && shimPath, 'five paths are required');
globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(resolve(shimPath));

async function load(path) {
  const go = new globalThis.Go();
  const { instance } = await WebAssembly.instantiate(readFileSync(path), go.importObject);
  void go.run(instance);
}
await load(engineWasm);
await load(dslWasm);
assert.equal(typeof globalThis.engineRunFixture, 'function');
assert.equal(typeof globalThis.dslParse, 'function');

const corpus = new URL('../../conformance/', import.meta.url);
const digest = (value) => createHash('sha256').update(value).digest('hex');
const scratch = mkdtempSync(join(tmpdir(), 'clock-range-breakout-'));
const results = { parse: [], run: [] };
try {
  const parseNames = readdirSync(new URL('parse/', corpus)).filter((n) => /clock-range-breakout.*\.strat$/.test(n)).sort();
  for (const file of parseNames) {
    const name = file.replace(/\.strat$/, '');
    const source = readFileSync(new URL(`parse/${file}`, corpus), 'utf8');
    const golden = JSON.parse(readFileSync(new URL(`parse/${name}.cfg.json`, corpus), 'utf8'));
    const sourcePath = join(scratch, 'source.strat');
    writeFileSync(sourcePath, source);
    const native = execFileSync(resolve(nativeDsl), [sourcePath], { encoding: 'utf8' }).trim();
    const wasm = globalThis.dslParse(source);
    assert.equal(wasm, native, `${name}: native and WASM parse envelopes differ`);
    const envelope = JSON.parse(native);
    assert.equal(envelope.ok, true, `${name}: parser failure`);
    const { cfg, errors, warnings, diagnostics } = envelope.result;
    assert.deepStrictEqual({ cfg, errors, warnings, diagnostics },
      { cfg: golden.cfg, errors: golden.errors, warnings: golden.warnings, diagnostics: golden.diagnostics }, `${name}: parse differs from golden`);
    results.parse.push({ name, errors: errors.length, sha256: digest(native) });
  }

  const runNames = readdirSync(new URL('run/', corpus)).filter((n) => /clock-range-breakout.*\.fixture\.json$/.test(n)).sort();
  for (const file of runNames) {
    const name = file.replace(/\.fixture\.json$/, '');
    const raw = readFileSync(new URL(`run/${file}`, corpus), 'utf8');
    const source = readFileSync(new URL(`run/${name}.strat`, corpus), 'utf8');
    const golden = JSON.parse(readFileSync(new URL(`run/${name}.trades.json`, corpus), 'utf8'));
    writeFileSync(join(scratch, 'fixture.json'), raw);
    writeFileSync(join(scratch, 'source.strat'), source);
    const native = execFileSync(resolve(nativeEngine), [join(scratch, 'fixture.json'), join(scratch, 'source.strat')], { encoding: 'utf8' }).trim();
    const wasm = globalThis.engineRunFixture(raw, source);
    assert.equal(wasm, native, `${name}: native and WASM run output differ`);
    assert.deepStrictEqual(JSON.parse(native), golden, `${name}: run output differs from golden`);
    results.run.push({ name, trades: golden.tradeCount, sha256: digest(native) });
  }

  // Malformed series fail closed in both runtimes instead of counting as coverage.
  const base = JSON.parse(readFileSync(new URL('run/family-clock-range-breakout-ordinary-long.fixture.json', corpus), 'utf8'));
  const baseSource = readFileSync(new URL('run/family-clock-range-breakout-ordinary-long.strat', corpus), 'utf8');
  const malformed = {
    duplicate: (bars) => { bars[10] = [...bars[9]]; },
    offGrid: (bars) => { bars[12][0] += 1000; },
    ohlc: (bars) => { bars[20][2] = bars[20][1] - 0.5; },
  };
  for (const [label, mutate] of Object.entries(malformed)) {
    const fixture = structuredClone(base);
    mutate(fixture.bars);
    const raw = JSON.stringify(fixture);
    const wasm = JSON.parse(globalThis.engineRunFixture(raw, baseSource));
    assert.match(wasm.error ?? '', /malformed series/, `${label}: WASM accepted a malformed series`);
    writeFileSync(join(scratch, 'fixture.json'), raw);
    writeFileSync(join(scratch, 'source.strat'), baseSource);
    assert.throws(() => execFileSync(resolve(nativeEngine), [join(scratch, 'fixture.json'), join(scratch, 'source.strat')], { stdio: 'pipe' }), `${label}: native accepted a malformed series`);
  }
  // Original rows are checked before the lossy adapters can drop or cut them.
  const rows = {
    shortRow: (bars) => { bars[10] = bars[10].slice(0, 4); },
    sevenValueRow: (bars) => { bars[10] = [...bars[10], 7]; },
    nullTimestamp: (bars) => { bars[0][0] = null; },
    nullOpen: (bars) => { bars[36][1] = null; bars[36][3] = 0; },
    nullVolume: (bars) => { bars[10][5] = null; },
  };
  for (const [label, mutate] of Object.entries(rows)) {
    const fixture = structuredClone(base);
    mutate(fixture.bars);
    const raw = JSON.stringify(fixture);
    const wasm = JSON.parse(globalThis.engineRunFixture(raw, baseSource));
    assert.match(wasm.error ?? '', /malformed series/, `${label}: WASM accepted a malformed row`);
    writeFileSync(join(scratch, 'fixture.json'), raw);
    writeFileSync(join(scratch, 'source.strat'), baseSource);
    assert.throws(() => execFileSync(resolve(nativeEngine), [join(scratch, 'fixture.json'), join(scratch, 'source.strat')], { stdio: 'pipe' }), `${label}: native accepted a malformed row`);
  }
  // The bars container is required; an explicit empty array is the only way to supply none.
  for (const [label, mutate] of Object.entries({ omittedBars: (f) => { delete f.bars; }, nullBars: (f) => { f.bars = null; } })) {
    const fixture = structuredClone(base);
    mutate(fixture);
    const raw = JSON.stringify(fixture);
    const wasm = JSON.parse(globalThis.engineRunFixture(raw, baseSource));
    assert.match(wasm.error ?? '', /malformed series/, `${label}: WASM accepted a missing container`);
    writeFileSync(join(scratch, 'fixture.json'), raw);
    writeFileSync(join(scratch, 'source.strat'), baseSource);
    assert.throws(() => execFileSync(resolve(nativeEngine), [join(scratch, 'fixture.json'), join(scratch, 'source.strat')], { stdio: 'pipe' }), `${label}: native accepted a missing container`);
    results.malformedRejected = (results.malformedRejected ?? 0) + 1;
  }
  const empty = structuredClone(base);
  empty.bars = [];
  const emptyRaw = JSON.stringify(empty);
  writeFileSync(join(scratch, 'fixture.json'), emptyRaw);
  writeFileSync(join(scratch, 'source.strat'), baseSource);
  const emptyNative = execFileSync(resolve(nativeEngine), [join(scratch, 'fixture.json'), join(scratch, 'source.strat')], { encoding: 'utf8' }).trim();
  assert.equal(globalThis.engineRunFixture(emptyRaw, baseSource), emptyNative);
  assert.equal(JSON.parse(emptyNative).tradeCount, 0);
  results.malformedRejected = (results.malformedRejected ?? 0) + Object.keys(malformed).length + Object.keys(rows).length;
} finally {
  rmSync(scratch, { recursive: true, force: true });
}
assert(results.parse.length >= 20 && results.run.length >= 30, 'unexpectedly few cases were compared');
console.log(JSON.stringify({ schema: 'clock-range-breakout-native-wasm-v1', parseCases: results.parse.length, runCases: results.run.length, results }, null, 2));
