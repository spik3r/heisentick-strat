// Oracle generator for the HT-227 legacy Setup-9 port.
//
// Runs the PINNED archived JavaScript (heisentick 48a1761867342494c69c77e7ecce325e10d5d935)
// over invented bars and writes the expected signals and trades to
// ../oracle.json. Nothing here reads market data.
//
//   mkdir pinned && git -C <heisentick checkout> archive 48a1761867342494c69c77e7ecce325e10d5d935 engine strat package.json | tar -x -C pinned
//   node gen.mjs <path-to-pinned> > ../oracle.json
//
// The Go test engine/legacy_setup9_oracle_test.go compares exact float values.
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { pathToFileURL } from 'node:url';

const pinned = resolve(process.argv[2] ?? '');
const url = (p) => pathToFileURL(join(pinned, p)).href;
const { runBacktest, buildContext } = await import(url('engine/engine.js'));
const { computeTDSetup } = await import(url('engine/indicators.js'));
const raw = (await import(url('engine/strategies/dslTDSequential.js'))).default;
const seasonal = (await import(url('engine/strategies/dslTDSeasonalReversal.js'))).default;

const PROFILES = { 'seq.legacy.setup9.v1': raw, 'seq.legacy.setup9_perf_seasonal.v1': seasonal };
const HOUR = 3600e3, DAY = 24 * HOUR;
const T0 = Date.UTC(2026, 0, 5); // Monday 00:00Z

function mulberry32(a) {
  return () => {
    a |= 0; a = (a + 0x6D2B79F5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}
const round = (x) => Math.round(x * 100) / 100;
const bar = (t, o, h, l, c) => ({ t, o, h, l, c, v: 100 });

// Trace-style bars: o = c - 0.5, h = c + 1, l = c - 1.
const traceBars = (closes, step = HOUR) => closes.map((c, i) => bar(T0 + i * step, c - 0.5, c + 1, c - 1, c));

function walk({ seed, n, step, skipWeekend, mom, sigma, hourDrift }) {
  const rnd = mulberry32(seed);
  const bars = [];
  let price = 2000, prevRet = 0, t = T0;
  while (bars.length < n) {
    const dow = new Date(t).getUTCDay();
    if (skipWeekend && (dow === 6 || (dow === 0 && new Date(t).getUTCHours() < 22))) { t += step; continue; }
    const hour = new Date(t).getUTCHours();
    const z = (rnd() + rnd() + rnd() - 1.5) * 2;
    const ret = mom * prevRet + sigma * z + (hourDrift[hour] ?? 0);
    prevRet = ret;
    const o = round(price), c = round(price + ret);
    const h = round(Math.max(o, c) + rnd() * sigma * 0.6);
    const l = round(Math.min(o, c) - rnd() * sigma * 0.6);
    bars.push(bar(t, o, h, l, c));
    price = c;
    t += step;
  }
  return bars;
}

const hourDriftA = Object.fromEntries([...Array(24).keys()].map((h) => [h, [9, 10, 11, 12, 13, 14].includes(h) ? 0.9 : [15, 16, 17, 18, 19].includes(h) ? -0.9 : 0]));

// Crafted seasonality boundary scenarios. History days hold two bars (08:00 and
// 09:00 UTC); the 09:00 bar is the bucket the signal reads as "next hour".
// The final day falls (buy 9 -> long) or rises (sell 9 -> short) from 00:00 so
// that Setup 9 completes at 08:00 and the next hour is 09:00.
function boundary({ side, up, down, doji, extraOldBullish = 0, oldStart = 1 }) {
  const days = 100;
  const bars = [];
  const recent = [];
  for (let k = 0; k < up; k++) recent.push('u');
  for (let k = 0; k < down; k++) recent.push('d');
  for (let k = 0; k < doji; k++) recent.push('0');
  // Shuffle deterministically so ordering is not a function of direction.
  const rnd = mulberry32(7 + up * 31 + down * 17 + doji * 5);
  for (let k = recent.length - 1; k > 0; k--) { const j = Math.floor(rnd() * (k + 1)); [recent[k], recent[j]] = [recent[j], recent[k]]; }
  const placed = [];
  // Extra bullish observations sit on days oldStart.., stale (older than 90 days at the signal day) when oldStart is small.
  for (let k = 0; k < extraOldBullish; k++) placed.push({ day: oldStart + k, d: 'u' });
  recent.forEach((d, k) => placed.push({ day: days - 1 - 20 + k * 2 - recent.length, d }));
  const last = days; // index of the signal day
  const byDay = new Map(placed.map((p) => [p.day, p.d]));
  for (let day = 0; day < last; day++) {
    const base = T0 + day * DAY;
    bars.push(bar(base + 8 * HOUR, 100, 100.5, 99.5, 100));
    const d = byDay.get(day);
    if (d === undefined) continue;
    const c = d === 'u' ? 101 : d === 'd' ? 99 : 100;
    const o = d === 'u' ? 99.5 : d === 'd' ? 100.5 : 100;
    bars.push(bar(base + 9 * HOUR, o, Math.max(o, c) + 0.5, Math.min(o, c) - 0.5, c));
  }
  const base = T0 + last * DAY;
  const step = side === 'long' ? -1 : 1;
  for (let h = 0; h <= 8; h++) {
    const c = 100 + step * (h + 1);
    bars.push(bar(base + h * HOUR, c - step * 0.5, c + 1, c - 1, c));
  }
  bars.push(bar(base + 9 * HOUR, 100, 101, 99, 100));
  bars.push(bar(base + 10 * HOUR, 100, 101, 99, 100));
  return bars;
}

const cases = [];
const add = (name, bars, note, tf = '1h', riskUsd = 200) => cases.push({ name, note, bars, tf, riskUsd });

add('trace-a-rise-14', traceBars(Array.from({ length: 14 }, (_, i) => 100 + i)), 'contract 8.1 A');
{
  const closes = Array.from({ length: 14 }, (_, i) => 100 + i); closes[10] = closes[6];
  add('trace-b-equality-14', traceBars(closes), 'contract 8.1 B');
}
add('trace-c-flip-8', traceBars([100, 101, 102, 103, 99, 98, 104, 105]), 'contract 8.1 C');
{
  const bars = traceBars(Array.from({ length: 14 }, (_, i) => 100 + i));
  bars[9].h = 114; bars[10].h = 115; bars[11].h = 115; bars[12].h = 115;
  add('trace-d-perfection-equality-14', bars, 'contract 8.1 D');
}
add('trace-e-rise-300', traceBars(Array.from({ length: 300 }, (_, i) => 100 + i)), 'contract 8.1 E rising');
add('trace-e-fall-300', traceBars(Array.from({ length: 300 }, (_, i) => 1000 - i)), 'contract 8.1 E falling mirror');
add('mirror-up', traceBars([100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 100]), 'sell 9 then reversal');
add('mirror-down', traceBars([100, 99, 98, 97, 96, 95, 94, 93, 92, 91, 90, 89, 88, 87, 100]), 'buy 9 then reversal');
add('walk-1h-continuous', walk({ seed: 11, n: 3000, step: HOUR, skipWeekend: false, mom: 0.55, sigma: 1.2, hourDrift: hourDriftA }), '125 days, past the 90-day window');
add('walk-1h-weekends', walk({ seed: 23, n: 3200, step: HOUR, skipWeekend: true, mom: 0.6, sigma: 1.0, hourDrift: hourDriftA }), 'weekend gaps; counter is index based');
add('walk-15m-weekends', walk({ seed: 37, n: 4200, step: 15 * 60e3, skipWeekend: true, mom: 0.7, sigma: 0.5, hourDrift: Object.fromEntries(Object.entries(hourDriftA).map(([h, v]) => [h, v / 3])) }), '15m bars; current-hour bucket holds earlier bars of the hour', '15m');

const edgeDefs = [
  ['seas-long-60pct-10samples', 'long', 6, 4, 0],
  ['seas-long-60pct-with-dojis', 'long', 6, 4, 3],
  ['seas-long-9samples', 'long', 6, 3, 0],
  ['seas-long-50pct', 'long', 5, 5, 0],
  ['seas-long-40pct', 'long', 4, 6, 0],
  ['seas-long-6of11', 'long', 6, 5, 0],
  ['seas-long-7of11', 'long', 7, 4, 0],
  ['seas-short-40pct-10samples', 'short', 4, 6, 0],
  ['seas-short-9samples', 'short', 3, 6, 0],
  ['seas-short-50pct', 'short', 5, 5, 0],
  ['seas-short-60pct', 'short', 6, 4, 0],
  ['seas-short-5of11', 'short', 5, 6, 0],
];
for (const [name, side, up, down, doji] of edgeDefs) {
  add(name, boundary({ side, up, down, doji }), `next-hour bias ${up} up / ${down} down / ${doji} doji; ${side} setup`);
}
// Old observations beyond the 90-day window must not count. 8 stale bullish
// bars would give a long if kept; the recent 4 up / 6 down must not.
add('seas-window-fresh-bullish', boundary({ side: 'long', up: 4, down: 6, doji: 0, extraOldBullish: 8, oldStart: 40 }), 'the same 8 bullish bars inside the window: long fires');
add('seas-window-stale-bullish', boundary({ side: 'long', up: 4, down: 6, doji: 0, extraOldBullish: 8 }), 'stale bullish history outside 90 days');
add('walk-1h-risk-150', walk({ seed: 5, n: 700, step: HOUR, skipWeekend: false, mom: 0.6, sigma: 1.1, hourDrift: hourDriftA }), 'non-default riskUsd 150 changes sizing only', '1h', 150);

function sha(path) { return createHash('sha256').update(readFileSync(join(pinned, path))).digest('hex'); }

const costVariants = [{ slippage: 0, feePerUnit: 0 }, { slippage: 0.07, feePerUnit: 0.02 }];

function runOne(profileId, bars, costs, riskUsd) {
  const base = PROFILES[profileId];
  const strategy = { ...base, params: { ...base.params, riskUsd } };
  const seasonalCtx = profileId === 'seq.legacy.setup9_perf_seasonal.v1';
  const ctxOpts = { ...(strategy.contextOptions ?? {}), contextRequirements: strategy.contextRequirements };
  const ctx = buildContext(bars, ctxOpts);
  const trades = runBacktest(bars, strategy, { ctx, ...costs, closeAtEnd: true }).trades;
  // Rule decisions on every bar, ignoring position state.
  const signals = [];
  for (let i = 0; i < bars.length; i++) {
    let entered = null;
    const api = {
      i, bar: bars[i], bars, ctx: ctx[i], params: strategy.params, position: null,
      enter(side, o) { entered = { side, sl: o.sl, tp: o.tp, riskUsd: o.riskUsd, tag: o.tag }; return true; },
    };
    strategy.onBar(api);
    if (entered) {
      const next = seasonalCtx ? ctx[i].seasonality?.intraday?.['90d']?.next ?? null : null;
      signals.push({ index: i, ...entered, next });
    }
  }
  return {
    profile: profileId, costs, signals,
    trades: trades.map((t) => ({
      side: t.side, entry: t.entry, exit: t.exit, sl: t.sl, tp: t.tp, size: t.size, entryIndex: t.entryIndex,
      exitIndex: t.exitIndex, entryT: t.entryT, exitT: t.exitT, points: t.points, pnl: t.pnl, reason: t.reason, tag: t.tag,
    })),
  };
}

const out = {
  schema: 'ht227-legacy-setup9-oracle-v1',
  jsCommit: '48a1761867342494c69c77e7ecce325e10d5d935',
  sources: Object.fromEntries([
    'engine/strategies/dslTDSequential.js', 'engine/strategies/dslTDSeasonalReversal.js', 'engine/indicators.js',
    'engine/dsl/predicates.js', 'engine/dsl/strategy.js', 'engine/context/seasonality.js', 'engine/broker.js', 'engine/engine.js',
  ].map((p) => [p, sha(p)])),
  cases: cases.map((c) => ({
    name: c.name, note: c.note, timeframe: c.tf, riskUsd: c.riskUsd,
    bars: c.bars.map((b) => [b.t, b.o, b.h, b.l, b.c, b.v]),
    stored: [...computeTDSetup(c.bars)],
    runs: Object.keys(PROFILES).flatMap((p) => costVariants.map((costs) => runOne(p, c.bars, costs, c.riskUsd))),
  })),
};
process.stdout.write(`${JSON.stringify(out)}\n`);
