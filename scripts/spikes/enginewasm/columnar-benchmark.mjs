import { createHash } from 'node:crypto';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const [wasmPath, wasmExecPath, dataPath, appRoot, requestedSize] = process.argv.slice(2);
if (!wasmPath || !wasmExecPath || !dataPath || !appRoot) throw new Error('usage: node columnar-benchmark.mjs engine.wasm wasm_exec.js XAUUSD-5m.bin app-root');
await import(pathToFileURL(resolve(wasmExecPath)).href);
const data = readFileSync(dataPath);
const bytes = data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength);
const view = new DataView(bytes);
if (view.getUint32(0, true) !== 0x31425442 || view.getUint32(4, true) !== 1 || view.getUint32(12, true) !== 6) throw new Error('expected BBT1 v1 six-column input');
const count = view.getUint32(8, true);
const columns = ['t', 'o', 'h', 'l', 'c', 'v'].map((_, i) => new Float64Array(bytes, 16 + i * count * 8, count));
const source = ['dsl v7', 'strategy "T-F0 XAUUSD 5m ORB measurement" {', 'description "Bridge measurement input."', '}', 'market conditions {', 'slices(XAUUSD 5m)', 'trade window unrestricted', 'day type in (trending, ranging, choppy)', 'trendiness below 999', '}', 'setup {', 'type: opening range breakout', 'opening range every 3 hours UTC', 'break beyond range edge', 'hold 1 candle', '}', 'filters {', 'side both', '}', 'risk {', 'stop 10 pips', '}', 'target {', 'target 20 pips', '}', 'management {', 'move stop to breakeven after 999R plus 0 ATR', '}', 'execution {', 'risk: 200 USD', '}'].join('\n');
const meta = JSON.stringify({ schema: 'enginewasm-columnar-v1', case: 't-f0-xauusd-5m', strategyId: 't-f0-xauusd-5m', symbol: 'XAUUSD', timeframe: '5m', rangeMethod: 'zone', costs: { fillOn: 'close', startEquity: 10000 } });
const wasm = readFileSync(wasmPath);
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(wasm, go.importObject);
void go.run(instance);
const { barsViewFromCols } = await import(pathToFileURL(resolve(appRoot, 'engine/chartDataView.js')).href);
const { buildContextColumns, contextCursorFromColumns, runBacktest } = await import(pathToFileURL(resolve(appRoot, 'engine/engine.js')).href);
const { createSpecStrategy } = await import(pathToFileURL(resolve(appRoot, 'engine/dsl/specStrategy.js')).href);
const jsStrategy = createSpecStrategy(source);
function reconstructTrades(values, stringsJSON) {
  const text = JSON.parse(stringsJSON);
  return text.map((item, index) => {
    const at = index * 13;
    return { entry: values[at], entryIndex: values[at + 1], entryT: values[at + 2], exit: values[at + 3], exitIndex: values[at + 4], exitT: values[at + 5], initialSl: item.noStop ? null : values[at + 6], initialTp: item.noTarget ? null : values[at + 7], pnl: values[at + 8], points: values[at + 9], size: values[at + 10], sl: item.noStop ? null : values[at + 11], tp: item.noTarget ? null : values[at + 12], side: item.side, reason: item.reason, tag: item.tag, meta: item.meta, partial: item.partial };
  });
}
function comparableTrades(trades) {
  return trades.map(({ positionId, partial = false, ...trade }) => ({ ...trade, pnl: Math.round(trade.pnl * 1e9) / 1e9, points: Math.round(trade.points * 1e9) / 1e9, partial }));
}
const sizes = requestedSize ? [Number(requestedSize)] : [50_000, 200_000, 400_000];
if (sizes.some((size) => !Number.isInteger(size) || size <= 0)) throw new Error('requested size must be a positive integer');
const results = { input: { path: resolve(dataPath), sha256: createHash('sha256').update(data).digest('hex'), count, sourceSha256: createHash('sha256').update(source).digest('hex') }, cases: [] };
for (const size of sizes) {
  if (size > count) { results.cases.push({ size, status: 'unavailable', reason: `real input has ${count} bars` }); continue; }
  console.error(`T-F0 measuring ${size} real bars`);
  const slice = Object.fromEntries(['t', 'o', 'h', 'l', 'c', 'v'].map((key, i) => [key, columns[i].subarray(0, size)]));
  const wasmStart = performance.now();
  const output = globalThis.engineRunColumns(meta, source, slice.t, slice.o, slice.h, slice.l, slice.c, slice.v);
  const wasmWallMs = performance.now() - wasmStart;
  if (!output.ok) throw new Error(output.error);
  const bars = barsViewFromCols(slice);
  const contextStart = performance.now();
  const ctx = contextCursorFromColumns(buildContextColumns(bars, { tickSize: 0.1, range: { method: 'zone' } }));
  const contextMs = performance.now() - contextStart;
  const runStart = performance.now();
  const js = runBacktest(bars, { name: jsStrategy.name, params: jsStrategy.params || {}, onBar: jsStrategy.onBar }, { ctx, startEquity: 10000, fillOn: 'close', timeframe: '5m', symbol: 'XAUUSD' });
  const jsRunMs = performance.now() - runStart;
  const reconstructed = reconstructTrades(output.trades, output.stringsJSON);
  assert.deepEqual(comparableTrades(reconstructed), comparableTrades(js.trades), 'full reconstructed columnar trades must equal the app runtime output except app-only positionId and 1e-9 derived-metric rounding');
  console.error(`T-F0 completed ${size} real bars`);
  results.cases.push({ size, status: 'measured', wasmWallMs, wasm: output.timings, typedTradeCount: output.trades.length / 13, js: { contextMs, runMs: jsRunMs, tradeCount: js.trades.length }, fullTradeEquality: true });
}
console.log(JSON.stringify(results, null, 2));
process.exit(0);
