// Native and Go/WASM check of the legacy Setup-9 profiles against the pinned
// JavaScript oracle.
// Usage: node scripts/checks/legacy-setup9-parity.mjs \
//   <native enginewasm> <enginewasm.wasm> <native dslwasm> <dslwasm.wasm> <wasm_exec.js>
//
// What it proves, for the invented cases in engine/testdata/legacy_setup9/oracle.json:
//   1. native and WASM engine output are byte-identical;
//   2. that output equals the oracle's trades after the 15-significant-digit
//      rounding the fixture bridge applies (serialized parity);
//   3. the rejection cases fail in native and WASM alike: a profile under
//      another family, a switched family, and any fill other than close.
// It does not prove raw-bit parity (the engine test does that on the broker's
// trades), browser qualification, or equality across CPU architectures.
import assert from 'node:assert/strict';
import { webcrypto } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { createRequire } from 'node:module';

const [nativeEngine, engineWasm, nativeDsl, dslWasm, shimPath] = process.argv.slice(2);
assert(nativeEngine && engineWasm && nativeDsl && dslWasm && shimPath, 'five paths are required');
globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(resolve(shimPath));
async function load(path) {
  const go = new globalThis.Go();
  const { instance } = await WebAssembly.instantiate(readFileSync(path), go.importObject);
  void go.run(instance);
}
await load(engineWasm);
await load(dslWasm);
assert.equal(typeof globalThis.engineRunFixture, 'function');
assert.equal(typeof globalThis.dslParse, 'function');

const root = new URL('../../', import.meta.url);
const oracle = JSON.parse(readFileSync(new URL('engine/testdata/legacy_setup9/oracle.json', root), 'utf8'));
assert.equal(oracle.jsCommit, '48a1761867342494c69c77e7ecce325e10d5d935');
const scratch = mkdtempSync(join(tmpdir(), 'legacy-setup9-'));
const round15 = (x) => Number(x.toPrecision(15));
const REASON = { eod: 'end-of-test' };

const sourceFor = (profile, timeframe, riskUsd, extra = '') =>
  `dsl v7\nstrategy "legacy setup 9 parity" {\n  description "invented traces"\n}\nmarket conditions {\n  slices(XAUUSD ${timeframe})\n}\nsetup {\n  type: legacy setup 9\n  sequential profile ${profile}\n${extra}}\n${riskUsd === 200 ? '' : `risk {\n  riskUsd ${riskUsd}\n}\n`}`;
const fixtureFor = (c, costs) => ({
  schema: 'dsl-conformance-run-fixture-v1', case: c.name, strategyId: `legacy-setup9-${c.name}`, symbol: 'XAUUSD',
  timeframe: c.timeframe, higherTimeframe: '1d', rangeMethod: 'zone', contextOptions: {}, htfBars: [], bars: c.bars,
  costs: { feePerUnit: costs.feePerUnit, fillOn: costs.fillOn ?? 'close', slippage: costs.slippage, startEquity: 10000 },
  source: { bars: { kind: 'invented-legacy-setup9' }, htfBars: null },
});
function nativeRun(fixture, source) {
  writeFileSync(join(scratch, 'fixture.json'), JSON.stringify(fixture));
  writeFileSync(join(scratch, 'source.strat'), source);
  return execFileSync(resolve(nativeEngine), [join(scratch, 'fixture.json'), join(scratch, 'source.strat')], { encoding: 'utf8', maxBuffer: 1 << 28 }).trim();
}

let runs = 0, trades = 0;
try {
  for (const c of oracle.cases) {
    for (const run of c.runs) {
      const riskUsd = c.riskUsd ?? 200;
      const source = sourceFor(run.profile, c.timeframe, riskUsd);
      const fixture = fixtureFor(c, run.costs);
      const native = nativeRun(fixture, source);
      const wasm = globalThis.engineRunFixture(JSON.stringify(fixture), source);
      assert.equal(wasm, native, `${c.name}/${run.profile}: native and WASM output differ`);
      const got = JSON.parse(native).trades;
      assert.equal(got.length, run.trades.length, `${c.name}/${run.profile}/slip${run.costs.slippage}: trade count`);
      run.trades.forEach((want, k) => {
        const g = got[k];
        const where = `${c.name}/${run.profile}/slip${run.costs.slippage} trade ${k}`;
        for (const field of ['entryIndex', 'exitIndex', 'entryT', 'exitT']) assert.equal(g[field], want[field], `${where} ${field}`);
        assert.equal(g.side, want.side, `${where} side`);
        assert.equal(g.tag, want.tag, `${where} tag`);
        assert.equal(g.reason, REASON[want.reason] ?? want.reason, `${where} reason`);
        for (const field of ['entry', 'exit', 'sl', 'tp', 'size', 'points', 'pnl']) {
          assert.equal(g[field], round15(want[field]), `${where} ${field}: ${g[field]} vs JS ${want[field]}`);
        }
      });
      runs += 1;
      trades += run.trades.length;
    }
  }

  // Rejections must fail in both runtimes and never produce a zero-trade success.
  const rise = oracle.cases.find((c) => c.name === 'trace-e-rise-300');
  const base = fixtureFor(rise, { slippage: 0, feePerUnit: 0 });
  const raw = 'seq.legacy.setup9.v1';
  const rejections = {
    nextOpenFill: ['fillOn', fixtureFor(rise, { slippage: 0, feePerUnit: 0, fillOn: 'nextOpen' }), sourceFor(raw, '1h', 200)],
    openFill: ['fillOn', fixtureFor(rise, { slippage: 0, feePerUnit: 0, fillOn: 'open' }), sourceFor(raw, '1h', 200)],
    switchedMultiline: ['final setup type', base, `dsl v7\nmarket conditions { slices(XAUUSD 1h) }\nsetup {\n type: legacy setup 9\n sequential profile ${raw}\n type: opening range breakout\n}\n`],
    switchedCompact: ['final setup type', base, `dsl v7\nmarket conditions { slices(XAUUSD 1h) }\nsetup { type: legacy setup 9 sequential profile ${raw} type: opening range breakout }\n`],
    missingProfile: ['requires `sequential profile', base, 'dsl v7\nmarket conditions { slices(XAUUSD 1h) }\nsetup {\n type: legacy setup 9\n}\n'],
    unknownProfile: ['unknown sequential profile', base, sourceFor('seq.legacy.setup9.v2_i8fix', '1h', 200)],
    unsupportedSide: ['does not support authored directive', base, sourceFor(raw, '1h', 200, ' side long only\n')],
  };
  for (const [label, [expected, fixture, source]] of Object.entries(rejections)) {
    const wasm = JSON.parse(globalThis.engineRunFixture(JSON.stringify(fixture), source));
    assert.ok(typeof wasm.error === 'string' && !('trades' in wasm), `${label}: WASM did not fail: ${JSON.stringify(wasm).slice(0, 200)}`);
    assert.ok(wasm.error.includes(expected), `${label}: WASM error ${JSON.stringify(wasm.error)} lacks ${JSON.stringify(expected)}`);
    writeFileSync(join(scratch, 'fixture.json'), JSON.stringify(fixture));
    writeFileSync(join(scratch, 'source.strat'), source);
    let nativeError = '';
    try {
      execFileSync(resolve(nativeEngine), [join(scratch, 'fixture.json'), join(scratch, 'source.strat')], { stdio: 'pipe' });
    } catch (error) {
      nativeError = String(error.stderr ?? '');
    }
    assert.ok(nativeError.includes(expected), `${label}: native did not fail with ${JSON.stringify(expected)}: ${nativeError.slice(0, 200)}`);
  }

  // Parse diagnostics: native and WASM envelopes are identical and equal the committed goldens.
  const corpus = new URL('conformance/parse/', root);
  const names = readdirSync(corpus).filter((n) => /legacy-setup9.*\.strat$/.test(n)).sort();
  assert.ok(names.length >= 7, 'legacy setup 9 parse cases are missing');
  for (const file of names) {
    const name = file.replace(/\.strat$/, '');
    const source = readFileSync(new URL(file, corpus), 'utf8');
    const golden = JSON.parse(readFileSync(new URL(`${name}.cfg.json`, corpus), 'utf8'));
    writeFileSync(join(scratch, 'source.strat'), source);
    const native = execFileSync(resolve(nativeDsl), [join(scratch, 'source.strat')], { encoding: 'utf8' }).trim();
    const wasm = globalThis.dslParse(source);
    assert.equal(wasm, native, `${name}: native and WASM parse envelopes differ`);
    const { cfg, errors, warnings, diagnostics } = JSON.parse(native).result;
    assert.deepStrictEqual({ cfg, errors, warnings, diagnostics }, { cfg: golden.cfg, errors: golden.errors, warnings: golden.warnings, diagnostics: golden.diagnostics }, `${name}: differs from golden`);
    if (name.startsWith('diagnostic-')) assert.ok(errors.length > 0, `${name}: expected a parse error`);
  }
  console.log(JSON.stringify({ ok: true, runs, trades, rejections: Object.keys(rejections).length, parseCases: names.length }));
} finally {
  rmSync(scratch, { recursive: true, force: true });
}
