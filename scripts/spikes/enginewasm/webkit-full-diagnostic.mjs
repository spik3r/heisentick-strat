#!/usr/bin/env node
import { createHash } from 'node:crypto';
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { basename, dirname, extname, join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { createRequire } from 'node:module';

const args = Object.fromEntries(process.argv.slice(2).map((value) => {
  const [key, ...rest] = value.replace(/^--/, '').split('=');
  return [key, rest.join('=') || true];
}));
for (const required of ['wasm', 'wasm-exec', 'data', 'output', 'release', 'release-commit']) {
  if (!args[required] || args[required] === true) throw new Error(`--${required}=<value> is required`);
}
const wasmPath = resolve(args.wasm);
const wasmExecPath = resolve(args['wasm-exec']);
const dataPath = resolve(args.data);
const outputPath = resolve(args.output);
const cellTimeoutMs = Number(args['cell-timeout-ms'] || 15 * 60 * 1000);
const playwrightPackage = process.env.ENGINEWASM_PLAYWRIGHT_PACKAGE || 'playwright';
const require = createRequire(import.meta.url);
const { webkit } = require(playwrightPackage);
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

function mimeType(path) {
  return new Map([['.html', 'text/html'], ['.js', 'text/javascript'], ['.wasm', 'application/wasm'], ['.bin', 'application/octet-stream']]).get(extname(path)) || 'application/octet-stream';
}

async function serve(root) {
  const server = createServer((request, response) => {
    const path = resolve(root, decodeURIComponent(new URL(request.url, 'http://127.0.0.1').pathname).replace(/^\//, ''));
    if (path !== root && !path.startsWith(`${root}/`)) return response.writeHead(403).end();
    try {
      response.writeHead(200, { 'content-type': mimeType(path), 'cache-control': 'no-store' }).end(readFileSync(path));
    } catch {
      response.writeHead(404).end();
    }
  });
  await new Promise((resolveServer) => server.listen(0, '127.0.0.1', resolveServer));
  return { baseURL: `http://127.0.0.1:${server.address().port}`, close: () => new Promise((resolveServer) => server.close(resolveServer)) };
}

const data = readFileSync(dataPath);
if (data.readUInt32LE(0) !== 0x31425442 || data.readUInt32LE(4) !== 1 || data.readUInt32LE(12) !== 6) throw new Error('expected BBT1 v1 six-column input');
const count = data.readUInt32LE(8);
const evidence = {
  schema: 'enginewasm-webkit-full-diagnostic-v1',
  generatedAt: new Date().toISOString(),
  status: 'pending',
  release: String(args.release),
  releaseCommit: String(args['release-commit']),
  artifacts: {
    wasm: { label: basename(wasmPath), bytes: statSync(wasmPath).size, sha256: sha256(readFileSync(wasmPath)) },
    wasmExec: { label: basename(wasmExecPath), bytes: statSync(wasmExecPath).size, sha256: sha256(readFileSync(wasmExecPath)) },
  },
  input: { label: `XAUUSD/${basename(dataPath)}`, bytes: data.length, count, sha256: sha256(data) },
  environment: { node: process.version, playwright: playwrightVersion },
  cellTimeoutMs,
  events: [],
};
const persist = () => {
  evidence.generatedAt = new Date().toISOString();
  mkdirSync(dirname(outputPath), { recursive: true });
  writeFileSync(outputPath, `${JSON.stringify(evidence, null, 2)}\n`);
};
persist();

const scratch = mkdtempSync(join(tmpdir(), 'heisentick-webkit-diagnostic-'));
let browser;
let server;
try {
  copyFileSync(wasmPath, join(scratch, 'enginewasm.wasm'));
  copyFileSync(wasmExecPath, join(scratch, 'wasm_exec.js'));
  copyFileSync(dataPath, join(scratch, '5m.bin'));
  writeFileSync(join(scratch, 'index.html'), '<!doctype html><meta charset="utf-8">');
  server = await serve(scratch);
  browser = await webkit.launch({ headless: true });
  evidence.browser = { name: 'webkit', version: browser.version() };
  const page = await browser.newPage();
  const event = (type, detail) => {
    evidence.events.push({ elapsedMs: Math.round(performance.now() - started), type, detail });
    persist();
  };
  const started = performance.now();
  page.on('console', (message) => event('console', { level: message.type(), text: message.text() }));
  page.on('pageerror', (error) => event('pageerror', { name: error.name, message: error.message, stack: error.stack }));
  page.on('crash', () => event('page-crash', {}));
  page.on('close', () => event('page-close', {}));
  await page.goto(`${server.baseURL}/index.html`);
  await page.addScriptTag({ url: `${server.baseURL}/wasm_exec.js` });
  evidence.setup = await page.evaluate(async ({ baseURL, source, count }) => {
    const marks = {};
    const mark = (name) => { marks[name] = performance.now(); };
    mark('start');
    const wasmBytes = await fetch(`${baseURL}/enginewasm.wasm`).then((response) => response.arrayBuffer());
    mark('wasmFetched');
    const module = await WebAssembly.compile(wasmBytes);
    mark('wasmCompiled');
    const go = new globalThis.Go();
    const originalExit = go.exit;
    go.exit = (code) => {
      globalThis.__tf0GoExitCode = code;
      return originalExit.call(go, code);
    };
    const instance = await WebAssembly.instantiate(module, go.importObject);
    globalThis.__tf0WasmMemory = instance.exports.mem;
    mark('wasmInstantiated');
    globalThis.__tf0GoRunState = 'pending';
    globalThis.__tf0GoRunError = null;
    go.run(instance).then(() => { globalThis.__tf0GoRunState = 'resolved'; }, (error) => {
      globalThis.__tf0GoRunState = 'rejected';
      globalThis.__tf0GoRunError = { name: error?.name, message: error?.message, stack: error?.stack };
    });
    while (typeof globalThis.engineRunColumns !== 'function') await new Promise((resolveWait) => setTimeout(resolveWait, 10));
    mark('runtimeReady');
    const dataBytes = await fetch(`${baseURL}/5m.bin`).then((response) => response.arrayBuffer());
    mark('dataFetched');
    const view = new DataView(dataBytes);
    const columns = Array.from({ length: 6 }, (_, index) => new Float64Array(dataBytes, 16 + index * count * 8, count));
    globalThis.__tf0Diagnostic = { columns, source, meta: JSON.stringify({ schema: 'enginewasm-columnar-v1', case: 't-f0-xauusd-5m', strategyId: 't-f0-xauusd-5m', symbol: 'XAUUSD', timeframe: '5m', rangeMethod: 'zone', costs: { fillOn: 'close', startEquity: 10000 } }) };
    mark('dataDecoded');
    return { marks, wasmBytes: wasmBytes.byteLength, dataBytes: dataBytes.byteLength, columnBytes: columns.reduce((sum, column) => sum + column.byteLength, 0), wasmMemoryBytes: instance.exports.mem.buffer.byteLength, userAgent: navigator.userAgent };
  }, { baseURL: server.baseURL, source, count });
  evidence.status = 'bridge-running';
  persist();
  let timeoutId;
  const timeout = new Promise((_, reject) => { timeoutId = setTimeout(() => reject(new Error(`cell exceeded ${cellTimeoutMs} ms timeout`)), cellTimeoutMs); });
  try {
    evidence.bridge = await Promise.race([
      page.evaluate(() => {
        const before = { wasmMemoryBytes: globalThis.__tf0WasmMemory.buffer.byteLength, goRunState: globalThis.__tf0GoRunState, goExitCode: globalThis.__tf0GoExitCode ?? null };
        const started = performance.now();
        try {
          const { meta, source, columns } = globalThis.__tf0Diagnostic;
          const value = globalThis.engineRunColumns(meta, source, ...columns);
          return { classification: value === undefined ? 'bridge-returned-undefined' : 'bridge-returned', elapsedMs: performance.now() - started, before, after: { wasmMemoryBytes: globalThis.__tf0WasmMemory.buffer.byteLength, goRunState: globalThis.__tf0GoRunState, goRunError: globalThis.__tf0GoRunError, goExitCode: globalThis.__tf0GoExitCode ?? null }, value: value ? { ok: value.ok, error: value.error, tradeValues: value.trades?.length, stringsBytes: value.stringsJSON?.length } : null };
        } catch (error) {
          return { classification: error instanceof WebAssembly.RuntimeError ? 'wasm-trap' : 'bridge-threw', elapsedMs: performance.now() - started, before, after: { wasmMemoryBytes: globalThis.__tf0WasmMemory?.buffer?.byteLength ?? null, goRunState: globalThis.__tf0GoRunState, goRunError: globalThis.__tf0GoRunError, goExitCode: globalThis.__tf0GoExitCode ?? null }, error: { name: error?.name, message: error?.message, stack: error?.stack } };
        }
      }),
      timeout,
    ]);
    evidence.status = evidence.bridge.classification;
  } catch (error) {
    evidence.status = error.message.includes('cell exceeded') ? 'timed-out' : 'driver-failed';
    evidence.driverError = { name: error.name, message: error.message, stack: error.stack };
  } finally {
    clearTimeout(timeoutId);
  }
  persist();
} finally {
  if (browser) await browser.close().catch(() => {});
  if (server) await server.close();
  rmSync(scratch, { recursive: true, force: true });
  persist();
}

console.log(JSON.stringify({ output: outputPath, status: evidence.status, bridge: evidence.bridge, events: evidence.events }, null, 2));
