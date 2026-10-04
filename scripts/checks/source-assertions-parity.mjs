// Synthetic only. Usage: node source-assertions-parity.mjs native wasm wasm_exec.js baseline-native
// All binaries must be built from the intended source revisions with one Go toolchain.
import assert from 'node:assert/strict';
import { createHash, webcrypto } from 'node:crypto';
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { execFileSync } from 'node:child_process';

const [nativePath, wasmPath, shimPath, baselinePath] = process.argv.slice(2).map(path => resolve(path));
assert(nativePath && wasmPath && shimPath && baselinePath, 'four build paths required');
const sha = (bytes) => createHash('sha256').update(bytes).digest('hex');
globalThis.crypto ??= webcrypto;
await import(pathToFileURL(shimPath).href);
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject);
void go.run(instance);
assert.equal(typeof globalThis.engineInspectInteractiveSource, 'function');
const temp = mkdtempSync(join(tmpdir(), 'source-assertions-'));
const results = [];
const operations = [
  ['inspect', '--interactive-source-profile', 'engineInspectInteractiveSource'],
  ['finalized', '--interactive-draft', 'engineRunInteractiveDraftFixture'],
  ['prefix', '--interactive-draft-prefix', 'engineRunInteractiveDraftPrefixFixture'],
];
function source(tf) {
  const [fast, slow, trail] = tf === '4h' ? [20, 80, 3] : [10, 40, 2];
  return `dsl v7\nstrategy "Synthetic recovery contract"\nmarket conditions { slices(XAUUSD ${tf}) }\nsetup {\n type: dual ema resumption\n ema fast ${fast}\n ema slow ${slow}\n ema slow rise 12\n ema wilder atr 20\n stop initial 2.5 ATR\n trail close ${trail} ATR\n fallback below slow ema\n}\nfilters { side long only }\nexecution { risk 200 USD }`;
}
function claims(tf) {
  const [fast, slow, trail, suffix] = tf === '4h' ? [20, 80, 3, 'FourHour'] : [10, 40, 2, 'Daily'];
  return { schema: 'dsl-source-assertions-v1', strategyId: `dslDualEmaResumptionXauusd${suffix}`,
    params: { fastEmaLen: fast, slowEmaLen: slow, slowRiseBars: 12, atrLen: 20, stopAtr: 2.5,
      trailAtr: trail, allowLong: 1, allowShort: 0, riskUsd: 200 },
    contextOptions: {}, contextRequirements: ['sessions'], preferredRangeMethod: 'zone' };
}
function request(tf, operation, assertion = claims(tf), calculation = 'raw') {
  const fields = { schema: 'dsl-interactive-source-profile-v1', symbol: 'XAUUSD', timeframe: tf, calculationSource: calculation };
  if (operation !== 'inspect') {
    Object.assign(fields, { schema: 'dsl-conformance-run-fixture-v1', case: 'recovery-source-assertions',
      strategyId: 'dslDraftStrategy', rangeMethod: 'pivot',
      costs: { fillOn: 'nextOpen', startEquity: 10000, feePerUnit: 0.1, slippage: 0.06, slippageBps: 0 },
      bars: Array.from({ length: 400 }, (_, i) => {
        const c = 1900 + i * 0.2 - (i % 12 === 9 ? 2.4 : 0);
        return [i * (tf === '4h' ? 14400000 : 86400000), c - 0.1, c + 0.3, c - 0.3, c, 1];
      }) });
  }
  if (assertion !== undefined) fields.sourceAssertions = assertion;
  return fields;
}
function native(binary, mode, raw, text) {
  const input = join(temp, 'fixture.json'), sourcePath = join(temp, 'source.strat');
  writeFileSync(input, raw); writeFileSync(sourcePath, text);
  return execFileSync(binary, [mode, input, sourcePath], { encoding: 'utf8', maxBuffer: 20 * 1024 * 1024 }).trimEnd();
}
function probe(name, operation, raw, text, success, baseline = false) {
  const [, mode, fn] = operations.find(([id]) => id === operation);
  const actual = native(nativePath, mode, raw, text);
  const wasm = globalThis[fn](raw, text);
  assert.equal(wasm, actual, `${name}: complete native/WASM bytes differ`);
  const decoded = JSON.parse(actual);
  assert.equal(!decoded.error, success, `${name}: unexpected outcome ${actual.slice(0, 400)}`);
  if (success) {
    assert.equal(decoded.provenance.fixtureSha256, sha(raw));
    assert.equal(decoded.provenance.sourceSha256, sha(text));
    const expected = JSON.parse(raw).sourceAssertions;
    if (expected !== undefined) assert.deepEqual(decoded.sourceAssertions, expected);
    else assert.equal('sourceAssertions' in decoded, false);
    if (operation === 'finalized') assert(decoded.run.tradeCount > 0);
  } else {
    assert(['invalid-request', 'unsupported-route'].includes(decoded.error.code));
  }
  if (baseline) assert.equal(native(baselinePath, mode, raw, text), actual, `${name}: assertion-free baseline output changed`);
  results.push({ name, operation, success, baselineExact: baseline, requestSha256: sha(raw), sourceSha256: sha(text), outputSha256: sha(actual) });
}
try {
  for (const tf of ['1d', '4h']) {
    for (const [operation] of operations) {
      for (const calculation of ['raw', 'heikinAshi']) {
        const raw = JSON.stringify(request(tf, operation, claims(tf), calculation));
        const success = operation !== 'prefix' || calculation === 'raw';
        probe(`${tf}-${operation}-${calculation}`, operation, raw, source(tf), success);
        const without = request(tf, operation, claims(tf), calculation); delete without.sourceAssertions;
        probe(`${tf}-${operation}-${calculation}-no-assertions`, operation, JSON.stringify(without), source(tf), success, true);
      }
    }
  }
  const mutations = [
    ['null', () => null], ['missing-fields', () => ({})],
    ['missing-sessions', c => ({ ...c, contextRequirements: [] })],
    ['unknown-context', c => ({ ...c, contextOptions: { higherTimeframe: '1d' } })],
    ['wrong-preference', c => ({ ...c, preferredRangeMethod: 'pivot' })],
    ['wrong-id', c => ({ ...c, strategyId: 'dslEditorStrategy' })],
    ...Object.keys(claims('4h').params).map(key => [`mismatch-${key}`, c => ({ ...c, params: { ...c.params, [key]: -1 } })]),
  ];
  for (const [name, mutate] of mutations) for (const [operation] of operations) {
    probe(`${name}-${operation}`, operation, JSON.stringify(request('4h', operation, mutate(claims('4h')))), source('4h'), false);
  }
  for (const [operation] of operations) {
    const raw = JSON.stringify(request('4h', operation));
    probe(`duplicate-param-${operation}`, operation, raw.replace('"fastEmaLen":20', '"fastEmaLen":20,"fastEmaLen":20'), source('4h'), false);
    probe(`source-drift-${operation}`, operation, raw, source('4h').replace('risk 200 USD', 'risk 201 USD'), false);
  }
  console.log(JSON.stringify({ schema: 'recovery-source-assertions-parity-v1', syntheticOnly: true,
    hashes: { native: sha(readFileSync(nativePath)), wasm: sha(readFileSync(wasmPath)), shim: sha(readFileSync(shimPath)), baselineNative: sha(readFileSync(baselinePath)) },
    total: results.length, successes: results.filter(x => x.success).length,
    refusals: results.filter(x => !x.success).length, baselineExact: results.filter(x => x.baselineExact).length, results }, null, 2));
} finally { rmSync(temp, { recursive: true, force: true }); }
