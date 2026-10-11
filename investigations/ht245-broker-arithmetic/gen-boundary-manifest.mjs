#!/usr/bin/env node
// Build one-bar boundary patches from a scaled-price receipt pair.
//   node gen-boundary-manifest.mjs <ref.jsonl> <other.jsonl> <scaled-manifest.json> > out.json
// For each case where the first trade that exits on a stop or target whose
// level differs between the two targets in its bits, move the exit bar's extreme to the larger stop (long: low,
// short: high; opposite for the smaller) so that exactly one target's stop is
// touched on that bar. The entire exit bar is replaced (including open/close). This is an
// invented one-bar edit; it does not claim the original data hit the boundary.
import { readFileSync } from 'node:fs';
import { basename, dirname } from 'node:path';
import assert from 'node:assert/strict';
import { loadRun, fileSha } from './evidence.mjs';
let referenceBundle;
function validatedRows(path, manifestPath) {
  const bundle = loadRun(dirname(path));
  if (referenceBundle) {
    assert.equal(bundle.identity.sourceSha256, referenceBundle.identity.sourceSha256, 'cross-run source differs');
    assert.equal(bundle.identity.toolsSha256, referenceBundle.identity.toolsSha256, 'cross-run harness/comparator differs');
    assert.equal(bundle.identity.inputFilesSha256, referenceBundle.identity.inputFilesSha256, 'cross-run fixture/source inputs differ');
  } else referenceBundle = bundle;
  assert.equal(bundle.run.manifestSha256, fileSha(manifestPath), 'generator manifest differs from executed manifest');
  const set = bundle.sets.get(basename(path, '.jsonl'));
  assert.ok(set, 'generator target absent from current run');
  return new Map(set.rows.map(r => [r.id, r]));
}
const [refPath, otherPath, manifestPath] = process.argv.slice(2);
const buf = new DataView(new ArrayBuffer(8));
const f64 = (hex) => { buf.setBigUint64(0, BigInt('0x' + hex)); return buf.getFloat64(0); };
const ref = validatedRows(refPath, manifestPath), other = validatedRows(otherPath, manifestPath);
const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
const out = [];
for (const c of manifest.cases) {
  const a = ref.get(c.id), b = other.get(c.id);
  if (a.error || b.error || !a.trades || !b.trades || a.tradeCount !== b.tradeCount) continue;
  for (let i = 0; i < a.trades.length; i++) {
    const x = a.trades[i], y = b.trades[i];
    if (x.reason !== y.reason || (x.reason !== 'sl' && x.reason !== 'tp')) continue;
    const field = x.reason;
    if (x[field] === y[field] || x.exitIndex !== y.exitIndex) continue;
    // Heuristic only: serialized initial/final equality does not prove raw
    // equality or that an intervening level never moved. Rerun every candidate.
    const initial = field === 'sl' ? 'initialSl' : 'initialTp';
    if (x[initial] !== x[field] || y[initial] !== y[field]) continue;
    const sa = f64(x[field]), sb = f64(y[field]);
    const hi = Math.max(sa, sb), lo = Math.min(sa, sb);
    // long stop: touched when low <= stop; long target: high >= target;
    // short stop: high >= stop; short target: low <= target.
    // Pick the extreme so exactly the target with the more reachable level is touched.
    const long = x.side === 'long';
    // Replace the exit bar by a bar that opens, closes and (for the far side) peaks
    // just inside the level so no gap rule fires, and whose one touching extreme sits
    // exactly on the level that only one target reaches.
    const up = 1 + 1e-6, down = 1 - 1e-6;
    let patch;
    if (field === 'sl') patch = long ? { l: hi, away: hi * up } : { h: lo, away: lo * down };
    else patch = long ? { h: lo, away: lo * down } : { l: hi, away: hi * up };
    patch = patch.l !== undefined
      ? { index: x.exitIndex, o: String(patch.away), h: String(patch.away), c: String(patch.away), l: String(patch.l) }
      : { index: x.exitIndex, o: String(patch.away), l: String(patch.away), c: String(patch.away), h: String(patch.h) };
    out.push({ ...c, id: `${c.id}#boundary-${field}-trade${i}`, patches: [patch],
      note: `exit bar ${x.exitIndex} replaced: one extreme set between the two targets' ${field} levels` });
    break;
  }
}
process.stdout.write(JSON.stringify({ schema: 'ht245.manifest.v1', cases: out }, null, 1) + '\n');
