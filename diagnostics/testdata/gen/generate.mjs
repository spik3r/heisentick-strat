// Regenerates ../<fixture>.expected.json from ../<fixture>.json by running
// the JS derivation the Go package ports. The functions below are copied
// verbatim from the app repo:
//
//   yearlyShape  — scripts/lib/yearlyStability.mjs
//   sliceStats   — scripts/strategy/overfitReport.mjs
//   deriveRow    — the per-strategy loop body of overfitReport.mjs
//                  (`for (const id of ids) { ... out.push({...}) }`), with the
//                  compare rows, recentFrom and MIN_TRADES passed as arguments
//                  instead of read from module scope.
//
// Fixture to compare-row mapping (what strategyCompare --aggregate=strategy
// --json-only=1 would have produced from the same numbers):
//
//   row.trades        = doc.costs[doc.primaryCost.index].trades
//   row.pf            = doc.costs[doc.primaryCost.index].pf     (null stays null)
//   row.net           = doc.costs[doc.primaryCost.index].net
//   row.sliceSummary  = doc.slices with primary trades > 0 or sourceTimeframe set,
//                       each { net: slice.costs[primary].net }
//   row.yearly        = yearlyShape(doc.groupings.year as { year, net, profitFactor },
//                                   doc.dateBounds.lastT)
//
// A null recent or harsh document is a strategy the compare pass returned no
// row for (`recentM.get(id)` undefined).
//
// Run: node diagnostics/testdata/gen/generate.mjs
// Needs Node 18+; no dependencies.

import { readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const testdata = join(here, '..');

// --- verbatim: scripts/lib/yearlyStability.mjs -----------------------------

function yearlyShape(rows, dataEndT) {
  if (!rows.length) {
    return {
      years: 0, positiveYears: 0, worstYearPf: 0, worstYearNet: 0, recentYear: null, recentYearNet: 0,
      completedYears: 0, positiveCompletedYears: 0, worstCompletedYearPf: 0, worstCompletedYearNet: 0,
      recentCompletedYear: null, recentCompletedYearNet: 0, partialYear: null, partialYearNet: null,
    };
  }
  const trailing = rows[rows.length - 1];
  const end = Number(dataEndT);
  // Complete when the data runs past 1 January of the following year.
  const trailingComplete = Number.isFinite(end) && end >= Date.UTC(trailing.year + 1, 0, 1);
  const completed = trailingComplete ? rows : rows.slice(0, -1);
  const worst = rows.reduce((a, b) => (b.net < a.net ? b : a), rows[0]);
  const worstCompleted = completed.length
    ? completed.reduce((a, b) => (b.net < a.net ? b : a), completed[0])
    : null;
  const recentCompleted = completed.length ? completed[completed.length - 1] : null;
  return {
    years: rows.length,
    positiveYears: rows.filter((r) => r.net > 0).length,
    worstYearPf: worst.profitFactor,
    worstYearNet: worst.net,
    recentYear: trailing.year,
    recentYearNet: trailing.net,
    completedYears: completed.length,
    positiveCompletedYears: completed.filter((r) => r.net > 0).length,
    worstCompletedYearPf: worstCompleted ? worstCompleted.profitFactor : 0,
    worstCompletedYearNet: worstCompleted ? worstCompleted.net : 0,
    recentCompletedYear: recentCompleted ? recentCompleted.year : null,
    recentCompletedYearNet: recentCompleted ? recentCompleted.net : 0,
    partialYear: trailingComplete ? null : trailing.year,
    partialYearNet: trailingComplete ? null : trailing.net,
  };
}

// --- verbatim: scripts/strategy/overfitReport.mjs --------------------------

// Best-slice contribution: share of total positive-slice net carried by the
// single largest positive slice. High share => result leans on one slice.
function sliceStats(row) {
  const slices = Array.isArray(row.sliceSummary) ? row.sliceSummary : [];
  if (!slices.length) return { count: 0, positiveRatio: 0, bestSharePct: 0 };
  const positives = slices.filter((s) => s.net > 0);
  const posNet = positives.reduce((a, s) => a + s.net, 0);
  const bestNet = positives.reduce((a, s) => Math.max(a, s.net), 0);
  return {
    count: slices.length,
    positiveRatio: positives.length / slices.length,
    bestSharePct: posNet > 0 ? (bestNet / posNet) * 100 : 0,
  };
}

function deriveRow(id, f, o, h, recentFrom, MIN_TRADES) {
  const ss = sliceStats(f);
  const y = f.yearly || {};
  const fullPf = Number(f.pf) || 0;
  const recentPf = o ? Number(o.pf) || 0 : null;
  const harshPf = h ? Number(h.pf) || 0 : null;
  const harshNet = h ? Number(h.net) || 0 : null;
  const costDeltaPf = (harshPf != null) ? fullPf - harshPf : null;
  const recentDeltaPf = (recentPf != null) ? recentPf - fullPf : null;

  // Each warning carries a stable code alongside its human text. Consumers that
  // gate on a warning read the code; only the console output reads the text.
  const warnings = [];
  const warningCodes = [];
  const warn = (code, text) => { warningCodes.push(code); warnings.push(text); };
  if (ss.bestSharePct > 70) warn('best-slice-concentration', `best slice ${ss.bestSharePct.toFixed(0)}% of net`);
  // Measured against the last COMPLETED calendar year. The trailing partial year
  // is the noisiest row in the table and the only one whose sign can still
  // change, so it is reported as a diagnostic rather than gated on. Payloads
  // without the completed fields fall back to the old trailing-year behaviour.
  const hasCompletedFields = y.completedYears != null;
  const gatedRecentYear = hasCompletedFields ? y.recentCompletedYear : y.recentYear;
  const gatedRecentNet = hasCompletedFields ? y.recentCompletedYearNet : y.recentYearNet;
  const gatedWorstPf = hasCompletedFields ? y.worstCompletedYearPf : y.worstYearPf;
  const gatedYearCount = hasCompletedFields ? y.completedYears : y.years;
  if (gatedRecentYear != null && Number(gatedRecentNet) < 0) {
    warn('recent-year-negative', `recent year (${gatedRecentYear}) negative`);
  }
  if (harshNet != null && harshNet <= 0) warn('harsh-cost-negative', 'harsh cost flips negative');
  if (Number(f.trades) < MIN_TRADES) warn('thin-sample', `thin sample (${f.trades}t < ${MIN_TRADES})`);
  // The wording here still avoids the substring "year". `evaluatePromotionBar`
  // now prefers `warningCodes`, but it keeps the old text match as a fallback
  // for payloads written before codes existed, so a recency warning phrased
  // with "years" would still fail those on the wrong component.
  if (recentPf != null && fullPf > 0 && recentPf < fullPf * 0.7) {
    warn('recent-window-pf-drop', `recent-window PF ${recentPf.toFixed(2)} << full ${fullPf.toFixed(2)} (from ${recentFrom})`);
  }
  if (Number(gatedWorstPf) < 0.5 && Number(gatedYearCount) >= 3) {
    warn('blow-up-year', `blow-up year PF ${Number(gatedWorstPf).toFixed(2)}`);
  }

  return {
    id, trades: f.trades, pf: fullPf, net: Math.round(f.net),
    posSlicePct: Math.round(ss.positiveRatio * 100), bestSlicePct: Math.round(ss.bestSharePct),
    posYears: y.years ? `${y.positiveYears}/${y.years}` : '-', worstYearPf: y.worstYearPf != null ? Number(y.worstYearPf).toFixed(2) : '-',
    recentYearNet: y.recentYearNet != null ? Math.round(y.recentYearNet) : '-',
    completedYears: y.completedYears ?? null,
    worstCompletedYearPf: y.worstCompletedYearPf != null ? Number(y.worstCompletedYearPf).toFixed(2) : '-',
    partialYear: y.partialYear ?? null,
    partialYearNet: y.partialYearNet != null ? Math.round(y.partialYearNet) : null,
    recentFrom,
    recentPf: recentPf != null ? recentPf.toFixed(2) : '-',
    recentDeltaPf: recentDeltaPf != null ? recentDeltaPf.toFixed(2) : '-',
    harshPf: harshPf != null ? harshPf.toFixed(2) : '-', costDeltaPf: costDeltaPf != null ? costDeltaPf.toFixed(2) : '-',
    warnings,
    warningCodes,
  };
}

// --- fixture mapping -------------------------------------------------------

// The JS pipeline reads compare rows from JSON, so every number here goes
// through a stringify/parse round trip, turning any Infinity into null.
function jsonRoundTrip(value) {
  return JSON.parse(JSON.stringify(value));
}

function compareRow(doc) {
  if (!doc) return undefined;
  const index = doc.primaryCost.index;
  const primary = doc.costs[index];
  const sliceSummary = doc.slices
    .filter((slice) => slice.costs[index].trades > 0 || slice.sourceTimeframe != null)
    .map((slice) => ({ net: slice.costs[index].net, trades: slice.costs[index].trades, pf: slice.costs[index].pf }));
  const rows = doc.groupings.year.map((group) => ({
    year: Number(group.key),
    net: group.net,
    profitFactor: group.profitFactor,
  }));
  return jsonRoundTrip({
    strategy: doc.strategy,
    trades: primary.trades,
    pf: primary.pf,
    net: primary.net,
    sliceSummary,
    yearly: yearlyShape(rows, doc.dateBounds ? doc.dateBounds.lastT : null),
  });
}

const fixtures = readdirSync(testdata)
  .filter((name) => name.endsWith('.json') && !name.endsWith('.expected.json'))
  .sort();

for (const name of fixtures) {
  const fixture = JSON.parse(readFileSync(join(testdata, name), 'utf8'));
  const id = name.replace(/\.json$/, '');
  const row = deriveRow(
    id,
    compareRow(fixture.full),
    compareRow(fixture.recent),
    compareRow(fixture.harsh),
    fixture.recentFrom,
    Number(fixture.minTrades),
  );
  writeFileSync(join(testdata, `${id}.expected.json`), `${JSON.stringify(row, null, 2)}\n`);
  console.log(`${id}: ${row.warningCodes.join(', ') || 'no warnings'}`);
}

// toFixed cases the Go jsToFixed helper must reproduce, including exact
// binary ties that Go's round-half-even formatting would print differently.
const toFixedCases = [
  [0, 2], [-0, 2], [-0.001, 2], [0.125, 2], [-0.125, 2], [0.375, 2], [2.5, 0], [3.5, 0], [-2.5, 0],
  [1.005, 2], [70.5, 0], [88.88888888888889, 0], [1.62, 2], [0.7, 2], [1.7999999999999998, 2],
  [0.44999999999999996, 2], [123456.785, 2], [1e-7, 2], [0.5, 0], [0.49999999999999994, 0],
];
writeFileSync(
  join(testdata, 'tofixed.expected.json'),
  `${JSON.stringify(toFixedCases.map(([value, digits]) => ({ value: Object.is(value, -0) ? '-0' : value, digits, text: value.toFixed(digits) })), null, 2)}\n`,
);
console.log(`tofixed: ${toFixedCases.length} cases`);
