import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { tmpdir } from 'node:os';
import { pathToFileURL } from 'node:url';
import { webcrypto } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import assert from 'node:assert/strict';

// Usage: node scripts/checks/basket-report-parity.mjs <native> <wasm> <wasm_exec.js>
// All fixtures are invented. No strategies, market data, or private defaults.
const paths = process.argv.slice(2);
if (paths.length !== 3) throw new Error('Expected native, WASM and matching wasm_exec.js paths.');
const [nativePath, wasmPath, shimPath] = paths.map(path => resolve(path));
if (!globalThis.crypto) globalThis.crypto = webcrypto;
await import(pathToFileURL(shimPath).href);
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject);
go.run(instance);

const trade = (year, pnl, more = {}) => ({
  entryT: Date.UTC(year, 1, 3), entry: 24, size: 5, pnl, initialSl: 20, sl: 23, ...more,
});
const identity = { run: 'invented', nested: { unicode: 'Δ', value: 1 } };
const base = {
  schema: 'basket-report-request-v1', nowMs: Date.UTC(2032, 5, 1), identity,
  legs: [{
    symbol: 'TEST_A', tf: '2h', bars: 3, identity, provenance: { input: 'synthetic' },
    dataSpan: { firstBarT: Date.UTC(2029, 0, 1), lastBarT: Date.UTC(2032, 0, 1) },
    trades: [trade(2030, 40), trade(2031, -20)],
  }],
};
const bar = {
  basket: ['TEST_A'], minAggregateTrades: 1, minAggregatePfR: 2,
  minPerSymbolPfR: 2, minTradesPerYearPerSymbol: 0.1,
  minAggregateHarshExpectancyR: 0.1, minPerSymbolHarshPfR: 2,
  requireAggregateYearStability: true, maxTrailingNegativeYearsPerSymbol: 0,
  requireDslSource: true,
};
const withTrades = trades => ({ ...base, legs: [{ ...base.legs[0], trades }] });
const variants = [
  base,
  { ...base, legs: [] },
  withTrades([]),
  withTrades([trade(2030, 0)]),
  withTrades([trade(2030, 40)]),
  withTrades([trade(2030, 12, { size: 2, partial: true }), trade(2030, 18, { size: 3 })]),
  withTrades([trade(2030, 12, { size: 2, partial: true })]),
  withTrades([trade(2030, 40, { status: 'open' }), trade(2031, -20, { status: 'prefix' })]),
  withTrades([trade(2030, 40, { initialSl: null, sl: null }), trade(2030, 40, { initialSl: 24 })]),
  {
    ...base, bar, sourceIsDsl: true,
    legs: [{ ...base.legs[0], harshIdentity: identity, harshTrades: [trade(2030, 10)] }],
  },
  {
    ...base,
    legs: [base.legs[0], {
      ...base.legs[0], symbol: 'TEST_B',
      trades: [trade(2030, 80, { size: 10 }), trade(2031, -80, { size: 10 })],
    }],
  },
  { ...base, source: 'not executed' },
];
const raws = variants.map(JSON.stringify);
raws.push(JSON.stringify(base).replace('"schema":', '"schema":"duplicate","schema":'), 'null', '');
// Mixed-leg harsh evidence must fail closed even if another leg is profitable.
for (const harshTrades of [[], [trade(2030, 10, { initialSl: null, sl: null })], [trade(2030, 10, { status: 'prefix' })]]) {
  raws.push(JSON.stringify({ ...base, bar, sourceIsDsl: true, legs: [
    { ...base.legs[0], harshTrades },
    { ...base.legs[0], tf: '6h', harshTrades: [trade(2030, 40)] },
  ] }));
}
const partialFallback = [
  trade(2030, 12, { size: 2, partial: true, initialSl: null, sl: 20, exitIndex: 1 }),
  trade(2030, 18, { size: 3, initialSl: null, sl: 23, exitIndex: 2 }),
];
raws.push(JSON.stringify(withTrades(partialFallback)));
raws.push(JSON.stringify(withTrades([partialFallback[0], { ...partialFallback[1], sl: 20 }])));
const mixedPartials = [
  trade(2030, 20), trade(2030, 20),
  trade(2031, 12, { size: 2, partial: true }), trade(2031, 18, { size: 3 }),
];
raws.push(JSON.stringify(withTrades(mixedPartials)));
raws.push(JSON.stringify(withTrades([...mixedPartials, mixedPartials.at(-1)])));
const errorCases = new Set([6, 11, 12, 13, 14, 18, 21]);
const temporary = mkdtempSync(join(tmpdir(), 'basket-report-parity-'));
const requestPath = join(temporary, 'request.json');
try {
  for (let i = 0; i < raws.length; i++) {
    writeFileSync(requestPath, raws[i]);
    const native = JSON.parse(execFileSync(nativePath, ['--basket-report', requestPath], { encoding: 'utf8' }));
    const wasm = JSON.parse(globalThis.engineBuildBasketReport(raws[i]));
    assert.deepEqual(wasm, native, `parity case ${i}`);
    assert.equal(native.schema, 'basket-report-result-v1');
    if (errorCases.has(i)) assert.equal(native.error?.code, 'invalid-request');
    else {
      assert.equal(native.error, undefined, `unexpected error case ${i}`);
      assert.match(native.requestSha256, /^[a-f0-9]{64}$/);
      assert.deepEqual(native.identity, identity);
      if (i >= 15 && i <= 17) {
        assert.equal(native.aggregate.missingHarsh, true);
        assert.equal(native.verdict.components.find(row => row.name === 'harsh-costs').pass, false);
        assert.equal(native.verdict.components.find(row => row.name === 'per-symbol-harsh').pass, false);
      }
      if (i === 19) assert.equal(native.aggregate.sumR, 1.5);
      if (i === 20) {
        assert.equal(native.aggregate.trades, 3);
        assert.equal(native.aggregate.sumR, 3.5);
      }
    }
  }
} finally {
  rmSync(temporary, { recursive: true, force: true });
}
console.log(`Native/WASM basket parity passed: ${raws.length} invented cases.`);
process.exit(0);
