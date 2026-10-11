#!/usr/bin/env node
// Exact comparison of 15-significant-digit serialized public output, not raw arithmetic.
import assert from 'node:assert/strict';
import { writeFileSync, renameSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { resolve } from 'node:path';
import { loadRun, fileSha, tradeFields, decisionFields } from './evidence.mjs';
const view=new DataView(new ArrayBuffer(8));
const f64=h=>{view.setBigUint64(0,BigInt('0x'+h));return view.getFloat64(0);};
const ordered=h=>{const n=BigInt('0x'+h);return n>>63n?-(n&0x7fffffffffffffffn):n;};
export function classify(a,b) {
  assert.equal(a.inputSha256,b.inputSha256,'input identity differs');
  assert.deepEqual(a.costBits,b.costBits,'normalized costs differ');
  if (a.error || b.error) { assert.ok(a.error && a.error===b.error,'refusal differs'); return {class:'matching-refusal',error:a.error}; }
  if(a.tradeCount!==b.tradeCount)return {class:'decision',firstDiffs:[{field:'tradeCount',a:a.tradeCount,b:b.tradeCount}]};
  let cls='identical'; const diffs=[];
  for(let i=0;i<a.tradeCount;i++) {
    const x=a.trades[i],y=b.trades[i];
    for(const k of decisionFields)if(x[k]!==y[k]){cls='decision';diffs.push({trade:i,field:k,a:x[k]??null,b:y[k]??null});}
    for(const k of tradeFields)if(x[k]!==y[k]){if(cls!=='decision')cls='path';diffs.push({trade:i,field:k,a:x[k],b:y[k]});}
    if(x.pnl!==y.pnl){if(cls==='identical')cls='pnl-only';const d=ordered(x.pnl)-ordered(y.pnl);diffs.push({trade:i,field:'pnl',a:x.pnl,b:y.pnl,aText:x.pnlText,bText:y.pnlText,ulps:String(d<0n?-d:d),signChange:Math.sign(f64(x.pnl))!==Math.sign(f64(y.pnl)),positiveLabelChange:(f64(x.pnl)>0)!==(f64(y.pnl)>0)});}
  }
  return {class:cls,firstDiffs:diffs};
}
export function compare(dirs,refLabel) {
  const bundles=dirs.map(loadRun),first=bundles[0],sets=new Map();
  for(const b of bundles){
    assert.equal(b.run.manifestSha256,first.run.manifestSha256,'cross-run manifests differ');
    assert.equal(b.identity.sourceSha256,first.identity.sourceSha256,'cross-run source differs');
    assert.equal(b.identity.toolsSha256,first.identity.toolsSha256,'cross-run harness/comparator differs');
    assert.equal(b.identity.inputFilesSha256,first.identity.inputFilesSha256,'cross-run fixture/source inputs differ');
    for(const [label,set]of b.sets){assert.ok(!sets.has(label),`duplicate cross-run target ${label}`);sets.set(label,set);}
  }
  refLabel ||= sets.keys().next().value; assert.ok(sets.has(refLabel),'reference missing');
  const ref=sets.get(refLabel).rows;
  const summary={schema:'ht245.compare.v2',observation:'15-significant-digit serialized public trade output',reference:refLabel,sourceSha256:first.identity.sourceSha256,manifestSha256:first.run.manifestSha256,comparatorSha256:fileSha(fileURLToPath(import.meta.url)),evidence:bundles.map(b=>({runSha256:b.runSha256,identitySha256:b.run.identitySha256,targets:b.run.targets.map(t=>({label:t.label,receiptSha256:t.receiptSha256,runtimeSha256:t.runtimeSha256,binarySha256:t.binarySha256,compiler:t.compiler,build:t.build,runner:t.runner}))})),targets:{}};
  for(const[label,set]of sets){if(label===refLabel)continue;
    const byId=new Map(set.rows.map(r=>[r.id,r]));
    const t={attemptedCases:ref.length,successfulPairs:0,matchingRefusals:0,counts:{},differingCases:[],refusals:[],pnlTradesCompared:0,pnlTradesDiffering:0,maxUlps:'0',signChanges:0,positiveLabelChanges:0};
    for(const a of ref){const b=byId.get(a.id),r=classify(a,b);t.counts[r.class]=(t.counts[r.class]||0)+1;
      if(r.class==='matching-refusal'){t.matchingRefusals++;t.refusals.push({id:a.id,error:r.error});continue;}
      t.successfulPairs++;t.pnlTradesCompared+=Math.min(a.tradeCount,b.tradeCount);
      for(const d of r.firstDiffs||[])if(d.field==='pnl'){t.pnlTradesDiffering++;if(BigInt(d.ulps)>BigInt(t.maxUlps))t.maxUlps=d.ulps;if(d.signChange)t.signChanges++;if(d.positiveLabelChange)t.positiveLabelChanges++;}
      if(r.class!=='identical')t.differingCases.push({id:a.id,...r});
    }
    summary.targets[label]=t;
  }
  assert.ok(Object.keys(summary.targets).length,'comparison needs at least two executed targets');
  return summary;
}
if(process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url)) {
  try { const args=process.argv.slice(2),dirs=[];let ref,json;
    for(let i=0;i<args.length;i++) {if(args[i]==='--ref')ref=args[++i];else if(args[i]==='--json')json=args[++i];else{assert.ok(!args[i].startsWith('--'),'unknown argument');dirs.push(args[i]);}}
    assert.ok(dirs.length,'usage: compare.mjs run-dir [run-dir...] [--ref target] [--json file]');
    const summary=compare(dirs,ref);
    for(const[label,t]of Object.entries(summary.targets))console.log(`${label} vs ${summary.reference}: attempts=${t.attemptedCases} successful=${t.successfulPairs} refusals=${t.matchingRefusals} ${JSON.stringify(t.counts)}`);
    if(json){writeFileSync(json+'.tmp',JSON.stringify(summary,null,2)+'\n');renameSync(json+'.tmp',json);}
  }catch(error){console.error(error.message);process.exitCode=1;}
}
