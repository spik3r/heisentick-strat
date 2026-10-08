// Qualification-only, immutable invented evidence. No oracle implementation or
// accounting is run here. The public attestation is a named sanitized derivative;
// it never pretends to be the original checkpoint whose digest it records.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { constants, closeSync, fstatSync, lstatSync, mkdirSync, openSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { dirname, join, parse, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { gunzipSync } from 'node:zlib';

export const EVIDENCE_IDENTITY = Object.freeze({
  schema: 'adaptive-flag-unit-evidence-bundle-v1',
  manifestSha256: 'a63f2ed3b78d331d085cff60d441abea2cabd5b2773c611b142dd4d9666a5a9c',
  packSha256: 'b4ebc0067d3bc6309d5a2374e250bbd8bf2119a1672b2e6f4a098f6e3a2b3e59',
  attestationSha256: 'e4ea6158ceb07ef1d172bef899fa04ca309332dfc5105e9650f478a4a16adeeb',
});
export const EVIDENCE_LIMITS = Object.freeze({ compressedBytes: 4 * 1024 * 1024, expandedBytes: 32 * 1024 * 1024, files: 1024, headerBytes: 512, manifestBytes: 512 * 1024 });
const dataURL = new URL('../../testsupport/testdata/adaptive-flag-unit-evidence-v1/', import.meta.url);
const magic = Buffer.from('HT195_EVIDENCE_V1\n');
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const digest = value => assert(typeof value === 'string' && /^[a-f0-9]{64}$/.test(value), 'invalid SHA256');
const integer = (value, limit) => assert(Number.isSafeInteger(value) && value >= 0 && value <= limit, 'invalid or excessive byte/file count');

function canonicalPath(path) {
  assert(typeof path === 'string' && path.length <= 200, 'invalid path');
  assert(/^[A-Za-z0-9][A-Za-z0-9._-]*(?:\/[A-Za-z0-9][A-Za-z0-9._-]*)*$/.test(path), 'noncanonical relative path');
  assert(path.split('/').every(segment => segment !== '.' && segment !== '..' && !segment.endsWith('.')), 'noncanonical path segment');
  return path;
}
function realDirectoryChain(path) {
  const absolute = resolve(path), root = parse(absolute).root;
  let current = root;
  for (const part of absolute.slice(root.length).split('/').filter(Boolean)) {
    current = join(current, part);
    const stat = lstatSync(current);
    assert(stat.isDirectory() && !stat.isSymbolicLink(), 'directory symlinks are forbidden');
  }
}
function readRegularFile(path, limit) {
  const observed = lstatSync(path);
  assert(observed.isFile() && !observed.isSymbolicLink() && observed.nlink === 1, 'only regular files without hardlinks are permitted');
  integer(observed.size, limit);
  const fd = openSync(path, constants.O_RDONLY | constants.O_NOFOLLOW);
  try {
    const stat = fstatSync(fd);
    assert(stat.isFile() && stat.nlink === 1, 'only regular files without hardlinks are permitted');
    assert.equal(stat.dev, observed.dev); assert.equal(stat.ino, observed.ino);
    integer(stat.size, limit);
    const bytes = readFileSync(fd);
    assert.equal(bytes.length, stat.size, 'file changed while reading');
    integer(bytes.length, limit);
    return bytes;
  } finally { closeSync(fd); }
}
function validateManifest(manifest) {
  assert.equal(manifest.schema, 'adaptive-flag-unit-evidence-manifest-v1');
  assert.equal(manifest.format, 'ht195-length-prefixed-regular-files-v1+gzip');
  assert.equal(manifest.pack.file, 'evidence.pack.gz');
  digest(manifest.pack.sha256);
  integer(manifest.pack.bytes, EVIDENCE_LIMITS.compressedBytes);
  integer(manifest.pack.expandedBytes, EVIDENCE_LIMITS.expandedBytes);
  integer(manifest.fileCount, EVIDENCE_LIMITS.files);
  integer(manifest.totalFileBytes, EVIDENCE_LIMITS.expandedBytes);
  assert(Array.isArray(manifest.files));
  assert.equal(manifest.files.length, manifest.fileCount);
  assert(manifest.fileCount > 0);
  const paths = new Set();
  let total = 0, previous = '';
  for (const file of manifest.files) {
    canonicalPath(file.path);
    assert(!paths.has(file.path), 'duplicate manifest path');
    assert(file.path > previous, 'manifest paths must be strictly sorted');
    previous = file.path;
    paths.add(file.path);
    integer(file.bytes, EVIDENCE_LIMITS.expandedBytes); digest(file.sha256);
    total += file.bytes; integer(total, EVIDENCE_LIMITS.expandedBytes);
    if (file.provenance.kind === 'byte-exact-original') canonicalPath(file.provenance.path);
    else {
      assert.equal(file.provenance.kind, 'sanitized-derivative');
      digest(file.provenance.originalCheckpointSha256);
    }
  }
  for (const path of paths) {
    const parts = path.split('/');
    while (parts.length > 1) { parts.pop(); assert(!paths.has(parts.join('/')), 'file/directory path collision'); }
  }
  assert.equal(total, manifest.totalFileBytes);
}
export function readEvidenceManifest() {
  const bytes = readRegularFile(fileURLToPath(new URL('manifest.json', dataURL)), EVIDENCE_LIMITS.manifestBytes);
  assert.equal(hash(bytes), EVIDENCE_IDENTITY.manifestSha256, 'evidence manifest digest mismatch');
  const manifest = JSON.parse(bytes);
  validateManifest(manifest);
  assert.equal(manifest.fileCount, 964);
  assert.equal(manifest.pack.sha256, EVIDENCE_IDENTITY.packSha256);
  assert.equal(manifest.files.find(file => file.path === 'oracle/ATTESTATION.json')?.sha256, EVIDENCE_IDENTITY.attestationSha256);
  return manifest;
}

// Exported for hostile-pack regression tests. Production entry points always
// supply readEvidenceManifest(), which first verifies the pinned manifest digest.
export function extractEvidencePack(packBytes, manifest, outputDir) {
  validateManifest(manifest);
  assert(Buffer.isBuffer(packBytes));
  integer(packBytes.length, EVIDENCE_LIMITS.compressedBytes);
  assert.equal(packBytes.length, manifest.pack.bytes, 'compressed byte count mismatch');
  // This happens before decompression, path parsing, directory creation or writes.
  assert.equal(hash(packBytes), manifest.pack.sha256, 'evidence pack digest mismatch');
  const expanded = gunzipSync(packBytes, { maxOutputLength: EVIDENCE_LIMITS.expandedBytes });
  assert.equal(expanded.length, manifest.pack.expandedBytes, 'expanded byte count mismatch');
  assert(expanded.subarray(0, magic.length).equals(magic), 'invalid pack magic');
  let offset = magic.length;
  const paths = new Set(), entries = [];
  for (const expected of manifest.files) {
    assert(offset + 4 <= expanded.length, 'truncated entry length');
    const headerBytes = expanded.readUInt32BE(offset); offset += 4;
    assert(headerBytes > 0 && headerBytes <= EVIDENCE_LIMITS.headerBytes, 'entry header limit');
    assert(offset + headerBytes <= expanded.length, 'truncated entry header');
    const headerBuffer = expanded.subarray(offset, offset + headerBytes); offset += headerBytes;
    const header = JSON.parse(headerBuffer.toString('utf8'));
    assert(headerBuffer.equals(Buffer.from(JSON.stringify(header))), 'noncanonical entry header');
    assert.deepEqual(Object.keys(header), ['path', 'type', 'bytes'], 'unsupported entry fields');
    canonicalPath(header.path);
    assert.equal(header.type, 'file', 'links and non-regular entries are forbidden');
    assert(!paths.has(header.path), 'duplicate pack path'); paths.add(header.path);
    assert.equal(header.path, expected.path, 'undeclared or reordered pack path');
    integer(header.bytes, EVIDENCE_LIMITS.expandedBytes);
    assert.equal(header.bytes, expected.bytes, 'entry byte count mismatch');
    assert(offset + header.bytes <= expanded.length, 'truncated entry payload');
    const bytes = expanded.subarray(offset, offset + header.bytes); offset += header.bytes;
    assert.equal(hash(bytes), expected.sha256, 'entry digest mismatch');
    entries.push({ path: header.path, bytes });
  }
  assert.equal(offset, expanded.length, 'extra entries or trailing bytes');
  const root = resolve(outputDir);
  realDirectoryChain(dirname(root));
  mkdirSync(root, { mode: 0o700 }); // Exclusive: a prior directory or link fails.
  const directories = new Set([root]);
  for (const entry of entries) {
    const components = entry.path.split('/'); components.pop();
    let directory = root;
    for (const component of components) {
      directory = join(directory, component);
      if (!directories.has(directory)) { mkdirSync(directory, { mode: 0o700 }); directories.add(directory); }
      const stat = lstatSync(directory);
      assert(stat.isDirectory() && !stat.isSymbolicLink(), 'directory symlink during extraction');
    }
    const fd = openSync(join(root, entry.path), constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | constants.O_NOFOLLOW, 0o600);
    try { writeFileSync(fd, entry.bytes); } finally { closeSync(fd); }
  }
}

export function verifyMaterializedEvidence(outputDir) {
  const manifest = readEvidenceManifest(), root = resolve(outputDir);
  realDirectoryChain(root);
  const expected = new Map(manifest.files.map(file => [file.path, file]));
  const expectedDirectories = new Set();
  for (const path of expected.keys()) {
    const parts = path.split('/');
    while (parts.length > 1) { parts.pop(); expectedDirectories.add(parts.join('/')); }
  }
  let count = 0, bytes = 0;
  function visit(relative = '') {
    for (const name of readdirSync(join(root, relative))) {
      const path = relative ? `${relative}/${name}` : name;
      canonicalPath(path);
      const target = join(root, path), stat = lstatSync(target);
      assert(!stat.isSymbolicLink(), 'materialized symlink');
      if (stat.isDirectory()) { assert(expectedDirectories.has(path), 'undeclared materialized directory'); visit(path); }
      else {
        const file = expected.get(path); assert(file, 'undeclared materialized file');
        assert(stat.isFile() && stat.nlink === 1, 'materialized non-regular or hardlinked file');
        assert.equal(stat.size, file.bytes, 'materialized byte count mismatch');
        const content = readRegularFile(target, file.bytes);
        assert.equal(hash(content), file.sha256, 'materialized digest mismatch');
        expected.delete(path); count++; bytes += content.length;
      }
    }
  }
  visit();
  assert.equal(expected.size, 0, 'missing materialized file');
  assert.equal(count, manifest.fileCount); assert.equal(bytes, manifest.totalFileBytes);
  const attestation = JSON.parse(readRegularFile(join(root, 'oracle/ATTESTATION.json'), EVIDENCE_LIMITS.manifestBytes));
  assert.equal(attestation.schema, 'ht195-source-qualification-attestation-v1');
  assert.equal(attestation.identity, 'sanitized-derivative-not-original-checkpoint');
  assert.equal(attestation.originalCheckpoint.sha256, manifest.originalEvidence.oracleCheckpointSha256);
  assert.equal(attestation.originalArtifactManifest.sha256, manifest.originalEvidence.oracleManifestSha256);
  assert.equal(attestation.invocationPlanSha256, manifest.originalEvidence.invocationPlanSha256);
  assert.equal(attestation.coreReceiptSha256, manifest.originalEvidence.coreReceiptSha256);
  return { coreDir: join(root, 'core'), oracleDir: join(root, 'oracle'), planPath: join(root, 'INVOCATION_PLAN.json'), attestation, identity: EVIDENCE_IDENTITY };
}
export function materializeEvidence(outputDir) {
  const manifest = readEvidenceManifest();
  const pack = readRegularFile(fileURLToPath(new URL(manifest.pack.file, dataURL)), EVIDENCE_LIMITS.compressedBytes);
  extractEvidencePack(pack, manifest, outputDir);
  return verifyMaterializedEvidence(outputDir);
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [outputDir, ...extra] = process.argv.slice(2);
  assert(outputDir && !extra.length, 'one new evidence output directory is required');
  const result = materializeEvidence(outputDir);
  process.stdout.write(`${JSON.stringify({ status: 'PASS', ...result.identity, files: 964 })}\n`);
}
