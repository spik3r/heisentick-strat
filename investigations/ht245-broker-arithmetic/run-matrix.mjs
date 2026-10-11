#!/usr/bin/env node
// Build and execute only explicitly requested targets. Each output directory is immutable.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, realpathSync, renameSync, writeFileSync } from 'node:fs';
import { basename, dirname, extname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { arch, platform, release } from 'node:os';
import { createHash } from 'node:crypto';
import { digest, fileSha, manifestCases, readRows, validateRows } from './evidence.mjs';
const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, '../..');
const [manifestArg, outArg, ...requested] = process.argv.slice(2);
assert.ok(manifestArg && outArg, 'usage: run-matrix.sh manifest.json NEW-output-dir [native|amd64v1|amd64v3|arm64|arm64nofma|wasmjs|wasip1 ...]');
const out = resolve(outArg), go = process.env.HT245_GO || 'go';
assert.ok(out !== root && !out.startsWith(root + '/'), 'evidence output must be outside source checkout');
assert.ok(!existsSync(out), 'output directory already exists; use a fresh path (stale evidence rejected)');
const manifestRaw = readFileSync(resolve(manifestArg)), manifest = JSON.parse(manifestRaw);
manifestCases(manifest);
const hostOS = { linux: 'linux', darwin: 'darwin' }[platform()], hostArch = { x64: 'amd64', arm64: 'arm64' }[arch()];
assert.ok(hostOS && hostArch, 'unsupported host');
const aliases = requested.length ? requested : ['native', 'wasmjs'];
assert.equal(new Set(aliases).size, aliases.length, 'duplicate target selection');
const work = resolve(process.env.HT245_WORK || join(out, 'work'));
assert.ok(work !== root && !work.startsWith(root + '/'), 'build/cache outputs must be outside source checkout');
mkdirSync(dirname(out), { recursive: true });
mkdirSync(out); // Exclusive atomic ownership: a concurrent invocation must fail.
mkdirSync(work, { recursive: true });
const buildDir = mkdtempSync(join(work, 'build-'));
const baseEnv = { ...process.env, CGO_ENABLED: '0', GOTOOLCHAIN: 'local', GOPROXY: 'off', GOWORK: 'off', GOFLAGS: '', GOEXPERIMENT: '', GOCOMPILEDEBUG: '', GODEBUG: '', GOWASM: '', GOARM64: '', GOCACHE: join(work, 'gocache'), GOTMPDIR: join(work, 'tmp'), GOTELEMETRY: 'off' };
mkdirSync(baseEnv.GOCACHE, { recursive: true }); mkdirSync(baseEnv.GOTMPDIR, { recursive: true });
function exec(command, args, env = baseEnv) {
  const r = spawnSync(command, args, { cwd: root, env, encoding: 'utf8', maxBuffer: 128 * 1024 * 1024 });
  assert.equal(r.status, 0, `${command} ${args.join(' ')} failed: ${r.error || r.stderr || r.signal}`);
  return r.stdout;
}
const writeJSON = (name, value) => { const path = join(out, name); writeFileSync(path + '.tmp', JSON.stringify(value, null, 2) + '\n'); renameSync(path + '.tmp', path); };
const run = { schema: 'ht245.run.v2', status: 'incomplete', startedAt: new Date().toISOString(), host: { os: platform(), arch: arch(), release: release() }, targets: [] };
writeJSON('run.json', run);
try {
  const changed = exec('git', ['diff', 'HEAD', '--name-only']).trim().split('\n').filter(Boolean);
  assert.ok(changed.every(p => p.startsWith('investigations/ht245-broker-arithmetic/') || p === 'scripts/checks/sequential-arithmetic-schedules.json'), 'production tracked source is dirty');
  const pinnedPath = 'scripts/checks/sequential-arithmetic-schedules.json';
  const oldPin = JSON.parse(exec('git', ['show', `HEAD:${pinnedPath}`])), pin = JSON.parse(readFileSync(join(root, pinnedPath)));
  const { sources: oldSources, ...oldRest } = oldPin, { sources: newSources, ...newRest } = pin;
  assert.deepEqual(oldRest, newRest, 'arithmetic schedule/build/closure change is out of scope');
  for (const [p, value] of Object.entries(oldSources)) assert.equal(newSources[p], value, 'existing source pin changed');
  const sourceFiles = {}, suffixes = new Set(['.go','.s','.S','.syso','.c','.h','.cc','.cpp','.cxx','.m','.f','.f90','.swig','.swigcxx','.o','.a']);
  function scan(dir) {
    for (const entry of readdirSync(dir, { withFileTypes: true }).sort((a,b) => a.name.localeCompare(b.name))) {
      if (entry.name.startsWith('.')) continue;
      const p = join(dir, entry.name);
      if (entry.isDirectory()) scan(p);
      else if (!entry.name.endsWith('_test.go') && (suffixes.has(extname(entry.name)) || ['go.mod','go.sum','go.work','go.work.sum'].includes(entry.name))) {
        assert.ok(realpathSync(p).startsWith(root + '/'), 'source symlink escapes checkout');
        sourceFiles[relative(root,p)] = fileSha(p);
      }
    }
  }
  scan(root);
  for (const [p, value] of Object.entries(oldSources)) if (!p.startsWith('investigations/ht245-broker-arithmetic/')) assert.equal(sourceFiles[p], value, `production source changed: ${p}`);
  const known = new Set([...Object.keys(oldSources), ...['harness','micro','probe'].map(x => `investigations/ht245-broker-arithmetic/${x}/main.go`)]);
  assert.deepEqual(new Set(Object.keys(sourceFiles)), known, 'unknown compiler input');
  const tools = {};
  for (const name of readdirSync(here).sort()) if (/\.(mjs|sh)$/.test(name)) tools[name] = fileSha(join(here,name));
  const inputs = {};
  for (const c of manifest.cases) for (const suffix of ['.fixture.json','.strat']) inputs[c.source+suffix] = fileSha(join(root,c.source+suffix));
  const caseInputSha256 = {};
  for (const c of manifest.cases) {
    const h=createHash('sha256').update(readFileSync(join(root,c.source+'.fixture.json'))).update('\0').update(readFileSync(join(root,c.source+'.strat')));
    if(c.costs || c.scale !== undefined || c.patches?.length) {
      const C=c.costs ? Object.fromEntries(['feePerUnit','slippage','slippageBps','startEquity'].filter(k=>c.costs[k]!==undefined).map(k=>[k,c.costs[k]])) : null;
      const P=c.patches?.length ? c.patches.map(p=>Object.fromEntries(['index','o','h','l','c'].filter(k=>p[k]!==undefined).map(k=>[k,p[k]]))) : null;
      h.update('\0').update(JSON.stringify({C,S:c.scale??null,P}));
    }
    caseInputSha256[c.id]=h.digest('hex');
  }
  const identity = { caseInputSha256, schema: 'ht245.identity.v2', head: exec('git',['rev-parse','HEAD']).trim(), workingChanges: changed, sourceFiles, sourceSha256: digest(sourceFiles), tools, toolsSha256: digest(tools), inputs, inputFilesSha256: digest(inputs) };
  writeJSON('identity.json', identity); writeFileSync(join(out,'manifest.json'), manifestRaw);
  run.identitySha256 = fileSha(join(out,'identity.json')); run.manifestSha256 = fileSha(join(out,'manifest.json'));
  const compilerEnv = JSON.parse(exec(go,['env','-json','GOVERSION','GOROOT','GOTOOLDIR','GOHOSTOS','GOHOSTARCH']));
  assert.equal(compilerEnv.GOHOSTOS, hostOS, 'compiler host OS mismatch'); assert.equal(compilerEnv.GOHOSTARCH, hostArch, 'compiler host architecture mismatch');
  const compiler = { version: compilerEnv.GOVERSION, goSha256: fileSha(join(compilerEnv.GOROOT,'bin/go')), compilerSha256: fileSha(join(compilerEnv.GOTOOLDIR,'compile')), hostOS, hostArch };
  for (const alias of aliases) {
    const native = hostArch === 'arm64' ? 'arm64' : 'amd64v1';
    const kind = alias === 'native' ? native : alias;
    assert.ok(['arm64','arm64nofma','amd64v1','amd64v3','wasmjs','wasip1'].includes(kind), `unknown target ${alias}`);
    const goos = kind === 'wasmjs' ? 'js' : kind === 'wasip1' ? 'wasip1' : hostOS;
    const goarch = kind.startsWith('wasm') || kind === 'wasip1' ? 'wasm' : kind.startsWith('arm64') ? 'arm64' : 'amd64';
    const label = `${goos}-${kind}-${compiler.version}`;
    assert.ok(!run.targets.some(t => t.label === label), 'target aliases resolve to duplicate target');
    const build = { GOOS: goos, GOARCH: goarch, GOAMD64: kind === 'amd64v3' ? 'v3' : 'v1', CGO_ENABLED: '0', GOEXPERIMENT: '', GOFLAGS: '', flags: ['-buildvcs=false', '-trimpath', '-ldflags=-X main.buildSourceSHA=' + identity.sourceSha256, ...(kind === 'arm64nofma' ? ['-gcflags=all=-d=fmahash=n'] : [])] };
    const target = { label, alias, status: 'not-run', compiler, build }; run.targets.push(target); writeJSON('run.json',run);
    try {
      assert.ok(goarch === 'wasm' || goarch === hostArch || (hostOS === 'darwin' && hostArch === 'arm64' && goarch === 'amd64'), 'requested target cannot execute on this host');
      const bin = join(buildDir,label+(goarch === 'wasm' ? '.wasm' : '.bin'));
      const env = { ...baseEnv, GOOS: goos, GOARCH: goarch, GOAMD64: build.GOAMD64 };
      exec(go, ['build', ...build.flags, '-o', bin, './investigations/ht245-broker-arithmetic/harness'], env);
      target.binarySha256 = fileSha(bin); target.goVersionM = exec(go,['version','-m',bin]);
      let command = bin, args = [], runtimeRoot = root, runtimeManifest = resolve(manifestArg);
      if (kind === 'wasmjs') {
        const glue = ['lib/wasm','misc/wasm'].map(p => join(compilerEnv.GOROOT,p,'go_js_wasm_exec')).find(existsSync);
        assert.ok(glue, 'Go JS WASM glue unavailable'); command = glue; args = [bin];
        target.runner = { kind: 'node-go-js-wasm', version: exec('node',['--version']).trim(), glueSha256: fileSha(glue), implementationSha256: fileSha(join(dirname(glue),'wasm_exec.js')) };
      } else if (kind === 'wasip1') {
        command = 'wasmtime'; args = ['run',`--dir=${root}::/w`,`--dir=${out}::/e`,bin]; runtimeRoot='/w'; runtimeManifest='/e/manifest.json';
        target.runner = { kind: 'wasmtime', version: exec(command,['--version']).trim() };
      } else if (hostArch !== goarch) {
        command = 'arch'; args = ['-x86_64',bin]; target.runner = { kind: 'rosetta', host: run.host };
      } else target.runner = { kind: 'native', host: run.host };
      const runtimeRaw = exec(command,[...args,'-identity'],env), runtime = JSON.parse(runtimeRaw);
      assert.equal(runtime.sourceSha256, identity.sourceSha256, 'executed binary source mismatch');
      assert.equal(runtime.go, compiler.version, 'executed compiler mismatch');
      assert.equal(runtime.goos, goos, 'executed OS mismatch'); assert.equal(runtime.goarch, goarch, 'executed architecture mismatch');
      const raw = exec(command,[...args,'-root',runtimeRoot,'-manifest',runtimeManifest,'-label',label],env);
      const receiptFile = join(out,`${label}.jsonl`);
      writeFileSync(receiptFile+'.tmp',raw);
      validateRows(readRows(receiptFile+'.tmp'),manifest,{label,...runtime},caseInputSha256);
      renameSync(receiptFile+'.tmp',receiptFile);
      writeFileSync(join(out,`${label}.runtime.json`),runtimeRaw);
      Object.assign(target,{status:'ran',runtime,runtimeSha256:fileSha(join(out,`${label}.runtime.json`)),receiptSha256:fileSha(receiptFile),attempts:manifest.cases.length,successful:manifest.cases.filter(c=>!c.expectedError).length,refusals:manifest.cases.filter(c=>c.expectedError).length});
      console.log(`ran ${label}: ${target.attempts} attempts, ${target.successful} successes, ${target.refusals} expected refusals`);
    } catch (error) { target.reason=error.message; console.error(`not-run ${label}: ${error.message}`); }
    writeJSON('run.json',run);
  }
  assert.ok(run.targets.every(t=>t.status==='ran'), 'one or more required targets failed');
  // Rehash every consumed input and source after execution to catch concurrent edits.
  for (const [p,value] of Object.entries({...sourceFiles,...inputs})) assert.equal(fileSha(join(root,p)),value,`source/input changed during execution: ${p}`);
  for (const [p,value] of Object.entries(tools)) assert.equal(fileSha(join(here,p)),value,`tool changed during execution: ${p}`);
  assert.equal(fileSha(resolve(manifestArg)),run.manifestSha256,'manifest changed during execution');
  scan(root); assert.equal(digest(sourceFiles),identity.sourceSha256,'compiler inputs changed during execution');
  run.status='complete'; run.finishedAt=new Date().toISOString(); writeJSON('run.json',run);
} catch (error) { run.status='failed'; run.error=error.message; writeJSON('run.json',run); console.error(error.message); process.exitCode=1; }
