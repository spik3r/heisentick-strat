// Go-authoritative SAR contract: actual native/Go-WASM bridge comparison.
// Usage: node scripts/checks/parabolic-sar-parity.mjs \
//   <native enginewasm> <engine.wasm> <native dslwasm> <dsl.wasm> <wasm_exec.js>
// No indicator or trade arithmetic is reimplemented in JavaScript.
import assert from 'node:assert/strict';
import { webcrypto } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { createRequire } from 'node:module';

const [nativeEngine, engineWasm, nativeDsl, dslWasm, shim] = process.argv.slice(2);
assert(nativeEngine && engineWasm && nativeDsl && dslWasm && shim, 'five paths are required');
globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(resolve(shim));
for (const path of [engineWasm, dslWasm]) {
  const go = new globalThis.Go();
  const { instance } = await WebAssembly.instantiate(readFileSync(path), go.importObject);
  void go.run(instance);
}
assert.equal(typeof globalThis.engineRunFixture, 'function');
assert.equal(typeof globalThis.dslParse, 'function');
const root = new URL('../../', import.meta.url);
const scratch = mkdtempSync(join(tmpdir(), 'parabolic-sar-parity-'));
const sourcePath = join(scratch, 'source.strat');
const fixturePath = join(scratch, 'fixture.json');
const read = (path) => readFileSync(new URL(path, root), 'utf8');
function nativeRun(raw, source) {
  writeFileSync(fixturePath, raw);
  writeFileSync(sourcePath, source);
  return execFileSync(resolve(nativeEngine), [fixturePath, sourcePath], { encoding: 'utf8', stdio: 'pipe' }).trim();
}
let parseCases = 0, runs = 0, rejections = 0;
try {
  for (const file of readdirSync(new URL('conformance/parse/', root)).filter((n) => /^(diagnostic-)?sar-.*\.strat$/.test(n)).sort()) {
    const name = file.replace(/\.strat$/, '');
    const source = read(`conformance/parse/${file}`);
    const golden = JSON.parse(read(`conformance/parse/${name}.cfg.json`));
    writeFileSync(sourcePath, source);
    const native = execFileSync(resolve(nativeDsl), [sourcePath], { encoding: 'utf8' }).trim();
    assert.equal(globalThis.dslParse(source), native, `${name}: native/WASM parse mismatch`);
    const { cfg, errors, warnings, diagnostics } = JSON.parse(native).result;
    assert.deepStrictEqual({ cfg, errors, warnings, diagnostics }, { cfg: golden.cfg, errors: golden.errors, warnings: golden.warnings, diagnostics: golden.diagnostics }, `${name}: parse golden mismatch`);
    assert.equal(errors.length > 0, name.startsWith('diagnostic-'), `${name}: acceptance expectation`);
    parseCases++;
  }
  assert.equal(parseCases, 15);
  for (const suffix of ['allow', 'block', 'reversal', 'close-equality']) {
    const name = `sar-named-level-${suffix}`;
    const source = read(`conformance/run/${name}.strat`);
    const raw = read(`conformance/run/${name}.fixture.json`);
    const golden = JSON.parse(read(`conformance/run/${name}.trades.json`));
    const native = nativeRun(raw, source);
    assert.equal(globalThis.engineRunFixture(raw, source), native, `${name}: native/WASM run mismatch`);
    const result = JSON.parse(native);
    assert.deepStrictEqual(result.trades, golden.trades, `${name}: run golden mismatch`);
    assert.equal(result.tradeCount, ['allow', 'reversal'].includes(suffix) ? 1 : 0, `${name}: hand-derived admission`);
    runs++;
  }
  const base = JSON.parse(read('conformance/run/sar-named-level-allow.fixture.json'));
  const source = read('conformance/run/sar-named-level-allow.strat');
  for (const mutate of [
    (f) => { f.bars[2][3] = null; },
    (f) => { f.bars[2] = null; },
    (f) => { f.bars[2] = [1, 2, 3]; },
    (f) => { f.bars[2].push(0); },
    (f) => { f.bars[2][1] = 102; },
    (f) => { f.bars[2][0] = f.bars[1][0]; },
    (f) => { f.sourceTimeframe = '1h'; },
  ]) {
    const fixture = structuredClone(base);
    mutate(fixture);
    const raw = JSON.stringify(fixture);
    const wasm = JSON.parse(globalThis.engineRunFixture(raw, source));
    assert.equal(typeof wasm.error, 'string');
    assert.ok(!('trades' in wasm));
    let nativeError = '';
    try { nativeRun(raw, source); } catch (error) { nativeError = String(error.stderr ?? '').trim(); }
    assert.ok(nativeError.includes(wasm.error), `native/WASM error mismatch: ${nativeError} / ${wasm.error}`);
    rejections++;
  }
  console.log(JSON.stringify({ ok: true, parseCases, runs, rejections }));
} finally {
  rmSync(scratch, { recursive: true, force: true });
}
