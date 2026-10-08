// Local full-output qualification; this never imports or calls the Python oracle.
// Usage: node adaptive-flag-unit-parity.mjs <native> <wasm> <wasm_exec.js> <go>
//   <retained-core-dir> <retained-oracle-dir> <invocation-plan.json>
//   <new-output-dir> [receipt.json]
// Fresh-checkout qualification: <native> <wasm> <wasm_exec.js> <go>
//   --bundle <materialized-evidence-dir> <new-output-dir> [receipt.json]
// The retained core bytes are bound to the independently audited oracle plan.
// Only seven individually verified build-identity leaves may differ. All other
// output tokens, numbers (including signed zero), ordering and whitespace must
// be byte-identical; the original Stage A raw segment is checked separately.
import assert from 'node:assert/strict';
import { emitAdaptiveReceipt } from './adaptive-runtime-receipt.mjs';
import { verifyMaterializedEvidence } from './adaptive-flag-unit-evidence.mjs';
import { createHash, webcrypto } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { tmpdir, platform, arch, release } from 'node:os';
import { basename, join, resolve } from 'node:path';

const [nativePath, wasmPath, shimPath, goPath, ...evidenceArgs] = process.argv.slice(2);
let coreDir, oracleDir, planPath, outputDir, receiptPath, bundle;
if (evidenceArgs[0] === '--bundle') {
  assert(evidenceArgs.length === 3 || evidenceArgs.length === 4, '--bundle requires evidence directory, new output directory and optional receipt');
  bundle = verifyMaterializedEvidence(evidenceArgs[1]);
  ({ coreDir, oracleDir, planPath } = bundle);
  [, , outputDir, receiptPath] = evidenceArgs;
} else {
  assert(evidenceArgs.length === 4 || evidenceArgs.length === 5, 'eight paths and an optional receipt path are required');
  [coreDir, oracleDir, planPath, outputDir, receiptPath] = evidenceArgs;
}
assert(nativePath && wasmPath && shimPath && goPath && coreDir && oracleDir && planPath && outputDir, 'all qualification paths are required');
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
assert.equal(hash(readFileSync(shimPath)), '45ce9dfe7211247544ab6f4268eb8cb5b6f3d5ae602dc3b51447b7eada99c229', 'matching Go1.22.12 shim required');
const readJSON = path => JSON.parse(readFileSync(path, 'utf8'));
const corpusBytes = readFileSync(new URL('../../testsupport/testdata/adaptive-flag-runtime-corpus-v1.json', import.meta.url));
const unitBytes = readFileSync(new URL('../../testsupport/testdata/adaptive-flag-unit-corpus-v1.json', import.meta.url));
assert.equal(hash(corpusBytes), 'af392754f48672cfe31d7161454dfc35ee73977e4c38f0a72b21c15be6fb2f97');
assert.equal(hash(unitBytes), 'cfc59bda0d58a347db1dcd84f38c2ca56a4a77053ce29385a7a70c77c48f7f75');
const corpus = JSON.parse(corpusBytes), units = JSON.parse(unitBytes);
assert.equal(corpus.cases.length, 96); assert.equal(units.cases.length, 288);
assert.equal(new Set(units.cases.map(x => x.id)).size, 288);
const coreReceiptBytes = readFileSync(join(coreDir, 'core-corpus-receipt.json'));
const coreReceipt = JSON.parse(coreReceiptBytes);
assert.equal(coreReceipt.RawCorpusSHA, hash(corpusBytes)); assert.equal(coreReceipt.UnitCorpusSHA, hash(unitBytes));
assert.equal(coreReceipt.Cases.length, 288);
const planBytes = readFileSync(planPath), plan = JSON.parse(planBytes);
assert.equal(hash(planBytes), '9d141c25ead79077685781fec2cd52c0166a09e5ccde07b3f4953cc72c75a161');
assert.equal(plan.cases.length, 306);
// The public pack contains a separately pinned sanitized attestation, never a
// rewritten CHECKPOINT.json. Preserve the original checkpoint identity in the
// receipt and record the derivative identity separately in evidenceBundle.
const checkpointBytes = bundle ? null : readFileSync(join(oracleDir, 'CHECKPOINT.json'));
const checkpointSha256 = bundle ? bundle.attestation.originalCheckpoint.sha256 : hash(checkpointBytes);
const checkpoint = bundle ? {
  invocation_plan_sha256: bundle.attestation.invocationPlanSha256,
  complete_matched_projections: bundle.attestation.originalExecution.completeMatchedProjections,
  case_failures: bundle.attestation.originalExecution.caseFailures,
  artifact_manifest: bundle.attestation.originalArtifactManifest,
} : JSON.parse(checkpointBytes);
assert.equal(checkpointSha256, '7b8096c10736d9f14df725ed0040484fd3a41063e0857e2e4c92fa5ba711e926');
assert.equal(checkpoint.invocation_plan_sha256, hash(planBytes));
assert.equal(checkpoint.complete_matched_projections, 303); assert.equal(checkpoint.case_failures, 0);
const oracleManifestBytes = readFileSync(join(oracleDir, 'ARTIFACTS.json')), oracleManifest = JSON.parse(oracleManifestBytes);
assert.equal(hash(oracleManifestBytes), checkpoint.artifact_manifest.sha256);
assert.equal(Object.keys(oracleManifest.files).length, 611);
const identityKeys = ['goVersion', 'compiler', 'goos', 'goarch', 'vcsRevision', 'vcsModified', 'verified'];
const mapping = readJSON(new URL('./adaptive-flag-unit-reference/field-mapping-v1.json', import.meta.url));
assert.deepEqual(mapping.nativeWasmIdentityAllowlist.map(x => x.path), identityKeys.map(key => '/manifest/buildIdentity/' + key));
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
for (const artifact of [native, wasm]) {
  assert.equal(artifact.goVersion, 'go1.22.12');
  assert.equal(artifact.settings['-compiler'], 'gc');
  assert.equal(artifact.settings.CGO_ENABLED, '0');
  assert.equal(artifact.settings['-trimpath'], 'true');
  assert.equal(artifact.settings['-buildmode'], 'exe');
  assert(!Object.hasOwn(artifact.settings, '-gcflags') && !Object.hasOwn(artifact.settings, '-ldflags'));
}
assert.equal(native.settings.GOOS, platform());
assert.equal(native.settings.GOARCH, { x64: 'amd64', arm64: 'arm64' }[arch()]);
assert.equal(wasm.settings.GOOS, 'js'); assert.equal(wasm.settings.GOARCH, 'wasm');
assert.equal(native.settings['vcs.revision'], wasm.settings['vcs.revision']);
assert.equal(native.settings['vcs.modified'], wasm.settings['vcs.modified']);
function expectedIdentity(artifact) {
  const revision = artifact.settings['vcs.revision'] ?? '';
  const modified = artifact.settings['vcs.modified'];
  assert(revision === '' || /^[a-f0-9]{40}$/.test(revision));
  assert(modified === undefined || modified === 'true' || modified === 'false');
  return { goVersion: artifact.goVersion, compiler: artifact.settings['-compiler'], goos: artifact.settings.GOOS, goarch: artifact.settings.GOARCH, vcsRevision: revision, vcsModified: modified === undefined ? null : modified === 'true', verified: revision !== '' && modified !== undefined };
}
const nativeIdentity = expectedIdentity(native), wasmIdentity = expectedIdentity(wasm);
const evidence = { schema: bundle ? 'adaptive-flag-unit-public-parity-v2' : 'adaptive-flag-unit-public-parity-v1', rawCorpusSha256: hash(corpusBytes), unitCorpusSha256: hash(unitBytes),
  host: { platform: platform(), architecture: arch(), release: release(), node: process.version, v8: process.versions.v8 },
  artifacts: { native, wasm, wasmExecSha256: hash(readFileSync(shimPath)) },
  nativeIdentity, wasmIdentity, retainedCoreIdentity: coreReceipt.Build,
  retainedEvidence: { coreReceiptSha256: hash(coreReceiptBytes), oracleCheckpointSha256: checkpointSha256, oracleManifestSha256: hash(oracleManifestBytes), invocationPlanSha256: hash(planBytes) },
  ...(bundle ? { evidenceBundle: bundle.identity } : {}),
  identityLeafAllowlist: mapping.nativeWasmIdentityAllowlist.map(x => x.path),
  qualification: 'Actual same-host native and Node-Go/WASM execution of 96 frozen invented source cases under three policies. Full nonidentity bytes equal retained core bytes bound by SHA256 to independently audited original oracle comparisons. Zero new oracle calls; no browser, ARM64 unless this host is ARM64, market, capacity or Backtester claim.',
  reports: [], sourceCases: 0, policies: 3, oracleCalls: 0 };

// These offsets select original tokens; neither serialization nor floating-point
// conversion is used to compare the report payloads.
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
    if (i > at) return i;
  }
  throw new Error('invalid JSON token');
}
function members(text, offset = 0) {
  let at = offset;
  const whitespace = () => { while (at < text.length && /\s/.test(text[at])) at++; };
  whitespace(); assert.equal(text[at++], '{');
  const result = new Map();
  for (;;) {
    whitespace(); if (text[at] === '}') return result;
    const keyEnd = valueEnd(text, at), key = JSON.parse(text.slice(at, keyEnd)); at = keyEnd;
    assert.equal(typeof key, 'string'); assert(!result.has(key), 'duplicate decoded member');
    whitespace(); assert.equal(text[at++], ':'); whitespace();
    const start = at, end = valueEnd(text, at); at = end; whitespace();
    result.set(key, { start, end, delimiter: at });
    if (text[at] === '}') return result;
    assert.equal(text[at++], ',');
  }
}
function getMember(text, path) {
  let current = { start: 0 };
  for (const key of path) { current = members(text, current.start).get(key); assert(current, `missing ${path.join('/')}`); }
  return current;
}
function fragment(text, path) { const span = getMember(text, path); return text.slice(span.start, span.end); }
function withoutVerifiedIdentityLeaves(text, prefix, expected) {
  const identityPath = [...prefix, 'manifest', 'buildIdentity'];
  const identity = JSON.parse(fragment(text, identityPath));
  assert.deepEqual(Object.keys(identity), identityKeys, 'identity shape/order drift');
  assert.deepEqual(identity, expected, 'identity is not the observed executing artifact');
  const spans = identityKeys.map(key => getMember(text, [...identityPath, key])).sort((a, b) => b.start - a.start);
  for (const span of spans) text = text.slice(0, span.start) + 'null' + text.slice(span.end);
  return text;
}
function inspect(text, unit, expectedRaw, identity) {
  const document = JSON.parse(text);
  assert.deepEqual(Object.keys(document), ['schema', 'projectionRequest', 'projectionRequestSha256', 'rawEnvelopeSha256', 'raw', 'projection']);
  assert.equal(document.schema, 'strat-adaptive-volume-flag-unit-runtime-v1');
  assert.deepEqual(document.projectionRequest, JSON.parse(unit.projectionMetadataJSON));
  assert.equal(document.projectionRequestSha256, unit.projectionMetadataSha256);
  assert.equal(document.rawEnvelopeSha256, hash(expectedRaw));
  const rawSpan = getMember(text, ['raw']);
  const rawSegment = Buffer.from(text.slice(rawSpan.start, rawSpan.delimiter), 'utf8');
  assert(rawSegment.equals(expectedRaw), `${unit.id}: original raw byte segment including newline changed`);
  assert.equal(document.raw.rawOnly, true); assert.equal(document.raw.economics, 'unavailable-stage-a');
  assert.equal(document.projection.schema, 'adaptive-unit-projection-go-v1');
  assert.equal(document.projection.manifest.rawEnvelopeSha256, hash(expectedRaw));
  assert(text.endsWith('}\n') && !text.endsWith('\n\n'));
  return { document, projection: fragment(text, ['projection']), comparable: withoutVerifiedIdentityLeaves(text, ['projection'], identity) };
}
function oracleArtifact(name) {
  assert.equal(basename(name), name);
  const entry = oracleManifest.files[name]; assert(entry, 'missing retained oracle artifact');
  const bytes = readFileSync(join(oracleDir, name));
  assert.equal(bytes.length, entry.bytes); assert.equal(hash(bytes), entry.sha256);
  return { bytes, sha256: entry.sha256 };
}
// Self-controls prove the comparison cannot discard an ordinary economic token,
// signed zero, raw whitespace, extra identity field or wrong artifact identity.
{
  const base = JSON.stringify({ manifest: { buildIdentity: nativeIdentity }, amount: -1 });
  const expected = withoutVerifiedIdentityLeaves(base, [], nativeIdentity);
  assert.notEqual(withoutVerifiedIdentityLeaves(base.replace('"amount":-1', '"amount":-0'), [], nativeIdentity), expected);
  assert.notEqual(withoutVerifiedIdentityLeaves(base.replace('"amount":-1', '"amount":0'), [], nativeIdentity), withoutVerifiedIdentityLeaves(base.replace('"amount":-1', '"amount":-0'), [], nativeIdentity));
  assert.throws(() => withoutVerifiedIdentityLeaves(base, [], { ...nativeIdentity, goarch: 'other' }));
  assert.throws(() => withoutVerifiedIdentityLeaves(base.replace('"buildIdentity":{', '"buildIdentity":{"extra":0,'), [], nativeIdentity));
}
globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(resolve(shimPath));
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject);
void go.run(instance);
assert.equal(typeof globalThis.engineRunAdaptiveFlagUnitReport, 'function');
assert.equal(typeof globalThis.engineRunAdaptiveFlagReport, 'function');
mkdirSync(outputDir); // Exclusive run directory: never overwrite prior evidence.
const scratch = mkdtempSync(join(tmpdir(), 'adaptive-unit-parity-'));
const requestPath = join(scratch, 'request.json'), projectionPath = join(scratch, 'projection.json'), sourcePath = join(scratch, 'source.strat'), barsPath = join(scratch, 'bars.btb1');
try {
  for (let i = 0; i < corpus.cases.length; i++) {
    const item = corpus.cases[i], source = corpus.sources[item.sourceSha256].text;
    const bytes = new Uint8Array(Buffer.from(corpus.assets[item.btb1Sha256].payload, 'base64'));
    assert.equal(hash(bytes), item.btb1Sha256); assert.equal(hash(source), item.sourceSha256); assert.equal(hash(item.metadataJSON), item.metadataSha256);
    writeFileSync(requestPath, item.metadataJSON); writeFileSync(sourcePath, source); writeFileSync(barsPath, bytes);
    // execFileSync refuses every nonzero exit, regardless of any partial stdout.
    const rawNative = execFileSync(resolve(nativePath), ['adaptive-flag-runtime-report', `--request-file=${requestPath}`, `--dsl-file=${sourcePath}`, `--bars-file=${barsPath}`], { maxBuffer: 128 * 1024 * 1024 });
    const rawWasm = Buffer.from(globalThis.engineRunAdaptiveFlagReport(item.metadataJSON, source, bytes), 'utf8');
    assert(rawNative.equals(rawWasm), `${item.id}: fresh Stage A native/WASM byte mismatch`);
    for (let j = 0; j < 3; j++) {
      const index = 3 * i + j, unit = units.cases[index], retained = coreReceipt.Cases[index], planned = plan.cases[index];
      assert.equal(unit.rawCaseId, item.id); assert.equal(retained.id, unit.id); assert.equal(planned.id, unit.id);
      assert.equal(planned.domain, 'source-backed-runtime-report');
      assert.equal(hash(unit.projectionMetadataJSON), unit.projectionMetadataSha256);
      assert.equal(retained.projectionRequestSha256, unit.projectionMetadataSha256);
      assert.equal(retained.rawSha256, hash(rawNative)); assert.equal(planned.raw_sha256, hash(rawNative));
      assert.equal(basename(retained.rawFile), retained.rawFile); assert.equal(basename(retained.projectionFile), retained.projectionFile);
      const retainedRaw = readFileSync(join(coreDir, retained.rawFile)), coreBytes = readFileSync(join(coreDir, retained.projectionFile));
      assert(retainedRaw.equals(rawNative)); assert.equal(hash(coreBytes), retained.projectionSha256);
      assert.equal(hash(coreBytes), planned.go_projection_sha256, 'core is not exact audited oracle input');
      const oracleIndex = String(index).padStart(3, '0');
      const reference = oracleArtifact(`${oracleIndex}-reference.json`), comparison = oracleArtifact(`${oracleIndex}-comparison.json`);
      const comparisonDoc = JSON.parse(comparison.bytes);
      assert.equal(comparisonDoc.pass, true); assert.deepEqual(comparisonDoc.mismatches, []);
      writeFileSync(projectionPath, unit.projectionMetadataJSON);
      const nativeBytes = execFileSync(resolve(nativePath), ['adaptive-flag-unit-report', `--request-file=${requestPath}`, `--projection-file=${projectionPath}`, `--dsl-file=${sourcePath}`, `--bars-file=${barsPath}`], { maxBuffer: 64 * 1024 * 1024 });
      const wasmText = globalThis.engineRunAdaptiveFlagUnitReport(item.metadataJSON, unit.projectionMetadataJSON, source, bytes);
      assert.equal(typeof wasmText, 'string');
      const nativeText = nativeBytes.toString('utf8'), wasmBytes = Buffer.from(wasmText, 'utf8');
      const n = inspect(nativeText, unit, rawNative, nativeIdentity), w = inspect(wasmText, unit, rawWasm, wasmIdentity);
      assert.equal(n.comparable, w.comparable, `${unit.id}: complete nonidentity output byte mismatch`);
      const coreText = coreBytes.toString('utf8'); assert(coreText.endsWith('\n') && !coreText.endsWith('\n\n'));
      assert.equal(withoutVerifiedIdentityLeaves(n.projection, [], nativeIdentity), withoutVerifiedIdentityLeaves(coreText.slice(0, -1), [], coreReceipt.Build), `${unit.id}: full retained core projection mismatch`);
      const filePrefix = String(index).padStart(3, '0');
      const nativeFile = `${filePrefix}-native.json`, wasmFile = `${filePrefix}-wasm.json`;
      writeFileSync(join(outputDir, nativeFile), nativeBytes, { flag: 'wx' });
      writeFileSync(join(outputDir, wasmFile), wasmBytes, { flag: 'wx' });
      assert(readFileSync(join(outputDir, nativeFile)).equals(nativeBytes)); assert(readFileSync(join(outputDir, wasmFile)).equals(wasmBytes));
      evidence.reports.push({ id: unit.id, rawCaseId: item.id, projectionRequestSha256: unit.projectionMetadataSha256, rawEnvelopeSha256: hash(rawNative), nativeSha256: hash(nativeBytes), wasmSha256: hash(wasmBytes), nativeFile, wasmFile, nativeBytes: nativeBytes.length, wasmBytes: wasmBytes.length, nonidentitySha256: hash(n.comparable), coreProjectionSha256: hash(coreBytes), oracleReferenceSha256: reference.sha256, oracleComparisonSha256: comparison.sha256, fullNonidentityBytesEqual: true, originalRawBytesEqual: true, fullCoreProjectionEqual: true });
    }
    evidence.sourceCases++;
  }
  assert.equal(evidence.sourceCases, 96); assert.equal(evidence.reports.length, 288);
  assert.equal(new Set(evidence.reports.map(x => x.id)).size, 288);
  evidence.status = 'PASS';
  await emitAdaptiveReceipt(evidence, receiptPath);
} finally { rmSync(scratch, { recursive: true, force: true }); }
process.exit(0);
