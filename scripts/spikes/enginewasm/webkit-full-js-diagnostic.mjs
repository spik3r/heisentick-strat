#!/usr/bin/env node
import { createHash } from 'node:crypto';
import { mkdirSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { basename, dirname, extname, resolve } from 'node:path';
import { createRequire } from 'node:module';
import { spawnSync } from 'node:child_process';

const args = Object.fromEntries(process.argv.slice(2).map((value) => {
  const [key, ...rest] = value.replace(/^--/, '').split('=');
  return [key, rest.join('=') || true];
}));
for (const required of ['data', 'app', 'output', 'release', 'release-commit']) {
  if (!args[required] || args[required] === true) throw new Error(`--${required}=<value> is required`);
}
const dataPath = resolve(args.data);
const appRoot = resolve(args.app);
const outputPath = resolve(args.output);
const phaseTimeoutMs = Number(args['phase-timeout-ms'] || 2 * 60 * 1000);
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

const sha256 = (value) => createHash('sha256').update(value).digest('hex');
const mimeType = (path) => new Map([['.html', 'text/html'], ['.js', 'text/javascript'], ['.bin', 'application/octet-stream']]).get(extname(path)) || 'application/octet-stream';

async function serve() {
  const roots = new Map([['app', appRoot], ['data', dirname(dataPath)]]);
  const server = createServer((request, response) => {
    const parts = decodeURIComponent(new URL(request.url, 'http://127.0.0.1').pathname).split('/').filter(Boolean);
    if (parts.length === 1 && parts[0] === 'index.html') return response.writeHead(200, { 'content-type': 'text/html' }).end('<!doctype html><meta charset="utf-8">');
    const root = roots.get(parts.shift());
    if (!root) return response.writeHead(404).end();
    const path = resolve(root, parts.join('/'));
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

async function bounded(page, name, expression) {
  let timeoutId;
  const timeout = new Promise((_, reject) => {
    timeoutId = setTimeout(() => reject(new Error(`${name} exceeded ${phaseTimeoutMs} ms timeout`)), phaseTimeoutMs);
  });
  try {
    return { status: 'measured', ...(await Promise.race([page.evaluate(expression), timeout])) };
  } catch (error) {
    return { status: error.message.includes('exceeded') ? 'timed-out' : 'failed', error: { name: error.name, message: error.message } };
  } finally {
    clearTimeout(timeoutId);
  }
}

const data = readFileSync(dataPath);
if (data.readUInt32LE(0) !== 0x31425442 || data.readUInt32LE(4) !== 1 || data.readUInt32LE(12) !== 6) throw new Error('expected BBT1 v1 six-column input');
const count = data.readUInt32LE(8);
const appCommit = spawnSync('git', ['rev-parse', 'HEAD'], { cwd: appRoot, encoding: 'utf8' }).stdout.trim();
const appDiff = spawnSync('git', ['diff', '--binary', '--', 'engine/dsl/setups/openingRangeBreakout.js', 'test/engine.test.mjs'], { cwd: appRoot, encoding: 'utf8' }).stdout;
const evidence = {
  schema: 'enginewasm-webkit-full-js-diagnostic-v1', generatedAt: new Date().toISOString(), status: 'pending',
  release: String(args.release), releaseCommit: String(args['release-commit']), appCommit, appWorkingTreeDiffSha256: appDiff ? sha256(appDiff) : null,
  input: { label: `XAUUSD/${basename(dataPath)}`, bytes: statSync(dataPath).size, count, sha256: sha256(data) },
  environment: { node: process.version, playwright: playwrightVersion }, phaseTimeoutMs, phases: {}, events: [],
};
const persist = () => { evidence.generatedAt = new Date().toISOString(); mkdirSync(dirname(outputPath), { recursive: true }); writeFileSync(outputPath, `${JSON.stringify(evidence, null, 2)}\n`); };
persist();

let browser;
let server;
try {
  server = await serve();
  browser = await webkit.launch({ headless: true });
  evidence.browser = { name: 'webkit', version: browser.version() };
  const page = await browser.newPage();
  const started = performance.now();
  const record = (type, detail) => { evidence.events.push({ elapsedMs: Math.round(performance.now() - started), type, detail }); persist(); };
  page.on('console', (message) => record('console', { level: message.type(), text: message.text() }));
  page.on('pageerror', (error) => record('pageerror', { name: error.name, message: error.message, stack: error.stack }));
  page.on('crash', () => record('page-crash', {}));
  await page.goto(`${server.baseURL}/index.html`);
  evidence.phases.setup = await bounded(page, 'setup', async () => {
    const started = performance.now();
    const [{ barsViewFromCols }, engine, { createSpecStrategy }, { applyStratReleaseSemantics }] = await Promise.all([
      import(`${location.origin}/app/engine/chartDataView.js`), import(`${location.origin}/app/engine/engine.js`), import(`${location.origin}/app/engine/dsl/specStrategy.js`), import(`${location.origin}/app/engine/stratReleaseSemantics.js`),
    ]);
    const dataBytes = await fetch(`${location.origin}/data/5m.bin`).then((response) => response.arrayBuffer());
    const view = new DataView(dataBytes);
    const count = view.getUint32(8, true);
    const columns = Array.from({ length: 6 }, (_, index) => new Float64Array(dataBytes, 16 + index * count * 8, count));
    globalThis.__tf0JS = { barsViewFromCols, engine, createSpecStrategy, applyStratReleaseSemantics, columns };
    return { elapsedMs: performance.now() - started, dataBytes: dataBytes.byteLength, columnBytes: columns.reduce((sum, column) => sum + column.byteLength, 0) };
  });
  persist();
  if (evidence.phases.setup.status === 'measured') {
    await page.evaluate(({ source, release }) => { globalThis.__tf0JSSource = source; globalThis.__tf0JSRelease = release; }, { source, release: evidence.release });
    evidence.phases.context = await bounded(page, 'context', () => {
      const started = performance.now();
      const { barsViewFromCols, engine, createSpecStrategy, applyStratReleaseSemantics, columns } = globalThis.__tf0JS;
      const strategy = createSpecStrategy(globalThis.__tf0JSSource);
      const bars = barsViewFromCols({ t: columns[0], o: columns[1], h: columns[2], l: columns[3], c: columns[4], v: columns[5] });
      const options = applyStratReleaseSemantics({ ...(strategy.contextOptions || {}), tickSize: 0.1, range: { method: 'zone', ...(strategy.contextOptions?.range || {}) } }, globalThis.__tf0JSRelease);
      const ctx = engine.contextCursorFromColumns(engine.buildContextColumns(bars, options));
      globalThis.__tf0JSRun = { bars, strategy, ctx };
      return { elapsedMs: performance.now() - started, bars: bars.length };
    });
    persist();
  }
  if (evidence.phases.context?.status === 'measured') {
    evidence.phases.engine = await bounded(page, 'engine', () => {
      const started = performance.now();
      const { engine, applyStratReleaseSemantics } = globalThis.__tf0JS;
      const { bars, strategy, ctx } = globalThis.__tf0JSRun;
      const result = engine.runBacktest(bars, { name: strategy.name, params: strategy.params || {}, onBar: strategy.onBar }, applyStratReleaseSemantics({ ctx, startEquity: 10000, fillOn: 'close', timeframe: '5m', symbol: 'XAUUSD' }, globalThis.__tf0JSRelease));
      globalThis.__tf0JSTrades = result.trades;
      return { elapsedMs: performance.now() - started, tradeCount: result.trades.length };
    });
    persist();
  }
  if (evidence.phases.engine?.status === 'measured') {
    evidence.phases.normalization = await bounded(page, 'normalization', () => {
      const started = performance.now();
      const json = JSON.stringify(globalThis.__tf0JSTrades);
      return { elapsedMs: performance.now() - started, bytes: new TextEncoder().encode(json).byteLength };
    });
    persist();
  }
  evidence.status = Object.values(evidence.phases).every((phase) => phase.status === 'measured') ? 'measured' : 'incomplete';
  persist();
} finally {
  if (browser) await browser.close().catch(() => {});
  if (server) await server.close();
  persist();
}

console.log(JSON.stringify({ output: outputPath, status: evidence.status, phases: evidence.phases }, null, 2));
