#!/usr/bin/env python3
"""Same-target raw-bit preservation against the immutable ordinary-engine baseline.

Test-only overlays never edit either source tree. Output must be outside the tree.
The actual host must be Linux AMD64 or ARM64 with Go 1.22.12 and Node available.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import signal
import subprocess
import tarfile
import tempfile
import time

BASELINE = '94a2b5a16eae61ffab53a933a78aa302ce57379c'
TREE = 'c97a215bd5c3055e32d981564d48d58208273d46'
PROBE = '''package engine
import("fmt";"math";"testing";"github.com/spik3r/heisentick-strat/contextcols";"github.com/spik3r/heisentick-strat/marketdata")
func TestP2CPartialBaselineProbe(t *testing.T) {
for _,direction:=range []side{sideLong,sideShort}{for _,size:=range []float64{.7,10,13.3}{for _,fraction:=range []float64{.3,.5,.7,1}{for _,fee:=range []float64{0,.1}{
closes:=[]float64{100,105,110};stop:=90.;if direction==sideShort{closes=[]float64{100,95,90};stop=110}
bars:=make([]marketdata.Bar,3);for i,v:=range closes{bars[i]=marketdata.Bar{T:float64(i+1),O:v,H:v+1,L:v-1,C:v,V:1}}
var b broker;b.reset(marketdata.SeriesFromBars(bars),contextcols.Columns{},nil,nil,nil,flagParams{},RunFixture{Costs:Costs{StartEquity:1000,FeePerUnit:fee,Slippage:.25,FillOn:"close"}},nil)
P2C_ATTACH
b.params.Partial.Enabled=true;b.params.Partial.Fraction=fraction;b.params.Partial.TriggerR=.25;b.params.Partial.MoveBreakeven=true
b.openPosition(direction,100,order{Size:size,HasSize:true,SL:stop,NoTarget:true},0)
b.applyPartialManagement(1)
fmt.Printf("P2C %s %.17g %.17g %.17g partial %016x %016x %t %t %d",direction.String(),size,fraction,fee,math.Float64bits(b.realized),math.Float64bits(b.position.Size),b.hasPosition,b.position.PartialTaken,len(b.trades))
for _,tr:=range b.trades{fmt.Printf(" %016x:%016x:%016x",math.Float64bits(tr.Size),math.Float64bits(tr.PnL),math.Float64bits(tr.Points))};fmt.Println()
if b.hasPosition{b.closePosition(closes[2],2,"end","")}
fmt.Printf("P2C %s %.17g %.17g %.17g final %016x %016x %t %d",direction.String(),size,fraction,fee,math.Float64bits(b.realized),math.Float64bits(b.position.Size),b.hasPosition,len(b.trades));for _,tr:=range b.trades{fmt.Printf(" %016x:%016x:%016x",math.Float64bits(tr.Size),math.Float64bits(tr.PnL),math.Float64bits(tr.Points))};fmt.Println()
P2C_ASSERT
}}}}
}
'''
ATTACH = {'off': '', 'financial': 'b.paperAccounting = newPaperCollector("probe",1000)',
          'state': 'x,err:=attachOrdinaryBacktestStateCapture(&b);if err!=nil{t.Fatal(err)}'}
STATE_ASSERT = r'''
if x.err != nil || x.financial.collector.err != nil || b.paperAccounting != x.financial.collector || b.paperAccounting.ordinaryState != x { t.Fatal("state capture inactive or failed") }
counts:=map[string]int{};for _,state:=range x.states{counts[state.Boundary]++}
want:=map[string]int{"initial":1,"before-entry":1,"after-entry":1,"before-exit":1,"after-exit":1}
if fraction < 1 {want["before-partial"]=1;want["after-partial"]=1}
if len(counts)!=len(want){t.Fatalf("state boundary count mismatch: %v",counts)}
for boundary,n:=range want{if counts[boundary]!=n{t.Fatalf("missing actual state boundary %s: %v",boundary,counts)}}
last:=x.states[len(x.states)-1];if last.Boundary!="after-exit" || last.HasPosition || math.Float64bits(last.Realized)!=math.Float64bits(b.realized) || last.TradeCount!=len(b.trades){t.Fatal("state terminal exit does not match actual broker")}
'''

MUTATIONS = {
    'partial-credit': ('closedSize := pos.Size * fraction', 'closedSize := pos.Size * (fraction / 2)'),
    'fee-sign': ('pnl := points*closedSize - b.costs.FeePerUnit*closedSize', 'pnl := points*closedSize + b.costs.FeePerUnit*closedSize'),
    'remainder': ('pos.Size -= closedSize', 'pos.Size -= closedSize / 2'),
}


def sha(data):
    return hashlib.sha256(data).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--out', required=True)
    parser.add_argument('--go', default='go')
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[2]
    output = Path(args.out).resolve()
    if output == root or root in output.parents or output.exists():
        raise SystemExit('output must be a new directory outside the source tree')
    output.mkdir(parents=True)
    receipt = {'schema': 'ordinary-state-preservation-v1', 'status': 'FAIL',
               'baselineCommit': BASELINE, 'baselineTree': TREE, 'runs': [], 'mutations': [],
               'qualification': 'Invented partial-credit/fee/remainder cases only; same-target exact bits, actual native and Node-WASM. No historical or cross-target parity claim.'}
    def reject(message):
        receipt['error'] = message
        (output/'receipt.json').write_text(json.dumps(receipt, indent=2)+'\n')
        raise RuntimeError(message)

    deadline = time.monotonic() + 900
    go = shutil.which(args.go)
    node = shutil.which('node')
    if not go or not node:
        reject('Go and Node are required')
    go = str(Path(go).resolve())
    for key in ('GOFLAGS', 'GOCOMPILEDEBUG', 'GOEXPERIMENT', 'GODEBUG', 'NODE_OPTIONS'):
        if os.environ.get(key):
            reject('ordinary compiler/runtime settings required: ' + key)
    env = {'PATH': str(Path(go).parent)+':'+str(Path(node).resolve().parent)+':/usr/local/bin:/usr/bin:/bin',
           'HOME': str(output), 'LANG': 'C.UTF-8', 'TZ': 'UTC'}
    if os.environ.get('GOROOT'):
        env['GOROOT'] = os.environ['GOROOT']
    env.update(GOTOOLCHAIN='local', GOENV='off', GOPROXY='off', GOSUMDB='off', GOWORK='off',
               CGO_ENABLED='0', GOMAXPROCS='2', GOMEMLIMIT='3GiB')
    for key in ('GOOS', 'GOARCH', 'GOAMD64', 'GOARM64', 'GOWASM'):
        env.pop(key, None)
    # A new private directory is retained with the receipt on failure for diagnosis.
    work = output / 'work'
    work.mkdir()
    for key, directory in [('GOCACHE', 'cache'), ('GOTMPDIR', 'tmp'), ('TMPDIR', 'tmp'),
                           ('GOMODCACHE', 'modules'), ('GOPATH', 'gopath')]:
        path = work / directory
        path.mkdir(exist_ok=True)
        env[key] = str(path)

    def execute(label, command, cwd=root, extra=None):
        runenv = dict(env, **(extra or {}))
        log = output / (label + '.log')
        started = time.monotonic()
        stop = None
        peak = 0
        with log.open('wb') as stream:
            proc = subprocess.Popen(command, cwd=cwd, env=runenv, stdout=stream,
                                    stderr=subprocess.STDOUT, start_new_session=True)
            try:
                while proc.poll() is None:
                    free = shutil.disk_usage(output).free
                    memory = {line.split(':')[0]: int(line.split()[1])*1024
                              for line in Path('/proc/meminfo').read_text().splitlines()}
                    rss = 0
                    for entry in Path('/proc').iterdir():
                        if not entry.name.isdigit():
                            continue
                        try:
                            stat = (entry/'stat').read_text().rsplit(') ', 1)[1].split()
                            if int(stat[2]) == proc.pid:
                                for line in (entry/'status').read_text().splitlines():
                                    if line.startswith('VmRSS:'):
                                        rss += int(line.split()[1])*1024
                        except (OSError, ValueError, IndexError):
                            pass
                    peak = max(peak, rss)
                    if time.monotonic() > min(deadline, started+180):
                        stop = 'time limit'
                    elif free < 512*1024**2 or memory.get('MemAvailable', 0) < 512*1024**2:
                        stop = 'disk/memory floor'
                    elif rss > 4*1024**3:
                        stop = 'process-group RSS limit'
                    if stop:
                        os.killpg(proc.pid, signal.SIGKILL)
                        break
                    time.sleep(.5)
                code = proc.wait()
            finally:
                # Monitoring failures must not leave qualification children running.
                if proc.poll() is None:
                    try:
                        os.killpg(proc.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                proc.wait()
        item = {'label': label, 'argv': command, 'cwd': str(cwd), 'targetOverrides': extra or {}, 'effectiveEnvironment': runenv,
                'exitCode': code, 'seconds': time.monotonic()-started, 'stopReason': stop,
                'sampledPeakRSSBytes': peak, 'logSha256': sha(log.read_bytes())}
        receipt['runs'].append(item)
        if code or stop:
            raise RuntimeError('qualification command failed: ' + label)
        return log.read_text()

    def source_inventory():
        suffixes = {'.go', '.s', '.S', '.syso', '.c', '.h', '.cc', '.cpp', '.cxx',
                    '.m', '.f', '.f90', '.swig', '.swigcxx', '.o', '.a'}
        names = {'go.mod', 'go.sum', 'go.work', 'go.work.sum'}
        paths = [p for p in root.rglob('*') if p.is_file() and '.git' not in p.parts and
                 (p.suffix in suffixes or p.name in names)]
        paths += [Path(__file__).resolve(), root/'.github/workflows/ci.yml']
        return {str(p.relative_to(root)): sha(p.read_bytes()) for p in sorted(set(paths))}

    try:
        if platform.system() != 'Linux' or platform.machine() not in ('x86_64', 'aarch64', 'arm64'):
            raise RuntimeError('actual Linux AMD64/ARM64 host required')
        settings = json.loads(execute('go-env', [go, 'env', '-json', 'GOVERSION', 'GOARCH', 'GOOS', 'GOROOT', 'GOFLAGS', 'GOEXPERIMENT']))
        native = {'x86_64': 'amd64', 'aarch64': 'arm64', 'arm64': 'arm64'}[platform.machine()]
        if settings['GOVERSION'] != 'go1.22.12' or settings['GOOS'] != 'linux' or settings['GOARCH'] != native or settings['GOFLAGS'] or settings['GOEXPERIMENT']:
            raise RuntimeError('wrong native compiler/target/settings')
        wrapper = Path(settings['GOROOT'])/'misc/wasm/go_js_wasm_exec'
        receipt['toolHashes'] = {str(p): sha(p.read_bytes()) for p in
                                 [Path(go), Path(node).resolve(), wrapper,
                                  Path(settings['GOROOT'])/'misc/wasm/wasm_exec.js',
                                  Path(settings['GOROOT'])/'misc/wasm/wasm_exec_node.js']}
        receipt.update(nativeTarget='linux-'+native, go=settings, goBinarySha256=sha(Path(go).read_bytes()),
                       node=execute('node-version', [node, '--version']).strip(), probeSha256=sha(PROBE.encode()))
        actual = execute('baseline-revision', ['git', 'rev-parse', BASELINE+'^{commit}', BASELINE+'^{tree}']).splitlines()
        if actual != [BASELINE, TREE]:
            raise RuntimeError('immutable baseline identity mismatch')
        archive = work/'baseline.tar'
        execute('baseline-archive', ['git', 'archive', '--format=tar', '--output='+str(archive), BASELINE])
        baseline = work/'baseline'
        baseline.mkdir()
        with tarfile.open(archive) as tar:
            tar.extractall(baseline, filter='data')
        inventory = execute('baseline-tree', ['git', 'ls-tree', '-r', '--full-tree', BASELINE])
        leaves = 0
        for line in inventory.splitlines():
            header, path = line.split('\t', 1)
            mode, typ, blob = header.split()
            if typ != 'blob' or mode not in ('100644', '100755'):
                raise RuntimeError('unreviewed baseline entry')
            data = (baseline/path).read_bytes()
            if hashlib.sha1(b'blob '+str(len(data)).encode()+b'\0'+data).hexdigest() != blob:
                raise RuntimeError('baseline file mismatch: '+path)
            leaves += 1
        receipt['baselineLeaves'] = leaves
        receipt['candidateSources'] = source_inventory()
        before = dict(receipt['candidateSources'])
        original_flag = (root/'engine/flag.go').read_text()
        expected = {}
        def probe(kind, mode, target, mutation=None):
            source = baseline if kind == 'baseline' else root
            label = '-'.join([kind, mode, target]+([mutation] if mutation else []))
            test = work/(label+'_test.go')
            test.write_text(PROBE.replace('P2C_ATTACH', ATTACH[mode]).replace('P2C_ASSERT', STATE_ASSERT if mode == 'state' else ''))
            replacements = {str(source/'engine/zz_ordinary_state_probe_test.go'): str(test)}
            if mutation:
                old, new = MUTATIONS[mutation]
                if original_flag.count(old) != 1:
                    raise RuntimeError('mutation anchor is not unique: '+mutation)
                mutated = work/(label+'-flag.go')
                mutated.write_text(original_flag.replace(old, new))
                replacements[str(root/'engine/flag.go')] = str(mutated)
            overlay = work/(label+'-overlay.json')
            overlay.write_text(json.dumps({'Replace': replacements}))
            command = [go, 'test', '-vet=off', '-p=1', '-count=1', '-v', '-overlay='+str(overlay)]
            extra = {}
            if target == 'wasm':
                extra.update(GOOS='js', GOARCH='wasm', GOMAXPROCS='1')
                command.append('-exec='+str(Path(settings['GOROOT'])/'misc/wasm/go_js_wasm_exec'))
            command += ['-run', '^TestP2CPartialBaselineProbe$', './engine']
            text = execute(label, command, source, extra)
            rows = [line for line in text.splitlines() if line.startswith('P2C ')]
            if len(rows) != 96 or len(set(rows)) != 96 or '--- PASS: TestP2CPartialBaselineProbe' not in text:
                raise RuntimeError('missing/duplicate raw probe rows: '+label)
            receipt['runs'][-1].update(probeRows=len(rows), rawRowsSha256=sha(('\n'.join(rows)+'\n').encode()),
                                        overlaySha256=sha(overlay.read_bytes()), testSha256=sha(test.read_bytes()))
            return rows
        for target in ('native', 'wasm'):
            expected[target] = probe('baseline', 'off', target)
            for kind, mode in [('baseline', 'financial'), ('candidate', 'off'), ('candidate', 'financial'), ('candidate', 'state')]:
                if probe(kind, mode, target) != expected[target]:
                    raise RuntimeError('financial raw bits changed: '+kind+'/'+mode+'/'+target)
        for mutation in MUTATIONS:
            rows = probe('candidate', 'off', 'native', mutation)
            differences = sum(a != b for a,b in zip(rows, expected['native']))
            if not differences:
                raise RuntimeError('deliberate mutation escaped: '+mutation)
            receipt['mutations'].append({'name': mutation, 'differentRows': differences, 'detected': True})
        after = source_inventory()
        if after != before:
            raise RuntimeError('candidate source changed during qualification')
        receipt.update(status='PASS', casesPerMode=48, rowsPerMode=96, captureModesPerTarget=5,
                       targetComparisons=2, vetNote='Only virtual probe tests use -vet=off; repository vet is a separate required gate.')
    except Exception as exc:
        receipt['error'] = str(exc)
        raise
    finally:
        (output/'receipt.json').write_text(json.dumps(receipt, indent=2)+'\n')
    print(json.dumps({'status': receipt['status'], 'nativeTarget': receipt['nativeTarget'],
                      'casesPerMode': 48, 'modesPerTarget': 5, 'mutationsDetected': 3,
                      'receiptSha256': sha((output/'receipt.json').read_bytes())}))


if __name__ == '__main__':
    main()
