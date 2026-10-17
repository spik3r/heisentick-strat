// Actual Go native/Node-hosted Go-WASM ordinary API qualification.
// First: node scripts/checks/ordinary-backtest-parity.mjs --write-corpus INPUT
// Then run TestOrdinaryBacktestParityCorpus with INPUT/OUTPUT environment paths.
// Finally: node scripts/checks/ordinary-backtest-parity.mjs OUTPUT engine.wasm wasm_exec.js
// The ordinary requests below are NEW single-original-series requests, never
// replacements for the untouched full HTF-bearing conformance fixtures. Native
// expectations must be generated on the same inputs. No JS strategy/accounting,
// numerical tolerance, release authority, or universal cross-target claim.
import assert from 'node:assert/strict';
import { webcrypto, createHash } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { resolve } from 'node:path';

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

const args=process.argv.slice(2);
if(args[0]==='--write-corpus') {
  assert.equal(args.length,2);const cases=corpus();writeFileSync(args[1],JSON.stringify(cases),{flag:'wx',mode:0o600});console.log(JSON.stringify({ok:true,cases:cases.length}));
} else {
  assert.equal(args.length,3,'native expected corpus, wasm, official wasm_exec.js required');
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
    assert.deepEqual(actual,c.response,`${c.name}: actual native/WASM result mismatch; investigate target arithmetic, never add tolerance`);
    if(c.expectedCode)assert.equal(actual.error?.code,`BACKTEST_${c.expectedCode}`,c.name);
    else assert.equal(actual.ok,true,c.name);
    if(actual.binding){
      assert.deepEqual(actual.binding.source,digest(c.source));assert.deepEqual(actual.binding.request,digest(c.requestJSON));
      const request=JSON.parse(c.requestJSON);assert.equal(actual.requestId,request.requestId);assert.deepEqual(actual.binding.execution,request.execution);
      if(actual.ok&&c.operation==='run'){assert.equal(actual.nativeRun.case,request.requestId);assert.equal(actual.nativeRun.strategyId,request.strategyId);assert.equal(actual.equity.length,request.data.bars.length);}
    }
    actual.ok?successes++:refusals++;
  }
  for(const [name,args,code] of [['arity',[],'ARGUMENTS'],['bigint',[1n,''],'ARGUMENTS'],['symbol',[Symbol('x'),''],'ARGUMENTS'],['proxy',[new Proxy({}, {get(){throw new Error('must not coerce')}}),''],'ARGUMENTS'],['boxed',[new String('{}'),''],'ARGUMENTS'],['source-utf16',['{}','\ud800'],'SOURCE_ENCODING'],['request-utf16',['\udfff',''],'JSON_ENCODING']]){
    for(const op of ['inspect','run']){const fn=globalThis[op==='inspect'?'engineInspectBacktestV1':'engineRunBacktestReportV1'];const actual=JSON.parse(fn(...args));validate(actual,schemas[op],schemas[op]);assert.equal(actual.error.code,`BACKTEST_${code}`,name);assert.equal(actual.binding,null);refusals++;}
  }
  console.log(JSON.stringify({ok:true,cases:cases.length,successes,refusals,comparison:'exact native fixture target versus actual Node Go-WASM; no cross-target universality'}));
}
