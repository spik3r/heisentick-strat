// Exact-source native and actual Node-hosted Go/WASM companion qualification.
// Usage: node scripts/checks/sequential-backtest-parity.mjs [--go <go binary>]
//   [--baseline-engine <prechange native engine>] [--baseline-wasm <prechange WASM>]
//   [--baseline-repo <prechange archived repository>] [--out <directory outside the repository>]
// Builds use the installed toolchain only, in an OS temporary directory. No
// release/consumer/browser or historical-outcome qualification is implied.
import assert from 'node:assert/strict';
import { createHash, webcrypto } from 'node:crypto';
import { execFileSync, spawnSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
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
  const result = globalThis.engineRunColumns(JSON.stringify(meta), source, ...columns);
  assert.equal(result.ok, true, `${label}: column export refused`);
  const { trades, ...summary } = run;
  // The pre-existing column summary spells an absent HTF as an empty string.
  summary.higherTimeframe ??= '';
  assert.deepEqual(JSON.parse(result.summaryJSON), summary, `${label}: column summary changed`);
  if (baselineColumns) {
    const before = baselineColumns(JSON.stringify(meta), source, ...columns);
    assert.equal(before.ok, true);
    for (const field of ['summaryJSON', 'stringsJSON']) assert.equal(result[field], before[field], `${label}: prechange column ${field} changed`);
    assert.deepEqual(Array.from(result.trades), Array.from(before.trades), `${label}: prechange column values changed`);
  }
  const text = JSON.parse(result.stringsJSON);
  assert.equal(text.length, trades.length);
  const numeric = ['entry', 'entryIndex', 'entryT', 'exit', 'exitIndex', 'exitT', 'initialSl', 'initialTp', 'pnl', 'points', 'size', 'sl', 'tp'];
  trades.forEach((trade, i) => {
    numeric.forEach((field, j) => assert.equal(result.trades[i * 13 + j], trade[field], `${label}: column trade ${i}.${field}`));
    for (const field of ['side', 'reason', 'tag', 'meta']) assert.deepEqual(text[i][field], trade[field]);
  });
}
let baselineWasm, baselineColumns;
function success(label, fixture, source, { meaningful = false, noTrades = false, fixtureJSON } = {}) {
  const raw = fixtureJSON ?? JSON.stringify(fixture);
  const native = nativeRun(raw, source);
  assert.equal(native.status, 0, `${label}: ${native.raw} ${native.stderr}`);
  const wasmRaw = globalThis.engineRunSequentialFixture(raw, source);
  // Intentionally exact on both supported hosts: no blanket epsilon and no
  // cross-target sign/classification exception is permitted. ARM64 rounding
  // differences halt this gate pending independently reviewed operand bounds.
  assert.equal(wasmRaw, native.raw, `${label}: native/actual WASM companion differs`);
  const result = JSON.parse(native.raw);
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
  const generic = nativeRun(raw, source, nativeEngine, false);
  assert.equal(generic.status, 0, generic.stderr);
  assert.deepEqual(JSON.parse(generic.raw), run, `${label}: native capture-off changed`);
  const wasmGeneric = globalThis.engineRunFixture(raw, source);
  assert.equal(wasmGeneric, generic.raw, `${label}: generic native/WASM changed`);
  assert.deepEqual(JSON.parse(wasmGeneric), run, `${label}: WASM capture-off changed`);
  columnsUnchanged(fixture, source, run, label);
  assert.equal(globalThis.engineRunSequentialFixture(raw, source), wasmRaw, `${label}: repeat run changed`);
  if (options['--baseline-engine']) {
    const baseline = nativeRun(raw, source, resolve(options['--baseline-engine']), false);
    assert.equal(baseline.status, 0, baseline.stderr); assert.equal(baseline.raw, generic.raw, `${label}: prechange native changed`);
  }
  if (baselineWasm) assert.equal(baselineWasm(raw, source), wasmGeneric, `${label}: prechange WASM changed`);
  if (output) {
    writeFileSync(join(output, `${label}.fixture.json`), raw);
    writeFileSync(join(output, `${label}.native.json`), native.raw);
    writeFileSync(join(output, `${label}.wasm.json`), wasmRaw);
  }
  records.push({ name: label, kind: 'success', trades: run.tradeCount, bars: fixture.bars.length, sha256: sha(native.raw), tradeNetSigns: metrics.tradeAccounting.map((t) => Math.sign(t.netPnl)) });
  return result;
}
function refusal(label, raw, source, code) {
  const native = nativeRun(raw, source);
  assert.notEqual(native.status, 0, `${label}: native accepted invalid input`);
  const wasm = globalThis.engineRunSequentialFixture(raw, source);
  assert.equal(wasm, native.raw, `${label}: native/WASM refusal differs`);
  const result = JSON.parse(wasm);
  assert.deepEqual(Object.keys(result).sort(), ['contractVersion', 'error', 'schema']);
  assert.equal(result.schema, 'strat-sequential-backtest-result-v1'); assert.equal(result.contractVersion, 1);
  assert.equal(typeof result.error.code, 'string'); assert.equal(typeof result.error.message, 'string');
  if (code) assert.equal(result.error.code, code);
  if (output) writeFileSync(join(output, `${label}.refusal.json`), wasm);
  records.push({ name: label, kind: 'refusal', ...result.error });
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
  const shim = join(go(['env', 'GOROOT']).trim(), 'misc/wasm/wasm_exec.js');
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
      writeFileSync(sourcePath, source);
      const native = execFileSync(nativeDsl, [sourcePath], { encoding: 'utf8' }).trim();
      assert.equal(globalThis.dslParse(source), native, `${id}: parser export differs`);
      const parsed = JSON.parse(native); assert.equal(parsed.ok, true); assert.deepEqual(parsed.result.errors, []);
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
  const receipt = {
    schema: 'sequential-backtest-parity-receipt-v1', go: go(['version']).trim(), node: process.version,
    nativeArch, runtime: 'native-and-actual-node-go-wasm', comparison: 'exact; no numeric tolerance',
    prechangeNative: Boolean(options['--baseline-engine']), prechangeWasm: Boolean(baselineWasm),
    browser: 'unrun', release: 'unpublished-local-qualification',
    artifacts: { engineNative: sha(readFileSync(nativeEngine)), engineWasm: sha(readFileSync(engineWasm)), parserNative: sha(readFileSync(nativeDsl)), parserWasm: sha(readFileSync(dslWasm)), wasmExec: sha(readFileSync(shim)),
      ...(options['--baseline-engine'] ? { baselineEngineNative: sha(readFileSync(options['--baseline-engine'])) } : {}),
      ...(options['--baseline-wasm'] ? { baselineEngineWasm: sha(readFileSync(options['--baseline-wasm'])) } : {}) }, records,
  };
  if (output) writeFileSync(join(output, 'receipt.json'), JSON.stringify(receipt, null, 2) + '\n');
  console.log(JSON.stringify(receipt, null, 2));
} finally {
  rmSync(scratch, { recursive: true, force: true });
}
