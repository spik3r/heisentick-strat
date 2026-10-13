// Actual native CLI and Node-hosted Go/WASM parity for the Sequential E1/E2
// market companion v2 (run and admit). No JS strategy engine: both sides are
// the same Go code. Inputs are invented valid OHLC; nothing is historical.
// Usage: node scripts/checks/sequential-market-parity.mjs <enginewasm native binary> <enginewasm.wasm> <wasm_exec.js>
//
// Comparison rule (explicit, never widened by regenerating anything):
//  - schema, identity, counts, indices, timestamps, strings, booleans and null
//    reasons must be identical;
//  - every other number may differ by at most MONEY_TOLERANCE, relative, no
//    absolute floor. Fused multiply-add on arm64 can move a money leaf by one
//    unit in the last place; 1e-12 is far above that and far below any cent.
//    The script reports how many leaves were not bit-identical.
import assert from 'node:assert/strict';
import { webcrypto, createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';

const MONEY_TOLERANCE = 1e-12;
const args = process.argv.slice(2);
assert.equal(args.length, 3, 'native binary, wasm and wasm_exec.js paths required');
const [native, wasm, shim] = args.map((path) => resolve(path));
const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');

// Test-only closed-schema validator (same keyword subset as the inspection check).
const known = new Set(['$schema', '$id', 'title', 'description', '$defs', '$ref', 'oneOf', 'anyOf', 'type', 'properties', 'required', 'additionalProperties', 'items', 'minItems', 'maxItems', 'minLength', 'minimum', 'maximum', 'pattern', 'enum', 'const', 'exclusiveMinimum', 'if', 'then', 'else', 'not', 'minProperties', 'allOf']);
const loadSchema = (name) => { const path = join(root, 'spec/schemas', name); return [JSON.parse(readFileSync(path, 'utf8')), path]; };
function validate(value, rule, base, path) {
  for (const key of Object.keys(rule)) assert(known.has(key), `unknown schema keyword ${key}`);
  if (rule.$ref) {
    const [file, fragment] = rule.$ref.split('#');
    const [nextBase, nextPath] = file ? loadSchema(file) : [base, path];
    let target = nextBase;
    if (fragment) for (const part of fragment.slice(1).split('/')) target = target[part.replaceAll('~1', '/').replaceAll('~0', '~')];
    assert(target, `unresolved schema reference ${rule.$ref}`);
    return validate(value, target, nextBase, nextPath);
  }
  if (rule.oneOf || rule.anyOf) {
    const failures = [];
    const matches = (rule.oneOf ?? rule.anyOf).filter((child) => { try { validate(value, child, base, path); return true; } catch (error) { failures.push(error.message); return false; } });
    assert(rule.oneOf ? matches.length === 1 : matches.length > 0, `schema alternatives matched ${matches.length}: ${failures.join(' | ')}`);
  }
  const passes = (child) => { try { validate(value, child, base, path); return true; } catch { return false; } };
  for (const child of rule.allOf ?? []) validate(value, child, base, path);
  if (rule.not) assert(!passes(rule.not), 'schema not');
  if (rule.if) {
    if (passes(rule.if)) { if (rule.then) validate(value, rule.then, base, path); } else if (rule.else) validate(value, rule.else, base, path);
  }
  if (Object.hasOwn(rule, 'const')) assert.deepEqual(value, rule.const);
  if (rule.enum) assert(rule.enum.includes(value), `schema enum ${JSON.stringify(value)}`);
  switch (rule.type) {
    case 'object':
      assert(value !== null && typeof value === 'object' && !Array.isArray(value), 'schema object');
      for (const key of rule.required ?? []) assert(Object.hasOwn(value, key), `required ${key}`);
      if (rule.minProperties !== undefined) assert(Object.keys(value).length >= rule.minProperties, 'schema minProperties');
      for (const [key, child] of Object.entries(value)) {
        if (Object.hasOwn(rule.properties ?? {}, key)) validate(child, rule.properties[key], base, path);
        else assert(rule.additionalProperties !== false, `unexpected ${key}`);
      }
      break;
    case 'array':
      assert(Array.isArray(value), 'schema array');
      if (rule.minItems !== undefined) assert(value.length >= rule.minItems);
      if (rule.maxItems !== undefined) assert(value.length <= rule.maxItems);
      for (const item of value) validate(item, rule.items ?? {}, base, path);
      break;
    case 'string':
      assert.equal(typeof value, 'string');
      if (rule.minLength !== undefined) assert([...value].length >= rule.minLength);
      if (rule.pattern) assert(new RegExp(rule.pattern).test(value), `schema pattern ${rule.pattern}`);
      break;
    case 'integer': assert(Number.isSafeInteger(value), 'schema integer'); break;
    case 'number': assert(typeof value === 'number' && Number.isFinite(value), 'schema number'); break;
    case 'boolean': assert.equal(typeof value, 'boolean'); break;
    case 'null': assert.equal(value, null); break;
    case undefined: break;
    default: assert.fail(`unsupported schema type ${rule.type}`);
  }
  if (rule.minimum !== undefined) assert(value >= rule.minimum);
  if (rule.exclusiveMinimum !== undefined) assert(value > rule.exclusiveMinimum);
  if (rule.maximum !== undefined) assert(value <= rule.maximum);
}
const [resultSchema, resultPath] = loadSchema('sequential-market-result-v2.schema.json');
const [admissionSchema, admissionPath] = loadSchema('sequential-market-admission-v2.schema.json');

// ---- invented fixtures ----------------------------------------------------
const setupBars = [[100, 105, 95, 100], [100, 105, 95, 100], [100, 105, 95, 100], [100, 105, 95, 100], [100, 105, 95, 101],
  [101, 108, 98, 99], [99, 107, 97, 98], [98, 98, 88, 97], [97, 105, 95, 96], [96, 104, 94, 95],
  [95, 103, 93, 94], [94, 102, 92, 93], [93, 101, 91, 92], [92, 100, 90, 91]];
const target = [...setupBars, [91, 93, 90, 92], [92, 96, 91, 95], [95, 101, 94, 100]];
const stop = [...setupBars, [91, 93, 90, 92], [92, 93, 86.5, 87.5]];
const heldAtEnd = [...setupBars, [91, 93, 90, 92]];
const mirror = (bars, constant = 200) => bars.map(([o, h, l, c]) => [constant - o, constant - l, constant - h, constant - c]);
const monday = 1709510400000;
const routes = [['EURUSD', '1m', 60e3, 0.011], ['GBPUSD', '5m', 300e3, 0.0127], ['USDJPY', '15m', 900e3, 1.5], ['AUDUSD', '30m', 1800e3, 0.0066], ['XAUUSD', '1h', 3600e3, 21.5], ['GER40', '4h', 14400e3, 180], ['SPX500', '1d', 86400e3, 52]];
const source = (symbol, tf, policy = 'E1') => `dsl v7\nstrategy "Invented" { description "Invented valid OHLC" }\nmarket { sequential symbol ${symbol} sequential timeframe ${tf} }\nsetup { type: sequential full sequential profile seq.full.public_approx.v1 sequential policy ${policy} }\nrisk { sequential riskUsd 90 sequential maxNotionalUsd 1000000 }\n`;
function fixture(symbol, tf, step, bars, scale, fee, { holes = {}, costs } = {}) {
  let offset = 0;
  const rows = bars.map((bar, i) => {
    offset += (holes[i] ?? 0) * step;
    return [monday + i * step + offset, ...bar.map((v) => v * scale), 1];
  });
  return JSON.stringify({
    schema: 'dsl-conformance-run-fixture-v1', case: 'invented', strategyId: 'invented-e1', symbol, timeframe: tf, rangeMethod: 'zone',
    costs: costs ?? { feePerUnit: fee * scale, fillOn: 'open', slippage: 0.5 * scale, startEquity: 10000 }, bars: rows,
  });
}
const cases = [];
for (const [symbol, tf, step, scale] of routes) {
  for (const [name, bars] of [['target', target], ['stop', stop], ['mirror-target', mirror(target)], ['no-order', setupBars]]) {
    for (const fee of [0, 0.01]) cases.push({ id: `${symbol}/${tf}/${name}/fee${fee}`, fixture: fixture(symbol, tf, step, bars, scale, fee), source: source(symbol, tf), ok: true });
  }
}
const xau = routes[4];
cases.push({ id: 'refuse/terminal-open', fixture: fixture(xau[0], xau[1], xau[2], heldAtEnd, xau[3], 0.01), source: source(xau[0], xau[1]), ok: false, code: 'unsupported-incomplete-terminal-run', disposition: 'unsupported' });
cases.push({ id: 'refuse/weekend-hole', fixture: fixture(xau[0], xau[1], xau[2], target, xau[3], 0.01, { holes: { 7: 48 } }), source: source(xau[0], xau[1]), ok: false, code: 'unsupported-sequential-time-gap', disposition: 'unsupported' });
cases.push({ id: 'refuse/route-mismatch', fixture: fixture('EURUSD', '1h', 3600e3, target, 1, 0.01), source: source(xau[0], xau[1]), ok: false, code: 'invalid-sequential-accounting', disposition: 'invalid' });
cases.push({ id: 'refuse/no-start-equity', fixture: fixture(xau[0], xau[1], xau[2], target, 1, 0, { costs: { feePerUnit: 0, fillOn: 'open', slippage: 0.5 } }), source: source(xau[0], xau[1]), ok: false, code: 'invalid-sequential-accounting', disposition: 'invalid' });
cases.push({ id: 'refuse/legacy-source', fixture: fixture(xau[0], xau[1], xau[2], target, 1, 0), source: readFileSync(join(root, 'conformance/parse/setup-legacy-setup9.strat'), 'utf8'), ok: false, code: 'invalid-sequential-accounting', disposition: 'unsupported' });
cases.push({ id: 'refuse/p1-policy', fixture: fixture(xau[0], xau[1], xau[2], target, 1, 0), source: source(xau[0], xau[1], 'P1'), ok: false, code: 'unsupported_config', disposition: 'unsupported' });
cases.push({ id: 'refuse/not-json', fixture: '{', source: source(xau[0], xau[1]), ok: false, code: 'invalid-sequential-input', disposition: 'invalid' });
cases.push({ id: 'refuse/empty-source', fixture: fixture(xau[0], xau[1], xau[2], target, 1, 0), source: '', ok: false, code: 'SEQUENTIAL_DSL_INVALID', disposition: 'invalid' });

// ---- runners --------------------------------------------------------------
globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(shim);
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(readFileSync(wasm), go.importObject);
void go.run(instance);
for (const name of ['engineRunSequentialMarket', 'engineAdmitSequentialMarket']) assert.equal(typeof globalThis[name], 'function', `${name} export missing`);
const scratch = mkdtempSync(join(tmpdir(), 'sequential-market-'));
const fixturePath = join(scratch, 'fixture.json'), sourcePath = join(scratch, 'source.strat');
function nativeCall(mode, c) {
  writeFileSync(fixturePath, c.fixture);
  writeFileSync(sourcePath, c.source);
  const out = spawnSync(native, ['--sequential-market', mode, fixturePath, sourcePath], { encoding: 'utf8', maxBuffer: 1 << 28 });
  assert(out.status === 0 || out.status === 1, `${c.id}: native exit ${out.status} ${out.stderr}`);
  return { raw: out.stdout.replace(/\n$/, ''), status: out.status };
}

const integerKeys = new Set(['index', 'entryIndex', 'exitIndex', 'barIndex', 'barCount', 'firstBarMs', 'lastBarMs', 't', 'entryT', 'exitT', 'trades', 'tradeCount', 'maxWinStreak', 'maxLossStreak', 'timeframeMs', 'gapCount', 'bars', 'previousOpenMs', 'openMs', 'missingBars', 'setupIndex', 'setupFirstIndex', 'decisionIndex', 'decisionOpenMs', 'decisionMs', 'nextOpenIndex', 'fillIndex', 'signalIndex']);
let leaves = 0, inexact = 0, worst = 0;
function compare(a, b, path, key) {
  assert.equal(Array.isArray(a), Array.isArray(b), `${path}: shape`);
  if (a === null || b === null || typeof a !== 'object') {
    leaves++;
    if (typeof a === 'number' && typeof b === 'number') {
      if (Object.is(a, b)) return;
      assert(!integerKeys.has(key), `${path}: ${a} vs ${b} (exact leaf)`);
      const relative = Math.abs(a - b) / Math.max(Math.abs(a), Math.abs(b));
      assert(relative <= MONEY_TOLERANCE, `${path}: ${a} vs ${b} (relative ${relative})`);
      inexact++; worst = Math.max(worst, relative);
      return;
    }
    assert.deepEqual(a, b, `${path}: leaf`);
    return;
  }
  assert.deepEqual(Object.keys(a), Object.keys(b), `${path}: keys`);
  for (const k of Object.keys(a)) compare(a[k], b[k], `${path}.${k}`, k);
}

let runs = 0, successes = 0, refusals = 0;
for (const c of cases) {
  const nativeRun = nativeCall('run', c), nativeAdmit = nativeCall('admit', c);
  const wasmRun = globalThis.engineRunSequentialMarket(c.fixture, c.source), wasmAdmit = globalThis.engineAdmitSequentialMarket(c.fixture, c.source);
  assert.equal(wasmRun, globalThis.engineRunSequentialMarket(c.fixture, c.source), `${c.id}: repeated WASM run changed`);
  const [run, runN, admit, admitN] = [JSON.parse(wasmRun), JSON.parse(nativeRun.raw), JSON.parse(wasmAdmit), JSON.parse(nativeAdmit.raw)];
  for (const doc of [run, runN]) validate(doc, resultSchema, resultSchema, resultPath);
  for (const doc of [admit, admitN]) validate(doc, admissionSchema, admissionSchema, admissionPath);
  compare(run, runN, `${c.id}.run`, '');
  compare(admit, admitN, `${c.id}.admit`, '');
  assert.equal(nativeRun.status === 0, c.ok, `${c.id}: native exit status`);
  assert.equal(nativeAdmit.status === 0, c.ok, `${c.id}: native admission exit status`);
  assert.equal(admit.runnable, c.ok, `${c.id}: admission runnable`);
  assert.equal(Boolean(run.error), !c.ok, `${c.id}: run error presence`);
  if (c.ok) {
    assert.equal(admit.disposition, 'runnable');
    assert.equal(run.terminal.position, 'flat');
    assert.equal(run.metrics.equity.length, run.identity.barCount, `${c.id}: one mark per bar`);
    assert.equal(run.identity.dslSha256, createHash('sha256').update(c.source).digest('hex'));
    assert.equal(run.identity.fixtureSha256, createHash('sha256').update(c.fixture).digest('hex'));
    successes++;
  } else {
    assert.equal(run.error.code, c.code, `${c.id}: code`);
    assert.equal(admit.error.code, c.code, `${c.id}: admission code`);
    assert.equal(admit.disposition, c.disposition, `${c.id}: disposition`);
    for (const key of ['identity', 'run', 'metrics', 'terminal', 'capabilities', 'limits']) assert(!(key in run), `${c.id}: refusal leaked ${key}`);
    refusals++;
  }
  runs++;
}
// Foreign argument types must be refused in-band and must not stop the instance.
for (const bad of [[], [1, 2], [1n, 'x'], ['x']]) {
  for (const name of ['engineRunSequentialMarket', 'engineAdmitSequentialMarket']) {
    const out = JSON.parse(globalThis[name](...bad));
    assert.equal(out.error.field, 'arguments', `${name}(${bad.length} args)`);
  }
}
assert(JSON.parse(globalThis.engineRunSequentialMarket(cases[0].fixture, cases[0].source)).identity, 'instance survived malformed calls');
rmSync(scratch, { recursive: true, force: true });
console.log(JSON.stringify({ comparisons: runs * 2, successes, refusals, leaves, nonIdenticalLeaves: inexact, worstRelativeDifference: worst, tolerance: MONEY_TOLERANCE, nativeHost: `${process.platform}/${process.arch}` }));
