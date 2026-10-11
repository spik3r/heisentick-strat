import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawn, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { dirname } from 'node:path';
import { digest, fileSha, loadRun, validateRows } from './evidence.mjs';
import { classify, compare } from './compare.mjs';
const here=dirname(fileURLToPath(import.meta.url));
const hash='a'.repeat(64), bit='3ff0000000000000';
const error='invalid-sequential-input: bars.binding at bar 0';
const manifest={schema:'ht245.manifest.v1',cases:[{id:'good',source:'conformance/run/good'},{id:'refused',source:'conformance/run/refused',expectedError:error}]};
const trade={side:'long',entryIndex:1,exitIndex:2,reason:'sl',entry:bit,exit:bit,sl:bit,tp:bit,initialSl:bit,initialTp:bit,size:bit,points:bit,pnl:bit,pnlText:'1'};
function row(id,runtime,failed=false){return {schema:'ht245.receipt.v1',id,target:runtime.label+' '+JSON.stringify(Object.fromEntries(Object.entries(runtime).filter(([k])=>k!=='label'))),inputSha256:hash,costBits:{},tradeCount:failed?0:1,trades:failed?null:[structuredClone(trade)],...(failed?{error}:{})};}
function bundle(){
 const dir=mkdtempSync(join(tmpdir(),'ht245-test-'));
 const json=(name,x)=>writeFileSync(join(dir,name),JSON.stringify(x)+'\n');
 const sourceFiles={'harness/main.go':hash},tools={'compare.mjs':hash},inputs={'fixture':hash};
 const identity={sourceFiles,sourceSha256:digest(sourceFiles),tools,toolsSha256:digest(tools),inputs,inputFilesSha256:digest(inputs),caseInputSha256:{good:hash,refused:hash}};
 json('identity.json',identity);json('manifest.json',manifest);
 const run={schema:'ht245.run.v2',status:'complete',manifestSha256:fileSha(join(dir,'manifest.json')),identitySha256:fileSha(join(dir,'identity.json')),targets:[]};
 for(const label of ['linux-amd64v1-go1.22.12','linux-amd64v3-go1.22.12']){
   const level=label.includes('v3')?'v3':'v1';
   const runtime={go:'go1.22.12',goos:'linux',goarch:'amd64',goamd64:level,sourceSha256:identity.sourceSha256,settings:{CGO_ENABLED:'0','-trimpath':'true'}};
   json(label+'.runtime.json',runtime);writeFileSync(join(dir,label+'.jsonl'),[row('good',{label,...runtime}),row('refused',{label,...runtime},true)].map(r=>JSON.stringify(r)+'\n').join(''));
   run.targets.push({label,status:'ran',runtime,compiler:{version:'go1.22.12'},build:{GOOS:'linux',GOARCH:'amd64',GOAMD64:level,flags:[]},receiptSha256:fileSha(join(dir,label+'.jsonl')),runtimeSha256:fileSha(join(dir,label+'.runtime.json'))});
 }
 json('run.json',run);return {dir,run,identity,json};
}
test('successful pairs and expected refusals have independent denominators',()=>{
 const b=bundle(),s=compare([b.dir]);const t=Object.values(s.targets)[0];
 assert.equal(t.attemptedCases,2);assert.equal(t.successfulPairs,1);assert.equal(t.matchingRefusals,1);assert.equal(t.counts.identical,1);assert.equal(t.pnlTradesCompared,1);
});
test('equal nonempty arbitrary errors never count as identical',()=>{
 const r=row('good',{label:'x',go:'x'},true);r.error='cannot read fixture';
 assert.throws(()=>validateRows([r],{schema:manifest.schema,cases:[manifest.cases[0]]}),/unexpected execution error/);
});
test('missing, extra and duplicate case rows fail closed',()=>{
 const b=bundle(),rows=loadRun(b.dir).sets.values().next().value.rows;
 for(const broken of [rows.slice(1),[...rows,rows[0]],[rows[0],rows[0]]])assert.throws(()=>validateRows(broken,manifest));
});
test('receipt runtime mismatch and missing input identity are rejected',()=>{
 const b=bundle(),set=loadRun(b.dir).sets.values().next().value,rows=structuredClone(set.rows);
 rows[0].target=rows[0].target.replace('go1.22.12','go9.99.0');assert.throws(()=>validateRows(rows,manifest,{label:set.target.label,...set.target.runtime}),/runtime/);
 rows[0]=structuredClone(set.rows[0]);rows[0].inputSha256='';assert.throws(()=>validateRows(rows,manifest),/input digest/);
});
test('receipt, manifest and identity edits invalidate a bound run',()=>{
 for(const what of ['receipt','manifest','identity']){
   const b=bundle(),name=what==='receipt'?b.run.targets[0].label+'.jsonl':what+'.json';
   writeFileSync(join(b.dir,name),readFileSync(join(b.dir,name))+' ');
   assert.throws(()=>loadRun(b.dir),/changed since execution/);
 }
});
test('failed or unavailable target cannot use retained rows',()=>{
 const b=bundle();b.run.targets[0].status='not-run';b.json('run.json',b.run);assert.throws(()=>loadRun(b.dir),/not run/);
 b.run.status='failed';b.json('run.json',b.run);assert.throws(()=>loadRun(b.dir),/failed or incomplete/);
});
test('source, compiler and build option mismatch invalidates evidence',()=>{
 for(const change of [t=>t.compiler.version='go9',t=>t.build.GOARCH='arm64',t=>t.build.flags=['-gcflags=all=-d=fmahash=n']]){
  const b=bundle();change(b.run.targets[0]);b.json('run.json',b.run);assert.throws(()=>loadRun(b.dir));
 }
});
test('decision differences retain both observed exit indices',()=>{
 const a=row('good',{label:'x'}),b=structuredClone(a);b.trades[0].exitIndex=3;
 const result=classify(a,b);assert.equal(result.class,'decision');assert.deepEqual(result.firstDiffs[0],{trade:0,field:'exitIndex',a:2,b:3});
});
test('runner rejects a preexisting directory before attempting any build',()=>{
 const dir=mkdtempSync(join(tmpdir(),'ht245-stale-'));writeFileSync(join(dir,'old.jsonl'),'stale');
 const result=spawnSync('bash',[join(here,'run-matrix.sh'),join(here,'manifests/boundary-patches.json'),dir],{encoding:'utf8',env:{...process.env,HT245_GO:'/definitely/missing/go'}});
 assert.notEqual(result.status,0);assert.match(result.stderr,/already exists/);assert.equal(readFileSync(join(dir,'old.jsonl'),'utf8'),'stale');
});
test('failed build creates failed envelope without a publishable receipt',t=>{
 const parent=mkdtempSync(join(tmpdir(),'ht245-failed-')),dir=join(parent,'out'),fake=join(parent,'go');
 const actual=process.env.HT245_TEST_GO;
 if(!actual){t.skip('Set HT245_TEST_GO to an actual Go compiler for the build fixture');return;}
 writeFileSync(fake,`#!/usr/bin/env bash\nif [ "$1" = build ]; then echo forced-build-failure >&2; exit 7; fi\nexec '${actual}' "$@"\n`,{mode:0o755});
 const result=spawnSync('bash',[join(here,'run-matrix.sh'),join(here,'manifests/boundary-patches.json'),dir,'native'],{encoding:'utf8',env:{...process.env,HT245_GO:fake,HT245_WORK:join(parent,'work')}});
 assert.notEqual(result.status,0);assert.match(result.stderr,/forced-build-failure/);assert.throws(()=>loadRun(dir),/failed or incomplete/);
});

test('boundary generator refuses cross-run source, tool and input identity mismatches',()=>{
 for(const [field,hashField] of [['sourceFiles','sourceSha256'],['tools','toolsSha256'],['inputs','inputFilesSha256']]){
  const a=bundle(),b=bundle();
  b.identity[field].changed='b'.repeat(64);b.identity[hashField]=digest(b.identity[field]);b.json('identity.json',b.identity);b.run.identitySha256=fileSha(join(b.dir,'identity.json'));
  if(field==='sourceFiles')for(const target of b.run.targets){
    target.runtime.sourceSha256=b.identity.sourceSha256;b.json(target.label+'.runtime.json',target.runtime);target.runtimeSha256=fileSha(join(b.dir,target.label+'.runtime.json'));
    writeFileSync(join(b.dir,target.label+'.jsonl'),[row('good',{label:target.label,...target.runtime}),row('refused',{label:target.label,...target.runtime},true)].map(r=>JSON.stringify(r)+'\n').join(''));target.receiptSha256=fileSha(join(b.dir,target.label+'.jsonl'));
  }
  b.json('run.json',b.run);loadRun(b.dir);
  const result=spawnSync('node',[join(here,'gen-boundary-manifest.mjs'),join(a.dir,a.run.targets[0].label+'.jsonl'),join(b.dir,b.run.targets[0].label+'.jsonl'),join(a.dir,'manifest.json')],{encoding:'utf8'});
  assert.notEqual(result.status,0);assert.match(result.stderr,/cross-run/);
 }
});

test('concurrent invocations cannot both claim the same output directory',async t=>{
 const actual=process.env.HT245_TEST_GO;
 if(!actual){t.skip('Set HT245_TEST_GO to an actual Go compiler for the build fixture');return;}
 const parent=mkdtempSync(join(tmpdir(),'ht245-concurrent-')),dir=join(parent,'out'),fake=join(parent,'go'),marker=join(parent,'builds');
 writeFileSync(fake,`#!/usr/bin/env bash
if [ "$1" = build ]; then echo build >> '${marker}'; sleep 0.2; exit 7; fi
exec '${actual}' "$@"
`,{mode:0o755});
 const invoke=()=>new Promise(resolve=>{
  const child=spawn('bash',[join(here,'run-matrix.sh'),join(here,'manifests/boundary-patches.json'),dir,'native'],{env:{...process.env,HT245_GO:fake,HT245_WORK:join(parent,'work')},stdio:['ignore','ignore','pipe']});
  let stderr='';child.stderr.on('data',chunk=>stderr+=chunk);child.on('close',code=>resolve({code,stderr}));
 });
 const results=await Promise.all([invoke(),invoke()]);
 assert.ok(results.every(r=>r.code!==0));
 assert.equal(readFileSync(marker,'utf8').trim().split('\n').length,1,'only one invocation may reach build');
 assert.equal(results.filter(r=>/already exists|EEXIST/.test(r.stderr)).length,1);
});
