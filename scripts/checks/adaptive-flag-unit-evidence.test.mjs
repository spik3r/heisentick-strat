import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { existsSync, linkSync, mkdirSync, mkdtempSync, readFileSync, renameSync, rmSync, symlinkSync, unlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { gzipSync } from 'node:zlib';
import { EVIDENCE_IDENTITY, EVIDENCE_LIMITS, extractEvidencePack, materializeEvidence, readEvidenceManifest, verifyMaterializedEvidence } from './adaptive-flag-unit-evidence.mjs';

const sha = bytes => createHash('sha256').update(bytes).digest('hex');
const magic = Buffer.from('HT195_EVIDENCE_V1\n');
function temporary(fn) {
  const root = mkdtempSync(join(tmpdir(), 'adaptive-unit-evidence-test-'));
  try { return fn(root, join(root, 'new-evidence')); }
  finally { rmSync(root, { recursive: true, force: true }); }
}
function frame(header, payload = Buffer.from('invented\n'), rawHeader) {
  const bytes = Buffer.from(rawHeader ?? JSON.stringify(header));
  const length = Buffer.alloc(4); length.writeUInt32BE(bytes.length);
  return Buffer.concat([length, bytes, payload]);
}
function specimen(paths = ['core/example.json']) {
  const payload = Buffer.from('invented\n');
  const files = paths.map(path => ({ path, bytes: payload.length, sha256: sha(payload), provenance: { kind: 'byte-exact-original', path: 'invented/example.json' } }));
  const manifest = { schema: 'adaptive-flag-unit-evidence-manifest-v1', format: 'ht195-length-prefixed-regular-files-v1+gzip', pack: { file: 'evidence.pack.gz' }, fileCount: files.length, totalFileBytes: files.length * payload.length, files };
  return repack(manifest, Buffer.concat([magic, ...files.map(file => frame({ path: file.path, type: 'file', bytes: file.bytes }, payload))]));
}
function repack(manifest, expanded) {
  const pack = gzipSync(expanded, { level: 9, mtime: 0 });
  manifest.pack = { file: 'evidence.pack.gz', sha256: sha(pack), bytes: pack.length, expandedBytes: expanded.length };
  return { manifest, pack, expanded };
}
function refused(specimen, pattern) {
  temporary((_root, output) => {
    assert.throws(() => extractEvidencePack(specimen.pack, specimen.manifest, output), pattern);
    assert(!existsSync(output), 'invalid archive created an output directory');
  });
}

test('pinned invented bundle materializes all 964 byte-exact declared files and identities', () => temporary((_root, output) => {
  const result = materializeEvidence(output), manifest = readEvidenceManifest();
  assert.deepEqual(result.identity, EVIDENCE_IDENTITY);
  assert.equal(manifest.fileCount, 964);
  assert.equal(manifest.files.filter(file => file.provenance.kind === 'byte-exact-original').length, 963);
  assert.equal(manifest.files.filter(file => /^core\/raw-/.test(file.path)).length, 96);
  assert.equal(manifest.files.filter(file => /^core\/projection-/.test(file.path)).length, 288);
  assert.equal(manifest.files.filter(file => /^oracle\/\d+-reference/.test(file.path)).length, 288);
  assert.equal(manifest.files.filter(file => /^oracle\/\d+-comparison/.test(file.path)).length, 288);
  assert(!existsSync(join(result.oracleDir, 'CHECKPOINT.json')));
  for (const file of manifest.files) {
    const bytes = readFileSync(join(output, file.path));
    assert.equal(bytes.length, file.bytes); assert.equal(sha(bytes), file.sha256);
    assert(!/\/workspace\/|\/home\/|\/Users\/|\/tmp\/|[A-Z]:\\|file:\/\//.test(bytes.toString('utf8')), file.path);
  }
  const plan = JSON.parse(readFileSync(result.planPath));
  const core = JSON.parse(readFileSync(join(result.coreDir, 'core-corpus-receipt.json')));
  const artifacts = JSON.parse(readFileSync(join(result.oracleDir, 'ARTIFACTS.json')));
  assert.equal(plan.cases.length, 306); assert.equal(core.Cases.length, 288);
  assert.equal(Object.keys(artifacts.files).length, 611);
  assert.equal(result.attestation.identity, 'sanitized-derivative-not-original-checkpoint');
  assert.deepEqual(result.attestation.retainedSubset, { sourceCases: 96, policies: 3, reportCases: 288, referenceFiles: 288, comparisonFiles: 288 });
  assert.equal(result.attestation.originalCheckpoint.sha256, '7b8096c10736d9f14df725ed0040484fd3a41063e0857e2e4c92fa5ba711e926');
  for (let i = 0; i < core.Cases.length; i++) {
    const retained = core.Cases[i], planned = plan.cases[i];
    assert.equal(retained.id, planned.id);
    assert.equal(retained.rawSha256, planned.raw_sha256);
    assert.equal(retained.projectionSha256, planned.go_projection_sha256);
    assert.equal(sha(readFileSync(join(result.coreDir, retained.rawFile))), retained.rawSha256);
    assert.equal(sha(readFileSync(join(result.coreDir, retained.projectionFile))), retained.projectionSha256);
    for (const kind of ['reference', 'comparison']) {
      const name = `${String(i).padStart(3, '0')}-${kind}.json`;
      const bytes = readFileSync(join(result.oracleDir, name));
      assert.equal(sha(bytes), artifacts.files[name].sha256); assert.equal(bytes.length, artifacts.files[name].bytes);
    }
  }
  assert.deepEqual(verifyMaterializedEvidence(output).identity, EVIDENCE_IDENTITY);
}));

test('compressed digest is checked before gzip decoding or any output creation', () => {
  const s = specimen(); s.pack = Buffer.alloc(s.pack.length, 0);
  refused(s, /pack digest mismatch/);
});

test('compressed bytes, expanded bytes, file count and headers are bounded', () => {
  for (const change of [
    s => { s.manifest.pack.bytes = EVIDENCE_LIMITS.compressedBytes + 1; },
    s => { s.pack = Buffer.alloc(EVIDENCE_LIMITS.compressedBytes + 1); },
    s => { s.manifest.pack.expandedBytes = EVIDENCE_LIMITS.expandedBytes + 1; },
    s => { s.manifest.fileCount = EVIDENCE_LIMITS.files + 1; },
    s => { s.manifest.files[0].bytes = EVIDENCE_LIMITS.expandedBytes + 1; },
    s => { s.manifest.totalFileBytes = EVIDENCE_LIMITS.expandedBytes + 1; },
    s => { s.manifest.fileCount = -1; },
  ]) { const s = specimen(); change(s); refused(s, /invalid or excessive/); }
  const s = specimen();
  const largeHeader = Buffer.alloc(4); largeHeader.writeUInt32BE(EVIDENCE_LIMITS.headerBytes + 1);
  refused(repack(s.manifest, Buffer.concat([magic, largeHeader])), /header limit/);
  const bomb = Buffer.alloc(EVIDENCE_LIMITS.expandedBytes + 1);
  const bounded = repack(specimen().manifest, bomb);
  bounded.manifest.pack.expandedBytes = EVIDENCE_LIMITS.expandedBytes;
  refused(bounded, /larger than|Cannot create a Buffer|buffer|Buffer/);
});

test('absolute, traversal, slash alias, Windows and noncanonical entry paths are rejected', () => {
  for (const path of ['/outside.json', '../outside.json', 'core/../../outside.json', 'core/./a.json', 'core//a.json', 'core\\a.json', 'C:/outside.json', './core/a.json', 'core/a.json/', 'core/a.json\0', 'core./a.json', 'core/%2e%2e/a.json']) {
    const s = specimen();
    refused(repack(s.manifest, Buffer.concat([magic, frame({ path, type: 'file', bytes: 9 })])), /path/);
    const m = specimen(); m.manifest.files[0].path = path;
    refused(m, /path/);
  }
});

test('symbolic links, hardlinks, directories and unsupported entry metadata are rejected', () => {
  for (const type of ['symlink', 'hardlink', 'directory', 'fifo', 'device']) {
    const s = specimen();
    refused(repack(s.manifest, Buffer.concat([magic, frame({ path: 'core/example.json', type, bytes: 9 })])), /non-regular/);
  }
  const s = specimen();
  refused(repack(s.manifest, Buffer.concat([magic, frame({ path: 'core/example.json', type: 'file', bytes: 9, linkname: '/outside' })])), /unsupported entry fields/);
});

test('duplicate, undeclared, reordered and colliding file paths are rejected', () => {
  refused(specimen(['core/a.json', 'core/a.json']), /duplicate manifest/);
  refused(specimen(['core/z.json', 'core/a.json']), /strictly sorted/);
  refused(specimen(['core/a', 'core/a/b.json']), /collision/);
  const s = specimen(['core/a.json', 'core/b.json']);
  refused(repack(s.manifest, Buffer.concat([magic, frame({ path: 'core/a.json', type: 'file', bytes: 9 }), frame({ path: 'core/a.json', type: 'file', bytes: 9 })])), /duplicate pack/);
  for (const path of ['other.json', 'core/b.json']) {
    const v = specimen();
    refused(repack(v.manifest, Buffer.concat([magic, frame({ path, type: 'file', bytes: 9 })])), /undeclared or reordered/);
  }
});

test('truncation, extra entries, wrong byte counts and mutated payloads fail before writes', () => {
  for (const end of [0, magic.length, magic.length + 2, magic.length + 6, specimen().expanded.length - 1]) {
    const s = specimen(); refused(repack(s.manifest, s.expanded.subarray(0, end)));
  }
  for (const suffix of [Buffer.from('junk'), frame({ path: 'extra.json', type: 'file', bytes: 9 })]) {
    const s = specimen(); refused(repack(s.manifest, Buffer.concat([s.expanded, suffix])), /extra entries or trailing/);
  }
  const mutation = specimen(); mutation.expanded[mutation.expanded.length - 2] ^= 1;
  refused(repack(mutation.manifest, mutation.expanded), /entry digest mismatch/);
  for (const bytes of [8, 10, -1, 1.5]) {
    const s = specimen(); refused(repack(s.manifest, Buffer.concat([magic, frame({ path: 'core/example.json', type: 'file', bytes })])));
  }
  const count = specimen(); count.manifest.pack.expandedBytes--;
  refused(count, /expanded byte count mismatch/);
});

test('duplicate JSON keys and noncanonical header encodings are rejected', () => {
  for (const header of [
    '{"path":"ignored","path":"core/example.json","type":"file","bytes":9}',
    '{ "path":"core/example.json","type":"file","bytes":9}',
    '{"path":"core/example.json","type":"file","bytes":9.0}',
    '{"path":"core\\/example.json","type":"file","bytes":9}',
  ]) {
    const s = specimen(); refused(repack(s.manifest, Buffer.concat([magic, frame(null, undefined, header)])), /noncanonical entry header/);
  }
});

test('materialization requires a new directory and rejects existing output or linked ancestors', () => temporary((root, output) => {
  const s = specimen();
  mkdirSync(output); writeFileSync(join(output, 'preserve'), 'untouched');
  assert.throws(() => extractEvidencePack(s.pack, s.manifest, output), /EEXIST/);
  assert.equal(readFileSync(join(output, 'preserve'), 'utf8'), 'untouched');
  const link = join(root, 'link'); symlinkSync(output, link, 'dir');
  assert.throws(() => extractEvidencePack(s.pack, s.manifest, link), /EEXIST/);
  assert.throws(() => extractEvidencePack(s.pack, s.manifest, join(link, 'nested')), /directory symlinks/);
  const file = join(root, 'file'); writeFileSync(file, 'untouched');
  assert.throws(() => extractEvidencePack(s.pack, s.manifest, file), /EEXIST/);
}));

test('materialized evidence rejects mutations, omissions, extras, links and directory replacement', () => temporary((root, output) => {
  materializeEvidence(output);
  const path = join(output, 'core/raw-000.json'), original = readFileSync(path);
  writeFileSync(path, Buffer.alloc(original.length, 0));
  assert.throws(() => verifyMaterializedEvidence(output), /digest mismatch/);
  writeFileSync(path, original);
  unlinkSync(path); assert.throws(() => verifyMaterializedEvidence(output), /missing materialized file/);
  writeFileSync(path, original);
  const extra = join(output, 'undeclared.json'); writeFileSync(extra, '{}');
  assert.throws(() => verifyMaterializedEvidence(output), /undeclared materialized file/); unlinkSync(extra);
  const directory = join(output, 'undeclared'); mkdirSync(directory);
  assert.throws(() => verifyMaterializedEvidence(output), /undeclared materialized directory/); rmSync(directory, { recursive: true });
  const outside = join(root, 'outside.json'); writeFileSync(outside, original);
  unlinkSync(path); symlinkSync(outside, path);
  assert.throws(() => verifyMaterializedEvidence(output), /materialized symlink/); unlinkSync(path);
  linkSync(outside, path);
  assert.throws(() => verifyMaterializedEvidence(output), /hardlinked/); unlinkSync(path); writeFileSync(path, original);
  const core = join(output, 'core'), moved = join(root, 'moved-core'); renameSync(core, moved); symlinkSync(moved, core, 'dir');
  assert.throws(() => verifyMaterializedEvidence(output), /materialized symlink/); unlinkSync(core); renameSync(moved, core);
  assert.deepEqual(verifyMaterializedEvidence(output).identity, EVIDENCE_IDENTITY);
}));
