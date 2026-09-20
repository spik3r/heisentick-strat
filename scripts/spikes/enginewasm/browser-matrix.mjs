#!/usr/bin/env node
// Run the experimental enginewasm bridge in real Chromium, Firefox, and WebKit
// pages. This intentionally lives outside the release and production paths.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdtempSync, mkdirSync, copyFileSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { tmpdir } from 'node:os';
import { dirname, extname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';
import { spawnSync } from 'node:child_process';

const scriptPath = fileURLToPath(import.meta.url);
const repoRoot = resolve(dirname(scriptPath), '../../..');
const outputFlag = process.argv.indexOf('--output');
const outputPath = outputFlag === -1 ? null : resolve(process.argv[outputFlag + 1] || '');
if (outputFlag !== -1 && !process.argv[outputFlag + 1]) throw new Error('--output requires a path');
const playwrightPackage = process.env.ENGINEWASM_PLAYWRIGHT_PACKAGE || 'playwright';
const require = createRequire(import.meta.url);
const { chromium, firefox, webkit } = require(playwrightPackage);
const playwrightVersion = require(`${playwrightPackage}/package.json`).version;

function sha256(value) {
  return createHash('sha256').update(value).digest('hex');
}

function canonical(value) {
  if (Array.isArray(value)) return value.map(canonical);
  if (value && typeof value === 'object') return Object.fromEntries(Object.keys(value).sort().map((key) => [key, canonical(value[key])]));
  return value;
}

function canonicalJSON(value) {
  return JSON.stringify(canonical(value));
}

function command(command, args) {
  const result = spawnSync(command, args, { cwd: repoRoot, encoding: 'utf8' });
  if (result.status !== 0) throw new Error(`${command} ${args.join(' ')} failed:\n${result.stderr || result.stdout}`);
  return result.stdout;
}

function mimeType(path) {
  return new Map([['.html', 'text/html; charset=utf-8'], ['.js', 'text/javascript; charset=utf-8'], ['.json', 'application/json; charset=utf-8'], ['.strat', 'text/plain; charset=utf-8'], ['.wasm', 'application/wasm']]).get(extname(path)) || 'application/octet-stream';
}

async function serve(directory) {
  const server = createServer((request, response) => {
    const pathname = decodeURIComponent(new URL(request.url, 'http://127.0.0.1').pathname);
    const file = resolve(directory, `.${pathname === '/' ? '/index.html' : pathname}`);
    if (!file.startsWith(`${directory}/`)) {
      response.writeHead(403).end();
      return;
    }
    try {
      const body = readFileSync(file);
      response.writeHead(200, { 'content-type': mimeType(file), 'cache-control': 'no-store' }).end(body);
    } catch {
      response.writeHead(404).end();
    }
  });
  await new Promise((resolveServer) => server.listen(0, '127.0.0.1', resolveServer));
  const { port } = server.address();
  return { baseURL: `http://127.0.0.1:${port}`, close: () => new Promise((resolveServer) => server.close(resolveServer)) };
}

function prepareArtifacts(directory) {
  const wasmPath = join(directory, 'engine.wasm');
  const goRoot = command('go', ['env', 'GOROOT']).trim();
  // A native build must be used for expected results. The browser JSON bridge
  // is deliberately not its own oracle.
  // The artifact is evidence, not a release. Exclude VCS stamping so its hash
  // is stable when only the harness or recorded evidence changes.
  const wasmBuild = spawnSync('go', ['build', '-trimpath', '-buildvcs=false', '-o', wasmPath, './cmd/enginewasm'], {
    cwd: repoRoot,
    encoding: 'utf8',
    env: { ...process.env, GOOS: 'js', GOARCH: 'wasm' },
  });
  if (wasmBuild.status !== 0) throw new Error(`WASM build failed:\n${wasmBuild.stderr || wasmBuild.stdout}`);
  copyFileSync(join(goRoot, 'lib', 'wasm', 'wasm_exec.js'), join(directory, 'wasm_exec.js'));

  const fixtureDir = join(repoRoot, 'conformance', 'run');
  const expectedDir = join(directory, 'expected');
  mkdirSync(expectedDir);
  const cases = [];
  for (const fixtureName of readdirSync(fixtureDir).filter((name) => name.startsWith('deployed-') && name.endsWith('.fixture.json')).sort()) {
    const caseName = fixtureName.slice(0, -'.fixture.json'.length);
    const fixturePath = join(fixtureDir, fixtureName);
    const sourcePath = join(fixtureDir, `${caseName}.strat`);
    const goldenPath = join(fixtureDir, `${caseName}.trades.json`);
    const fixture = JSON.parse(readFileSync(fixturePath));
    const source = readFileSync(sourcePath, 'utf8');
    const nativeRaw = command('go', ['run', './cmd/enginewasm', relative(repoRoot, fixturePath), relative(repoRoot, sourcePath)]).trim();
    const native = JSON.parse(nativeRaw);
    const golden = JSON.parse(readFileSync(goldenPath));
    assert.equal(canonicalJSON(native.trades), canonicalJSON(golden.trades), `${caseName}: direct native output differs from committed trades golden`);
    copyFileSync(fixturePath, join(directory, `${caseName}.fixture.json`));
    copyFileSync(sourcePath, join(directory, `${caseName}.strat`));
    writeFileSync(join(expectedDir, `${caseName}.native.json`), `${nativeRaw}\n`);
    cases.push({
      case: caseName,
      fixture: `${caseName}.fixture.json`,
      source: `${caseName}.strat`,
      expected: `expected/${caseName}.native.json`,
      fixtureSha256: sha256(readFileSync(fixturePath)),
      sourceSha256: sha256(source),
      nativeOutputSha256: sha256(nativeRaw),
      goldenTradesSha256: sha256(readFileSync(goldenPath)),
      columnar: fixture.higherTimeframe ? { supported: false, reason: `fixture requires higher timeframe ${fixture.higherTimeframe}` } : { supported: true },
    });
  }
  if (cases.length === 0) throw new Error('no deployed fixtures found for the browser matrix');
  return { cases, artifact: { sha256: sha256(readFileSync(wasmPath)), wasmExecSha256: sha256(readFileSync(join(directory, 'wasm_exec.js'))) } };
}

const pageFunction = async ({ baseURL, entry }) => {
  const canonical = (value) => {
    if (Array.isArray(value)) return value.map(canonical);
    if (value && typeof value === 'object') return Object.fromEntries(Object.keys(value).sort().map((key) => [key, canonical(value[key])]));
    return value;
  };
  const stable = (value) => JSON.stringify(canonical(value));
  const firstDifference = (left, right, path = '$') => {
    if (Object.is(left, right)) return null;
    if (!left || !right || typeof left !== 'object' || typeof right !== 'object') return `${path}: ${JSON.stringify(left)} !== ${JSON.stringify(right)}`;
    if (Array.isArray(left) !== Array.isArray(right)) return `${path}: array shape differs`;
    if (Array.isArray(left) && left.length !== right.length) return `${path}: length ${left.length} !== ${right.length}`;
    const keys = [...new Set([...Object.keys(left), ...Object.keys(right)])].sort();
    for (const key of keys) {
      if (!Object.hasOwn(left, key) || !Object.hasOwn(right, key)) return `${path}.${key}: key presence differs`;
      const difference = firstDifference(left[key], right[key], `${path}.${key}`);
      if (difference) return difference;
    }
    return null;
  };
  const [fixture, source, expected] = await Promise.all([
    fetch(`${baseURL}/${entry.fixture}`).then((response) => response.json()),
    fetch(`${baseURL}/${entry.source}`).then((response) => response.text()),
    fetch(`${baseURL}/${entry.expected}`).then((response) => response.json()),
  ]);
  const jsonStart = performance.now();
  const jsonOutput = JSON.parse(globalThis.engineRunFixture(JSON.stringify(fixture), source));
  const jsonMs = performance.now() - jsonStart;
  if (jsonOutput.error) throw new Error(`JSON bridge: ${jsonOutput.error}`);
  if (stable(jsonOutput) !== stable(expected)) throw new Error('JSON bridge differs from direct native output');
  const result = { case: entry.case, jsonBridge: { completeNativeEquality: true, wallMs: jsonMs, tradeCount: jsonOutput.trades.length } };
  const cols = Array.from({ length: 6 }, (_, index) => Float64Array.from(fixture.bars, (bar) => bar[index]));
  const meta = { schema: 'enginewasm-columnar-v1', case: fixture.case, strategyId: fixture.strategyId, symbol: fixture.symbol, timeframe: fixture.timeframe, rangeMethod: fixture.rangeMethod, costs: fixture.costs };
  if (!entry.columnar.supported) {
    meta.higherTimeframe = fixture.higherTimeframe;
    const unsupported = globalThis.engineRunColumns(JSON.stringify(meta), source, ...cols);
    if (unsupported.ok || !/unsupported/.test(unsupported.error)) throw new Error('higher-timeframe column input was not rejected explicitly');
    result.columnar = { status: 'unsupported', reason: entry.columnar.reason, rejection: unsupported.error };
    return result;
  }
  const columnStart = performance.now();
  const columnOutput = globalThis.engineRunColumns(JSON.stringify(meta), source, ...cols);
  const columnMs = performance.now() - columnStart;
  if (!columnOutput.ok) throw new Error(`column bridge: ${columnOutput.error}`);
  const text = JSON.parse(columnOutput.stringsJSON);
  const names = ['entry', 'entryIndex', 'entryT', 'exit', 'exitIndex', 'exitT', 'initialSl', 'initialTp', 'pnl', 'points', 'size', 'sl', 'tp'];
  if (text.length === 0) throw new Error('supported columnar fixture produced no trades');
  if (columnOutput.trades.length !== text.length * names.length) throw new Error(`typed trade values=${columnOutput.trades.length}, want ${text.length * names.length}`);
  const trades = text.map((stringFields, index) => {
    const trade = Object.fromEntries(names.map((name, offset) => [name, columnOutput.trades[index * names.length + offset]]));
    // noStop/noTarget are transport sentinels for nullable float slots. They
    // are represented by null stops/targets in the native JSON result.
    for (const name of ['side', 'reason', 'tag', 'meta', 'partial']) {
      if (stringFields[name]) trade[name] = stringFields[name];
    }
    if (stringFields.noStop) trade.initialSl = trade.sl = null;
    if (stringFields.noTarget) trade.initialTp = trade.tp = null;
    return trade;
  });
  if (stable(trades) !== stable(expected.trades)) throw new Error(`reconstructed columnar trades differ from direct native trades: ${firstDifference(trades, expected.trades)}`);
  const malformed = {
    mismatchedLengthRejected: !globalThis.engineRunColumns(JSON.stringify(meta), source, cols[0], cols[1].subarray(1), cols[2], cols[3], cols[4], cols[5]).ok,
    nonFloat64Rejected: !globalThis.engineRunColumns(JSON.stringify(meta), source, [], cols[1], cols[2], cols[3], cols[4], cols[5]).ok,
    unknownMetadataRejected: !globalThis.engineRunColumns(JSON.stringify({ ...meta, extra: true }), source, ...cols).ok,
    invalidSourceRejected: !globalThis.engineRunColumns(JSON.stringify(meta), 'not a strategy', ...cols).ok,
  };
  if (Object.values(malformed).some((accepted) => !accepted)) throw new Error('one or more malformed inputs were accepted');
  result.columnar = { status: 'verified', completeTradeEquality: true, tradeCount: trades.length, wallMs: columnMs, malformed, timings: columnOutput.timings };
  return result;
};

async function runBrowser(name, browserType, baseURL, entries) {
  const browser = await browserType.launch({ headless: true });
  try {
    const page = await browser.newPage();
    await page.goto(baseURL, { waitUntil: 'load' });
    await page.addScriptTag({ url: `${baseURL}/wasm_exec.js` });
    await page.evaluate(async (wasmURL) => {
      const go = new globalThis.Go();
      const { instance } = await WebAssembly.instantiateStreaming(fetch(wasmURL), go.importObject);
      void go.run(instance);
      await new Promise((resolveReady, rejectReady) => {
        const deadline = performance.now() + 15_000;
        const check = () => {
          if (typeof globalThis.engineRunFixture === 'function' && typeof globalThis.engineRunColumns === 'function') return resolveReady();
          if (performance.now() > deadline) return rejectReady(new Error('enginewasm did not initialise'));
          setTimeout(check, 10);
        };
        check();
      });
    }, `${baseURL}/engine.wasm`);
    const userAgent = await page.evaluate(() => navigator.userAgent);
    const cases = [];
    for (const entry of entries) cases.push(await page.evaluate(pageFunction, { baseURL, entry }));
    if (!cases.some((entry) => entry.columnar.status === 'verified' && entry.columnar.tradeCount > 0)) {
      throw new Error(`${name}: no supported nonempty columnar case was exercised`);
    }
    return { name, version: browser.version(), userAgent, cases };
  } finally {
    await browser.close();
  }
}

const artifactDirectory = mkdtempSync(join(tmpdir(), 'heisentick-enginewasm-browser-matrix-'));
let server;
try {
  writeFileSync(join(artifactDirectory, 'index.html'), '<!doctype html><title>enginewasm matrix</title>');
  const prepared = prepareArtifacts(artifactDirectory);
  server = await serve(artifactDirectory);
  const sourceHead = command('git', ['rev-parse', 'HEAD']).trim();
  const results = [];
  for (const [name, browserType] of [['chromium', chromium], ['firefox', firefox], ['webkit', webkit]]) {
    results.push(await runBrowser(name, browserType, server.baseURL, prepared.cases));
  }
  const evidence = {
    schema: 'enginewasm-browser-matrix-v1',
    source: {
      repository: 'spik3r/heisentick-strat',
      engineCommit: sourceHead,
      harnessPath: relative(repoRoot, scriptPath),
      harnessSha256: sha256(readFileSync(scriptPath)),
      goVersion: command('go', ['version']).trim(),
      playwrightVersion,
    },
    artifact: prepared.artifact,
    fixtures: prepared.cases,
    browsers: results,
    reproducibleCommands: {
      installPlaywright: `npm install --prefix /tmp/heisentick-enginewasm-playwright playwright@${playwrightVersion}`,
      installBrowsers: 'npx --prefix /tmp/heisentick-enginewasm-playwright playwright install chromium firefox webkit',
      runMatrix: 'ENGINEWASM_PLAYWRIGHT_PACKAGE=/tmp/heisentick-enginewasm-playwright/node_modules/playwright node scripts/spikes/enginewasm/browser-matrix.mjs --output scripts/spikes/enginewasm/evidence/browser-matrix.json',
    },
    timingNote: 'Wall timings are browser smoke observations. The deployed fixtures are synthetic conformance inputs and do not establish real-market performance.',
    largerBenchmarkData: 'A larger benchmark needs a separately supplied, read-only public or user-provided market-data file with its source, date range, bar count, and SHA-256 recorded. This harness neither fetches AWS data nor assumes private local data.',
  };
  const serialized = `${JSON.stringify(evidence, null, 2)}\n`;
  if (outputPath) {
    mkdirSync(dirname(outputPath), { recursive: true });
    writeFileSync(outputPath, serialized);
  }
  process.stdout.write(serialized);
} finally {
  if (server) await server.close();
  rmSync(artifactDirectory, { recursive: true, force: true });
}
