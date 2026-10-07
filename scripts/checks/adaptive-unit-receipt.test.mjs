import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Writable } from 'node:stream';
import { test } from 'node:test';
import { EVIDENCE_IDENTITY } from './adaptive-flag-unit-evidence.mjs';
import { emitAdaptiveReceipt } from './adaptive-runtime-receipt.mjs';
import { parseStrictJSON, readUnitReceipt, readUnitArtifact, validateUnitReceiptDocuments, validateUnitReceiptFiles, verifyUnitPublicReports } from './adaptive-unit-receipt.mjs';

const fixture = name => new URL(`../../testsupport/testdata/adaptive-unit-receipts/${name}`, import.meta.url);
const parityRaw = readFileSync(fixture('b2-parity.json'));
const transportRaw = readFileSync(fixture('b2-transport.json'));
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
const H = 'a'.repeat(64);
const helperURL = new URL('./adaptive-runtime-receipt.mjs', import.meta.url).href;
function pair() {
  const p = JSON.parse(parityRaw), t = JSON.parse(transportRaw);
  p.schema = 'adaptive-flag-unit-public-parity-v2'; p.evidenceBundle = structuredClone(EVIDENCE_IDENTITY);
  t.schema = 'adaptive-unit-wasm-transport-check-v2';
  const baseline = { reportSha256: t.accepted[0].reportSha256, bytes: t.accepted[0].bytes };
  t.recoveries = t.rejected.map((row, rejectionIndex) => ({ rejectionIndex, label: row.label, ...baseline }));
  const error = 'adaptive-volume-flag-native-dedicated-runner-required';
  t.genericResults = [{ label: 'engineRunFixture', result: JSON.stringify({ error }) }, { label: 'engineRunColumns', result: { ok: false, error } }];
  t.finalRecovery = { ...baseline };
  const observed = { host: structuredClone(p.host), native: structuredClone(p.artifacts.native), wasm: structuredClone(p.artifacts.wasm),
    wasmExecSha256: p.artifacts.wasmExecSha256, revision: p.nativeIdentity.vcsRevision,
    vcsTime: p.artifacts.native.settings['vcs.time'], vcsModified: true, executionTarget: 'linux-amd64' };
  return { p, t, observed };
}
function temporary(fn) {
  const dir = mkdtempSync(join(tmpdir(), 'adaptive-unit-receipt-test-'));
  try { return fn(dir); } finally { rmSync(dir, { recursive: true, force: true }); }
}
const validate = ({ p, t, observed }) => validateUnitReceiptDocuments(p, t, observed);

test('approved original B2 receipt bytes remain immutable; complete v2 documents validate', () => {
  assert.equal(sha(parityRaw), 'b981ed430fcad8318a444d06fb5eb7684956e2b14de115c248672c93cbf3e91a');
  assert.equal(sha(transportRaw), 'ed1fd53bad0af48dbbb2800f9288b7281ad3683f0d4f69726d7d3ed351f1c968');
  assert.deepEqual(validate(pair()), { executionTarget: 'linux-amd64', reports: 288, accepted: 27, rejected: 168, recoveries: 168, genericRefusals: 2 });
});

test('real child PIPE drains an entire >64KiB unit receipt before explicit exit', () => temporary(dir => {
  const path = join(dir, 'receipt.json');
  const child = `import {readFileSync} from 'node:fs'; import {emitAdaptiveReceipt} from ${JSON.stringify(helperURL)}; const value=JSON.parse(readFileSync(new URL(${JSON.stringify(fixture('b2-parity.json').href)}))); await emitAdaptiveReceipt(value, ${JSON.stringify(path)}); process.exit(0);`;
  const raw = execFileSync(process.execPath, ['--input-type=module', '-e', child], { maxBuffer: 2 * 1024 * 1024 });
  assert(raw.length > 65536); assert.equal(raw.at(-1), 10);
  assert(raw.equals(parityRaw)); assert(raw.equals(readFileSync(path)));
  assert.equal(readUnitReceipt(path).value.reports.length, 288);
}));

test('missing, empty, truncated, oversized and symlinked receipt files fail closed', () => temporary(dir => {
  const path = join(dir, 'parity.json');
  assert.throws(() => readUnitReceipt(path), /ENOENT/);
  for (const raw of [Buffer.alloc(0), Buffer.from('\n'), Buffer.from('{\n'), parityRaw.subarray(0, 65536), parityRaw.subarray(0, -1), Buffer.alloc(2 * 1024 * 1024 + 1, 32)]) {
    writeFileSync(path, raw); assert.throws(() => readUnitReceipt(path));
  }
  writeFileSync(path, parityRaw); const link = join(dir, 'linked.json'); symlinkSync(path, link);
  assert.throws(() => readUnitReceipt(link), /regular file/);
  assert.throws(() => readUnitReceipt(dir), /regular file/);
  assert.throws(() => validateUnitReceiptFiles(join(dir, 'missing'), path), /ENOENT/);
}));

test('strict JSON rejects duplicate decoded members, invalid UTF-8, surrogates and partial tokens', () => {
  for (const text of [
    '{"schema":1,"sch\\u0065ma":2}', '{"a":{"code":1,"c\\u006fde":2}}', '{"a":[{"x":1,"x":1}]}',
    '{"x":"\\ud800"}', '{"x":"\\udfff"}', '{"\\ud800":1}', '{"x":"\\ud800A"}',
    '{"x":"\\udc00\\ud800"}', '{"x":1e9999}', '{"x":01}', '{"x":1,}', '[1,]',
    '{"x":true}{}', '{"x":"line\nfeed"}', '\ufeff{}', '['.repeat(66) + ']'.repeat(66),
  ]) assert.throws(() => parseStrictJSON(Buffer.from(text)), text);
  for (const bytes of [[0xc0, 0xaf], [0xed, 0xa0, 0x80], [0xf4, 0x90, 0x80, 0x80], [0xe2, 0x82]]) {
    assert.throws(() => parseStrictJSON(Buffer.concat([Buffer.from('{"x":"'), Buffer.from(bytes), Buffer.from('"}')])));
  }
  assert.deepEqual(parseStrictJSON(Buffer.from('{"sch\\u0065ma":"\\ud800\\udc00","array":[null,true,-0,1.5]}')).value,
    { schema: '\u{10000}', array: [null, true, -0, 1.5] });
  const proto = parseStrictJSON(Buffer.from('{"__proto__":{"polluted":true}}')).value;
  assert.equal(Object.getPrototypeOf(proto), Object.prototype); assert.equal({}.polluted, undefined);
});

const invalidReceiptNumbers = [
  ['p', 'oracleCalls', '1e-9999'],
  ['p', 'sourceCases', '96.000000000000000000000000001'],
  ['t', 'recoveryCalls', '168.00000000000000000001'],
  ['p', 'sourceCases', '96e0'], ['p', 'sourceCases', '9.6e1'], ['p', 'sourceCases', '96.0'],
  ['t', 'recoveryCalls', '168E+0'], ['t', 'recoveryCalls', '1.68E2'],
  ['p', 'oracleCalls', '0e0'], ['p', 'oracleCalls', '0.0'], ['p', 'oracleCalls', '-0'],
  ['p', 'oracleCalls', '-0.0'], ['p', 'oracleCalls', '-1e-9999'], ['p', 'oracleCalls', '-1'],
  ['t', 'wordReads', '0e9999'], ['t', 'rejectionIndex', '0.00000000000000000001'],
  ['p', 'nativeBytes', '67143.00000000000000000001'], ['t', 'bytes', '18017e0'],
  ['p', 'oracleCalls', '9007199254740992'],
];
function receiptNumberMutation(which, field, token) {
  const { p, t } = pair(), original = { p: `${JSON.stringify(p)}\n`, t: `${JSON.stringify(t)}\n` };
  const pattern = new RegExp(`("${field}":)(?:0|[1-9][0-9]*)(?=[,}])`);
  assert(pattern.test(original[which]), `missing mutation field ${field}`);
  const mutated = { ...original, [which]: original[which].replace(pattern, (_match, prefix) => prefix + token) };
  return { original, mutated };
}

test('receipt reader rejects lossy numeric lexemes before conversion at every depth', () => temporary(dir => {
  const path = join(dir, 'receipt.json');
  for (const [which, field, token] of invalidReceiptNumbers) {
    const { mutated } = receiptNumberMutation(which, field, token);
    writeFileSync(path, mutated[which]);
    assert.throws(() => readUnitReceipt(path), /receipt (number requires a canonical nonnegative integer token|integer exceeds safe range)/, `${field}:${token}`);
  }
  for (const token of ['0', '1', '96', '168', '9007199254740991']) {
    writeFileSync(path, `{"counter":${token},"nested":[${token}],"string":"1e-9999"}\n`);
    const parsed = readUnitReceipt(path);
    assert.equal(parsed.value.counter, Number(token)); assert.equal(parsed.value.nested[0], Number(token));
    assert.equal(parsed.value.string, '1e-9999');
  }
  for (const token of ['+0', '00', '01', '.0']) {
    writeFileSync(path, `{"counter":${token}}\n`); assert.throws(() => readUnitReceipt(path), `${token} is not JSON integer grammar`);
  }
  writeFileSync(path, parityRaw); assert.deepEqual(readUnitReceipt(path).value, JSON.parse(parityRaw));
  writeFileSync(path, transportRaw); assert.deepEqual(readUnitReceipt(path).value, JSON.parse(transportRaw));
}));

test('public CLI rejects raw decimal, exponent and underflow receipt counts without a PASS', () => temporary(dir => {
  const cli = new URL('./adaptive-unit-receipt.mjs', import.meta.url);
  const parity = join(dir, 'parity.json'), transport = join(dir, 'transport.json');
  for (const [which, field, token] of invalidReceiptNumbers) {
    const { mutated } = receiptNumberMutation(which, field, token);
    writeFileSync(parity, mutated.p); writeFileSync(transport, mutated.t);
    // The public entry point must reject the original receipt token before any
    // artifact access. Deliberately absent artifacts make this ordering explicit.
    assert.throws(() => execFileSync(process.execPath, [cli.pathname, parity, transport, dir, 'native', 'wasm', 'shim', 'go', 'b'.repeat(40), 'linux-amd64'], { stdio: ['ignore', 'pipe', 'pipe'] }),
      error => error.status !== 0 && error.stdout.length === 0 && /receipt (number requires a canonical nonnegative integer token|integer exceeds safe range)/.test(error.stderr.toString()), `${field}:${token}`);
  }
}));

test('economic report parser keeps decimal, exponent, subnormal and signed-zero bytes unchanged', () => {
  const tokens = ['1.5', '96.000000000000000000000000001', '168e0', '1e-9999', '5e-324', '-0', '-0.0', '-1e-9999', '1.7976931348623157e308'];
  for (const token of tokens) {
    const raw = Buffer.from(`{"value":${token}}\n`);
    const parsed = parseStrictJSON(raw, new Set(['/value']));
    const span = parsed.spans.get('/value');
    assert.equal(parsed.text, raw.toString()); assert.equal(parsed.text.slice(span.start, span.end), token);
    assert(Object.is(parsed.value.value, JSON.parse(token)), token);
  }
});

test('every parity level has a closed schema and requires complete ordered evidence', () => {
  const mutations = [
    p => { p.extra = true; }, p => { delete p.status; }, p => { p.status = 'FAIL'; }, p => { p.schema = 'adaptive-flag-unit-public-parity-v1'; },
    p => { p.rawCorpusSha256 = H; }, p => { p.unitCorpusSha256 = H; }, p => { p.retainedEvidence.invocationPlanSha256 = H; },
    p => { p.retainedEvidence.extra = H; }, p => { delete p.retainedEvidence.oracleManifestSha256; }, p => { p.retainedCoreIdentity.verified = true; },
    p => { p.evidenceBundle.packSha256 = H; }, p => { p.evidenceBundle.path = '/private'; }, p => { delete p.evidenceBundle; },
    p => { p.identityLeafAllowlist.push('/economics'); }, p => { p.sourceCases--; }, p => { p.policies++; }, p => { p.oracleCalls = 1; },
    p => { p.qualification = 'PASS'; }, p => { p.host.extra = 'fake'; }, p => { delete p.host.release; }, p => { p.host.node = 'v99.0.0'; },
    p => { p.artifacts.extra = H; }, p => { p.artifacts.native.sha256 = H; }, p => { p.artifacts.wasm.sha256 = H; },
    p => { p.artifacts.wasmExecSha256 = H; }, p => { p.artifacts.native.buildInfoSource = 'inferred'; },
    p => { p.artifacts.native.settings.GOARCH = 'arm64'; }, p => { p.artifacts.native.settings['vcs.modified'] = 'false'; },
    p => { p.artifacts.native.settings['-gcflags'] = 'all=-N'; }, p => { p.artifacts.wasm.settings['-ldflags'] = '-s'; },
    p => { p.artifacts.wasm.goVersion = 'go1.23.0'; }, p => { delete p.artifacts.native.settings.vcs; },
    p => { p.nativeIdentity.vcsRevision = 'b'.repeat(40); }, p => { p.wasmIdentity.goarch = 'arm64'; }, p => { p.nativeIdentity.extra = true; },
    p => { p.reports.pop(); }, p => { p.reports[0] = p.reports[1]; }, p => { [p.reports[0], p.reports[1]] = [p.reports[1], p.reports[0]]; },
    p => { p.reports[0].extra = true; }, p => { delete p.reports[0].fullCoreProjectionEqual; }, p => { p.reports[0].fullNonidentityBytesEqual = false; },
    p => { p.reports[0].originalRawBytesEqual = false; }, p => { p.reports[0].nativeFile = '../outside.json'; },
    p => { p.reports[0].rawCaseId = p.reports[3].rawCaseId; }, p => { p.reports[0].nativeBytes--; }, p => { p.reports[0].wasmBytes = null; },
    ...['projectionRequestSha256', 'rawEnvelopeSha256', 'nativeSha256', 'wasmSha256', 'nonidentitySha256', 'coreProjectionSha256', 'oracleReferenceSha256', 'oracleComparisonSha256'].map(key => p => { p.reports[0][key] = H; }),
  ];
  for (const mutate of mutations) { const value = pair(); mutate(value.p); assert.throws(() => validate(value), String(mutate)); }
});

test('every ordered transport refusal, recovery and generic result is mandatory', () => {
  const mutations = [
    t => { t.extra = 1; }, t => { t.status = 'FAIL'; }, t => { t.schema = 'adaptive-unit-wasm-transport-check-v1'; },
    t => { t.wasmSha256 = H; }, t => { t.node = 'other'; }, t => { t.accepted.pop(); }, t => { t.rejected.pop(); },
    t => { t.accepted[0] = t.accepted[1]; }, t => { t.accepted[1].reportSha256 = t.accepted[0].reportSha256; },
    t => { t.accepted[0].bytes--; }, t => { delete t.accepted[1].reportSha256; }, t => { t.accepted[21].bytes = 1; },
    t => { t.rejected[0].extra = true; }, t => { t.rejected[0] = t.rejected[1]; }, t => { delete t.rejected[0].code; },
    t => { t.rejected[0].phase = 'input'; }, t => { t.rejected[0].wordReads++; }, t => { t.rejected[1].label = 'made-up'; },
    t => { t.recoveryCalls--; }, t => { t.genericRefusals--; }, t => { delete t.recoveries; }, t => { t.recoveries.pop(); },
    t => { t.recoveries[2] = t.recoveries[1]; }, t => { t.recoveries[0].rejectionIndex = 1; },
    t => { t.recoveries[0].label = 'other'; }, t => { t.recoveries[0].reportSha256 = H; }, t => { t.recoveries[0].bytes--; },
    t => { t.recoveries[0].extra = 1; }, t => { t.genericResults.pop(); }, t => { t.genericResults.reverse(); },
    t => { t.genericResults[0].result = '{}'; }, t => { t.genericResults[1].result.ok = true; },
    t => { t.genericResults[1].result.extra = true; }, t => { t.finalRecovery.reportSha256 = H; }, t => { delete t.finalRecovery; },
  ];
  for (const mutate of mutations) { const value = pair(); mutate(value.t); assert.throws(() => validate(value), String(mutate)); }
});

test('observations bind target, host, checkout revision, dirty state and linked artifacts', () => {
  for (const mutate of [
    o => { o.executionTarget = 'linux-arm64'; }, o => { o.host.architecture = 'arm64'; }, o => { o.host.release = 'other'; },
    o => { o.revision = 'b'.repeat(40); }, o => { o.vcsModified = false; }, o => { o.vcsTime = '2020-01-01T00:00:00Z'; },
    o => { o.wasmExecSha256 = H; }, o => { o.native.sha256 = H; }, o => { o.wasm.sha256 = H; },
  ]) { const value = pair(); mutate(value.observed); assert.throws(() => validate(value)); }
});

test('schema accepts a separately observed ARM64 checkout identity without pinning B2 revision', () => {
  const value = pair(), { p, t, observed } = value;
  p.host.architecture = 'arm64'; observed.host.architecture = 'arm64'; observed.executionTarget = 'linux-arm64';
  observed.revision = 'c'.repeat(40); observed.vcsModified = false;
  for (const kind of ['native', 'wasm']) {
    p.artifacts[kind].settings['vcs.revision'] = observed.revision;
    p.artifacts[kind].settings['vcs.modified'] = 'false';
    p[`${kind}Identity`].vcsRevision = observed.revision; p[`${kind}Identity`].vcsModified = false;
    if (kind === 'native') { p.artifacts.native.settings.GOARCH = 'arm64'; delete p.artifacts.native.settings.GOAMD64; p.nativeIdentity.goarch = 'arm64'; }
    observed[kind] = structuredClone(p.artifacts[kind]);
    for (const row of p.reports) row[`${kind}Bytes`]++; // false has one more byte than true
  }
  for (const row of t.accepted) if (Object.hasOwn(row, 'bytes')) row.bytes++;
  for (const row of t.recoveries) row.bytes++;
  t.finalRecovery.bytes++;
  assert.equal(validate(value).executionTarget, 'linux-arm64');
});

test('CLI refuses missing or truncated evidence with a nonzero exit and no success stdout', () => temporary(dir => {
  const cli = new URL('./adaptive-unit-receipt.mjs', import.meta.url);
  const parity = join(dir, 'parity.json'), transport = join(dir, 'transport.json');
  writeFileSync(transport, transportRaw);
  for (const raw of [undefined, parityRaw.subarray(0, 65536)]) {
    if (raw) writeFileSync(parity, raw);
    assert.throws(() => execFileSync(process.execPath, [cli.pathname, parity, transport, dir, 'native', 'wasm', 'shim', 'go', 'b'.repeat(40), 'linux-amd64'], { stdio: ['ignore', 'pipe', 'pipe'] }),
      error => error.status !== 0 && error.stdout.length === 0);
  }
}));

function syntheticReports(dir) {
  const { p } = pair();
  const row = { id: 'invented-file-boundary-control', nativeFile: '000-native.json', wasmFile: '000-wasm.json', projectionRequestSha256: H };
  const raw = '{"rawOnly":true,"signedZero":-0}\n'; row.rawEnvelopeSha256 = sha(raw);
  for (const kind of ['native', 'wasm']) {
    const text = `{"schema":"strat-adaptive-volume-flag-unit-runtime-v1","projectionRequest":{},"projectionRequestSha256":"${H}","rawEnvelopeSha256":"${row.rawEnvelopeSha256}","raw":${raw},"projection":{"manifest":{"buildIdentity":${JSON.stringify(p[`${kind}Identity`])}},"value":-0}}\n`;
    row[`${kind}Bytes`] = Buffer.byteLength(text); row[`${kind}Sha256`] = sha(text);
    const nulled = Object.fromEntries(Object.keys(p[`${kind}Identity`]).map(key => [key, null]));
    const normalized = text.replace(JSON.stringify(p[`${kind}Identity`]), JSON.stringify(nulled));
    if (row.nonidentitySha256) assert.equal(sha(normalized), row.nonidentitySha256); else row.nonidentitySha256 = sha(normalized);
    writeFileSync(join(dir, row[`${kind}File`]), text);
  }
  p.reports = [row]; return p;
}
test('public file validation binds complete bytes and preserves economic signed zero', () => temporary(dir => {
  let p = syntheticReports(dir); assert.equal(verifyUnitPublicReports(p, dir), 2);
  const path = join(dir, '000-native.json'), original = readFileSync(path);
  for (const changed of [original.subarray(0, -1), Buffer.concat([original, Buffer.from(' ')]), Buffer.from(original.toString().replace('"value":-0', '"value":0'))]) {
    writeFileSync(path, changed); assert.throws(() => verifyUnitPublicReports(p, dir));
  }
  // Matching a forged receipt's hash and length cannot conceal economic drift.
  const changed = Buffer.from(original.toString().replace('"value":-0', '"value":0'));
  writeFileSync(path, changed); p.reports[0].nativeBytes = changed.length; p.reports[0].nativeSha256 = sha(changed);
  assert.throws(() => verifyUnitPublicReports(p, dir), /nonidentity report bytes/);
  p = syntheticReports(dir); rmSync(path); assert.throws(() => verifyUnitPublicReports(p, dir), /report files/);
  p = syntheticReports(dir); writeFileSync(join(dir, 'unexpected.json'), '{}'); assert.throws(() => verifyUnitPublicReports(p, dir), /report files/);
}));

test('linked WASM identity reader refuses malformed, duplicated and truncated evidence', () => temporary(dir => {
  const path = join(dir, 'invented.wasm');
  const start = Buffer.from('3077af0c9274080241e1c107e6d618e6', 'hex'), end = Buffer.from('f932433186182072008242104116d8f2', 'hex');
  const header = Buffer.from([0, 97, 115, 109, 1, 0, 0, 0]);
  const module = Buffer.from('\n\tbuild\tGOARCH=wasm\n');
  const valid = Buffer.concat([header, Buffer.from('\0go1.22.12\0'), start, module, end]);
  writeFileSync(path, valid); assert.equal(readUnitArtifact(path, 'unused', true).settings.GOARCH, 'wasm');
  for (const bytes of [Buffer.from('not wasm'), valid.subarray(0, -1), Buffer.concat([valid, start]), Buffer.concat([valid, end]),
    Buffer.concat([valid, Buffer.from('\0go1.23.0\0')]), Buffer.concat([header, Buffer.from('\0go1.22.12\0'), start, module, module, end])]) {
    writeFileSync(path, bytes); assert.throws(() => readUnitArtifact(path, 'unused', true));
  }
}));

test('file and stdout failures cannot become successful receipt emission', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'adaptive-unit-emission-test-'));
  try {
    const path = join(dir, 'existing.json'); writeFileSync(path, 'keep');
    await assert.rejects(emitAdaptiveReceipt(pair().p, path), /EEXIST/); assert.equal(readFileSync(path, 'utf8'), 'keep');
    await assert.rejects(emitAdaptiveReceipt(pair().p, join(dir, 'missing', 'receipt.json')), /ENOENT/);
    const output = new Writable({ write(_chunk, _encoding, callback) { callback(new Error('invented broken pipe')); } });
    await assert.rejects(emitAdaptiveReceipt(pair().p, undefined, output), /invented broken pipe/);
  } finally { rmSync(dir, { recursive: true, force: true }); }
});
