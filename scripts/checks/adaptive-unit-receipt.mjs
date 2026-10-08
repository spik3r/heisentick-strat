// Fail-closed validation of retained qualification evidence; no engine or oracle
// is executed here. The public harness's seven build leaves remain the only
// byte-comparison exception. Stage A's receipt helper is deliberately unchanged.
// Usage: node adaptive-unit-receipt.mjs <parity.json> <transport.json>
//   <public-reports-dir> <native> <wasm> <wasm_exec.js> <go>
//   <checkout-revision> <execution-target, e.g. linux-arm64>
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { lstatSync, readFileSync, readdirSync } from 'node:fs';
import { arch, platform, release } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { emitAdaptiveReceipt } from './adaptive-runtime-receipt.mjs';
import { EVIDENCE_IDENTITY } from './adaptive-flag-unit-evidence.mjs';

const ROOT = fileURLToPath(new URL('../../', import.meta.url));
const MAX_RECEIPT = 2 * 1024 * 1024;
const MAX_REPORT = 64 * 1024 * 1024;
const SHA = /^[a-f0-9]{64}$/;
const REVISION = /^[a-f0-9]{40}$/;
const IDENTITY_KEYS = ['goVersion', 'compiler', 'goos', 'goarch', 'vcsRevision', 'vcsModified', 'verified'];
const digest = bytes => createHash('sha256').update(bytes).digest('hex');
function objectKeys(value, keys, context) {
  assert(value !== null && typeof value === 'object' && !Array.isArray(value), `${context}: expected object`);
  assert.deepEqual(Object.keys(value).sort(), [...keys].sort(), `${context}: unknown or missing fields`);
}
function sha(value, context) { assert(typeof value === 'string' && SHA.test(value), `${context}: invalid SHA256`); }
function integer(value, minimum, maximum, context) {
  assert(Number.isSafeInteger(value) && !Object.is(value, -0) && value >= minimum && value <= maximum, `${context}: invalid integer`);
}
function scalarString(value) {
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(++i);
      assert(next >= 0xdc00 && next <= 0xdfff, 'unpaired high surrogate');
    } else assert(code < 0xdc00 || code > 0xdfff, 'unpaired low surrogate');
  }
  return value;
}

// JSON.parse alone loses duplicate decoded keys and replaces malformed UTF-8.
// Scan the original bytes first, with bounded depth and token count. Retained
// spans let report verification replace identity tokens without reserializing
// any economic values, signed zero, ordering or whitespace.
export function parseStrictJSON(raw, capture = new Set(), { integerTokensOnly = false } = {}) {
  assert(Buffer.isBuffer(raw), 'JSON input must be bytes');
  const text = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(raw);
  let at = 0, tokens = 0;
  const spans = new Map();
  const ws = () => { while (/[\x20\t\r\n]/.test(text[at] ?? '') && at < text.length) at++; };
  function string() {
    assert.equal(text[at], '"', 'expected JSON string');
    const start = at++;
    while (at < text.length) {
      const c = text[at++];
      if (c === '"') return scalarString(JSON.parse(text.slice(start, at)));
      if (c === '\\') { assert(at < text.length, 'truncated JSON escape'); at++; }
    }
    throw new Error('unterminated JSON string');
  }
  function value(path, depth) {
    assert(depth <= 64 && ++tokens <= 2_000_000, 'JSON complexity limit');
    ws(); const start = at;
    let result;
    if (text[at] === '{') {
      at++; ws(); result = {}; const keys = new Set();
      if (text[at] !== '}') for (;;) {
        ws(); const key = string();
        assert(!keys.has(key), `duplicate decoded key: ${key}`); keys.add(key);
        ws(); assert.equal(text[at++], ':', 'expected colon');
        Object.defineProperty(result, key, { value: value([...path, key], depth + 1), enumerable: true, writable: true, configurable: true });
        ws(); if (text[at] === '}') break;
        assert.equal(text[at++], ',', 'expected comma');
      }
      assert.equal(text[at++], '}', 'expected object end');
    } else if (text[at] === '[') {
      at++; ws(); result = [];
      if (text[at] !== ']') for (;;) {
        result.push(value([...path, String(result.length)], depth + 1));
        ws(); if (text[at] === ']') break;
        assert.equal(text[at++], ',', 'expected comma');
      }
      assert.equal(text[at++], ']', 'expected array end');
    } else if (text[at] === '"') result = string();
    else {
      const token = /^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/.exec(text.slice(at));
      assert(token, 'invalid JSON token');
      const numeric = /^[-0-9]/.test(token[0]);
      // Operational receipts contain only nonnegative integer counts/lengths.
      // Check their original lexemes BEFORE conversion can round a fraction or
      // underflow an exponent to an otherwise valid integer. Economic reports
      // use the default parser and retain all original floating-point tokens.
      if (integerTokensOnly && numeric) assert(/^(?:0|[1-9]\d*)$/.test(token[0]), 'receipt number requires a canonical nonnegative integer token');
      at += token[0].length; result = JSON.parse(token[0]);
      if (typeof result === 'number') {
        assert(Number.isFinite(result), 'nonfinite JSON number');
        if (integerTokensOnly) assert(Number.isSafeInteger(result), 'receipt integer exceeds safe range');
      }
    }
    const end = at; ws();
    const key = '/' + path.join('/');
    if (capture.has(key)) spans.set(key, { start, end, delimiter: at });
    return result;
  }
  const result = value([], 0); ws(); assert.equal(at, text.length, 'trailing JSON content');
  return { value: result, text, spans };
}
function fileBytes(path, limit, newline = false) {
  const stat = lstatSync(path);
  assert(stat.isFile() && stat.size > 0 && stat.size <= limit, `not a bounded regular file: ${path}`);
  const raw = readFileSync(path);
  assert.equal(raw.length, stat.size, 'file size changed during read');
  if (newline) assert.equal(raw.at(-1), 10, 'missing final newline');
  return raw;
}
export function readUnitReceipt(path) {
  const raw = fileBytes(path, MAX_RECEIPT, true);
  return { ...parseStrictJSON(raw, new Set(), { integerTokensOnly: true }), raw, sha256: digest(raw) };
}
function pinnedFixture(name, expected) {
  const file = readUnitReceipt(new URL(`../../testsupport/testdata/adaptive-unit-receipts/${name}`, import.meta.url));
  assert.equal(file.sha256, expected, 'approved B2 receipt fixture changed');
  return file.value;
}
const P = pinnedFixture('b2-parity.json', 'b981ed430fcad8318a444d06fb5eb7684956e2b14de115c248672c93cbf3e91a');
const T = pinnedFixture('b2-transport.json', 'ed1fd53bad0af48dbbb2800f9288b7281ad3683f0d4f69726d7d3ed351f1c968');
const rawCorpus = fileBytes(new URL('../../testsupport/testdata/adaptive-flag-runtime-corpus-v1.json', import.meta.url), MAX_REPORT);
const unitCorpus = fileBytes(new URL('../../testsupport/testdata/adaptive-flag-unit-corpus-v1.json', import.meta.url), MAX_REPORT);
assert.equal(digest(rawCorpus), P.rawCorpusSha256, 'frozen raw corpus changed');
assert.equal(digest(unitCorpus), P.unitCorpusSha256, 'frozen unit corpus changed');
const units = parseStrictJSON(unitCorpus).value;
assert.equal(units.cases.length, 288);

function identity(artifact) {
  return { goVersion: artifact.goVersion, compiler: artifact.settings['-compiler'], goos: artifact.settings.GOOS,
    goarch: artifact.settings.GOARCH, vcsRevision: artifact.settings['vcs.revision'],
    vcsModified: artifact.settings['vcs.modified'] === 'true', verified: true };
}
function identityDelta(current, previous) {
  return Buffer.byteLength(JSON.stringify(current)) - Buffer.byteLength(JSON.stringify(previous));
}
function checkArtifact(artifact, kind, observed) {
  objectKeys(artifact, Object.keys(P.artifacts[kind]), `${kind} artifact`);
  const settings = { '-buildmode': 'exe', '-compiler': 'gc', '-trimpath': 'true', CGO_ENABLED: '0',
    GOARCH: kind === 'wasm' ? 'wasm' : { x64: 'amd64', arm64: 'arm64' }[observed.host.architecture],
    GOOS: kind === 'wasm' ? 'js' : observed.host.platform };
  if (settings.GOARCH === 'amd64') settings.GOAMD64 = 'v1';
  Object.assign(settings, { vcs: 'git', 'vcs.revision': observed.revision, 'vcs.time': observed.vcsTime, 'vcs.modified': String(observed.vcsModified) });
  assert.equal(artifact.goVersion, 'go1.22.12', 'normal pinned Go toolchain required');
  assert.deepEqual(artifact.settings, settings, `${kind}: unexpected or untruthful build settings`);
  assert.equal(artifact.buildInfoSource, P.artifacts[kind].buildInfoSource);
  sha(artifact.sha256, `${kind} artifact`);
  assert.deepEqual(artifact, observed[kind], `${kind} differs from actual artifact`);
}

// The observed argument is supplied by actual files, git and this Node host in
// the public entry point. Keeping schema checks pure also allows exhaustive
// mutation tests without rebuilding production or running the Python oracle.
export function validateUnitReceiptDocuments(p, t, observed) {
  objectKeys(p, [...Object.keys(P), 'evidenceBundle'], 'parity receipt');
  objectKeys(t, [...Object.keys(T), 'recoveries', 'genericResults', 'finalRecovery'], 'transport receipt');
  assert.equal(p.schema, 'adaptive-flag-unit-public-parity-v2');
  for (const key of ['rawCorpusSha256', 'unitCorpusSha256', 'retainedCoreIdentity', 'retainedEvidence',
    'identityLeafAllowlist', 'qualification', 'sourceCases', 'policies', 'oracleCalls', 'status']) {
    assert.deepEqual(p[key], P[key], `parity ${key} changed`);
  }
  assert.deepEqual(p.evidenceBundle, EVIDENCE_IDENTITY, 'qualification bundle identity changed');
  objectKeys(p.host, Object.keys(P.host), 'host');
  assert.deepEqual(p.host, observed.host, 'receipt was not produced on this Node host');
  assert(['linux', 'darwin'].includes(p.host.platform));
  assert(['x64', 'arm64'].includes(p.host.architecture));
  const target = `${p.host.platform}-${{ x64: 'amd64', arm64: 'arm64' }[p.host.architecture]}`;
  assert.equal(observed.executionTarget, target, 'execution target differs from actual host');
  assert(REVISION.test(observed.revision), 'invalid checkout revision');
  assert.equal(typeof observed.vcsModified, 'boolean');
  objectKeys(p.artifacts, ['native', 'wasm', 'wasmExecSha256'], 'artifacts');
  checkArtifact(p.artifacts.native, 'native', observed); checkArtifact(p.artifacts.wasm, 'wasm', observed);
  assert.equal(p.artifacts.wasmExecSha256, P.artifacts.wasmExecSha256, 'matching Go shim required');
  assert.equal(p.artifacts.wasmExecSha256, observed.wasmExecSha256, 'shim differs from actual file');
  for (const kind of ['native', 'wasm']) {
    objectKeys(p[`${kind}Identity`], IDENTITY_KEYS, `${kind} identity`);
    assert.deepEqual(p[`${kind}Identity`], identity(p.artifacts[kind]), 'identity does not match linked build');
  }
  assert(Array.isArray(p.reports)); assert.equal(p.reports.length, 288, 'incomplete report corpus');
  assert.equal(new Set(p.reports.map(row => row.id)).size, 288, 'duplicate report case');
  for (let i = 0; i < 288; i++) {
    const row = p.reports[i], expected = P.reports[i], unit = units.cases[i];
    objectKeys(row, Object.keys(expected), `report ${i}`);
    for (const key of Object.keys(expected)) {
      if (!['nativeSha256', 'wasmSha256', 'nativeBytes', 'wasmBytes'].includes(key)) assert.deepEqual(row[key], expected[key], `report ${i}: ${key} differs`);
    }
    assert.equal(row.id, unit.id); assert.equal(row.rawCaseId, unit.rawCaseId);
    assert.equal(row.projectionRequestSha256, unit.projectionMetadataSha256);
    for (const kind of ['native', 'wasm']) {
      sha(row[`${kind}Sha256`], `report ${i} ${kind}`);
      integer(row[`${kind}Bytes`], 1, MAX_REPORT, `report ${i} ${kind}`);
      assert.equal(row[`${kind}Bytes`], expected[`${kind}Bytes`] + identityDelta(p[`${kind}Identity`], P[`${kind}Identity`]), `report ${i}: unexpected length`);
      if (JSON.stringify(p[`${kind}Identity`]) === JSON.stringify(P[`${kind}Identity`])) assert.equal(row[`${kind}Sha256`], expected[`${kind}Sha256`], 'unchanged identity changed report bytes');
    }
  }
  assert.equal(t.schema, 'adaptive-unit-wasm-transport-check-v2'); assert.equal(t.status, 'PASS');
  assert.equal(t.node, p.host.node); assert.equal(t.wasmSha256, p.artifacts.wasm.sha256);
  assert(Array.isArray(t.accepted)); assert.equal(t.accepted.length, 27, 'incomplete accepted controls');
  assert(Array.isArray(t.rejected)); assert.equal(t.rejected.length, 168, 'incomplete refusal controls');
  // Parameterized labels repeat intentionally. Compare the entire ordered
  // descriptor list, including phase, code and exact read count, never counts
  // or label uniqueness alone.
  assert.deepEqual(t.rejected, T.rejected, 'ordered refusal controls changed');
  assert.equal(t.recoveryCalls, 168); assert.equal(t.genericRefusals, 2);
  const firstHash = new Map(), currentHash = new Map();
  for (let i = 0; i < 27; i++) {
    const row = t.accepted[i], expected = T.accepted[i];
    objectKeys(row, Object.keys(expected), `accepted ${i}`);
    assert.equal(row.label, expected.label, `accepted ${i}: ordered control changed`);
    sha(row.reportSha256, `accepted ${i}`);
    if (Object.hasOwn(expected, 'bytes')) {
      integer(row.bytes, 1, MAX_REPORT, `accepted ${i}`);
      assert.equal(row.bytes, expected.bytes + identityDelta(p.wasmIdentity, P.wasmIdentity), `accepted ${i}: unexpected length`);
    }
    if (firstHash.has(expected.reportSha256)) assert.equal(row.reportSha256, firstHash.get(expected.reportSha256), 'equivalent control output differs');
    else { assert(!currentHash.has(row.reportSha256), 'distinct accepted outputs collapsed'); firstHash.set(expected.reportSha256, row.reportSha256); currentHash.set(row.reportSha256, true); }
    if (JSON.stringify(p.wasmIdentity) === JSON.stringify(P.wasmIdentity)) assert.equal(row.reportSha256, expected.reportSha256, 'unchanged identity changed accepted output');
  }
  assert(Array.isArray(t.recoveries)); assert.equal(t.recoveries.length, 168, 'incomplete recovery results');
  const baseline = { reportSha256: t.accepted[0].reportSha256, bytes: t.accepted[0].bytes };
  for (let i = 0; i < 168; i++) assert.deepEqual(t.recoveries[i], { rejectionIndex: i, label: T.rejected[i].label, ...baseline }, `recovery ${i} missing, duplicated or changed`);
  const refusal = 'adaptive-volume-flag-native-dedicated-runner-required';
  assert.deepEqual(t.genericResults, [
    { label: 'engineRunFixture', result: JSON.stringify({ error: refusal }) },
    { label: 'engineRunColumns', result: { ok: false, error: refusal } },
  ], 'generic refusal result missing or changed');
  assert.deepEqual(t.finalRecovery, baseline, 'generic refusal poisoned final recovery');
  return { executionTarget: target, reports: 288, accepted: 27, rejected: 168, recoveries: 168, genericRefusals: 2 };
}

export function readUnitArtifact(path, goPath, wasm = false) {
  const bytes = fileBytes(path, MAX_REPORT);
  let raw;
  if (wasm) {
    assert(bytes.subarray(0, 8).equals(Buffer.from([0, 97, 115, 109, 1, 0, 0, 0])), 'invalid WASM header');
    const begin = Buffer.from('3077af0c9274080241e1c107e6d618e6', 'hex'), end = Buffer.from('f932433186182072008242104116d8f2', 'hex');
    const start = bytes.indexOf(begin), finish = bytes.indexOf(end, start + begin.length);
    assert(start >= 0 && finish > start && finish - start < 16384, 'missing bounded WASM module info');
    assert.equal(bytes.indexOf(begin, start + 1), -1, 'ambiguous WASM module info');
    assert.equal(bytes.indexOf(end, finish + 1), -1, 'ambiguous WASM module end');
    const versions = [...new Set([...bytes.toString('latin1').matchAll(/\x00(go1\.\d+\.\d+)\x00/g)].map(match => match[1]))];
    assert.equal(versions.length, 1, 'missing or ambiguous linked WASM Go version');
    raw = `artifact: ${versions[0]}\n` + new TextDecoder('utf-8', { fatal: true }).decode(bytes.subarray(start + begin.length, finish));
  } else raw = execFileSync(resolve(goPath), ['version', '-m', resolve(path)], { encoding: 'utf8', maxBuffer: 1024 * 1024 });
  const lines = raw.trimEnd().split('\n'), settings = {};
  const version = /: (go\d+\.\d+\.\d+)$/.exec(lines[0]); assert(version, 'missing artifact Go version');
  for (const line of lines.slice(1)) {
    const fields = line.trim().split('\t');
    if (fields[0] !== 'build') continue;
    assert.equal(fields.length, 2, 'malformed linked build setting');
    const at = fields[1].indexOf('='); assert(at > 0, 'malformed build setting');
    const key = fields[1].slice(0, at); assert(!Object.hasOwn(settings, key), 'duplicate linked build setting');
    Object.defineProperty(settings, key, { value: fields[1].slice(at + 1), enumerable: true });
  }
  return { goVersion: version[1], settings, sha256: digest(bytes), buildInfoSource: wasm ? P.artifacts.wasm.buildInfoSource : P.artifacts.native.buildInfoSource };
}
export function verifyUnitPublicReports(p, directory) {
  const files = p.reports.flatMap(row => [row.nativeFile, row.wasmFile]);
  assert.deepEqual(readdirSync(directory).sort(), [...files].sort(), 'missing or unexpected public report files');
  const identityPaths = IDENTITY_KEYS.map(key => `/projection/manifest/buildIdentity/${key}`);
  const capture = new Set(['/raw', ...identityPaths]);
  for (const row of p.reports) for (const kind of ['native', 'wasm']) {
    const raw = fileBytes(join(directory, row[`${kind}File`]), MAX_REPORT, true);
    assert.equal(raw.length, row[`${kind}Bytes`], `${row.id}: report byte count`);
    assert.equal(digest(raw), row[`${kind}Sha256`], `${row.id}: report digest`);
    const parsed = parseStrictJSON(raw, capture), document = parsed.value;
    objectKeys(document, ['schema', 'projectionRequest', 'projectionRequestSha256', 'rawEnvelopeSha256', 'raw', 'projection'], `${row.id}: public wrapper`);
    assert.equal(document.schema, 'strat-adaptive-volume-flag-unit-runtime-v1');
    assert.deepEqual(document.projection.manifest.buildIdentity, p[`${kind}Identity`], `${row.id}: wrong executing identity`);
    assert.equal(document.projectionRequestSha256, row.projectionRequestSha256);
    assert.equal(document.rawEnvelopeSha256, row.rawEnvelopeSha256);
    const rawSpan = parsed.spans.get('/raw'); assert(rawSpan);
    assert.equal(digest(parsed.text.slice(rawSpan.start, rawSpan.delimiter)), row.rawEnvelopeSha256, 'original raw bytes changed');
    let comparable = parsed.text;
    const spans = identityPaths.map(path => { const span = parsed.spans.get(path); assert(span, `missing ${path}`); return span; }).sort((a, b) => b.start - a.start);
    for (const span of spans) comparable = comparable.slice(0, span.start) + 'null' + comparable.slice(span.end);
    assert.equal(digest(comparable), row.nonidentitySha256, `${row.id}: nonidentity report bytes changed`);
  }
  return files.length;
}
export function validateUnitReceiptFiles(parityPath, transportPath, reportDir, nativePath, wasmPath, shimPath, goPath, revision, executionTarget) {
  const p = readUnitReceipt(parityPath), t = readUnitReceipt(transportPath);
  const git = (...args) => execFileSync('git', ['-C', ROOT, ...args], { encoding: 'utf8', maxBuffer: MAX_RECEIPT }).trimEnd();
  assert(REVISION.test(revision), 'checkout revision argument required');
  assert.equal(revision, git('rev-parse', 'HEAD'), 'expected revision differs from checked-out HEAD');
  if (process.env.GITHUB_SHA) assert.equal(revision, process.env.GITHUB_SHA, 'expected revision differs from actual CI checkout SHA');
  const observed = { host: { platform: platform(), architecture: arch(), release: release(), node: process.version, v8: process.versions.v8 },
    native: readUnitArtifact(nativePath, goPath), wasm: readUnitArtifact(wasmPath, goPath, true),
    wasmExecSha256: digest(fileBytes(shimPath, MAX_RECEIPT)), revision, executionTarget,
    vcsModified: git('status', '--porcelain', '--untracked-files=normal') !== '',
    vcsTime: new Date(git('show', '-s', '--format=%cI', 'HEAD')).toISOString().replace('.000Z', 'Z') };
  const summary = validateUnitReceiptDocuments(p.value, t.value, observed);
  const publicReportFiles = verifyUnitPublicReports(p.value, reportDir);
  return { schema: 'adaptive-unit-receipt-integrity-v1', status: 'PASS', ...summary, publicReportFiles,
    rawCorpusSha256: P.rawCorpusSha256, unitCorpusSha256: P.unitCorpusSha256,
    paritySha256: p.sha256, transportSha256: t.sha256, retainedEvidence: p.value.retainedEvidence,
    evidenceBundle: p.value.evidenceBundle, host: observed.host, checkoutRevision: revision, checkoutModified: observed.vcsModified,
    artifacts: p.value.artifacts, nativeIdentity: p.value.nativeIdentity, wasmIdentity: p.value.wasmIdentity };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  assert.equal(process.argv.length, 11, 'exactly nine arguments are required');
  await emitAdaptiveReceipt(validateUnitReceiptFiles(...process.argv.slice(2)));
}
