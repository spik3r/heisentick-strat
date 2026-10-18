// Actual Go native/Node-hosted Go-WASM ordinary API qualification.
// First: node scripts/checks/ordinary-backtest-parity.mjs --write-corpus INPUT
// Then run TestOrdinaryBacktestParityCorpus with INPUT/OUTPUT environment paths.
// Finally: node scripts/checks/ordinary-backtest-parity.mjs OUTPUT engine.wasm wasm_exec.js
// The ordinary requests below are NEW single-original-series requests, never
// replacements for the untouched full HTF-bearing conformance fixtures. Native
// expectations must be generated on the same inputs. No JS strategy/accounting,
// numerical tolerance, release authority, or universal cross-target claim.
// --build-and-qualify TAG OUT builds a source-bound CI artifact outside Git.
// --qualify TAG WASM SHIM OUT qualifies existing actual release bytes. Neither
// mode creates a tag/release or grants consumer adoption authority.
import assert from 'node:assert/strict';
import { webcrypto, createHash } from 'node:crypto';
import { existsSync, readFileSync, readdirSync, realpathSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';
import { isAbsolute, relative, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = new URL('../../', import.meta.url);
const read = path => readFileSync(new URL(path, root), 'utf8');
const digest = text => ({ sha256: createHash('sha256').update(text).digest('hex'), utf8Bytes: Buffer.byteLength(text) });
const steps = { '1m':60000, '5m':300000, '15m':900000, '30m':1800000, '1h':3600000, '4h':14400000, '1d':86400000 };
function originalRequest(fixture, name, operation, bars = fixture.bars) {
  return { schema:'strat-ordinary-backtest-request-v1', requestId:name, operation, strategyId:`ordinary:${name}`,
    route:{ symbol:fixture.symbol, timeframe:fixture.timeframe },
    execution:{ rangeMethod:fixture.rangeMethod, costs:{...fixture.costs, slippageBps:fixture.costs.slippageBps ?? 0}, params:{}, forceRoute:false, chartType:'candles' },
    reportingBasis:'ordinary-logical-completed-close-mtm-v1',
    data:{ schema:'strat-original-bars-v1', profile:'original-chart-bars-v1', bars, firstT:bars[0][0], lastT:bars.at(-1)[0], closedThroughT:bars.at(-1)[0]+steps[fixture.timeframe],
      origin:{ datasetId:`qualification-new-primary:${name}`, historyGeneration:0, provenance:'caller-declared' } } };
}
function corpus() {
  const cases=[];
  const add=(name,operation,request,source,code=null)=>cases.push({ name, operation, requestJSON:typeof request==='string'?request:JSON.stringify(request), source, expectedCode:code });
  for (const stem of ['sar-named-level-allow','family-price-momentum-atr-stop','family-donchian-breakout']) {
    const fixture=JSON.parse(read(`conformance/run/${stem}.fixture.json`));
    const source=read(`conformance/run/${stem}.strat`);
    for (const [suffix,text] of [['original',source],['edited-risk',source.replace('risk 200 USD','risk 137 USD')]]) {
      assert(suffix==='original'||text!==source);
      for (const operation of ['inspect','run']) {
        const name=`${stem}.${suffix}.${operation}`;
        add(name,operation,originalRequest(fixture,name,operation),text);
      }
    }
  }
  const fixture=JSON.parse(read('conformance/run/sar-named-level-allow.fixture.json'));
  const source=read('conformance/run/sar-named-level-allow.strat');
  const req=(name,operation='run')=>originalRequest(fixture,name,operation);
  let r=req('text-only','inspect');r.data=null;add('text-only','inspect',r,source);
  r=originalRequest(fixture,'zero-trades','run',fixture.bars.slice(0,1));add('zero-trades','run',r,source);
  for (const [name,mutate,code] of [
    ['off-route',x=>x.route.symbol='NO-SUCH-SYMBOL','ROUTE_UNSUPPORTED'],
    ['invalid-cost',x=>x.execution.costs.startEquity=0,'SCHEMA'],
    ['invalid-geometry',x=>x.data.bars[0][2]=-1,'DATA_INVALID'],
    ['duplicate-time',x=>x.data.bars[1][0]=x.data.bars[0][0],'DATA_INVALID'],
    ['forming',x=>x.data.closedThroughT=x.data.lastT,'FORMING_BAR'],
    ['no-data',x=>x.data=null,'DATA_REQUIRED'],
    ['unknown-option',x=>x.execution.partialStart=1,'UNKNOWN_FIELD'],
    ['force-route',x=>x.execution.forceRoute=true,'SCHEMA'],
    ['companion-series',x=>x.data.htfBars=[],'UNKNOWN_FIELD'],
  ]) { r=structuredClone(req(name));mutate(r);add(name,'run',r,source,code); }
  for(const [name,stem,code] of [
    ['uncovered-daily','family-daily-flush-failure','ACCOUNTING_UNAVAILABLE'],
    ['uncovered-weekend','deployed-dsl-weekend-extreme-fade-btcusdt-four-hour','ACCOUNTING_UNAVAILABLE'],
    ['uncovered-intrahour','family-intra-hour-run-exhaustion','ACCOUNTING_UNAVAILABLE'],
    ['uncovered-shock','family-down-shock-rebound','ACCOUNTING_UNAVAILABLE'],
    ['dedicated-clock','family-clock-range-breakout-ordinary-long','DEDICATED_REQUIRED'],
  ]) {
    const text=read(`conformance/run/${stem}.strat`);
    for(const operation of ['inspect','run']) {r=req(`${name}.${operation}`,operation);add(`${name}.${operation}`,operation,r,text,code);}
  }
  add('parse-rejected','run',req('parse-rejected'),'dsl v999\n','PARSE_REJECTED');
  add('duplicate-json','run',JSON.stringify(req('duplicate-json')).replace('"operation":"run"','"operation":"run","operation":"run"'),source,'DUPLICATE_KEY');
  add('source-over-limit','run',req('source-over-limit'),' '.repeat(262145),'INPUT_LIMIT');
  add('source-exact-limit','inspect',req('source-exact-limit','inspect'),source+' '.repeat(262144-Buffer.byteLength(source)));
  r=req('bar-limit');r.data.bars=Array.from({length:10001},(_,i)=>[i*14400000,100,101,99,100,1]);r.data.firstT=0;r.data.lastT=r.data.bars.at(-1)[0];r.data.closedThroughT=r.data.lastT+14400000;add('bar-limit','run',r,source,'DATA_LIMIT');
  r=req('exact-max-bars');r.data.bars=Array.from({length:10000},(_,i)=>[1704067200000+i*14400000,100,101,99,100,1]);r.data.firstT=r.data.bars[0][0];r.data.lastT=r.data.bars.at(-1)[0];r.data.closedThroughT=r.data.lastT+14400000;add('exact-max-bars','run',r,source);
  r=req('partial-finalization');r.execution.costs.feePerUnit=.13;
  add('partial-finalization','run',r,source.replace('target 0.5R','target 100R').replace('risk 200 USD','risk 137 USD')+'\nmanagement { partial 70% at 0.25R move stop to breakeven }\n');
  r=originalRequest(fixture,'pending-last-bar','run',fixture.bars.slice(0,11));r.execution.costs.fillOn='nextOpen';add('pending-last-bar','run',r,source);
  add('source-multibyte-limit','run',req('source-multibyte-limit'),'😀'.repeat(65537),'INPUT_LIMIT');
  add('escaped-lone-surrogate','run',JSON.stringify(req('escaped-lone-surrogate')).replace('qualification-new-primary:escaped-lone-surrogate','\\ud800'),source,'JSON_ENCODING');
  add('valid-astral-source','run',req('valid-astral-source'),source.replace('Named Level Sweep Synthetic','Arbitrary 😀 source'));
  return cases;
}

function qualify(args) {
  const build = args[0] === '--build-and-qualify';
  assert.equal(args.length, build ? 3 : 5, 'qualification tag, artifacts and external output directory required');
  const tag = args[1], out = realpathSync(args.at(-1)), repo = realpathSync(fileURLToPath(root));
  assert.match(tag, /^[A-Za-z0-9][A-Za-z0-9.+_-]{0,63}$/, 'bounded qualification tag required');
  const rel = relative(repo, out);
  assert.ok(rel.startsWith('..' + sep) || isAbsolute(rel), 'qualification output must be outside the checkout');
  assert.deepEqual(readdirSync(out), [], 'qualification output directory must be empty');
  const run = (exe, argv, env = process.env) => {
    try {
      return execFileSync(exe, argv, { cwd: repo, env, encoding: 'utf8',
        stdio: ['ignore', 'pipe', 'pipe'], maxBuffer: 32 << 20 });
    } catch (error) {
      writeFileSync(resolve(out, 'failure.json'), JSON.stringify({ executable: exe, arguments: argv,
        status: error.status, signal: error.signal, stdout: error.stdout, stderr: error.stderr }, null, 2) + '\n', { flag: 'wx' });
      throw error;
    }
  };
  const git = (...argv) => run('git', argv).trim();
  assert.equal(git('status', '--porcelain'), '', 'qualification requires a clean source checkout');
  const commit = git('rev-parse', 'HEAD'), tree = git('rev-parse', 'HEAD^{tree}');
  const schemaManifest = ['error', 'inspection', 'report', 'request'].map(kind => {
    const name = `spec/schemas/ordinary-backtest-${kind}-v1.schema.json`;
    return `${digest(readFileSync(new URL(name, root))).sha256}  ${name}\n`;
  }).join('');
  const schemaSetSha256 = digest(schemaManifest).sha256;
  const goEnv = JSON.parse(run('go', ['env', '-json', 'GOROOT', 'GOVERSION', 'GOOS', 'GOARCH',
    'GOHOSTOS', 'GOHOSTARCH', 'GOAMD64', 'GOFLAGS', 'GOEXPERIMENT', 'GOWASM', 'GOWORK']));
  assert.equal(goEnv.GOVERSION, 'go1.22.12', 'reviewed compiler required');
  assert.equal(goEnv.GOOS, 'linux'); assert.equal(goEnv.GOHOSTOS, 'linux');
  assert.equal(goEnv.GOARCH, goEnv.GOHOSTARCH);
  assert.equal(goEnv.GOARCH, { x64: 'amd64', arm64: 'arm64' }[process.arch]);
  for (const key of ['GOFLAGS', 'GOEXPERIMENT', 'GOWASM']) assert.equal(goEnv[key], '', `unsupported effective ${key}`);
  for (const key of ['GOCOMPILEDEBUG', 'GODEBUG', 'GOARM64', 'GOFLAGS', 'GOEXPERIMENT', 'GOWASM']) assert.ok(!process.env[key], `unsupported ${key}`);
  assert.ok(['', 'v1'].includes(goEnv.GOAMD64));
  assert.ok([undefined, '', 'v1'].includes(process.env.GOAMD64));
  assert.ok(['', 'off'].includes(goEnv.GOWORK), 'unreviewed Go workspace');
  assert.equal(run('go', ['env', 'GOAMD64'], { ...process.env, GOARCH: 'amd64' }).trim(), 'v1');
  assert.equal(run('go', ['env', 'GOWASM'], { ...process.env, GOOS: 'js', GOARCH: 'wasm' }).trim(), '');
  // After rejecting unsupported effective/persisted settings, use exactly the
  // same explicit compiler configuration for artifact, tests and proof builds.
  const buildEnv = { ...process.env, GOROOT: goEnv.GOROOT, GOENV: 'off', GOWORK: 'off', GOAMD64: 'v1',
    GOTOOLCHAIN: 'local', CGO_ENABLED: '0' };
  const official = ['misc/wasm/wasm_exec.js', 'lib/wasm/wasm_exec.js']
    .map(name => resolve(goEnv.GOROOT, name)).find(existsSync);
  const wasmRunner = ['misc/wasm/go_js_wasm_exec', 'lib/wasm/go_js_wasm_exec']
    .map(name => resolve(goEnv.GOROOT, name)).find(existsSync);
  assert.ok(official && wasmRunner, 'official Go WASM shim and test runner required');
  const wasm = build ? resolve(out, 'enginewasm.wasm') : resolve(args[2]);
  const shim = build ? resolve(out, 'wasm_exec.js') : resolve(args[3]);
  const expectedProducer = { repo: 'spik3r/heisentick-strat', tag, sourceCommit: commit,
    sourceTree: tree, target: 'js/wasm', goVersion: goEnv.GOVERSION, schemaSetSha256 };
  const ldflags = `-X main.ordinaryBacktestBuildTag=${tag} -X main.ordinaryBacktestBuildSourceCommit=${commit} -X main.ordinaryBacktestBuildSourceTree=${tree} -X main.ordinaryBacktestBuildSchemaSetSHA256=${schemaSetSha256}`;
  if (build) {
    run('go', ['build', '-trimpath', '-ldflags', ldflags, '-o', wasm, './cmd/enginewasm'],
      { ...buildEnv, GOOS: 'js', GOARCH: 'wasm' });
    writeFileSync(shim, readFileSync(official), { flag: 'wx' });
  }
  assert.deepEqual(readFileSync(shim), readFileSync(official), 'qualification requires the same-toolchain official shim');
  const input = resolve(out, 'input.json'), native = resolve(out, 'native.json'), wasmCorpus = resolve(out, 'wasm.json');
  writeFileSync(input, JSON.stringify(corpus()), { flag: 'wx', mode: 0o600 });
  // A test binary has its own main; package-under-test globals retain their
  // import-path symbol. Production enginewasm uses the four main.* symbols.
  const testLdflags = ldflags.replaceAll('main.ordinaryBacktestBuild',
    'github.com/spik3r/heisentick-strat/cmd/enginewasm.ordinaryBacktestBuild');
  const testArgs = ['test', '-trimpath', '-count=1', '-ldflags', testLdflags, '-run', '^TestOrdinaryBacktestParityCorpus$', './cmd/enginewasm'];
  // Go's WASM startup has a bounded argv/environment region. Preserve build
  // controls and task-local caches without forwarding unrelated runner state.
  const inherited = ['PATH', 'HOME', 'GOROOT', 'TMPDIR', 'GOCACHE', 'GOTMPDIR', 'GOMODCACHE',
    'GOTOOLCHAIN', 'GOWORK', 'GOPROXY', 'GOSUMDB', 'GOFLAGS', 'GOENV', 'GOEXPERIMENT',
    'GOCOMPILEDEBUG', 'GODEBUG', 'GOAMD64', 'GOWASM', 'GOARM64'];
  const testEnv = { ...Object.fromEntries(inherited.filter(key => buildEnv[key] !== undefined)
    .map(key => [key, buildEnv[key]])), CGO_ENABLED: '0', GOMAXPROCS: '1', ORDINARY_BACKTEST_PARITY_INPUT: input };
  writeFileSync(resolve(out, 'native.log'), run('go', testArgs, { ...testEnv, ORDINARY_BACKTEST_PARITY_OUTPUT: native }), { flag: 'wx' });
  writeFileSync(resolve(out, 'wasm.log'), run('go', [...testArgs.slice(0, 1), '-exec', wasmRunner, ...testArgs.slice(1)],
    { ...testEnv, GOOS: 'js', GOARCH: 'wasm', ORDINARY_BACKTEST_PARITY_OUTPUT: wasmCorpus }), { flag: 'wx' });
  const publicParity = JSON.parse(run(process.execPath, [fileURLToPath(import.meta.url), wasmCorpus, wasm, shim,
    '--expected-producer', JSON.stringify(expectedProducer)]));
  // Full native and WASM envelopes are retained unchanged. The independent
  // exact replay rejects every unpredicted difference, including identity.
  const arithmetic = JSON.parse(run('python3', ['scripts/checks/ordinary-backtest-arithmetic-proof.py',
    native, wasmCorpus, input, `${goEnv.GOOS}-${goEnv.GOARCH}`, out], buildEnv));
  assert.equal(git('status', '--porcelain'), '', 'qualification modified the source checkout');
  assert.equal(git('rev-parse', 'HEAD'), commit);
  assert.equal(git('rev-parse', 'HEAD^{tree}'), tree);
  const receipt = { schema: 'ordinary-build-qualification-v1', ok: true, producer: expectedProducer,
    nativeTarget: `${goEnv.GOOS}/${goEnv.GOARCH}`, publicComparison: 'complete same-target Go-WASM test/public-export equality',
    engineWasmSha256: digest(readFileSync(wasm)).sha256, wasmExecSha256: digest(readFileSync(shim)).sha256,
    publicParity, arithmetic };
  writeFileSync(resolve(out, 'qualification.json'), JSON.stringify(receipt, null, 2) + '\n', { flag: 'wx' });
  console.log(JSON.stringify(receipt));
}

const args=process.argv.slice(2);
if (args[0] === '--qualify' || args[0] === '--build-and-qualify') {
  qualify(args);
} else if(args[0]==='--write-corpus') {
  assert.equal(args.length,2);const cases=corpus();writeFileSync(args[1],JSON.stringify(cases),{flag:'wx',mode:0o600});console.log(JSON.stringify({ok:true,cases:cases.length}));
} else {
  assert.ok(args.length === 3 || (args.length === 5 && args[3] === '--expected-producer'),
    'same-target expected corpus, wasm, official wasm_exec.js and optional complete producer required');
  const expectedProducer = args.length === 5 ? JSON.parse(args[4]) : null;
  const cases=JSON.parse(readFileSync(args[0],'utf8'));
  // Closed-schema test validator. Unknown keywords fail; additionalProperties
  // schemas and propertyNames are applied rather than silently ignored.
  const known=new Set(['$schema','$id','$comment','$defs','$ref','type','properties','required','additionalProperties','propertyNames','items','minItems','maxItems','uniqueItems','minLength','maxLength','minimum','exclusiveMinimum','maximum','pattern','enum','const','oneOf','anyOf','allOf','if','then','else','not','maxProperties']);
  function validate(value,rule,base) {
    for(const k of Object.keys(rule))assert(known.has(k),`unknown schema keyword ${k}`);
    if(rule.$ref) {let target=base;assert(rule.$ref.startsWith('#/'));for(const p of rule.$ref.slice(2).split('/'))target=target[p.replaceAll('~1','/').replaceAll('~0','~')];validate(value,target,base);}
    const passes=x=>{try{validate(value,x,base);return true;}catch{return false;}};
    if(rule.oneOf)assert.equal(rule.oneOf.filter(passes).length,1,'oneOf');
    if(rule.anyOf)assert(rule.anyOf.some(passes),'anyOf');
    for(const x of rule.allOf??[])validate(value,x,base);
    if(rule.not)assert(!passes(rule.not),'not');
    if(rule.if){const next=passes(rule.if)?rule.then:rule.else;if(next)validate(value,next,base);}
    if(Object.hasOwn(rule,'const'))assert.deepEqual(value,rule.const);
    if(rule.enum)assert(rule.enum.includes(value));
    if(rule.type) {
      const types=Array.isArray(rule.type)?rule.type:[rule.type];
      assert(types.some(t=>t==='null'?value===null:t==='array'?Array.isArray(value):t==='object'?value!==null&&typeof value==='object'&&!Array.isArray(value):t==='integer'?Number.isSafeInteger(value):t==='number'?typeof value==='number'&&Number.isFinite(value):typeof value===t),`type ${types}`);
    }
    if(value!==null&&typeof value==='object'&&!Array.isArray(value)) {
      for(const k of rule.required??[])assert(Object.hasOwn(value,k),`required ${k}`);
      if(rule.maxProperties!==undefined)assert(Object.keys(value).length<=rule.maxProperties);
      for(const [k,v] of Object.entries(value)){
        if(rule.propertyNames)validate(k,rule.propertyNames,base);
        if(Object.hasOwn(rule.properties??{},k))validate(v,rule.properties[k],base);
        else {assert(rule.additionalProperties!==false,`unknown field ${k}`);if(rule.additionalProperties&&typeof rule.additionalProperties==='object')validate(v,rule.additionalProperties,base);}
      }
    }
    if(Array.isArray(value)){
      if(rule.minItems!==undefined)assert(value.length>=rule.minItems);if(rule.maxItems!==undefined)assert(value.length<=rule.maxItems);
      if(rule.uniqueItems)assert.equal(new Set(value.map(x=>JSON.stringify(x))).size,value.length);
      if(rule.items)for(const x of value)validate(x,rule.items,base);
    }
    if(typeof value==='string'){
      if(rule.minLength!==undefined)assert([...value].length>=rule.minLength);if(rule.maxLength!==undefined)assert([...value].length<=rule.maxLength);if(rule.pattern)assert(new RegExp(rule.pattern).test(value));
    }
    if(typeof value==='number'){
      if(rule.minimum!==undefined)assert(value>=rule.minimum);if(rule.maximum!==undefined)assert(value<=rule.maximum);if(rule.exclusiveMinimum!==undefined)assert(value>rule.exclusiveMinimum);
    }
  }
  const schemas=Object.fromEntries(['inspect','run'].map(op=>[op,JSON.parse(read(`spec/schemas/ordinary-backtest-${op==='inspect'?'inspection':'report'}-v1.schema.json`))]));
  globalThis.crypto??=webcrypto;createRequire(import.meta.url)(resolve(args[2]));
  const go=new globalThis.Go();go.env={...go.env,GOMAXPROCS:'1'};
  const {instance}=await WebAssembly.instantiate(readFileSync(args[1]),go.importObject);void go.run(instance);
  assert.equal(typeof globalThis.engineRunFixture,'function','legacy export preserved');
  let successes=0,refusals=0;
  for(const c of cases){
    const fn=globalThis[c.operation==='inspect'?'engineInspectBacktestV1':'engineRunBacktestReportV1'];assert.equal(typeof fn,'function');
    const raw=fn(c.requestJSON,c.source);assert.equal(typeof raw,'string');assert(Buffer.byteLength(raw)<=8388608);
    const actual=JSON.parse(raw);validate(actual,schemas[c.operation],schemas[c.operation]);
    assert.deepEqual(actual,c.response,`${c.name}: actual Go reference/public-WASM result mismatch; investigate target arithmetic, never add tolerance`);
    if(c.expectedCode)assert.equal(actual.error?.code,`BACKTEST_${c.expectedCode}`,c.name);
    else assert.equal(actual.ok,true,c.name);
    if(actual.binding){
      if (expectedProducer) assert.deepEqual(actual.binding.producer, expectedProducer, `${c.name}: complete producer identity`);
      assert.deepEqual(actual.binding.source,digest(c.source));assert.deepEqual(actual.binding.request,digest(c.requestJSON));
      const request=JSON.parse(c.requestJSON);assert.equal(actual.requestId,request.requestId);assert.deepEqual(actual.binding.execution,request.execution);
      if(actual.ok&&c.operation==='run'){assert.equal(actual.nativeRun.case,request.requestId);assert.equal(actual.nativeRun.strategyId,request.strategyId);assert.equal(actual.equity.length,request.data.bars.length);}
    }
    actual.ok?successes++:refusals++;
  }
  for(const [name,args,code] of [['arity',[],'ARGUMENTS'],['bigint',[1n,''],'ARGUMENTS'],['symbol',[Symbol('x'),''],'ARGUMENTS'],['proxy',[new Proxy({}, {get(){throw new Error('must not coerce')}}),''],'ARGUMENTS'],['boxed',[new String('{}'),''],'ARGUMENTS'],['source-utf16',['{}','\ud800'],'SOURCE_ENCODING'],['request-utf16',['\udfff',''],'JSON_ENCODING']]){
    for(const op of ['inspect','run']){const fn=globalThis[op==='inspect'?'engineInspectBacktestV1':'engineRunBacktestReportV1'];const actual=JSON.parse(fn(...args));validate(actual,schemas[op],schemas[op]);assert.equal(actual.error.code,`BACKTEST_${code}`,name);assert.equal(actual.binding,null);refusals++;}
  }
  console.log(JSON.stringify({ok:true,cases:cases.length,successes,refusals,comparison:'exact complete expected Go target versus actual Node Go-WASM; no cross-target universality'}));
}
