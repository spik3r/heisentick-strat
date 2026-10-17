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
// Match GitHub's unspecified Linux shell (bash -e). A step needing pipeline
// failure propagation must enable it in its own script, as the real workflow does.
function shell(script, cwd, env) {
  return command('bash', ['--noprofile', '--norc', '-e', '-c', script], cwd, env);
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
    'cmd/enginewasm/main.go': `package main
import ("encoding/json"; "os")
var ordinaryBacktestBuildTag, ordinaryBacktestBuildSourceCommit, ordinaryBacktestBuildSourceTree, ordinaryBacktestBuildSchemaSetSHA256 string
func main() { _ = json.NewEncoder(os.Stdout).Encode([]string{ordinaryBacktestBuildTag, ordinaryBacktestBuildSourceCommit, ordinaryBacktestBuildSourceTree, ordinaryBacktestBuildSchemaSetSHA256}) }
`,
    'cmd/enginewasm/main_test.go': `package main
import ("encoding/json"; "fmt"; "testing")
func TestLinkerStamp(t *testing.T) { stamp, err := json.Marshal([]string{ordinaryBacktestBuildTag, ordinaryBacktestBuildSourceCommit, ordinaryBacktestBuildSourceTree, ordinaryBacktestBuildSchemaSetSHA256}); if err != nil { t.Fatal(err) }; fmt.Println("STAMP:" + string(stamp)) }
`,
    'scripts/checks/ordinary-backtest-parity.mjs': readFileSync(new URL('./ordinary-backtest-parity.mjs', import.meta.url), 'utf8'),
    'conformance/nested/case with spaces.json': '{"invented":true}\n',
    'spec/contract.json': '{"version":1}\n',
    'examples/program.strat': 'invented fixture only\n',
  };
  for (const kind of ['error', 'inspection', 'report', 'request']) {
    files[`spec/schemas/ordinary-backtest-${kind}-v1.schema.json`] = JSON.stringify({ invented: kind }) + '\n';
  }
  for (const [name, value] of Object.entries(files)) {
    mkdirSync(path.dirname(path.join(root, name)), { recursive: true });
    writeFileSync(path.join(root, name), value);
  }
  git(root, 'init', '-q');
  git(root, 'add', '.');
  git(root, '-c', 'user.name=Release Test', '-c', 'user.email=release-test@example.invalid',
    '-c', 'commit.gpgsign=false', 'commit', '-qm', 'Invented release fixture');
  git(root, 'tag', 'v1.2.3-fixture');
  const env = { ...process.env, GITHUB_REF: 'refs/tags/v1.2.3-fixture', RUNNER_TEMP: runnerTemp, GITHUB_ENV: path.join(temp, 'github-env'),
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
    const scripts = [block(name)];
    for (const script of scripts) {
      shell(script, root, env);
      updateEnv(env);
      assert.equal(git(root, 'status', '--porcelain'), '', `source changed after ${script}`);
      assert.deepEqual(snapshot(root), before);
    }
  }
  assert.ok(env.RELEASE_DIR && !env.RELEASE_DIR.startsWith(root + path.sep));
  const schemaManifest = ['error', 'inspection', 'report', 'request'].map((kind) => {
    const name = `spec/schemas/ordinary-backtest-${kind}-v1.schema.json`;
    return `${digest(readFileSync(path.join(root, name)))}  ${name}\n`;
  }).join('');
  const expectedStamp = ['v1.2.3-fixture', revision, git(root, 'rev-parse', 'HEAD^{tree}'), digest(schemaManifest)];
  assert.deepEqual([env.ORDINARY_BACKTEST_BUILD_TAG, env.ORDINARY_BACKTEST_BUILD_SOURCE_COMMIT,
    env.ORDINARY_BACKTEST_BUILD_SOURCE_TREE, env.ORDINARY_BACKTEST_BUILD_SCHEMA_SET_SHA256], expectedStamp);
  const toolchain = command('go', ['env', 'GOROOT'], root, env).trim();
  const wasmRunner = ['misc/wasm/go_js_wasm_exec', 'lib/wasm/go_js_wasm_exec']
    .map((name) => path.join(toolchain, name)).find(existsSync);
  assert.ok(wasmRunner, 'official Go WASM runner required');
  assert.deepEqual(JSON.parse(command(wasmRunner, [path.join(env.RELEASE_DIR, 'enginewasm.wasm')], root,
    { PATH: env.PATH, GOMAXPROCS: '1' })), expectedStamp, 'actual compiled WASM must retain the complete stamp');
  const fields = ['Tag', 'SourceCommit', 'SourceTree', 'SchemaSetSHA256'];
  const flags = (prefix) => fields.map((field, i) => `-X ${prefix}.ordinaryBacktestBuild${field}=${expectedStamp[i]}`).join(' ');
  const testStamp = (prefix) => {
    const output = command('go', ['test', '-count=1', '-v', '-ldflags', flags(prefix), '-run', '^TestLinkerStamp$', './cmd/enginewasm'], root, env);
    const stamp = /^STAMP:(.+)$/m.exec(output);
    assert.ok(stamp, 'test binary must expose its complete linker identity');
    return JSON.parse(stamp[1]);
  };
  assert.deepEqual(testStamp('main'), ['', '', '', ''], 'test package is not the test binary main');
  assert.deepEqual(testStamp('release-fixture/cmd/enginewasm'), expectedStamp, 'test binary requires import-path linker symbols');
  assert.equal(git(root, 'status', '--porcelain'), '');
  assert.deepEqual(snapshot(root), before);
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

test('missing, invalid, absent and wrong-commit release tags fail before output creation', (t) => {
  const { root, env } = fixture(t);
  for (const ref of ['', 'refs/heads/main', 'refs/tags/v', 'refs/tags/v1.2.3/bad',
    `refs/tags/v1.${'2'.repeat(64)}`, 'refs/tags/v9.9.9']) {
    assert.throws(() => shell(block('Prepare release directory'), root, { ...env, GITHUB_REF: ref }), ref);
    assert.equal(readFileSync(env.GITHUB_ENV, 'utf8'), '');
    assert.deepEqual(readdirSync(env.RUNNER_TEMP), []);
  }
  writeFileSync(path.join(root, 'new-file'), 'a different commit\n');
  git(root, 'add', 'new-file');
  git(root, '-c', 'user.name=Release Test', '-c', 'user.email=release-test@example.invalid',
    '-c', 'commit.gpgsign=false', 'commit', '-qm', 'Move HEAD beyond release tag');
  assert.throws(() => shell(block('Prepare release directory'), root, env));
  assert.equal(readFileSync(env.GITHUB_ENV, 'utf8'), '');
});

test('missing or inconsistent build stamps fail before any binary is written', (t) => {
  const { root, env } = fixture(t);
  shell(block('Prepare release directory'), root, env); updateEnv(env);
  for (const key of ['ORDINARY_BACKTEST_BUILD_TAG', 'ORDINARY_BACKTEST_BUILD_SOURCE_COMMIT',
    'ORDINARY_BACKTEST_BUILD_SOURCE_TREE', 'ORDINARY_BACKTEST_BUILD_SCHEMA_SET_SHA256']) {
    for (const value of ['', 'invalid', '0'.repeat(key.endsWith('SHA256') ? 64 : 40)]) {
      assert.throws(() => shell(block('Build release binaries'), root, { ...env, [key]: value }), `${key}: ${value}`);
      assert.deepEqual(readdirSync(env.RELEASE_DIR), []);
    }
  }
  writeFileSync(path.join(root, 'dirty-after-preparation'), 'dirty');
  assert.throws(() => shell(block('Build release binaries'), root, env));
  assert.deepEqual(readdirSync(env.RELEASE_DIR), []);
});

test('release and qualification reject inherited and persisted unsupported compiler settings', (t) => {
  const { root, env } = fixture(t);
  shell(block('Prepare release directory'), root, env); updateEnv(env);
  const badSettings = { GOFLAGS: '-gcflags=all=-N', GOEXPERIMENT: 'none', GOAMD64: 'v3',
    GOWASM: 'satconv', GOCOMPILEDEBUG: 'fmahash=1', GODEBUG: 'cpu.all=off' };
  const check = (changed, label) => {
    assert.throws(() => shell(block('Build release binaries'), root, changed), label);
    assert.deepEqual(readdirSync(env.RELEASE_DIR), [], `${label}: no release binary created`);
    const out = path.join(env.RUNNER_TEMP, `qualification-${label}`); mkdirSync(out);
    assert.throws(() => command(process.execPath,
      ['scripts/checks/ordinary-backtest-parity.mjs', '--build-and-qualify', 'ci-fixture', out], root, changed), label);
    assert.equal(existsSync(path.join(out, 'enginewasm.wasm')), false, `${label}: no qualification binary created`);
  };
  for (const [key, value] of Object.entries(badSettings)) check({ ...env, [key]: value }, key);
  for (const key of ['GOFLAGS', 'GOAMD64', 'GOWASM']) {
    const config = path.join(env.RUNNER_TEMP, `goenv-${key}`);
    writeFileSync(config, `${key}=${badSettings[key]}\n`);
    const changed = { ...env, GOENV: config }; delete changed[key];
    check(changed, `persisted-${key}`);
  }
  const workspace = path.join(env.RUNNER_TEMP, 'go.work'); writeFileSync(workspace, 'go 1.22\n');
  check({ ...env, GOWORK: workspace }, 'GOWORK');
  assert.equal(git(root, 'status', '--porcelain'), '');
});

test('a missing ordinary schema fails even in a clean checkout', (t) => {
  const { root, env } = fixture(t);
  git(root, 'rm', 'spec/schemas/ordinary-backtest-error-v1.schema.json');
  git(root, '-c', 'user.name=Release Test', '-c', 'user.email=release-test@example.invalid',
    '-c', 'commit.gpgsign=false', 'commit', '-qm', 'Missing schema fixture');
  git(root, 'tag', 'v1.2.4-fixture');
  assert.throws(() => shell(block('Prepare release directory'), root,
    { ...env, GITHUB_REF: 'refs/tags/v1.2.4-fixture' }));
  assert.equal(readFileSync(env.GITHUB_ENV, 'utf8'), '');
  assert.deepEqual(readdirSync(env.RUNNER_TEMP), []);
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
  assert.doesNotMatch(block('Build release binaries'), /-buildvcs/);
  const build = block('Build release binaries');
  assert.equal((build.match(/-ldflags/g) ?? []).length, 1);
  for (const suffix of ['Tag', 'SourceCommit', 'SourceTree', 'SchemaSetSHA256']) {
    assert.equal(build.split(`-X main.ordinaryBacktestBuild${suffix}=`).length - 1, 1);
  }
  assert.match(build, /GOOS=js GOARCH=wasm go build -trimpath -ldflags "\$ordinary_ldflags" -o "\$RELEASE_DIR\/enginewasm.wasm"/);
  assert.match(block('Qualify ordinary release API'), /ordinary-backtest-parity\.mjs --qualify/);
  assert.match(block('Qualify ordinary release API'), /\$RELEASE_DIR\/enginewasm.wasm/);
  assert.match(block('Qualify ordinary release API'), /\$RELEASE_DIR\/wasm_exec.js/);
  assert.match(block('Qualify ordinary release API'), /qualification_dir="\$RUNNER_TEMP\/ordinary-release-\$GITHUB_RUN_ID-\$GITHUB_RUN_ATTEMPT"/);
  assert.match(block('Qualify ordinary release API'), /mkdir "\$qualification_dir"/);
  assert.match(steps.get('Upload ordinary release qualification diagnostics'), /\*\.instructions\.txt/);
  assert.match(steps.get('Create GitHub Release'), /overwrite_files: false\n/);
  assert.match(steps.get('Create GitHub Release'), /fail_on_unmatched_files: true\n/);
  assert.match(steps.get('Refuse an existing release'), /uses: actions\/github-script@v7/);
  assert.doesNotMatch(steps.get('Refuse an existing release'), /github-token:|permissions:|secrets\./);
  const ordered = [...steps.keys()];
  assert.equal(ordered.indexOf('Refuse an existing release') + 1, ordered.indexOf('Create GitHub Release'));
  assert.deepEqual([...steps].filter(([, value]) => /^        run:/m.test(value)).map(([name]) => name),
    [...runNames.slice(0, 3), 'Qualify ordinary release API', ...runNames.slice(3)]);
  assert.ok(ordered.indexOf('Qualify ordinary release API') < ordered.indexOf('Write checksums'));
  assert.ok(ordered.indexOf('Qualify ordinary release API') < ordered.indexOf('Refuse an existing release'));
});
