#!/usr/bin/env node
// Compare harness receipts across targets bit for bit.
//   node compare.mjs <receipt-dir> [--ref <label>] [--json out.json]
// Classes per case and target pair: identical, pnl-only (only the reported pnl
// bits differ; decisions, prices and sizes identical), path (an entry, exit,
// stop, target, size or points bit differs), decision (trade count, side,
// index or reason differs), error (error text differs).
import { readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

const args = process.argv.slice(2);
const dir = args[0];
const refIdx = args.indexOf('--ref');
const jsonIdx = args.indexOf('--json');
const refLabel = refIdx >= 0 ? args[refIdx + 1] : 'arm64-go1.27.0';
const load = (f) => readFileSync(join(dir, f), 'utf8').trim().split('\n').filter(Boolean).map((l) => JSON.parse(l));
const sets = Object.fromEntries(readdirSync(dir).filter((f) => f.endsWith('.jsonl')).sort().map((f) => [f.slice(0, -6), load(f)]));
if (!sets[refLabel]) { console.error(`reference ${refLabel} missing`); process.exit(2); }

const buf = new DataView(new ArrayBuffer(8));
const f64 = (hex) => { buf.setBigUint64(0, BigInt('0x' + hex)); return buf.getFloat64(0); };
const ordered = (hex) => { const b = BigInt('0x' + hex); return b >> 63n ? -(b & 0x7fffffffffffffffn) : b; };
const ulpDist = (a, b) => { const d = ordered(a) - ordered(b); return d < 0n ? -d : d; };
const tradeFields = ['entry', 'exit', 'sl', 'tp', 'initialSl', 'initialTp', 'size', 'points'];
const decisionFields = ['side', 'entryIndex', 'exitIndex', 'reason', 'rule', 'tag', 'partial'];

function classify(a, b) {
  if ((a.error || '') !== (b.error || '')) return { cls: 'error' };
  if (a.inputSha256 !== b.inputSha256) return { cls: 'input-mismatch' };
  if (a.tradeCount !== b.tradeCount) return { cls: 'decision', note: 'trade count' };
  const diffs = [];
  let cls = 'identical';
  for (let i = 0; i < (a.trades || []).length; i++) {
    const x = a.trades[i], y = b.trades[i];
    for (const k of decisionFields) if (x[k] !== y[k]) return { cls: 'decision', note: `trade ${i} ${k}` };
    for (const k of tradeFields) if (x[k] !== y[k]) { cls = 'path'; diffs.push({ trade: i, field: k, a: x[k], b: y[k] }); }
    if (x.pnl !== y.pnl) {
      if (cls === 'identical') cls = 'pnl-only';
      diffs.push({ trade: i, field: 'pnl', a: x.pnl, b: y.pnl, aText: x.pnlText, bText: y.pnlText,
        ulps: String(ulpDist(x.pnl, y.pnl)), signChange: Math.sign(f64(x.pnl)) !== Math.sign(f64(y.pnl)) });
    }
  }
  return { cls, diffs };
}

const summary = { reference: refLabel, targets: {} };
for (const [label, rows] of Object.entries(sets)) {
  if (label === refLabel) continue;
  const byId = new Map(rows.map((r) => [r.id, r]));
  const t = { cases: 0, missing: 0, counts: {}, differingCases: [], pnlTradesDiffering: 0, pnlTradesCompared: 0, maxUlps: '0', signChanges: 0 };
  for (const a of sets[refLabel]) {
    const b = byId.get(a.id);
    if (!b) { t.missing++; continue; }
    t.cases++;
    t.pnlTradesCompared += a.tradeCount;
    const r = classify(a, b);
    t.counts[r.cls] = (t.counts[r.cls] || 0) + 1;
    if (r.cls !== 'identical') {
      const pn = (r.diffs || []).filter((d) => d.field === 'pnl');
      t.pnlTradesDiffering += pn.length;
      for (const d of pn) { if (BigInt(d.ulps) > BigInt(t.maxUlps)) t.maxUlps = d.ulps; if (d.signChange) t.signChanges++; }
      t.differingCases.push({ id: a.id, class: r.cls, note: r.note, firstDiffs: (r.diffs || []).slice(0, 3) });
    }
  }
  summary.targets[label] = t;
}
for (const [label, t] of Object.entries(summary.targets)) {
  console.log(`${label.padEnd(20)} vs ${refLabel}: cases=${t.cases} ${JSON.stringify(t.counts)} pnlTrades=${t.pnlTradesDiffering}/${t.pnlTradesCompared} maxUlps=${t.maxUlps} signChanges=${t.signChanges}`);
  for (const c of t.differingCases.slice(0, 12)) console.log(`    ${c.class} ${c.id} ${c.note || ''}`.trimEnd());
}
if (jsonIdx >= 0) writeFileSync(args[jsonIdx + 1], JSON.stringify(summary, null, 1) + '\n');
