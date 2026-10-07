// Invented source-plumbing regression plus existing positive corpus controls.
// Usage: node scripts/checks/down-shock-source-parity.mjs \
//   <heisentick> <native enginewasm> <enginewasm.wasm> <wasm_exec.js>
// Exercises real native CLI BTB1 loading and Node-hosted Go/WASM. It makes no
// browser, consumer-adoption, market-data or strategy-performance claim.
import assert from 'node:assert/strict';
import { createHash, webcrypto } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

const [reportPath, nativePath, wasmPath, shimPath] = process.argv.slice(2);
assert(reportPath && nativePath && wasmPath && shimPath, 'four paths are required');
const nativeReport = resolve(reportPath);
const nativeEngine = resolve(nativePath);
globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(resolve(shimPath));
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject);
void go.run(instance);
assert.equal(typeof globalThis.engineRunFixture, 'function');

const source = `dsl v7
strategy "Invented down-shock source plumbing"
market conditions {
  slices(XAUUSD 1m)
}
setup {
  type: down shock rebound
  source timeframe 15m
  entryTf 1m
  shock entry immediate
  shock target 13 bp
}
execution {
  risk 200 USD
}
`;
const minute = 60_000;
const sourceBars = [];
for (let day = Date.UTC(2020, 0, 6, 9, 45); sourceBars.length < 461; day += 24 * 60 * minute) {
  if ([0, 6].includes(new Date(day).getUTCDay())) continue;
  sourceBars.push([day, 100, 100.1, 99.9, 100, 1]);
}
sourceBars[460].splice(2, 3, 100.2, 98, 98.2);
const knownAt = sourceBars[460][0] + 15 * minute;
const fixture = {
  schema: 'dsl-conformance-run-fixture-v1', case: 'invented-down-shock', strategyId: 'inventedDownShock',
  symbol: 'XAUUSD', timeframe: '1m', sourceTimeframe: '15m', rangeMethod: 'zone',
  costs: { fillOn: 'close', startEquity: 10000, slippage: 0 },
  bars: [[knownAt - minute, 98.3, 98.4, 98.1, 98.2, 1], [knownAt, 98.2, 98.8, 98.1, 98.6, 1]],
  sourceBars,
};
const scratch = mkdtempSync(join(tmpdir(), 'down-shock-source-'));
const fixturePath = join(scratch, 'fixture.json');
const sourcePath = join(scratch, 'source.strat');
const digest = (text) => createHash('sha256').update(text).digest('hex');
const results = [];

function encodeBTB1(rows) {
  const bytes = Buffer.alloc(16 + rows.length * 6 * 8);
  bytes.writeUInt32LE(0x31425442, 0);
  bytes.writeUInt32LE(1, 4);
  bytes.writeUInt32LE(rows.length, 8);
  bytes.writeUInt32LE(6, 12);
  for (let c = 0; c < 6; c++) {
    for (let r = 0; r < rows.length; r++) bytes.writeDoubleLE(rows[r][c], 16 + (c * rows.length + r) * 8);
  }
  return bytes;
}

function runCase(name, input, text, expectedCount, golden) {
  const raw = JSON.stringify(input);
  writeFileSync(fixturePath, raw);
  writeFileSync(sourcePath, text);
  const native = execFileSync(nativeEngine, [fixturePath, sourcePath], { encoding: 'utf8' }).trim();
  const wasm = globalThis.engineRunFixture(raw, text);
  assert.equal(wasm, native, `${name}: native fixture and actual WASM differ`);
  const result = JSON.parse(native);
  assert.equal(result.tradeCount, expectedCount, `${name}: expected trade count`);
  if (golden) assert.deepEqual(result, golden, `${name}: committed golden changed`);
  const symbolDir = join(scratch, input.symbol);
  mkdirSync(symbolDir, { recursive: true });
  writeFileSync(join(symbolDir, `${input.timeframe}.bin`), encodeBTB1(input.bars));
  if (input.sourceBars) writeFileSync(join(symbolDir, `${input.sourceTimeframe}.bin`), encodeBTB1(input.sourceBars));
  const reportRaw = execFileSync(nativeReport, ['report', `--dsl-file=${sourcePath}`, `--dsl-id=${input.strategyId}`,
    `--symbol=${input.symbol}`, `--tf=${input.timeframe}`, `--range=${input.rangeMethod}`,
    `--data-root=${scratch}`, `--slippage=${input.costs.slippage ?? 0}`, '--include-trades=1', '--json-only=1'], { encoding: 'utf8' });
  const report = JSON.parse(reportRaw);
  assert.deepEqual(report.warnings, [], `${name}: report warnings`);
  assert.equal(report.costs[0].trades, expectedCount, `${name}: native report lost trades`);
  const slice = report.slices[0];
  if (input.sourceBars) {
    assert.equal(slice.sourceBars, input.sourceBars.length, `${name}: source provenance count`);
    assert.equal(slice.sourceTimeframe, input.sourceTimeframe);
  }
  assert.equal(slice.trades.length, expectedCount);
  for (let i = 0; i < result.trades.length; i++) {
    // Compare every engine trade field; report-only annotations are additive.
    for (const [key, value] of Object.entries(result.trades[i])) {
      assert.deepEqual(slice.trades[i][key], value, `${name}: report trade ${i}.${key}`);
    }
  }
  const net = result.trades.reduce((sum, trade) => sum + trade.pnl, 0);
  assert(Math.abs(report.costs[0].net - net) < 1e-9, `${name}: net differs`);
  results.push({ name, trades: result.tradeCount, nativeWasmSha256: digest(native) });
  return result;
}

try {
  const positive = runCase('invented-positive-source', fixture, source, 1);
  const trade = positive.trades[0];
  assert.equal(trade.side, 'long');
  assert.equal(trade.entryIndex, 1);
  assert.equal(trade.entryT, knownAt);
  assert.equal(trade.entry, 98.2);
  assert.equal(trade.reason, 'tp');
  assert.equal(trade.meta.sourceRow, 460);
  assert.equal(trade.meta.sourceCloseT, knownAt);
  assert(Math.abs(trade.exit - 98.32766) < 1e-12);
  assert(Math.abs(trade.pnl - 37.2341666666695) < 1e-10);
  runCase('before-source-close', { ...fixture, bars: fixture.bars.slice(0, 1) }, source, 0);
  runCase('at-source-close', { ...fixture, bars: fixture.bars.slice(1) }, source, 1);
  runCase('source-prefix-without-shock', { ...fixture, sourceBars: sourceBars.slice(0, 460) }, source, 0);

  const corpus = new URL('../../conformance/run/', import.meta.url);
  for (const name of [
    ...['immediate-time', 'immediate-atr', 'immediate-13bp', 'reversal-time', 'reversal-atr', 'reversal-13bp'].map((mode) => `family-down-shock-${mode}`),
    'family-clock-range-breakout-ordinary-long', 'family-sma-golden-cross',
  ]) {
    const input = JSON.parse(readFileSync(new URL(`${name}.fixture.json`, corpus), 'utf8'));
    const text = readFileSync(new URL(`${name}.strat`, corpus), 'utf8');
    const golden = JSON.parse(readFileSync(new URL(`${name}.trades.json`, corpus), 'utf8'));
    assert(golden.tradeCount > 0, `${name}: positive control must execute`);
    runCase(name, input, text, golden.tradeCount, golden);
  }

  for (const [name, input] of [
    ['missing-source', { ...fixture, sourceBars: [] }],
    ['wrong-source-timeframe', { ...fixture, sourceTimeframe: '5m' }],
  ]) {
    const raw = JSON.stringify(input);
    writeFileSync(fixturePath, raw);
    writeFileSync(sourcePath, source);
    const wasm = JSON.parse(globalThis.engineRunFixture(raw, source));
    assert.match(wasm.error ?? '', /source (bars|timeframe)/, `${name}: WASM must refuse`);
    assert.throws(() => execFileSync(nativeEngine, [fixturePath, sourcePath], { stdio: 'pipe' }), `${name}: native fixture must refuse`);
  }
} finally {
  rmSync(scratch, { recursive: true, force: true });
}
console.log(JSON.stringify({ schema: 'down-shock-source-parity-v1', runtime: 'native-and-node-go-wasm', cases: results, rejected: 2 }, null, 2));
