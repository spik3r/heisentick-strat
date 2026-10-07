import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { Writable } from 'node:stream';
import { emitAdaptiveReceipt, validateAdaptiveReceiptFiles } from './adaptive-runtime-receipt.mjs';

const moduleURL = new URL('./adaptive-runtime-receipt.mjs', import.meta.url).href;
const corpus = JSON.parse(readFileSync(new URL('../../testsupport/testdata/adaptive-flag-runtime-corpus-v1.json', import.meta.url)));
const H = 'a'.repeat(64);
function pair() {
  const settings = { '-buildmode': 'exe', '-compiler': 'gc', '-trimpath': 'true', CGO_ENABLED: '0', GOARCH: 'amd64', GOOS: 'linux', 'vcs.revision': 'b'.repeat(40), 'vcs.modified': 'false' };
  const p = { schema: 'adaptive-flag-runtime-exact-parity-v1', status: 'PASS', corpusSha256: 'af392754f48672cfe31d7161454dfc35ee73977e4c38f0a72b21c15be6fb2f97', designatedLifecycles: 12, suffixInvariancePairs: 6,
    host: { platform: 'linux', architecture: 'x64', node: 'invented-node-version' }, artifacts: { native: { sha256: H, goVersion: 'go1.22.12', settings }, wasm: { sha256: H, goVersion: 'go1.22.12', settings: { ...settings, GOARCH: 'wasm', GOOS: 'js' } }, wasmExecSha256: H },
    reports: corpus.cases.map(c => ({ id: c.id, requestSha256: c.metadataSha256, sourceSha256: c.sourceSha256, btb1Sha256: c.btb1Sha256, configSha256: corpus.sources[c.sourceSha256].configSha256, effectiveConfigSha256: H, retainedRowsSha256: H, nativeSha256: H, wasmSha256: H, rawRunSha256: H, bytes: 1, snapshots: c.expect.usedRows, filled: 1, closed: 1, byteIdentical: true })) };
  const t = { schema: 'adaptive-runtime-wasm-transport-check-v1', status: 'PASS', wasmSha256: H, node: 'invented-node-version', accepted: Array.from({ length: 19 }, (_, i) => ({ label: `ok-${i}`, reportSha256: H, bytes: 1 })), rejected: Array.from({ length: 115 }, (_, i) => ({ label: `bad-${i}`, phase: 'input', wordReads: 0 })), recoveryCalls: 115, genericRefusals: 2 };
  return { p, t };
}
function withFiles(fn) {
  const dir = mkdtempSync(join(tmpdir(), 'adaptive-receipt-test-'));
  try { return fn(join(dir, 'parity.json'), join(dir, 'transport.json')); }
  finally { rmSync(dir, { recursive: true, force: true }); }
}
function write(path, value) { writeFileSync(path, `${JSON.stringify(value)}\n`); }

test('large child-process PIPE drains completely before explicit exit', () => withFiles(path => {
  const value = { schema: 'invented-pipe-regression-v1', payload: 'x'.repeat(256 * 1024), tail: 'complete' };
  const child = `import { emitAdaptiveReceipt } from ${JSON.stringify(moduleURL)}; await emitAdaptiveReceipt({ schema: 'invented-pipe-regression-v1', payload: 'x'.repeat(256 * 1024), tail: 'complete' }, ${JSON.stringify(path)}); process.exit(0);`;
  // spawn's captured stdout is a real pipe, not a terminal or redirected file.
  const raw = execFileSync(process.execPath, ['--input-type=module', '-e', child], { maxBuffer: 2 * 1024 * 1024 });
  assert(raw.length > 65536); assert.equal(raw.at(-1), 10);
  assert.deepEqual(JSON.parse(raw), value); assert(raw.equals(readFileSync(path)));
}));

test('persisted output failure cannot fall through to success', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'adaptive-receipt-write-'));
  try {
    const path = join(dir, 'existing.json'); writeFileSync(path, 'preserve');
    await assert.rejects(emitAdaptiveReceipt({ ok: true }, path), /EEXIST/);
    assert.equal(readFileSync(path, 'utf8'), 'preserve');
  } finally { rmSync(dir, { recursive: true, force: true }); }
});

test('complete invented receipt pair validates and binds artifacts', () => withFiles((a, b) => {
  const { p, t } = pair(); write(a, p); write(b, t);
  const result = validateAdaptiveReceiptFiles(a, b); assert.equal(result.status, 'PASS'); assert.equal(result.reports, 96);
}));

test('missing empty malformed truncated or valid-but-incomplete receipt fails', () => withFiles((a, b) => {
  const { p, t } = pair(); write(b, t);
  assert.throws(() => validateAdaptiveReceiptFiles(a, b));
  for (const raw of ['', '\n', '{', JSON.stringify(p).slice(0, 65536), JSON.stringify(p)]) {
    writeFileSync(a, raw); assert.throws(() => validateAdaptiveReceiptFiles(a, b));
  }
  for (const mutate of [x => { x.reports.pop(); }, x => { x.reports[0] = x.reports[1]; }, x => { x.status = 'FAIL'; }, x => { x.corpusSha256 = H; }, x => { x.reports[0].nativeSha256 = 'c'.repeat(64); }, x => { delete x.reports[0].retainedRowsSha256; }, x => { x.artifacts.native.settings.GOARCH = 'arm64'; }]) {
    const value = structuredClone(p); mutate(value); write(a, value); assert.throws(() => validateAdaptiveReceiptFiles(a, b));
  }
  write(a, p);
  for (const mutate of [x => { x.rejected.pop(); }, x => { x.status = 'FAIL'; }, x => { x.wasmSha256 = 'c'.repeat(64); }, x => { delete x.rejected[0].phase; }, x => { x.recoveryCalls--; }]) {
    const value = structuredClone(t); mutate(value); write(b, value); assert.throws(() => validateAdaptiveReceiptFiles(a, b));
  }
}));

test('stdout callback failure rejects rather than reaching an explicit success exit', async () => {
  const output = new Writable({ write(_chunk, _encoding, callback) { callback(new Error('invented pipe failure')); } });
  await assert.rejects(emitAdaptiveReceipt({ test: 'pipe failure' }, undefined, output), /invented pipe failure/);
});
