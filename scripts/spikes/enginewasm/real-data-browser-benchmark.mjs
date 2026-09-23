#!/usr/bin/env node
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { cpus, platform, release, tmpdir, totalmem } from 'node:os';
import { basename, dirname, extname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';
import { brotliCompressSync } from 'node:zlib';

const scriptPath = fileURLToPath(import.meta.url);
const repoRoot = resolve(dirname(scriptPath), '../../..');
const args = Object.fromEntries(process.argv.slice(2).map((value) => {
  const [key, ...rest] = value.replace(/^--/, '').split('=');
  return [key, rest.join('=') || true];
}));
for (const required of ['wasm', 'wasm-exec', 'native', 'data', 'app', 'output', 'release', 'release-commit']) {
  if (!args[required] || args[required] === true) throw new Error(`--${required}=<path> is required`);
}
const wasmPath = resolve(args.wasm);
const wasmExecPath = resolve(args['wasm-exec']);
const nativePath = resolve(args.native);
const dataPath = resolve(args.data);
const appRoot = resolve(args.app);
const outputPath = resolve(args.output);
const stratRelease = String(args.release);
const releaseCommit = String(args['release-commit']);
const repeats = Number(args.repeats || 3);
if (!Number.isInteger(repeats) || repeats < 1) throw new Error('--repeats must be a positive integer');
const cellTimeoutMs = Number(args['cell-timeout-ms'] || 15 * 60 * 1000);
if (!Number.isInteger(cellTimeoutMs) || cellTimeoutMs < 1) throw new Error('--cell-timeout-ms must be a positive integer');
const requestedBrowsers = String(args.browsers || 'chromium,firefox,webkit').split(',').filter(Boolean);
const playwrightPackage = process.env.ENGINEWASM_PLAYWRIGHT_PACKAGE || 'playwright';
const require = createRequire(import.meta.url);
const playwright = require(playwrightPackage);
const playwrightVersion = require(`${playwrightPackage}/package.json`).version;

const source = [
  'dsl v7', 'strategy "T-F0 XAUUSD 5m ORB measurement" {',
  'description "Bridge measurement input."', '}', 'market conditions {',
  'slices(XAUUSD 5m)', 'trade window unrestricted',
  'day type in (trending, ranging, choppy)', 'trendiness below 999', '}',
  'setup {', 'type: opening range breakout', 'opening range every 3 hours UTC',
  'break beyond range edge', 'hold 1 candle', '}', 'filters {', 'side both', '}',
  'risk {', 'stop 10 pips', '}', 'target {', 'target 20 pips', '}',
  'management {', 'move stop to breakeven after 999R plus 0 ATR', '}',
  'execution {', 'risk: 200 USD', '}',
].join('\n');

function sha256(value) {
  return createHash('sha256').update(value).digest('hex');
}

function canonical(value) {
  if (Array.isArray(value)) return value.map(canonical);
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.keys(value).sort().map((key) => [key, canonical(value[key])]));
  }
  if (typeof value === 'number') {
    if (!Number.isFinite(value)) return null;
    const rounded = Number(value.toPrecision(15));
    return Object.is(rounded, -0) ? 0 : rounded;
  }
  return value;
}

assert.equal(canonical(2047.445 + 2), 2049.445,
  'result comparison must use the established 15-significant-digit serialization contract');

function comparableTrade(trade) {
  const fields = ['entry', 'entryIndex', 'entryT', 'exit', 'exitIndex', 'exitT', 'initialSl', 'initialTp', 'pnl', 'points', 'size', 'sl', 'tp', 'side', 'reason', 'rule', 'tag', 'meta', 'partial'];
  const out = {};
  for (const field of fields) {
    if (field in trade) out[field] = trade[field];
  }
  out.partial = out.partial || false;
  return canonical(out);
}

function tradeDigest(trades) {
  return sha256(JSON.stringify(trades.map(comparableTrade)));
}

function parseBBT1(path) {
  const data = readFileSync(path);
  if (data.readUInt32LE(0) !== 0x31425442 || data.readUInt32LE(4) !== 1 || data.readUInt32LE(12) !== 6) {
    throw new Error('expected BBT1 v1 six-column input');
  }
  return { data, count: data.readUInt32LE(8) };
}

function writePrefix(data, count, size, path) {
  const output = Buffer.allocUnsafe(16 + 6 * size * 8);
  data.copy(output, 0, 0, 16);
  output.writeUInt32LE(size, 8);
  for (let column = 0; column < 6; column++) {
    data.copy(output, 16 + column * size * 8, 16 + column * count * 8, 16 + (column * count + size) * 8);
  }
  writeFileSync(path, output);
}

function runNative(root, size) {
  const runs = [];
  let expectedDigest;
  let tradeCount;
  for (let repeat = 0; repeat < repeats; repeat++) {
    const started = performance.now();
    const result = spawnSync(nativePath, [
      'report', `--dsl-file=${join(root, 'strategy.strat')}`, '--symbol=XAUUSD', '--tf=5m', '--range=zone',
      '--slippage=0', '--include-trades=1', '--json-only=1', `--data-root=${join(root, String(size))}`,
    ], { encoding: 'utf8', maxBuffer: 512 * 1024 * 1024 });
    const wallMs = performance.now() - started;
    if (result.status !== 0) throw new Error(`native ${size} failed: ${result.stderr || result.stdout}`);
    const payload = JSON.parse(result.stdout);
    const trades = payload.slices[0].trades;
    const digest = tradeDigest(trades);
    expectedDigest ??= digest;
    tradeCount ??= trades.length;
    assert.equal(digest, expectedDigest, `native ${size} repeat ${repeat + 1} changed trade output`);
    runs.push({ repeat: repeat + 1, wallMs });
  }
  return { size, status: 'measured', tradeCount, tradeDigest: expectedDigest, runs };
}

function mimeType(path) {
  return new Map([
    ['.html', 'text/html; charset=utf-8'], ['.js', 'text/javascript; charset=utf-8'],
    ['.wasm', 'application/wasm'], ['.bin', 'application/octet-stream'], ['.strat', 'text/plain; charset=utf-8'],
  ]).get(extname(path)) || 'application/octet-stream';
}

async function serve(artifactRoot) {
  const roots = new Map([['artifact', artifactRoot], ['app', appRoot], ['data', dirname(dataPath)]]);
  const server = createServer((request, response) => {
    const parts = decodeURIComponent(new URL(request.url, 'http://127.0.0.1').pathname).split('/').filter(Boolean);
    const root = roots.get(parts.shift());
    if (!root) return response.writeHead(404).end();
    const file = resolve(root, parts.join('/'));
    if (file !== root && !file.startsWith(`${root}/`)) return response.writeHead(403).end();
    try {
      const body = readFileSync(file);
      response.writeHead(200, { 'content-type': mimeType(file), 'cache-control': 'no-store' }).end(body);
    } catch {
      response.writeHead(404).end();
    }
  });
  await new Promise((resolveServer) => server.listen(0, '127.0.0.1', resolveServer));
  return {
    baseURL: `http://127.0.0.1:${server.address().port}`,
    close: () => new Promise((resolveServer) => server.close(resolveServer)),
  };
}

const browserFunction = async ({ baseURL, size, repeats, stratRelease }) => {
  const timings = {};
  const fetchStart = performance.now();
  const wasmResponse = await fetch(`${baseURL}/artifact/enginewasm.wasm`);
  const wasmBytes = await wasmResponse.arrayBuffer();
  timings.assetFetchMs = performance.now() - fetchStart;
  timings.assetBytes = wasmBytes.byteLength;
  const compileStart = performance.now();
  const module = await WebAssembly.compile(wasmBytes);
  timings.compileMs = performance.now() - compileStart;
  const go = new globalThis.Go();
  const instantiateStart = performance.now();
  const instance = await WebAssembly.instantiate(module, go.importObject);
  timings.instantiateMs = performance.now() - instantiateStart;
  const readyStart = performance.now();
  void go.run(instance);
  while (typeof globalThis.engineRunColumns !== 'function') {
    if (performance.now() - readyStart > 30_000) throw new Error('enginewasm did not initialise');
    await new Promise((resolveWait) => setTimeout(resolveWait, 10));
  }
  timings.runtimeReadyMs = performance.now() - readyStart;

  const moduleStart = performance.now();
  const [{ barsViewFromCols }, { buildContextColumns, contextCursorFromColumns, runBacktest }, { createSpecStrategy }, { applyStratReleaseSemantics }] = await Promise.all([
    import(`${baseURL}/app/engine/chartDataView.js`), import(`${baseURL}/app/engine/engine.js`), import(`${baseURL}/app/engine/dsl/specStrategy.js`), import(`${baseURL}/app/engine/stratReleaseSemantics.js`),
  ]);
  const jsStrategy = createSpecStrategy(globalThis.__tf0Source);
  timings.jsModuleAndParseMs = performance.now() - moduleStart;

  const dataStart = performance.now();
  const dataBytes = await fetch(`${baseURL}/data/${globalThis.__tf0DataName}`).then((response) => response.arrayBuffer());
  timings.dataFetchMs = performance.now() - dataStart;
  const decodeStart = performance.now();
  const view = new DataView(dataBytes);
  if (view.getUint32(0, true) !== 0x31425442 || view.getUint32(4, true) !== 1 || view.getUint32(12, true) !== 6) throw new Error('invalid BBT1 input');
  const count = view.getUint32(8, true);
  const columns = Array.from({ length: 6 }, (_, index) => new Float64Array(dataBytes, 16 + index * count * 8, count));
  timings.dataDecodeMs = performance.now() - decodeStart;

  const canonical = (value) => {
    if (Array.isArray(value)) return value.map(canonical);
    if (value && typeof value === 'object') return Object.fromEntries(Object.keys(value).sort().map((key) => [key, canonical(value[key])]));
    if (typeof value === 'number') {
      if (!Number.isFinite(value)) return null;
      const rounded = Number(value.toPrecision(15));
      return Object.is(rounded, -0) ? 0 : rounded;
    }
    return value;
  };
  const comparableTrade = (trade) => {
    const fields = ['entry', 'entryIndex', 'entryT', 'exit', 'exitIndex', 'exitT', 'initialSl', 'initialTp', 'pnl', 'points', 'size', 'sl', 'tp', 'side', 'reason', 'rule', 'tag', 'meta', 'partial'];
    const out = {};
    for (const field of fields) if (field in trade) out[field] = trade[field];
    out.partial = out.partial || false;
    return canonical(out);
  };
  const digest = async (trades) => {
    const bytes = new TextEncoder().encode(JSON.stringify(trades.map(comparableTrade)));
    return Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256', bytes)), (byte) => byte.toString(16).padStart(2, '0')).join('');
  };
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
  const reconstruct = (output) => {
    const text = JSON.parse(output.stringsJSON);
    const fields = ['entry', 'entryIndex', 'entryT', 'exit', 'exitIndex', 'exitT', 'initialSl', 'initialTp', 'pnl', 'points', 'size', 'sl', 'tp'];
    return text.map((strings, index) => {
      const trade = Object.fromEntries(fields.map((field, offset) => [field, output.trades[index * 13 + offset]]));
      for (const field of ['side', 'reason', 'rule', 'tag', 'meta', 'partial']) if (field in strings) trade[field] = strings[field];
      if (strings.noStop) trade.initialSl = trade.sl = null;
      if (strings.noTarget) trade.initialTp = trade.tp = null;
      return trade;
    });
  };
  const slice = Object.fromEntries(['t', 'o', 'h', 'l', 'c', 'v'].map((key, index) => [key, columns[index].subarray(0, size)]));
    const meta = JSON.stringify({ schema: 'enginewasm-columnar-v1', case: 't-f0-xauusd-5m', strategyId: 't-f0-xauusd-5m', symbol: 'XAUUSD', timeframe: '5m', rangeMethod: 'zone', costs: { fillOn: 'close', startEquity: 10000 } });
    const runs = [];
    let expectedDigest;
    let jsTradeDigest;
    let wasmJsFirstDifference = null;
    let wasmJsFirstDifferenceTrades = null;
    let tradeCount;
    for (let repeat = 0; repeat < repeats; repeat++) {
      const wasmStart = performance.now();
      const wasm = globalThis.engineRunColumns(meta, globalThis.__tf0Source, slice.t, slice.o, slice.h, slice.l, slice.c, slice.v);
      const wasmWallMs = performance.now() - wasmStart;
      if (!wasm) throw new Error('engineRunColumns returned no result');
      if (!wasm.ok) throw new Error(wasm.error);
      const wasmTrades = reconstruct(wasm);
      const bars = barsViewFromCols(slice);
      const contextStart = performance.now();
      const ctx = contextCursorFromColumns(buildContextColumns(bars, applyStratReleaseSemantics({ ...(jsStrategy.contextOptions || {}), tickSize: 0.1, range: { method: 'zone', ...(jsStrategy.contextOptions?.range || {}) } }, stratRelease)));
      const jsContextMs = performance.now() - contextStart;
      const jsStart = performance.now();
      const js = runBacktest(bars, { name: jsStrategy.name, params: jsStrategy.params || {}, onBar: jsStrategy.onBar }, applyStratReleaseSemantics({ ctx, startEquity: 10000, fillOn: 'close', timeframe: '5m', symbol: 'XAUUSD' }, stratRelease));
      const jsRunMs = performance.now() - jsStart;
      const [wasmDigest, jsDigest] = await Promise.all([digest(wasmTrades), digest(js.trades)]);
      if (wasmDigest !== jsDigest) {
        // Compare the same JSON value that is hashed. This removes JavaScript-only
        // distinctions such as -0 versus 0 that JSON.stringify does not preserve.
        const normalizedWasm = JSON.parse(JSON.stringify(wasmTrades.map(comparableTrade)));
        const normalizedJS = JSON.parse(JSON.stringify(js.trades.map(comparableTrade)));
        wasmJsFirstDifference ??= firstDifference(normalizedWasm, normalizedJS);
        if (!wasmJsFirstDifferenceTrades) {
          const index = normalizedWasm.findIndex((trade, tradeIndex) => firstDifference(trade, normalizedJS[tradeIndex]));
          wasmJsFirstDifferenceTrades = { index, wasm: normalizedWasm[index] ?? null, js: normalizedJS[index] ?? null };
        }
      }
      expectedDigest ??= wasmDigest;
      jsTradeDigest ??= jsDigest;
      tradeCount ??= wasmTrades.length;
      if (wasmDigest !== expectedDigest) throw new Error(`${size}: repeat output changed`);
      if (jsDigest !== jsTradeDigest) throw new Error(`${size}: JavaScript repeat output changed`);
      runs.push({ repeat: repeat + 1, wasmWallMs, wasm: wasm.timings, jsContextMs, jsRunMs, jsTotalMs: jsContextMs + jsRunMs });
    }
  const measuredCase = { size, status: 'measured', tradeCount, tradeDigest: expectedDigest, jsTradeDigest, exactWasmJsTradeEquality: expectedDigest === jsTradeDigest, wasmJsFirstDifference, wasmJsFirstDifferenceTrades, runs };
  return { timings, inputCount: count, measuredCase, memory: { performanceMemory: globalThis.performance.memory ? { jsHeapSizeLimit: performance.memory.jsHeapSizeLimit, totalJSHeapSize: performance.memory.totalJSHeapSize, usedJSHeapSize: performance.memory.usedJSHeapSize } : null } };
};

async function runBrowserCell(name, browserType, baseURL, size, onProgress = () => {}) {
  let browser;
  const completedRuns = [];
  const memorySamples = [];
  const setupTimings = [];
  let measuredCase = null;
  let version = null;
  let userAgent = null;
  try {
    browser = await browserType.launch({ headless: true });
    version = browser.version();
  } catch (error) {
    return { name, status: 'unavailable', reason: error.message };
  }
  try {
    const deadline = Date.now() + cellTimeoutMs;
    for (let repeat = 0; repeat < repeats; repeat++) {
      const page = await browser.newPage();
      try {
        page.setDefaultTimeout(30 * 60 * 1000);
        await page.goto(`${baseURL}/artifact/index.html`);
        await page.addScriptTag({ url: `${baseURL}/artifact/wasm_exec.js` });
        await page.evaluate(({ source, dataName }) => { globalThis.__tf0Source = source; globalThis.__tf0DataName = dataName; }, { source, dataName: dataPath.split('/').at(-1) });
        const remainingMs = deadline - Date.now();
        if (remainingMs <= 0) throw new Error(`cell exceeded ${cellTimeoutMs} ms timeout`);
        let timeoutId;
        const timeout = new Promise((_, reject) => {
          timeoutId = setTimeout(() => reject(new Error(`cell exceeded ${cellTimeoutMs} ms timeout`)), remainingMs);
        });
        const measured = await Promise.race([page.evaluate(browserFunction, { baseURL, size, repeats: 1, stratRelease }), timeout]).finally(() => clearTimeout(timeoutId));
        userAgent ??= await page.evaluate(() => navigator.userAgent);
        let chromiumMetrics = null;
        if (name === 'chromium') {
          const session = await page.context().newCDPSession(page);
          const metrics = await session.send('Performance.getMetrics');
          chromiumMetrics = Object.fromEntries(metrics.metrics.filter(({ name: metric }) => ['JSHeapUsedSize', 'JSHeapTotalSize'].includes(metric)).map(({ name: metric, value }) => [metric, value]));
        }
        const current = measured.measuredCase;
        measuredCase ??= { ...current, runs: [], completedRepeats: 0 };
        assert.equal(current.tradeCount, measuredCase.tradeCount, `${name} ${size}: repeat trade count changed`);
        assert.equal(current.tradeDigest, measuredCase.tradeDigest, `${name} ${size}: repeat WASM digest changed`);
        assert.equal(current.jsTradeDigest, measuredCase.jsTradeDigest, `${name} ${size}: repeat JavaScript digest changed`);
        completedRuns.push({ ...current.runs[0], repeat: repeat + 1 });
        setupTimings.push({ repeat: repeat + 1, ...measured.timings });
        memorySamples.push({ repeat: repeat + 1, ...measured.memory, chromiumCDP: chromiumMetrics });
        measuredCase = { ...measuredCase, runs: [...completedRuns], completedRepeats: completedRuns.length,
          setupTimings: [...setupTimings], memorySamples: [...memorySamples] };
        onProgress({ ...measuredCase, status: 'running' });
      } finally {
        await page.close().catch(() => {});
      }
    }
    return { status: 'measured', version, userAgent, case: { ...measuredCase, status: 'measured' } };
  } catch (error) {
    const timedOut = String(error?.message || error).includes('cell exceeded');
    return { status: timedOut ? 'timed-out' : 'failed', version, userAgent, reason: error.stack || error.message,
      case: { ...(measuredCase || { size }), runs: [...completedRuns], completedRepeats: completedRuns.length,
        setupTimings: [...setupTimings], memorySamples: [...memorySamples], status: timedOut ? 'timed-out' : 'failed', reason: error.message } };
  } finally {
    await browser.close();
  }
}

const { data, count } = parseBBT1(dataPath);
const sizes = args.sizes
  ? String(args.sizes).split(',').map((size) => size === 'full' ? count : Number(size))
  : [50_000, 200_000, count];
if (sizes.some((size) => !Number.isInteger(size) || size <= 0 || size > count)) throw new Error(`invalid --sizes for ${count}-bar input`);
if (count < 200_000) throw new Error(`real input has only ${count} bars`);
const scratch = mkdtempSync(join(tmpdir(), 'heisentick-tf0-real-'));
let server;
try {
  const artifactRoot = join(scratch, 'artifact');
  mkdirSync(artifactRoot);
  copyFileSync(wasmPath, join(artifactRoot, 'enginewasm.wasm'));
  copyFileSync(wasmExecPath, join(artifactRoot, 'wasm_exec.js'));
  writeFileSync(join(artifactRoot, 'index.html'), '<!doctype html><meta charset="utf-8"><title>T-F0 real-data benchmark</title>');
  writeFileSync(join(scratch, 'strategy.strat'), `${source}\n`);
  const nativeCases = [];
  for (const size of sizes) {
    const root = join(scratch, String(size), 'XAUUSD');
    mkdirSync(root, { recursive: true });
    writePrefix(data, count, size, join(root, '5m.bin'));
    nativeCases.push(runNative(scratch, size));
  }
  server = await serve(artifactRoot);
  const wasm = readFileSync(wasmPath);
  const nativeVersionMetadata = spawnSync('go', ['version', '-m', nativePath], { encoding: 'utf8' }).stdout
    .trim()
    .split('\n')
    .map((line, index) => index === 0 ? line.replace(nativePath, basename(nativePath)) : line)
    .join('\n');
  const browsers = requestedBrowsers.map((name) => ({ name, status: 'pending', cases: [] }));
  const evidence = {
    schema: 'enginewasm-real-data-browser-benchmark-v1',
    generatedAt: new Date().toISOString(),
    source: { repository: 'spik3r/heisentick-strat', release: stratRelease, releaseCommit, harnessPath: relative(repoRoot, scriptPath), harnessSha256: sha256(readFileSync(scriptPath)), strategySha256: sha256(source), appCommit: spawnSync('git', ['rev-parse', 'HEAD'], { cwd: appRoot, encoding: 'utf8' }).stdout.trim() },
    artifacts: { wasm: { label: basename(wasmPath), bytes: wasm.length, brotliBytes: brotliCompressSync(wasm).length, sha256: sha256(wasm) }, wasmExec: { label: basename(wasmExecPath), sha256: sha256(readFileSync(wasmExecPath)) }, native: { label: basename(nativePath), sha256: sha256(readFileSync(nativePath)), versionMetadata: nativeVersionMetadata } },
    input: { label: `XAUUSD/${basename(dataPath)}`, bytes: statSync(dataPath).size, sha256: sha256(data), count, measuredSizes: sizes },
    environment: { platform: platform(), release: release(), architecture: process.arch, cpu: cpus()[0]?.model || null, logicalCPUs: cpus().length, totalMemoryBytes: totalmem(), node: process.version, playwright: playwrightVersion },
    repeats, cellTimeoutMs,
    native: nativeCases,
    browsers,
    reliability: {},
    limitations: ['Browser heap metrics cover JavaScript/WASM page heap only where exposed; Firefox and WebKit do not expose comparable process RSS through Playwright.', `Each cell requested ${repeats} local ${repeats === 1 ? 'repeat' : 'repeats'}; completed results and timeouts are recorded per cell and do not establish production shadow reliability.`, 'The columnar bridge supports a single chart-timeframe series and this benchmark strategy has no source or higher-timeframe dependency.'],
  };
  const persistEvidence = () => {
    evidence.generatedAt = new Date().toISOString();
    evidence.source.harnessSha256 = sha256(readFileSync(scriptPath));
    const cases = browsers.flatMap((browser) => browser.cases);
    evidence.reliability = {
      attemptedCells: cases.filter(({ status }) => status !== 'pending').length,
      measuredCells: cases.filter(({ status }) => status === 'measured').length,
      failedCells: cases.filter(({ status }) => status === 'failed').length,
      timedOutCells: cases.filter(({ status }) => status === 'timed-out').length,
      unavailableBrowsers: browsers.filter(({ status }) => status === 'unavailable').length,
    };
    mkdirSync(dirname(outputPath), { recursive: true });
    writeFileSync(outputPath, `${JSON.stringify(evidence, null, 2)}\n`);
  };
  persistEvidence();
  for (const browser of browsers) {
    const browserType = playwright[browser.name];
    if (!browserType) {
      browser.status = 'unavailable';
      browser.reason = 'browser type is not provided by Playwright';
      persistEvidence();
      continue;
    }
    for (const size of sizes) {
      browser.cases.push({ size, status: 'pending' });
      persistEvidence();
      const result = await runBrowserCell(browser.name, browserType, server.baseURL, size, (partial) => {
        browser.cases[browser.cases.length - 1] = partial;
        persistEvidence();
      });
      browser.version ??= result.version;
      browser.userAgent ??= result.userAgent;
      browser.cases[browser.cases.length - 1] = result.case;
      if (result.status === 'measured') {
        const native = nativeCases.find((candidate) => candidate.size === size);
        assert.equal(result.case.tradeCount, native.tradeCount, `${browser.name} ${size}: native trade count differs`);
        assert.equal(result.case.tradeDigest, native.tradeDigest, `${browser.name} ${size}: native trade digest differs`);
        result.case.exactNativeParity = true;
      }
      persistEvidence();
    }
    browser.status = browser.cases.every(({ status }) => status === 'measured') ? 'measured' : 'incomplete';
    persistEvidence();
  }
  console.log(JSON.stringify({ output: outputPath, inputCount: count, browsers: browsers.map(({ name, status, reason }) => ({ name, status, reason })), native: nativeCases.map(({ size, tradeCount }) => ({ size, tradeCount })) }, null, 2));
} finally {
  if (server) await server.close();
  rmSync(scratch, { recursive: true, force: true });
}
