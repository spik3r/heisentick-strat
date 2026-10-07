// Dedicated Master Structural runtime parity, using invented data only.
// Usage: node scripts/checks/master-runtime-parity.mjs \
//   <native CLI> <enginewasm.wasm> <wasm_exec.js>
// All architectures read the same frozen invented BTB1 bytes and fixed windows.
import assert from 'node:assert/strict';
import { createHash, webcrypto } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { gunzipSync } from 'node:zlib';
import { createRequire } from 'node:module';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

const [nativeCLI, engineWasm, shim, ...extra] = process.argv.slice(2);
assert(nativeCLI && engineWasm && shim && !extra.length, 'exactly three paths are required');
globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(resolve(shim));
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(readFileSync(engineWasm), go.importObject);
void go.run(instance);
assert.equal(typeof globalThis.engineRunMasterPortableReport, 'function', 'dedicated WASM export is absent');

assert.equal(globalThis.engineRunMasterReport, undefined, 'withdrawn nonportable export must not be an alias');

const M5 = 300000, M30 = 6 * M5, H4 = 8 * M30;
const modes = ['SOURCE_HISTORICAL_REFERENCE', 'PROTECTED_STABLE_REFERENCE'];
const metadata = { schema: 'master-structural-portable-runtime-request-v1', arithmeticContract: 'master-binary64-separated-v1', warmupFromT: 0, tradeFromT: 40 * M30, tradeToT: 500 * M30, spread: 0 };
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

// Immutable synthetic bytes. No host transcendental generation or output-selected windows.
const corpusRaw = readFileSync(new URL('../../testsupport/testdata/master-portable-corpus-v1.json', import.meta.url));
assert(corpusRaw.length <= 2 * 1024 * 1024);
assert.equal(digest(corpusRaw), '1dc35945d0822756ba1b2be70182858eff7c80f24ac315c23d6cda4cd8107b32', 'frozen corpus changed');
const corpus = JSON.parse(corpusRaw);
assert.equal(corpus.schema, 'master-portable-invented-corpus-v1');
assert.equal(corpus.cases.length, 38); assert.equal(Object.keys(corpus.assets).length, 11);
const assets = new Map();
for (const [key, asset] of Object.entries(corpus.assets)) {
  assert.equal(asset.encoding, 'gzip-base64-btb1');
  assert(asset.rows > 0 && asset.rows <= 100000 && asset.byteLength === 16 + asset.rows * 48);
  assert(asset.payload.length <= 6400024);
  const bytes = new Uint8Array(gunzipSync(Buffer.from(asset.payload, 'base64'), { maxOutputLength: 4800016 }));
  assert.equal(bytes.length, asset.byteLength); assert.equal(digest(bytes), key); assert.equal(asset.sha256, key);
  assets.set(key, bytes);
}
for (const item of corpus.cases) {
  assert.equal(digest(item.source), item.sourceSha256); assert(assets.has(item.dataSha256));
  assert.equal(item.metadata.schema, metadata.schema); assert.equal(item.metadata.arithmeticContract, metadata.arithmeticContract);
}
evidence.corpusSha256 = digest(corpusRaw);
function rowsFrom(bytes) {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength), n = view.getUint32(8, true);
  return Array.from({length:n},(_,i)=>Array.from({length:6},(_,col)=>view.getFloat64(16+(col*n+i)*8,true)));
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
  return execFileSync(resolve(binary), ['master-portable-report', `--arithmetic-contract=${meta.arithmeticContract}`, `--dsl-file=${sourcePath}`, `--m5-file=${dataPath}`,
    `--warmup-from=${stamp(meta.warmupFromT)}`, `--trade-from=${stamp(meta.tradeFromT)}`,
    `--trade-to=${stamp(meta.tradeToT)}`, `--spread=${meta.spread}`], { encoding: 'utf8', maxBuffer: 64 * 1024 * 1024 });
}
// Diagnostic helpers: exact section fragments; never raw input/report dumps.
let byteMismatchCount = 0;
const maxDiagnosticRecords = 80;
function rawMembers(raw) {
  let i = 0;
  const skip = () => { while (/\s/.test(raw[i] ?? '') && i < raw.length) i++; };
  const stringEnd = () => {
    i++;
    while (i < raw.length) { if (raw[i] === '\\') i += 2; else if (raw[i++] === '"') return; }
    throw new Error('unterminated string');
  };
  skip(); if (raw[i++] !== '{') throw new Error('expected object');
  const members = new Map();
  for (;;) {
    skip(); if (raw[i] === '}') return members;
    const start = i; if (raw[i] !== '"') throw new Error('expected key');
    stringEnd(); const key = JSON.parse(raw.slice(start, i)); skip();
    if (raw[i++] !== ':') throw new Error('expected colon'); skip();
    const valueStart = i;
    if (raw[i] === '"') stringEnd();
    else if (raw[i] === '{' || raw[i] === '[') {
      let depth = 0;
      do {
        if (raw[i] === '"') { stringEnd(); continue; }
        if (raw[i] === '{' || raw[i] === '[') depth++;
        if (raw[i] === '}' || raw[i] === ']') depth--;
        i++;
      } while (depth && i < raw.length);
      if (depth) throw new Error('unclosed value');
    } else { while (i < raw.length && !/[,}\s]/.test(raw[i])) i++; }
    members.set(key, { value: raw.slice(valueStart, i), fragment: raw.slice(start, i) });
    skip(); if (raw[i] === '}') return members;
    if (raw[i++] !== ',') throw new Error('expected comma');
  }
}
const runSections = ['indicators', 'h4', 'signals', 'trades', 'edits', 'openPosition', 'pendingSignal', 'summary', 'monthlyByEntry'];
function sectionFragments(raw) {
  const top = rawMembers(raw), run = rawMembers(top.get('run')?.value ?? '{}');
  const selected = (members, omit) => [...members].filter(([key]) => !omit.includes(key)).map(([, value]) => value.fragment).join('\n');
  return new Map([
    ['envelope', selected(top, ['config', 'run'])], ['config', top.get('config')?.value ?? '<missing>'],
    ['run.metadata', selected(run, runSections)],
    ...runSections.map((key) => [`run.${key}`, run.get(key)?.value ?? '<missing>']),
  ]);
}
function fieldDifferences(actual, expected) {
  try {
    const a = JSON.parse(actual), b = JSON.parse(expected);
    const af = sectionFragments(actual), bf = sectionFragments(expected);
    const sectionOf = (parts) => parts[0] === 'run' ? (runSections.includes(parts[1]) ? `run.${parts[1]}` : 'run.metadata') : parts[0] === 'config' ? 'config' : 'envelope';
    const sections = new Map([...af].map(([section, value]) => [section, { section, actualSha256: digest(value), expectedSha256: digest(bf.get(section)), differingFields: 0, numericFields: 0, identityDecisionFields: 0, otherFields: 0, maxAbsoluteDelta: 0, maxRelativeDelta: 0, numericSamples: [], identityDecisionSamples: [], otherSamples: [] }]));
    const counters = new Set(['count', 'm30Count', 'usedM5Rows', 'trades', 'partialBars', 'stopWidenings', 'targetChanges', 'marketableTargetEdits', 'lockTransitions', 'lockDeactivations', 'hypotheticalLockRelaxations', 'rejectedWorseST', 'preentryOnlyActivations', 'nonprofitLockEdits', 'partialEntryFills', 'partialPositionBars']);
    const discrete = new Set(['symbol', 'mode', 'schema', 'sourceTimeframe', 'nativeTimeframe', 'executionModel', 'quantityModel', 'priceModel', 'contractVersion', 'side', 'direction', 'regime', 'candidate', 'sourceCandidate', 'protectedCandidate', 'cross', 'reason', 'forcedReason', 'entryMonth']);
    const isIdentity = (key, x, y) => /(^id$|Id$|ID$|Index$|^index$|T$|^time$)/.test(key) || discrete.has(key) || counters.has(key) || typeof x === 'boolean' || typeof y === 'boolean' || x === null || y === null || x === undefined || y === undefined;
    const describe = (value, key) => {
      if (value === undefined) return { type: 'missing' };
      if (typeof value === 'number' && Object.is(value, -0)) return { type: 'number', value: '-0' };
      if (value === null || typeof value === 'number' || typeof value === 'boolean') return value;
      if (typeof value === 'string') return /reason|Reason|^id$|Id$|ID$/.test(key) ? value.slice(0, 80) : { type: 'string', length: value.length };
      return { type: Array.isArray(value) ? 'array' : 'object' };
    };
    const walk = (x, y, parts) => {
      if (Object.is(x, y)) return;
      const objects = x !== null && y !== null && typeof x === 'object' && typeof y === 'object';
      if (objects && Array.isArray(x) === Array.isArray(y)) {
        for (const key of new Set([...Object.keys(x), ...Object.keys(y)])) walk(x[key], y[key], [...parts, key]);
        return;
      }
      const section = sections.get(sectionOf(parts)), key = parts.at(-1) ?? '$';
      const identity = isIdentity(key, x, y);
      section.differingFields++;
      if (identity) section.identityDecisionFields++;
      else if (typeof x === 'number' && typeof y === 'number') section.numericFields++;
      else section.otherFields++;
      if (!identity && typeof x === 'number' && typeof y === 'number') {
        const absolute = Math.abs(x - y), scale = Math.max(Math.abs(x), Math.abs(y), Number.MIN_VALUE);
        const relative = Math.abs(x / scale - y / scale);
        section.maxAbsoluteDelta = Math.max(section.maxAbsoluteDelta, absolute);
        section.maxRelativeDelta = Math.max(section.maxRelativeDelta, relative);
      }
      const samples = identity ? section.identityDecisionSamples : (typeof x === 'number' && typeof y === 'number' ? section.numericSamples : section.otherSamples);
      if (samples.length < 2) samples.push({ path: `$.${parts.join('.')}`.slice(0, 160), actual: describe(x, key), expected: describe(y, key) });
    };
    walk(a, b, []);
    for (const section of sections.values()) {
      const key = section.section.slice(4), x = a.run?.[key], y = b.run?.[key];
      if (Array.isArray(x) || Array.isArray(y)) { section.actualCount = Array.isArray(x) ? x.length : null; section.expectedCount = Array.isArray(y) ? y.length : null; }
      if (!Number.isFinite(section.maxAbsoluteDelta)) section.maxAbsoluteDelta = 'overflow';
    }
    const all = [...sections.values()];
    const differingFields = all.reduce((sum, value) => sum + value.differingFields, 0);
    return { differingFields, noParsedFieldDifferences: differingFields === 0, sectionHashEncoding: 'exact JSON value/member fragments joined by LF; excludes outer framing/separators', sections: all };
  } catch {
    return { diagnosticUnavailable: true, differingFields: null, sections: [] };
  }
}
function identical(actual, expected, label) {
  assert.equal(typeof actual, 'string', `${label}: expected a JSON string`);
  if (actual === expected) return true;
  byteMismatchCount++;
  if (byteMismatchCount <= maxDiagnosticRecords) {
    console.error(JSON.stringify({ schema: 'master-runtime-parity-difference-v2', case: label.slice(0, 160),
      actualSha256: digest(actual), expectedSha256: digest(expected), ...fieldDifferences(actual, expected) }));
  }
  return false; // Continue fixed synthetic cases; the final gate always fails.
}
function requireByteParity() {
  assert.equal(byteMismatchCount, 0, `${byteMismatchCount} exact byte-parity comparisons failed; no tolerance or normalization is permitted`);
}
// End diagnostic helpers.
function compare(label, rows, meta = metadata, text = source(), suppliedBytes) {
  const bytes = suppliedBytes ?? encode(rows);
  const wasm = globalThis.engineRunMasterPortableReport(JSON.stringify(meta), text, bytes);
  const result = JSON.parse(wasm);
  assert.equal(result.error, undefined, `${label}: ${result.error}`);
  const cli = native(nativeCLI, meta, text, bytes);
  identical(wasm, cli, `${label}/native-WASM`); // Including indentation and trailing newline.
  assert.equal(result.schema, 'strat-master-structural-portable-cli-v1');
  assert.equal(result.arithmetic.contract, metadata.arithmeticContract);
  assert.equal(result.arithmetic.implicitContraction, false); assert.equal(result.arithmetic.tickQuantization, false);
  assert.equal(result.run.arithmeticContract, metadata.arithmeticContract);
  assert.equal(result.run.schema, 'strat-master-structural-portable-report-v1');
  assert.equal(result.dslSha256, digest(Buffer.from(text, 'utf8')));
  assert.equal(result.dataSha256, digest(bytes));
  assert.equal(result.run.pineParityVerified, false);
  assert.equal(result.run.costComplete, false);
  evidence.comparisons.push({ label, sourceSha256: digest(text), dataSha256: digest(bytes), configSha256: result.configSha256, rows: rows.length, trades: result.run.trades.length, sha256: digest(wasm), nativeSha256: digest(cli), byteIdentical: wasm === cli });
  return { raw: wasm, envelope: result, run: result.run };
}
function reject(label, meta = JSON.stringify(metadata), text = source(), bytes = baseBytes) {
  let raw;
  assert.doesNotThrow(() => { raw = globalThis.engineRunMasterPortableReport(meta, text, bytes); }, `${label}: exception escaped the bridge`);
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
const baseBytes = assets.get(corpus.cases[0].dataSha256), base = rowsFrom(baseBytes);
const backing = new Uint8Array(baseBytes.length + 31).fill(0xa5); backing.set(baseBytes, 13);
const fixedResults = new Map();
function fixedCompare(item) {
  let bytes = assets.get(item.dataSha256);
  if (item.label === 'nonzero-byte-offset') bytes = backing.subarray(13, 13 + baseBytes.length);
  const result = compare(item.label, rowsFrom(bytes), item.metadata, item.source, bytes);
  fixedResults.set(item.label, result);
  return result;
}
try {
  for (const item of corpus.cases.filter((item) => item.label !== 'transport-after-refusals')) fixedCompare(item);
  for (const mode of modes) for (const spread of [0, 1]) {
    const label = `${mode}/spread-${spread}`;
    const baseline = fixedResults.get(label), reflected = fixedResults.get(`${label}/reflected`);
    for (const {run} of [baseline,reflected]) {
      assert(run.trades.length > 0 && run.signals.length > 0, `${label}: vacuous coverage`);
      for (const trade of run.trades) assert(trade.entryT >= trade.signal.time && trade.exitT >= trade.entryT && trade.exitT <= metadata.tradeToT);
    }
    const directions = new Set([...baseline.run.trades,...reflected.run.trades].map((trade)=>trade.signal.direction));
    assert(directions.has(1) && directions.has(-1));
    assert.deepEqual(fixedResults.get(`${label}/future-appended`).run,baseline.run);
    assert.deepEqual(fixedResults.get(`${label}/future-mutated`).run,baseline.run);
    assert.notEqual(fixedResults.get(`${label}/future-appended`).envelope.dataSha256,baseline.envelope.dataSha256);
    assert.notEqual(fixedResults.get(`${label}/future-mutated`).envelope.dataSha256,fixedResults.get(`${label}/future-appended`).envelope.dataSha256);
    const gap = fixedResults.get(`${label}/gaps-partial`).run;
    assert.equal(gap.usedM5Rows,2993); assert.equal(gap.indicators.length,499);
    assert.equal(gap.indicators[0].count,5);assert.equal(gap.indicators[0].complete,false);
    assert.equal(gap.h4[0].count,47);assert.equal(gap.h4[0].complete,false);
    assert(!gap.indicators.some((row)=>row.bucketT===200*M30));
    const partial = fixedResults.get(`${label}/partial-entry`).run;
    const firstTrade = baseline.run.trades[0];
    const position = [...partial.trades,...(partial.openPosition?[partial.openPosition]:[])].find((p)=>p.signal.time===firstTrade.signal.time);
    assert(position);assert.equal(position.entryT,firstTrade.entryBucketT+M5);
    assert(partial.summary.partialEntryFills>0 && partial.summary.partialPositionBars>0);
    const terminal = fixedResults.get(`${label}/terminal-open`).run;
    assert(terminal.openPosition);assert.equal(terminal.summary.openEntryFee,terminal.openPosition.entryFee);
    assert(!terminal.trades.some((t)=>t.entryT===terminal.openPosition.entryT));assert.equal(terminal.pendingSignal,null);
    const excludedItem = corpus.cases.find((item)=>item.label===`${label}/terminal-signal-excluded`);
    const excluded = fixedResults.get(excludedItem.label).run;
    assert(!excluded.signals.some((signal)=>signal.time===excludedItem.metadata.tradeToT));assert.equal(excluded.pendingSignal,null);
  }
  for (const mode of modes) {
    const run = fixedResults.get(`${mode}/H4-boundary`).run;
    assert.equal(run.h4.length,12);assert.equal(run.indicators[6].historicalH4,null);
    assert.equal(run.indicators[7].historicalH4.availableT,H4);assert.equal(run.indicators[7].stableH4,null);
    assert.equal(run.indicators[8].stableH4.availableT,H4);
    assert.equal(run.indicators[87].historicalH4.regime,-1);assert.equal(run.indicators[87].stableH4.regime,1);
    assert.equal(run.indicators[87].historicalH4.availableT,11*H4);assert.equal(run.indicators[87].stableH4.availableT,10*H4);
    assert.equal(run.indicators[88].stableH4.regime,-1);
  }
  const paddedSource = corpus.cases.find((item)=>item.label==='source-exact-byte-limit').source;
  for (const token of ['-0','0.000','1.000']) {
    const rawMetadata = JSON.stringify(metadata).replace('"spread":0', `"spread":${token}`);
    const wasm = globalThis.engineRunMasterPortableReport(rawMetadata, source(), baseBytes);
    const cli = native(nativeCLI, { ...metadata, spread: token }, source(), baseBytes);
    identical(wasm, cli, `plain-decimal-spread-${token}`);
    assert.equal(JSON.parse(wasm).error, undefined);
    if (token === '-0') assert(wasm.includes('"spread": -0'), 'signed zero was normalized');
  }
  evidence.plainDecimalControls = 3;
  const paddedMeta = JSON.stringify(metadata).padEnd(4096, ' ');
  identical(globalThis.engineRunMasterPortableReport(paddedMeta, source(), baseBytes), native(nativeCLI, metadata, source(), baseBytes), 'metadata-exact-byte-limit');
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
  reject('legacy-schema', JSON.stringify({ ...metadata, schema: 'master-structural-runtime-request-v1' }));
  reject('wrong-arithmetic', JSON.stringify({ ...metadata, arithmeticContract: 'future-v2' }));
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
  const recovered = fixedCompare(corpus.cases.find((item)=>item.label==='transport-after-refusals'));
  const shadowed = backing.subarray(13, 13 + baseBytes.length);
  for (const key of ['byteLength', 'byteOffset', 'buffer']) {
    Object.defineProperty(shadowed, key, { get() { throw new Error(`shadowed ${key} getter must not execute`); } });
  }
  identical(globalThis.engineRunMasterPortableReport(JSON.stringify(metadata), source(), shadowed), recovered.raw, 'intrinsic-accessors-ignore-shadowed-fields');
  evidence.shadowedAccessorsVerified = true;
  for (const [meta, text] of [[metadata, source()], [null, source()], [JSON.stringify(metadata), null], [JSON.stringify(metadata), new String(source())]]) reject('wrong-string-type', meta, text);
  for (const args of [[], [JSON.stringify(metadata), source()], [JSON.stringify(metadata), source(), baseBytes, 'extra']]) {
    const result = JSON.parse(globalThis.engineRunMasterPortableReport(...args));
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
  console.log(JSON.stringify({ ...evidence, byteMismatchCount, diagnosticsTruncated: byteMismatchCount > maxDiagnosticRecords, status: byteMismatchCount ? 'FAIL' : 'PASS' }, null, 2));
  requireByteParity();
} finally {
  rmSync(scratch, { recursive: true, force: true });
}
process.exit(0);
