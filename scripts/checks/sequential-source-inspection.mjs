// Actual native CLI and Node-hosted Go/WASM inspection. No JS strategy engine.
// Usage: node scripts/checks/sequential-source-inspection.mjs <heisentick> <dsl> <engine.wasm> <wasm_exec.js>
import assert from 'node:assert/strict';
import { webcrypto, createHash } from 'node:crypto';
import { execFileSync, spawnSync } from 'node:child_process';
import { mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';

const args = process.argv.slice(2);
assert.equal(args.length, 4, 'four artifact paths required');
const [nativeCLI, nativeDSL, wasm, shim] = args.map((path) => resolve(path));
const root = fileURLToPath(new URL('../../', import.meta.url));
const schemaPath = join(root, 'spec/schemas/sequential-source-capability-v1.schema.json');
const schema = JSON.parse(readFileSync(schemaPath, 'utf8'));
const hash = (value) => createHash('sha256').update(value).digest('hex');

// Test-only closed-schema validator. Fail on unknown validation keywords;
// this is not an application admission implementation or general validator.
const known = new Set(['$schema', '$id', 'title', 'description', '$defs', '$ref', 'oneOf', 'anyOf', 'type', 'properties', 'required', 'additionalProperties', 'items', 'minItems', 'maxItems', 'minLength', 'minimum', 'maximum', 'pattern', 'enum', 'const']);
function validate(value, rule = schema, base = schema, path = schemaPath) {
  for (const key of Object.keys(rule)) assert(known.has(key), `unknown schema keyword ${key}`);
  if (rule.$ref) {
    const [file, fragment] = rule.$ref.split('#');
    const nextPath = file ? resolve(dirname(path), file) : path;
    const nextBase = file ? JSON.parse(readFileSync(nextPath, 'utf8')) : base;
    let target = nextBase;
    if (fragment) for (const part of fragment.slice(1).split('/')) target = target[part.replaceAll('~1', '/').replaceAll('~0', '~')];
    assert(target, 'unresolved local schema reference');
    return validate(value, target, nextBase, nextPath);
  }
  if (rule.oneOf || rule.anyOf) {
    const matches = (rule.oneOf ?? rule.anyOf).filter((child) => { try { validate(value, child, base, path); return true; } catch { return false; } });
    assert(rule.oneOf ? matches.length === 1 : matches.length > 0, 'schema alternatives');
  }
  if (Object.hasOwn(rule, 'const')) assert.deepEqual(value, rule.const);
  if (rule.enum) assert(rule.enum.includes(value), 'schema enum');
  switch (rule.type) {
    case 'object':
      assert(value !== null && typeof value === 'object' && !Array.isArray(value), 'schema object');
      for (const key of rule.required ?? []) assert(Object.hasOwn(value, key), `required ${key}`);
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
      if (rule.pattern) assert(new RegExp(rule.pattern).test(value), 'schema pattern');
      break;
    case 'integer': assert(Number.isSafeInteger(value), 'schema integer'); break;
    case 'boolean': assert.equal(typeof value, 'boolean'); break;
    case 'null': assert.equal(value, null); break;
    case undefined: break;
    default: assert.fail(`unsupported schema type ${rule.type}`);
  }
  if (rule.minimum !== undefined) assert(value >= rule.minimum);
  if (rule.maximum !== undefined) assert(value <= rule.maximum);
}

globalThis.crypto ??= webcrypto;
createRequire(import.meta.url)(shim);
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(readFileSync(wasm), go.importObject);
void go.run(instance);
assert.equal(typeof globalThis.engineInspectSequentialSource, 'function');
const scratch = mkdtempSync(join(tmpdir(), 'sequential-inspection-'));
const sourcePath = join(scratch, 'source.strat');
let comparisons = 0, refusals = 0, mutations = 0;
const samples = [];

function compare(source, label) {
  writeFileSync(sourcePath, source);
  const rawNative = execFileSync(nativeCLI, ['inspect-sequential-source', `--dsl-file=${sourcePath}`], { encoding: 'utf8' });
  const native = JSON.parse(rawNative);
  const rawWasm = globalThis.engineInspectSequentialSource(source);
  const actual = JSON.parse(rawWasm);
  validate(native); validate(actual);
  assert.equal(rawWasm, globalThis.engineInspectSequentialSource(source), `${label}: repeat changed`);
  assert.equal(actual.producer.goos, 'js');
  assert.equal(actual.producer.goarch, 'wasm');
  assert.equal(native.producer.goos, process.platform);
  assert.equal(native.producer.goarch, process.arch === 'x64' ? 'amd64' : process.arch);
  assert.equal(actual.producer.goVersion, native.producer.goVersion);
  assert(!Object.hasOwn(actual.producer, 'assetSha256'));
  const semantic = ({ producer, ...result }) => result;
  assert.deepEqual(semantic(actual), semantic(native), `${label}: native/WASM facts differ`);
  const parsed = JSON.parse(execFileSync(nativeDSL, [], { input: source, encoding: 'utf8' }));
  assert.deepEqual(actual.result, parsed.result, `${label}: original Go ParseResult changed`);
  assert.equal(actual.grammarHash, parsed.grammarHash);
  assert.equal(actual.source.sha256, hash(Buffer.from(source, 'utf8')));
  assert.equal(actual.source.utf8Bytes, Buffer.byteLength(source, 'utf8'));
  if (actual.result.errors.length) {
    assert.equal(actual.configIdentity, undefined);
    assert.equal(actual.capabilities.sourceParsed.status, 'unsupported');
    assert.notEqual(actual.capabilities.engineSupported.status, 'supported');
  } else if (actual.classification?.family === 'sequentialFull') {
    assert.equal(actual.capabilities.engineSupported.status, 'supported');
    assert.equal(actual.capabilities.sourceAccounting.status, 'unsupported');
    assert.equal(actual.capabilities.fixtureAccounting.status, 'unknown');
    for (const key of ['calendarExecution', 'preserveOpenEndpoint', 'preservedPrefix', 'checkpointResume']) assert.equal(actual.capabilities[key].status, 'unsupported');
    for (const key of ['marketDataReadiness', 'consumerIntegration', 'recovery']) assert.equal(actual.capabilities[key].status, 'unknown');
  } else {
    assert.equal(actual.capabilities.engineSupported.status, 'unknown');
    assert.equal(actual.genericExecutionContract, undefined);
  }
  comparisons++;
  samples.push(actual);
  return actual;
}

function reject(call, expectedCode) {
  const actual = JSON.parse(call());
  validate(actual);
  assert.equal(actual.ok, false);
  assert.equal(actual.error.code, expectedCode);
  for (const key of ['result', 'source', 'classification', 'capabilities', 'configIdentity', 'genericExecutionContract']) assert.equal(actual[key], undefined);
  refusals++;
  samples.push(actual);
}

try {
  const corpus = join(root, 'conformance/parse');
  const paths = readdirSync(corpus).filter((name) => name.endsWith('.strat')).sort();
  assert.equal(paths.length, 163);
  const runFixture = readFileSync(join(root, 'conformance/run/family-sequential-full-e1-long.fixture.json'), 'utf8');
  const runSource = readFileSync(join(root, 'conformance/run/family-sequential-full-e1-long.strat'), 'utf8');
  const runBefore = globalThis.engineRunFixture(runFixture, runSource);
  assert(JSON.parse(runBefore).tradeCount > 0, 'meaningful old execution required');
  for (const name of paths) compare(new TextDecoder('utf-8', { fatal: true }).decode(readFileSync(join(corpus, name))), name);
  const authored = runSource.replace('Synthetic Sequential E1', 'Independent authored Ω').replace('symbol SYNTH', 'symbol XAUUSD').replace('timeframe 5m', 'timeframe 15m').replace('riskUsd 100 ', 'riskUsd 31 ').replace('maxNotionalUsd 100000', 'maxNotionalUsd 1000');
  const first = compare(authored, 'authored E1');
  compare(authored.replace('policy E1', 'policy E2'), 'authored E2');
  const commented = compare(`# extra comment\n${authored}\n`, 'source-only change');
  assert.notEqual(first.source.sha256, commented.source.sha256);
  assert.equal(first.configIdentity.sha256, commented.configIdentity.sha256);
  const renamed = compare(authored.replace('Independent authored Ω', 'Renamed source'), 'whole-cfg metadata change');
  assert.notEqual(first.configIdentity.sha256, renamed.configIdentity.sha256);
  compare(authored.replace('riskUsd 31 ', 'riskUsd 0 '), 'unsupported risk');
  compare(authored.replace('policy E1', 'policy P1'), 'unsupported policy');
  compare(authored.replace('public_approx.v1', 'public_approx.v99'), 'unsupported profile');
  assert.equal(globalThis.engineRunFixture(runFixture, runSource), runBefore, 'inspection changed old engine state');

  for (const args of [[], [1], [1n], [Symbol('unsupported')], [true], [() => {}], [null], [undefined], [{}], [authored, { ready: true }]]) {
    reject(() => globalThis.engineInspectSequentialSource(...args), 'SEQUENTIAL_INSPECTION_ARGUMENTS');
    assert.deepEqual(JSON.parse(globalThis.engineInspectSequentialSource(authored)), first, 'wrong-type call damaged inspector');
    assert.equal(globalThis.engineRunFixture(runFixture, runSource), runBefore, 'wrong-type call damaged existing execution export');
  }
  reject(() => globalThis.engineInspectSequentialSource(''), 'SEQUENTIAL_INSPECTION_SOURCE');
  for (const bad of ['\ud800', '\udfff', '\ud800x', '\udc00\ud800', '\ud800\udc00\ud800']) reject(() => globalThis.engineInspectSequentialSource(`# ${bad}\n${authored}`), 'SEQUENTIAL_INSPECTION_SOURCE');
  const max = 8 << 20;
  reject(() => globalThis.engineInspectSequentialSource('x'.repeat(max + 1)), 'SEQUENTIAL_INSPECTION_SOURCE');
  reject(() => globalThis.engineInspectSequentialSource('€'.repeat(Math.floor(max / 3) + 1)), 'SEQUENTIAL_INSPECTION_SOURCE');
  const atLimit = JSON.parse(globalThis.engineInspectSequentialSource(`#${'x'.repeat(max - 2)}\n`));
  validate(atLimit); assert.equal(atLimit.ok, true); assert.equal(atLimit.source.utf8Bytes, max);
  for (const bytes of [Buffer.from([0xff]), Buffer.alloc(max + 1, 120)]) {
    writeFileSync(sourcePath, bytes);
    const failed = spawnSync(nativeCLI, ['inspect-sequential-source', `--dsl-file=${sourcePath}`], { encoding: 'utf8' });
    assert.notEqual(failed.status, 0);
    reject(() => failed.stdout, 'SEQUENTIAL_INSPECTION_SOURCE');
    assert(!failed.stderr.includes(sourcePath), 'native error leaked input path');
  }
  for (const alter of [
    (v) => { delete v.producer; }, (v) => { v.extra = true; },
    (v) => { v.configIdentity.sha256 = 'wrong'; },
    (v) => { v.capabilities.engineSupported.status = 'ready'; },
    (v) => { v.producer.assetSha256 = 'f'.repeat(64); },
    (v) => { v.source.utf8Bytes = 1.5; },
    (v) => { v.genericExecutionContract.costs.fillOn = ['close']; },
    (v) => { v.schema = 'strat-sequential-source-capability-v99'; },
  ]) {
    const changed = structuredClone(first); alter(changed);
    assert.throws(() => validate(changed)); mutations++;
  }
  console.log(JSON.stringify({ schema: 'sequential-source-inspection-proof-v1', status: 'passed',
    comparisons, transportRefusals: refusals, schemaMutations: mutations,
    runtime: 'actual-native-and-node-go-wasm', nativeTarget: process.arch,
    producer: first.producer, schemaSha256: hash(readFileSync(schemaPath)),
    artifacts: { nativeCLI: hash(readFileSync(nativeCLI)), nativeDSL: hash(readFileSync(nativeDSL)), wasm: hash(readFileSync(wasm)), shim: hash(readFileSync(shim)) },
    browser: 'unrun', marketReadiness: 'not-evaluated', executionExpansion: false }));
} finally { rmSync(scratch, { recursive: true, force: true }); }
