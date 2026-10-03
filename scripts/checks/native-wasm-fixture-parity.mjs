#!/usr/bin/env node
// Fast same-build check for the fixture bridge. Browser lifecycle and memory
// qualification remain in scripts/spikes/enginewasm/browser-matrix.mjs.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdtempSync, readFileSync, rmSync, writeFileSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import { runInThisContext } from 'node:vm';

const root = resolve(import.meta.dirname, '../..');
const temp = mkdtempSync(join(tmpdir(), 'heisentick-native-wasm-parity-'));
const sha256 = (value) => createHash('sha256').update(value).digest('hex');
function command(executable, args, env = process.env) {
  const result = spawnSync(executable, args, { cwd: root, env, encoding: 'utf8', maxBuffer: 32 * 1024 * 1024 });
  if (result.status !== 0) throw new Error(`${executable} ${args.join(' ')} failed: ${result.stderr || result.stdout}`);
  return result.stdout.trim();
}

try {
  const env = { ...process.env, GOCACHE: process.env.GOCACHE || join(temp, 'go-cache') };
  const nativePath = join(temp, 'engine-native');
  const wasmPath = join(temp, 'engine.wasm');
  command('go', ['build', '-trimpath', '-buildvcs=false', '-o', nativePath, './cmd/enginewasm'], env);
  command('go', ['build', '-trimpath', '-buildvcs=false', '-o', wasmPath, './cmd/enginewasm'],
    { ...env, GOOS: 'js', GOARCH: 'wasm' });
  const goRoot = command('go', ['env', 'GOROOT'], env);
  const shim = [join(goRoot, 'lib/wasm/wasm_exec.js'), join(goRoot, 'misc/wasm/wasm_exec.js')]
    .find((path) => existsSync(path));
  if (!shim) throw new Error('Go wasm_exec.js was not found');
  runInThisContext(readFileSync(shim, 'utf8'), { filename: shim });
  const go = new globalThis.Go();
  const { instance } = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject);
  void go.run(instance);
  if (typeof globalThis.engineRunFixture !== 'function') throw new Error('engineRunFixture export missing');

  const names = [
    'family-named-level-sweep', 'money-risk-sizing', 'money-partial-exit',
    'family-down-shock-immediate-13bp',
  ];
  const cases = [];
  function compare(name, fixtureRaw, source) {
    const fixturePath = join(temp, `${name}.fixture.json`);
    const sourcePath = join(temp, `${name}.strat`);
    writeFileSync(fixturePath, fixtureRaw);
    writeFileSync(sourcePath, source);
    const native = JSON.parse(command(nativePath, [fixturePath, sourcePath], env));
    const wasm = JSON.parse(globalThis.engineRunFixture(fixtureRaw, source));
    assert.deepStrictEqual(wasm, native, `${name}: native/WASM result mismatch`);
    assert.equal(wasm.schema, 'dsl-conformance-trades-v1');
    assert.equal(wasm.tradeCount, wasm.trades.length);
    return { name, trades: wasm.tradeCount, costs: wasm.costs,
      inputSha256: sha256(fixtureRaw), sourceSha256: sha256(source), outputSha256: sha256(JSON.stringify(native)) };
  }
  for (const name of names) {
    const base = join(root, 'conformance/run', name);
    const fixtureRaw = readFileSync(`${base}.fixture.json`, 'utf8');
    const source = readFileSync(`${base}.strat`, 'utf8');
    const row = compare(name, fixtureRaw, source);
    assert.ok(row.trades > 0, `${name}: fixture has no decision to compare`);
    cases.push(row);
    if (name === 'money-risk-sizing') {
      const changed = JSON.parse(fixtureRaw);
      changed.costs = { ...changed.costs, fillOn: 'open', slippage: 0.12,
        slippageBps: 2.5, feePerUnit: 0.01, startEquity: 12000 };
      const varied = compare(`${name}-varied-costs`, JSON.stringify(changed), source);
      assert.deepStrictEqual(varied.costs, changed.costs);
      cases.push(varied);
    }
  }
  const base = join(root, 'conformance/run/family-named-level-sweep');
  const valid = JSON.parse(readFileSync(`${base}.fixture.json`, 'utf8'));
  const source = readFileSync(`${base}.strat`, 'utf8');
  for (const [name, fixture] of [
    ['bad-schema', { ...valid, schema: 'unknown-schema' }],
    ['short-row', { ...valid, bars: [...valid.bars.slice(0, 1), valid.bars[1].slice(0, 5), ...valid.bars.slice(2)] }],
  ]) {
    const raw = JSON.stringify(fixture);
    const fixturePath = join(temp, `${name}.fixture.json`);
    const sourcePath = join(temp, `${name}.strat`);
    writeFileSync(fixturePath, raw);
    writeFileSync(sourcePath, source);
    const native = spawnSync(nativePath, [fixturePath, sourcePath], { cwd: root, env, encoding: 'utf8' });
    const wasm = JSON.parse(globalThis.engineRunFixture(raw, source));
    assert.notEqual(native.status, 0, `${name}: native accepted invalid fixture`);
    assert.equal(typeof wasm.error, 'string', `${name}: WASM accepted invalid fixture`);
    assert.ok(native.stderr.includes(wasm.error), `${name}: native/WASM errors differ`);
  }
  if (typeof globalThis.engineRunInteractiveFixture !== 'function') throw new Error('interactive WASM export missing');
  const interactiveBase = join(root, 'conformance/run/family-sma-golden-cross');
  const interactiveSource = readFileSync(`${interactiveBase}.strat`, 'utf8');
  const interactiveFixture = JSON.parse(readFileSync(`${interactiveBase}.fixture.json`, 'utf8'));
  interactiveFixture.costs.feePerUnit = 0.1;
  const interactiveRaw = JSON.stringify(interactiveFixture);
  const interactiveFixturePath = join(temp, 'interactive.fixture.json');
  const interactiveSourcePath = join(temp, 'interactive.strat');
  writeFileSync(interactiveFixturePath, interactiveRaw);
  writeFileSync(interactiveSourcePath, interactiveSource);
  const nativeInteractive = JSON.parse(command(nativePath,
    ['--interactive', interactiveFixturePath, interactiveSourcePath], env));
  const wasmInteractive = JSON.parse(globalThis.engineRunInteractiveFixture(interactiveRaw, interactiveSource));
  assert.deepStrictEqual(wasmInteractive, nativeInteractive, 'interactive native/WASM result mismatch');
  assert.equal(wasmInteractive.schema, 'dsl-interactive-run-v1');
  assert.equal(wasmInteractive.equityCurve.length, interactiveFixture.bars.length);
  assert.equal(wasmInteractive.closedEquityCurve.length, interactiveFixture.bars.length);
  assert.ok(wasmInteractive.run.tradeCount > 0);
  assert.equal(wasmInteractive.stats.endEquity, wasmInteractive.cashEndEquity);
  assert.deepStrictEqual(wasmInteractive.skips, {});
  assert.equal(wasmInteractive.skipReasonSchema, 'dsl-skip-reasons-v1');
  const blockedSMASource = interactiveSource.replace('  side long only',
    '  side long only\n  rmv atr period 1\n  rmv lookback 3\n  rmv below 0');
  assert.notEqual(blockedSMASource, interactiveSource);
  writeFileSync(interactiveSourcePath, blockedSMASource);
  const nativeBlockedSMA = JSON.parse(command(nativePath,
    ['--interactive', interactiveFixturePath, interactiveSourcePath], env));
  const wasmBlockedSMA = JSON.parse(globalThis.engineRunInteractiveFixture(interactiveRaw, blockedSMASource));
  assert.deepStrictEqual(wasmBlockedSMA, nativeBlockedSMA, 'SMA RMV rejection parity');
  assert.ok((wasmBlockedSMA.skips['gate.rmv_threshold'] || 0) > 0);
  assert.equal(wasmBlockedSMA.run.tradeCount, 0);
  const dualBase = join(root, 'conformance/run/deployed-dsl-dual-ema-resumption-xauusd-four-hour');
  const dualFixtureRaw = readFileSync(`${dualBase}.fixture.json`, 'utf8');
  const dualSource = readFileSync(`${dualBase}.strat`, 'utf8');
  writeFileSync(interactiveFixturePath, dualFixtureRaw);
  writeFileSync(interactiveSourcePath, dualSource);
  const nativeDual = JSON.parse(command(nativePath,
    ['--interactive', interactiveFixturePath, interactiveSourcePath], env));
  const wasmDual = JSON.parse(globalThis.engineRunInteractiveFixture(dualFixtureRaw, dualSource));
  const dualNumericDiffs = [];
  function compareDual(a, b, path = '', numericDiffs = dualNumericDiffs) {
    if (typeof a === 'number' && typeof b === 'number') {
      if (a !== b) numericDiffs.push({ path, abs: Math.abs(a - b), a, b });
      return;
    }
    if (Array.isArray(a) && Array.isArray(b)) {
      assert.equal(a.length, b.length, `${path} length`);
      a.forEach((item, index) => compareDual(item, b[index], `${path}[${index}]`, numericDiffs));
      return;
    }
    if (a && b && typeof a === 'object' && typeof b === 'object') {
      assert.deepStrictEqual(Object.keys(a).sort(), Object.keys(b).sort(), `${path} keys`);
      for (const key of Object.keys(a)) compareDual(a[key], b[key], path ? `${path}.${key}` : key, numericDiffs);
      return;
    }
    assert.deepStrictEqual(a, b, `${path} nonnumeric value`);
  }
  compareDual(wasmDual, nativeDual);
  const dualRoundingFields = [
    [/^(?:equityCurve|closedEquityCurve)\[\d+\]$/, 1e-9],
    [/^run\.trades\[\d+\]\.pnl$/, 1e-9],
    [/^tradeNetPnl\[\d+\]$/, 1e-9],
    [/^run\.trades\[\d+\]\.points$/, 1e-10],
    [/^run\.trades\[\d+\]\.meta\.signalAtr$/, 1e-12],
    [/^stats\.(?:tradeNet|maxDD|maxClosedDD)$/, 1e-9],
    [/^stats\.(?:maxDDpct|maxClosedDDpct)$/, 1e-10],
  ];
  assert.ok(dualNumericDiffs.every((difference) => {
    const tolerance = dualRoundingFields.find(([rule]) => rule.test(difference.path))?.[1];
    return tolerance !== undefined && Number.isFinite(difference.abs) && difference.abs <= tolerance;
  }), 'dual EMA changed a decision, unlisted field, or derived value beyond its explicit budget');
  assert.equal(wasmDual.schema, 'dsl-interactive-run-v1');
  assert.ok(wasmDual.run.tradeCount > 0);
  assert.equal(wasmDual.equityCurve.length, 5000);
  assert.equal(wasmDual.closedEquityCurve.length, 5000);
  assert.deepStrictEqual(wasmDual.skips, {});
  const namedBase = join(root, 'conformance/run/research-dsl-daily-snd-retest-xauusd-4h');
  const namedFixtureRaw = readFileSync(`${namedBase}.fixture.json`, 'utf8');
  const namedSource = readFileSync(`${namedBase}.strat`, 'utf8');
  writeFileSync(interactiveFixturePath, namedFixtureRaw);
  writeFileSync(interactiveSourcePath, namedSource);
  const nativeNamed = JSON.parse(command(nativePath,
    ['--interactive', interactiveFixturePath, interactiveSourcePath], env));
  const wasmNamed = JSON.parse(globalThis.engineRunInteractiveFixture(namedFixtureRaw, namedSource));
  const namedNumericDiffs = [];
  compareDual(wasmNamed, nativeNamed, '', namedNumericDiffs);
  assert.ok(namedNumericDiffs.every((difference) => {
    const tolerance = dualRoundingFields.find(([rule]) => rule.test(difference.path))?.[1];
    return tolerance !== undefined && Number.isFinite(difference.abs) && difference.abs <= tolerance;
  }), 'named level sweep changed a decision, skip count, or derived value beyond its budget');
  assert.deepStrictEqual(wasmNamed.skips, {
    'gate.utc_window': 1605, 'gate.prior_day_type_allow': 1649, 'gate.movement_er': 13,
  });
  assert.equal(wasmNamed.skipReasonSchema, 'dsl-skip-reasons-v1');
  const ordinaryProfiles = [
    ['dslSmaGoldenCrossXauusdOneMinuteCanary', 'deployed-dsl-sma-golden-cross-xauusd-one-minute-canary'],
    ['dslCloseVwapExtremeMagnetDefensive', 'deployed-dsl-close-vwap-extreme-magnet-defensive'],
    ['dslGoldNamedLevelFlagBodyHalfAtr', 'family-named-level-flag', 'dslGoldNamedLevelFlagBodyHalfAtr'],
    ['dslGoldNamedLevelFlagBodyHalfAtrRiskFloor12', 'family-named-level-flag', 'dslGoldNamedLevelFlagBodyHalfAtrRiskFloor12'],
    ['dslForwardTesterCanaryOrbXauusdFiveMinute', 'research-dsl-failed-breakout-five-minute-early-breakeven', 'dslForwardTesterCanaryOrbXauusdFiveMinute'],
    ['dslForwardTesterCanaryOrbBtcusdFiveMinute', 'research-dsl-failed-breakout-five-minute-early-breakeven', 'dslForwardTesterCanaryOrbBtcusdFiveMinute'],
    ['dslEditorStrategy', 'research-dsl-failed-breakout-five-minute-early-breakeven', 'dslEditorStrategy'],
  ];
  const ordinaryResults = [];
  for (const [id, fixtureName, snapshotName] of ordinaryProfiles) {
    const fixture = JSON.parse(readFileSync(join(root, 'conformance/run', `${fixtureName}.fixture.json`), 'utf8'));
    fixture.strategyId = id;
    if (fixtureName === 'family-named-level-flag') {
      fixture.bars.unshift([Date.UTC(2023, 11, 25), 100, 110, 95, 100, 1]);
    }
    if (id === 'dslForwardTesterCanaryOrbBtcusdFiveMinute') {
      fixture.symbol = 'BTCUSD';
      fixture.bars.forEach((bar) => { bar[0] += 5 * 24 * 60 * 60 * 1000; });
    }
    const fixtureRaw = JSON.stringify(fixture);
    const profileSource = readFileSync(snapshotName
      ? join(root, 'engine/testdata/interactive', `${snapshotName}.strat`)
      : join(root, 'conformance/run', `${fixtureName}.strat`), 'utf8');
    writeFileSync(interactiveFixturePath, fixtureRaw);
    writeFileSync(interactiveSourcePath, profileSource);
    const nativeProfile = JSON.parse(command(nativePath,
      ['--interactive', interactiveFixturePath, interactiveSourcePath], env));
    const wasmProfile = JSON.parse(globalThis.engineRunInteractiveFixture(fixtureRaw, profileSource));
    const numericDiffs = [];
    compareDual(wasmProfile, nativeProfile, '', numericDiffs);
    assert.equal(wasmProfile.schema, 'dsl-interactive-run-v1', `${id}: unexpected schema`);
    assert.ok(wasmProfile.run.tradeCount > 0, `${id}: no whole-strategy trades`);
    assert.equal(wasmProfile.equityCurve.length, fixture.bars.length, `${id}: incomplete marks`);
    assert.equal(wasmProfile.run.strategyId, id, `${id}: wrong strategy ID`);
    assert.deepStrictEqual(wasmProfile.skips, nativeProfile.skips, `${id}: skip mismatch`);
    assert.ok(numericDiffs.every((difference) => {
      const tolerance = dualRoundingFields.find(([rule]) => rule.test(difference.path))?.[1];
      return tolerance !== undefined && Number.isFinite(difference.abs) && difference.abs <= tolerance;
    }), `${id}: native/WASM decision or derived value differs beyond budget: ${JSON.stringify(numericDiffs.slice(0, 3))}`);
    ordinaryResults.push({ id, tradeCount: wasmProfile.run.tradeCount, numericDifferences: numericDiffs.length,
      maxNumericAbsDrift: Math.max(0, ...numericDiffs.map((difference) => difference.abs)),
      nativeOutputSha256: sha256(JSON.stringify(nativeProfile)), wasmOutputSha256: sha256(JSON.stringify(wasmProfile)) });
    const wrongRouteRaw = JSON.stringify({ ...fixture, timeframe: '1d' });
    writeFileSync(interactiveFixturePath, wrongRouteRaw);
    const nativeWrongRoute = JSON.parse(command(nativePath,
      ['--interactive', interactiveFixturePath, interactiveSourcePath], env));
    const wasmWrongRoute = JSON.parse(globalThis.engineRunInteractiveFixture(wrongRouteRaw, profileSource));
    assert.deepStrictEqual(wasmWrongRoute, nativeWrongRoute, `${id}: route rejection parity`);
    assert.equal(wasmWrongRoute.error.code, 'unsupported-route', `${id}: wrong timeframe was admitted`);
    writeFileSync(interactiveFixturePath, fixtureRaw);
    if (id === 'dslEditorStrategy') {
      const mutated = profileSource.replace('by 0.1 ATR', 'by 0.2 ATR');
      assert.notEqual(mutated, profileSource, 'editor mutation did not alter the source');
      writeFileSync(interactiveSourcePath, mutated);
      const nativeRejected = JSON.parse(command(nativePath,
        ['--interactive', interactiveFixturePath, interactiveSourcePath], env));
      const wasmRejected = JSON.parse(globalThis.engineRunInteractiveFixture(fixtureRaw, mutated));
      assert.deepStrictEqual(wasmRejected, nativeRejected, 'mutated editor rejection parity');
      assert.equal(wasmRejected.error.code, 'unsupported-route', 'mutated editor inherited default capability');
    }
  }
  const unsupported = { ...interactiveFixture, sourceBars: [interactiveFixture.bars[0]] };
  const unsupportedRaw = JSON.stringify(unsupported);
  writeFileSync(interactiveFixturePath, unsupportedRaw);
  writeFileSync(interactiveSourcePath, interactiveSource);
  const nativeUnsupported = JSON.parse(command(nativePath,
    ['--interactive', interactiveFixturePath, interactiveSourcePath], env));
  const wasmUnsupported = JSON.parse(globalThis.engineRunInteractiveFixture(unsupportedRaw, interactiveSource));
  assert.deepStrictEqual(wasmUnsupported, nativeUnsupported);
  assert.equal(wasmUnsupported.error.code, 'unsupported-route');
  console.log(JSON.stringify({ schema: 'native-wasm-fixture-parity-v1',
    source: { commit: command('git', ['rev-parse', 'HEAD']),
      dirty: Boolean(command('git', ['status', '--porcelain', '--untracked-files=normal'])) },
    artifact: { wasmSha256: sha256(readFileSync(wasmPath)), shimSha256: sha256(readFileSync(shim)) },
    cases, invalidCases: ['bad-schema', 'short-row'],
    interactiveResult: { route: 'chart-timeframe-sma-golden-cross',
      tradeCount: wasmInteractive.run.tradeCount,
      bars: wasmInteractive.equityCurve.length,
      outputSha256: sha256(JSON.stringify(nativeInteractive)),
      unsupportedCode: wasmUnsupported.error.code },
    dualEMAResult: { route: 'chart-timeframe-dual-ema-resumption',
      tradeCount: wasmDual.run.tradeCount, bars: wasmDual.equityCurve.length,
      nativeOutputSha256: sha256(JSON.stringify(nativeDual)),
      wasmOutputSha256: sha256(JSON.stringify(wasmDual)),
      numericDifferences: dualNumericDiffs.length,
      maxMarkedEquityAbsDrift: Math.max(0, ...dualNumericDiffs.map((difference) => difference.abs)) },
    skipReasonResults: { blockedSMA: wasmBlockedSMA.skips, namedLevelSweep: wasmNamed.skips,
      namedNumericDifferences: namedNumericDiffs.length }, ordinaryResults }, null, 2));
} catch (error) {
  console.error(error);
  process.exitCode = 1;
} finally {
  rmSync(temp, { recursive: true, force: true });
  // The Go WASM runtime waits for work indefinitely after registering its
  // exports. Node needs an explicit exit after this bounded command.
  process.exit(process.exitCode || 0);
}
