// Offline release-workflow regression: execute only packaging shell blocks in a
// disposable invented Go repository. Never run an action or call a release API.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { test } from 'node:test';

const workflowPath = process.env.RELEASE_WORKFLOW_PATH ?? new URL('../../.github/workflows/release.yml', import.meta.url);
const workflow = readFileSync(workflowPath, 'utf8');
const assets = ['heisentick-linux-arm64', 'heisentick-linux-amd64', 'heisentick-darwin-arm64',
  'dslwasm.wasm', 'enginewasm.wasm', 'wasm_exec.js', 'conformance.tar.gz', 'SHA256SUMS'];
const buildAssets = assets.slice(0, 5);
const targets = [['linux', 'arm64', 'heisentick'], ['linux', 'amd64', 'heisentick'],
  ['darwin', 'arm64', 'heisentick'], ['js', 'wasm', 'dslwasm'], ['js', 'wasm', 'enginewasm']];
const runNames = ['Prepare release directory', 'Build release binaries', 'Copy wasm_exec.js',
  'Build conformance archive', 'Write checksums'];

// Deliberately narrow extraction, not a general YAML parser. Fail when the
// checked workflow changes shape rather than silently exercising another script.
const steps = new Map([...workflow.matchAll(/^      - name: (.+)\n([\s\S]*?)(?=^      - name: |$(?![\s\S]))/gm)]
  .map((match) => [match[1], match[2]]));
function block(name, key = 'run', indent = 8) {
  const step = steps.get(name);
  assert.ok(step, `missing workflow step: ${name}`);
  const lines = step.split('\n');
  const prefix = ' '.repeat(indent) + key + ': ';
  const start = lines.findIndex((line) => line.startsWith(prefix));
  assert.ok(start >= 0, `missing ${key} in ${name}`);
  const value = lines[start].slice(prefix.length);
  if (value !== '|') return value;
  const content = [];
  for (const line of lines.slice(start + 1)) {
    if (line && !line.startsWith(' '.repeat(indent + 2))) break;
    content.push(line.slice(indent + 2));
  }
  return content.join('\n').trimEnd();
}
function command(exe, args, cwd, env = process.env) {
  return execFileSync(exe, args, { cwd, env, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
}
function shell(script, cwd, env) {
  return command('bash', ['--noprofile', '--norc', '-e', '-o', 'pipefail', '-c', script], cwd, env);
}
function git(root, ...args) { return command('git', args, root).trim(); }
function digest(bytes) { return createHash('sha256').update(bytes).digest('hex'); }
function wasmModuleInfo(file) {
  const bytes = readFileSync(file);
  const startMarker = Buffer.from('3077af0c9274080241e1c107e6d618e6', 'hex');
  const endMarker = Buffer.from('f932433186182072008242104116d8f2', 'hex');
  const start = bytes.indexOf(startMarker);
  const end = bytes.indexOf(endMarker);
  assert.ok(start >= 0 && end > start + startMarker.length, 'missing WASM module info');
  assert.equal(bytes.indexOf(startMarker, start + 1), -1, 'duplicate WASM module info');
  assert.equal(bytes.indexOf(endMarker, end + 1), -1, 'duplicate WASM module info');
  return bytes.subarray(start + startMarker.length, end).toString('utf8');
}
function snapshot(root) {
  return Object.fromEntries(git(root, 'ls-files', '-z').split('\0').filter(Boolean)
    .map((name) => [name, digest(readFileSync(path.join(root, name)))]));
}
function fixture(t) {
  const temp = mkdtempSync(path.join(tmpdir(), 'release-packaging-'));
  t.after(() => rmSync(temp, { recursive: true, force: true }));
  const root = path.join(temp, 'source with spaces');
  const runnerTemp = path.join(temp, 'runner temp');
  mkdirSync(root); mkdirSync(runnerTemp);
  const files = {
    'go.mod': 'module release-fixture\n\ngo 1.22\n',
    'cmd/heisentick/main.go': 'package main\nfunc main() {}\n',
    'cmd/dslwasm/main.go': 'package main\nfunc main() {}\n',
    'cmd/enginewasm/main.go': 'package main\nfunc main() {}\n',
    'conformance/nested/case with spaces.json': '{"invented":true}\n',
    'spec/contract.json': '{"version":1}\n',
    'examples/program.strat': 'invented fixture only\n',
  };
  for (const [name, value] of Object.entries(files)) {
    mkdirSync(path.dirname(path.join(root, name)), { recursive: true });
    writeFileSync(path.join(root, name), value);
  }
  git(root, 'init', '-q');
  git(root, 'add', '.');
  git(root, '-c', 'user.name=Release Test', '-c', 'user.email=release-test@example.invalid',
    '-c', 'commit.gpgsign=false', 'commit', '-qm', 'Invented release fixture');
  const env = { ...process.env, RUNNER_TEMP: runnerTemp, GITHUB_ENV: path.join(temp, 'github-env'),
    CGO_ENABLED: '0', GOTOOLCHAIN: 'local', GOPROXY: 'off', GOSUMDB: 'off', GOFLAGS: '' };
  writeFileSync(env.GITHUB_ENV, '');
  return { root, env, files };
}
function updateEnv(env) {
  for (const line of readFileSync(env.GITHUB_ENV, 'utf8').split('\n').filter(Boolean)) {
    const at = line.indexOf('=');
    assert.ok(at > 0);
    env[line.slice(0, at)] = line.slice(at + 1);
  }
}

// Real Go compilation on tiny invented inputs reproduces the original dirty
// second-build regression without strategy execution or network access.
test('packaging keeps every build clean and preserves all eight assets', (t) => {
  const { root, env } = fixture(t);
  const revision = git(root, 'rev-parse', 'HEAD');
  const before = snapshot(root);
  for (const name of runNames) {
    // Baseline mode intentionally reaches its old builds, exposing contamination.
    if (name === 'Prepare release directory' && !steps.has(name)) continue;
    const scripts = name === 'Build release binaries' ? block(name).split('\n') : [block(name)];
    for (const script of scripts) {
      shell(script, root, env);
      updateEnv(env);
      assert.equal(git(root, 'status', '--porcelain'), '', `source changed after ${script}`);
      assert.deepEqual(snapshot(root), before);
    }
  }
  assert.ok(env.RELEASE_DIR && !env.RELEASE_DIR.startsWith(root + path.sep));
  assert.deepEqual(readdirSync(env.RELEASE_DIR).sort(), [...assets].sort());
  for (const [index, name] of buildAssets.entries()) {
    const file = path.join(env.RELEASE_DIR, name);
    // Go 1.22's go version -m does not support WASM containers; inspect the
    // unique embedded runtime/debug module-info settings for those two files.
    const info = index < 3 ? command('go', ['version', '-m', file], root, env)
      : wasmModuleInfo(file);
    for (const setting of [`vcs.revision=${revision}`, 'vcs.modified=false',
      'CGO_ENABLED=0', '-trimpath=true', `GOOS=${targets[index][0]}`, `GOARCH=${targets[index][1]}`]) {
      assert.equal(info.split(`build\t${setting}\n`).length - 1, 1, `${name}: ${setting}`);
    }
    assert.equal(info.includes('build\tvcs.modified=true\n'), false, name);
    assert.equal(info.split(`path\trelease-fixture/cmd/${targets[index][2]}\n`).length - 1, 1, name);
    const keys = [...info.matchAll(/build\t([^=\n]+)=/g)].map((match) => match[1]);
    assert.equal(new Set(keys).size, keys.length, `${name}: duplicate build settings`);
  }
  const sums = readFileSync(path.join(env.RELEASE_DIR, 'SHA256SUMS'), 'utf8').trim().split('\n');
  assert.deepEqual(sums.map((line) => line.slice(66)), assets.slice(0, 7));
  for (const line of sums) {
    const name = line.slice(66);
    assert.equal(line.slice(0, 64), digest(readFileSync(path.join(env.RELEASE_DIR, name))));
  }
  shell('sha256sum --check SHA256SUMS', env.RELEASE_DIR, env);
  const archive = path.join(env.RELEASE_DIR, 'conformance.tar.gz');
  const members = command('tar', ['tzf', archive], root, env).trim().split('\n');
  const expected = Object.keys(before).filter((name) => /^(conformance|spec|examples)\//.test(name));
  assert.deepEqual(members.filter((name) => !name.endsWith('/')).sort(), expected.sort());
  for (const name of expected) {
    const bytes = execFileSync('tar', ['xOzf', archive, name]);
    assert.equal(digest(bytes), before[name], name);
  }
  const goroot = command('go', ['env', 'GOROOT'], root, env).trim();
  const shim = ['misc/wasm/wasm_exec.js', 'lib/wasm/wasm_exec.js']
    .map((name) => path.join(goroot, name)).find(existsSync);
  assert.ok(shim);
  assert.deepEqual(readFileSync(path.join(env.RELEASE_DIR, 'wasm_exec.js')), readFileSync(shim));
  const uploadPaths = block('Create GitHub Release', 'files', 10).split('\n')
    .map((line) => line.replace('${{ env.RELEASE_DIR }}', env.RELEASE_DIR));
  assert.deepEqual(uploadPaths, assets.map((name) => path.join(env.RELEASE_DIR, name)));
});

test('dirty source and an in-tree output directory fail closed', (t) => {
  const { root, env } = fixture(t);
  writeFileSync(path.join(root, 'untracked'), 'dirty');
  assert.throws(() => shell(block('Prepare release directory'), root, env));
  assert.equal(readFileSync(env.GITHUB_ENV, 'utf8'), '');
  rmSync(path.join(root, 'untracked'));
  assert.throws(() => shell(block('Prepare release directory'), root, { ...env, RUNNER_TEMP: root }));
  assert.equal(readFileSync(env.GITHUB_ENV, 'utf8'), '');
});

test('release guard admits only an absent release, including draft pagination', async () => {
  const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
  const guard = new AsyncFunction('github', 'context', block('Refuse an existing release', 'script', 10));
  const context = { repo: { owner: 'fixture', repo: 'fixture' }, ref: 'refs/tags/v1.2.3/test' };
  const missing = () => Object.assign(new Error('not found'), { status: 404 });
  async function run(response, pages = [[]]) {
    let lookupCalls = 0;
    let listingCalls = 0;
    let pagesSeen = 0;
    const listReleases = () => { throw new Error('must use pagination'); };
    const github = {
      rest: { repos: { listReleases, getReleaseByTag: async (args) => {
        lookupCalls++;
        assert.deepEqual(args, { ...context.repo, tag: 'v1.2.3/test' });
        if (response instanceof Error) throw response;
        return response;
      } } },
      paginate: { iterator: async function* (endpoint, args) {
        listingCalls++;
        assert.equal(endpoint, listReleases);
        assert.deepEqual(args, { ...context.repo, per_page: 100 });
        for (const data of pages) {
          pagesSeen++;
          if (data instanceof Error) throw data;
          yield { data };
        }
      } },
    };
    try {
      await guard(github, context);
      assert.equal(pagesSeen, pages.length, 'every absent-release page must be checked');
    } finally {
      assert.equal(lookupCalls, 1);
      assert.equal(listingCalls, response.status === 404 ? 1 : 0);
    }
  }
  await assert.rejects(run({ data: { id: 1, draft: false } }), /already exists/);
  await run(missing());
  await run(missing(), [[{ tag_name: 'v0.1.0' }], [{ tag_name: 'v0.2.0', draft: true }]]);
  for (const draft of [true, false]) {
    await assert.rejects(run(missing(), [[{ tag_name: 'v0.1.0' }],
      [{ tag_name: 'v1.2.3/test', draft }]]), /already exists/);
  }
  for (const status of [401, 403, 429, 500, '404', undefined]) {
    const error = Object.assign(new Error('lookup failed'), { status });
    await assert.rejects(run(error), (seen) => seen === error);
    await assert.rejects(run(missing(), [[{ tag_name: 'v0.1.0' }], error]), (seen) => seen === error);
  }
  for (const page of [null, {}, [{ id: 1 }], [null], [{ tag_name: 123 }]]) {
    await assert.rejects(run(missing(), [page]), /Malformed release listing/);
  }
});

test('release contract preserves permissions, runner, toolchain and strict uploads', () => {
  assert.match(workflow, /on:\n  push:\n    tags:\n      - "v\*"\n/);
  assert.match(workflow, /permissions:\n  contents: write\n/);
  assert.match(workflow, /concurrency:\n  group: release-\$\{\{ github.ref \}\}\n  cancel-in-progress: false\n/);
  assert.ok(workflow.includes('runs-on: ${{ fromJSON(vars.CI_RUNNER_MODE == \'self-hosted\' && \'["self-hosted","linux"]\' || \'["ubuntu-24.04"]\') }}'));
  assert.match(workflow, /go-version: "1\.22\.x"/);
  assert.match(workflow, /CGO_ENABLED: "0"/);
  assert.doesNotMatch(block('Build release binaries'), /-buildvcs|-ldflags/);
  assert.match(steps.get('Create GitHub Release'), /overwrite_files: false\n/);
  assert.match(steps.get('Create GitHub Release'), /fail_on_unmatched_files: true\n/);
  assert.match(steps.get('Refuse an existing release'), /uses: actions\/github-script@v7/);
  assert.doesNotMatch(steps.get('Refuse an existing release'), /github-token:|permissions:|secrets\./);
  const ordered = [...steps.keys()];
  assert.equal(ordered.indexOf('Refuse an existing release') + 1, ordered.indexOf('Create GitHub Release'));
  assert.deepEqual([...steps].filter(([, value]) => /^        run:/m.test(value)).map(([name]) => name), runNames);
});
