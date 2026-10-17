#!/usr/bin/env python3
"""Bounded ordinary qualification, CPython 3.12 / Go 1.22.12 only.

Complete envelopes are compared without modification. A separately replayed,
fixed-input partial/finalization lifecycle explains only five ARM64/WASM leaves.
Every raw financial observation and projected number is checked with exact
integer/Fraction binary64 RN-even arithmetic. Compiler streams are re-emitted
and verified; this never turns cross-compilation into native execution evidence.
"""
import copy
import importlib.util
import json
import os
from pathlib import Path
import platform
import re
import struct
import subprocess
import sys
import tempfile
from fractions import Fraction

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('exact_sequential', Path(__file__).with_name('sequential-accounting-proof.py'))
exact = importlib.util.module_from_spec(spec)
spec.loader.exec_module(exact)
require, same, decode = exact.require, exact.same, exact.decode
b, add, sub, mul, div, fma, neg, rat, rn = (getattr(exact, x) for x in
    ('float_bits', 'add', 'sub', 'mul', 'div', 'fma', 'neg', 'rat', 'rn'))
ZERO, ONE, HUNDRED = b(0), b(1), b(100)
# These are input identities, not output values. Changing the bounded corpus
# requires a fresh derivation and review, rather than expanding an exemption.
SOURCE_SHA = '97bfd7a72f948d6a214a50301c1f996a29e196feccda785b773fa1ca6d60e917'
REQUEST_SHA = '09e4e884a7f590954e92679a9fd070db27e55b79ea39821a3b1434d0a7b04b26'
FIVE_PATHS = {
    '$.account.completedNetPnl.value', '$.account.realizedCashPnl.value',
    '$.account.markedNetPnl.value', '$.equity[11].unrealizedPnl',
    '$.equity[12].realizedCashPnl',
}
EXTRA_FUNCTIONS = [exact.MODULE + name for name in (
    'engine.(*broker).applyPartialManagement', 'report.ProjectOrdinaryBacktestFinancials',
    'report.validateOrdinaryFinancials')]
# Complete optimized instruction streams, collected from the unchanged source
# and Go 1.22.12 with the same build flags as the existing manifest.
EXTRA_PINS = {'js-wasm': {'github.com/spik3r/heisentick-strat/engine.(*broker).applyPartialManagement': {'instructionCount': 1578,
                                                                                            'sha256': '9d7c2d5d8dd436b46e1240240f6071bdc65cf9fa80732ac8a1eccf707c8b7aab'},
             'github.com/spik3r/heisentick-strat/report.ProjectOrdinaryBacktestFinancials': {'instructionCount': 5410,
                                                                                             'sha256': '75db7ad8d357e0c00c42cdbb00538c3400fe06d46c81bd2324ed36a1918ac20b'},
             'github.com/spik3r/heisentick-strat/report.validateOrdinaryFinancials': {'instructionCount': 6776,
                                                                                      'sha256': '2f023f81a779d45f5e9363cd9a08f88c6345da82e15b9ee4351d0abf626afb5c'}},
 'linux-amd64': {'github.com/spik3r/heisentick-strat/engine.(*broker).applyPartialManagement': {'instructionCount': 335,
                                                                                                'sha256': '09385687ad7b4286705573065ee39eb5c3e6058bac478b42321bac5d8f0ac9e8'},
                 'github.com/spik3r/heisentick-strat/report.ProjectOrdinaryBacktestFinancials': {'instructionCount': 1632,
                                                                                                 'sha256': '23e2b270976cc2d8d073c622a42582d272e8ef17d0da93cdcfc4b9138b099e8c'},
                 'github.com/spik3r/heisentick-strat/report.validateOrdinaryFinancials': {'instructionCount': 1452,
                                                                                          'sha256': 'eb610779d3ef7fe3186a85d69c17930c8df8b38893aad682941442b71d3dfaf0'}},
 'linux-arm64': {'github.com/spik3r/heisentick-strat/engine.(*broker).applyPartialManagement': {'instructionCount': 312,
                                                                                                'sha256': '89b6f5d14de4f6929094698803f010d0ecf9bd70c5cce91e5ace6657138cbb78'},
                 'github.com/spik3r/heisentick-strat/report.ProjectOrdinaryBacktestFinancials': {'instructionCount': 1626,
                                                                                                 'sha256': 'a3235f046f777238b9513f524a25968e3969d79633086d7ef0eaee940146b01b'},
                 'github.com/spik3r/heisentick-strat/report.validateOrdinaryFinancials': {'instructionCount': 1377,
                                                                                          'sha256': 'f787a0cfbdac6456f77223f3fe6ee208f2d3fac5b02f63c23caf5bd37e24758e'}}}


def value(bits):
    return struct.unpack('>d', bits.to_bytes(8, 'big'))[0]


def decimal15(bits):
    """Existing engine.numberForJSON's g/15 conversion, exact decimal RN-even."""
    x = rat(bits)
    if not x:
        return ZERO
    sign, x = (-1 if x < 0 else 1), abs(x)
    exponent = len(str(x.numerator)) - len(str(x.denominator))
    power = lambda n: Fraction(10 ** n) if n >= 0 else Fraction(1, 10 ** -n)
    if x < power(exponent):
        exponent -= 1
    scaled = x / power(exponent - 14)
    q, rem = divmod(scaled.numerator, scaled.denominator)
    if 2 * rem > scaled.denominator or (2 * rem == scaled.denominator and q & 1):
        q += 1
    return rn(sign * q * power(exponent - 14))


def at(obj, path):
    for key in path:
        obj = obj[key]
    return obj


def differences(left, right, path='$'):
    if type(left) in (int, float) and type(right) in (int, float):
        return [path] if b(left) != b(right) else []
    if type(left) is dict:
        require(type(right) is dict and left.keys() == right.keys(), 'different keys at ' + path)
        return sum((differences(left[k], right[k], path + '.' + k) for k in left), [])
    if type(left) is list:
        require(type(right) is list and len(left) == len(right), 'different array at ' + path)
        return sum((differences(x, y, f'{path}[{i}]') for i, (x, y) in enumerate(zip(left, right))), [])
    same(left, right, path)
    return []


def replay(row, target, check=True):
    require(target in exact.TARGETS, 'unknown arithmetic target')
    require(row.get('captureTarget') == target and row.get('captureGoVersion') == 'go1.22.12',
            'capture target/compiler differs from requested schedule')
    require(exact.sha(row['source'].encode()) == SOURCE_SHA and
            exact.sha(row['requestJSON'].encode()) == REQUEST_SHA, 'unreviewed partial input')
    request, response = decode(row['requestJSON']), row['response']
    financials = row.get('financials')
    require(type(financials) is dict, 'missing same-invocation financial capture')
    require(response['ok'] is True and response['binding']['producer']['goVersion'] == 'go1.22.12', 'partial response/compiler')
    bars, costs = request['data']['bars'], request['execution']['costs']
    require(len(bars) == 13 and len(financials['Events']) == 3 and len(financials['Marks']) == 13,
            'bounded partial lifecycle shape')
    events = financials['Events']
    require(re.fullmatch('[0-9a-f]{64}', events[0]['PositionID']) is not None, 'position identity')
    position = events[0]['PositionID']
    # Source risk/percentage operands, original close/low prices and costs are
    # independently bound, never fitted from observed credit/size differences.
    risk_match = re.findall(r'risk (\d+) USD', row['source'])
    fraction_match = re.findall(r'partial (\d+)% at 0.25R', row['source'])
    require(len(risk_match) == len(fraction_match) == 1, 'source operand ambiguity')
    entry, stop = b(bars[10][4]), b(bars[10][3])
    size = div(b(int(risk_match[0])), sub(entry, stop))
    fraction = div(b(int(fraction_match[0])), HUNDRED)
    part = mul(size, fraction)
    fused = exact.TARGETS[target] == 'fused'
    remainder = fma(neg(size), fraction, size) if fused else sub(size, part)
    fee, start = b(costs['feePerUnit']), b(costs['startEquity'])
    require(costs['slippage'] == costs['slippageBps'] == 0 and costs['fillOn'] == 'close', 'cost schedule')
    entry_fee = mul(fee, size)
    realized = fma(neg(fee), size, ZERO) if fused else sub(ZERO, entry_fee)
    expected_events = []
    expected_events.append(dict(Kind='entry', PositionID=position, Side='long', Index=10, Ordinal=0,
        TradeIndex=-1, T=bars[10][0], Price=value(entry), Size=value(size), Fee=value(entry_fee),
        RealizedBefore=0, RealizedAfter=value(realized), ExitCredit=0))
    realized_at_entry = realized
    credits, exit_fees = ZERO, ZERO
    for n, (kind, index, amount, ordinal) in enumerate((('partial', 11, part, 0), ('exit', 12, remainder, 1))):
        price = b(bars[index][4])
        points = mul(sub(price, entry), ONE)
        credit = fma(neg(fee), amount, mul(points, amount)) if fused else sub(mul(points, amount), mul(fee, amount))
        before, realized = realized, add(realized, credit)
        nominal_fee = mul(fee, amount)
        credits, exit_fees = add(credits, credit), add(exit_fees, nominal_fee)
        expected_events.append(dict(Kind=kind, PositionID=position, Side='long', Index=index, Ordinal=ordinal,
            TradeIndex=n, T=bars[index][0], Price=value(price), Size=value(amount), Fee=value(nominal_fee),
            RealizedBefore=value(before), RealizedAfter=value(realized), ExitCredit=value(credit)))
    marks = []
    for i, bar in enumerate(bars):
        r = ZERO if i < 10 else realized_at_entry if i == 10 else b(expected_events[i - 10]['RealizedAfter'])
        u = mul(mul(sub(b(bar[4]), entry), ONE), size if i == 10 else remainder) if i in (10, 11) else ZERO
        ef = entry_fee if i >= 10 else ZERO
        xf = ZERO if i <= 10 else b(expected_events[1]['Fee']) if i == 11 else exit_fees
        marks.append(dict(Index=i, T=bar[0], Realized=value(r), Unrealized=value(u),
            Equity=value(add(add(start, r), u)), EntryFees=value(ef), ExitFees=value(xf)))
    expected_financials = dict(StartEquity=value(start), TerminalRealized=value(realized),
        LastFlatRealized=value(realized), TerminalUnrealized=0, OpenEntryFee=0,
        EntryFees=value(entry_fee), ExitFees=value(exit_fees), Events=expected_events, Marks=marks)
    if check:
        same(financials, expected_financials, '$financials')
    predicted = {}
    def expect(path, bits):
        # The fixture is finite with positive/zero projection operands. Reject
        # unexpected signed zero; never normalize any observed response.
        predicted[path] = bits
        if check:
            require(b(at(response, path)) == bits, 'replay mismatch at ' + repr(path))
    account = dict(startEquity=start, completedEndEquity=add(start, realized), completedNetPnl=realized,
        completedReturnPct=mul(div(realized, start), HUNDRED), realizedCashPnl=realized,
        markedEndEquity=add(add(start, realized), ZERO), markedNetPnl=add(realized, ZERO),
        markedReturnPct=mul(div(add(realized, ZERO), start), HUNDRED), openUnrealizedPnl=ZERO,
        openEntryFeePaid=ZERO, entryFeesPaid=entry_fee, exitFeesPaid=exit_fees)
    for key, bits in account.items():
        expect(('account', key, 'value'), bits)
    net = add(ZERO, sub(credits, entry_fee))
    require(rat(net) > 0, 'bounded lifecycle must be profitable')
    for key, bits in dict(count=ONE, wins=ONE, losses=ZERO, breakeven=ZERO, netPnl=net, winRate=ONE).items():
        expect(('completedPositions', key, 'value'), bits)
    peak, dd, pct, dd_at, pct_at = start, ZERO, ZERO, None, None
    for i, mark in enumerate(marks):
        for key, raw in dict(index='Index', t='T', realizedCashPnl='Realized', unrealizedPnl='Unrealized',
                entryFeesPaid='EntryFees', exitFeesPaid='ExitFees', equity='Equity').items():
            expect(('equity', i, key), b(mark[raw]))
        equity = b(mark['Equity'])
        if rat(equity) > rat(peak):
            peak = equity
        amount = sub(peak, equity)
        percent = mul(div(amount, peak), HUNDRED)
        if rat(amount) > rat(dd):
            dd, dd_at = amount, dict(index=i, t=mark['T'])
        if rat(percent) > rat(pct):
            pct, pct_at = percent, dict(index=i, t=mark['T'])
    expect(('drawdown', 'amount', 'value'), dd)
    expect(('drawdown', 'percent', 'value'), pct)
    if check:
        same(response['drawdown']['amountAt'], dd_at)
        same(response['drawdown']['percentAt'], pct_at)
    # The public nativeRun intentionally uses the pre-existing 15-significant
    # decimal serialization; reproduce it, do not compare rounded raw captures.
    require(len(response['nativeRun']['trades']) == 2 and response['nativeRun']['tradeCount'] == 2, 'native trade membership')
    for i, event in enumerate(expected_events[1:]):
        for key, bits in dict(entry=entry, exit=b(event['Price']), size=b(event['Size']),
                points=sub(b(event['Price']), entry), pnl=b(event['ExitCredit'])).items():
            expect(('nativeRun', 'trades', i, key), decimal15(bits))
    return expected_financials, predicted


def path_string(path):
    return '$' + ''.join(f'[{k}]' if isinstance(k, int) else '.' + k for k in path)


def verify_pair(native, wasm, inputs, target):
    require(target in ('linux-amd64', 'linux-arm64'), 'unsupported native target')
    require(type(inputs) is list and len(inputs) == 44, 'complete 44-case input corpus required')
    require(len(native) == len(wasm) == len(inputs), 'missing or extra corpus cases')
    names = [x['name'] for x in inputs]
    require(len(set(names)) == len(names) and names.count('partial-finalization') == 1, 'duplicate/missing case')
    differences_seen, raw_differences, arithmetic_checks = [], [], 0
    for n, w, original in zip(native, wasm, inputs):
        require(set(original) == {'name', 'operation', 'requestJSON', 'source', 'expectedCode'}, 'input fields')
        extra = {'response'} | ({'financials', 'captureTarget', 'captureGoVersion'} if original['name'] == 'partial-finalization' else set())
        require(set(n) == set(w) == set(original) | extra, 'capture fields/membership')
        for key in original:
            same(n[key], original[key], '$native.' + key)
            same(w[key], original[key], '$wasm.' + key)
        if original['name'] != 'partial-finalization':
            same(n['response'], w['response'], '$' + original['name'])
            continue
        nf, np = replay(n, target)
        wf, wp = replay(w, 'js-wasm')
        arithmetic_checks += len(np) + len(wp)
        # Every raw field is replayed. Non-arithmetic identity fields must also
        # be equal, and each actual differing envelope leaf must be predicted.
        same(n['response']['binding'], w['response']['binding'], '$binding')
        raw_differences = differences(nf, wf, '$financials')
        actual = set(differences(n['response'], w['response']))
        predicted = {path_string(path) for path in np if np[path] != wp[path]}
        allowed = FIVE_PATHS if target == 'linux-arm64' else set()
        require(predicted == allowed and actual == predicted, 'unexplained/missing cross-target difference')
        differences_seen = sorted(actual)
    return dict(schema='ordinary-exact-arithmetic-proof-v1', ok=True, nativeTarget=target,
        wasmTarget='js-wasm', cases=len(inputs), exactArithmeticChecks=arithmetic_checks,
        explainedResponsePaths=differences_seen, explainedRawFinancialPaths=raw_differences, allOtherCompleteEnvelopesExact=True,
        sameInvocationRawCapturesChecked=True, tolerance=None)


def mutation_tests(native, wasm, inputs, target):
    passed = []
    index = next(i for i, row in enumerate(inputs) if row['name'] == 'partial-finalization')
    def reject(name, edit, selected_target=target):
        n, w, source = copy.deepcopy(native), copy.deepcopy(wasm), copy.deepcopy(inputs)
        edit(n, w, source)
        try:
            verify_pair(n, w, source, selected_target)
        except (exact.ProofError, KeyError, TypeError, IndexError):
            passed.append(name)
            return
        raise exact.ProofError('mutation escaped: ' + name)
    reject('producer identity', lambda n,w,s: n[index]['response']['binding']['producer'].__setitem__('tag', 'changed'))
    reject('upstream size', lambda n,w,s: n[index]['financials']['Events'][0].__setitem__('Size', 20))
    reject('raw credit', lambda n,w,s: n[index]['financials']['Events'][1].__setitem__('ExitCredit', 40))
    reject('unexplained report number', lambda n,w,s: n[index]['response']['account']['completedNetPnl'].__setitem__('value', 54))
    reject('report category', lambda n,w,s: n[index]['response']['account']['completedNetPnl'].__setitem__('status', 'unavailable'))
    reject('unexpected other case', lambda n,w,s: n[0]['response'].__setitem__('ok', False))
    reject('missing case', lambda n,w,s: n.pop())
    reject('extra case', lambda n,w,s: n.append(copy.deepcopy(n[-1])))
    reject('missing both targets and input', lambda n,w,s: (n.pop(), w.pop(), s.pop()))
    reject('missing capture', lambda n,w,s: n[index].pop('financials'))
    reject('wrong target schedule', lambda n,w,s: n[index].__setitem__('captureTarget', 'js-wasm'))
    other_target = 'linux-arm64' if target == 'linux-amd64' else 'linux-amd64'
    reject('wrong arithmetic with coherent metadata',
        lambda n,w,s: n[index].__setitem__('captureTarget', other_target), other_target)
    reject('wrong compiler', lambda n,w,s: n[index].__setitem__('captureGoVersion', 'go1.23'))
    reject('changed input', lambda n,w,s: s[index].__setitem__('source', s[index]['source'] + '\n'))
    reject('signed zero', lambda n,w,s: n[index]['financials'].__setitem__('TerminalUnrealized', -0.0))
    return passed


def codegen(target, output):
    manifest = decode(exact.MANIFEST.read_text())
    exact.validate_manifest(manifest, ROOT)
    env = os.environ.copy()
    require(not any(env.get(key) for key in ('GOCOMPILEDEBUG', 'GOEXPERIMENT', 'GOFLAGS', 'GODEBUG', 'GOWASM', 'GOARM64')),
            'unsupported compiler/runtime options')
    require(env.get('GOAMD64', 'v1') in ('', 'v1'), 'unsupported GOAMD64')
    def run(args, specific=None):
        p = subprocess.run(['go'] + args, cwd=ROOT, env=env | (specific or {}), text=True, capture_output=True)
        require(p.returncode == 0, 'compiler command failed: ' + p.stderr[:1000])
        return p
    config = decode(run(['env', '-json', 'GOVERSION', 'GOARCH', 'GOHOSTARCH', 'GOOS', 'GOHOSTOS',
        'GOAMD64', 'GOFLAGS', 'GOEXPERIMENT', 'GOWASM', 'GOWORK', 'GOROOT']).stdout)
    machine = {'x86_64': 'amd64', 'aarch64': 'arm64'}.get(platform.machine())
    require({k: config[k] for k in ('GOVERSION', 'GOARCH', 'GOHOSTARCH', 'GOOS', 'GOHOSTOS')} ==
            dict(GOVERSION='go1.22.12', GOARCH=machine, GOHOSTARCH=machine, GOOS='linux', GOHOSTOS='linux') and
            target == 'linux-' + machine, 'native compiler/host/selected schedule mismatch')
    require(all(config[k] == '' for k in ('GOFLAGS', 'GOEXPERIMENT', 'GOWASM')) and
            config['GOAMD64'] in ('', 'v1') and config['GOWORK'] in ('', 'off'), 'unsupported effective Go settings')
    require(run(['env', 'GOAMD64'], dict(GOARCH='amd64')).stdout.strip() == 'v1' and
            run(['env', 'GOWASM'], dict(GOOS='js', GOARCH='wasm')).stdout.strip() == '',
            'unsupported effective cross-target features')
    env.update(CGO_ENABLED='0', GOTOOLCHAIN='local', GOPROXY='off', GOENV='off', GOWORK='off',
               GOAMD64='v1', GOROOT=config['GOROOT'])
    receipts = {}
    with tempfile.TemporaryDirectory(prefix='ordinary-codegen-', dir=output) as scratch:
        for name in (target, 'js-wasm'):
            goos, arch = name.split('-')
            specific = dict(GOOS=goos, GOARCH=arch)
            dependencies = run(['list', '-buildvcs=false', '-trimpath', '-deps', '-json', './cmd/enginewasm', './cmd/dslwasm', './engine', './report'], specific)
            require(exact.dependency_inputs(dependencies.stdout, ROOT) == manifest['targets'][name]['buildInputs'], 'changed compiler dependencies')
            built = run(['build', '-buildvcs=false', '-trimpath', '-gcflags=' + exact.MODULE + 'engine=-S',
                '-gcflags=' + exact.MODULE + 'report=-S', '-o', str(Path(scratch) / name), './cmd/enginewasm'], specific)
            original_functions = exact.FUNCTIONS
            try:
                exact.FUNCTIONS = original_functions + EXTRA_FUNCTIONS
                streams = exact.instruction_lists(built.stderr)
            finally:
                exact.FUNCTIONS = original_functions
            observed = {fn: dict(sha256=exact.sha(('\n'.join(lines) + '\n').encode()), instructionCount=len(lines)) for fn, lines in streams.items()}
            require(observed == manifest['targets'][name]['instructions'] | EXTRA_PINS[name], 'unknown compiler schedule ' + name)
            for fn, lines in streams.items():
                filename = fn.removeprefix(exact.MODULE).replace('/', '-').replace('(*', '').replace(')', '')
                (output / (name + '.' + filename + '.instructions.txt')).write_text('\n'.join(lines) + '\n')
            receipts[name] = observed
    require(exact.source_hashes(ROOT) == manifest['sources'], 'source changed during compiler check')
    return dict(goVersion='go1.22.12', nativeTarget=target, manifestSha256=exact.sha(exact.MANIFEST.read_bytes()), targets=receipts)


def main():
    exact.supported_python()
    require(len(sys.argv) == 6, 'usage: proof.py native.json wasm.json input.json linux-amd64|linux-arm64 external-output')
    native, wasm, inputs = [decode(Path(path).read_text()) for path in sys.argv[1:4]]
    target, output = sys.argv[4], Path(sys.argv[5]).resolve()
    require(output.is_dir() and not output.is_relative_to(ROOT), 'external evidence directory required')
    result = verify_pair(native, wasm, inputs, target)
    result['mutationTests'] = mutation_tests(native, wasm, inputs, target)
    result['compiler'] = codegen(target, output)
    print(json.dumps(result, sort_keys=True))


if __name__ == '__main__':
    try:
        main()
    except (exact.ProofError, KeyError, TypeError, IndexError) as error:
        print('ordinary arithmetic proof refused: ' + str(error), file=sys.stderr)
        sys.exit(1)
