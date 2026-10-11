// Strict evidence validation shared by the runner, comparator and generators.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
export const sha = (bytes) => createHash('sha256').update(bytes).digest('hex');
export const fileSha = (file) => sha(readFileSync(file));
export const stable = (value) => JSON.stringify(value, (_, x) => x && !Array.isArray(x) && typeof x === 'object' ? Object.fromEntries(Object.keys(x).sort().map(k => [k, x[k]])) : x);
export const digest = (value) => sha(stable(value));
const hex = /^[0-9a-f]{16}$/;
const hash = /^[0-9a-f]{64}$/;
export const tradeFields = ['entry', 'exit', 'sl', 'tp', 'initialSl', 'initialTp', 'size', 'points'];
export const decisionFields = ['side', 'entryIndex', 'exitIndex', 'reason', 'rule', 'tag', 'partial'];
export function manifestCases(manifest) {
  assert.equal(manifest.schema, 'ht245.manifest.v1', 'unknown manifest schema');
  assert.ok(Array.isArray(manifest.cases), 'manifest cases required');
  const ids = new Set();
  for (const c of manifest.cases) {
    assert.ok(typeof c.id === 'string' && c.id && !ids.has(c.id), 'missing/duplicate manifest case ID');
    assert.ok(typeof c.source === 'string' && /^conformance\/run\/[\w.-]+$/.test(c.source), 'invalid fixture source');
    if (c.expectedError !== undefined) assert.equal(c.expectedError, 'invalid-sequential-input: bars.binding at bar 0', 'unreviewed expected refusal');
    ids.add(c.id);
  }
  return manifest.cases;
}
export function parseTarget(target) {
  assert.equal(typeof target, 'string', 'missing runtime identity');
  const at = target.indexOf(' {');
  assert.ok(at > 0, 'missing runtime label/metadata');
  return { label: target.slice(0, at), ...JSON.parse(target.slice(at + 1)) };
}
export function validateRows(rows, manifest, expected, inputHashes) {
  const cases = manifestCases(manifest), byId = new Map();
  assert.equal(rows.length, cases.length, 'incomplete/extra receipt count');
  for (const row of rows) {
    assert.equal(row.schema, 'ht245.receipt.v1', 'unknown receipt schema');
    assert.ok(!byId.has(row.id), 'duplicate receipt ID');
    const runtime = parseTarget(row.target);
    if (expected) assert.deepEqual(runtime, expected, 'receipt runtime/source differs from executed binary');
    assert.ok(hash.test(row.inputSha256), 'missing input digest');
    assert.ok(Number.isInteger(row.tradeCount) && row.tradeCount >= 0, 'invalid trade count');
    byId.set(row.id, row);
  }
  for (const c of cases) {
    const row = byId.get(c.id);
    assert.ok(row, `missing expected case ${c.id}`);
    if (inputHashes) assert.equal(row.inputSha256, inputHashes[c.id], 'receipt input digest differs from actual fixture/source/overrides');
    if (row.error) {
      assert.equal(row.error, c.expectedError, `${c.id}: unexpected execution error`);
      assert.equal(row.tradeCount, 0, 'refusal has trades');
      assert.ok(row.trades === null || (Array.isArray(row.trades) && row.trades.length === 0), 'refusal has trade rows');
    } else {
      assert.ok(!c.expectedError, `${c.id}: expected refusal was not observed`);
      assert.ok(Array.isArray(row.trades) || (row.trades === null && row.tradeCount === 0), 'invalid trades');
      assert.equal((row.trades || []).length, row.tradeCount, 'trade count does not match rows');
      for (const t of row.trades || []) {
        for (const k of [...tradeFields, 'pnl']) {
          assert.ok(hex.test(t[k]), `invalid serialized float bits: ${k}`);
          assert.notEqual((BigInt('0x' + t[k]) >> 52n) & 0x7ffn, 0x7ffn, 'non-finite serialized value');
        }
        for (const k of ['entryIndex', 'exitIndex']) assert.ok(Number.isInteger(t[k]) && t[k] >= 0, `invalid ${k}`);
        for (const k of ['side', 'reason']) assert.equal(typeof t[k], 'string', `invalid ${k}`);
      }
    }
  }
  return byId;
}
export function readRows(file) {
  const text = readFileSync(file, 'utf8').trim();
  return text ? text.split('\n').map(line => JSON.parse(line)) : [];
}
export function loadRun(dir) {
  const run = JSON.parse(readFileSync(join(dir, 'run.json'), 'utf8'));
  assert.equal(run.schema, 'ht245.run.v2', 'missing current run envelope');
  assert.equal(run.status, 'complete', 'run failed or incomplete');
  assert.ok(Array.isArray(run.targets) && run.targets.length, 'no requested targets');
  const manifestFile = join(dir, 'manifest.json'), identityFile = join(dir, 'identity.json');
  assert.equal(fileSha(manifestFile), run.manifestSha256, 'manifest changed since execution');
  assert.equal(fileSha(identityFile), run.identitySha256, 'source identity changed since execution');
  const manifest = JSON.parse(readFileSync(manifestFile, 'utf8'));
  const identity = JSON.parse(readFileSync(identityFile, 'utf8'));
  assert.equal(digest(identity.sourceFiles), identity.sourceSha256, 'invalid source digest');
  assert.equal(digest(identity.tools), identity.toolsSha256, 'invalid harness/tool digest');
  assert.equal(digest(identity.inputs), identity.inputFilesSha256, 'invalid input-file digest');
  assert.deepEqual(Object.keys(identity.caseInputSha256).sort(), manifest.cases.map(c=>c.id).sort(), 'input digest coverage differs from manifest');
  const sets = new Map();
  for (const target of run.targets) {
    assert.equal(target.status, 'ran', `required target not run: ${target.label}`);
    assert.ok(/^[a-z0-9.-]+$/.test(target.label) && !sets.has(target.label), 'invalid/duplicate target');
    const receipt = join(dir, `${target.label}.jsonl`), runtimeFile = join(dir, `${target.label}.runtime.json`);
    assert.equal(fileSha(receipt), target.receiptSha256, 'receipt changed since execution');
    assert.equal(fileSha(runtimeFile), target.runtimeSha256, 'runtime receipt changed since execution');
    const runtime = JSON.parse(readFileSync(runtimeFile, 'utf8'));
    assert.deepEqual(runtime, target.runtime, 'runtime evidence mismatch');
    assert.equal(runtime.sourceSha256, identity.sourceSha256, 'binary/source binding mismatch');
    assert.equal(runtime.go, target.compiler.version, 'compiler/runtime version mismatch');
    assert.equal(runtime.goos, target.build.GOOS, 'runtime GOOS mismatch');
    assert.equal(runtime.goarch, target.build.GOARCH, 'runtime GOARCH mismatch');
    assert.equal(runtime.settings.CGO_ENABLED, '0', 'unexpected cgo setting');
    assert.equal(runtime.settings['-trimpath'], 'true', 'unexpected trimpath setting');
    assert.equal(runtime.settings['-gcflags'] || '', target.build.flags.find(f => f.startsWith('-gcflags='))?.slice(9) || '', 'diagnostic compiler flag differs');
    if (runtime.goarch === 'amd64') assert.equal(runtime.goamd64, target.build.GOAMD64, 'runtime GOAMD64 mismatch');
    const expected = { label: target.label, ...runtime };
    const rows = readRows(receipt);
    validateRows(rows, manifest, expected, identity.caseInputSha256);
    sets.set(target.label, { rows, target, dir });
  }
  return { run, manifest, identity, sets, runSha256: fileSha(join(dir, 'run.json')) };
}
