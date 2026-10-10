// Exact-source native and actual Node-hosted Go/WASM companion qualification.
// Usage: node scripts/checks/sequential-backtest-parity.mjs [--go <go binary>]
//   [--baseline-engine <prechange native engine>] [--baseline-wasm <prechange WASM>]
//   [--baseline-repo <prechange archived repository>] [--out <directory outside the repository>]
// Builds use the installed toolchain only, in an OS temporary directory. No
// release/consumer/browser or historical-outcome qualification is implied.
import assert from 'node:assert/strict';
import { createHash, webcrypto } from 'node:crypto';
import { execFileSync, spawnSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const options = {};
for (let i = 2; i < process.argv.length; i += 2) {
  const key = process.argv[i];
  assert(['--go', '--baseline-engine', '--baseline-wasm', '--baseline-repo', '--out'].includes(key) && process.argv[i + 1], `unknown/missing argument ${key}`);
  assert(!(key in options), `duplicate option ${key}`);
  options[key] = process.argv[i + 1];
}
const goBinary = options['--go'] || 'go';
const env = { ...process.env, CGO_ENABLED: '0', GOTOOLCHAIN: 'local', GOPROXY: 'off' };
const go = (args, extra = {}, cwd = root) => execFileSync(goBinary, args, { cwd, env: { ...env, ...extra }, encoding: 'utf8', maxBuffer: 1 << 28 });
assert.equal(go(['env', 'GOFLAGS']).trim(), '', 'qualification requires ordinary compiler flags');
assert(!env.GOCOMPILEDEBUG, 'qualification cannot override compiler/FMA behavior');
const nativeArch = go(['env', 'GOARCH']).trim();
assert.equal(nativeArch, { x64: 'amd64', arm64: 'arm64' }[process.arch], 'native Go target must match the actual Node host');
assert(['amd64', 'arm64'].includes(nativeArch), 'qualification host must be amd64 or ARM64');
assert(!options['--baseline-repo'] || (!options['--baseline-engine'] && !options['--baseline-wasm']), 'provide baseline repository or artifacts, not both');
const scratch = mkdtempSync(join(tmpdir(), 'sequential-backtest-'));
const output = options['--out'] && resolve(options['--out']);
if (output) {
  assert(output !== root && !output.startsWith(root + sep), 'qualification output must be outside the Git tree');
  mkdirSync(output, { recursive: true });
}
// Adjacent binary64 values, without decimal epsilon or relative-to-one floor.
function adjacentPositive(value, direction) {
  assert(Number.isFinite(value) && value > 0);
  const bytes = new ArrayBuffer(8), data = new DataView(bytes);
  data.setFloat64(0, value, false);
  data.setBigUint64(0, data.getBigUint64(0, false) + BigInt(direction), false);
  const adjacent = data.getFloat64(0, false);
  assert(Number.isFinite(adjacent) && adjacent >= 0);
  return adjacent;
}
const sha = (value) => createHash('sha256').update(value).digest('hex');
const fixtures = [
  ['family-legacy-setup9', 'dslSequentialLegacySetup9', 'conformance/parse/setup-legacy-setup9.strat'],
  ['family-legacy-setup9-fall', 'dslSequentialLegacySetup9', 'conformance/parse/setup-legacy-setup9.strat'],
  ['family-legacy-setup9-perf-seasonal-long', 'dslSequentialLegacySetup9PerfSeasonal', 'conformance/parse/setup-legacy-setup9-perf-seasonal.strat'],
  ['family-legacy-setup9-perf-seasonal-short', 'dslSequentialLegacySetup9PerfSeasonal', 'conformance/parse/setup-legacy-setup9-perf-seasonal.strat'],
  ['family-sequential-full-e1-long', 'dslSequentialFullE1', 'conformance/run/family-sequential-full-e1-long.strat'],
  ['family-sequential-full-e1-short', 'dslSequentialFullE1', 'conformance/run/family-sequential-full-e1-long.strat'],
  ['family-sequential-full-e2-long', 'dslSequentialFullE2', 'conformance/run/family-sequential-full-e2-long.strat'],
  ['family-sequential-full-e2-short', 'dslSequentialFullE2', 'conformance/run/family-sequential-full-e2-long.strat'],
];
const hashes = {
  dslSequentialLegacySetup9: 'e2995b9057bc091ce5061bb66d5a06b35c33ebd621a8bef4e00f4496d774dc44',
  dslSequentialLegacySetup9PerfSeasonal: 'f16afb623842d05ef85db179e25138bf75ac1778396a4cb72563333e043658eb',
  dslSequentialFullE1: '6ffd40ee4ffc3ba5c606a1e23e203757972e5742263a694fcc373de8d92cf8ac',
  dslSequentialFullE2: '94086f5b62ed6abd89265754572f838e3c024226666d99ceb730f4537e0c8ec2',
};
const records = [];
const mismatches = [];
const attemptedCases = [];
let activeCase = null, activeStage = 'build', shim;
const checks = Object.fromEntries(['nativePrechangeToCurrent', 'nativeCaptureOffToOn', 'wasmPrechangeToCurrent', 'wasmCaptureOffToOn', 'wasmRepeat', 'wasmColumnsToCompanion', 'wasmPrechangeColumns'].map((key) => [key, 0]));
function beginCase(name) { activeCase = name; activeStage = 'execution'; attemptedCases.push(name); }
function checked(name, fn) { activeStage = name; fn(); checks[name]++; activeStage = 'case-invariants'; }

function save(label, suffix, value) {
  if (output) writeFileSync(join(output, `${label}.${suffix}`), typeof value === 'string' ? value : JSON.stringify(value));
}
function floatBits(value) {
  const bytes = new ArrayBuffer(8), data = new DataView(bytes);
  data.setFloat64(0, value, false);
  return data.getBigUint64(0, false).toString(16).padStart(16, '0');
}
function exactLeaves(native, wasm, path = '$') {
  if (Object.is(native, wasm)) return [];
  if (Array.isArray(native) && Array.isArray(wasm)) {
    const leaves = native.length === wasm.length ? [] : [{ path: `${path}.length`, native: native.length, wasm: wasm.length }];
    for (let i = 0; i < Math.max(native.length, wasm.length); i++) leaves.push(...exactLeaves(native[i], wasm[i], `${path}[${i}]`));
    return leaves;
  }
  if (native !== null && wasm !== null && typeof native === 'object' && typeof wasm === 'object' && !Array.isArray(native) && !Array.isArray(wasm)) {
    return [...new Set([...Object.keys(native), ...Object.keys(wasm)])].sort().flatMap((key) => exactLeaves(native[key], wasm[key], `${path}.${key}`));
  }
  const leaf = { path, native: native === undefined ? { missing: true } : native, wasm: wasm === undefined ? { missing: true } : wasm };
  if (typeof native === 'number' && typeof wasm === 'number') {
    const delta = native - wasm;
    Object.assign(leaf, { nativeMinusWasm: Number.isFinite(delta) ? delta : String(delta), nativeSign: Math.sign(native), wasmSign: Math.sign(wasm), nativeBits: floatBits(native), wasmBits: floatBits(wasm) });
  }
  return [leaf];
}
function classifications(result) {
  try { return classifyResult(result); }
  catch (error) { return { outcome: 'invalid-envelope', shapeError: error.message }; }
}
function classifyResult(result) {
  if (result.error) return { outcome: 'refusal', error: result.error };
  const run = result.run ?? (Array.isArray(result.trades) ? result : null);
  if (!run) return null;
  const snapshot = {
    outcome: result.metrics ? 'companion-success' : 'generic-success',
    identity: result.identity ?? null,
    runIdentity: Object.fromEntries(['schema', 'case', 'strategyId', 'symbol', 'timeframe', 'higherTimeframe', 'rangeMethod', 'costs'].map((key) => [key, run[key] ?? null])),
    runTradeCount: run.tradeCount,
    tradeOrder: run.trades.map((trade, index) => ({ index, ...Object.fromEntries(['entryIndex', 'exitIndex', 'entryT', 'exitT', 'side', 'reason', 'rule', 'tag', 'partial'].map((key) => [key, trade[key] ?? null])) })),
  };
  const audit = run.sequentialFull;
  snapshot.sequentialFull = audit ? {
    ...Object.fromEntries(['schema', 'profile', 'policy', 'symbol', 'timeframe'].map((key) => [key, audit[key] ?? null])),
    opportunities: audit.opportunities.map((opportunity, index) => ({ index,
      ...Object.fromEntries(['id', 'episodeId', 'trigger', 'side', 'setupIndex', 'setupFirstIndex', 'decisionIndex', 'decisionOpenMs', 'decisionMs', 'nextOpenIndex', 'status', 'reason', 'fillIndex', 'capBinds'].map((key) => [key, opportunity[key] ?? null])),
    })),
  } : null;
  if (!result.metrics) return snapshot;
  const h = result.metrics.headline;
  const signs = result.metrics.tradeAccounting.map((trade) => Math.sign(trade.netPnl));
  return { ...snapshot,
    trades: h.trades, tradeNetSigns: signs,
    accountNetSign: Math.sign(h.net), returnPctSign: Math.sign(h.returnPct), expectancySign: h.expectancy === null ? null : Math.sign(h.expectancy),
    wins: signs.filter((sign) => sign > 0).length, nonWinners: signs.filter((sign) => sign <= 0).length,
    winRate: h.winRate, maxWinStreak: h.maxWinStreak, maxLossStreak: h.maxLossStreak,
    nullsAndReasons: Object.fromEntries(['winRate', 'profitFactor', 'expectancy', 'avgWin', 'avgLoss', 'avgHoldBars'].map((field) => [field, { isNull: h[field] === null, reason: h[`${field}Reason`] ?? null }])),
  };
}
function compareTargets(name, surface, nativeRaw, wasmRaw) {
  if (nativeRaw === wasmRaw) return;
  const parse = (raw) => { try { return JSON.parse(raw); } catch (error) { return { error: { code: 'INVALID_JSON_OUTPUT', message: error.message }, rawSha256: sha(raw) }; } };
  const native = parse(nativeRaw), wasm = parse(wasmRaw);
  const leaves = exactLeaves(native, wasm);
  if (!leaves.length) leaves.push({ path: '$serialization', native: sha(nativeRaw), wasm: sha(wasmRaw) });
  const nativeClasses = classifications(native), wasmClasses = classifications(wasm);
  const categoryLeaves = exactLeaves(nativeClasses, wasmClasses);
  const numericSignChangeLeaves = leaves.filter((leaf) => typeof leaf.native === 'number' && typeof leaf.wasm === 'number' && Math.sign(leaf.native) !== Math.sign(leaf.wasm));
  const mismatch = { name, surface, nativeSha256: sha(nativeRaw), wasmSha256: sha(wasmRaw), categoricalMismatch: categoryLeaves.length > 0 || numericSignChangeLeaves.length > 0, classifications: { native: nativeClasses, wasm: wasmClasses }, leaves, categoryLeaves, numericSignChangeLeaves };
  mismatches.push(mismatch);
  // One compact leaf per log line; never print two entire result envelopes.
  console.log(JSON.stringify({ kind: 'exact-cross-target-mismatch', name, surface, nativeSha256: mismatch.nativeSha256, wasmSha256: mismatch.wasmSha256, categoricalMismatch: mismatch.categoricalMismatch, classifications: mismatch.classifications, numericSignChanges: numericSignChangeLeaves.map(({ path, nativeSign, wasmSign }) => ({ path, nativeSign, wasmSign })) }));
  for (const leaf of leaves) console.log(JSON.stringify({ kind: 'exact-cross-target-leaf', name, surface, ...leaf }));
}

const fixturePath = join(scratch, 'fixture.json'), sourcePath = join(scratch, 'source.strat');
const nativeEngine = join(scratch, 'engine'), nativeDsl = join(scratch, 'dsl');
const engineWasm = join(scratch, 'engine.wasm'), dslWasm = join(scratch, 'dsl.wasm');
function nativeRun(raw, source, binary = nativeEngine, companion = true) {
  writeFileSync(fixturePath, raw); writeFileSync(sourcePath, source);
  const result = spawnSync(binary, [...(companion ? ['--sequential-backtest'] : []), fixturePath, sourcePath], { encoding: 'utf8', maxBuffer: 1 << 28 });
  assert.ifError(result.error);
  return { raw: result.stdout.trim(), status: result.status, stderr: result.stderr };
}
async function load(path) {
  const runtime = new globalThis.Go();
  const { instance } = await WebAssembly.instantiate(readFileSync(path), runtime.importObject);
  void runtime.run(instance);
}
function prepare([name, strategyId, sourceFile]) {
  const fixture = JSON.parse(readFileSync(join(root, 'conformance/run', `${name}.fixture.json`)));
  fixture.strategyId = strategyId;
  fixture.higherTimeframe = null;
  for (const key of ['source', 'contextOptions', 'htfBars']) delete fixture[key];
  const source = readFileSync(join(root, sourceFile), 'utf8');
  assert.equal(sha(source), hashes[strategyId], `${strategyId}: frozen source changed`);
  return { fixture, source };
}
function columnsUnchanged(fixture, source, run, label) {
  const meta = { schema: 'enginewasm-columnar-v1', case: fixture.case, strategyId: fixture.strategyId,
    symbol: fixture.symbol, timeframe: fixture.timeframe, rangeMethod: fixture.rangeMethod, costs: fixture.costs };
  const columns = Array.from({ length: 6 }, (_, col) => Float64Array.from(fixture.bars, (row) => row[col]));
  activeStage = 'wasmColumnsToCompanion';
  const result = globalThis.engineRunColumns(JSON.stringify(meta), source, ...columns);
  save(label, 'wasm.columns.json', { ...result, trades: Array.from(result.trades ?? []) });
  assert.equal(result.ok, true, `${label}: column export refused`);
  const { trades, ...summary } = run;
  // The pre-existing column summary spells an absent HTF as an empty string.
  summary.higherTimeframe ??= '';
  assert.deepEqual(JSON.parse(result.summaryJSON), summary, `${label}: column summary changed`);
  if (baselineColumns) {
    const before = baselineColumns(JSON.stringify(meta), source, ...columns);
    save(label, 'wasm.prechange-columns.json', { ...before, trades: Array.from(before.trades ?? []) });
    assert.equal(before.ok, true);
    checked('wasmPrechangeColumns', () => {
      for (const field of ['summaryJSON', 'stringsJSON']) assert.equal(result[field], before[field], `${label}: prechange column ${field} changed`);
      assert.deepEqual(Array.from(result.trades), Array.from(before.trades), `${label}: prechange column values changed`);
    });
    activeStage = 'wasmColumnsToCompanion';
  }
  const text = JSON.parse(result.stringsJSON);
  assert.equal(text.length, trades.length);
  const numeric = ['entry', 'entryIndex', 'entryT', 'exit', 'exitIndex', 'exitT', 'initialSl', 'initialTp', 'pnl', 'points', 'size', 'sl', 'tp'];
  trades.forEach((trade, i) => {
    numeric.forEach((field, j) => assert.equal(result.trades[i * 13 + j], trade[field], `${label}: column trade ${i}.${field}`));
    for (const field of ['side', 'reason', 'tag', 'meta']) assert.deepEqual(text[i][field], trade[field]);
  });
  checks.wasmColumnsToCompanion++;
}
let baselineWasm, baselineColumns;
function validateSuccess(result, raw, fixture, source, label, { meaningful, noTrades }) {
  assert.deepEqual(Object.keys(result).sort(), ['capabilities', 'contractVersion', 'identity', 'metrics', 'run', 'schema']);
  assert.equal(result.schema, 'strat-sequential-backtest-result-v1'); assert.equal(result.contractVersion, 1);
  assert.equal(result.identity.fixtureSha256, sha(raw)); assert.equal(result.identity.dslSha256, sha(source));
  assert.equal(result.identity.strategyId, fixture.strategyId);
  assert.equal(result.identity.barCount, fixture.bars.length);
  assert.equal(result.identity.firstBarMs, fixture.bars[0][0]); assert.equal(result.identity.lastBarMs, fixture.bars.at(-1)[0]);
  assert.deepEqual(result.capabilities, { headline: true, trades: true, equity: true, groupings: false, monthly: false, rDistribution: false, portfolio: false });
  const { metrics, run } = result;
  assert.equal(metrics.basis, 'sequential-broker-close-mtm-v1');
  assert.equal(metrics.equity.length, fixture.bars.length);
  assert.equal(metrics.tradeAccounting.length, run.tradeCount); assert.equal(metrics.headline.trades, run.tradeCount);
  assert.equal(metrics.equity.at(-1).equity, metrics.headline.endEquity);
  metrics.equity.forEach((point, index) => { assert.equal(point.index, index); assert.equal(point.t, fixture.bars[index][0]); });
  metrics.tradeAccounting.forEach((trade, index) => assert.equal(trade.index, index));
  for (const field of ['winRate', 'profitFactor', 'expectancy', 'avgWin', 'avgLoss', 'avgHoldBars']) {
    assert.equal(`${field}Reason` in metrics.headline, metrics.headline[field] === null, `${label}: invalid null reason for ${field}`);
  }
  if (meaningful) assert(run.tradeCount > 0, `${label}: expected meaningful trades`);
  if (noTrades) {
    assert.equal(run.tradeCount, 0);
    for (const field of ['net', 'returnPct', 'maxDD', 'maxDDpct', 'maxWinStreak', 'maxLossStreak']) assert.equal(metrics.headline[field], 0);
    for (const field of ['winRate', 'profitFactor', 'expectancy', 'avgHoldBars']) { assert.equal(metrics.headline[field], null); assert.equal(metrics.headline[`${field}Reason`], 'no-trades'); }
    metrics.equity.forEach((point) => assert.equal(point.equity, metrics.headline.startEquity));
  }
}
function success(label, fixture, source, { meaningful = false, noTrades = false, fixtureJSON } = {}) {
  beginCase(label);
  const raw = fixtureJSON ?? JSON.stringify(fixture);
  const native = nativeRun(raw, source);
  const wasmRaw = globalThis.engineRunSequentialFixture(raw, source);
  save(label, 'fixture.json', raw); save(label, 'source.strat', source);
  save(label, 'native.json', native.raw); save(label, 'wasm.json', wasmRaw);
  compareTargets(label, 'companion', native.raw, wasmRaw);
  activeStage = 'per-runtime-invariants';
  assert.equal(native.status, 0, `${label}: native companion failed: ${native.stderr}`);
  const result = JSON.parse(native.raw), wasmResult = JSON.parse(wasmRaw);
  validateSuccess(result, raw, fixture, source, `${label}/native`, { meaningful, noTrades });
  validateSuccess(wasmResult, raw, fixture, source, `${label}/wasm`, { meaningful, noTrades });
  // Cross-target mismatches accumulate so later cancellation scenarios run.
  // They remain unconditional qualification failures at the final gate.
  const generic = nativeRun(raw, source, nativeEngine, false);
  save(label, 'native.capture-off.json', generic.raw);
  assert.equal(generic.status, 0, generic.stderr);
  checked('nativeCaptureOffToOn', () => assert.deepEqual(JSON.parse(generic.raw), result.run, `${label}: native capture-off changed`));
  const wasmGeneric = globalThis.engineRunFixture(raw, source);
  save(label, 'wasm.capture-off.json', wasmGeneric);
  checked('wasmCaptureOffToOn', () => assert.deepEqual(JSON.parse(wasmGeneric), wasmResult.run, `${label}: WASM capture-off changed`));
  compareTargets(label, 'generic', generic.raw, wasmGeneric);
  columnsUnchanged(fixture, source, wasmResult.run, label);
  checked('wasmRepeat', () => assert.equal(globalThis.engineRunSequentialFixture(raw, source), wasmRaw, `${label}: WASM repeat run changed`));
  if (options['--baseline-engine']) {
    const baseline = nativeRun(raw, source, resolve(options['--baseline-engine']), false);
    save(label, 'native.prechange.json', baseline.raw);
    checked('nativePrechangeToCurrent', () => { assert.equal(baseline.status, 0, baseline.stderr); assert.equal(baseline.raw, generic.raw, `${label}: prechange native changed`); });
  }
  if (baselineWasm) {
    const before = baselineWasm(raw, source);
    save(label, 'wasm.prechange.json', before);
    checked('wasmPrechangeToCurrent', () => assert.equal(before, wasmGeneric, `${label}: prechange WASM changed`));
  }
  records.push({ name: label, kind: 'success', trades: result.run.tradeCount, bars: fixture.bars.length, fixtureSha256: sha(raw), sourceSha256: sha(source), nativeSha256: sha(native.raw), wasmSha256: sha(wasmRaw), classifications: { native: classifications(result), wasm: classifications(wasmResult) } });
  return result;
}
function refusal(label, raw, source, code) {
  beginCase(label);
  const native = nativeRun(raw, source);
  const wasm = globalThis.engineRunSequentialFixture(raw, source);
  save(label, 'fixture.json', raw); save(label, 'source.strat', source);
  save(label, 'native.refusal.json', native.raw); save(label, 'wasm.refusal.json', wasm);
  compareTargets(label, 'refusal', native.raw, wasm);
  activeStage = 'per-runtime-invariants';
  assert.notEqual(native.status, 0, `${label}: native accepted invalid input`);
  for (const [target, text] of [['native', native.raw], ['wasm', wasm]]) {
    const result = JSON.parse(text);
    assert.deepEqual(Object.keys(result).sort(), ['contractVersion', 'error', 'schema']);
    assert.equal(result.schema, 'strat-sequential-backtest-result-v1'); assert.equal(result.contractVersion, 1);
    assert.equal(typeof result.error.code, 'string'); assert.equal(typeof result.error.message, 'string');
    if (code) assert.equal(result.error.code, code, `${label}/${target}: refusal code`);
  }
  records.push({ name: label, kind: 'refusal', fixtureSha256: sha(raw), sourceSha256: sha(source), nativeSha256: sha(native.raw), wasmSha256: sha(wasm), nativeError: JSON.parse(native.raw).error, wasmError: JSON.parse(wasm).error });
}

function artifactHash(path) { return path && existsSync(path) ? sha(readFileSync(path)) : null; }
function emitReceipt(complete, failure = null) {
  const receipt = {
    schema: 'sequential-backtest-parity-receipt-v1', go: go(['version']).trim(), node: process.version,
    nativeArch, runtime: 'native-and-actual-node-go-wasm', comparison: 'exact; no numeric tolerance',
    prechangeNative: Boolean(options['--baseline-engine']), prechangeWasm: Boolean(baselineWasm),
    browser: 'unrun', release: 'unpublished-local-qualification',
    status: complete && !mismatches.length ? 'passed' : 'failed', complete,
    expectedCaseCount: 88, attemptedCaseCount: attemptedCases.length,
    failure: failure ? { name: failure.name, message: failure.message, code: failure.code ?? null, operator: failure.operator ?? null, activeCase, activeStage, actual: failure.actual, expected: failure.expected } : null,
    caseCount: records.length, mismatchComparisons: mismatches.length,
    mismatchCases: [...new Set(mismatches.map((mismatch) => mismatch.name))].length,
    categoricalMismatchCases: complete ? [...new Set(mismatches.filter((mismatch) => mismatch.categoricalMismatch).map((mismatch) => mismatch.name))].length : null,
    observedCategoricalMismatchCases: [...new Set(mismatches.filter((mismatch) => mismatch.categoricalMismatch).map((mismatch) => mismatch.name))].length,
    sameArchitectureChecks: Object.fromEntries(Object.entries(checks).map(([name, passedCases]) => {
      const available = name === 'nativePrechangeToCurrent' ? Boolean(options['--baseline-engine']) : name === 'wasmPrechangeToCurrent' ? Boolean(baselineWasm) : name === 'wasmPrechangeColumns' ? Boolean(baselineColumns) : true;
      return [name, { status: !available ? 'unrun' : failure && activeStage === name ? 'failed' : complete && passedCases === 64 ? 'passed' : 'incomplete', passedCases }];
    })),
    compiler: JSON.parse(go(['env', '-json', 'GOARCH', 'GOOS', 'GOHOSTARCH', 'GOHOSTOS', 'GOAMD64', 'GOEXPERIMENT', 'GOFLAGS'])),
    scriptSha256: sha(readFileSync(fileURLToPath(import.meta.url))),
    artifacts: { engineNative: artifactHash(nativeEngine), engineWasm: artifactHash(engineWasm), parserNative: artifactHash(nativeDsl), parserWasm: artifactHash(dslWasm), wasmExec: artifactHash(shim),
      ...(options['--baseline-engine'] ? { baselineEngineNative: artifactHash(options['--baseline-engine']) } : {}),
      ...(options['--baseline-wasm'] ? { baselineEngineWasm: artifactHash(options['--baseline-wasm']) } : {}) }, records, mismatches,
  };
  if (output) writeFileSync(join(output, 'receipt.json'), JSON.stringify(receipt, null, 2) + '\n');
  const { records: caseRecords, mismatches: mismatchRecords, ...summary } = receipt;
  if (summary.failure) summary.failure = { name: summary.failure.name, message: summary.failure.message.split('\n')[0], code: summary.failure.code, activeCase, activeStage };
  console.log(JSON.stringify({ kind: 'qualification-summary', ...summary }));
  if (!complete || mismatches.length) {
    console.error(`Sequential exact parity FAILED: ${receipt.mismatchCases} case(s), ${receipt.mismatchComparisons} comparison(s), ${receipt.categoricalMismatchCases} categorical case(s); ${records.length}/88 cases completed${complete ? '' : '; INCOMPLETE after hard failure'}. No tolerance was applied.`);
    process.exitCode = 1;
  }
}

try {
  if (options['--baseline-repo']) {
    const baseline = resolve(options['--baseline-repo']);
    options['--baseline-engine'] = join(scratch, 'baseline-engine');
    options['--baseline-wasm'] = join(scratch, 'baseline-engine.wasm');
    go(['build', '-buildvcs=false', '-trimpath', '-o', options['--baseline-engine'], './cmd/enginewasm'], {}, baseline);
    go(['build', '-buildvcs=false', '-trimpath', '-o', options['--baseline-wasm'], './cmd/enginewasm'], { GOOS: 'js', GOARCH: 'wasm' }, baseline);
  }
  go(['build', '-buildvcs=false', '-trimpath', '-o', nativeEngine, './cmd/enginewasm']);
  go(['build', '-buildvcs=false', '-trimpath', '-o', nativeDsl, './cmd/dslwasm']);
  go(['build', '-buildvcs=false', '-trimpath', '-o', engineWasm, './cmd/enginewasm'], { GOOS: 'js', GOARCH: 'wasm' });
  go(['build', '-buildvcs=false', '-trimpath', '-o', dslWasm, './cmd/dslwasm'], { GOOS: 'js', GOARCH: 'wasm' });
  shim = join(go(['env', 'GOROOT']).trim(), 'misc/wasm/wasm_exec.js');
  globalThis.crypto ??= webcrypto;
  createRequire(import.meta.url)(shim);
  if (options['--baseline-wasm']) { await load(resolve(options['--baseline-wasm'])); baselineWasm = globalThis.engineRunFixture; baselineColumns = globalThis.engineRunColumns; }
  await load(engineWasm); await load(dslWasm);
  assert.equal(typeof globalThis.engineRunSequentialFixture, 'function'); assert.equal(typeof globalThis.dslParse, 'function');
  const parsedSources = new Set();
  for (const descriptor of fixtures) {
    const [name, id] = descriptor;
    const { fixture, source } = prepare(descriptor);
    if (!parsedSources.has(id)) {
      beginCase(id);
      writeFileSync(sourcePath, source);
      const native = execFileSync(nativeDsl, [sourcePath], { encoding: 'utf8' }).trim();
      const wasm = globalThis.dslParse(source);
      save(id, 'native.parser.json', native); save(id, 'wasm.parser.json', wasm);
      compareTargets(id, 'parser', native, wasm);
      activeStage = 'per-runtime-invariants';
      for (const text of [native, wasm]) { const parsed = JSON.parse(text); assert.equal(parsed.ok, true); assert.deepEqual(parsed.result.errors, []); }
      parsedSources.add(id); records.push({ name: id, kind: 'parser', sha256: sha(source) });
    }
    const original = success(name, fixture, source, { meaningful: true });
    success(`${name}-costs`, { ...fixture, costs: { ...fixture.costs, feePerUnit: 0.13, slippage: 0.03, slippageBps: 0.7 } }, source, { meaningful: true });
    success(`${name}-no-trades`, { ...fixture, bars: fixture.bars.slice(0, 4), costs: { ...fixture.costs, startEquity: 0 } }, source, { noTrades: true });
    // Exercise the actual frozen source at a non-power-of-two cancellation,
    // on both trade sides. Its entry remains causal and unchanged. Legacy uses
    // its existing last-close liquidation; Full keeps its natural time exit.
    const first = original.run.trades[0];
    const near = structuredClone(fixture);
    near.costs = { ...near.costs, feePerUnit: 0, slippage: 0, slippageBps: 0 };
    const legacy = id.startsWith('dslSequentialLegacy');
    const entryPrice = near.bars[first.entryIndex][legacy ? 4 : 1];
    const sign = first.side === 'long' ? 1 : -1;
    const goal = entryPrice + sign * 0.2;
    const rawPoints = (goal - entryPrice) * sign;
    assert(rawPoints > 0);
    const exitIndex = legacy ? first.entryIndex + 1 : first.exitIndex;
    near.bars = near.bars.slice(0, exitIndex + 1);
    near.bars[exitIndex].splice(1, 4, goal, goal, goal, goal);
    const positive = success(`${name}-cancellation-control`, near, source, { meaningful: true });
    assert.equal(positive.run.tradeCount, 1);
    assert.equal(positive.run.trades[0].entryIndex, first.entryIndex);
    assert.equal(positive.run.trades[0].exitIndex, exitIndex);
    assert.equal(positive.run.trades[0].side, first.side);
    assert.equal(positive.run.trades[0].reason, legacy ? 'end-of-test' : 'time');
    const cancellationFee = rawPoints / 2;
    for (const [suffix, fee] of [['below', adjacentPositive(cancellationFee, -1)], ['at', cancellationFee], ['above', adjacentPositive(cancellationFee, 1)]]) {
      const result = success(`${name}-cancellation-${suffix}`, { ...near, costs: { ...near.costs, feePerUnit: fee } }, source, { meaningful: true });
      assert.equal(result.run.trades[0].entryIndex, first.entryIndex);
      assert.equal(result.run.trades[0].exitIndex, exitIndex);
      assert.equal(result.metrics.headline.winRate, result.metrics.tradeAccounting[0].netPnl > 0 ? 100 : 0);
    }

  }
  const { fixture: full, source } = prepare(fixtures[4]);
  const gap = structuredClone(full); for (const row of gap.bars.slice(1)) row[0] += 300000;
  refusal('full-gap', JSON.stringify(gap), source, 'unsupported-sequential-time-gap');
  refusal('full-terminal', JSON.stringify({ ...full, bars: full.bars.slice(0, 15) }), source, 'unsupported-incomplete-terminal-run');
  const raw = JSON.stringify(full);
  // Hash the original valid transport bytes, never a silent JS UTF-16 repair.
  const unicodeFixture = { ...full, case: 'unicode-λ-😀' };
  success('unicode-original', unicodeFixture, source, { meaningful: true });
  const escapedJSON = JSON.stringify(unicodeFixture).replace('λ', '\\u03bb').replace('😀', '\\ud83d\\ude00');
  success('unicode-escaped', unicodeFixture, source, { meaningful: true, fixtureJSON: escapedJSON });
  for (const [name, surrogate] of [['high', '\ud800'], ['low', '\udc00']]) {
    beginCase(`wasm-raw-${name}-surrogate`);
    const malformed = JSON.stringify({ ...full, case: 'raw-surrogate-placeholder' }).replace('raw-surrogate-placeholder', surrogate);
    const refused = globalThis.engineRunSequentialFixture(malformed, source);
    const result = JSON.parse(refused);
    assert.deepEqual(Object.keys(result).sort(), ['contractVersion', 'error', 'schema']);
    assert.equal(result.error.code, 'SEQUENTIAL_FIXTURE_INVALID'); assert.equal(result.error.field, 'fixture');
    if (output) writeFileSync(join(output, `wasm-raw-${name}-surrogate.refusal.json`), refused);
    records.push({ name: `wasm-raw-${name}-surrogate`, kind: 'wasm-transport-refusal', ...result.error });
  }
  const bad = (change) => { const fixture = structuredClone(full); change(fixture); return JSON.stringify(fixture); };
  for (const [name, input, text] of [
    ['source-mismatch', raw, source + '\n'], ['empty-source', raw, ''],
    ['source-byte-limit', raw, ' '.repeat((8 << 20) + 1)], ['profile-mismatch', bad((f) => { f.strategyId = 'dslSequentialFullE2'; }), source],
    ['duplicate-key', raw.replace('"symbol":"SYNTH"', '"symbol":"SYNTH","symbol":"SYNTH"'), source],
    ['trailing-json', raw + '{}', source], ['fractional-timestamp', raw.replace('[300000,', '[300000.00000000001,'), source],
    ['null-cell', bad((f) => { f.bars[0][1] = null; }), source], ['missing-cost', bad((f) => { delete f.costs.feePerUnit; }), source],
    ['extra-column', bad((f) => { f.bars[0].push(0); }), source], ['empty-bars', bad((f) => { f.bars = []; }), source],
    ['htf-route', bad((f) => { f.higherTimeframe = '1d'; }), source], ['context', bad((f) => { f.contextOptions = { atrLen: 14 }; }), source],
    ['calendar', bad((f) => { f.timedCalendar = null; }), source], ['unknown-field', bad((f) => { f.capture = true; }), source],
    ['overflow-number', raw.replace('"feePerUnit":0', '"feePerUnit":1e400'), source],
  ]) refusal(name, input, text);
  // An error in the same running WASM instance must not poison the next run.
  success('full-recovery', full, source, { meaningful: true });
  // Positive, all-flat and fee-flipped near-cancellation executions, preserving
  // raw sign/classification exactly. No epsilon can convert a refusal to pass.
  const profit = structuredClone(full); profit.bars.at(-1).splice(1, 4, 104, 105, 103, 104);
  for (const fee of [0, 1.9999999999999998, 2, 2.0000000000000004, 3]) {
    success(`full-fee-boundary-${fee}`, { ...profit, costs: { ...profit.costs, feePerUnit: fee } }, source, { meaningful: true });
  }
  assert.equal(records.length, 88, 'all frozen qualification cases must execute');
  emitReceipt(true);
} catch (error) {
  emitReceipt(false, error);

} finally {
  rmSync(scratch, { recursive: true, force: true });
}
