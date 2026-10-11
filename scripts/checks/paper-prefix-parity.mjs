// Actual CLI/public Go WASM transport proof on invented bars. No JS financial
// engine, approximate numeric comparison, historical run, or browser claim.
// node scripts/checks/paper-prefix-parity.mjs <heisentick> <engine.wasm> <wasm_exec.js> [evidence-dir]
import assert from 'node:assert/strict';
import { createHash, webcrypto } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const args = process.argv.slice(2);
assert(args.length === 3 || args.length === 4, 'three artifact paths and optional evidence directory required');
const [nativeCLI, wasmPath, shim, evidence] = args.map((path) => resolve(path));
const root = fileURLToPath(new URL('../../', import.meta.url));
const maxRequest = 16 << 20;
const maxOutput = 128 << 20;
const digest = (bytes) => createHash('sha256').update(bytes).digest('hex');
const clone = (value) => structuredClone(value);
// Test-only encoding of the deliberately bounded identity fixtures below.
// No financial values are computed or rounded by this helper.
const goJSON = (value) => JSON.stringify(value).replace(/[<>&\u2028\u2029]/g,
  (char) => `\\u${char.charCodeAt(0).toString(16).padStart(4, '0')}`);
const identity = (value) => digest(goJSON(value));
const hex = /^[0-9a-f]{64}$/;
const scratch = mkdtempSync(join(tmpdir(), 'paper-prefix-public-'));
const requestPath = join(scratch, 'request.json');
const sourcePath = join(scratch, 'source.strat');
if (evidence) mkdirSync(evidence, { recursive: true });

globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(shim);
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject);
void go.run(instance);
assert.equal(typeof globalThis.engineRunPaperPrefix, 'function', 'actual public export missing');
const invoke = (...values) => {
  const raw = globalThis.engineRunPaperPrefix(...values);
  assert.equal(typeof raw, 'string');
  assert(Buffer.byteLength(raw) <= maxOutput, 'unbounded public output');
  return { raw, value: JSON.parse(raw) };
};
const native = (raw) => {
  writeFileSync(requestPath, raw);
  return spawnSync(nativeCLI, ['paper-prefix', `--request-file=${requestPath}`],
    { encoding: 'utf8', timeout: 30000, maxBuffer: maxOutput + 4096 });
};
const nativeSuccess = (raw) => {
  const result = native(raw);
  assert.equal(result.error, undefined);
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.stderr, '');
  assert(result.stdout.endsWith('\n'));
  return { raw: result.stdout.slice(0, -1), value: JSON.parse(result.stdout) };
};

const base = 1735689600000;
const orbSource = readFileSync(join(root, 'conformance/parse/setup-opening-range-breakout.strat'), 'utf8');
const legacySource = readFileSync(join(root, 'conformance/parse/setup-legacy-setup9.strat'), 'utf8');
const orb = {
  schema: 'heisentick-paper-prefix-request-v1', source: orbSource,
  route: { strategyId: 'authored-orb', symbol: 'XAUUSD', timeframe: '5m', rangeMethod: 'zone' },
  units: { quoteCurrency: 'USD', priceUnit: 'USD/unit', quantityUnit: 'unit', multiplier: 1 },
  costs: { startEquity: 5000, feePerUnit: 0.125, fillOn: 'close', slippage: 0.25, slippageBps: 0 },
  activationFromT: base, closedThroughT: base + 1800000,
  bars: [
    [base, 100, 100.5, 99.5, 100, 1],
    [base + 300000, 100, 100.5, 99.5, 100, 1],
    [base + 600000, 100, 100.5, 99.5, 100, 1],
    [base + 900000, 100, 101.5, 99.75, 101, 1],
    [base + 1200000, 101, 102.5, 100.75, 102, 1],
    [base + 1500000, 102, 104, 101.5, 103, 1],
  ],
};
function prefix(request, count) {
  const next = clone(request);
  next.bars = next.bars.slice(0, count);
  const duration = { '1m': 60000, '5m': 300000, '15m': 900000, '30m': 1800000, '1h': 3600000, '4h': 14400000, '1d': 86400000 }[next.route.timeframe];
  assert(duration, 'fixture timeframe has no declared duration');
  next.closedThroughT = next.bars.at(-1)[0] + duration;
  delete next.checkpoint;
  return next;
}
const inspectCache = new Map();
function inspect(source) {
  if (inspectCache.has(source)) return inspectCache.get(source);
  writeFileSync(sourcePath, source);
  const process = spawnSync(nativeCLI, ['inspect-sequential-source', `--dsl-file=${sourcePath}`], { encoding: 'utf8', timeout: 30000 });
  assert.equal(process.status, 0, process.stderr);
  const pair = [JSON.parse(process.stdout), JSON.parse(globalThis.engineInspectSequentialSource(source))];
  assert.deepEqual(pair[0].configIdentity, pair[1].configIdentity);
  inspectCache.set(source, pair);
  return pair;
}

function validateIdentity(result, request, producer, priorEvents = []) {
  assert.deepEqual(Object.keys(result).sort(), ['schema', 'mode', 'binding', 'bindingDigest', 'inputDigest', 'processedBars',
    'lastBarT', 'closedThroughT', 'eventRange', 'events', 'account', 'completedTrades', 'openPositions', 'pending', 'checkpoint'].sort());
  assert.equal(result.schema, 'heisentick-paper-prefix-v1');
  assert.equal(result.mode, 'replay-verified');
  assert.deepEqual(result.binding, {
    contract: result.schema, source: { sha256: digest(request.source), utf8Bytes: Buffer.byteLength(request.source) },
    configIdentity: producer.configIdentity, route: request.route, units: request.units, costs: request.costs,
    activationFromT: request.activationFromT, producer: producer.producer,
  });
  assert.equal(result.bindingDigest, identity(result.binding));
  assert.equal(result.inputDigest, identity({ bars: request.bars, closedThroughT: request.closedThroughT }));
  assert.equal(result.processedBars, request.bars.length);
  assert.equal(result.lastBarT, request.bars.at(-1)[0]);
  assert.equal(result.closedThroughT, request.closedThroughT);
  assert.equal(result.eventRange.from, priorEvents.length);
  assert.equal(result.eventRange.to, priorEvents.length + result.events.length);
  assert.deepEqual(result.checkpoint, {
    schema: 'heisentick-paper-replay-checkpoint-v1', bindingDigest: result.bindingDigest, inputDigest: result.inputDigest,
    processedBars: result.processedBars, lastBarT: result.lastBarT, closedThroughT: result.closedThroughT,
    eventCount: result.eventRange.to, stateDigest: result.checkpoint.stateDigest,
  });
  assert.match(result.checkpoint.stateDigest, hex);
  assert.match(result.pending.digest, hex);
  const seen = new Set();
  const positions = new Set();
  let lastIndex = -1, ordinal = 0;
  for (const event of [...priorEvents, ...result.events]) {
    assert(['entry', 'partial', 'exit', 'mark'].includes(event.kind));
    assert(Number.isSafeInteger(event.index));
    assert(event.index >= lastIndex && event.t >= request.activationFromT);
    assert.equal(event.t, request.bars[event.index][0]);
    if (event.index !== lastIndex) ordinal = 0;
    assert.equal(event.ordinal, ordinal++);
    lastIndex = event.index;
    if (event.kind === 'entry') {
      assert.equal(event.positionId, identity({ Domain: 'paper-position-v1', Binding: result.bindingDigest,
        Side: event.effect.side, Index: event.index, T: event.t, Ordinal: event.ordinal }));
      positions.add(event.positionId);
    }
    if (event.positionId) assert(positions.has(event.positionId), 'unbound position effect');
    assert.equal(event.eventId, identity({ Domain: 'paper-effect-v1', Binding: result.bindingDigest, Kind: event.kind,
      Position: event.positionId ?? '', Index: event.index, T: event.t, Ordinal: event.ordinal }));
    assert(!seen.has(event.eventId)); seen.add(event.eventId);
  }
  for (const position of result.openPositions) assert(positions.has(position.positionId));
}

// Exact cross-target comparison. Only the enumerated, separately validated
// target-bound identity fields differ. No financial/event payload is removed.
function comparable(result) {
  const copy = clone(result);
  delete copy.binding.producer.goos; delete copy.binding.producer.goarch;
  copy.bindingDigest = '<validated target binding>';
  copy.checkpoint.bindingDigest = '<validated target binding>';
  if (copy.openPositions.length) copy.checkpoint.stateDigest = '<same-target replay-verified open state>';
  for (const event of copy.events) {
    event.eventId = '<validated causal event>';
    if (event.positionId) event.positionId = '<validated causal position>';
  }
  for (const position of copy.openPositions) position.positionId = '<validated causal position>';
  return copy;
}

let comparisons = 0, refusals = 0, resumeSplits = 0;
const captures = [];
function compare(request, label, checkpoints, prior) {
  const nr = { ...request, ...(checkpoints ? { checkpoint: checkpoints[0] } : {}) };
  const wr = { ...request, ...(checkpoints ? { checkpoint: checkpoints[1] } : {}) };
  const nativeRaw = JSON.stringify(nr), wasmRaw = JSON.stringify(wr);
  const results = [nativeSuccess(nativeRaw), invoke(wasmRaw)];
  const inspectors = inspect(request.source);
  assert.equal(results[0].value.binding.producer.goos, process.platform);
  assert.equal(results[0].value.binding.producer.goarch, process.arch === 'x64' ? 'amd64' : process.arch);
  assert.equal(results[1].value.binding.producer.goos, 'js');
  assert.equal(results[1].value.binding.producer.goarch, 'wasm');
  for (let i = 0; i < 2; i++) validateIdentity(results[i].value, request, inspectors[i], prior?.[i] ?? []);
  assert.deepEqual(comparable(results[0].value), comparable(results[1].value), `${label}: exact native/WASM mismatch`);
  assert.equal(invoke(wasmRaw).raw, results[1].raw, `${label}: repeat changed public bytes`);
  comparisons++;
  captures.push({ label, requestSha256: digest(JSON.stringify(request)), nativeSha256: digest(results[0].raw), wasmSha256: digest(results[1].raw) });
  if (evidence) {
    writeFileSync(join(evidence, `${label}.request.json`), JSON.stringify(request));
    writeFileSync(join(evidence, `${label}.native.json`), results[0].raw);
    writeFileSync(join(evidence, `${label}.wasm.json`), results[1].raw);
  }
  return results.map((result) => result.value);
}

const healthyRaw = JSON.stringify(prefix(orb, 4));
const healthy = invoke(healthyRaw).raw;
const oldSource = readFileSync(join(root, 'conformance/run/family-sequential-full-e1-long.strat'), 'utf8');
const oldFixture = readFileSync(join(root, 'conformance/run/family-sequential-full-e1-long.fixture.json'), 'utf8');
const oldResult = globalThis.engineRunFixture(oldFixture, oldSource);
assert(JSON.parse(oldResult).tradeCount > 0);
function refuse(args, code, nativeRaw) {
  const result = invoke(...args);
  assert.deepEqual(result.value, { ok: false, error: { code } });
  if (nativeRaw !== undefined) {
    const result = native(nativeRaw);
    assert.equal(result.error, undefined);
    assert.equal(result.status, 1);
    assert.equal(result.stdout, '', 'native failure emitted success candidate');
    assert.equal(result.stderr, `heisentick: ${code}\n`);
  }
  assert.equal(invoke(healthyRaw).raw, healthy, 'bad invocation damaged public paper export');
  assert.equal(globalThis.engineRunFixture(oldFixture, oldSource), oldResult, 'bad invocation damaged old export');
  refusals++;
}

try {
  for (const fill of ['close', 'nextOpen']) {
    const request = clone(orb); request.costs.fillOn = fill;
    const full = compare(request, `orb-${fill}-full`);
    for (const result of full) {
      assert.equal(result.account.realizedNet, 300); assert.equal(result.account.equity, 5300);
      assert.equal(result.account.entryFees, 25); assert.equal(result.account.exitFees, 25);
      assert.equal(result.completedTrades.count, 1); assert.equal(result.completedTrades.netPnl, 300);
      assert.equal(result.completedTrades.profitFactor, null);
      assert.equal(result.completedTrades.profitFactorStatus, 'no-losses');
    }
    for (let split = 1; split <= request.bars.length; split++) {
      const first = compare(prefix(request, split), `orb-${fill}-prefix-${split}`);
      const cp = first.map((result) => result.checkpoint);
      const earlier = first.map((result) => result.events);
      const resumed = compare(request, `orb-${fill}-resume-${split}`, cp, earlier);
      for (let i = 0; i < 2; i++) {
        assert.deepEqual([...first[i].events, ...resumed[i].events], full[i].events);
        const complete = { ...resumed[i], events: full[i].events, eventRange: full[i].eventRange };
        assert.deepEqual(complete, full[i], 'checkpoint resume changed full public state');
        const retried = i === 0 ? nativeSuccess(JSON.stringify({ ...request, checkpoint: cp[i] })).value
          : invoke(JSON.stringify({ ...request, checkpoint: cp[i] })).value;
        assert.deepEqual(retried, resumed[i], 'crash retry changed output');
      }
      const repeated = compare(request, `orb-${fill}-committed-retry-${split}`, resumed.map((r) => r.checkpoint), full.map((r) => r.events));
      for (const result of repeated) assert.equal(result.events.length, 0);
      resumeSplits++;
    }
    const open = compare(prefix(request, 4), `orb-${fill}-endpoint`);
    for (const result of open) {
      assert.equal(result.completedTrades.count, 0);
      assert.equal(result.completedTrades.winRate, null);
      if (fill === 'close') {
        assert.equal(result.openPositions.length, 1); assert.equal(result.openPositions[0].entry, 101.25);
        assert.equal(result.account.realizedNet, -25); assert.equal(result.account.unrealized, -50);
        assert.equal(result.account.equity, 4925); assert.equal(result.account.maxDrawdown, 75);
      } else {
        assert.equal(result.openPositions.length, 0); assert.equal(result.pending.market, 1);
        assert.equal(result.account.fees, 0);
      }
    }
  }

  const partial = clone(orb);
  partial.source = partial.source.replace('management {', 'management {\n  partial 50% at 0.5R');
  const partialOpen = compare(prefix(partial, 5), 'partial-open');
  const partialFull = compare(partial, 'partial-full');
  const partialResume = compare(partial, 'partial-resume', partialOpen.map((r) => r.checkpoint), partialOpen.map((r) => r.events));
  for (let i = 0; i < 2; i++) {
    assert.equal(partialOpen[i].account.realizedNet, 12.5);
    assert.equal(partialOpen[i].account.equity, 5087.5);
    assert.equal(partialOpen[i].account.fees, 37.5);
    assert.equal(partialOpen[i].openPositions[0].size, 100);
    assert.equal(partialOpen[i].completedTrades.count, 0);
    assert.deepEqual(partialFull[i].events.filter((e) => e.kind !== 'mark').map((e) => e.effect), [
      { side: 'long', price: 101.25, size: 200, fee: 25, stop: 100.25, target: 103.25 },
      { side: 'long', price: 101.75, size: 100, fee: 12.5, stop: 100.25, target: 103.25, exitCredit: 37.5, reason: 'partial' },
      { side: 'long', price: 103, size: 100, fee: 12.5, stop: 100.25, target: 103.25, exitCredit: 162.5, completedNetPnl: 175, reason: 'tp' },
    ]);
    assert.equal(partialFull[i].completedTrades.netPnl, 175);
    assert.equal(partialFull[i].account.equity, 5175);
    assert.deepEqual([...partialOpen[i].events, ...partialResume[i].events], partialFull[i].events);
    assert.deepEqual(partialResume[i].checkpoint, partialFull[i].checkpoint);
  }
  resumeSplits++;

  const short = clone(orb);
  short.bars[3] = [base + 900000, 100, 100.25, 98.5, 99, 1];
  short.bars[4] = [base + 1200000, 99, 99.25, 97.5, 98, 1];
  short.bars[5] = [base + 1500000, 98, 98.5, 96, 97, 1];
  const shortFull = compare(short, 'orb-short-full');
  for (const result of shortFull) {
    assert.equal(result.events.find((e) => e.kind === 'entry').effect.side, 'short');
    assert.equal(result.account.realizedNet, 300); assert.equal(result.completedTrades.netPnl, 300);
  }
  // Deliberately binary-exact Bps fixture: validates nonzero transport/options
  // without importing an approximate cross-target arithmetic oracle into JS.
  const bps = clone(orb); bps.costs.slippageBps = 625;
  const bpsFull = compare(bps, 'orb-nonzero-bps');
  for (const result of bpsFull) {
    assert.equal(result.binding.costs.slippageBps, 625);
    assert(result.account.fees > 0);
    assert.notEqual(result.events.find((e) => e.kind === 'entry').effect.price, 101.25);
  }

  const warm = clone(orb); warm.activationFromT = base + 1200000;
  warm.bars[4] = [base + 1200000, 100, 100.5, 99.5, 100, 1];
  warm.bars[5] = [base + 1500000, 100, 101.5, 99.75, 101, 1];
  const warmup = compare(prefix(warm, 4), 'warmup-only');
  const active = compare(warm, 'warmup-activation', warmup.map((r) => r.checkpoint), warmup.map((r) => r.events));
  for (let i = 0; i < 2; i++) {
    assert.deepEqual(warmup[i].events, []); assert.equal(warmup[i].account.equity, 5000);
    assert.equal(warmup[i].account.fees, 0); assert.equal(active[i].openPositions[0].entryIndex, 5);
  }
  resumeSplits++;

  const legacy = clone(orb); legacy.source = legacySource; legacy.route.strategyId = 'authored-legacy';
  legacy.route.timeframe = '1h'; legacy.costs.slippage = 0;
  legacy.bars = Array.from({ length: 14 }, (_, i) => [1767571200000 + i * 3600000, 99.5 + i, 101 + i, 99 + i, 100 + i, 100]);
  legacy.activationFromT = legacy.bars[12][0]; legacy.closedThroughT = legacy.bars[13][0] + 3600000;
  const legacyOpen = compare(prefix(legacy, 13), 'legacy-open-short');
  const legacyFull = compare(legacy, 'legacy-loss');
  compare(legacy, 'legacy-resume', legacyOpen.map((r) => r.checkpoint), legacyOpen.map((r) => r.events));
  for (let i = 0; i < 2; i++) {
    assert.equal(legacyOpen[i].openPositions[0].side, 'short'); assert.equal(legacyOpen[i].openPositions[0].entryIndex, 12);
    assert.equal(legacyOpen[i].account.fees, 12.5); assert.equal(legacyFull[i].completedTrades.losses, 1);
    assert.equal(legacyFull[i].completedTrades.profitFactor, 0); assert.equal(legacyFull[i].account.equity, 4775);
  }
  resumeSplits++;

  // VEF expectations are independently specified rational fixtures checked by
  // Go tests. This transport compares every financial field exactly, rather
  // than introducing another JS financial implementation or a tolerance.
  const vwapSource = readFileSync(join(root, 'engine/testdata/paper_vwap/defensive-risk50.strat'), 'utf8');
  assert.equal(digest(vwapSource), '98abb9dc45ae9b4c4cf379ebd9981f3cd9d3bd276fac4ad6186c3c8cddc1efb4');
  const vwapCorpus = JSON.parse(readFileSync(join(root, 'engine/testdata/paper_vwap/cases.json'), 'utf8'));
  assert.equal(vwapCorpus.schema, 'paper-vwap-invented-cases-v1');
  let vwapRequest;
  for (const c of vwapCorpus.cases) {
    const request = {
      schema: orb.schema, source: vwapSource,
      route: { strategyId: 'authored-vwap', symbol: 'XAUUSD', timeframe: '15m', rangeMethod: 'zone' },
      units: { quoteCurrency: 'USD', priceUnit: 'USD-per-unit', quantityUnit: 'abstract-unit', multiplier: 1 },
      costs: c.costs, activationFromT: c.bars[32][0], closedThroughT: c.bars.at(-1)[0] + 900000, bars: c.bars,
    };
    vwapRequest ??= request;
    const full = compare(request, `vwap-${c.name}-full`);
    for (const result of full) {
      assert.equal(result.events.find((e) => e.kind === 'entry').effect.side, c.side);
      assert.equal(result.events.filter((e) => e.kind === 'partial').length, 0);
      assert.equal(result.completedTrades.count, c.expected.state === 'closed' ? 1 : 0);
      if (c.expected.state === 'closed') assert.equal(result.events.find((e) => e.kind === 'exit').effect.reason, c.expected.reason);
      else assert.equal(result.openPositions.length, 1);
    }
    const splits = [...new Set([1, 32, 33, request.bars.length - 1, request.bars.length])].filter((n) => n > 0 && n <= request.bars.length);
    for (const split of splits) {
      const first = compare(prefix(request, split), `vwap-${c.name}-prefix-${split}`);
      const resumed = compare(request, `vwap-${c.name}-resume-${split}`, first.map((r) => r.checkpoint), first.map((r) => r.events));
      for (let i = 0; i < 2; i++) {
        assert.deepEqual([...first[i].events, ...resumed[i].events], full[i].events);
        assert.deepEqual({ ...resumed[i], events: full[i].events, eventRange: full[i].eventRange }, full[i]);
      }
      resumeSplits++;
    }
  }
  const vwapOpen = compare(vwapRequest, 'vwap-receipt-open');
  const stateCase = vwapCorpus.stateCase;
  const stateRequest = { ...clone(vwapRequest), bars: stateCase.bars, costs: stateCase.costs,
    closedThroughT: stateCase.bars.at(-1)[0] + 900000 };
  const stateFull = compare(stateRequest, 'vwap-seen-reset-full');
  for (const result of stateFull) assert.deepEqual(result.events.filter((e) => e.kind === 'entry').map((e) => e.index), stateCase.entryIndexes);
  for (const split of stateCase.splits) {
    const first = compare(prefix(stateRequest, split), `vwap-seen-reset-prefix-${split}`);
    const resumed = compare(stateRequest, `vwap-seen-reset-resume-${split}`, first.map((r) => r.checkpoint), first.map((r) => r.events));
    for (let i = 0; i < 2; i++) assert.deepEqual({ ...resumed[i], events: [...first[i].events, ...resumed[i].events], eventRange: stateFull[i].eventRange }, stateFull[i]);
    resumeSplits++;
  }
  for (const [mutate, code] of [
    [(r) => { r.costs.fillOn = 'nextOpen'; }, 'PAPER_PREFIX_FILL_UNSUPPORTED'],
    [(r) => { r.route.rangeMethod = 'pivot'; }, 'PAPER_PREFIX_ADMISSION'],
    [(r) => { r.units.quoteCurrency = 'EUR'; }, 'PAPER_PREFIX_ADMISSION'],
    [(r) => { r.source += '\nmanagement { partial 0 at 1R }'; }, 'PAPER_PREFIX_ADMISSION'],
    [(r) => { r.source += '\ntarget { take profit at opposite range edge }'; }, 'PAPER_PREFIX_ADMISSION'],
    [(r) => { r.source += '\nmanagement { trail 1 ATR trail off }'; }, 'PAPER_PREFIX_ADMISSION'],
    [(r) => { r.source += '\nfilters { approach 0 ATR side both }'; }, 'PAPER_PREFIX_ADMISSION'],
    [(r) => { r.source += '\nmanagement { breakeven after 1R trail off }'; }, 'PAPER_PREFIX_ADMISSION'],
    [(r) => { r.source += '\nmanagement { breakeven off } partial 0.5 at 1R'; }, 'PAPER_PREFIX_ADMISSION'],
    [(r) => { r.source += '\nmarket conditions { weekday monday sessions london }'; }, 'PAPER_PREFIX_ADMISSION'],
    [(r) => { r.source += '\n{}'; }, 'PAPER_PREFIX_ADMISSION'],
    [(r) => { r.source += '\nmanagement { trail off } }'; }, 'PAPER_PREFIX_ADMISSION'],
    [(r) => { r.source += '\nfilters { ?! side both }'; }, 'PAPER_PREFIX_ADMISSION'],
    [(r) => { r.source += '\nfilters { side both } ()'; }, 'PAPER_PREFIX_ADMISSION'],
  ]) {
    const bad = clone(vwapRequest); mutate(bad); const raw = JSON.stringify(bad); refuse([raw], code, raw);
  }
  // Independent review's generalized dropped-source matrix. Parse semantics
  // stay unchanged; admission must now refuse every negative source on both targets.
  for (const extra of [
    "filters { approach 0 ATR side both }",
    "management { breakeven after 1R trail off }",
    "market conditions { weekday monday sessions london }",
    "market conditions { hour 12 sessions london }",
    "market conditions { local weekday in (monday) sessions london }",
    "market conditions { seasonality off sessions london }",
    "filters { unrecognized 1 side both }",
    "management { trail off } partial 0.5 at 1R",
    "management { breakeven off } partial 0.5 at 1R",
    "filters { side both } higher timeframe must agree",
    "risk { stop size <= 2 ATR } stop 1 ATR",
    "market conditions { symbols(XAUUSD) } source timeframe 15m",
    "strategy \"innocent\" {} take profit at opposite range edge",
    "management { trail { off } }",
    "management {\n{ trail off\n}",
    "management {\ntrail off }\n}",
    "management { trail off }}",
    "management {{ trail off }",
    "filters { } approach 0 ATR",
    "market conditions {} weekday monday",
    "management { partial 0 at 1R trail off }",
    "management { trail 1 ATR trail off }",
    "target { take profit at opposite range edge target 1R }"
]) {
    const bad = clone(vwapRequest); bad.source += '\n' + extra;
    const raw = JSON.stringify(bad); refuse([raw], 'PAPER_PREFIX_ADMISSION', raw);
  }
  for (const key of ['schema', 'bindingDigest', 'inputDigest', 'stateDigest', 'processedBars', 'lastBarT', 'closedThroughT', 'eventCount']) {
    const bad = clone(vwapRequest); bad.checkpoint = clone(vwapOpen[1].checkpoint);
    bad.checkpoint[key] = typeof bad.checkpoint[key] === 'string' ? 'f'.repeat(64) : bad.checkpoint[key] + 1;
    refuse([JSON.stringify(bad)], 'PAPER_PREFIX_CHECKPOINT');
  }
  for (const mutate of [
    (r) => { r.source += '\n# changed'; }, (r) => { r.costs.feePerUnit += 0.25; },
    (r) => { r.activationFromT -= 900000; }, (r) => { r.units.quantityUnit = 'other'; },
    (r) => { r.route.strategyId = 'other'; }, (r) => { r.bars[0][5]++; },
  ]) {
    const bad = clone(vwapRequest); bad.checkpoint = clone(vwapOpen[1].checkpoint); mutate(bad);
    refuse([JSON.stringify(bad)], 'PAPER_PREFIX_CHECKPOINT');
  }
  refuse([JSON.stringify({ ...vwapRequest, checkpoint: vwapOpen[0].checkpoint })], 'PAPER_PREFIX_CHECKPOINT');

  const unicode = clone(orb); unicode.source += '\r\n# exact Ω 🌱 � source bytes\r\n';
  compare(unicode, 'valid-unicode-crlf');
  assert.notEqual(invoke(JSON.stringify(unicode)).value.binding.source.sha256, invoke(JSON.stringify(orb)).value.binding.source.sha256);
  compare({ ...orb, checkpoint: null }, 'explicit-null-checkpoint');

  for (const values of [[], [1], [1n], [Symbol('private')], [true], [() => {}], [null], [undefined], [{}], [new String(healthyRaw)], [healthyRaw, 1]]) {
    refuse(values, 'PAPER_PREFIX_ARGUMENTS');
  }
  for (const bad of ['\ud800', '\udfff', '\ud800x', '\udc00\ud800', '\ud800\udc00\ud800']) {
    refuse([healthyRaw.replace('"source":"', `"source":"${bad}`)], 'PAPER_PREFIX_INPUT');
  }
  const malformed = ['', '{}', 'null', '[]', healthyRaw + ' {}',
    healthyRaw.replace('"schema":', '"unknown":0,"schema":'),
    healthyRaw.replace('"schema":', '"schema":"duplicate","schema":'),
    healthyRaw.replace('"feePerUnit":0.125', '"FeePerUnit":0.125'),
    healthyRaw.replace('"feePerUnit":0.125,', ''),
    healthyRaw.replace('"feePerUnit":0.125', '"feePerUnit":null'),
    healthyRaw.replace('"feePerUnit":0.125', '"feePerUnit":1e9999'),
    healthyRaw.replace(`[[${base},`, `[[${base}.00001,`),
    healthyRaw.replace('"source":"', '"source":"\\ud800'),
    healthyRaw.replace('"source":"', '"source":"\\udfff'),
  ];
  for (const raw of malformed) refuse([raw], 'PAPER_PREFIX_INPUT', raw);
  for (const [mutate, code] of [
    [(r) => { r.costs.startEquity = 0; }, 'PAPER_PREFIX_COSTS'],
    [(r) => { r.units.multiplier = 2; }, 'PAPER_PREFIX_UNITS_UNSUPPORTED'],
    [(r) => { r.closedThroughT--; }, 'PAPER_PREFIX_BARS'],
    [(r) => { r.activationFromT++; }, 'PAPER_PREFIX_ACTIVATION'],
    [(r) => { r.route.symbol = 'WRONG'; }, 'PAPER_PREFIX_ROUTE_UNSUPPORTED'],
    [(r) => { r.source = 'private malformed source'; }, 'PAPER_PREFIX_SOURCE'],
    [(r) => { r.source += '\nfilters { higher timeframe must agree }'; }, 'PAPER_PREFIX_CONTEXT_UNSUPPORTED'],
    [(r) => { r.source = oldSource; r.route.symbol = 'SYNTH'; }, 'PAPER_PREFIX_FAMILY_UNSUPPORTED'],
  ]) {
    const bad = clone(orb); mutate(bad); const raw = JSON.stringify(bad); refuse([raw], code, raw);
  }
  const unsupportedFill = clone(legacy); unsupportedFill.costs.fillOn = 'nextOpen';
  const badFillRaw = JSON.stringify(unsupportedFill); refuse([badFillRaw], 'PAPER_PREFIX_FILL_UNSUPPORTED', badFillRaw);

  for (const key of ['schema', 'bindingDigest', 'inputDigest', 'stateDigest', 'processedBars', 'lastBarT', 'closedThroughT', 'eventCount']) {
    const bad = clone(partial); bad.checkpoint = clone(partialOpen[1].checkpoint);
    bad.checkpoint[key] = typeof bad.checkpoint[key] === 'string' ? 'f'.repeat(64) : bad.checkpoint[key] + 1;
    refuse([JSON.stringify(bad)], 'PAPER_PREFIX_CHECKPOINT');
  }
  for (const mutate of [
    (r) => { r.source += '\n# changed'; }, (r) => { r.costs.feePerUnit = 0.25; },
    (r) => { r.activationFromT += 300000; }, (r) => { r.units.quantityUnit = 'other'; },
    (r) => { r.route.strategyId = 'other'; }, (r) => { r.bars[0][5]++; },
  ]) {
    const bad = clone(partial); bad.checkpoint = clone(partialOpen[1].checkpoint); mutate(bad);
    refuse([JSON.stringify(bad)], 'PAPER_PREFIX_CHECKPOINT');
  }
  refuse([JSON.stringify({ ...partial, checkpoint: partialOpen[0].checkpoint })], 'PAPER_PREFIX_CHECKPOINT');
  const wrongTarget = native(JSON.stringify({ ...partial, checkpoint: partialOpen[1].checkpoint }));
  assert.equal(wrongTarget.status, 1); assert.equal(wrongTarget.stdout, '');
  assert.equal(wrongTarget.stderr, 'heisentick: PAPER_PREFIX_CHECKPOINT\n');

  // Original UTF-16 length can fit while its UTF-8 byte budget overflows.
  refuse(['x'.repeat(maxRequest + 1)], 'PAPER_PREFIX_INPUT_LIMIT');
  refuse(['\u0800'.repeat(Math.floor(maxRequest / 3) + 1)], 'PAPER_PREFIX_INPUT_LIMIT');
  const exact = healthyRaw + ' '.repeat(maxRequest - Buffer.byteLength(healthyRaw));
  assert.equal(Buffer.byteLength(exact), maxRequest);
  assert.equal(invoke(exact).raw, healthy, 'exact UTF-8 request budget refused');
  refuse([exact + ' '], 'PAPER_PREFIX_INPUT_LIMIT');

  console.log(JSON.stringify({ schema: 'paper-prefix-public-export-proof-v1', status: 'passed',
    runtime: 'actual-native-cli-and-node-hosted-public-go-wasm-export', export: 'engineRunPaperPrefix',
    comparisons, checkpointResumeSplits: resumeSplits, transportRefusals: refusals,
    inputMaxUTF8Bytes: maxRequest, outputMaxUTF8Bytes: maxOutput, financialComparison: 'exact-no-tolerance',
    checkpointPortability: 'refused-across-targets', endpoint: 'open', node: process.version,
    nativeProducer: inspect(orb.source)[0].producer,
    wasmProducer: JSON.parse(healthy).binding.producer,
    artifacts: { nativeCLI: digest(readFileSync(nativeCLI)), wasm: digest(readFileSync(wasmPath)), shim: digest(readFileSync(shim)) },
    captures, browser: 'unrun', historicalOrLiveData: 'none', releaseOrConsumerAdoption: 'not-claimed' }));
} finally { rmSync(scratch, { recursive: true, force: true }); }
