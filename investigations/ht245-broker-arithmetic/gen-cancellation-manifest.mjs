#!/usr/bin/env node
// Derive fee-injection variants of the unchanged frozen run corpus.
//   node gen-cancellation-manifest.mjs <baseline.jsonl> <frozen-manifest.json> > out.json
// For each case, pick the trade with the largest |points*size| and set
// feePerUnit to that trade's points exactly (A), one ulp above (B) and one ulp
// below (C). Fee, size and the exit are unchanged by the fee, so the trade's
// pnl is points*size - fee*size with the two products nearly or exactly equal:
// the case where fused and separately rounded products differ most visibly.
// Bars, strategy source and every other cost stay the frozen bytes.
import { readFileSync } from 'node:fs';

const [baselinePath, manifestPath] = process.argv.slice(2);
const buf = new DataView(new ArrayBuffer(8));
const f64 = (hex) => { buf.setBigUint64(0, BigInt('0x' + hex)); return buf.getFloat64(0); };
const fromBits = (b) => { buf.setBigUint64(0, b); return buf.getFloat64(0); };
const toBits = (x) => { buf.setFloat64(0, x); return buf.getBigUint64(0); };
const up = (x) => fromBits(toBits(x) + 1n);
const down = (x) => fromBits(toBits(x) - 1n);

const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
const base = new Map(readFileSync(baselinePath, 'utf8').trim().split('\n').map((l) => JSON.parse(l)).map((r) => [r.id, r]));
const cases = [];
for (const c of manifest.cases) {
  const r = base.get(c.id);
  if (!r || !r.trades) continue;
  let best = null;
  r.trades.forEach((t, i) => {
    const points = f64(t.points), size = f64(t.size);
    const score = points * size;
    if (points > 0 && Number.isFinite(score) && (best === null || score > best.score)) best = { i, points, score };
  });
  if (!best) continue;
  for (const [tag, fee] of [['A-exact', best.points], ['B-up1ulp', up(best.points)], ['C-down1ulp', down(best.points)]]) {
    cases.push({ id: `${c.id}#${tag}`, source: c.source, costs: { feePerUnit: String(fee) },
      note: `fee = points of trade ${best.i}${tag === 'A-exact' ? '' : tag === 'B-up1ulp' ? ' + 1ulp' : ' - 1ulp'}` });
  }
}
process.stdout.write(JSON.stringify({ schema: 'ht245.manifest.v1', cases }, null, 1) + '\n');
