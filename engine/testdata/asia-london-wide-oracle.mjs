// Rebuild the frozen whole-strategy oracle from the app checkout:
// node engine/testdata/asia-london-wide-oracle.mjs /absolute/path/to/heisentick-one-engine
import { createHash } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const app = process.argv[2];
if (!app) throw new Error('app checkout path required');
const strategyPath = resolve(app, 'engine/strategies/vpAsiaLondonSweepContinuationFiveMinuteWideAsia.js');
const basePath = resolve(app, 'engine/strategies/vpAsiaLondonSweepContinuation.js');
const enginePath = resolve(app, 'engine/engine.js');
const { default: strategy } = await import(pathToFileURL(strategyPath));
const { runBacktest } = await import(pathToFileURL(enginePath));
const hash = (path) => createHash('sha256').update(readFileSync(path)).digest('hex');

const day = 86_400_000;
const t0 = Date.UTC(2024, 0, 1);
const bar = (d, minute, o, h, l, c, v) => ({ t: t0 + d * day + minute * 60_000, o, h, l, c, v });
const longSource = (d) => [bar(d, 0, 100, 102, 100, 101, 100), bar(d, 180, 100, 102, 100, 101, 100), bar(d, 415, 100, 110, 100, 101, 10)];
const shortSource = (d) => [bar(d, 0, 109, 110, 108, 109, 100), bar(d, 180, 109, 110, 108, 109, 100), bar(d, 415, 109, 110, 100, 109, 10)];
const bars = [];
for (let d = 0; d < 30; d++) {
  bars.push(...longSource(d), bar(d, 420, 109, 109.5, 108, 109, 20));
}
bars.push(...longSource(30), bar(30, 420, 109, 111, 108, 110.5, 20), bar(30, 425, 110.5, 113, 110, 112.7, 20));
bars.push(...shortSource(31), bar(31, 420, 101, 102, 99, 99.5, 20), bar(31, 425, 99.5, 104, 99, 103.8, 20));
bars.push(...longSource(32), bar(32, 420, 107, 111, 106, 110.5, 20)); // target edge too distant
bars.push(...longSource(33), bar(33, 420, 109, 111, 99, 110.5, 20)); // both extremes in first raid
bars.push(...longSource(34), bar(34, 420, 109, 111, 108, 110.5, 20), bar(34, 425, 110.5, 111, 109, 110, 20), bar(34, 960, 110, 111, 109, 110, 20));
bars.push(bar(35, 0, 100, 102, 100, 101, 100), bar(35, 180, 100, 102, 100, 101, 100), bar(35, 415, 100, 108, 100, 101, 10), bar(35, 420, 107, 109, 106, 108.5, 20)); // below historical range threshold
bars.push(...longSource(36), bar(36, 420, 109, 109.5, 99.5, 100, 20), bar(36, 425, 109, 111, 108, 110.5, 20)); // opposing edge raids first

const costs = { slippage: 0, feePerUnit: 0, fillOn: 'close', startEquity: 10_000 };
const result = runBacktest(bars, strategy, { symbol: 'XAUUSD', timeframe: '5m', ctx: Array(bars.length).fill(null), ...costs });
const expected = result.trades.map(({ side, entry, exit, sl, tp, size, entryIndex, exitIndex, entryT, exitT, reason, tag, pnl, meta }) => ({ side, entry, exit, sl, tp, size, entryIndex, exitIndex, entryT, exitT, reason, tag, pnl, meta }));
if (expected.length !== 3) throw new Error(`expected 3 oracle trades, got ${expected.length}`);
const realisticResult = runBacktest(bars, strategy, { symbol: 'XAUUSD', timeframe: '5m', ctx: Array(bars.length).fill(null), ...costs, slippage: 0.06 });
const expectedRealistic = realisticResult.trades.map(({ side, entry, exit, sl, tp, size, entryIndex, exitIndex, entryT, exitT, reason, tag, pnl, meta }) => ({ side, entry, exit, sl, tp, size, entryIndex, exitIndex, entryT, exitT, reason, tag, pnl, meta }));
if (expectedRealistic.length !== expected.length) throw new Error('realistic cost changed oracle trade count');
const feeCosts = { ...costs, slippage: 0.06, feePerUnit: 0.25 };
const feeResult = runBacktest(bars, strategy, { symbol: 'XAUUSD', timeframe: '5m', ctx: Array(bars.length).fill(null), ...feeCosts });
const feeTrades = feeResult.trades.map(({ side, entry, exit, sl, tp, size, entryIndex, exitIndex, entryT, exitT, reason, tag, pnl, meta }) => ({ side, entry, exit, sl, tp, size, entryIndex, exitIndex, entryT, exitT, reason, tag, pnl, meta }));
if (feeTrades.length !== expected.length) throw new Error('fees changed oracle trade count');
const closedEquityCurve = bars.map((_, i) => feeCosts.startEquity + feeTrades.reduce((cash, trade) => cash - (trade.entryIndex <= i ? feeCosts.feePerUnit * trade.size : 0) + (trade.exitIndex <= i ? trade.pnl : 0), 0));
const cashEndEquity = closedEquityCurve.at(-1);
const fixture = {
  schema: 'heisentick-strat/authored-js-vp-asia-london-wide/v1',
  provenance: { strategyId: strategy.id, strategySha256: hash(strategyPath), inheritedBaseSha256: hash(basePath), engineSha256: hash(enginePath), generator: 'engine/testdata/asia-london-wide-oracle.mjs', costs },
  bars, expected, expectedRealistic,
  expectedWithFees: { trades: feeTrades, equityCurve: feeResult.equityCurve, closedEquityCurve, cashEndEquity },
};
writeFileSync(resolve(import.meta.dirname, 'asia-london-wide-whole-strategy.json'), JSON.stringify(fixture, null, 2) + '\n');
const input = {
  schema: 'dsl-conformance-run-fixture-v1', case: 'authored-vp-asia-london-wide-fees-v1',
  strategyId: strategy.id, symbol: 'XAUUSD', timeframe: '5m', rangeMethod: 'zone',
  costs: { ...feeCosts, slippageBps: 0 },
  bars: bars.map(({ t, o, h, l, c, v }) => [t, o, h, l, c, v]),
};
writeFileSync(resolve(import.meta.dirname, 'asia-london-wide-interactive.fixture.json'), JSON.stringify(input, null, 2) + '\n');
