// Actual native CLI and public Go WASM parity on supplied invented OHLC only.
// No JS strategy execution, financial statistics, historical or release claim.
// node scripts/checks/supply-demand-retest-parity.mjs <heisentick> <engine.wasm> <wasm_exec.js> [evidence-dir]
import assert from 'node:assert/strict';
import { createHash, webcrypto } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

const args = process.argv.slice(2);
assert(args.length === 3 || args.length === 4, 'three artifact paths and optional evidence directory required');
const [nativeCLI, wasmPath, shim, evidence] = args.map((path) => resolve(path));
const maxBytes = 64 << 20;
const scratch = mkdtempSync(join(tmpdir(), 'supply-demand-retest-'));
const requestPath = join(scratch, 'request.json');
const sourcePath = join(scratch, 'source.strat');
const hash = (value) => createHash('sha256').update(value).digest('hex');
const receipts = [];
if (evidence) mkdirSync(evidence, { recursive: true });
globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(shim);
const go = new globalThis.Go();
go.env = { ...go.env, GOMAXPROCS: '1' };
const { instance } = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject);
void go.run(instance);
for (const name of ['engineRunFixture', 'engineRunSequentialFixture', 'engineRunColumns', 'engineRunClockRangeFixture', 'engineRunPaperPrefix', 'engineRunPaperBidAsk', 'engineRunSupplyDemandRetestReport']) {
  assert.equal(typeof globalThis[name], 'function', `retained or new export ${name}`);
}
const call = globalThis.engineRunSupplyDemandRetestReport;
function wasm(...values) {
  const raw = call(...values);
  assert.equal(typeof raw, 'string');
  assert(Buffer.byteLength(raw) <= maxBytes, 'unbounded WASM output');
  return { raw, value: JSON.parse(raw) };
}
function native(raw, source) {
  writeFileSync(requestPath, raw); writeFileSync(sourcePath, source);
  const child = spawnSync(nativeCLI, ['supply-demand-retest-report', `--request-file=${requestPath}`, `--dsl-file=${sourcePath}`],
    { encoding: 'utf8', timeout: 120000, maxBuffer: maxBytes + 4096, env: { ...process.env, GOMAXPROCS: '1' } });
  assert.equal(child.error, undefined);
  assert.equal(child.status, 0, child.stderr);
  assert.equal(child.stderr, '');
  assert(child.stdout.endsWith('\n'), 'native output must be one complete JSON line');
  const encoded = child.stdout.slice(0, -1);
  assert(Buffer.byteLength(encoded) <= maxBytes);
  return { raw: encoded, value: JSON.parse(encoded) };
}
function refused(value) {
  assert.deepEqual(Object.keys(value).sort(), ['contractVersion', 'error', 'schema']);
  assert.equal(value.schema, 'strat-supply-demand-retest-result-v1');
  assert.equal(value.contractVersion, 1);
  assert.equal(typeof value.error.code, 'string');
}
function pair(name, raw, source, success) {
  const a = native(raw, source), b = wasm(raw, source);
  assert.equal(b.raw, a.raw, `complete native/WASM bytes: ${name}`);
  if (!success) refused(a.value);
  else {
    const r = a.value;
    assert.equal(r.error, undefined, `${name}: ${a.raw}`);
    assert.deepEqual(Object.keys(r).sort(), ['capabilities', 'contractVersion', 'identity', 'metrics', 'run', 'schema', 'terminal']);
    assert.equal(r.identity.requestSha256, hash(raw));
    assert.equal(r.identity.sourceSha256, hash(source));
    assert.equal(r.metrics.basis, 'supply-demand-retest-broker-close-mtm-v1');
    assert.equal(r.identity.basis, r.metrics.basis);
    assert.equal(r.terminal.policy, 'last-close-liquidation');
    assert.equal(r.terminal.position, 'flat');
    for (const field of ['headline', 'trades', 'equity']) assert.equal(r.capabilities[field], true);
    for (const field of ['groupings', 'monthly', 'rDistribution', 'portfolio', 'live', 'paper']) assert.equal(r.capabilities[field], false);
    assert.equal(r.metrics.equity.length, r.identity.chart.count);
    assert.equal(r.run.tradeCount, r.run.trades.length);
    assert.equal(r.metrics.tradeAccounting.length, r.run.tradeCount);
  }
  receipts.push({ name, success, requestSha256: hash(raw), sourceSha256: hash(source), resultSha256: hash(a.raw), resultBytes: Buffer.byteLength(a.raw) });
  return a.value;
}
const sourceTemplate = `dsl v7
strategy "Invented public retest" { description "Mechanical supplied-input parity." }
market conditions {
 slices(XAUUSD 1h)
 sessions(london, ny)
 day type in (trending, ranging, choppy)
 movement below 1.0
}
setup {
 type: supply demand
 source timeframe 4h
 retest on entry timeframe
 patterns(DBR, RBR, RBD, DBD)
 base 1 to 4 candles
 impulse at least 2.5 ATR within 4 candles
 zone width max 1.0 ATR
 retest within 96 candles
 retest first touch only
 retest must reject zone
 wait 0 candles after zone
}
filters {
 higher timeframe off
 side both
}
risk { stop: beyond zone + 0.3 ATR min 0.4 ATR max 3 ATR }
target { target 2R }
management {
 breakeven off
 wait 6 candles after trade
}
entryTf 1h
`;
function invented({ timeframe = '1h', short = false, daily = false, fillOn = 'close', truncate = 200 } = {}) {
  const hourly = Array.from({ length: 200 }, (_, i) => {
    let o = 100, c = 100, h = 100.5, l = 99.5;
    if (i >= 120 && i < 124) { h = 100.2; l = 99.8; }
    else if (i >= 124 && i < 140) { o = 100 + 0.5 * (i - 124); c = o + 0.5; h = c + 0.1; l = o - 0.1; }
    else if (i >= 140) { o = c = 108; h = 108.5; l = 107.5; }
    if (i === 157) { o = 108; h = 108.2; l = 100.1; c = 101; }
    if (short) [o, h, l, c] = [200 - o, 200 - l, 200 - h, 200 - c];
    return [i * 3600000, o, h, l, c, 1];
  });
  // OHLC construction from the invented hourly recipe, not output statistics.
  const sourceBars = Array.from({ length: 50 }, (_, i) => {
    const rows = hourly.slice(i * 4, i * 4 + 4);
    return [rows[0][0], rows[0][1], Math.max(...rows.map((r) => r[2])), Math.min(...rows.map((r) => r[3])), rows[3][4], 4];
  });
  const steps = timeframe === '15m' ? 4 : timeframe === '30m' ? 2 : 1;
  const bars = hourly.slice(0, truncate).flatMap((row) => Array.from({ length: steps }, (_, i) => [row[0] + i * 3600000 / steps, ...row.slice(1)]));
  let source = sourceTemplate.replaceAll('1h', timeframe);
  if (daily) source = source.replace('higher timeframe off', 'higher timeframe must agree');
  const sourceHtfBars = daily ? Array.from({ length: 10 }, (_, i) => [i * 86400000, 100, 102, 98, short ? 99 : 101, 1]) : [];
  return { source, request: { schema: 'strat-supply-demand-retest-request-v1', case: 'invented-public-retest', strategyId: 'invented-supply-demand', symbol: 'XAUUSD', timeframe, sourceTimeframe: '4h', higherTimeframe: daily ? '1d' : '', rangeMethod: 'zone', costs: { feePerUnit: 0.01, slippage: 0.02, slippageBps: 0, startEquity: 10000, fillOn }, bars, sourceBars, sourceHtfBars } };
}
try {
  for (const timeframe of ['15m', '30m', '1h']) for (const short of [false, true]) for (const daily of [false, true]) {
    const { source, request } = invented({ timeframe, short, daily });
    const result = pair(`${timeframe}-${short ? 'short' : 'long'}-${daily ? 'daily' : 'off'}`, JSON.stringify(request), source, true);
    assert.equal(result.run.tradeCount, 1, 'invented recipe must exercise actual trade accounting');
    assert.equal(result.run.trades[0].side, short ? 'short' : 'long');
    assert.equal(result.identity.htfPolicy, daily ? 'completed-daily-direction-decision-time-weekend-v1' : 'off');
  }
  for (const fillOn of ['open', 'nextOpen']) {
    const { source, request } = invented({ fillOn });
    const result = pair(fillOn, JSON.stringify(request), source, true);
    assert.equal(result.run.trades[0].entryIndex, 158);
    const pending = invented({ fillOn, truncate: 158 });
    const end = pair(`${fillOn}-last-bar-pending`, JSON.stringify(pending.request), pending.source, true);
    assert.equal(end.run.tradeCount, 0);
    assert.equal(end.terminal.pendingMarketOrders, 1);
    assert.equal(end.terminal.liquidated, false);
  }
  const base = invented(), raw = JSON.stringify(base.request);
  const healthy = pair('repeat-baseline', raw, base.source, true);
  const terminal = invented({ truncate: 158 });
  const liquidated = pair('last-close-liquidation', JSON.stringify(terminal.request), terminal.source, true);
  assert.equal(liquidated.terminal.liquidated, true); assert.equal(liquidated.terminal.endOfTestTrades, 1);
  const quiet = invented(); quiet.request.bars[157] = [157 * 3600000, 108, 108.5, 107.5, 108, 1];
  const noTrades = pair('no-trades', JSON.stringify(quiet.request), quiet.source, true);
  assert.equal(noTrades.run.tradeCount, 0); assert.equal(noTrades.metrics.headline.net, 0);
  assert.equal(noTrades.metrics.headline.winRate, null); assert.equal(noTrades.metrics.headline.winRateReason, 'no-trades');
  const unicode = invented(); unicode.request.case = '<>& Ω 😀 \u2028';
  pair('exact-unicode-identity', JSON.stringify(unicode.request), unicode.source.replace('Mechanical supplied-input parity.', 'Mechanical 😀 Ω supplied-input parity.'), true);
  const future = invented(); future.request.sourceBars[49] = [49 * 14400000, 100, 150, 50, 130, 1];
  const changed = pair('future-source-mutation', JSON.stringify(future.request), future.source, true);
  assert.deepEqual(changed.run.trades, healthy.run.trades);
  const daily = invented({ daily: true });
  const originalDaily = pair('daily-prefix', JSON.stringify(daily.request), daily.source, true);
  daily.request.sourceHtfBars[9] = [20 * 86400000, 100, 150, 50, 60, 1];
  const futureDaily = pair('future-daily-time-value', JSON.stringify(daily.request), daily.source, true);
  assert.deepEqual(futureDaily.run.trades, originalDaily.run.trades);
  for (const [name, changedRaw, changedSource] of [
    ['empty', '', base.source], ['null', 'null', base.source], ['unknown', raw.replace('"case":', '"unknown":0,"case":'), base.source],
    ['duplicate', raw.replace('"case":', '"case":"duplicate","case":'), base.source],
    ['escaped-duplicate', raw.replace('"case":', '"\\u0063ase":"duplicate","case":'), base.source],
    ['unpaired-json', raw.replace('"case":"', '"case":"\\ud800'), base.source], ['trailing-json', raw + '{}', base.source],
    ['contractVersion', raw.replace('"case":', '"contractVersion":1,"case":'), base.source],
    ['fractional-timestamp', raw.replace('"bars":[[0,', '"bars":[[0.0000000000000000001,'), base.source],
    ['rounded-timestamp', raw.replace('"bars":[[0,', '"bars":[[0.999999999999999999999,'), base.source],
    ['null-row-value', raw.replace('"bars":[[0,', '"bars":[[null,'), base.source],
    ['wrong-route', raw.replace('"symbol":"XAUUSD"', '"symbol":"EURUSD"'), base.source],
    ['missing-daily', raw, base.source.replace('higher timeframe off', 'higher timeframe must agree')],
    ['wrong-family', raw, base.source.replace('type: supply demand', 'type: flag')],
    ['bad-source', raw, 'invalid source'],
  ]) pair(name, changedRaw, changedSource, false);
  for (const args of [[], [raw], [raw, base.source, 'extra']]) {
    const result = wasm(...args).value; refused(result); assert.equal(result.error.code, 'invalid-arguments');
  }
  let coerced = false;
  for (const value of [undefined, null, true, 1, [], {}, new String(raw), 1n, Symbol('x'), { toString() { coerced = true; return raw; } }]) {
    for (const args of [[value, base.source], [raw, value]]) {
      const result = wasm(...args).value; refused(result); assert.equal(result.error.code, 'invalid-arguments');
    }
  }
  assert.equal(coerced, false, 'WASM must not coerce caller objects');
  for (const value of ['\ud800', '\udc00', '\ud800x', 'x\udc00']) {
    for (const args of [[value, base.source], [raw, value]]) { const result = wasm(...args).value; refused(result); assert.equal(result.error.code, 'invalid-input'); }
  }
  assert.equal(wasm(raw, base.source).raw, native(raw, base.source).raw, 'bad invocations damaged later valid execution');
  const receipt = { schema: 'supply-demand-retest-native-wasm-parity-v1', inventedInputsOnly: true, exactBytes: true, nativeSHA256: hash(readFileSync(nativeCLI)), wasmSHA256: hash(readFileSync(wasmPath)), cases: receipts };
  if (evidence) writeFileSync(join(evidence, 'parity.json'), JSON.stringify(receipt, null, 2) + '\n', { flag: 'wx' });
  console.log(`S&D retest exact native/public-WASM parity passed: ${receipts.length} supplied-input cases plus strict argument and UTF-16 refusals`);
} finally {
  rmSync(scratch, { recursive: true, force: true });
}
process.exit(0);
