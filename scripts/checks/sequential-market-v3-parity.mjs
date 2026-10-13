// Actual native CLI and Node-hosted Go/WASM parity for the Sequential E1/E2
// market companion v3 (run and admit, with an optional closure calendar and
// open-terminal reporting). Both sides are the same Go code. Inputs are invented
// valid OHLC and invented calendars; nothing is historical.
// Usage: node scripts/checks/sequential-market-v3-parity.mjs <enginewasm native binary> <enginewasm.wasm> <wasm_exec.js>
//
// Comparison rule (explicit; never widened by regenerating anything):
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
const [resultSchema, resultPath] = loadSchema('sequential-market-result-v3.schema.json');
const [admissionSchema, admissionPath] = loadSchema('sequential-market-admission-v3.schema.json');
const [calendarSchema, calendarPath] = loadSchema('sequential-closure-calendar-v1.schema.json');

// ---- invented inputs ------------------------------------------------------
const exampleDir = join(root, 'engine/testdata/sequential_market_v3');
const index = JSON.parse(readFileSync(join(exampleDir, 'EXAMPLES.json'), 'utf8'));
const setupBars = [[100, 105, 95, 100], [100, 105, 95, 100], [100, 105, 95, 100], [100, 105, 95, 100], [100, 105, 95, 101],
  [101, 108, 98, 99], [99, 107, 97, 98], [98, 98, 88, 97], [97, 105, 95, 96], [96, 104, 94, 95],
  [95, 103, 93, 94], [94, 102, 92, 93], [93, 101, 91, 92], [92, 100, 90, 91]];
const target = [...setupBars, [91, 93, 90, 92], [92, 96, 91, 95], [95, 101, 94, 100]];
const entryOnly = [...setupBars, [91, 93, 90, 92]];
const falling = [...setupBars, [91, 93, 90, 92], [92, 93, 88.5, 89]];
const stopOut = [...setupBars, [91, 93, 90, 92], [92, 93, 86.5, 87.5]];
const monday = 1709510400000;
const routes = [['EURUSD', '1m', 60e3, 0.011], ['GBPUSD', '5m', 300e3, 0.0127], ['USDJPY', '15m', 900e3, 1.5], ['AUDUSD', '30m', 1800e3, 0.0066], ['XAUUSD', '1h', 3600e3, 21.5], ['GER40', '4h', 14400e3, 180], ['SPX500', '1d', 86400e3, 52]];
const source = (symbol, tf, policy = 'E1') => `dsl v7\nstrategy "Invented" { description "Invented valid OHLC" }\nmarket { sequential symbol ${symbol} sequential timeframe ${tf} }\nsetup { type: sequential full sequential profile seq.full.public_approx.v1 sequential policy ${policy} }\nrisk { sequential riskUsd 90 sequential maxNotionalUsd 1000000 }\n`;
function fixture(symbol, tf, step, bars, scale, fee, holes = {}) {
  let offset = 0;
  const rows = bars.map((bar, i) => { offset += (holes[i] ?? 0) * step; return [monday + (i) * step + offset, ...bar.map((v) => v * scale), 1]; });
  return JSON.stringify({ schema: 'dsl-conformance-run-fixture-v1', case: 'invented', strategyId: 'invented-e1', symbol, timeframe: tf, rangeMethod: 'zone',
    costs: { feePerUnit: fee * scale, fillOn: 'open', slippage: 0.5 * scale, startEquity: 10000 }, bars: rows });
}
function calendar(symbol, tf, step, id, holes, { tweak, hash } = {}) {
  let pairs = [], offset = 0;
  for (const key of Object.keys(holes).map(Number).sort((a, b) => a - b)) { const start = monday + (key + offset) * step; pairs.push([start, start + holes[key] * step]); offset += holes[key]; }
  if (tweak) pairs = tweak(pairs);
  const canonical = `{"schema":"sequential-closure-calendar-v1","calendarId":"${id}","provider":"invented-venue","symbol":"${symbol}","timeframe":"${tf}","version":"1","closures":[${pairs.map(([a, b]) => `[${a},${b}]`).join(',')}]}`;
  return JSON.stringify({ schema: 'sequential-closure-calendar-v1', calendarId: id, provider: 'invented-venue', symbol, timeframe: tf, version: '1', closures: pairs.map(([a, b]) => ({ startMs: a, endMs: b })), sha256: hash ?? createHash('sha256').update(canonical).digest('hex') });
}
const cases = [];
for (const [name, file] of index.examples.map((e) => [e.id, e])) {
  cases.push({ id: `example/${name}`, fixture: readFileSync(join(exampleDir, file.fixture), 'utf8'), source: readFileSync(join(exampleDir, file.source), 'utf8'),
    calendar: file.calendar ? readFileSync(join(exampleDir, file.calendar), 'utf8') : '', ok: file.expect.ok, open: file.expect.open ?? false, code: file.expect.code, disposition: file.expect.disposition });
}
for (const [symbol, tf, step, scale] of routes) {
  for (const [name, bars, holes] of [['target', target, {}], ['open', entryOnly, {}], ['falling-open', falling, {}], ['stop', stopOut, {}],
    ['closure-in-setup', target, { 7: 3 }], ['reopened-fill', target, { 14: 20 }], ['closure-then-open', entryOnly, { 7: 3, 14: 9 }], ['two-closures', falling, { 3: 2, 15: 4 }]]) {
    for (const fee of [0, 0.01]) {
      cases.push({ id: `${symbol}/${tf}/${name}/fee${fee}`, fixture: fixture(symbol, tf, step, bars, scale, fee, holes), source: source(symbol, tf),
        calendar: Object.keys(holes).length ? calendar(symbol, tf, step, `cal-${name}`, holes) : '', ok: true, open: ['open', 'falling-open', 'closure-then-open', 'two-closures'].includes(name) });
    }
  }
}
const xau = routes[4];
const hole = { 7: 48 };
const refuse = (id, calendarText, code, disposition, bars = target, holes = hole) => cases.push({ id: `refuse/${id}`, fixture: fixture(xau[0], xau[1], xau[2], bars, xau[3], 0.01, holes), source: source(xau[0], xau[1]), calendar: calendarText, ok: false, code, disposition });
refuse('no-calendar', '', 'unsupported-sequential-time-gap', 'unsupported');
refuse('one-step-short', calendar(xau[0], xau[1], xau[2], 'c', hole, { tweak: (p) => p.map(([a, b]) => [a, b - xau[2]]) }), 'unsupported-sequential-time-gap', 'unsupported');
refuse('one-step-late', calendar(xau[0], xau[1], xau[2], 'c', hole, { tweak: (p) => p.map(([a, b]) => [a + xau[2], b]) }), 'unsupported-sequential-time-gap', 'unsupported');
refuse('other-hole', calendar(xau[0], xau[1], xau[2], 'c', hole, { tweak: (p) => p.map(([a, b]) => [a + 100 * xau[2], b + 100 * xau[2]]) }), 'unsupported-sequential-time-gap', 'unsupported');
refuse('hash-mismatch', calendar(xau[0], xau[1], xau[2], 'c', hole, { hash: '0'.repeat(64) }), 'invalid-sequential-calendar', 'invalid');
refuse('wrong-symbol', calendar('EURUSD', xau[1], xau[2], 'c', hole), 'invalid-sequential-calendar', 'invalid');
refuse('wrong-timeframe', calendar(xau[0], '4h', 14400e3, 'c', {}), 'invalid-sequential-calendar', 'invalid');
refuse('closure-over-bars', calendar(xau[0], xau[1], xau[2], 'c', hole, { tweak: (p) => [[monday + 2 * xau[2], monday + 4 * xau[2]], ...p] }), 'invalid-sequential-calendar', 'invalid');
refuse('not-json', '{', 'invalid-sequential-calendar', 'invalid');
refuse('duplicate-key', calendar(xau[0], xau[1], xau[2], 'c', hole).replace('"provider":"invented-venue"', '"provider":"invented-venue","provider":"x"'), 'invalid-sequential-calendar', 'invalid');
for (const [name, delta, onGrid] of [['step+1ms', 1, false], ['1.5 steps', 0.5, false], ['2 steps-1ms', 1 - 1 / xau[2], false], ['2 steps', 1, true]]) {
  const f = JSON.parse(fixture(xau[0], xau[1], xau[2], target, 1, 0.01));
  const extra = name === 'step+1ms' ? 1 : name === '1.5 steps' ? xau[2] / 2 : name === '2 steps-1ms' ? xau[2] - 1 : xau[2];
  for (let i = 7; i < f.bars.length; i++) f.bars[i][0] += extra;
  cases.push({ id: `refuse/off-grid/${name}`, fixture: JSON.stringify(f), source: source(xau[0], xau[1]), calendar: '', ok: false, code: 'unsupported-sequential-time-gap', disposition: 'unsupported', gap: { onGrid } });
}

// ---- runners --------------------------------------------------------------
globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(shim);
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(readFileSync(wasm), go.importObject);
void go.run(instance);
for (const name of ['engineRunSequentialMarketV3', 'engineAdmitSequentialMarketV3']) assert.equal(typeof globalThis[name], 'function', `${name} export missing`);
const scratch = mkdtempSync(join(tmpdir(), 'sequential-market-v3-'));
const fixturePath = join(scratch, 'fixture.json'), sourcePath = join(scratch, 'source.strat'), calendarFile = join(scratch, 'calendar.json');
function nativeCall(mode, c) {
  writeFileSync(fixturePath, c.fixture); writeFileSync(sourcePath, c.source);
  const argv = ['--sequential-market-v3', mode, fixturePath, sourcePath];
  if (c.calendar) { writeFileSync(calendarFile, c.calendar); argv.push(calendarFile); }
  const out = spawnSync(native, argv, { encoding: 'utf8', maxBuffer: 1 << 28 });
  assert(out.status === 0 || out.status === 1, `${c.id}: native exit ${out.status} ${out.stderr}`);
  return { raw: out.stdout.replace(/\n$/, ''), status: out.status };
}
const integerKeys = new Set(['index', 'entryIndex', 'exitIndex', 'barIndex', 'barCount', 'firstBarMs', 'lastBarMs', 't', 'entryT', 'exitT', 'trades', 'tradeCount', 'maxWinStreak', 'maxLossStreak', 'timeframeMs', 'gapCount', 'undeclaredGapCount', 'bars', 'previousOpenMs', 'openMs', 'missingBars', 'deltaMs', 'closureIndex', 'beforeIndex', 'startMs', 'endMs', 'declaredClosures', 'appliedCount', 'barsHeld', 'setupIndex', 'setupFirstIndex', 'decisionIndex', 'decisionOpenMs', 'decisionMs', 'nextOpenIndex', 'fillIndex', 'signalIndex']);
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

let runs = 0, successes = 0, refusals = 0, opens = 0, closureRuns = 0;
const admissions = new Map(), results = new Map();
for (const c of cases) {
  const nativeRun = nativeCall('run', c), nativeAdmit = nativeCall('admit', c);
  const wasmRun = globalThis.engineRunSequentialMarketV3(c.fixture, c.source, c.calendar), wasmAdmit = globalThis.engineAdmitSequentialMarketV3(c.fixture, c.source, c.calendar);
  assert.equal(wasmRun, globalThis.engineRunSequentialMarketV3(c.fixture, c.source, c.calendar), `${c.id}: repeated WASM run changed`);
  const [run, runN, admit, admitN] = [JSON.parse(wasmRun), JSON.parse(nativeRun.raw), JSON.parse(wasmAdmit), JSON.parse(nativeAdmit.raw)];
  for (const doc of [run, runN]) validate(doc, resultSchema, resultSchema, resultPath);
  for (const doc of [admit, admitN]) validate(doc, admissionSchema, admissionSchema, admissionPath);
  compare(run, runN, `${c.id}.run`, '');
  compare(admit, admitN, `${c.id}.admit`, '');
  for (const gap of admit.timeline?.gaps ?? []) {
    const step = admit.route.timeframeMs;
    assert.equal(gap.deltaMs, gap.openMs - gap.previousOpenMs, `${c.id}: deltaMs accurate`);
    assert.equal(gap.onGrid, gap.deltaMs % step === 0, `${c.id}: onGrid accurate`);
    if (gap.onGrid) assert.equal(gap.missingBars, gap.deltaMs / step - 1, `${c.id}: missingBars accurate`);
  }
  if (admit.timeline) {
    assert.equal(admit.timeline.undeclaredGapCount, admit.timeline.gaps.filter((g) => !g.declared).length, `${c.id}: undeclared count`);
    assert.equal(admit.timeline.gapCount >= admit.timeline.undeclaredGapCount, true);
  }
  admissions.set(c.id, admit); results.set(c.id, run);
  assert.equal(nativeRun.status === 0, c.ok, `${c.id}: native exit status`);
  assert.equal(admit.runnable, c.ok, `${c.id}: admission runnable`);
  assert.equal(Boolean(run.error), !c.ok, `${c.id}: run error presence`);
  if (c.ok) {
    assert.equal(admit.disposition, 'runnable');
    assert.equal(run.terminal.position, c.open ? 'open' : 'flat', `${c.id}: terminal position`);
    assert.equal(run.metrics.equity.length, run.identity.barCount, `${c.id}: one mark per observed bar, none for a closure`);
    assert.equal(run.identity.dslSha256, createHash('sha256').update(c.source).digest('hex'));
    assert.equal(run.identity.fixtureSha256, createHash('sha256').update(c.fixture).digest('hex'));
    if (c.calendar) { assert.equal(run.identity.calendar.sha256, JSON.parse(c.calendar).sha256); closureRuns++; } else assert(!('calendar' in run.identity));
    if (c.open) {
      opens++;
      const m = run.metrics, o = run.terminal.openTrade;
      assert.equal(m.marked.openUnrealizedPnl, o.unrealizedPnl, `${c.id}: marked block matches the open trade`);
      assert.equal(m.marked.openEntryFeePaid, o.entryFeePaid);
      assert.equal(m.equity.at(-1).equity, m.marked.endEquity, `${c.id}: last mark is the marked end equity`);
    } else {
      assert.equal(run.metrics.marked.openUnrealizedPnl, 0); assert.equal(run.metrics.marked.net, run.metrics.headline.net);
    }
    successes++;
  } else {
    assert.equal(run.error.code, c.code, `${c.id}: code`);
    assert.equal(admit.error.code, c.code, `${c.id}: admission code`);
    assert.equal(admit.disposition, c.disposition, `${c.id}: disposition`);
    for (const key of ['identity', 'run', 'metrics', 'terminal', 'capabilities', 'limits']) assert(!(key in run), `${c.id}: refusal leaked ${key}`);
    if (c.gap) {
      assert.equal(admit.timeline.gaps.length, 1);
      assert.equal(admit.timeline.gaps[0].onGrid, c.gap.onGrid, `${c.id}: onGrid`);
      assert.equal('missingBars' in admit.timeline.gaps[0], c.gap.onGrid);
      assert.equal(admit.timeline.gaps[0].declared, false);
    }
    refusals++;
  }
  runs++;
}

// Negative schema tests for the conditional rules of v3.
const negatives = [];
const mutate = (doc, edit) => { const copy = structuredClone(doc); edit(copy); return copy; };
const openId = [...results.keys()].find((id) => results.get(id).terminal?.position === 'open');
const flatId = [...results.keys()].find((id) => results.get(id).terminal?.position === 'flat' && !results.get(id).identity.calendar);
const calId = [...results.keys()].find((id) => results.get(id).identity?.calendar);
const refusalGapId = 'refuse/off-grid/step+1ms';
const calendarAdmit = [...admissions.keys()].find((id) => admissions.get(id).timeline?.gaps.some((g) => g.declared));
negatives.push(['open position without openTrade', resultSchema, resultPath, mutate(results.get(openId), (d) => { delete d.terminal.openTrade; })]);
negatives.push(['flat position with openTrade', resultSchema, resultPath, mutate(results.get(flatId), (d) => { d.terminal.openTrade = structuredClone(results.get(openId).terminal.openTrade); })]);
negatives.push(['unknown position', resultSchema, resultPath, mutate(results.get(flatId), (d) => { d.terminal.position = 'closed'; })]);
negatives.push(['marked block missing', resultSchema, resultPath, mutate(results.get(openId), (d) => { delete d.metrics.marked; })]);
negatives.push(['limits claim contiguous-only', resultSchema, resultPath, mutate(results.get(flatId), (d) => { d.limits.undeclaredGaps = 'accepted'; })]);
negatives.push(['calendar digest not hex', resultSchema, resultPath, mutate(results.get(calId), (d) => { d.identity.calendar.sha256 = 'XYZ'; })]);
negatives.push(['applied list beyond 256', resultSchema, resultPath, mutate(results.get(calId), (d) => { d.identity.calendar.applied = Array.from({ length: 257 }, () => structuredClone(d.identity.calendar.applied[0])); })]);
negatives.push(['v2 contract version', resultSchema, resultPath, mutate(results.get(flatId), (d) => { d.contractVersion = 2; })]);
negatives.push(['declared gap without closureIndex', admissionSchema, admissionPath, mutate(admissions.get(calendarAdmit), (d) => { const g = d.timeline.gaps.find((x) => x.declared); delete g.closureIndex; })]);
negatives.push(['undeclared gap with closureIndex', admissionSchema, admissionPath, mutate(admissions.get(refusalGapId), (d) => { d.timeline.gaps[0].closureIndex = 0; })]);
negatives.push(['off-grid gap with missingBars', admissionSchema, admissionPath, mutate(admissions.get(refusalGapId), (d) => { d.timeline.gaps[0].missingBars = 1; })]);
negatives.push(['on-grid gap without missingBars', admissionSchema, admissionPath, mutate(admissions.get('refuse/off-grid/2 steps'), (d) => { delete d.timeline.gaps[0].missingBars; })]);
negatives.push(['undeclaredGapCount missing', admissionSchema, admissionPath, mutate(admissions.get(refusalGapId), (d) => { delete d.timeline.undeclaredGapCount; })]);
negatives.push(['runnable with error', admissionSchema, admissionPath, mutate(admissions.get(flatId), (d) => { d.error = { code: 'x', message: 'y' }; })]);
const goodCalendar = JSON.parse(cases.find((c) => c.calendar).calendar);
negatives.push(['calendar extra field', calendarSchema, calendarPath, mutate(goodCalendar, (d) => { d.timezone = 'UTC'; })]);
negatives.push(['calendar digest uppercase', calendarSchema, calendarPath, mutate(goodCalendar, (d) => { d.sha256 = d.sha256.toUpperCase(); })]);
negatives.push(['calendar closure without endMs', calendarSchema, calendarPath, mutate(goodCalendar, (d) => { delete d.closures[0].endMs; })]);
negatives.push(['calendar lowercase symbol', calendarSchema, calendarPath, mutate(goodCalendar, (d) => { d.symbol = 'xauusd'; })]);
negatives.push(['calendar unknown timeframe', calendarSchema, calendarPath, mutate(goodCalendar, (d) => { d.timeframe = '2h'; })]);
for (const [label, schema, path, doc] of negatives) assert.throws(() => validate(doc, schema, schema, path), undefined, `schema accepted: ${label}`);
validate(goodCalendar, calendarSchema, calendarSchema, calendarPath);
for (const [id, doc] of admissions) if (cases.find((c) => c.id === id).calendar !== undefined) validate(doc, admissionSchema, admissionSchema, admissionPath);

// Transport failures keep the selected operation's envelope.
const operations = [['engineRunSequentialMarketV3', resultSchema, resultPath, 'strat-sequential-market-result-v3'], ['engineAdmitSequentialMarketV3', admissionSchema, admissionPath, 'strat-sequential-market-admission-v3']];
const good = cases[0];
const transport = [[[], 'arguments'], [[1, 2, 3], 'arguments'], [[1n, 'x', ''], 'arguments'], [['x', 'y'], 'arguments'], [[good.fixture, good.source, 5], 'arguments'],
  [[good.fixture, 'a'.repeat(8 * 1024 * 1024 + 1), ''], 'source'], [[good.fixture, '€'.repeat(Math.floor(8 * 1024 * 1024 / 3) + 1), ''], 'source'],
  [[good.fixture, good.source, ' '.repeat(4 * 1024 * 1024 + 1)], 'calendar.size'], [[good.fixture, good.source, '€'.repeat(Math.floor(4 * 1024 * 1024 / 3) + 1)], 'calendar.size'],
  [[good.fixture, good.source, '\ud800'], 'calendar'], [[good.fixture, '\ud800', ''], 'source'], [['{'.repeat(10), good.source, ''], null], [[good.fixture, good.source, '{'], null]];
let transportChecks = 0;
for (const [name, schema, schemaPath, schemaId] of operations) {
  for (const [bad, field] of transport) {
    const out = JSON.parse(globalThis[name](...bad));
    validate(out, schema, schema, schemaPath);
    assert.equal(out.schema, schemaId, `${name}: envelope kind for ${bad.length} args / ${field}`);
    assert(out.error, `${name}: error expected`);
    if (field) assert.equal(out.error.field, field, `${name}: field for ${JSON.stringify(bad.map((v) => typeof v === "string" ? v.slice(0, 12) + "(" + v.length + ")" : String(typeof v)))}: ${JSON.stringify(out.error)}`);
    if (name === 'engineAdmitSequentialMarketV3') assert.equal(out.runnable, false);
    transportChecks++;
  }
}
for (const [mode, schema, schemaPath, id] of [['run', resultSchema, resultPath, 'strat-sequential-market-result-v3'], ['admit', admissionSchema, admissionPath, 'strat-sequential-market-admission-v3']]) {
  for (const argv of [[], ['/nonexistent/fixture.json', sourcePath], [fixturePath, '/nonexistent/source.strat'], [fixturePath, sourcePath, '/nonexistent/calendar.json']]) {
    const out = spawnSync(native, ['--sequential-market-v3', mode, ...argv], { encoding: 'utf8' });
    assert(out.status === 1 || out.status === 2, `native ${mode} ${argv.length} args: exit ${out.status}`);
    const doc = JSON.parse(out.stdout);
    validate(doc, schema, schema, schemaPath);
    assert.equal(doc.schema, id, `native ${mode}: envelope kind`);
    transportChecks++;
  }
}
assert(JSON.parse(globalThis.engineRunSequentialMarketV3(good.fixture, good.source, good.calendar)).schema, 'instance survived malformed calls');
// v1 and v2 exports are still present and unchanged in shape.
for (const name of ['engineRunSequentialFixture', 'engineRunSequentialMarket', 'engineAdmitSequentialMarket']) assert.equal(typeof globalThis[name], 'function', `${name} was removed`);
rmSync(scratch, { recursive: true, force: true });
console.log(JSON.stringify({ negativeSchemaChecks: negatives.length, transportChecks, comparisons: runs * 2, successes, refusals, openPositionRuns: opens, closureRuns, leaves, nonIdenticalLeaves: inexact, worstRelativeDifference: worst, tolerance: MONEY_TOLERANCE, nativeHost: `${process.platform}/${process.arch}` }));
