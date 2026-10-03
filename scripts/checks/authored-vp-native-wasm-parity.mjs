#!/usr/bin/env node
// Same-build native/WASM parity for the authored VP interactive export.
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync, writeFileSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import { runInThisContext } from 'node:vm';

const root = resolve(import.meta.dirname, '../..');
const temp = mkdtempSync(join(tmpdir(), 'heisentick-vp-parity-'));
function command(executable, args, env = process.env) {
  const result = spawnSync(executable, args, { cwd: root, env, encoding: 'utf8' });
  if (result.status !== 0) throw new Error(`${executable} ${args.join(' ')} failed: ${result.stderr || result.stdout}`);
  return result.stdout.trim();
}

try {
  const env = { ...process.env, GOCACHE: process.env.GOCACHE || join(temp, 'go-cache') };
  const nativePath = join(temp, 'engine-native');
  const wasmPath = join(temp, 'engine.wasm');
  command('go', ['build', '-trimpath', '-buildvcs=false', '-o', nativePath, './cmd/enginewasm'], env);
  command('go', ['build', '-trimpath', '-buildvcs=false', '-o', wasmPath, './cmd/enginewasm'], { ...env, GOOS: 'js', GOARCH: 'wasm' });
  const goRoot = command('go', ['env', 'GOROOT'], env);
  const shim = [join(goRoot, 'lib/wasm/wasm_exec.js'), join(goRoot, 'misc/wasm/wasm_exec.js')].find((path) => existsSync(path));
  if (!shim) throw new Error('Go wasm_exec.js not found');
  runInThisContext(readFileSync(shim, 'utf8'), { filename: shim });
  const go = new globalThis.Go();
  const { instance } = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject);
  void go.run(instance);
  assert.equal(typeof globalThis.engineRunInteractiveFixture, 'function');
  const fixturePath = join(root, 'engine/testdata/asia-london-wide-interactive.fixture.json');
  const raw = readFileSync(fixturePath, 'utf8');
  const native = JSON.parse(command(nativePath, ['--authored-vp-asia-london-wide', fixturePath], env));
  const wasm = JSON.parse(globalThis.engineRunInteractiveFixture(raw, ''));
  assert.deepStrictEqual(wasm, native);
  assert.equal(wasm.schema, 'authored-vp-asia-london-wide-interactive-v1');
  assert.equal(wasm.run.tradeCount, 3);
  assert.equal(wasm.equityCurve.length, wasm.closedEquityCurve.length);
  assert.equal(wasm.stats.endEquity, wasm.cashEndEquity);
  const invalid = JSON.parse(raw);
  invalid.costs.startEquity = 0;
  const invalidRaw = JSON.stringify(invalid);
  const invalidPath = join(temp, 'invalid.fixture.json');
  writeFileSync(invalidPath, invalidRaw);
  const rejectedNative = spawnSync(nativePath, ['--authored-vp-asia-london-wide', invalidPath], { cwd: root, env, encoding: 'utf8' });
  const rejectedWasm = JSON.parse(globalThis.engineRunInteractiveFixture(invalidRaw, ''));
  assert.notEqual(rejectedNative.status, 0);
  assert.equal(rejectedWasm.error.code, 'invalid-request');
  assert.ok(rejectedNative.stderr.includes('startEquity') && rejectedWasm.error.message.includes('startEquity'));
  console.log(`authored VP native/WASM parity OK: ${wasm.run.tradeCount} trades, ${wasm.equityCurve.length} equity bars; zero start equity rejected`);
} finally {
  rmSync(temp, { recursive: true, force: true });
}
