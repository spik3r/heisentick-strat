#!/usr/bin/env node
// Build one-bar boundary patches from a scaled-price receipt pair.
//   node gen-boundary-manifest.mjs <ref.jsonl> <other.jsonl> <scaled-manifest.json> > out.json
// For each case where the first trade that exits on a stop or target whose
// level differs between the two targets in its bits, move the exit bar's extreme to the larger stop (long: low,
// short: high; opposite for the smaller) so that exactly one target's stop is
// touched on that bar. Only the exit bar's low/high changes. This is an
// invented one-bar edit; it does not claim the original data hit the boundary.
import { readFileSync } from 'node:fs';
const [refPath, otherPath, manifestPath] = process.argv.slice(2);
const buf = new DataView(new ArrayBuffer(8));
const f64 = (hex) => { buf.setBigUint64(0, BigInt('0x' + hex)); return buf.getFloat64(0); };
const rows = (p) => new Map(readFileSync(p, 'utf8').trim().split('\n').map((l) => JSON.parse(l)).map((r) => [r.id, r]));
const ref = rows(refPath), other = rows(otherPath);
const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
const out = [];
for (const c of manifest.cases) {
  const a = ref.get(c.id), b = other.get(c.id);
  if (!a?.trades || !b?.trades || a.tradeCount !== b.tradeCount) continue;
  for (let i = 0; i < a.trades.length; i++) {
    const x = a.trades[i], y = b.trades[i];
    if (x.reason !== y.reason || (x.reason !== 'sl' && x.reason !== 'tp')) continue;
    const field = x.reason;
    if (x[field] === y[field] || x.exitIndex !== y.exitIndex) continue;
    // Only levels never moved after entry, so the patched bar cannot feed a trail.
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
