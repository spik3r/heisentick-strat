// Synthetic target regression over the real native and Go/WASM fixture bridges.
// Usage: node scripts/checks/failed-breakout-target-parity.mjs <native> <wasm> <wasm_exec.js>
import assert from 'node:assert/strict';
import { createHash, webcrypto } from 'node:crypto';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { createRequire } from 'node:module';
import { execFileSync } from 'node:child_process';
const [nativePath, wasmPath, shimPath] = process.argv.slice(2);
assert(nativePath && wasmPath && shimPath, 'native, WASM and Go shim paths are required');
globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(resolve(shimPath));
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject);
void go.run(instance);
assert.equal(typeof globalThis.engineRunFixture, 'function');
const base = JSON.parse(readFileSync(new URL('../../engine/testdata/failed-breakout-target.fixture.json', import.meta.url)));
const source = readFileSync(new URL('../../engine/testdata/failed-breakout-target.strat', import.meta.url), 'utf8');
const directory = mkdtempSync(join(tmpdir(), 'failed-breakout-target-'));
const digest = value => createHash('sha256').update(value).digest('hex');
const cases = [];
try {
  for (const side of ['long', 'short']) {
    for (const target of ['target 1R', 'target 2R', 'fallback 2R', 'take profit at opposite range edge']) {
      for (const costs of [{ feePerUnit: 0, slippage: 0 }, { feePerUnit: 0.01, slippage: 0.02 }]) {
        for (const fillOn of ['close', 'open']) {
          const fixture = structuredClone(base);
          if (side === 'short') fixture.bars = fixture.bars.map(([t, o, h, l, c, v]) => [t, 200-o, 200-l, 200-h, 200-c, v]);
          fixture.costs = { ...fixture.costs, ...costs, fillOn };
          const program = source.replace('target 2R', target);
          const raw = JSON.stringify(fixture);
          const fixturePath = join(directory, 'fixture.json');
          const sourcePath = join(directory, 'source.strat');
          writeFileSync(fixturePath, raw); writeFileSync(sourcePath, program);
          const native = execFileSync(resolve(nativePath), [fixturePath, sourcePath], { encoding: 'utf8' }).trim();
          const wasm = globalThis.engineRunFixture(raw, program);
          assert.equal(wasm, native, `${side}/${target}/${fillOn}/${costs.slippage}: complete output mismatch`);
          const result = JSON.parse(native);
          assert.equal(result.tradeCount, 1);
          const trade = result.trades[0];
          assert.equal(trade.side, side); assert.equal(trade.reason, 'tp');
          const reward = target.includes('2R') ? 2 : 1;
          if (fillOn === 'close' && costs.slippage === 0) {
            assert.equal(trade.entryIndex, 163); assert.equal(trade.entry, 100);
            assert.equal(trade.exitIndex, reward === 2 ? 166 : 165);
            assert(Math.abs(trade.pnl - reward * 200) < 1e-9);
          }
          cases.push({ side, target, fillOn, costs, inputSha256: digest(raw + '\n' + program), outputSha256: digest(native), exitIndex: trade.exitIndex, pnl: trade.pnl });
        }
      }
    }
    const fixture = structuredClone(base);
    fixture.bars[163][3] = 98.5;
    if (side === 'short') fixture.bars = fixture.bars.map(([t, o, h, l, c, v]) => [t, 200-o, 200-l, 200-h, 200-c, v]);
    const raw = JSON.stringify(fixture);
    writeFileSync(join(directory, 'fixture.json'), raw); writeFileSync(join(directory, 'source.strat'), source);
    const native = execFileSync(resolve(nativePath), [join(directory, 'fixture.json'), join(directory, 'source.strat')], { encoding: 'utf8' }).trim();
    assert.equal(globalThis.engineRunFixture(raw, source), native);
    const result = JSON.parse(native); assert.equal(result.tradeCount, 1);
    const trade = result.trades[0]; assert.equal(trade.exitIndex, 164);
    assert.equal(trade.exit, trade.meta[side === 'long' ? 'rangeHi' : 'rangeLo']);
    cases.push({ side, target: 'eligible opposite edge', inputSha256: digest(raw + '\n' + source), outputSha256: digest(native), exitIndex: trade.exitIndex, pnl: trade.pnl });
  }
  console.log(JSON.stringify({ schema: 'failed-breakout-target-native-wasm-v1', cases: cases.length, results: cases }, null, 2));
} finally { rmSync(directory, { recursive: true, force: true }); }
