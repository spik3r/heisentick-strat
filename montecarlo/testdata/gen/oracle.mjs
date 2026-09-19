// JS reference oracle for the montecarlo package fixtures.
//
// The functions below (maxDDfrac, shuffleInto, mulberry32, pctl) and the
// sampling loop are copied verbatim from heisentick
// scripts/strategy/monteCarlo.mjs. Only the strategy loading, route
// collection and printing are left out. Rerun with:
//
//   node montecarlo/testdata/gen/oracle.mjs
//
// from the repository root. It rewrites every montecarlo/testdata/*.json
// fixture. Do not hand-edit those files.

import { writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const START_EQUITY = 10_000;

// ---- verbatim from scripts/strategy/monteCarlo.mjs ----------------------

// Max drawdown (as a fraction of running peak) for an equity path built by
// accumulating pnl from a fixed starting equity.
function maxDDfrac(pnls, start = START_EQUITY) {
  let eq = start;
  let peak = start;
  let worst = 0;
  for (const p of pnls) {
    eq += p;
    if (eq > peak) peak = eq;
    const dd = (peak - eq) / peak;
    if (dd > worst) worst = dd;
  }
  return worst;
}

function shuffleInto(dst, src, rnd) {
  for (let i = 0; i < src.length; i++) dst[i] = src[i];
  for (let i = dst.length - 1; i > 0; i--) {
    const j = (rnd() * (i + 1)) | 0;
    const tmp = dst[i]; dst[i] = dst[j]; dst[j] = tmp;
  }
}

// Deterministic PRNG (mulberry32) so runs are reproducible.
function mulberry32(seed) {
  let a = seed >>> 0;
  return function () {
    a |= 0; a = (a + 0x6D2B79F5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function pctl(sorted, q) {
  if (!sorted.length) return NaN;
  const idx = Math.min(sorted.length - 1, Math.max(0, Math.round(q * (sorted.length - 1))));
  return sorted[idx];
}

// The per-strategy block of the script's main loop, with the strategy id,
// routes and provenance removed and START_EQUITY made a parameter so a
// fixture can exercise a different starting equity through the same
// formula.
function monteCarlo(pnls, iters, seed, startEquity) {
  if (!pnls.length) {
    return { trades: 0, warning: 'no trades' };
  }
  const realizedDD = maxDDfrac(pnls, startEquity);
  const realizedNet = pnls.reduce((s, p) => s + p, 0);

  const rnd = mulberry32(seed);
  const buf = new Array(pnls.length);
  const orderDD = [];
  const bootDD = [];
  const bootNet = [];
  for (let k = 0; k < iters; k++) {
    shuffleInto(buf, pnls, rnd);          // permutation: same trades, new order
    orderDD.push(maxDDfrac(buf, startEquity));
    let net = 0;
    for (let i = 0; i < pnls.length; i++) { // bootstrap: sample with replacement
      buf[i] = pnls[(rnd() * pnls.length) | 0];
      net += buf[i];
    }
    bootDD.push(maxDDfrac(buf, startEquity));
    bootNet.push(net);
  }
  orderDD.sort((a, b) => a - b);
  bootDD.sort((a, b) => a - b);
  bootNet.sort((a, b) => a - b);
  return {
    trades: pnls.length,
    iters,
    seed,
    realized: {
      net: realizedNet,
      maxDD: realizedDD,
    },
    orderShuffleMaxDD: {
      p50: pctl(orderDD, .5),
      p95: pctl(orderDD, .95),
      p99: pctl(orderDD, .99),
      worst: orderDD[orderDD.length - 1],
    },
    bootstrapMaxDD: {
      p50: pctl(bootDD, .5),
      p95: pctl(bootDD, .95),
      p99: pctl(bootDD, .99),
      worst: bootDD[bootDD.length - 1],
    },
    bootstrapNet: {
      p5: pctl(bootNet, .05),
      p50: pctl(bootNet, .5),
      p95: pctl(bootNet, .95),
      probNetLeZero: bootNet.filter((n) => n <= 0).length / bootNet.length,
    },
  };
}

// ---- fixture inputs ------------------------------------------------------

// ~200 trades from the same PRNG, seeded independently of the sampling
// seed. 45% losers in [-200, -50), winners in [60, 310), two decimals.
function generatedStream(count, seed) {
  const r = mulberry32(seed);
  const out = [];
  for (let i = 0; i < count; i++) {
    const lose = r() < 0.45;
    const raw = lose ? -(50 + r() * 150) : 60 + r() * 250;
    out.push(Math.round(raw * 100) / 100);
  }
  return out;
}

const cases = [
  {
    name: 'handcrafted-12',
    pnls: [120.5, -80, 45.25, 0, -150.75, 300, -60, 95, -20.5, 210, -310.25, 75],
    startEquity: 10_000,
    iters: 1000,
    seed: 12345,
  },
  {
    name: 'empty',
    pnls: [],
    startEquity: 10_000,
    iters: 1000,
    seed: 12345,
  },
  {
    name: 'single-trade',
    pnls: [250],
    startEquity: 10_000,
    iters: 100,
    seed: 7,
  },
  {
    name: 'generated-200',
    pnls: generatedStream(200, 99),
    startEquity: 25_000,
    iters: 2000,
    seed: 2024,
  },
];

const outDir = join(dirname(fileURLToPath(import.meta.url)), '..');
for (const c of cases) {
  const fixture = {
    input: { pnls: c.pnls, startEquity: c.startEquity, iters: c.iters, seed: c.seed },
    expected: monteCarlo(c.pnls, c.iters, c.seed, c.startEquity),
  };
  const path = join(outDir, `${c.name}.json`);
  writeFileSync(path, `${JSON.stringify(fixture, null, 2)}\n`);
  console.log(`wrote ${path}`);
}
