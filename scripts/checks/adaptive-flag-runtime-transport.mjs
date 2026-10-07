// Dedicated adaptive runtime JS/WASM transport checks using invented bars only.
// Usage: node scripts/checks/adaptive-flag-runtime-transport.mjs \
//   <enginewasm.wasm> <wasm_exec.js> [receipt.json]
// The shared Go report and corpus parity checks qualify report execution;
// these checks exercise the original JS call boundary and recovery behavior.
import assert from 'node:assert/strict';
import { emitAdaptiveReceipt } from './adaptive-runtime-receipt.mjs';
import { createHash, webcrypto } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { resolve } from 'node:path';
import { runInNewContext } from 'node:vm';

const [wasmPath, shimPath, receiptPath, ...extra] = process.argv.slice(2);
assert(wasmPath && shimPath && !extra.length, 'two artifact paths and optionally one receipt path are required');
globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(resolve(shimPath));

// Test-only instrumentation around the captured intrinsic. The injected fault
// targets the bridge's DataView calls, never the Go shim's own memory accesses.
// This proves the actual copy exception path is recoverable, without claiming
// security against arbitrary modifications to an executing worker's globals.
const originalReadWord = DataView.prototype.getUint32;
const dataViewBuffer = Object.getOwnPropertyDescriptor(DataView.prototype, 'buffer').get;
const dataViewLength = Object.getOwnPropertyDescriptor(DataView.prototype, 'byteLength').get;
const observedBuffers = new WeakSet();
let reads = [], faultAt = 0;
DataView.prototype.getUint32 = function (offset, littleEndian) {
  if (observedBuffers.has(dataViewBuffer.call(this))) {
    reads.push({ offset, length: dataViewLength.call(this) });
    if (faultAt && reads.length === faultAt) {
      faultAt = 0;
      throw new Error('injected adaptive transport copy trap');
    }
  }
  return originalReadWord.call(this, offset, littleEndian);
};
const wasmBytes = readFileSync(wasmPath);
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(wasmBytes, go.importObject);
void go.run(instance);
DataView.prototype.getUint32 = originalReadWord;
const run = globalThis.engineRunAdaptiveFlagReport;
assert.equal(typeof run, 'function', 'dedicated export is absent');
const hash = (bytes) => createHash('sha256').update(bytes).digest('hex');
const M30 = 1800000;
const request = { schema: 'adaptive-flag-runtime-request-v1', numericalPolicy: 'BINARY64_ORDERED_V1', symbol: 'XAUUSD', timeframe: 'M30', executionWindow: null };
const metadata = JSON.stringify(request);
const sourceFor = (bundle = 'initial') => readFileSync(new URL(`../../conformance/parse/family-adaptive-volume-flag-${bundle}.strat`, import.meta.url), 'utf8');
const source = sourceFor();
function observe(bytes) { observedBuffers.add(bytes.buffer); return bytes; }
function encode(rows) {
  const bytes = observe(new Uint8Array(16 + rows.length * 48));
  const data = new DataView(bytes.buffer);
  [0x31425442, 1, rows.length, 6].forEach((value, i) => data.setUint32(i * 4, value, true));
  for (let column = 0; column < 6; column++) for (let row = 0; row < rows.length; row++) {
    data.setFloat64(16 + (column * rows.length + row) * 8, rows[row][column], true);
  }
  return bytes;
}
const rows = Array.from({ length: 4 }, (_, i) => [i * M30, 100, 101, 99, 100, i === 0 ? -0 : 1]);
const data = encode(rows);
const evidence = { schema: 'adaptive-runtime-wasm-transport-check-v1', wasmSha256: hash(wasmBytes), node: process.version, accepted: [], rejected: [], recoveryCalls: 0, genericRefusals: 0 };
function success(label, meta = metadata, text = source, bytes = data) {
  reads = [];
  const raw = run(meta, text, bytes);
  assert.equal(typeof raw, 'string', `${label}: expected string result`);
  const document = JSON.parse(raw);
  assert.equal(document.error, undefined, `${label}: ${JSON.stringify(document.error)}`);
  assert.deepEqual(Object.keys(document), ['schema', 'request', 'requestSha256', 'dslSha256', 'configSha256', 'btb1Sha256', 'rawOnly', 'economics', 'config', 'run']);
  assert.equal(document.schema, 'strat-adaptive-volume-flag-runtime-v1');
  assert.equal(document.requestSha256, hash(meta));
  assert.equal(document.dslSha256, hash(text));
  assert.equal(document.btb1Sha256, hash(bytes));
  assert.equal(document.rawOnly, true);
  assert.equal(document.economics, 'unavailable-stage-a');
  for (const key of ['researchPolicy', 'researchPolicySha256', 'researchProducer']) assert.equal(document.run[key], undefined);
  assert(raw.endsWith('\n') && !raw.endsWith('\n\n'), 'exact final newline');
  evidence.accepted.push({ label, reportSha256: hash(raw), bytes: Buffer.byteLength(raw) });
  return raw;
}
const baseline = success('baseline');
assert.equal(reads.length, 4 + data.length / 4, 'header and full copy read counts');
assert(reads.slice(0, 4).every((item) => item.length === 16), 'header view is bounded to 16 bytes');
assert(reads.slice(4).every((item) => item.length === data.length), 'full copy uses exactly the selected view');

function reject(label, args, phase, expectedReads) {
  reads = [];
  let raw;
  assert.doesNotThrow(() => { raw = run(...args); }, `${label}: exception escaped the export`);
  assert.equal(typeof raw, 'string');
  const document = JSON.parse(raw);
  assert.deepEqual(Object.keys(document), ['error'], `${label}: partial success data`);
  assert.deepEqual(Object.keys(document.error), ['code', 'phase', 'message']);
  assert.equal(document.error.code, 'ADAPTIVE_RUNTIME_REJECTED');
  assert(['request', 'source', 'input', 'resource', 'execution'].includes(document.error.phase));
  if (phase) assert.equal(document.error.phase, phase, `${label}: phase`);
  assert.equal(typeof document.error.message, 'string');
  assert(Buffer.byteLength(document.error.message) > 0 && Buffer.byteLength(document.error.message) <= 1024);
  if (expectedReads !== undefined) assert.equal(reads.length, expectedReads, `${label}: data was copied before admission`);
  evidence.rejected.push({ label, phase: document.error.phase, wordReads: reads.length });
  faultAt = 0;
  assert.equal(run(metadata, source, data), baseline, `${label}: a rejected call poisoned the runtime`);
  evidence.recoveryCalls++;
  return document.error;
}
function rejectSource(label, text, phase = 'source') { reject(label, [metadata, text, data], phase, 0); }
function rejectInput(label, bytes, phase = 'input', expectedReads) { reject(label, [metadata, source, bytes], phase, expectedReads); }
function mutate(mutateBytes) { const copy = observe(data.slice()); mutateBytes(new DataView(copy.buffer), copy); return copy; }

for (const args of [[], [metadata], [metadata, source], [metadata, source, data, 0]]) reject('exact-three-arguments', args, 'request', 0);
let coercions = 0;
const coercible = { toString() { coercions++; throw new Error('must not coerce'); } };
for (const value of [undefined, null, 0, true, new String(metadata), coercible]) reject('metadata-primitive-string', [value, source, data], 'request', 0);
for (const value of [undefined, null, 0, true, new String(source), coercible]) rejectSource('source-primitive-string', value);
assert.equal(coercions, 0);

const invalidUnits = [[0xd800], [0xdbff], [0xdc00], [0xdfff], [0xd800, 65], [0xd800, 0xd800, 0xdc00], [0xdc00, 0xd800], [0xd800, 0xdc00, 0xd800], [0xd800, 0xfffd, 0xdc00]];
for (const units of invalidUnits) {
  const text = String.fromCharCode(...units);
  rejectSource('original-source-UTF16', `${source}\n# ${text}`);
  reject('original-request-UTF16', [metadata.replace('XAUUSD', text), source, data], 'request', 0);
}
for (const text of ['\ufffd', '\u{10000}', '\u{10ffff}', 'e\u0301', 'é']) success('valid-original-Unicode', metadata, `${source}\n# ${text}`);
for (const text of ['\\ud800', '\\udfff']) {
  rejectSource('source-unpaired-escape', source.replace('synthetic control', text));
  reject('request-unpaired-escape', [metadata.replace('XAUUSD', text), source, data], 'request', 0);
}
success('source-exact-4096', metadata, source.padEnd(4096));
rejectSource('source-4097-units', source.padEnd(4097), 'resource');
rejectSource('source-UTF8-overlimit-before-conversion', `${source}\n# ${'\u0800'.repeat(1400)}`, 'resource');
success('metadata-exact-4096', metadata.padEnd(4096));
reject('metadata-4097-units', [metadata.padEnd(4097), source, data], 'resource', 0);
reject('metadata-UTF8-overlimit', [`{"${'\u0800'.repeat(1400)}":0}`, source, data], 'resource', 0);

for (const bundle of ['initial', 'tweaked', 'snapshot_c']) for (const timeframe of ['M30', 'H1']) {
  const meta = JSON.stringify({ ...request, timeframe });
  const text = sourceFor(bundle).replace('timeframe M30', `timeframe ${timeframe}`);
  const bytes = timeframe === 'M30' ? data : encode(rows.map((row) => [row[0] * 2, ...row.slice(1)]));
  success(`named-${bundle}-${timeframe}`, meta, text, bytes);
}
for (const field of ['equity', 'startEquity', 'statistics', 'pnl', 'returnPct', 'costs', 'risk', 'quantity', 'fee', 'spread', 'slippage', 'fillOn', 'researchAblation', 'checkpoint', 'resume', 'sourceTf', 'htfBars', 'aggregation']) {
  reject(`unknown-${field}`, [JSON.stringify({ ...request, [field]: 0 }), source, data], 'request', 0);
}
const boundedError = reject('bounded-multibyte-error-message', [JSON.stringify({ ...request, ['é'.repeat(1400)]: 0 }), source, data], 'request', 0);
assert(Buffer.byteLength(boundedError.message) >= 1023, 'control did not exercise the 1024-byte error cap');
assert(!boundedError.message.includes('\ufffd'), 'error cap split a UTF-8 code point');
for (const text of ['', source + '\nunknown', source.replace('bundle INITIAL', 'bundle CUSTOM'), source.replace('targetR 2.5', 'targetR 2.6'), source.replace('timeframe M30', 'timeframe H1'), source.replace('bundle INITIAL', 'bundle G1'), source.replace('bundle INITIAL', 'bundle F1')]) rejectSource('closed-baseline-source-profile', text);
rejectSource('mixed-family', source + '\nmarket { clock timeframe M30 }');
rejectSource('wrong-family-bounded-cartesian', `dsl v7\nstrategy "wrong family"\nmarket { clock symbols ${Array(32).fill('XAUUSD').join(',')} timeframes ${Array(32).fill('M30').join(',')} }\nsetup { type: clock range breakout }`);
rejectSource('bounded-malformed-inline', 'dsl v7\n' + 'strategy "x" {'.repeat(250));
reject('duplicate-decoded-key', [metadata.replace('"schema":', '"sch\\u0065ma":"adaptive-flag-runtime-request-v1","schema":'), source, data], 'request', 0);
success('escaped-canonical-key', metadata.replace('"schema":', '"sch\\u0065ma":'));

const backing = observe(new Uint8Array(data.length + 37).fill(0xa5));
backing.set(data, 13);
assert.equal(success('unaligned-selected-view', metadata, source, backing.subarray(13, 13 + data.length)), baseline);
assert(reads.every((item) => item.length <= data.length), 'unrelated backing bytes were copied');
let ownAccesses = 0;
const shadowed = observe(data.slice());
for (const key of ['buffer', 'byteOffset', 'byteLength', 'length', 'constructor', 'subarray']) {
  Object.defineProperty(shadowed, key, { get() { ownAccesses++; throw new Error(`own ${key} must not run`); } });
}
// Save the real buffer before installing caller-owned shadow fields.
reads = [];
assert.equal(run(metadata, source, shadowed), baseline, 'own accessors must not affect transport');
assert.equal(ownAccesses, 0);
evidence.accepted.push({ label: 'shadowed-own-accessors-ignored', reportSha256: hash(baseline) });
const shadowedBuffer = new ArrayBuffer(data.length);
const bufferView = observe(new Uint8Array(shadowedBuffer));
bufferView.set(data);
for (const key of ['byteLength', 'constructor']) Object.defineProperty(shadowedBuffer, key, { get() { ownAccesses++; throw new Error('buffer own accessor'); } });
assert.equal(run(metadata, source, bufferView), baseline);
assert.equal(ownAccesses, 0);

for (const value of [undefined, null, [], {}, data.buffer, new DataView(data.buffer), new Int8Array(data.buffer), new Uint8ClampedArray(data.buffer), Object.create(Uint8Array.prototype), Buffer.from(data), new (class extends Uint8Array {})(data), new Uint8Array(new (class extends ArrayBuffer {})(data.length)), runInNewContext('new Uint8Array(64)')]) rejectInput('ordinary-view-required', value);
for (const constructor of [Int8Array, Uint8ClampedArray, Int16Array, Uint16Array, Int32Array, Uint32Array, Float32Array, Float64Array, BigInt64Array, BigUint64Array]) {
  const wrongKind = new constructor(data.buffer.slice(0));
  observedBuffers.add(wrongKind.buffer);
  Object.setPrototypeOf(wrongKind, Uint8Array.prototype);
  Object.defineProperty(wrongKind, Symbol.toStringTag, { get() { throw new Error('own type tag must not run'); } });
  rejectInput(`prototype-mutated-${constructor.name}`, wrongKind, 'input', 0);
}
let proxyAccesses = 0;
const hostileProxy = new Proxy(data, { get() { proxyAccesses++; throw new Error('proxy get'); }, getPrototypeOf() { proxyAccesses++; throw new Error('proxy prototype'); } });
rejectInput('proxy-view-brand-check', hostileProxy, 'input', 0);
assert.equal(proxyAccesses, 0, 'intrinsic brand check must precede proxy traps');
const revoked = Proxy.revocable(data, {}); revoked.revoke();
rejectInput('revoked-proxy', revoked.proxy, 'input', 0);
if (typeof SharedArrayBuffer !== 'undefined') rejectInput('shared-buffer', new Uint8Array(new SharedArrayBuffer(data.length)), 'input', 0);
const detached = data.slice(); structuredClone(detached.buffer, { transfer: [detached.buffer] });
rejectInput('detached-view', detached, 'input', 0);
rejectInput('empty-view', new Uint8Array(0), 'input', 0);
rejectInput('short-header', new Uint8Array(15), 'input', 0);
rejectInput('byte-budget', new Uint8Array(786449), 'resource', 0);
for (const [label, offset, value] of [['magic', 0, 0], ['version', 4, 2], ['zero-rows', 8, 0], ['provided-rows-16385', 8, 16385], ['overflow-count', 8, 0xffffffff], ['columns', 12, 7]]) {
  rejectInput(`header-${label}`, mutate((view) => view.setUint32(offset, value, true)), 'input', 4);
}
rejectInput('exact-envelope-extra-byte', observe(new Uint8Array([...data, 0])), 'input', 4);
rejectInput('exact-envelope-missing-byte', observe(data.slice(0, -1)), 'input', 4);
faultAt = 1;
rejectInput('header-copy-trap-recovery', data, 'input', 1);
faultAt = 5;
rejectInput('full-copy-trap-recovery', data, 'input', 5);

const tooMany = encode(Array.from({ length: 4097 }, (_, i) => [i * M30, 100, 101, 99, 100, 1]));
rejectInput('retained-row-budget', tooMany, 'resource');
rejectInput('retained-NaN', mutate((view) => view.setFloat64(16, NaN, true)), 'input');
rejectInput('retained-magnitude', mutate((view) => view.setFloat64(16 + 5 * rows.length * 8, 1e101, true)), 'input');
const excluded = encode([...rows, [Infinity, NaN, NaN, NaN, NaN, NaN]]);
const window = JSON.stringify({ ...request, executionWindow: { tradeFromMs: 0, tradeToMs: rows.length * M30 } });
const included = JSON.parse(success('window-retained-prefix', window));
const ignored = JSON.parse(success('ignored-positive-infinity-boundary', window, source, excluded));
assert.equal(ignored.run.providedSourceRows, included.run.providedSourceRows + 1);
assert.equal(ignored.run.ignoredSuffixRows, included.run.ignoredSuffixRows + 1);
for (const key of Object.keys(included.run)) {
  if (key !== 'providedSourceRows' && key !== 'ignoredSuffixRows') assert.deepEqual(ignored.run[key], included.run[key], `excluded suffix altered retained run field ${key}`);
}
assert.notEqual(ignored.btb1Sha256, included.btb1Sha256, 'full-byte identity ignored supplied suffix');

const fixture = JSON.stringify({ schema: 'dsl-conformance-run-fixture-v1', symbol: 'XAUUSD', timeframe: '30m', bars: rows });
const columns = Array.from({ length: 6 }, (_, col) => new Float64Array(rows.map((row) => row[col])));
assert(JSON.parse(globalThis.engineRunFixture(fixture, source)).error, 'generic fixture route accepted adaptive source');
evidence.genericRefusals++;
const generic = globalThis.engineRunColumns(JSON.stringify({ schema: 'enginewasm-columnar-v1', symbol: 'XAUUSD', timeframe: '30m' }), source, ...columns);
assert.equal(generic.ok, false, 'generic column route accepted adaptive source');
assert(generic.error);
evidence.genericRefusals++;
assert.equal(run(metadata, source, data), baseline);
evidence.status = 'PASS';
await emitAdaptiveReceipt(evidence, receiptPath);
process.exit(0);
