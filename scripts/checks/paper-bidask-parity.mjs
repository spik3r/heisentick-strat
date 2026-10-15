// Actual native CLI versus the public Go WASM export, on invented observations.
// No JS financial engine, tolerance, output-derived oracle, provider, or consumer.
// node scripts/checks/paper-bidask-parity.mjs <heisentick> <engine.wasm> <wasm_exec.js> [evidence-dir]
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
const maxBytes = 64 << 20;
const maxOutputBytes = 128 << 20;
const maxRecords = 65536;
const schema = 'heisentick-paper-bidask-v1';
const hash = (bytes) => createHash('sha256').update(bytes).digest('hex');
const clone = (value) => structuredClone(value);
// Identity encoding only. Fixtures contain strings and bounded integers; this
// does not implement quantity, price, fee, P&L, or family arithmetic.
const goJSON = (value) => JSON.stringify(value).replace(/[<>&\u2028\u2029]/g,
  (char) => `\\u${char.charCodeAt(0).toString(16).padStart(4, '0')}`);
const identity = (value) => hash(goJSON(value));
const hex = /^[0-9a-f]{64}$/;
const scratch = mkdtempSync(join(tmpdir(), 'paper-bidask-public-'));
const requestPath = join(scratch, 'request.json');
const sourcePath = join(scratch, 'source.strat');
if (evidence) mkdirSync(evidence, { recursive: true });

globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(shim);
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject);
void go.run(instance);
const oldExports = ['engineRunSequentialFixture', 'engineRunFixture', 'engineRunClockRangeFixture',
  'engineRunR11Fixture', 'engineRunColumns', 'engineRunMasterPortableReport',
  'engineRunAdaptiveFlagReport', 'engineRunAdaptiveFlagUnitReport', 'engineRunPaperPrefix',
  'engineInspectSequentialSource'];
for (const name of [...oldExports, 'engineRunPaperBidAsk']) assert.equal(typeof globalThis[name], 'function', name);
const call = globalThis.engineRunPaperBidAsk;
function wasm(...values) {
  const raw = call(...values);
  assert.equal(typeof raw, 'string');
  assert(Buffer.byteLength(raw) <= maxOutputBytes, 'unbounded public output');
  return { raw, value: JSON.parse(raw) };
}
function native(raw) {
  writeFileSync(requestPath, raw);
  const result = spawnSync(nativeCLI, ['paper-bidask', `--request-file=${requestPath}`],
    { encoding: 'utf8', timeout: 120000, maxBuffer: maxOutputBytes + 4096 });
  assert.equal(result.error, undefined);
  assert.equal(result.status, 0, result.stderr); // Refusals also exit normally.
  assert.equal(result.stderr, '');
  assert(result.stdout.endsWith('\n'), 'native must return one complete JSON line');
  assert(Buffer.byteLength(result.stdout) <= maxOutputBytes + 1);
  return { raw: result.stdout.slice(0, -1), value: JSON.parse(result.stdout) };
}

const baseNS = 1735689600000000000n;
const minute = 60000000000n;
// Clock construction is exact integer transport bookkeeping, not finance.
function stamp(offset) {
  const ns = baseNS + BigInt(offset);
  const second = ns / 1000000000n;
  const fraction = String(ns % 1000000000n).padStart(9, '0').replace(/0+$/, '');
  return new Date(Number(second) * 1000).toISOString().slice(0, 19) + (fraction ? `.${fraction}` : '') + 'Z';
}
const repeated = (character) => character.repeat(64);
const source = readFileSync(join(root, 'conformance/parse/setup-opening-range-breakout.strat'), 'utf8');
function bar(at, o = '100', h = '100.5', l = '99.5', c = '100') {
  const ohlc = { open: o, high: h, low: l, close: c };
  return { openT: stamp(at), closeT: stamp(at + 5n * minute), receivedT: stamp(at + 5n * minute + 1n),
    providerId: repeated('6'), provenanceDigest: repeated('7'), mid: clone(ohlc), bid: clone(ohlc), ask: clone(ohlc),
    priceCount: '1', complete: true };
}
function append(r, kind, payload, commitT) {
  const key = kind === 'coverage-gap' ? 'gap' : kind;
  r.records.push({ sequence: String(r.records.length + 1), epoch: r.identity.epoch, commitT, kind, [key]: payload });
}
function addBar(r, at, ...ohlc) {
  const b = bar(at, ...ohlc); append(r, 'bar', b, b.receivedT); return b;
}
function quote(r, at, bid = '101', ask = '101.2', signal = '') {
  return { providerT: stamp(at), requestStartT: stamp(at - 1000000n), receivedT: stamp(at + 1000000n),
    qualifiedT: stamp(at + 2000000n), providerId: repeated('8'), provenanceDigest: repeated('9'),
    accountDigest: r.identity.accountDigest, instrument: r.product.instrument, tradeable: true,
    bids: [{ price: bid, liquidity: '10000' }], asks: [{ price: ask, liquidity: '10000' }], episodeSignalSequence: signal };
}
function addQuote(r, at, bid, ask, signal = '') {
  const q = quote(r, at, bid, ask, signal); append(r, 'quote', q, stamp(at + 3000000n)); return q;
}
// Same public source and invented record definitions as the native CLI/replay
// fixtures. No expected output is loaded from those tests or another target.
function request(short = false, risk = '200') {
  const r = { schema: 'heisentick-paper-bidask-request-v1', source: source.replace('risk: 200 USD', `risk: ${risk} USD`),
    route: { strategyId: 'invented-public-bidask', symbol: 'XAUUSD', timeframe: '5m', rangeMethod: 'zone' },
    identity: { artifactDigest: repeated('a'), principalDigest: repeated('1'), accountDigest: repeated('2'), runId: repeated('3'), epoch: repeated('4') },
    product: { provider: 'oanda-v20', instrument: 'XAU_USD', attestationDigest: repeated('5'), accountCurrency: 'USD', priceUnit: 'USD/troy-ounce', quantityUnit: 'troy-ounce', multiplier: '1' },
    economics: { startEquity: '1000', entryFee: '0', exitFee: '0', slippageFixed: '0', slippageBps: '0', quantityQuantum: '0.25', quantityMinimum: '0.25', quantityMaximum: '10000', notionalPolicy: 'none', notionalMaximum: '0' },
    policy: { model: 'sampled-straddle-full-bar-stop-first-v1', maxQuoteAgeMs: '5000', maxSignalAgeMs: '290000', coverage: 'refuse-discontinuous-exposure', arithmetic: 'decimal-rational-family-shortest-v1' },
    activationFromT: stamp(0n), records: [] };
  for (let i = 0; i < 3; i++) addBar(r, BigInt(i) * 5n * minute);
  addBar(r, 15n * minute, ...(short ? ['100', '100.25', '98.5', '99'] : ['100', '101.5', '99.75', '101']));
  return r;
}
const entryAt = 20n * minute + 1000000000n;
function entered(short = false, risk = '200') {
  const r = request(short, risk);
  const episode = { signalSequence: '4', admittedT: stamp(20n * minute + 100000000n), deadlineT: stamp(20n * minute + 200000000000n) };
  append(r, 'episode', episode, episode.admittedT);
  addQuote(r, entryAt, short ? '98.8' : '101', short ? '99' : '101.2', '4');
  return r;
}
function prefix(r, count) { const v = clone(r); v.records = v.records.slice(0, count); delete v.checkpoint; return v; }
function phase() {
  const r = entered();
  addQuote(r, entryAt + 1000000000n, '101.3', '101.5');
  addBar(r, 20n * minute, '101', '106', '90', '101.4');
  addQuote(r, 25n * minute, '99', '99.2');
  addBar(r, 25n * minute, '101.3', '104', '99', '102');
  return r;
}

const inspectCache = new Map();
function inspect(text) {
  if (!inspectCache.has(text)) {
    writeFileSync(sourcePath, text);
    const result = spawnSync(nativeCLI, ['inspect-sequential-source', `--dsl-file=${sourcePath}`], { encoding: 'utf8', timeout: 30000 });
    assert.equal(result.status, 0, result.stderr); assert.equal(result.stderr, '');
    const pair = [JSON.parse(result.stdout), JSON.parse(globalThis.engineInspectSequentialSource(text))];
    assert.deepEqual(pair[0].configIdentity, pair[1].configIdentity);
    assert.match(pair[0].configIdentity.sha256, hex);
    inspectCache.set(text, pair);
  }
  return inspectCache.get(text);
}
// These field orders are the public typed Go document, not the input object's
// insertion order. Reject unexpected fields instead of silently excluding them.
function ordered(value, keys) {
  assert.deepEqual(Object.keys(value).sort(), [...keys].sort());
  return Object.fromEntries(keys.map((key) => [key, value[key]]));
}
function canonicalRecords(records) {
  const payloads = {
    bar: ['openT', 'closeT', 'receivedT', 'providerId', 'provenanceDigest', 'mid', 'bid', 'ask', 'priceCount', 'complete'],
    episode: ['signalSequence', 'admittedT', 'deadlineT'],
    quote: ['providerT', 'requestStartT', 'receivedT', 'qualifiedT', 'providerId', 'provenanceDigest', 'accountDigest', 'instrument', 'tradeable', 'bids', 'asks', 'episodeSignalSequence'],
    seal: ['signalSequence', 'reason', 'effectiveT'], gap: ['fromT', 'throughT', 'reason'],
  };
  return records.map((r) => {
    const key = r.kind === 'coverage-gap' ? 'gap' : r.kind;
    const out = ordered(r, ['sequence', 'epoch', 'commitT', 'kind', key]);
    out[key] = ordered(r[key], payloads[key]);
    if (key === 'bar') for (const side of ['mid', 'bid', 'ask']) out.bar[side] = ordered(out.bar[side], ['open', 'high', 'low', 'close']);
    if (key === 'quote') for (const side of ['bids', 'asks']) out.quote[side] = out.quote[side].map((v) => ordered(v, ['price', 'liquidity']));
    return out;
  });
}
const eventKeys = ['id', 'recordSequence', 'kind', 'outcome', 'decisionId', 'positionId', 'side', 'price', 'quantity', 'fee', 'netPnl', 'stop', 'target', 'protectionVersion', 'priorProtectionVersion', 'basis', 'clock'];
const clockKeys = ['signalOpenT', 'signalCloseT', 'providerT', 'requestStartT', 'receivedT', 'qualifiedT', 'commitT', 'modelEffectiveT', 'intervalOpenT', 'intervalCloseT'];
function validate(result, r, inspector, prior = []) {
  const keys = ['schema', 'mode', 'binding', 'bindingDigest', 'inputDigest', 'processedRecords', 'processedBars',
    'eventRange', 'events', 'account', 'completedTrades', 'coverage', 'stateDigest', 'checkpoint'];
  if (result.position) keys.push('position'); if (result.pending) keys.push('pending');
  ordered(result, keys);
  assert.equal(result.schema, schema); assert.equal(result.mode, 'replay-verified-candidate');
  const expectedBinding = { family: 'openingRangeBreakout', profile: 'ordinary', signalRecipe: 'provider-midpoint-ohlc-price-count-v1',
    sizingPolicy: 'source-risk-usd-total-stop-budget-v1', contract: schema,
    source: { sha256: hash(r.source), utf8Bytes: Buffer.byteLength(r.source) }, configIdentity: inspector.configIdentity,
    route: r.route, identity: r.identity, product: r.product, economics: r.economics, policy: r.policy,
    activationFromT: r.activationFromT, producer: inspector.producer };
  assert.deepEqual(result.binding, expectedBinding);
  assert.equal(result.bindingDigest, identity(expectedBinding));
  assert.equal(result.inputDigest, identity(canonicalRecords(r.records)));
  assert.equal(result.processedRecords, r.records.length);
  assert.equal(result.processedBars, r.records.filter((record) => record.kind === 'bar').length);
  assert.deepEqual(result.eventRange, { from: prior.length, to: prior.length + result.events.length });
  const events = [...prior, ...result.events];
  const decisions = new Map(), positions = new Map(), ids = new Set();
  let lastSequence = 0;
  for (const [ordinal, event] of events.entries()) {
    ordered(event, eventKeys); ordered(event.clock, clockKeys);
    assert.match(event.id, hex);
    const sequence = Number(event.recordSequence);
    assert(Number.isSafeInteger(sequence) && sequence >= lastSequence && sequence <= r.records.length);
    lastSequence = sequence;
    const record = r.records[sequence - 1];
    assert.equal(event.clock.commitT, record.commitT);
    assert.equal(event.id, identity({ Binding: result.bindingDigest, Sequence: event.recordSequence, Ordinal: ordinal, Kind: event.kind }));
    assert(!ids.has(event.id)); ids.add(event.id);
    if (event.kind === 'decision') {
      assert.equal(record.kind, 'bar');
      assert.equal(event.decisionId, identity({ Binding: result.bindingDigest, Signal: event.recordSequence, Ordinal: 0 }));
      assert.equal(event.clock.signalOpenT, record.bar.openT); assert.equal(event.clock.signalCloseT, record.bar.closeT);
      decisions.set(event.decisionId, event.recordSequence);
    }
    if (event.decisionId) assert(decisions.has(event.decisionId), 'unbound causal decision');
    if (event.kind === 'entry') {
      assert.equal(record.kind, 'quote');
      assert.equal(event.positionId, identity({ Decision: event.decisionId }));
      positions.set(event.positionId, event.decisionId);
    }
    if (event.positionId) assert(positions.has(event.positionId), 'unbound causal position');
  }
  if (result.position) {
    assert.equal(result.position.id, identity({ Decision: result.position.decisionId }));
    assert.equal(positions.get(result.position.id), result.position.decisionId);
  }
  if (result.pending) assert.equal(decisions.get(result.pending.decisionId), result.pending.signalSequence);
  const cumulative = events.map((event) => ({ ...ordered(event, eventKeys), clock: ordered(event.clock, clockKeys) }));
  assert.match(result.stateDigest, hex);
  assert.deepEqual(result.checkpoint, { schema: 'heisentick-paper-bidask-replay-v1', bindingDigest: result.bindingDigest,
    inputDigest: result.inputDigest, recordCount: result.processedRecords, eventCount: result.eventRange.to,
    eventDigest: identity(cumulative), stateDigest: result.stateDigest });
}
// Only this enumerated set is normalized, after validating every public hash.
// Private state contains target-derived position/pending identities. Its hash is
// opaque here: all-split exact replay, corrupt-receipt refusal, and the separate
// same-target raw full-API Go tests establish it; no cross-target hash claim.
function comparable(result, prior = []) {
  const v = clone(result);
  v.binding.producer.goos = '<target>'; v.binding.producer.goarch = '<target>';
  v.bindingDigest = '<validated-binding>'; v.checkpoint.bindingDigest = '<validated-binding>';
  v.stateDigest = '<opaque-target-state>'; v.checkpoint.stateDigest = '<opaque-target-state>';
  v.checkpoint.eventDigest = '<validated-cumulative-events>';
  const decisions = new Map(), positions = new Map();
  for (const event of [...prior, ...result.events]) {
    if (event.kind === 'decision') decisions.set(event.decisionId, `decision:${event.recordSequence}`);
    if (event.kind === 'entry') positions.set(event.positionId, `position:${decisions.get(event.decisionId)}`);
  }
  for (const [index, event] of v.events.entries()) {
    event.id = `event:${event.recordSequence}:${prior.length + index}:${event.kind}`;
    if (event.decisionId) { assert(decisions.has(event.decisionId)); event.decisionId = decisions.get(event.decisionId); }
    if (event.positionId) { assert(positions.has(event.positionId)); event.positionId = positions.get(event.positionId); }
  }
  if (v.position) { v.position.id = positions.get(v.position.id); v.position.decisionId = decisions.get(v.position.decisionId); }
  if (v.pending) v.pending.decisionId = decisions.get(v.pending.decisionId);
  return v;
}
let comparisons = 0, refusals = 0, resumeSplits = 0;
const captures = [];
function compare(r, label, checkpoints, previous) {
  const raw = [0, 1].map((i) => JSON.stringify({ ...r, ...(checkpoints ? { checkpoint: checkpoints[i] } : {}) }));
  const pair = [native(raw[0]), wasm(raw[1])];
  const inspectors = inspect(r.source);
  assert.equal(pair[0].value.binding?.producer.goos, process.platform);
  assert.equal(pair[0].value.binding.producer.goarch, process.arch === 'x64' ? 'amd64' : process.arch);
  assert.equal(pair[1].value.binding?.producer.goos, 'js'); assert.equal(pair[1].value.binding.producer.goarch, 'wasm');
  for (let i = 0; i < 2; i++) validate(pair[i].value, r, inspectors[i], previous?.[i] ?? []);
  assert.deepEqual(comparable(pair[0].value, previous?.[0] ?? []), comparable(pair[1].value, previous?.[1] ?? []), `${label}: exact native/public mismatch`);
  assert.equal(wasm(raw[1]).raw, pair[1].raw, `${label}: repeated public bytes changed`);
  comparisons++;
  captures.push({ label, requestSha256: hash(JSON.stringify(r)), nativeSha256: hash(pair[0].raw), wasmSha256: hash(pair[1].raw) });
  if (evidence) for (let i = 0; i < 2; i++) {
    writeFileSync(join(evidence, `${label}.${i ? 'wasm' : 'native'}.request.json`), raw[i]);
    writeFileSync(join(evidence, `${label}.${i ? 'wasm' : 'native'}.json`), pair[i].raw);
  }
  return pair.map((v) => v.value);
}
function replay(r, label, expected = () => {}) {
  const full = compare(r, `${label}-full`);
  full.forEach(expected);
  for (let split = 0; split <= r.records.length; split++) {
    const first = compare(prefix(r, split), `${label}-prefix-${split}`);
    const cp = first.map((v) => v.checkpoint), events = first.map((v) => v.events);
    const same = compare(prefix(r, split), `${label}-same-${split}`, cp, events);
    for (let i = 0; i < 2; i++) {
      assert.equal(same[i].events.length, 0);
      assert.deepEqual({ ...same[i], events: first[i].events, eventRange: first[i].eventRange }, first[i]);
      assert.equal(goJSON(same[i].checkpoint), goJSON(cp[i]));
    }
    const resumed = compare(r, `${label}-resume-${split}`, cp, events);
    for (let i = 0; i < 2; i++) {
      assert.deepEqual([...first[i].events, ...resumed[i].events], full[i].events);
      assert.deepEqual({ ...resumed[i], events: full[i].events, eventRange: full[i].eventRange }, full[i]);
      assert.equal(goJSON(resumed[i].checkpoint), goJSON(full[i].checkpoint));
    }
    resumeSplits++;
  }
  const retried = compare(r, `${label}-final-retry`, full.map((v) => v.checkpoint), full.map((v) => v.events));
  for (let i = 0; i < 2; i++) assert.deepEqual({ ...retried[i], events: full[i].events, eventRange: full[i].eventRange }, full[i]);
  return full;
}

const healthyRaw = JSON.stringify(entered(false, '46'));
const healthy = wasm(healthyRaw).raw;
assert.equal(JSON.parse(healthy).position?.quantity, '46');
const oldSource = readFileSync(join(root, 'conformance/run/family-sequential-full-e1-long.strat'), 'utf8');
const oldFixture = readFileSync(join(root, 'conformance/run/family-sequential-full-e1-long.fixture.json'), 'utf8');
const oldResult = globalThis.engineRunFixture(oldFixture, oldSource);
assert(JSON.parse(oldResult).tradeCount > 0);
const oldPrefix = JSON.stringify({ schema: 'heisentick-paper-prefix-request-v1', source,
  route: { strategyId: 'invented-old-export', symbol: 'XAUUSD', timeframe: '5m', rangeMethod: 'zone' },
  units: { quoteCurrency: 'USD', priceUnit: 'USD/unit', quantityUnit: 'unit', multiplier: 1 },
  costs: { startEquity: 1000, feePerUnit: 0, fillOn: 'close', slippage: 0, slippageBps: 0 },
  activationFromT: 1735689600000, closedThroughT: 1735690800000,
  bars: [[1735689600000, 100, 100.5, 99.5, 100, 1], [1735689900000, 100, 100.5, 99.5, 100, 1],
    [1735690200000, 100, 100.5, 99.5, 100, 1], [1735690500000, 100, 101.5, 99.75, 101, 1]] });
const oldPrefixResult = globalThis.engineRunPaperPrefix(oldPrefix);
assert.equal(JSON.parse(oldPrefixResult).schema, 'heisentick-paper-prefix-v1');
function liveness() {
  assert.equal(wasm(healthyRaw).raw, healthy, 'rejection damaged bid/ask export');
  assert.equal(globalThis.engineRunFixture(oldFixture, oldSource), oldResult, 'rejection damaged fixture export');
  assert.equal(globalThis.engineRunPaperPrefix(oldPrefix), oldPrefixResult, 'rejection damaged paper-prefix export');
  for (const name of oldExports) assert.equal(typeof globalThis[name], 'function', name);
}
function envelope(value, code, label) { assert.deepEqual(value, { schema, status: 'refused', code: `PAPER_BIDASK_${code}` }, label); }
function refuse(values, code, nativeRaw, label = code) {
  envelope(wasm(...values).value, code, label);
  if (nativeRaw !== undefined) envelope(native(nativeRaw).value, code, label);
  liveness(); refusals++;
  captures.push({ label: `refusal-${label}`, code: `PAPER_BIDASK_${code}`, native: nativeRaw !== undefined });
}
function bad(r, code, label = code) { const raw = JSON.stringify(r); refuse([raw], code, raw, label); }
function mutateBad(base, mutate, code, label) { const r = clone(base); mutate(r); bad(r, code, label); }
function noFill(outcome) {
  return (v) => { assert.equal(v.position, undefined); assert.equal(v.pending, undefined); assert.equal(v.account.balance, '1000');
    assert.equal(v.completedTrades.completed, 0); assert(v.events.some((event) => event.outcome === outcome), outcome); };
}

try {
  for (const short of [false, true]) {
    const r = entered(short, '46');
    replay(r, `risk46-${short ? 'short' : 'long'}-entry`, (v) => {
      assert.equal(v.position.side, short ? 'short' : 'long'); assert.equal(v.position.entry, short ? '98.8' : '101.2');
      assert.equal(v.position.quantity, '46'); assert.equal(v.position.riskBudget, '46');
      assert.equal(v.account.equity, '990.8'); assert.equal(v.account.balance, '1000');
    });
    addQuote(r, entryAt + 1000000000n, '100', '100.2');
    replay(r, `risk46-${short ? 'short' : 'long'}-sampled-stop`, (v) => {
      assert.equal(v.position, undefined); assert.equal(v.account.balance, short ? '935.6' : '944.8');
      assert.equal(v.completedTrades.netPnl, short ? '-64.4' : '-55.2'); assert.equal(v.completedTrades.completed, 1);
      assert.equal(v.events.find((event) => event.kind === 'exit').price, short ? '100.2' : '100');
    });
    const costs = entered(short, '46');
    Object.assign(costs.economics, { entryFee: '0.1', exitFee: '0.1', slippageFixed: '0.1' });
    replay(costs, `fixed-costs-${short ? 'short' : 'long'}-entry`, (v) => {
      assert.equal(v.position.entry, short ? '98.7' : '101.3'); assert.equal(v.position.quantity, '35.25');
      assert.equal(v.account.balance, '996.475'); assert.equal(v.account.equity, '985.9'); assert.equal(v.account.paidFees, '3.525');
    });
    addQuote(costs, entryAt + 1000000000n, '100', '100.2');
    replay(costs, `fixed-costs-${short ? 'short' : 'long'}-exit`, (v) => {
      assert.equal(v.account.balance, short ? '936.55' : '943.6'); assert.equal(v.account.paidFees, '7.05');
      assert.equal(v.completedTrades.netPnl, short ? '-63.45' : '-56.4');
    });
  }
  const bps = entered(false, '44.012048'); bps.economics.slippageBps = '10';
  replay(bps, 'nonzero-bps-entry', (v) => { assert.equal(v.position.entry, '101.3012'); assert.equal(v.position.quantity, '40'); assert.equal(v.account.equity, '987.952'); });
  addQuote(bps, entryAt + 1000000000n, '100', '100.2');
  replay(bps, 'nonzero-bps-exit', (v) => { assert.equal(v.account.balance, '943.952'); assert.equal(v.completedTrades.netPnl, '-56.048'); });
  replay(phase(), 'native-1000-to-800-phase', (v) => {
    assert.equal(v.account.balance, '800'); assert.equal(v.completedTrades.netPnl, '-200');
    const exit = v.events.find((event) => event.kind === 'exit');
    assert.equal(exit.price, '100.2'); assert.equal(exit.basis, 'modeled-side-bar-stop-first');
    assert.equal(exit.clock.providerT, ''); assert.equal(exit.clock.intervalOpenT, stamp(25n * minute));
  });
  replay(prefix(phase(), 8), 'pre-entry-extrema-excluded', (v) => {
    assert(v.position); assert.equal(v.account.equity, '1020'); assert.equal(v.completedTrades.completed, 0);
  });
  replay(prefix(request(), 0), 'empty-journal', (v) => { assert.equal(v.events.length, 0); assert.equal(v.account.equity, '1000'); });
  replay(prefix(request(), 3), 'no-opportunity', (v) => assert.equal(v.events.length, 0));
  replay(request(), 'pending-original-episode', (v) => assert.equal(v.pending.state, 'awaiting-original-episode'));
  replay(prefix(entered(), 5), 'pending-original-commit', (v) => assert.equal(v.pending.state, 'awaiting-original-commit-or-seal'));
  const warm = request(); warm.activationFromT = stamp(20n * minute);
  replay(warm, 'warmup', (v) => { assert.equal(v.events.length, 0); assert.equal(v.pending, undefined); });
  addBar(warm, 20n * minute); addBar(warm, 25n * minute, '100', '101.5', '99.75', '101');
  replay(warm, 'activation-fresh-opportunity', (v) => assert.equal(v.pending.signalSequence, '6'));

  for (const [name, edit, outcome] of [
    ['liquidity', (r) => { r.records[5].quote.asks[0].liquidity = '45.99'; }, 'insufficient-entry-liquidity'],
    ['minimum', (r) => { r.economics.quantityMinimum = '46.25'; }, 'below-minimum-quantity'],
    ['maximum', (r) => { r.economics.quantityMaximum = '45.75'; }, 'above-maximum-quantity'],
    ['notional', (r) => { r.economics.notionalPolicy = 'cap'; r.economics.notionalMaximum = '4655.19'; }, 'notional-cap'],
    ['breached', (r) => { r.records[5].quote.bids[0].price = '99'; }, 'invalid-or-breached-protection'],
    ['stale', (r) => { r.records[5].quote.receivedT = stamp(entryAt + 6000000000n); r.records[5].quote.qualifiedT = r.records[5].quote.receivedT; r.records[5].commitT = r.records[5].quote.receivedT; }, 'original-quote-expired'],
    ['late-commit', (r) => { r.records[5].commitT = stamp(20n * minute + 200000000001n); }, 'original-quote-expired'],
    ['pre-episode', (r) => { r.records[5].quote.requestStartT = stamp(20n * minute); }, 'original-quote-expired'],
    ['untradeable', (r) => { r.records[5].quote.tradeable = false; }, 'not-tradeable'],
  ]) { const r = entered(false, '46'); edit(r); replay(r, `no-fill-${name}`, noFill(outcome)); }
  for (const duplicate of [true, false]) {
    const r = request();
    addQuote(r, 20n * minute + 10000000n, '101', '101.2');
    const episode = { signalSequence: '4', admittedT: stamp(20n * minute + 100000000n), deadlineT: stamp(20n * minute + 200000000000n) };
    append(r, 'episode', episode, episode.admittedT);
    const original = addQuote(r, entryAt, '101', '101.2', '4');
    original.providerT = stamp(20n * minute + (duplicate ? 10000000n : 5000000n));
    replay(r, duplicate ? 'duplicate-original-observation' : 'out-of-order-original-observation',
      noFill(duplicate ? 'duplicate-original-observation' : 'out-of-order-original-observation'));
  }
  const unhealthyEntry = entered();
  unhealthyEntry.records.splice(5, 0, { sequence: '6', epoch: unhealthyEntry.identity.epoch, commitT: stamp(20n * minute + 500000000n), kind: 'coverage-gap',
    gap: { fromT: stamp(20n * minute + 400000000n), throughT: stamp(20n * minute + 500000000n), reason: 'disconnect' } });
  unhealthyEntry.records[6].sequence = '7';
  replay(unhealthyEntry, 'coverage-unhealthy-entry', noFill('coverage-unhealthy'));
  const shortGap = entered(true);
  addBar(shortGap, 20n * minute, '99', '101', '90', '98.8');
  addBar(shortGap, 25n * minute, '101', '102', '96', '99');
  replay(shortGap, 'short-full-opening-gap', (v) => { assert.equal(v.account.balance, '560'); assert.equal(v.completedTrades.netPnl, '-440'); });
  const quantum = entered(false, '45.999999999999');
  replay(quantum, 'quantum-below-boundary', (v) => assert.equal(v.position.quantity, '45.75'));
  const maximum = entered(false, '46'); maximum.economics.quantityMaximum = '46'; maximum.economics.quantityMinimum = '46';
  maximum.records[5].quote.asks[0].liquidity = '46'; maximum.economics.notionalPolicy = 'cap'; maximum.economics.notionalMaximum = '4655.2';
  replay(maximum, 'inclusive-quantity-liquidity-notional', (v) => assert.equal(v.position.quantity, '46'));

  for (const reason of ['recovery', 'deadline']) {
    const r = prefix(entered(), 5);
    const seal = { signalSequence: '4', reason, effectiveT: stamp(reason === 'recovery' ? entryAt + 1000000000n : 20n * minute + 200000000000n) };
    append(r, 'seal', seal, seal.effectiveT);
    replay(r, `${reason}-durable-seal`, noFill(`expired-${reason}`));
    const original = clone(r); addQuote(original, 20n * minute + 201000000000n, '101', '101.2', '4'); bad(original, 'EPISODE_STATE', `${reason}-resolved-original-retry`);
    addQuote(r, 20n * minute + 201000000000n, '101', '101.2');
    addBar(r, 20n * minute, '101', '102.5', '100.75', '102');
    replay(r, `${reason}-unbound-cannot-rescue`, noFill(`expired-${reason}`));
  }

  const boundary = entered();
  boundary.records[4].episode.admittedT = stamp(20n * minute + 1n); boundary.records[4].commitT = stamp(20n * minute + 2n);
  boundary.records[5].quote = quote(boundary, 20n * minute, '101', '101.2', '4');
  boundary.records[5].quote.requestStartT = stamp(20n * minute + 2n); boundary.records[5].commitT = stamp(20n * minute + 3000000n);
  replay(boundary, 'exact-boundary-entry', (v) => { assert.equal(v.position.phase, 'full-bar'); assert.equal(v.position.fullPhaseFromT, stamp(20n * minute)); });
  addQuote(boundary, entryAt, '99', '99.2');
  replay(boundary, 'full-phase-quote-no-authority', (v) => { assert(v.position); assert.equal(v.account.markCount, 1); assert.equal(v.account.markStatus, 'awaiting-completed-interval'); });
  const delayed = bar(20n * minute, '101', '104', '99', '102'); delayed.receivedT = stamp(35n * minute);
  append(boundary, 'bar', delayed, delayed.receivedT);
  replay(boundary, 'delayed-full-bar', (v) => assert.equal(v.account.balance, '800'));
  const nano = entered();
  nano.records[5].quote.providerT = stamp(20n * minute + 1n);
  replay(nano, 'one-nanosecond-after-boundary', (v) => { assert.equal(v.position.phase, 'sampled-straddle'); assert.equal(v.position.fullPhaseFromT, stamp(25n * minute)); });
  const deadlineTie = entered(); deadlineTie.records[5].commitT = deadlineTie.records[4].episode.deadlineT;
  replay(deadlineTie, 'exact-deadline-commit', (v) => assert(v.position));
  const ageTie = entered(); ageTie.records[5].quote.receivedT = stamp(entryAt + 5000000000n);
  ageTie.records[5].quote.qualifiedT = ageTie.records[5].quote.receivedT; ageTie.records[5].commitT = ageTie.records[5].quote.receivedT;
  replay(ageTie, 'exact-quote-age', (v) => assert(v.position));

  for (const type of ['missing-bar', 'gap', 'unfillable-exit', 'untradeable-exit', 'stale-exit']) {
    const r = entered();
    if (type === 'gap') {
      const gap = { fromT: stamp(entryAt + 1000000000n), throughT: stamp(entryAt + 2000000000n), reason: 'disconnect' };
      append(r, 'coverage-gap', gap, gap.throughT);
    }
    if (type.endsWith('exit')) {
      const q = addQuote(r, entryAt + 1000000000n, '100', '100.2');
      if (type === 'unfillable-exit') q.bids[0].liquidity = '199.99';
      if (type === 'untradeable-exit') q.tradeable = false;
      if (type === 'stale-exit') { q.receivedT = stamp(entryAt + 7000000000n); q.qualifiedT = q.receivedT; r.records.at(-1).commitT = q.receivedT; }
    } else {
      addBar(r, 25n * minute, '101', '110', '90', '101'); addBar(r, 30n * minute, '101', '110', '90', '101');
    }
    replay(r, `unresolved-${type}`, (v) => { assert(v.position); assert.equal(v.completedTrades.completed, 0); assert.equal(v.account.markStatus, 'stale-unresolved'); assert(v.coverage.startsWith('unresolved-exposure:')); });
  }
  const protection = entered(); protection.source = protection.source.replace('after 999R plus 0 ATR', 'after 0.5R plus 0 ATR');
  addBar(protection, 20n * minute, '101.2', '101.5', '100.3', '101.4');
  addBar(protection, 25n * minute, '101.4', '102', '100.3', '101.8');
  replay(protection, 'future-only-protection', (v) => { assert.equal(v.position.stop, '101.2'); assert.equal(v.position.protectionVersion, 2); assert.equal(v.completedTrades.completed, 0); const e = v.events.find((e) => e.kind === 'protection'); assert.equal(e.priorProtectionVersion, 1); assert.equal(e.clock.modelEffectiveT, stamp(30n * minute)); });
  addBar(protection, 30n * minute, '101.3', '101.4', '101.1', '101.3');
  replay(protection, 'future-protection-exit', (v) => { assert.equal(v.account.balance, '1000'); assert.equal(v.completedTrades.flat, 1); });
  const timed = entered(); timed.source += '\nmanagement { maxHoldCandles 1 }\n';
  const sideClose = addBar(timed, 20n * minute, '101.2', '101.6', '100.8', '101.5'); sideClose.bid.close = '101.4'; sideClose.ask.close = '101.6';
  replay(timed, 'timed-side-close', (v) => { assert.equal(v.account.balance, '1040'); assert.equal(v.completedTrades.netPnl, '40'); });
  const slot = entered();
  for (let at = 20n * minute; at < 180n * minute; at += 5n * minute) addBar(slot, at, '101.2', '101.5', '100.8', '101.2');
  addBar(slot, 180n * minute, '100', '105', '90', '104');
  replay(slot, 'slot-open-before-brackets', (v) => { const e = v.events.find((e) => e.kind === 'exit'); assert.equal(e.outcome, 'window-close'); assert.equal(e.price, '100'); assert.equal(v.account.balance, '760'); });

  for (const short of [false, true]) for (const state of ['open', 'closed', 'unresolved']) {
    const r = entered(short);
    const newer = addQuote(r, entryAt + 1000000000n, state === 'open' ? (short ? '98.7' : '101.3') : '100', state === 'open' ? (short ? '98.9' : '101.5') : '100.2');
    if (state === 'unresolved') newer[short ? 'asks' : 'bids'][0].liquidity = '199.99';
    const before = compare(r, `historical-${short}-${state}-before`);
    const old = clone(r.records[5].quote); old.episodeSignalSequence = ''; old.providerId = repeated('b'); old.provenanceDigest = repeated('c');
    old.bids[0].price = short ? '98.800' : '101.0';
    old.requestStartT = stamp(entryAt + 1500000000n); old.receivedT = stamp(entryAt + 1501000000n); old.qualifiedT = stamp(entryAt + 1502000000n);
    append(r, 'quote', old, old.qualifiedT);
    const after = replay(r, `historical-${short}-${state}-duplicate`, (v) => assert.equal(v.events.at(-1).outcome, 'duplicate-provider-observation'));
    for (let i = 0; i < 2; i++) for (const field of ['account', 'position', 'completedTrades', 'coverage', 'stateDigest']) assert.deepEqual(after[i][field], before[i][field]);
    const conflict = clone(r); conflict.records.at(-1).quote.bids[0].liquidity = '9999';
    bad(conflict, 'QUOTE_CONFLICT', `historical-${short}-${state}-conflict`);
    for (let split = 0; split < conflict.records.length; split++) {
      const prior = compare(prefix(r, split), `historical-${short}-${state}-conflict-prefix-${split}`);
      for (let i = 0; i < 2; i++) {
        const raw = JSON.stringify({ ...conflict, checkpoint: prior[i].checkpoint });
        envelope((i ? wasm(raw) : native(raw)).value, 'QUOTE_CONFLICT'); refusals++; liveness();
      }
    }
    const resolved = clone(r); resolved.records.at(-1).quote.episodeSignalSequence = '4';
    bad(resolved, 'EPISODE_STATE', `historical-${short}-${state}-resolved-original`);
  }
  for (const short of [false, true]) {
    const r = entered(short);
    addQuote(r, entryAt + 1000000000n, short ? '98.7' : '101.3', short ? '98.9' : '101.5');
    const late = quote(r, entryAt + 500000000n, short ? '98.7' : '101.3', short ? '98.9' : '101.5');
    late.receivedT = stamp(entryAt + 1501000000n); late.qualifiedT = stamp(entryAt + 1502000000n);
    append(r, 'quote', late, late.qualifiedT);
    replay(r, `unseen-late-${short}`, (v) => { assert(v.position); assert.equal(v.coverage, 'unresolved-exposure:late-straddle-observation'); });
    const full = entered(short);
    for (let i = 0; i < 2; i++) addBar(full, 20n * minute + BigInt(i) * 5n * minute,
      ...(short ? ['98.8', '99.3', '98.5', '98.7'] : ['101.3', '101.5', '100.8', '101.4']));
    const before = compare(full, `resolved-full-${short}-before`);
    const q = quote(full, 30n * minute - 1000000000n, short ? '98.7' : '101.3', short ? '98.9' : '101.5');
    q.receivedT = stamp(30n * minute + 1000000n); q.qualifiedT = stamp(30n * minute + 2000000n);
    append(full, 'quote', q, q.qualifiedT);
    const after = replay(full, `resolved-full-${short}-no-regression`);
    for (let i = 0; i < 2; i++) for (const field of ['account', 'position', 'coverage', 'events']) assert.deepEqual(after[i][field], before[i][field]);
    const tie = quote(full, 30n * minute, short ? '98.7' : '101.3', short ? '98.9' : '101.5');
    tie.receivedT = stamp(30n * minute + 3000000n); tie.qualifiedT = stamp(30n * minute + 4000000n);
    append(full, 'quote', tie, tie.qualifiedT);
    replay(full, `resolved-full-${short}-close-tie`, (v) => assert.equal(v.account.markStatus, 'awaiting-completed-interval'));
  }

  // Every receipt field, every prior record, and each immutable binding category
  // must fail atomically. Each target carries its own receipt, never the other's.
  const receiptRequest = phase();
  const receipts = compare(prefix(receiptRequest, 8), 'mutation-receipt-base');
  const receiptChanges = Object.keys(receipts[0].checkpoint).map((key) => [key, (r) => {
    r.checkpoint[key] = typeof r.checkpoint[key] === 'number' ? r.checkpoint[key] + 1 : 'corrupted';
  }]);
  const commentOnly = clone(receiptRequest); commentOnly.source += '\n# immutable bytes changed\n';
  const originalInspection = inspect(receiptRequest.source), commentInspection = inspect(commentOnly.source);
  for (let target = 0; target < 2; target++) assert.deepEqual(commentInspection[target].configIdentity, originalInspection[target].configIdentity);
  const admittedComment = compare(prefix(commentOnly, 0), 'comment-only-admitted-before-receipt-mutation');
  for (const v of admittedComment) assert.notEqual(v.binding.source.sha256, hash(receiptRequest.source));
  const bindingChanges = [
    ['source-comment', (r) => { r.source = commentOnly.source; }],
    ['source-risk', (r) => { r.source = r.source.replace('risk: 200 USD', 'risk: 201 USD'); }],
    ['source-config', (r) => { r.source = r.source.replace('after 999R', 'after 998R'); }],
    ['route-strategy', (r) => { r.route.strategyId = 'changed'; }],
    ['route-range', (r) => { r.route.rangeMethod = 'pivot'; }],
    ['activation', (r) => { r.activationFromT = stamp(5n * minute); }],
    ['product-attestation', (r) => { r.product.attestationDigest = repeated('f'); }],
    ['policy-quote-age', (r) => { r.policy.maxQuoteAgeMs = '4000'; }],
    ['policy-signal-age', (r) => { r.policy.maxSignalAgeMs = '280000'; }],
  ];
  for (const key of Object.keys(receiptRequest.identity)) bindingChanges.push([`identity-${key}`, (r) => {
    r.identity[key] = repeated('f');
    if (key === 'epoch') for (const record of r.records) record.epoch = r.identity.epoch;
    if (key === 'accountDigest') for (const record of r.records) if (record.quote) record.quote.accountDigest = r.identity.accountDigest;
  }]);
  for (const [key, value] of Object.entries({ startEquity: '1001', entryFee: '0.1', exitFee: '0.1', slippageFixed: '0.1', slippageBps: '1', quantityQuantum: '0.125', quantityMinimum: '0.5', quantityMaximum: '9999', notionalPolicy: 'cap', notionalMaximum: '999999' })) {
    bindingChanges.push([`economics-${key}`, (r) => { r.economics[key] = value; if (key === 'notionalPolicy' || key === 'notionalMaximum') { r.economics.notionalPolicy = 'cap'; r.economics.notionalMaximum = '999999'; } }]);
  }
  const recordChanges = Array.from({ length: 8 }, (_, index) => [`record-${index}`, (r) => {
    const record = r.records[index];
    if (record.bar) record.bar.provenanceDigest = repeated('f');
    else if (record.quote) record.quote.provenanceDigest = repeated('f');
    else record.episode.deadlineT = stamp(20n * minute + 199000000000n);
  }]);
  for (const [label, change] of [...receiptChanges, ...bindingChanges, ...recordChanges]) for (let target = 0; target < 2; target++) {
    const r = clone(receiptRequest); r.checkpoint = clone(receipts[target].checkpoint); change(r);
    const raw = JSON.stringify(r); envelope((target ? wasm(raw) : native(raw)).value, 'CHECKPOINT', label);
    refusals++; liveness(); captures.push({ label: `atomic-${target}-${label}`, code: 'PAPER_BIDASK_CHECKPOINT' });
  }
  const structurallyFixedBindings = [
    ...['provider', 'instrument', 'accountCurrency', 'priceUnit', 'quantityUnit', 'multiplier'].map((key) =>
      [`product-${key}`, (r) => { r.product[key] = 'changed'; }, 'PRODUCT_UNSUPPORTED']),
    ...['model', 'coverage', 'arithmetic'].map((key) =>
      [`policy-${key}`, (r) => { r.policy[key] = 'changed'; }, 'POLICY']),
    ['route-symbol', (r) => { r.route.symbol = 'changed'; }, 'PRODUCT_UNSUPPORTED'],
    ['contract-schema', (r) => { r.schema = 'changed'; }, 'INPUT'],
  ];
  // Timeframe mismatch is already checked independently through public source
  // admission below; with this receipt it must still refuse before any effects.
  for (const [label, change, code] of structurallyFixedBindings) for (let target = 0; target < 2; target++) {
    const r = clone(receiptRequest); r.checkpoint = clone(receipts[target].checkpoint); change(r);
    envelope((target ? wasm(JSON.stringify(r)) : native(JSON.stringify(r))).value, code, label);
    refusals++; liveness();
  }
  for (let target = 0; target < 2; target++) {
    const raw = JSON.stringify({ ...receiptRequest, checkpoint: receipts[1 - target].checkpoint });
    envelope((target ? wasm(raw) : native(raw)).value, 'CHECKPOINT'); refusals++; liveness();
  }
  // Receipt byte ceiling includes preserved raw whitespace, not re-encoded JSON.
  for (let target = 0; target < 2; target++) {
    const cp = JSON.stringify(receipts[target].checkpoint), without = JSON.stringify(receiptRequest);
    const exact = without.slice(0, -1) + ',"checkpoint":' + cp.slice(0, -1) + ' '.repeat(4096 - Buffer.byteLength(cp)) + '}}';
    const out = (target ? wasm(exact) : native(exact)).value;
    assert.equal(out.mode, 'replay-verified-candidate');
    const over = exact.slice(0, -2) + ' ' + exact.slice(-2);
    envelope((target ? wasm(over) : native(over)).value, 'CHECKPOINT'); refusals++; liveness();
  }

  const admissionBase = prefix(request(), 0);
  for (const [extra, code] of [
    ['management { partial half at 1R }', 'PARTIAL_UNQUALIFIED'],
    ['management { partial half at 1R }\nsetup { type: opening range breakout }', 'PARTIAL_UNQUALIFIED'],
    ['management { trail 1 ATR after 1R }', 'TRAILING_UNQUALIFIED'],
    ['management {\n trail 1 ATR after 1R\n trail off\n}', 'TRAILING_UNQUALIFIED'],
    ['management { trail 1 ATR after 1R }\nsetup { type: opening range breakout }', 'TRAILING_UNQUALIFIED'],
    ['filters { higher timeframe must agree }', 'ADMISSION'],
    ['filters { source timeframe 1h }', 'ADMISSION'],
    ['filters {\n higher timeframe must agree\n higher timeframe off\n}', 'ADMISSION'],
    ['filters {\n source timeframe 1h\n entryTf current\n}', 'ADMISSION'],
    ['setup { retest off }', 'ADMISSION'],
    ['market conditions { new york hour in (1) }', 'ADMISSION'],
    ['execution {\n enter limit at swept edge\n enter market\n}', 'ADMISSION'],
    ['management { breakeven off } partial 0.5 at 1R', 'ADMISSION'],
    ['filters { approach 0 ATR side both }', 'ADMISSION'],
    ['filters { side both } unexpected discarded text', 'ADMISSION'],
    ['management { breakeven after 1R trail off }', 'ADMISSION'],
    ['setup {\n type: vwap extension fade\n type: opening range breakout\n}', 'ADMISSION'],
  ]) mutateBad(admissionBase, (r) => { r.source += `\n${extra}\n`; }, code, extra);
  mutateBad(admissionBase, (r) => { r.source = oldSource; }, 'FAMILY_UNQUALIFIED', 'unsupported-family');
  mutateBad(admissionBase, (r) => { r.source = 'PRIVATE_SOURCE malformed source'; }, 'SOURCE', 'bad-source');
  mutateBad(admissionBase, (r) => { r.route.timeframe = '15m'; }, 'ROUTE_UNSUPPORTED', 'source-route');
  for (const extra of ['# partial half at 1R is only a comment\nmanagement { trail off }',
    'strategy "Independent source identity" { description "partial trailing metadata" }',
    'filters {\n higher timeframe off\n entryTf current\n}']) {
    const r = clone(admissionBase); r.source += `\n${extra}\n`; compare(r, `admission-positive-${hash(extra).slice(0, 8)}`);
  }

  for (const [label, change, code] of [
    ['old-epoch', (r) => { r.records[5].epoch = repeated('f'); }, 'SEQUENCE'],
    ['sequence-hole', (r) => { r.records[5].sequence = '7'; }, 'SEQUENCE'],
    ['commit-regression', (r) => { r.records[5].commitT = r.records[0].commitT; }, 'COMMIT_ORDER'],
    ['incomplete-bar', (r) => { r.records[0].bar.complete = false; }, 'BAR'],
    ['crossed-quote', (r) => { r.records[5].quote.bids[0].price = '102'; }, 'QUOTE_SPREAD'],
    ['account-mismatch', (r) => { r.records[5].quote.accountDigest = repeated('f'); }, 'QUOTE'],
    ['no-fractional-volume', (r) => { r.records[0].bar.priceCount = '1.1'; }, 'VOLUME'],
    ['future-provider', (r) => { r.records[5].quote.providerT = stamp(entryAt + 1000001n); }, 'QUOTE_CLOCK'],
    ['episode-before-commit', (r) => { r.records[4].episode.admittedT = r.records[3].bar.closeT; }, 'EPISODE_DEADLINE'],
    ['deadline-too-large', (r) => { r.records[4].episode.deadlineT = stamp(25n * minute + 1n); }, 'EPISODE_DEADLINE'],
    ['product', (r) => { r.product.multiplier = '2'; }, 'PRODUCT_UNSUPPORTED'],
    ['policy', (r) => { r.policy.model = 'other'; }, 'POLICY'],
    ['quantity-quantum', (r) => { r.economics.quantityQuantum = '0'; }, 'ECONOMICS'],
    ['identity', (r) => { r.identity.runId = 'PRIVATE_IDENTITY'; }, 'IDENTITY'],
    ['activation', (r) => { r.activationFromT = stamp(1n); }, 'ACTIVATION'],
  ]) mutateBad(entered(), change, code, label);
  const unresolved = request(); addBar(unresolved, 20n * minute); bad(unresolved, 'UNRESOLVED_EPISODE');

  // No coercion, property probing, getters, or caller-controlled conversions.
  let hooks = 0;
  const hostile = { get toString() { hooks++; throw Error('PRIVATE_GETTER'); },
    get valueOf() { hooks++; throw Error('PRIVATE_VALUEOF'); }, get [Symbol.toPrimitive]() { hooks++; throw Error('PRIVATE_CONVERSION'); } };
  const proxy = new Proxy({}, { get() { hooks++; throw Error('PRIVATE_PROXY'); }, getPrototypeOf() { hooks++; throw Error('PRIVATE_PROTOTYPE'); } });
  const revoked = Proxy.revocable({}, {}); revoked.revoke();
  const callable = new Proxy(() => {}, { get() { hooks++; throw Error('PRIVATE_CALLABLE'); }, apply() { hooks++; throw Error('PRIVATE_CALL'); } });
  for (const values of [[], [undefined], [null], [true], [false], [1], [NaN], [Infinity], [1n], [Symbol('PRIVATE_SYMBOL')],
    [[]], [new Uint8Array([1])], [new String(healthyRaw)], [{}], [() => {}], [hostile], [proxy], [revoked.proxy], [callable], [healthyRaw, healthyRaw]]) {
    refuse(values, 'ARGUMENTS'); assert.equal(hooks, 0);
  }
  for (const badUnicode of ['\ud800', '\udfff', '\ud800x', '\udc00\ud800', '\ud800\udc00\ud800']) {
    refuse([healthyRaw.replace('"source":"', `"source":"${badUnicode}`)], 'INPUT', undefined, 'literal-unpaired-surrogate');
  }
  const intrinsic = String.prototype.charCodeAt;
  try {
    String.prototype.charCodeAt = function () { hooks++; return 65; };
    const raw = call(healthyRaw.replace('"source":"', '"source":"\ud800'));
    envelope(JSON.parse(raw), 'INPUT');
  } finally { String.prototype.charCodeAt = intrinsic; }
  assert.equal(hooks, 0); refusals++; liveness();
  const objectIntrinsic = globalThis.Object;
  try {
    globalThis.Object = () => { hooks++; throw Error('PRIVATE_OBJECT_OVERRIDE'); };
    envelope(JSON.parse(call(healthyRaw.replace('"source":"', '"source":"\ud800'))), 'INPUT');
    assert.equal(call(healthyRaw), healthy);
  } finally { globalThis.Object = objectIntrinsic; }
  assert.equal(hooks, 0); refusals++; liveness();
  // Go 1.22 Value.String crosses mutable wasm_exec.js String/TextEncoder
  // conversion. Input validation must not be followed by request substitution.
  const differentRisk = JSON.stringify(entered(false, '45'));
  const differentExpected = call(differentRisk);
  assert.equal(JSON.parse(differentExpected).position.quantity, '45');
  assert.equal(JSON.parse(differentExpected).account.equity, '991');
  assert.notEqual(differentExpected, healthy);
  const stringIntrinsic = globalThis.String;
  for (const throwing of [false, true]) {
    let conversionHooks = 0;
    try {
      globalThis.String = () => {
        conversionHooks++;
        if (throwing) throw Error('PRIVATE_STRING_CONVERSION');
        return healthyRaw;
      };
      envelope(JSON.parse(call('{}')), 'INPUT');
      assert.equal(call(healthyRaw), healthy);
      assert.equal(call(differentRisk), differentExpected);
      envelope(JSON.parse(call(healthyRaw.replace('"source":"', '"source":"\ud800'))), 'INPUT');
    } finally { globalThis.String = stringIntrinsic; }
    assert.equal(conversionHooks, 0, 'mutable String conversion invoked');
    refusals += 2; liveness();
  }
  const encodeIntrinsic = TextEncoder.prototype.encode;
  const substituteBytes = new TextEncoder().encode(healthyRaw);
  for (const throwing of [false, true]) {
    let conversionHooks = 0;
    try {
      TextEncoder.prototype.encode = () => {
        conversionHooks++;
        if (throwing) throw Error('PRIVATE_ENCODER_CONVERSION');
        return substituteBytes;
      };
      envelope(JSON.parse(call('{}')), 'INPUT');
      assert.equal(call(healthyRaw), healthy);
      assert.equal(call(differentRisk), differentExpected);
      envelope(JSON.parse(call(healthyRaw.replace('"source":"', '"source":"\ud800'))), 'INPUT');
    } finally { TextEncoder.prototype.encode = encodeIntrinsic; }
    assert.equal(conversionHooks, 0, 'mutable UTF-8 encoder invoked');
    refusals += 2; liveness();
  }
  const unicode = entered(false, '46'); unicode.source += '\r\n# Ω 🌱 � <>& exact source bytes\r\n'; unicode.route.strategyId += '<>&\u2028\u2029';
  compare(unicode, 'valid-unicode-source');
  const escaped = JSON.stringify(unicode).replace('🌱', '\\ud83c\\udf31').replace('�', '\\ufffd');
  assert.equal(wasm(escaped).raw, wasm(JSON.stringify(unicode)).raw);
  assert.equal(native(escaped).raw, native(JSON.stringify(unicode)).raw);

  for (const raw of ['', '{}', 'null', '[]', 'true', '"root"', '1', healthyRaw + ' {}', healthyRaw + ' garbage',
    healthyRaw.replace('"schema":', '"unknown":0,"schema":'),
    healthyRaw.replace('"schema":', '"schema":"duplicate","schema":'),
    healthyRaw.replace('"schema":', '"sch\\u0065ma":"duplicate","schema":'),
    healthyRaw.replace('"source":', '"Source":'),
    healthyRaw.replace('"source":"', '"source":"\\ud800'),
    healthyRaw.replace('"source":"', '"source":"\\udfff'),
    healthyRaw.replace('"source":"', '"source":"\\udc00\\ud800'),
    healthyRaw.replace('"startEquity":"1000"', '"startEquity":1000'),
    healthyRaw.replace('"startEquity":"1000"', '"startEquity":NaN'),
    healthyRaw.replace('"startEquity":"1000"', '"startEquity":Infinity'),
    healthyRaw.replace('"complete":true', '"complete":"true"'),
    healthyRaw.replace('"source":"', '"source":"\n'),
  ]) refuse([raw], 'INPUT', raw, 'malformed-json');
  // Each required member is absent/null/case-aliased, including nested payloads.
  function leaves(value, path = []) {
    const result = [];
    for (const [key, child] of Object.entries(value)) {
      result.push([...path, key]);
      if (child && typeof child === 'object' && !Array.isArray(child)) result.push(...leaves(child, [...path, key]));
    }
    return result;
  }
  const shapeBase = entered();
  const paths = [...leaves({ ...shapeBase, records: undefined }).filter((p) => p[0] !== 'records'),
    ...leaves(shapeBase.records[0], ['records', 0]), ...leaves(shapeBase.records[4], ['records', 4]),
    ...leaves(shapeBase.records[5], ['records', 5]), ['records']];
  for (const path of paths) for (const mode of ['missing', 'null', 'alias']) {
    const r = clone(shapeBase); let container = r;
    for (const key of path.slice(0, -1)) container = container[key];
    const key = path.at(-1);
    // Record payload pointers are optional: omitting the sole payload is a
    // semantic RECORD refusal rather than a transport INPUT refusal.
    const optional = path.length === 3 && ['bar', 'episode', 'quote'].includes(key);
    if (mode === 'missing') delete container[key];
    else if (mode === 'null') container[key] = null;
    else { container[key[0].toUpperCase() + key.slice(1)] = container[key]; delete container[key]; }
    bad(r, mode === 'missing' && optional ? 'RECORD' : 'INPUT', `shape-${path.join('.')}-${mode}`);
  }
  mutateBad(shapeBase, (r) => { r.checkpoint = null; }, 'INPUT', 'optional-checkpoint-null');
  mutateBad(shapeBase, (r) => { r.records[0].seal = null; }, 'INPUT', 'optional-payload-null');
  for (const [depth, code] of [[64, 'CHECKPOINT'], [65, 'INPUT']]) {
    // Root object is depth zero; checkpoint value is depth one.
    const raw = healthyRaw.slice(0, -1) + ',"checkpoint":' + '['.repeat(depth - 1) + '0' + ']'.repeat(depth - 1) + '}';
    refuse([raw], code, raw, `json-depth-${depth}`);
  }
  for (const value of ['-1', '+1', '01', '.1', '1.', '1e3', 'NaN', 'Infinity', '-Infinity', '0.0000000000001', '1000000000000']) {
    mutateBad(admissionBase, (r) => { r.economics.entryFee = value; }, 'DECIMAL', `decimal-${value}`);
  }
  for (const value of ['999999999999', '0.123456789012']) {
    const r = clone(admissionBase); r.economics.entryFee = value; compare(r, `decimal-inclusive-${value}`);
  }
  for (const value of ['2025-01-01T00:20:01,123456789Z', '2025-01-01T00:20:01.1234567890Z',
    '2025-01-01T0:20:01.0000000001Z', '2025-1-01T00:20:01Z', '2025-01-1T00:20:01Z',
    '2025-01-01T00:2:01Z', '2025-01-01T00:20:1Z', '2025-01-01T00:20:01.Z',
    '2025-01-01T00:20:01+00:00', '2025-01-01T00:20:01z', '2025-01-01t00:20:01Z', '2025-02-30T00:20:01Z']) {
    mutateBad(entered(), (r) => { r.records[5].quote.providerT = value; }, 'TIMESTAMP', `timestamp-${value}`);
  }

  const sourceLimit = clone(admissionBase);
  sourceLimit.source += ' '.repeat((64 << 10) - Buffer.byteLength(sourceLimit.source));
  assert.equal(Buffer.byteLength(sourceLimit.source), 64 << 10);
  compare(sourceLimit, 'exact-source-limit');
  sourceLimit.source += ' '; bad(sourceLimit, 'INPUT', 'over-source-limit');
  const tiers = entered();
  tiers.records[5].quote.bids = ['101', '100', '99', '98', '97', '96', '95', '94'].map((price) => ({ price, liquidity: '10000' }));
  tiers.records[5].quote.asks = ['101.2', '102', '103', '104', '105', '106', '107', '108'].map((price) => ({ price, liquidity: '10000' }));
  replay(tiers, 'exact-eight-tiers', (v) => assert.equal(v.position.quantity, '200'));
  mutateBad(tiers, (r) => { r.records[5].quote.bids.push({ price: '93', liquidity: '10000' }); }, 'QUOTE', 'nine-bid-tiers');
  mutateBad(tiers, (r) => { r.records[5].quote.asks.push({ price: '109', liquidity: '10000' }); }, 'QUOTE', 'nine-ask-tiers');
  mutateBad(tiers, (r) => { r.records[5].quote.asks[1].price = '101.2'; }, 'QUOTE_DEPTH', 'equal-depth-price');

  const bars = clone(admissionBase); bars.activationFromT = '2025-01-20T00:00:00Z';
  for (let i = 0; i < 4096; i++) addBar(bars, BigInt(i) * 5n * minute);
  const barResult = compare(bars, 'exact-4096-bars');
  for (const v of barResult) { assert.equal(v.processedBars, 4096); assert.equal(v.events.length, 0); }
  compare(bars, 'exact-4096-bars-retry', barResult.map((v) => v.checkpoint), barResult.map((v) => v.events));
  addBar(bars, 4096n * 5n * minute); bad(bars, 'BAR_LIMIT', '4097-bars');
  const quotes = clone(admissionBase);
  for (let i = 0; i < maxRecords; i++) {
    const q = quote(quotes, BigInt(i), '100', '100.2'); append(quotes, 'quote', q, q.qualifiedT);
  }
  const quoteResult = compare(quotes, 'exact-65536-records-and-quote-identities');
  for (const v of quoteResult) { assert.equal(v.processedRecords, maxRecords); assert.equal(v.processedBars, 0); assert.equal(v.events.length, 0); }
  compare(quotes, 'exact-65536-quotes-retry', quoteResult.map((v) => v.checkpoint), quoteResult.map((v) => v.events));
  const earliest = clone(quotes.records[0].quote); earliest.bids[0].liquidity = '9999';
  earliest.receivedT = quotes.records.at(-1).quote.receivedT; earliest.qualifiedT = quotes.records.at(-1).quote.qualifiedT;
  const conflictingQuotes = clone(quotes); conflictingQuotes.records.at(-1).quote = earliest;
  bad(conflictingQuotes, 'QUOTE_CONFLICT', 'earliest-identity-retained-at-65536-capacity');
  const q = quote(quotes, BigInt(maxRecords), '100', '100.2'); append(quotes, 'quote', q, q.qualifiedT);
  bad(quotes, 'INPUT', '65537-records');
  // A 65537th identity beyond the common 65536-record/identity ceiling is not
  // publicly reachable. Direct limit/output branches are covered by Go tests;
  // this harness neither weakens those limits nor invents a second encoder.
  const exact = healthyRaw + ' '.repeat(maxBytes - Buffer.byteLength(healthyRaw));
  assert.equal(Buffer.byteLength(exact), maxBytes);
  assert.equal(wasm(exact).raw, healthy); assert.equal(native(exact).raw, native(healthyRaw).raw);
  refuse([exact + ' '], 'INPUT', exact + ' ', 'request-max-plus-one');
  const unicodeRaw = JSON.stringify(unicode);
  const unicodeExact = unicodeRaw + ' '.repeat(maxBytes - Buffer.byteLength(unicodeRaw));
  assert(unicodeExact.length < maxBytes && Buffer.byteLength(unicodeExact) === maxBytes);
  assert.equal(wasm(unicodeExact).raw, wasm(unicodeRaw).raw);
  assert.equal(native(unicodeExact).raw, native(unicodeRaw).raw);
  refuse([unicodeExact + ' '], 'INPUT', unicodeExact + ' ', 'multibyte-exact-plus-one');
  const multi = '\u0800'.repeat(Math.floor(maxBytes / 3) + 1);
  assert(multi.length < maxBytes && Buffer.byteLength(multi) > maxBytes);
  refuse([multi], 'INPUT', multi, 'utf16-fits-utf8-overflow');
  refuse(['x'.repeat(maxBytes + 1)], 'INPUT', 'x'.repeat(maxBytes + 1), 'utf16-prebound');
  envelope(native(Buffer.concat([Buffer.from(healthyRaw.slice(0, -1)), Buffer.from([0xff]), Buffer.from('}')])).value, 'INPUT');
  refusals++; liveness();

  const summary = { schema: 'paper-bidask-public-export-proof-v1', status: 'passed',
    runtime: 'actual-native-cli-and-node-hosted-public-go-wasm-export', export: 'engineRunPaperBidAsk',
    comparisons, checkpointResumeSplits: resumeSplits, refusals, priorExportFunctions: oldExports.length,
    inputMaxUTF8Bytes: maxBytes, outputMaxUTF8Bytes: maxOutputBytes, financialComparison: 'exact-no-tolerance',
    targetBoundIdentityProof: 'source-config-producer-binding-input-decision-position-event-cumulative-event-receipt',
    normalizedFields: ['binding.producer.goos', 'binding.producer.goarch', 'bindingDigest', 'events[].id',
      'events[].decisionId', 'events[].positionId', 'position.id', 'position.decisionId', 'pending.decisionId',
      'stateDigest', 'checkpoint.bindingDigest', 'checkpoint.eventDigest', 'checkpoint.stateDigest'],
    privateStateProof: 'same-target-replay-and-corrupt-receipt-refusal; full-API-Go-tests-are-a-separate-required-gate',
    unreachablePublicBranches: 'output/event/identity overflow direct Go tests are separate gates; no alternate encoder',
    checkpointPortability: 'refused-across-targets', nativeRefusalExit: 0, node: process.version,
    nativeProducer: inspect(source)[0].producer, wasmProducer: inspect(source)[1].producer,
    artifacts: { nativeCLI: hash(readFileSync(nativeCLI)), wasm: hash(readFileSync(wasmPath)), shim: hash(readFileSync(shim)) },
    captures, browser: 'unrun', historicalOrLiveData: 'none', releaseOrConsumerAdoption: 'not-claimed' };
  if (evidence) writeFileSync(join(evidence, 'summary.json'), JSON.stringify(summary, null, 2));
  console.log(JSON.stringify(summary));
} finally { rmSync(scratch, { recursive: true, force: true }); }
