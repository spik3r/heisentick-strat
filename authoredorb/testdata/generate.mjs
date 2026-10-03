import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
const appRoot = process.argv[2];
if (!appRoot) throw new Error('usage: node generate.mjs /path/to/heisentick-app');
const fromApp = (relative) => pathToFileURL(path.join(appRoot, relative)).href;
const { runBacktest } = await import(fromApp('engine/engine.js'));
const { default: parity } = await import(fromApp('engine/strategies/daySessionOrbOriginalTvParity.js'));
const { default: base } = await import(fromApp('engine/strategies/daySessionOrbStrategy.js'));
const root=new URL('.', import.meta.url).pathname; fs.mkdirSync(root,{recursive:true});
function bars(day, rows) {return rows.map(([h,m,o,hi,lo,c,v=10])=>({t:Date.UTC(2026,0,day,h-10,m),o,h:hi,l:lo,c,v}));}
const shortRows=[[9,0,110,111,109,110],[9,30,108,109,107,108],[10,0,106,107,105,106],[10,30,104,105,103,104],[11,0,102,103,101,102],[11,30,100,101,99,100],[12,0,99,101,98,99],[12,30,98,100,97,98],[13,0,97,98,94,95],[13,30,96,98,94,96],[14,0,95,97,94,95],[15,30,94,96,93,94],[16,0,94,96,93,94]];
const longRows=[[9,0,90,91,89,90],[9,30,92,93,91,92],[10,0,94,95,93,94],[10,30,96,97,95,96],[11,0,98,99,97,98],[11,30,100,101,99,100],[12,0,100,102,99,100],[12,30,101,103,100,101],[13,0,103,105,102,104],[13,30,105,107,103,105],[14,0,108,114,107,113],[16,0,113,115,112,113]];
const retryRows=shortRows.map(row=>[...row]);retryRows[9]=[13,30,110,111,94,110];retryRows[10]=[14,0,100,110,94,95];
const computedVwapResumeRows=shortRows.map(row=>[...row]);computedVwapResumeRows[9]=[13,30,94,98,93,94,100000];computedVwapResumeRows[10]=[14,0,110,111,94,95,10];computedVwapResumeRows[11]=[15,30,94,95,92,93,10];
const pullRows=[[9,0,110,111,109,110],[9,30,108,109,107,108],[10,0,106,107,105,106],[10,30,104,105,103,104],[11,0,102,103,101,102],[11,30,100,101,99,100],[12,0,99,101,98,99],[12,30,98,100,97,98],[13,0,98,110,96,97],[13,30,110,111,95,110],[14,0,96,97,90,91],[16,0,91,92,90,91]];
const cases=[
 {name:'parity-held-through-session-close',strategy:parity,rows:shortRows,params:{},costs:{slippage:0.06,feePerUnit:0,fillOn:'close',startEquity:10000}},
 {name:'parity-explicit-zero-start-equity',strategy:parity,rows:shortRows,params:{},costs:{slippage:0.06,feePerUnit:0,fillOn:'close',startEquity:0}},
 {name:'base-route',strategy:base,rows:shortRows,params:{},costs:{slippage:0.06,feePerUnit:0,fillOn:'close',startEquity:10000}},
 {name:'parity-long-target-next-open',strategy:parity,rows:longRows,params:{direction:1},costs:{slippage:0.06,feePerUnit:0.1,fillOn:'nextOpen',startEquity:10000}},
 {name:'parity-computed-vwap',strategy:parity,rows:computedVwapResumeRows,params:{oneTradePerDay:0},contextMode:'computed',costs:{slippage:0.06,feePerUnit:0,fillOn:'close',startEquity:10000}},
 {name:'parity-hlc3-fallback',strategy:parity,rows:shortRows.map(row=>[...row.slice(0,6),0]),params:{},contextMode:'computed',costs:{slippage:0.06,feePerUnit:0,fillOn:'close',startEquity:10000}},
 {name:'parity-first-bar-hlc3-double-count',strategy:parity,rows:[[12,0,100,100,100,100,0],[12,30,110,110,110,110,0],[13,0,105,105,105,105,0]],params:{strategyStyle:1,direction:1},contextMode:'computed',costs:{slippage:0.06,feePerUnit:0,fillOn:'close',startEquity:10000}},
 {name:'parity-explicit-null-vwap',strategy:parity,rows:[[12,0,100,100,100,100,0],[12,30,110,110,110,110,0],[13,0,103,103,103,103,0]],params:{strategyStyle:1,direction:1},contextMode:'null',costs:{slippage:0.06,feePerUnit:0,fillOn:'close',startEquity:10000}},
 {name:'parity-one-trade-day',strategy:parity,rows:retryRows,params:{oneTradePerDay:1},costs:{slippage:0.06,feePerUnit:0,fillOn:'close',startEquity:10000}},
 {name:'parity-retry-same-day',strategy:parity,rows:retryRows,params:{oneTradePerDay:0},costs:{slippage:0.06,feePerUnit:0,fillOn:'close',startEquity:10000}},
 {name:'parity-pullback-short-stop',strategy:parity,rows:pullRows,params:{strategyStyle:2,oneTradePerDay:0},costs:{slippage:0.06,feePerUnit:0,fillOn:'close',startEquity:10000}},
];
for(const item of cases){
 const inputBars=bars(5,item.rows);const contextVwap=inputBars.map(()=>100);
 const strategy={...item.strategy,params:{...item.strategy.params,...item.params}};
 const opts={symbol:'XAUUSD',timeframe:item.strategy===base?'5m':'30m',...item.costs};if(item.contextMode!=='computed')opts.ctx=inputBars.map((_,i)=>({vwap:item.contextMode==='null'?null:contextVwap[i]}));
 const result=runBacktest(inputBars,strategy,opts);
 const request={schema:'authored-orb-run-v1',strategyId:strategy.id,symbol:'XAUUSD',timeframe:item.strategy===base?'5m':'30m',params:item.params,costs:item.costs,bars:inputBars};if(item.contextMode!=='computed')request.contextVwap=item.contextMode==='null'?inputBars.map(()=>null):contextVwap;
 fs.writeFileSync(`${root}/${item.name}.json`,JSON.stringify({request,expected:{trades:result.trades,equityCurve:result.equityCurve}},null,2)+'\n');
 console.log(item.name,result.trades.map(t=>`${t.side} ${t.reason} ${t.entryIndex}->${t.exitIndex} ${t.pnl}`));
}
