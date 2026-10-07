// Per-architecture legacy compatibility. Both binaries use the same Go toolchain.
// Usage: node scripts/checks/master-portable-legacy-parity.mjs <candidate> <baseline38613eb8>
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { gunzipSync } from 'node:zlib';
const [candidate,baseline,...extra]=process.argv.slice(2);
assert(candidate && baseline && !extra.length,'candidate and pinned baseline binaries required');
const digest=(bytes)=>createHash('sha256').update(bytes).digest('hex');
const raw=readFileSync(new URL('../../testsupport/testdata/master-portable-corpus-v1.json',import.meta.url));
assert(raw.length<=2*1024*1024);assert.equal(digest(raw),'1dc35945d0822756ba1b2be70182858eff7c80f24ac315c23d6cda4cd8107b32');
const corpus=JSON.parse(raw),assets=new Map();
assert.equal(corpus.cases.length,38);assert.equal(Object.keys(corpus.assets).length,11);
for(const [key,asset] of Object.entries(corpus.assets)) {
 assert.equal(asset.encoding,'gzip-base64-btb1');assert(asset.payload.length<=6400024);
 assert(asset.rows>0 && asset.rows<=100000 && asset.byteLength===16+asset.rows*48);
 const bytes=gunzipSync(Buffer.from(asset.payload,'base64'),{maxOutputLength:4800016});
 assert.equal(bytes.length,asset.byteLength);assert.equal(digest(bytes),key);assert.equal(asset.sha256,key);assets.set(key,bytes);
}
const scratch=mkdtempSync(join(tmpdir(),'master-legacy-parity-'));
const sourcePath=join(scratch,'invented.strat'),dataPath=join(scratch,'invented.btb1');
const evidence={schema:'master-portable-legacy-preservation-v1',baselineCommit:'38613eb8f4e8dd8afa5e68660d2f7b83937c64fb',corpusSha256:digest(raw),comparisons:[]};
const directions=new Map();let failures=0;
function compare(label,command,meta,source,bytes) {
 writeFileSync(sourcePath,source);writeFileSync(dataPath,bytes);
 const stamp=(ms)=>new Date(ms).toISOString().replace('.000Z','Z');
 const args=[command,`--dsl-file=${sourcePath}`,`--m5-file=${dataPath}`,`--warmup-from=${stamp(meta.warmupFromT)}`,`--trade-from=${stamp(meta.tradeFromT)}`,`--trade-to=${stamp(meta.tradeToT)}`,`--spread=${meta.spread}`];
 const options={encoding:'utf8',maxBuffer:128*1024*1024,timeout:30000};
 const before=execFileSync(resolve(baseline),args,options),after=execFileSync(resolve(candidate),args,options);
 const same=before===after;if(!same)failures++;
 const document=JSON.parse(after),run=document.run;
 assert.equal(document.dslSha256,digest(source));assert.equal(document.dataSha256,digest(bytes));
 assert.equal(run.arithmeticContract,undefined);assert.equal(document.arithmetic,undefined);
 const key=`${command}/${run.mode}/spread-${meta.spread}`;
 if(!directions.has(key))directions.set(key,new Set());
 for(const trade of run.trades)directions.get(key).add(trade.signal.direction);
 evidence.comparisons.push({label,sourceSha256:digest(source),dataSha256:digest(bytes),configSha256:document.configSha256,baselineSha256:digest(before),candidateSha256:digest(after),byteIdentical:same,trades:run.trades.length});
 return run;
}
try {
 for(const item of corpus.cases) {
  assert.equal(digest(item.source),item.sourceSha256);assert(assets.has(item.dataSha256));
  compare(`master/${item.label}`,'master-report',item.metadata,item.source,assets.get(item.dataSha256));
 }
 // Use fixed baseline/reflected/gap inputs and both v9 policies/costs. The
 // selected windows are frozen corpus metadata, never results from this build.
 for(const mode of ['source-like-v1','audit-baseline-v1']) for(const spread of [0,1]) for(const suffix of ['', '/reflected','/gaps-partial']) {
  const item=corpus.cases.find((c)=>c.label===`SOURCE_HISTORICAL_REFERENCE/spread-${spread}${suffix}`);
  const source=`dsl v7\nstrategy "Invented legacy v9 control" { description "Frozen synthetic bytes only" }\nmarket { regime timeframe M30 from M5 }\nsetup { type: regime engine\nregime profile v9-floor-half-v1\nregime mode ${mode}\n}\n`;
  const run=compare(`v9/${mode}/spread-${spread}${suffix}`,'regime-report',item.metadata,source,assets.get(item.dataSha256));
  assert(run.trades.length>0,`v9 ${mode} vacuous control`);
  if(suffix==='/gaps-partial')assert(run.indicators.some((row)=>!row.complete));
 }
 assert.equal(directions.size,8);
 for(const [key,sides]of directions)assert(sides.has(1)&&sides.has(-1),`${key}: both directions required`);
 console.log(JSON.stringify({...evidence,failures,status:failures?'FAIL':'PASS'},null,2));
 assert.equal(failures,0,'legacy successful bytes changed; no normalization or new baseline allowed');
} finally {rmSync(scratch,{recursive:true,force:true});}
