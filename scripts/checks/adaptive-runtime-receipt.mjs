// Qualification evidence transport only. No strategy/report calculation lives here.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { resolve } from 'node:path';

const MAX_RECEIPT_BYTES = 2 * 1024 * 1024;
const digest = bytes => createHash('sha256').update(bytes).digest('hex');
const hash = value => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);

// Explicit process.exit is needed by WASM harnesses with a live Go runtime.
// Wait until the entire pipe write completes; console.log followed by exit can
// truncate a large receipt at the pipe buffer boundary. Persist exact bytes too.
export async function emitAdaptiveReceipt(value, receiptPath, output = process.stdout) {
  const raw = Buffer.from(`${JSON.stringify(value)}\n`, 'utf8');
  assert(raw.length > 1 && raw.length <= MAX_RECEIPT_BYTES, 'receipt exceeds bounded evidence size');
  JSON.parse(raw.toString('utf8'));
  if (receiptPath !== undefined) {
    assert(typeof receiptPath === 'string' && receiptPath.length > 0, 'receipt path is empty');
    writeFileSync(receiptPath, raw, { flag: 'wx' });
    assert(readFileSync(receiptPath).equals(raw), 'persisted receipt differs from emitted bytes');
  }
  await new Promise((done, fail) => {
    const onError = error => fail(error);
    output.once('error', onError);
    try {
      output.write(raw, error => {
        // On an error, retain the one-shot listener for the stream's matching
        // error event, preventing an unhandled event after callback rejection.
        if (error) { fail(error); return; }
        output.removeListener('error', onError);
        done();
      });
    } catch (error) {
      output.removeListener('error', onError);
      fail(error);
    }
  });
}

function readReceipt(path) {
  const raw = readFileSync(path);
  assert(raw.length > 1 && raw.length <= MAX_RECEIPT_BYTES && raw.at(-1) === 10, 'receipt is empty, oversized or missing its final newline');
  const value = JSON.parse(raw.toString('utf8'));
  assert(value && typeof value === 'object' && !Array.isArray(value), 'receipt requires an object');
  return { value, raw, sha256: digest(raw) };
}

// Fail closed before upload: both complete reports, exact frozen case coverage,
// input identities and their common WASM artifact must agree. This validates
// retained evidence, not a substitute calculation for the native/WASM results.
export function validateAdaptiveReceiptFiles(parityPath, transportPath) {
  const parityFile = readReceipt(parityPath), transportFile = readReceipt(transportPath);
  const p = parityFile.value, t = transportFile.value;
  const corpusRaw = readFileSync(new URL('../../testsupport/testdata/adaptive-flag-runtime-corpus-v1.json', import.meta.url));
  const corpusHash = 'af392754f48672cfe31d7161454dfc35ee73977e4c38f0a72b21c15be6fb2f97';
  assert.equal(digest(corpusRaw), corpusHash, 'frozen corpus changed');
  const corpus = JSON.parse(corpusRaw);
  assert.equal(p.schema, 'adaptive-flag-runtime-exact-parity-v1'); assert.equal(p.status, 'PASS');
  assert.equal(p.corpusSha256, corpusHash); assert.equal(p.designatedLifecycles, 12); assert.equal(p.suffixInvariancePairs, 6);
  assert.equal(p.reports?.length, 96); assert.equal(new Set(p.reports.map(row => row.id)).size, 96);
  const expected = new Map(corpus.cases.map(row => [row.id, row]));
  for (const row of p.reports) {
    const item = expected.get(row.id); assert(item, 'unknown or missing corpus case'); expected.delete(row.id);
    assert.equal(row.requestSha256, item.metadataSha256); assert.equal(row.sourceSha256, item.sourceSha256);
    assert.equal(row.btb1Sha256, item.btb1Sha256); assert.equal(row.configSha256, corpus.sources[item.sourceSha256].configSha256);
    for (const key of ['requestSha256', 'sourceSha256', 'btb1Sha256', 'configSha256', 'effectiveConfigSha256', 'retainedRowsSha256', 'nativeSha256', 'wasmSha256', 'rawRunSha256']) assert(hash(row[key]), `invalid ${key}`);
    assert.equal(row.nativeSha256, row.wasmSha256); assert.equal(row.byteIdentical, true);
    assert(Number.isSafeInteger(row.bytes) && row.bytes > 0);
    assert.equal(row.snapshots, item.expect.usedRows);
    if (item.expect.designatedFullLifecycle) assert(row.filled > 0 && row.closed > 0, 'designated lifecycle missing');
  }
  assert.equal(expected.size, 0, 'corpus receipt is incomplete');
  assert.equal(t.schema, 'adaptive-runtime-wasm-transport-check-v1'); assert.equal(t.status, 'PASS');
  assert.equal(t.accepted?.length, 19); assert.equal(t.rejected?.length, 115);
  assert.equal(t.recoveryCalls, 115); assert.equal(t.genericRefusals, 2);
  // Several parameterized refusals intentionally share a label; completeness
  // uses the fixed count, required row fields and recovery count, not uniqueness.
  for (const row of t.accepted) {
    assert(typeof row.label === 'string' && row.label.length > 0); assert(hash(row.reportSha256));
    if (Object.hasOwn(row, 'bytes')) assert(Number.isSafeInteger(row.bytes) && row.bytes > 0);
    else assert.equal(row.label, 'shadowed-own-accessors-ignored'); // preserved original receipt shape
  }
  for (const row of t.rejected) {
    assert(typeof row.label === 'string' && row.label.length > 0);
    assert(['request', 'source', 'input', 'resource', 'execution'].includes(row.phase));
    assert(Number.isSafeInteger(row.wordReads) && row.wordReads >= 0);
  }
  assert.equal(t.node, p.host?.node);
  assert(['x64', 'arm64'].includes(p.host?.architecture));
  assert(['linux', 'darwin'].includes(p.host?.platform));
  assert.equal(t.wasmSha256, p.artifacts?.wasm?.sha256, 'receipt artifacts differ');
  for (const value of [t.wasmSha256, p.artifacts?.native?.sha256, p.artifacts?.wasmExecSha256]) assert(hash(value), 'invalid artifact identity');
  const native = p.artifacts.native, wasm = p.artifacts.wasm;
  assert.equal(native.goVersion, wasm.goVersion); assert.match(native.goVersion, /^go\d+\.\d+\.\d+$/);
  for (const artifact of [native, wasm]) {
    assert.equal(artifact.settings?.['-compiler'], 'gc'); assert.equal(artifact.settings?.CGO_ENABLED, '0');
    assert.equal(artifact.settings?.['-trimpath'], 'true'); assert.equal(artifact.settings?.['-buildmode'], 'exe');
    assert(!Object.hasOwn(artifact.settings, '-gcflags') && !Object.hasOwn(artifact.settings, '-ldflags'));
  }
  assert.equal(wasm.settings?.GOARCH, 'wasm'); assert.equal(wasm.settings?.GOOS, 'js');
  assert.equal(native.settings?.GOARCH, { x64: 'amd64', arm64: 'arm64' }[p.host?.architecture]);
  assert.equal(native.settings?.GOOS, p.host?.platform);
  assert.equal(native.settings?.['vcs.revision'], wasm.settings?.['vcs.revision']);
  assert.equal(native.settings?.['vcs.modified'], wasm.settings?.['vcs.modified']);
  assert.match(native.settings['vcs.revision'], /^[a-f0-9]{40}$/);
  assert(['true', 'false'].includes(native.settings['vcs.modified']));
  return { schema: 'adaptive-runtime-receipt-integrity-v1', status: 'PASS', corpusSha256: corpusHash,
    paritySha256: parityFile.sha256, transportSha256: transportFile.sha256, reports: 96,
    host: p.host, nativeSha256: native.sha256, wasmSha256: wasm.sha256 };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [parityPath, transportPath, ...extra] = process.argv.slice(2);
  assert(parityPath && transportPath && !extra.length, 'exactly two receipt paths are required');
  console.log(JSON.stringify(validateAdaptiveReceiptFiles(parityPath, transportPath)));
}
