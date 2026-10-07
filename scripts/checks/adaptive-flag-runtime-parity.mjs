// Exact full-report parity over frozen, invented Stage A transport bytes.
// Usage: node scripts/checks/adaptive-flag-runtime-parity.mjs \
//   <native CLI> <enginewasm.wasm> <wasm_exec.js> <matching Go tool>
// No source/data generation, host transcendental functions, tolerance, report
// normalization, or selection of windows based on observed output occurs here.
import assert from 'node:assert/strict';
import { createHash, webcrypto } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { tmpdir, platform, arch, release } from 'node:os';
import { join, resolve } from 'node:path';

const [nativePath, wasmPath, shimPath, goPath, ...extra] = process.argv.slice(2);
assert(nativePath && wasmPath && shimPath && goPath && !extra.length, 'exactly four artifact/tool paths are required');
const hash = (bytes) => createHash('sha256').update(bytes).digest('hex');
const corpusRaw = readFileSync(new URL('../../testsupport/testdata/adaptive-flag-runtime-corpus-v1.json', import.meta.url));
const corpusSha256 = hash(corpusRaw);
assert.equal(corpusSha256, 'af392754f48672cfe31d7161454dfc35ee73977e4c38f0a72b21c15be6fb2f97', 'frozen corpus v1 changed; changes require a new version and rationale');
const corpus = JSON.parse(corpusRaw);
assert.equal(corpus.schema, 'adaptive-flag-runtime-invented-corpus-v1');
assert.equal(corpus.version, 1);
assert.equal(corpus.cases.length, 96);
assert.equal(Object.keys(corpus.sources).length, 6);
assert.equal(Object.keys(corpus.assets).length, 22);
const assets = new Map();
for (const [key, asset] of Object.entries(corpus.assets)) {
  assert.equal(asset.encoding, 'base64-btb1');
  const bytes = new Uint8Array(Buffer.from(asset.payload, 'base64'));
  assert.equal(hash(bytes), key); assert.equal(asset.sha256, key);
  assert.equal(bytes.length, asset.byteLength); assert.equal(asset.byteLength, 16 + asset.rows * 48);
  assert(asset.rows >= 1 && asset.rows <= 16384 && asset.byteLength <= 786448);
  assets.set(key, bytes);
}
for (const [key, item] of Object.entries(corpus.sources)) {
  assert.equal(hash(item.text), key); assert.equal(item.sha256, key);
  assert.equal(Buffer.byteLength(item.text), item.utf8Bytes); assert(item.utf8Bytes <= 4096);
  assert(['INITIAL', 'TWEAKED', 'SNAPSHOT_C'].includes(item.bundle));
  assert(['M30', 'H1'].includes(item.timeframe));
  assert.match(item.configSha256, /^[0-9a-f]{64}$/);
}
assert.equal(new Set(corpus.cases.map((item) => item.id)).size, corpus.cases.length);
for (const item of corpus.cases) {
  assert.equal(hash(item.metadataJSON), item.metadataSha256);
  assert(corpus.sources[item.sourceSha256] && assets.has(item.btb1Sha256));
}

function buildInfo(path, wasmArtifact = false) {
  const bytes = readFileSync(path);
  let raw;
  if (wasmArtifact) {
    // Go 1.22's go version -m reader does not recognize WASM. Read the actual
    // linked module-info sentinels and NUL-delimited version in this artifact;
    // never borrow native build stamps or infer them from the requested build.
    assert(bytes.subarray(0, 8).equals(Buffer.from([0, 97, 115, 109, 1, 0, 0, 0])));
    const begin = Buffer.from('3077af0c9274080241e1c107e6d618e6', 'hex');
    const end = Buffer.from('f932433186182072008242104116d8f2', 'hex');
    const start = bytes.indexOf(begin), finish = bytes.indexOf(end, start + begin.length);
    assert(start >= 0 && finish > start && finish - start < 16384, 'missing bounded WASM module info');
    assert.equal(bytes.indexOf(begin, start + 1), -1, 'ambiguous WASM module info');
    const versions = [...new Set([...bytes.toString('latin1').matchAll(/\x00(go1\.\d+\.\d+)\x00/g)].map((match) => match[1]))];
    assert.equal(versions.length, 1, 'missing/ambiguous linked WASM Go version');
    raw = `${path}: ${versions[0]}\n` + bytes.subarray(start + begin.length, finish).toString('utf8');
  } else {
    raw = execFileSync(resolve(goPath), ['version', '-m', resolve(path)], { encoding: 'utf8', maxBuffer: 1024 * 1024 });
  }
  const lines = raw.trimEnd().split('\n');
  const goVersion = lines[0].slice(lines[0].lastIndexOf(': ') + 2);
  const settings = {};
  for (const line of lines.slice(1)) {
    const fields = line.trim().split('\t');
    if (fields[0] === 'build') {
      const at = fields[1].indexOf('=');
      settings[fields[1].slice(0, at)] = fields[1].slice(at + 1);
    }
  }
  return { goVersion, settings, sha256: hash(bytes), buildInfoSource: wasmArtifact ? 'linked-module-sentinels-and-NUL-delimited-version' : 'go-version-m' };
}
const native = buildInfo(nativePath), wasm = buildInfo(wasmPath, true);
assert.equal(native.goVersion, 'go1.22.12');
assert.equal(wasm.goVersion, native.goVersion);
assert.equal(native.settings.GOOS, platform());
assert.equal(native.settings.GOARCH, ({ x64: 'amd64', arm64: 'arm64' })[arch()], 'native artifact architecture must match actual executing host');
assert.equal(wasm.settings.GOOS, 'js'); assert.equal(wasm.settings.GOARCH, 'wasm');
assert.equal(native.settings.CGO_ENABLED, '0'); assert.equal(wasm.settings.CGO_ENABLED, '0');
assert.equal(native.settings['-compiler'], 'gc'); assert.equal(wasm.settings['-compiler'], 'gc');
assert.equal(native.settings['vcs.revision'], wasm.settings['vcs.revision']);
assert.equal(native.settings['vcs.modified'], wasm.settings['vcs.modified']);
const evidence = {
  schema: 'adaptive-flag-runtime-exact-parity-v1', corpusSha256,
  host: { platform: platform(), architecture: arch(), release: release(), node: process.version, v8: process.versions.v8 },
  artifacts: { native, wasm, wasmExecSha256: hash(readFileSync(shimPath)) },
  qualification: 'Actual same-host native/Node-WASM execution of these frozen invented bytes only. No browser, other-architecture, market, accounting or general Pine parity claim.',
  reports: [], designatedLifecycles: 0, suffixInvariancePairs: 0,
};
globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(resolve(shimPath));
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject);
void go.run(instance);
assert.equal(typeof globalThis.engineRunAdaptiveFlagReport, 'function');

// Extract exact JSON fragments solely to check the report's own compact hashes.
// Native/WASM report comparison below is complete byte equality performed first;
// these helpers never normalize or excuse a portability mismatch.
function valueEnd(text, at) {
  if (text[at] === '"') {
    for (let i = at + 1; i < text.length; i++) {
      if (text[i] === '\\') i++;
      else if (text[i] === '"') return i + 1;
    }
  } else if (text[at] === '{' || text[at] === '[') {
    let depth = 0;
    for (let i = at; i < text.length; i++) {
      if (text[i] === '"') i = valueEnd(text, i) - 1;
      else if (text[i] === '{' || text[i] === '[') depth++;
      else if ((text[i] === '}' || text[i] === ']') && --depth === 0) return i + 1;
    }
  } else {
    let i = at;
    while (i < text.length && !/[\s,}\]]/.test(text[i])) i++;
    return i;
  }
  throw new Error('invalid serialized JSON fragment');
}
function members(text) {
  const values = new Map(); let at = 0;
  const whitespace = () => { while (/\s/.test(text[at] ?? '') && at < text.length) at++; };
  whitespace(); assert.equal(text[at++], '{');
  for (;;) {
    whitespace(); if (text[at] === '}') return values;
    const end = valueEnd(text, at), key = JSON.parse(text.slice(at, end)); at = end;
    whitespace(); assert.equal(text[at++], ':'); whitespace();
    const start = at; at = valueEnd(text, at); values.set(key, text.slice(start, at));
    whitespace(); if (text[at] === '}') return values;
    assert.equal(text[at++], ',');
  }
}
function elements(text) {
  const values = []; let at = 1;
  for (;;) {
    while (/\s/.test(text[at] ?? '') && at < text.length) at++;
    if (text[at] === ']') return values;
    const end = valueEnd(text, at); values.push(text.slice(at, end)); at = end;
    while (/\s/.test(text[at] ?? '') && at < text.length) at++;
    if (text[at] === ']') return values;
    assert.equal(text[at++], ',');
  }
}
function compactFragment(text) {
  let result = '';
  for (let at = 0; at < text.length;) {
    if (text[at] === '"') { const end = valueEnd(text, at); result += text.slice(at, end); at = end; }
    else { if (!/\s/.test(text[at])) result += text[at]; at++; }
  }
  return result;
}
function subset(actual, expected, label) {
  assert(actual !== null && actual !== undefined, `${label}: missing object`);
  for (const [key, value] of Object.entries(expected)) assert.deepEqual(actual[key], value, `${label}.${key}`);
}
function checkReport(raw, item) {
  const document = JSON.parse(raw), frozenSource = corpus.sources[item.sourceSha256];
  assert.deepEqual(Object.keys(document), ['schema', 'request', 'requestSha256', 'dslSha256', 'configSha256', 'btb1Sha256', 'rawOnly', 'economics', 'config', 'run']);
  assert.equal(document.schema, 'strat-adaptive-volume-flag-runtime-v1');
  assert.deepEqual(document.request, JSON.parse(item.metadataJSON));
  assert.equal(document.requestSha256, item.metadataSha256); assert.equal(document.dslSha256, item.sourceSha256);
  assert.equal(document.btb1Sha256, item.btb1Sha256); assert.equal(document.configSha256, frozenSource.configSha256);
  assert.equal(document.rawOnly, true); assert.equal(document.economics, 'unavailable-stage-a');
  assert.equal(document.config.adaptiveVolumeFlag.bundle, item.bundle);
  const result = document.run;
  assert.equal(result.schema, 'strat-adaptive-volume-flag-reference-v1');
  assert.equal(result.policy, 'DELAYED_OHLC_REFERENCE_V1');
  assert.equal(result.numericalPolicy, 'BINARY64_ORDERED_V1');
  assert.equal(result.executionSemantics, 'delayed-first-stop-activation-v1');
  assert.equal(result.arithmeticQualification, 'explicit-float64-rounding-barriers; cross-architecture-not-qualified');
  assert.equal(result.sourcePineSha256, '3102bc71b810eb5559479f6910994d93d02899d90b32f17e1c71a02aa60a4819');
  assert.equal(result.referenceSha256, '0d99e7162984a28a0f5d63138b6bfba453d8cc6f659edc3aba6bde3cf9a4bcec');
  assert.equal(result.effectiveConfig.bundle, item.bundle); assert.equal(result.effectiveConfig.timeframe, item.timeframe);
  for (const key of ['researchPolicy', 'researchPolicySha256', 'researchProducer']) assert(!Object.hasOwn(result, key));
  assert.equal(result.assumptions.length, 13);
  const top = members(raw), run = members(top.get('run'));
  assert.equal(hash(compactFragment(top.get('config'))), document.configSha256, `${item.id}: exact compact root config hash`);
  assert.equal(hash(compactFragment(run.get('effectiveConfig'))), result.configSha256, `${item.id}: exact compact effective config hash`);
  const retainedRows = elements(run.get('snapshots')).map((snapshot) => {
    const row = members(snapshot);
    return '[' + ['openT', 'open', 'high', 'low', 'close', 'volume'].map((key) => row.get(key)).join(',') + ']';
  });
  assert.equal(hash('[' + retainedRows.join(',') + ']'), result.inputSha256, `${item.id}: exact ordered retained-row hash`);
  const expected = item.expect;
  assert.equal(result.usedSourceRows, expected.usedRows); assert.equal(result.providedSourceRows, expected.providedRows);
  assert.equal(result.ignoredSuffixRows, expected.providedRows - expected.usedRows);
  assert.equal(result.preTradeRows, expected.preTradeRows); assert.equal(result.eligibleTradeRows, expected.eligibleTradeRows);
  assert.equal(result.snapshots.length, expected.usedRows); assert.equal(result.states.length, expected.usedRows);
  assert.equal(result.timeframeMs, item.timeframe === 'M30' ? 1800000 : 3600000);
  assert.equal(result.lastRetainedCloseMs, result.lastRetainedOpenMs + result.timeframeMs);
  assert(result.orders.length <= expected.usedRows && result.events.length <= 4 * expected.usedRows);
  assert(result.gaps.length < expected.usedRows);
  assert(result.orders.every((order) => order.eventIds.length <= 26));
  if (expected.terminal) assert.equal(result.terminal.status, expected.terminal, `${item.id}: terminal`);
  if (expected.firstOrder) subset(result.orders[0], expected.firstOrder, `${item.id}: first order`);
  if (expected.secondOrder) subset(result.orders[1], expected.secondOrder, `${item.id}: second order`);
  if (expected.queuedExit) subset(result.terminal.queuedExit, expected.queuedExit, `${item.id}: queued exit`);
  if (expected.exactOrders !== undefined) assert.equal(result.orders.length, expected.exactOrders);
  if (expected.minOrders !== undefined) assert(result.orders.length >= expected.minOrders, `${item.id}: vacuous orders`);
  if (expected.minEvents !== undefined) assert(result.events.length >= expected.minEvents, `${item.id}: vacuous events`);
  if (expected.minGaps !== undefined) assert(result.gaps.length >= expected.minGaps, `${item.id}: vacuous gaps`);
  if (expected.designatedFullLifecycle) {
    assert(result.orders.some((order) => order.side === expected.designatedFullLifecycle && Number.isInteger(order.fillIdx) && Number.isInteger(order.exitIdx) && order.fillIdx > order.signalIdx && order.exitIdx > order.fillIdx && order.status === 'closed'), `${item.id}: missing designated ${expected.designatedFullLifecycle} fill and exit`);
    evidence.designatedLifecycles++;
  }
  for (const order of result.orders) {
    if (order.fillIdx !== null) assert(order.fillIdx > order.signalIdx && order.fillIdx < expected.usedRows);
    if (order.exitIdx !== null) assert(order.exitIdx > order.fillIdx && order.exitIdx < expected.usedRows);
    if (document.request.executionWindow) assert(result.snapshots[order.signalIdx].openT >= document.request.executionWindow.tradeFromMs);
  }
  for (const state of result.states.slice(0, expected.preTradeRows)) assert.equal(state.status, 'flat', `${item.id}: pre-trade order leakage`);
  assert(raw.endsWith('\n') && !raw.endsWith('\n\n'));
  return { result, configSha256: document.configSha256, rawRunHash: hash(top.get('run')) };
}
const scratch = mkdtempSync(join(tmpdir(), 'adaptive-runtime-parity-'));
const requestPath = join(scratch, 'request.json'), sourcePath = join(scratch, 'source.strat'), barsPath = join(scratch, 'bars.btb1');
const previous = new Map();
try {
  for (const item of corpus.cases) {
    const source = corpus.sources[item.sourceSha256].text, bytes = assets.get(item.btb1Sha256);
    writeFileSync(requestPath, item.metadataJSON); writeFileSync(sourcePath, source); writeFileSync(barsPath, bytes);
    const nativeRaw = execFileSync(resolve(nativePath), ['adaptive-flag-runtime-report', `--request-file=${requestPath}`, `--dsl-file=${sourcePath}`, `--bars-file=${barsPath}`], { maxBuffer: 128 * 1024 * 1024 });
    const wasmText = globalThis.engineRunAdaptiveFlagReport(item.metadataJSON, source, bytes);
    assert.equal(typeof wasmText, 'string', `${item.id}: invalid WASM return type`);
    const wasmRaw = Buffer.from(wasmText, 'utf8');
    const nativeSha256 = hash(nativeRaw), wasmSha256 = hash(wasmRaw);
    assert(nativeRaw.equals(wasmRaw), `${item.id}: complete UTF-8 byte mismatch; native=${nativeSha256}; wasm=${wasmSha256}`);
    const { result, configSha256, rawRunHash } = checkReport(wasmText, item);
    if (item.sameRunAs) {
      const earlier = previous.get(item.sameRunAs); assert(earlier, 'missing named suffix control');
      assert.equal(rawRunHash, earlier.rawRunHash, `${item.id}: retained run changed with ignored suffix`);
      assert.notEqual(item.btb1Sha256, earlier.btb1Sha256);
      evidence.suffixInvariancePairs++;
    }
    previous.set(item.id, { rawRunHash, btb1Sha256: item.btb1Sha256 });
    evidence.reports.push({ id: item.id, requestSha256: item.metadataSha256, sourceSha256: item.sourceSha256, btb1Sha256: item.btb1Sha256, configSha256, effectiveConfigSha256: result.configSha256, retainedRowsSha256: result.inputSha256, nativeSha256, wasmSha256, rawRunSha256: rawRunHash, bytes: nativeRaw.length, snapshots: result.snapshots.length, orders: result.orders.length, filled: result.orders.filter((order) => order.fillIdx !== null).length, closed: result.orders.filter((order) => order.exitIdx !== null).length, events: result.events.length, gaps: result.gaps.length, terminal: result.terminal.status, byteIdentical: true });
  }
  assert.equal(evidence.designatedLifecycles, 12); assert.equal(evidence.suffixInvariancePairs, 6);
  assert.equal(evidence.reports.length, 96);
  evidence.status = 'PASS';
  console.log(JSON.stringify(evidence));
} finally {
  rmSync(scratch, { recursive: true, force: true });
}
process.exit(0);
