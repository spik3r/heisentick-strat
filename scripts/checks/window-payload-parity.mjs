// Invented original-byte payloads through native and WASM admission/windowing.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash, webcrypto } from 'node:crypto';
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { createRequire } from 'node:module';
import { resolve, join } from 'node:path';
import { tmpdir } from 'node:os';
const [native, wasm, shim] = process.argv.slice(2).map(value => resolve(value));
globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(shim);
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(readFileSync(wasm), go.importObject);
void go.run(instance);
const temp = mkdtempSync(join(tmpdir(), 'window-parity-'));
const base = { schema: 'market-data-window-request-v1', format: 'json', fromMs: null, toMs: null };
const rows = [[1, 10, 12, 9, 11, 0], [2, 11, 13, 10, 12, 1]];
const binary = Buffer.alloc(16 + rows.length * 48);
[0x31425442, 1, rows.length, 6].forEach((v, i) => binary.writeUInt32LE(v, i * 4));
for (let column = 0; column < 6; column++) rows.forEach((row, i) => binary.writeDoubleLE(row[column], 16 + (column * rows.length + i) * 8));
const cases = [
  ['json', base, JSON.stringify({ bars: rows }), false],
  ['window', { ...base, fromMs: 2, toMs: 2 }, JSON.stringify({ bars: rows }), false],
  ['original-bytes', base, JSON.stringify({ bars: rows }) + ' ', false],
  ['extra-value', base, JSON.stringify({ bars: rows.map(r => [...r, 99]) }), true],
  ['missing-volume', base, JSON.stringify({ bars: rows.map(r => r.slice(0, 5)) }), true],
  ['null-volume', base, JSON.stringify({ bars: rows.map(r => [...r.slice(0, 5), null]) }), true],
  ['string-volume', base, JSON.stringify({ bars: rows.map(r => [...r.slice(0, 5), '1']) }), true],
  ['outside-window-malformed', { ...base, fromMs: 2 }, JSON.stringify({ bars: [rows[0].slice(0, 5), rows[1]] }), true],
  ['duplicate-bars-key', base, '{"bars":[],"bars":[]}', true],
  ['unordered', base, JSON.stringify({ bars: [...rows].reverse() }), true],
  ['binary', { ...base, format: 'btb1' }, binary, false],
  ['binary-trailing', { ...base, format: 'btb1' }, Buffer.concat([binary, Buffer.from([0])]), true],
];
for (const columns of [5, 7]) { const bad = Buffer.from(binary); bad.writeUInt32LE(columns, 12); cases.push([`binary-${columns}-columns`, { ...base, format: 'btb1' }, bad, true]); }
const hashes = [], sha = value => createHash('sha256').update(value).digest('hex');
try {
  for (const [name, request, payload, refused] of cases) {
    const bytes = Buffer.from(payload), raw = JSON.stringify(request);
    writeFileSync(join(temp, 'request.json'), raw); writeFileSync(join(temp, 'payload'), bytes);
    const output = execFileSync(native, ['--market-data-window', join(temp, 'request.json'), join(temp, 'payload')], { encoding: 'utf8' }).trim();
    assert.equal(globalThis.engineReadMarketDataWindow(raw, bytes.toString('base64')), output, name);
    const result = JSON.parse(output); assert.equal(Boolean(result.error), refused, name);
    if (!refused) { assert.equal(result.payloadSha256, sha(bytes)); assert.equal(result.requestSha256, sha(raw)); }
    hashes.push({ name, refused, outputSha256: sha(output) });
  }
  console.log(JSON.stringify({ schema: 'window-payload-parity-v1', inventedOnly: true, cases: hashes }, null, 2));
} finally { rmSync(temp, { recursive: true, force: true }); }
