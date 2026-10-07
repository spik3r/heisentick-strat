// Dedicated Master Structural runtime parity, using invented data only.
// Usage: node scripts/checks/master-runtime-parity.mjs \
//   <native CLI> <enginewasm.wasm> <wasm_exec.js> [before-refactor CLI]
// Both runtimes receive the same JS-generated BTB1 bytes, never historical data.
import assert from 'node:assert/strict';
import { createHash, webcrypto } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

const [nativeCLI, engineWasm, shim, beforeCLI, ...extra] = process.argv.slice(2);
assert(nativeCLI && engineWasm && shim && !extra.length, 'three paths and an optional before-refactor CLI are required');
globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(resolve(shim));
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(readFileSync(engineWasm), go.importObject);
void go.run(instance);
assert.equal(typeof globalThis.engineRunMasterReport, 'function', 'dedicated WASM export is absent');

const M5 = 300000, M30 = 6 * M5, H4 = 8 * M30;
const modes = ['SOURCE_HISTORICAL_REFERENCE', 'PROTECTED_STABLE_REFERENCE'];
const metadata = { schema: 'master-structural-runtime-request-v1', warmupFromT: 0, tradeFromT: 40 * M30, tradeToT: 500 * M30, spread: 0 };
const source = (mode = modes[0], name = 'Invented Master runtime') => `dsl v7
strategy "${name}" { description "Deterministic invented OHLCV only" }
market { master timeframe M30 from M5 }
setup { type: master structural
master profile v10-phase0-floor-half-reference-v1
master mode ${mode}
}
`;
const scratch = mkdtempSync(join(tmpdir(), 'master-runtime-parity-'));
const sourcePath = join(scratch, 'invented.strat'), dataPath = join(scratch, 'invented.btb1');
const evidence = { schema: 'master-runtime-native-wasm-parity-v1', comparisons: [], rejected: 0, genericRefusals: 0, legacyControls: 0 };
const digest = (s) => createHash('sha256').update(s).digest('hex');

// Same waveform as engine/master/master_test.go; evaluate it once in JS so
// runtime parity cannot accidentally test different libm-generated inputs.
function waveform(n) {
  const rows = [];
  let previous = 100;
  for (let i = 0; i < n; i++) {
    const close = 100 + .015 * i + 4 * Math.sin(i * .13) + .8 * Math.sin(i * .71);
    for (let j = 0; j < 6; j++) {
      const o = previous + (close - previous) * j / 6;
      const c = previous + (close - previous) * (j + 1) / 6;
      rows.push([(i * 6 + j) * M5, o, Math.max(o, c) + .08, Math.min(o, c) - .08, c, 10 + (i * 17) % 23]);
    }
    previous = close;
  }
  return rows;
}
function encode(rows) {
  const bytes = new Uint8Array(16 + rows.length * 48);
  const view = new DataView(bytes.buffer);
  [0x31425442, 1, rows.length, 6].forEach((value, i) => view.setUint32(i * 4, value, true));
  for (let col = 0; col < 6; col++) {
    for (let row = 0; row < rows.length; row++) view.setFloat64(16 + (col * rows.length + row) * 8, rows[row][col], true);
  }
  return bytes;
}
function native(binary, meta, text, bytes) {
  writeFileSync(sourcePath, text);
  writeFileSync(dataPath, bytes);
  const stamp = (ms) => new Date(ms).toISOString().replace('.000Z', 'Z');
  return execFileSync(resolve(binary), ['master-report', `--dsl-file=${sourcePath}`, `--m5-file=${dataPath}`,
    `--warmup-from=${stamp(meta.warmupFromT)}`, `--trade-from=${stamp(meta.tradeFromT)}`,
    `--trade-to=${stamp(meta.tradeToT)}`, `--spread=${meta.spread}`], { encoding: 'utf8', maxBuffer: 64 * 1024 * 1024 });
}
function identical(actual, expected, label) {
  assert.equal(typeof actual, 'string', `${label}: expected a JSON string`);
  if (actual !== expected) assert.fail(`${label}: byte mismatch (${digest(actual)} != ${digest(expected)})`);
}
function compare(label, rows, meta = metadata, text = source(), suppliedBytes) {
  const bytes = suppliedBytes ?? encode(rows);
  const wasm = globalThis.engineRunMasterReport(JSON.stringify(meta), text, bytes);
  const result = JSON.parse(wasm);
  assert.equal(result.error, undefined, `${label}: ${result.error}`);
  const cli = native(nativeCLI, meta, text, bytes);
  identical(wasm, cli, `${label}/native-WASM`); // Including indentation and trailing newline.
  if (beforeCLI) identical(cli, native(beforeCLI, meta, text, bytes), `${label}/before-after`);
  assert.equal(result.schema, 'strat-master-structural-cli-v1');
  assert.equal(result.dslSha256, digest(Buffer.from(text, 'utf8')));
  assert.equal(result.dataSha256, digest(bytes));
  assert.equal(result.run.pineParityVerified, false);
  assert.equal(result.run.costComplete, false);
  evidence.comparisons.push({ label, rows: rows.length, trades: result.run.trades.length, sha256: digest(wasm) });
  return { raw: wasm, envelope: result, run: result.run };
}
function reject(label, meta = JSON.stringify(metadata), text = source(), bytes = baseBytes) {
  let raw;
  assert.doesNotThrow(() => { raw = globalThis.engineRunMasterReport(meta, text, bytes); }, `${label}: exception escaped the bridge`);
  assert.equal(typeof raw, 'string', `${label}: rejection must be a JSON string`);
  const result = JSON.parse(raw);
  assert.deepEqual(Object.keys(result), ['error'], `${label}: rejection leaked a partial result`);
  assert.equal(typeof result.error, 'string');
  assert(result.error.length > 0, `${label}: empty error`);
  evidence.rejected++;
  return result.error;
}
function mutateBytes(bytes, mutate) {
  const copy = bytes.slice();
  mutate(new DataView(copy.buffer), copy);
  return copy;
}
const wave = waveform(700), base = wave.slice(0, 500 * 6), baseBytes = encode(base);
const mirror = base.map(([t, o, h, l, c, v]) => [t, 250 - o, 250 - l, 250 - h, 250 - c, v]);

try {
  for (const mode of modes) {
    for (const spread of [0, 1]) {
      const meta = { ...metadata, spread }, text = source(mode), label = `${mode}/spread-${spread}`;
      const baseline = compare(label, base, meta, text);
      const reflected = compare(`${label}/reflected`, mirror, meta, text);
      for (const run of [baseline.run, reflected.run]) {
        assert(run.trades.length > 0 && run.signals.length > 0, `${label}: vacuous trade coverage`);
        for (const trade of run.trades) {
          assert(trade.entryT >= trade.signal.time && trade.exitT >= trade.entryT && trade.exitT <= meta.tradeToT);
        }
      }
      const directions = new Set([...baseline.run.trades, ...reflected.run.trades].map((trade) => trade.signal.direction));
      assert(directions.has(1) && directions.has(-1), `${label}: must exercise both long and short trades`);
      const future = compare(`${label}/future-appended`, wave, meta, text);
      const changed = wave.map((row, i) => i < base.length ? row : row.map((v, col) => col === 0 ? v : v * (col === 5 ? 10 : 2)));
      const mutated = compare(`${label}/future-mutated`, changed, meta, text);
      assert.deepEqual(future.run, baseline.run, `${label}: future append changed the run`);
      assert.deepEqual(mutated.run, baseline.run, `${label}: future mutation changed the run`);
      assert.notEqual(future.envelope.dataSha256, baseline.envelope.dataSha256);
      assert.notEqual(mutated.envelope.dataSha256, future.envelope.dataSha256);

      // Remove a full native bucket and one interior M5 row. Coverage remains
      // explicit: no invented candles and no false complete-bar certification.
      const gapped = base.filter((row, i) => i !== 2 && !(i >= 200 * 6 && i < 201 * 6));
      const gap = compare(`${label}/gaps-partial`, gapped, meta, text).run;
      assert.equal(gap.usedM5Rows, gapped.length);
      assert.equal(gap.indicators.length, 499);
      assert.equal(gap.indicators[0].count, 5);
      assert.equal(gap.indicators[0].complete, false);
      assert.equal(gap.h4[0].count, 47);
      assert.equal(gap.h4[0].complete, false);
      assert(!gap.indicators.some((row) => row.bucketT === 200 * M30));

      const firstTrade = baseline.run.trades[0];
      const partialRows = base.filter((row) => row[0] !== firstTrade.entryBucketT);
      const partial = compare(`${label}/partial-entry`, partialRows, meta, text).run;
      const position = [...partial.trades, ...(partial.openPosition ? [partial.openPosition] : [])].find((p) => p.signal.time === firstTrade.signal.time);
      assert(position, `${label}: partial future bar canceled an accepted entry`);
      assert.equal(position.entryT, firstTrade.entryBucketT + M5);
      assert(partial.summary.partialEntryFills > 0 && partial.summary.partialPositionBars > 0);

      // Stop immediately after a real fill, before its future exit. Pick an
      // existing multi-bar trade; this tests terminal exposure without a search.
      const trade = baseline.run.trades.find((t) => t.exitT > t.entryBucketT + M30);
      assert(trade, `${label}: no multi-bar trade available for terminal exposure`);
      const until = trade.entryBucketT + M30;
      const terminal = compare(`${label}/terminal-open`, base.filter((row) => row[0] < until), { ...meta, tradeToT: until }, text).run;
      assert(terminal.openPosition, `${label}: terminal liquidation was fabricated`);
      assert.equal(terminal.openPosition.entryT, trade.entryT);
      assert(!terminal.trades.some((t) => t.entryT === trade.entryT));
      assert.equal(terminal.summary.openEntryFee, terminal.openPosition.entryFee);
      assert.equal(terminal.pendingSignal, null);
      // A pending terminal signal is unreachable with a closed watermark:
      // close==tradeTo is excluded; every earlier close has a later observed row.
      const excluded = compare(`${label}/terminal-signal-excluded`, base.filter((row) => row[0] < trade.signal.time), { ...meta, tradeToT: trade.signal.time }, text).run;
      assert(!excluded.signals.some((s) => s.time === trade.signal.time));
      assert.equal(excluded.pendingSignal, null);
    }
  }

  // Hand-derived H4 horizon transition from the Go unit test, in both modes.
  const boundary = Array.from({ length: 96 * 6 }, (_, i) => {
    const p = i >= 80 * 6 && i < 88 * 6 ? 110 : 100;
    return [i * M5, p, p + 1, p - 1, p, 10];
  });
  for (const mode of modes) {
    const run = compare(`${mode}/H4-boundary`, boundary, { ...metadata, tradeToT: 96 * M30 }, source(mode)).run;
    assert.equal(run.h4.length, 12);
    assert.equal(run.indicators[6].historicalH4, null);
    assert.equal(run.indicators[7].historicalH4.availableT, H4);
    assert.equal(run.indicators[7].stableH4, null);
    assert.equal(run.indicators[8].stableH4.availableT, H4);
    assert.equal(run.indicators[87].historicalH4.regime, -1);
    assert.equal(run.indicators[87].stableH4.regime, 1);
    assert.equal(run.indicators[87].historicalH4.availableT, 11 * H4);
    assert.equal(run.indicators[87].stableH4.availableT, 10 * H4);
    assert.equal(run.indicators[88].stableH4.regime, -1);
  }

  // Correct byte offsets must ignore unrelated prefix and suffix bytes.
  const backing = new Uint8Array(baseBytes.length + 31).fill(0xa5);
  backing.set(baseBytes, 13);
  compare('nonzero-byte-offset', base, metadata, source(), backing.subarray(13, 13 + baseBytes.length));
  compare('valid-astral-source', base, metadata, source(modes[0], 'Invented Ω 😀 𝄞'));
  const paddedSource = source() + '#' + ' '.repeat(65536 - Buffer.byteLength(source()) - 1);
  compare('source-exact-byte-limit', base, metadata, paddedSource);
  const paddedMeta = JSON.stringify(metadata).padEnd(4096, ' ');
  identical(globalThis.engineRunMasterReport(paddedMeta, source(), baseBytes), native(nativeCLI, metadata, source(), baseBytes), 'metadata-exact-byte-limit');
  reject('metadata-over-byte-limit', paddedMeta + ' ');
  reject('source-over-byte-limit', JSON.stringify(metadata), paddedSource + ' ');
  reject('source-UTF8-byte-limit', JSON.stringify(metadata), source() + '# ' + '😀'.repeat(17000));

  for (const raw of ['null', '[]', 'true', '0', '"object"', '', '{', '{}', JSON.stringify(metadata) + '{}', JSON.stringify(metadata) + ' junk']) reject('malformed-metadata', raw);
  for (const key of Object.keys(metadata)) {
    const missing = { ...metadata }; delete missing[key];
    reject(`missing-${key}`, JSON.stringify(missing));
    reject(`null-${key}`, JSON.stringify({ ...metadata, [key]: null }));
    reject(`duplicate-${key}`, JSON.stringify(metadata).replace(/}$/, `,"${key}":${JSON.stringify(metadata[key])}}`));
    const alias = { ...missing, [key.toUpperCase()]: metadata[key] };
    reject(`case-alias-${key}`, JSON.stringify(alias));
  }
  reject('escaped-duplicate', JSON.stringify(metadata).replace(/}$/, ',"\\u0073chema":"master-structural-runtime-request-v1"}'));
  reject('unknown-field', JSON.stringify({ ...metadata, extra: true }));
  reject('wrong-schema', JSON.stringify({ ...metadata, schema: 'master-structural-runtime-request-v2' }));
  for (const key of ['warmupFromT', 'tradeFromT', 'tradeToT']) {
    for (const value of [-M30, 9007199254740992, .5, 1, '0', true, [], {}]) reject(`bad-${key}`, JSON.stringify({ ...metadata, [key]: value }));
  }
  for (const changes of [{ warmupFromT: metadata.tradeFromT }, { tradeFromT: metadata.tradeToT }, { tradeToT: 0 }, { warmupFromT: 2 * metadata.tradeToT }]) reject('endpoint-order', JSON.stringify({ ...metadata, ...changes }));
  for (const spread of [-1, .5, 2, '0', false]) reject('bad-spread', JSON.stringify({ ...metadata, spread }));
  // Preserve the raw JSON token: JSON.stringify would round these back to
  // allowed values and erase the admission defect the regression exercises.
  for (const token of ['1e-999', '-1e-999', '1.0000000000000001', '0.99999999999999999', '1e0']) {
    reject(`inexact-or-exponent-spread-${token}`, JSON.stringify(metadata).replace('"spread":0', `"spread":${token}`));
  }
  for (const key of ['warmupFromT', 'tradeFromT', 'tradeToT', 'spread']) reject(`overflow-${key}`, JSON.stringify(metadata).replace(new RegExp(`"${key}":\\d+`), `"${key}":1e999`));

  for (const bytes of [new Uint8Array(), baseBytes.subarray(0, 15), baseBytes.subarray(0, baseBytes.length - 1), new Uint8Array([...baseBytes, 0])]) reject('bad-BTB1-length', undefined, undefined, bytes);
  for (const [offset, values] of [[0, [0]], [4, [0, 2]], [8, [0, 1, 100001, 0xffffffff]], [12, [0, 5, 7, 0xffffffff]]]) {
    for (const value of values) reject('bad-BTB1-header', undefined, undefined, mutateBytes(baseBytes, (view) => view.setUint32(offset, value, true)));
  }
  const tooMany = new Uint8Array(16 + 100001 * 48);
  [0x31425442, 1, 100001, 6].forEach((value, i) => new DataView(tooMany.buffer).setUint32(i * 4, value, true));
  reject('BTB1-row-limit', undefined, undefined, tooMany);
  // Sparse M5 input can generate many native rows. Reject its conservative
  // output shape before execution, without coupling this test to a byte cap.
  const sparse = Array.from({ length: 100000 }, (_, i) => [i === 0 ? 0 : i * M30 + 5 * M5, 100, 101, 99, 100, 10]);
  assert.match(reject('conservative-output-bound', JSON.stringify({ ...metadata, tradeToT: sparse.length * M30 }), undefined, encode(sparse)), /conservative.*output|output.*bound/i);
  for (let col = 0; col < 6; col++) {
    for (const value of [NaN, Infinity, -Infinity]) reject(`nonfinite-column-${col}`, undefined, undefined, mutateBytes(baseBytes, (view) => view.setFloat64(16 + (col * base.length + 3) * 8, value, true)));
  }
  for (const [col, row, value] of [[0, 1, 0], [0, 2, M5], [0, 1, M5 + 1], [0, 1, M5 + .5], [0, 1, -M5], [0, 1, 9007199254740992], [2, 3, 1], [3, 3, 0], [5, 3, -1]]) {
    reject('invalid-M5-row', undefined, undefined, mutateBytes(baseBytes, (view) => view.setFloat64(16 + (col * base.length + row) * 8, value, true)));
  }
  reject('missing-source-start', undefined, undefined, encode(base.slice(1)));
  reject('unclosed-source-tail', undefined, undefined, encode(base.slice(0, -1)));

  for (const bytes of [null, [], {}, baseBytes.buffer, new DataView(baseBytes.buffer), new Int8Array(baseBytes.buffer), new Uint8ClampedArray(baseBytes.buffer), new Uint16Array(baseBytes.buffer), new Float64Array(1)]) reject('wrong-byte-container', undefined, undefined, bytes);
  if (typeof SharedArrayBuffer === 'function') {
    const shared = new Uint8Array(new SharedArrayBuffer(baseBytes.length)); shared.set(baseBytes);
    reject('shared-byte-buffer', undefined, undefined, shared);
  }
  const detached = baseBytes.slice();
  structuredClone(detached.buffer, { transfer: [detached.buffer] });
  reject('detached-byte-buffer', undefined, undefined, detached);
  reject('proxied-byte-view', undefined, undefined, new Proxy(baseBytes, {}));
  reject('prototype-only-byte-imitation', undefined, undefined, Object.create(Uint8Array.prototype));
  const recovered = compare('transport-after-refusals', base);
  const shadowed = backing.subarray(13, 13 + baseBytes.length);
  for (const key of ['byteLength', 'byteOffset', 'buffer']) {
    Object.defineProperty(shadowed, key, { get() { throw new Error(`shadowed ${key} getter must not execute`); } });
  }
  identical(globalThis.engineRunMasterReport(JSON.stringify(metadata), source(), shadowed), recovered.raw, 'intrinsic-accessors-ignore-shadowed-fields');
  evidence.shadowedAccessorsVerified = true;
  for (const [meta, text] of [[metadata, source()], [null, source()], [JSON.stringify(metadata), null], [JSON.stringify(metadata), new String(source())]]) reject('wrong-string-type', meta, text);
  for (const args of [[], [JSON.stringify(metadata), source()], [JSON.stringify(metadata), source(), baseBytes, 'extra']]) {
    const result = JSON.parse(globalThis.engineRunMasterReport(...args));
    assert.deepEqual(Object.keys(result), ['error']); assert(result.error); evidence.rejected++;
  }
  for (const units of [[0xd800], [0xdc00], [0xd800, 65], [0xdc00, 0xd800], [0xd800, 0xdc00, 0xdc00]]) {
    const invalid = String.fromCharCode(...units);
    for (const text of [source(modes[0], invalid), '# ' + invalid + '\n' + source(), source() + '# ' + invalid]) {
      assert.match(reject('malformed-UTF16-source', undefined, text), /UTF-16|surrogate/);
    }
  }

  const legacy = 'dsl v7\nstrategy "Legacy synthetic control"\nsetup { type: failed breakout }\n';
  for (const text of [legacy, source().replace('SOURCE_HISTORICAL_REFERENCE', 'UNKNOWN'), source().replace('v10-phase0-floor-half-reference-v1', 'UNKNOWN'), 'dsl v7\nstrategy "Bad intent"\nsetup { risk 100 USD master mode SOURCE_HISTORICAL_REFERENCE }', 'dsl v7\nstrategy "Bad intent"\nsetup { type: flag continuation master\nmode SOURCE_HISTORICAL_REFERENCE }']) reject('non-Master-or-malformed-intent', undefined, text);
  const fixture = JSON.stringify({ schema: 'dsl-conformance-run-fixture-v1', case: 'invented-master-boundary', symbol: 'XAUUSD', timeframe: '1m', bars: [[0, 100, 101, 99, 100, 1]] });
  const columnsMeta = JSON.stringify({ schema: 'enginewasm-columnar-v1', case: 'invented-master-boundary', symbol: 'XAUUSD', timeframe: '1m' });
  const columns = [0, 100, 101, 99, 100, 1].map((value) => new Float64Array([value]));
  for (const mode of modes) {
    const fixtureError = JSON.parse(globalThis.engineRunFixture(fixture, source(mode)));
    assert.match(fixtureError.error, /master-structural-native-dedicated-runner-required/);
    assert.deepEqual(Object.keys(fixtureError), ['error']);
    const columnError = globalThis.engineRunColumns(columnsMeta, source(mode), ...columns);
    assert.equal(columnError.ok, false);
    assert.match(columnError.error, /master-structural-native-dedicated-runner-required/);
    evidence.genericRefusals += 2;
  }
  const legacyFixture = JSON.parse(globalThis.engineRunFixture(fixture, legacy));
  assert.equal(legacyFixture.error, undefined); assert.equal(legacyFixture.tradeCount, 0);
  const legacyColumns = globalThis.engineRunColumns(columnsMeta, legacy, ...columns);
  assert.equal(legacyColumns.ok, true); assert.equal(JSON.parse(legacyColumns.summaryJSON).tradeCount, 0); assert.equal(legacyColumns.trades.length, 0);
  evidence.legacyControls = 2;
  evidence.beforeAfterVerified = Boolean(beforeCLI);
  console.log(JSON.stringify({ ...evidence, status: 'PASS' }, null, 2));
} finally {
  rmSync(scratch, { recursive: true, force: true });
}
process.exit(0);
