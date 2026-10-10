#!/usr/bin/env python3
"""Test-only exact Sequential qualification; CPython 3.12.x, standard library.

No engine execution, decimal epsilon, observed-delta fitting, or golden lookup is
used by the replay. Inputs are independently typed, complete Go Float64bits
captures bound to the real companion and original synthetic fixture/source.
Every downstream value is recomputed with integer/rational IEEE-754 RN-even
arithmetic. Other fields, upstream operands and signed classifications are exact.
The separate codegen command verifies the pinned final source and compiler's
optimized instruction schedules. It does not claim native execution coverage.
"""
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import re
import struct
import subprocess
import sys
import tempfile
from fractions import Fraction


class ProofError(ValueError):
    pass


def require(condition, message):
    if not condition:
        raise ProofError(message)


def supported_python():
    require(platform.python_implementation() == 'CPython' and sys.version_info[:2] == (3, 12),
            'exact accounting proof requires installed CPython 3.12.x; no fallback or installation')
    require(struct.calcsize('d') == 8 and sys.float_info.mant_dig == 53,
            'exact accounting proof requires IEEE-754 binary64 JSON decoding')


SIGN = 1 << 63
FRAC = (1 << 52) - 1
MAX = 0x7fefffffffffffff
TARGETS = {'linux-amd64': 'separate', 'linux-arm64': 'fused', 'js-wasm': 'separate'}
MODULE = 'github.com/spik3r/heisentick-strat/'
FUNCTIONS = [MODULE + suffix for suffix in (
    'engine.(*broker).openPosition', 'engine.(*broker).closePosition',
    'engine.(*sequentialAccountingCollector).entry', 'engine.(*sequentialAccountingCollector).exit',
    'engine.(*sequentialAccountingCollector).mark', 'engine.(*sequentialAccountingCollector).finish',
    'report.ProjectSequentialMetrics',
)]
MANIFEST = Path(__file__).with_name('sequential-arithmetic-schedules.json')
ADMISSION = {
    'dslSequentialLegacySetup9': ('e2995b9057bc091ce5061bb66d5a06b35c33ebd621a8bef4e00f4496d774dc44', 'seq.legacy.setup9.v1', None, 'XAUUSD', '1h'),
    'dslSequentialLegacySetup9PerfSeasonal': ('f16afb623842d05ef85db179e25138bf75ac1778396a4cb72563333e043658eb', 'seq.legacy.setup9_perf_seasonal.v1', None, 'XAUUSD', '1h'),
    'dslSequentialFullE1': ('6ffd40ee4ffc3ba5c606a1e23e203757972e5742263a694fcc373de8d92cf8ac', 'seq.full.public_approx.v1', 'E1', 'SYNTH', '5m'),
    'dslSequentialFullE2': ('94086f5b62ed6abd89265754572f838e3c024226666d99ceb730f4537e0c8ec2', 'seq.full.public_approx.v1', 'E2', 'SYNTH', '5m'),
}


def rat(bits):
    """Decode finite binary64 exactly; zero sign is tracked by operations."""
    exponent, significand = (bits >> 52) & 2047, bits & FRAC
    require(exponent != 2047, 'nonfinite binary64 operand')
    if exponent:
        significand |= 1 << 52
        shift = exponent - 1023 - 52
    else:
        shift = -1074
    value = Fraction(significand << shift) if shift >= 0 else Fraction(significand, 1 << -shift)
    return -value if bits & SIGN else value


def rn(value, negative_zero=False):
    """Round rational once, ties to even. Overflow is a qualification refusal."""
    if not value:
        return SIGN if negative_zero else 0
    sign = SIGN if value < 0 else 0
    value = abs(value)
    numerator, denominator = value.numerator, value.denominator
    exponent = numerator.bit_length() - denominator.bit_length()
    below = numerator < denominator << exponent if exponent >= 0 else numerator << -exponent < denominator
    if below:
        exponent -= 1
    shift = max(exponent - 52, -1074)
    if shift >= 0:
        denominator <<= shift
    else:
        numerator <<= -shift
    significand, remainder = divmod(numerator, denominator)
    if 2 * remainder > denominator or (2 * remainder == denominator and significand & 1):
        significand += 1
    if exponent < -1022:
        require(significand <= 1 << 52, 'invalid subnormal rounding')
        return sign | significand
    if significand == 1 << 53:
        significand >>= 1
        exponent += 1
    require(exponent <= 1023, 'nonfinite rounded reference result')
    return sign | ((exponent + 1023) << 52) | (significand - (1 << 52))


def neg(a):
    return a ^ SIGN


def add(a, b):
    return rn(rat(a) + rat(b), a == SIGN and b == SIGN)


def sub(a, b):
    return add(a, neg(b))


def mul(a, b):
    return rn(rat(a) * rat(b), bool((a ^ b) & SIGN))


def div(a, b):
    require(rat(b) != 0, 'zero reference divisor')
    return rn(rat(a) / rat(b), bool((a ^ b) & SIGN))


def fma(a, b, c):
    product = rat(a) * rat(b)
    return rn(product + rat(c), not product and bool((a ^ b) & SIGN) and c == SIGN)


def integer(value):
    return rn(Fraction(value))


def normalize(bits):
    return 0 if rat(bits) == 0 else bits


def category(bits):
    value = rat(bits)
    return ('-zero' if bits & SIGN else '+zero') if not value else ('positive' if value > 0 else 'negative')


def float_bits(value):
    require(type(value) in (int, float), 'expected JSON number')
    try:
        bits = int.from_bytes(struct.pack('>d', value), 'big')
    except (OverflowError, struct.error) as error:
        raise ProofError('nonfinite or invalid JSON number') from error
    rat(bits)
    return bits


def sha(value):
    return hashlib.sha256(value).hexdigest()


def decode(text):
    def object_pairs(pairs):
        obj = {}
        for key, value in pairs:
            require(key not in obj, 'duplicate JSON key ' + key)
            obj[key] = value
        return obj
    def constant(value):
        raise ProofError('nonfinite JSON constant ' + value)
    # Go encodes negative zero as -0, not necessarily -0.0.
    return json.loads(text, object_pairs_hook=object_pairs, parse_constant=constant,
                      parse_int=lambda value: -0.0 if value == '-0' else int(value))


COSTS = dict(feePerUnit='float', fillOn='string', slippage='float',
             slippageBps='float?', startEquity='float')
SHAPE = {
    'accounting': {
        'profile': 'string', 'policy': 'string?', 'startEquity': 'float',
        'endEquity': 'float', 'finalRealized': 'float',
        'marks': [dict(index='int', t='float', realized='float', unrealized='float', equity='float')],
        'trades': [dict(tradeIndex='int', entryIndex='int', exitIndex='int', entryT='float',
                        exitT='float', side='string', entry='float', exit='float', size='float',
                        points='float', entryFee='float', exitFee='float', exitCredit='float', netPnl='float')],
        'events': [dict(kind='string', tradeIndex='int', index='int', t='float',
                        realizedBefore='float', realizedAfter='float', amount='float')],
    },
    'metrics': {
        'basis': 'string',
        'headline': dict(startEquity='float', endEquity='float', net='float', returnPct='float',
                         trades='int', winRate='nullable-float', winRateReason='string?',
                         profitFactor='nullable-float', profitFactorReason='string?',
                         expectancy='nullable-float', expectancyReason='string?',
                         avgWin='nullable-float', avgWinReason='string?', avgLoss='nullable-float',
                         avgLossReason='string?', worstLoss='float', maxDD='float', maxDDpct='float',
                         maxWinStreak='int', maxLossStreak='int', avgHoldBars='nullable-float',
                         avgHoldBarsReason='string?'),
        'tradeAccounting': [dict(index='int', entryFee='float', exitFee='float', netPnl='float')],
        'equity': [dict(index='int', t='float', equity='float')],
    },
    'operands': dict(normalizedCosts=COSTS, bars=[['float']]),
}


def same(a, b, path='$'):
    """Strict recursive equality, including numeric type category and signed zero."""
    if type(a) in (int, float) and type(b) in (int, float):
        require(float_bits(a) == float_bits(b), 'exact number differs at ' + path)
    elif type(a) is dict:
        require(type(b) is dict and a.keys() == b.keys(), 'exact object keys differ at ' + path)
        for key in a:
            same(a[key], b[key], path + '.' + key)
    elif type(a) is list:
        require(type(b) is list and len(a) == len(b), 'exact array differs at ' + path)
        for index, (x, y) in enumerate(zip(a, b)):
            same(x, y, f'{path}[{index}]')
    else:
        require(type(a) is type(b) and a == b, 'exact value differs at ' + path)


def validate_capture(data):
    require(type(data) is dict and set(data) == {'schema', 'fixtureSha256', 'dslSha256', 'run',
            'accounting', 'metrics', 'operands', 'float64bits'}, 'raw envelope keys')
    require(data['schema'] == 'sequential-raw-accounting-diagnostic-v1', 'unknown raw schema')
    for key in ('fixtureSha256', 'dslSha256'):
        require(type(data[key]) is str and re.fullmatch('[0-9a-f]{64}', data[key]), 'invalid identity hash')
    values = {}
    def visit(value, shape, path):
        if type(shape) is dict:
            require(type(value) is dict and value.keys() <= shape.keys(), 'unknown/type raw field at ' + path)
            for key, subtype in shape.items():
                optional = type(subtype) is str and subtype.endswith('?')
                if key not in value:
                    require(optional, 'missing raw field ' + path + '.' + key)
                    continue
                visit(value[key], subtype[:-1] if optional else subtype, path + '.' + key)
        elif type(shape) is list:
            require(type(value) is list, 'expected raw array at ' + path)
            for index, item in enumerate(value):
                visit(item, shape[0], f'{path}[{index}]')
        elif shape in ('float', 'nullable-float'):
            if shape == 'nullable-float' and value is None:
                return
            values[path] = float_bits(value)
        elif shape == 'int':
            require(type(value) is int and abs(value) < (1 << 53), 'expected safe raw integer at ' + path)
        else:
            require(shape == 'string' and type(value) is str, 'expected raw string at ' + path)
    for key, shape in SHAPE.items():
        visit(data[key], shape, '$.' + key)
    bits = data['float64bits']
    require(type(bits) is dict and bits.keys() == values.keys(), 'missing/extra Float64bits evidence')
    for path, value in values.items():
        require(type(bits[path]) is str and re.fullmatch('[0-9a-f]{16}', bits[path]), 'invalid bits at ' + path)
        require(int(bits[path], 16) == value, 'JSON/Float64bits disagreement at ' + path)
    a, m, run = data['accounting'], data['metrics'], data['run']
    bars, count = data['operands']['bars'], len(a['trades'])
    require(0 < len(bars) <= 500000, 'invalid raw bar count')
    require(type(run) is dict and type(run.get('trades')) is list and
            type(run.get('tradeCount')) is int and run['tradeCount'] == len(run['trades']) == count,
            'raw run trade count')
    require(count == m['headline']['trades'] == len(m['tradeAccounting']), 'raw metric trade count')
    require(len(a['events']) == 2 * count, 'raw event count')
    require(len(a['marks']) == len(m['equity']) == len(bars), 'raw mark count')
    require(m['basis'] == 'sequential-broker-close-mtm-v1', 'unknown metric basis')
    require(a['profile'] in ('seq.legacy.setup9.v1', 'seq.legacy.setup9_perf_seasonal.v1', 'seq.full.public_approx.v1'),
            'unknown accounting profile')
    require((a.get('policy') in ('E1', 'E2')) if a['profile'] == 'seq.full.public_approx.v1' else 'policy' not in a,
            'invalid accounting policy')
    for index, row in enumerate(bars):
        require(len(row) == 6, 'expected six bar operands')
        require(row[0] >= 0 and (not index or row[0] > bars[index - 1][0]), 'bar time ordering')
        require(a['marks'][index]['index'] == m['equity'][index]['index'] == index, 'mark identity/order')
    for index, trade in enumerate(a['trades']):
        require(trade['tradeIndex'] == m['tradeAccounting'][index]['index'] == index, 'trade identity/order')
        require(0 <= trade['entryIndex'] <= trade['exitIndex'] < len(bars), 'trade index bounds')
        require(trade['side'] in ('long', 'short') and trade['size'] >= 0, 'trade side/size')
        for key in ('entryIndex', 'exitIndex', 'entryT', 'exitT', 'side'):
            same(trade[key], run['trades'][index].get(key), 'raw/run trade.' + key)
    require(a['events'] == sorted(a['events'], key=lambda event: event['index']), 'event ordering')
    for field in ('winRate', 'profitFactor', 'expectancy', 'avgWin', 'avgLoss', 'avgHoldBars'):
        require((field + 'Reason' in m['headline']) == (m['headline'][field] is None), 'nullable reason completeness')
    return values


class Replay:
    def __init__(self, data, target):
        require(target in TARGETS, 'unknown target schedule ' + str(target))
        self.data, self.mode = data, TARGETS[target]
        self.bits = validate_capture(data)
        self.expected = {}

    def get(self, path):
        return self.bits['$.' + path]

    def put(self, path, bits, projected=False):
        if projected:
            bits = normalize(bits)
        require(self.get(path) == bits, f'exact replay mismatch at $.{path}: actual {self.get(path):016x}, predicted {bits:016x}')
        self.expected['$.' + path] = bits
        return bits

    def run(self):
        a, m = self.data['accounting'], self.data['metrics']
        fee, start = self.get('operands.normalizedCosts.feePerUnit'), self.get('operands.normalizedCosts.startEquity')
        require(rat(start) > 0 and rat(fee) >= 0, 'invalid effective start/fee')
        self.put('accounting.startEquity', start)
        realized, states, nets = 0, [], []
        for index, trade in enumerate(a['trades']):
            path = f'accounting.trades[{index}]'
            side, size = integer(1 if trade['side'] == 'long' else -1), self.get(path + '.size')
            points = mul(sub(self.get(path + '.exit'), self.get(path + '.entry')), side)
            self.put(path + '.points', points)
            nominal = mul(fee, size)
            self.put(path + '.entryFee', nominal)
            self.put(path + '.exitFee', nominal)
            entry, exit_event = a['events'][2 * index:2 * index + 2]
            require(entry['kind'] == 'entry' and exit_event['kind'] == 'exit' and
                    entry['tradeIndex'] == exit_event['tradeIndex'] == index, 'event identity/kind')
            require(entry['index'] == trade['entryIndex'] and exit_event['index'] == trade['exitIndex'], 'event indices')
            ep, xp = f'accounting.events[{2 * index}]', f'accounting.events[{2 * index + 1}]'
            for event_path, key in ((ep, 'entry'), (xp, 'exit')):
                timestamp = self.get(f'operands.bars[{trade[key + "Index"]}][0]')
                self.put(event_path + '.t', timestamp)
                self.put(path + '.' + key + 'T', timestamp)
            self.put(ep + '.realizedBefore', realized)
            self.put(ep + '.amount', nominal)
            realized = fma(neg(fee), size, realized) if self.mode == 'fused' else sub(realized, nominal)
            self.put(ep + '.realizedAfter', realized)
            states.append(realized)
            gross = mul(points, size)
            credit = fma(neg(fee), size, gross) if self.mode == 'fused' else sub(gross, nominal)
            self.put(path + '.exitCredit', credit)
            net = self.put(path + '.netPnl', sub(credit, nominal))
            nets.append(net)
            self.put(xp + '.realizedBefore', realized)
            self.put(xp + '.amount', credit)
            realized = self.put(xp + '.realizedAfter', add(realized, credit))
            states.append(realized)
        self.put('accounting.finalRealized', realized)
        self.put('accounting.endEquity', add(start, realized))
        equities, event_index, current, held = [], 0, 0, None
        for index, mark in enumerate(a['marks']):
            path = f'accounting.marks[{index}]'
            timestamp = self.put(path + '.t', self.get(f'operands.bars[{index}][0]'))
            while event_index < len(a['events']) and a['events'][event_index]['index'] <= index:
                event = a['events'][event_index]
                current = states[event_index]
                held = event['tradeIndex'] if event['kind'] == 'entry' else None
                event_index += 1
            self.put(path + '.realized', current)
            unrealized = 0
            if held is not None:
                trade, tp = a['trades'][held], f'accounting.trades[{held}]'
                difference = sub(self.get(f'operands.bars[{index}][4]'), self.get(tp + '.entry'))
                points = mul(difference, integer(1 if trade['side'] == 'long' else -1))
                unrealized = mul(points, self.get(tp + '.size'))
            self.put(path + '.unrealized', unrealized)
            equity = self.put(path + '.equity', add(add(start, current), unrealized))
            equities.append(equity)
            self.put(f'metrics.equity[{index}].t', timestamp, True)
            self.put(f'metrics.equity[{index}].equity', equity, True)
        require(event_index == len(a['events']) and held is None, 'terminal event/position')
        h = 'metrics.headline.'
        for key, value in (('startEquity', start), ('endEquity', add(start, realized)), ('net', realized),
                           ('returnPct', mul(div(realized, start), integer(100)))):
            self.put(h + key, value, True)
        peak, max_dd, max_pct = start, 0, 0
        for equity in equities:
            if rat(equity) > rat(peak):
                peak = equity
            dd = sub(peak, equity)
            pct = mul(div(dd, peak), integer(100))
            if rat(dd) > rat(max_dd):
                max_dd = dd
            if rat(pct) > rat(max_pct):
                max_pct = pct
        self.put(h + 'maxDD', max_dd, True)
        self.put(h + 'maxDDpct', max_pct, True)
        gross_win = gross_loss = hold = worst = 0
        wins = win_streak = loss_streak = max_win_streak = max_loss_streak = 0
        for index, (trade, net) in enumerate(zip(a['trades'], nets)):
            for key in ('entryFee', 'exitFee', 'netPnl'):
                self.put(f'metrics.tradeAccounting[{index}].{key}', self.expected[f'$.accounting.trades[{index}].{key}'], True)
            hold = add(hold, integer(trade['exitIndex'] - trade['entryIndex']))
            if rat(net) > 0:
                gross_win = add(gross_win, net)
                wins += 1
                win_streak += 1
                loss_streak = 0
                max_win_streak = max(max_win_streak, win_streak)
            else:
                gross_loss = sub(gross_loss, net)
                loss_streak += 1
                win_streak = 0
                max_loss_streak = max(max_loss_streak, loss_streak)
                if rat(net) < rat(worst):
                    worst = net
        self.put(h + 'worstLoss', worst, True)
        require(m['headline']['maxWinStreak'] == max_win_streak and
                m['headline']['maxLossStreak'] == max_loss_streak, 'streak replay mismatch')
        if nets:
            count = integer(len(nets))
            self.put(h + 'winRate', mul(div(integer(wins), count), integer(100)), True)
            self.put(h + 'expectancy', div(realized, count), True)
            self.put(h + 'avgHoldBars', div(hold, count), True)
            if wins:
                self.put(h + 'avgWin', div(gross_win, integer(wins)), True)
            if len(nets) > wins:
                self.put(h + 'avgLoss', div(neg(gross_loss), integer(len(nets) - wins)), True)
            if rat(gross_loss) > 0:
                self.put(h + 'profitFactor', div(gross_win, gross_loss), True)
            elif not rat(gross_win):
                self.put(h + 'profitFactor', 0, True)
        nulls = {}
        if not nets:
            nulls.update(winRate='no-trades', profitFactor='no-trades', expectancy='no-trades', avgHoldBars='no-trades')
        elif rat(gross_loss) <= 0 and rat(gross_win) > 0:
            nulls['profitFactor'] = 'no-losses'
        if not wins:
            nulls['avgWin'] = 'no-winning-trades'
        if len(nets) == wins:
            nulls['avgLoss'] = 'no-nonwinning-trades'
        for key in ('winRate', 'profitFactor', 'expectancy', 'avgHoldBars', 'avgWin', 'avgLoss'):
            require((m['headline'][key] is None) == (key in nulls) and
                    m['headline'].get(key + 'Reason') == nulls.get(key), 'null/reason replay mismatch at ' + key)
        # The expected-map complement must contain ONLY fixed upstream operands.
        upstream = {p for p in self.bits if p.startswith('$.operands.') or re.fullmatch(
            r'\$\.accounting\.trades\[\d+\]\.(entry|exit|size)', p)}
        require(self.bits.keys() == self.expected.keys() | upstream, 'unreplayed downstream float field')
        self.categories = dict(nulls=nulls, tradeSigns=[category(value) for value in nets],
                               accountRawSign=category(realized), maxWinStreak=max_win_streak,
                               maxLossStreak=max_loss_streak, wins=wins)
        return self


def bind_companion(capture, companion, fixture_text, source):
    require(type(companion) is dict and set(companion) == {'schema', 'contractVersion', 'identity', 'capabilities', 'run', 'metrics'},
            'companion envelope keys')
    require(companion['schema'] == 'strat-sequential-backtest-result-v1' and
            type(companion['contractVersion']) is int and companion['contractVersion'] == 1, 'companion schema/version')
    same(capture['run'], companion['run'], '$.run')
    same(capture['metrics'], companion['metrics'], '$.metrics')
    identity, fixture = companion['identity'], decode(fixture_text)
    for field, value in (('fixtureSha256', sha(fixture_text.encode())), ('dslSha256', sha(source.encode()))):
        require(capture[field] == identity[field] == value, 'fixture/source identity differs')
    require(fixture['strategyId'] in ADMISSION, 'unknown admitted source/strategy')
    digest, profile, policy, symbol, timeframe = ADMISSION[fixture['strategyId']]
    require(identity['dslSha256'] == digest and identity['profile'] == profile and identity.get('policy') == policy and
            identity['symbol'] == symbol and identity['timeframe'] == timeframe, 'source/profile/policy/route binding')
    # Costs.normalized uses only these defaults. SlippageBps has Go omitempty
    # semantics, including -0. All retained floats keep their original bits.
    costs = fixture['costs']
    require(type(costs) is dict and {'feePerUnit', 'fillOn', 'slippage'} <= costs.keys() <= COSTS.keys(), 'original fixture costs shape')
    require(type(costs['fillOn']) is str, 'original fillOn type')
    for key in costs.keys() - {'fillOn'}:
        float_bits(costs[key])
    normalized_costs = {key: costs[key] for key in ('feePerUnit', 'fillOn', 'slippage')}
    normalized_costs['fillOn'] = costs['fillOn'] or 'close'
    normalized_costs['startEquity'] = costs.get('startEquity', 0) or 10000
    if costs.get('slippageBps', 0) != 0:
        normalized_costs['slippageBps'] = costs['slippageBps']
    same(capture['operands']['normalizedCosts'], normalized_costs, '$.original-normalized-costs')
    same(capture['operands']['bars'], fixture['bars'], '$.operands.bars')
    same(capture['operands']['normalizedCosts'], identity['costs'], '$.identity.costs')
    require(identity['barCount'] == len(fixture['bars']) and identity['firstBarMs'] == fixture['bars'][0][0] and
            identity['lastBarMs'] == fixture['bars'][-1][0], 'companion bar identity')
    for key in ('strategyId', 'symbol', 'timeframe'):
        same(identity[key], fixture[key], '$.identity.' + key)
    for key in ('profile', 'policy'):
        same(capture['accounting'].get(key), identity.get(key), '$.identity.' + key)


def verify_pair(request, target):
    require(target in ('linux-amd64', 'linux-arm64'), 'unsupported native arithmetic target')
    require(type(request) is dict and set(request) == {'nativeRaw', 'wasmRaw', 'nativeCompanion', 'wasmCompanion', 'fixture', 'source'} and
            all(type(value) is str for value in request.values()), 'pair request shape')
    native, wasm = decode(request['nativeRaw']), decode(request['wasmRaw'])
    nc, wc = decode(request['nativeCompanion']), decode(request['wasmCompanion'])
    nr, wr = Replay(native, target).run(), Replay(wasm, 'js-wasm').run()
    for capture, companion in ((native, nc), (wasm, wc)):
        bind_companion(capture, companion, request['fixture'], request['source'])
    same({key: value for key, value in nc.items() if key != 'metrics'},
         {key: value for key, value in wc.items() if key != 'metrics'}, '$companion')
    same(nr.categories, wr.categories, '$categories')
    # These are upstream boundary operands, even if points/timestamps were also
    # independently replayed. No target schedule can excuse their inequality.
    for path in nr.bits:
        if path.startswith('$.operands.') or path == '$.accounting.startEquity' or re.fullmatch(
                r'\$\.accounting\.trades\[\d+\]\.(entryT|exitT|entry|exit|size|points)', path):
            require(nr.bits[path] == wr.bits[path], 'unequal upstream operand ' + path)
    numeric_leaves, bit_leaves = [], []
    def walk(a, b, path='$'):
        if type(a) is dict:
            require(type(b) is dict and a.keys() == b.keys(), 'unequal raw keys at ' + path)
            for key in a:
                walk(a[key], b[key], path + '.' + key)
        elif type(a) is list:
            require(type(b) is list and len(a) == len(b), 'unequal raw array at ' + path)
            for index, (x, y) in enumerate(zip(a, b)):
                walk(x, y, f'{path}[{index}]')
        elif type(a) in (int, float):
            require(type(b) in (int, float), 'unequal numeric type at ' + path)
            ab, bb = float_bits(a), float_bits(b)
            if ab == bb:
                return
            require(path in nr.expected and path in wr.expected, 'unexplained numeric leaf ' + path)
            require(ab == nr.expected[path] and bb == wr.expected[path], 'unpredicted numeric leaf ' + path)
            require(category(ab) == category(bb), 'numeric sign/category mismatch at ' + path)
            numeric_leaves.append(dict(path=path, nativeBits=f'{ab:016x}', wasmBits=f'{bb:016x}'))
        elif type(a) is type(b) and a == b:
            return
        elif path.startswith('$.float64bits.$.'):
            key = path[len('$.float64bits.'):]
            require(key in nr.expected and key in wr.expected and a == f'{nr.expected[key]:016x}' and
                    b == f'{wr.expected[key]:016x}', 'unexplained raw bit leaf ' + path)
            bit_leaves.append(path)
        else:
            raise ProofError('unexplained categorical/unknown leaf ' + path)
    walk(native, wasm)
    require(len(numeric_leaves) == len(bit_leaves), 'unequal numeric/bit evidence counts')
    return dict(schema='sequential-exact-arithmetic-proof-v1', qualificationPass=True,
                nativeTarget=target, wasmTarget='js-wasm', captures=2,
                exactArithmeticChecks=len(nr.expected) + len(wr.expected),
                completeFloatBitChecks=len(nr.bits) + len(wr.bits), categoriesExact=True,
                rawCrossTargetExact=request['nativeRaw'] == request['wasmRaw'],
                explainedNumericLeaves=numeric_leaves, explainedBitLeaves=bit_leaves)


def source_hashes(root):
    # Include ignored/platform-specific compiler input candidates as well as
    # the currently selected closure. Adding assembly, cgo, object archives or
    # workspace configuration cannot silently escape the source inventory.
    suffixes = {'.go', '.s', '.S', '.syso', '.c', '.h', '.cc', '.cpp', '.cxx',
                '.m', '.f', '.f90', '.swig', '.swigcxx', '.o', '.a'}
    files = sorted(path for path in root.rglob('*') if path.is_file() and
                   (path.suffix in suffixes or path.name in ('go.mod', 'go.sum', 'go.work', 'go.work.sum')) and
                   not path.name.endswith('_test.go') and
                   not any(part.startswith('.') for part in path.relative_to(root).parts))
    return {str(path.relative_to(root)): sha(path.read_bytes()) for path in files}


def dependency_inputs(listing, root):
    decoder, packages, inputs = json.JSONDecoder(), [], {}
    fields = ('GoFiles', 'CgoFiles', 'CFiles', 'CXXFiles', 'MFiles', 'HFiles',
              'FFiles', 'SFiles', 'SwigFiles', 'SwigCXXFiles', 'SysoFiles', 'EmbedFiles')
    while listing.strip():
        package, end = decoder.raw_decode(listing.lstrip())
        listing = listing.lstrip()[end:]
        require(not package.get('Error') and not package.get('DepsErrors'), 'incomplete build dependency listing')
        if package.get('Standard'):
            continue
        module = package.get('Module', {})
        require(module.get('Main') is True and module.get('Path') == MODULE[:-1] and
                not module.get('Replace') and Path(module['Dir']).resolve() == root,
                'unreviewed external/replaced build module')
        packages.append(package['ImportPath'])
        for field in fields:
            for name in package.get(field, []):
                path = (Path(package['Dir']) / name).resolve()
                require(path.is_relative_to(root), 'build input outside pinned module')
                inputs[str(path.relative_to(root))] = sha(path.read_bytes())
    require(packages and inputs, 'missing build dependency closure')
    return {'packages': sorted(packages), 'files': dict(sorted(inputs.items()))}


def instruction_lists(assembly):
    """Keep complete instruction/operand streams, omit offsets/debug metadata."""
    result, active = {}, None
    for line in assembly.splitlines():
        if line and not line[0].isspace():
            active = next((name for name in FUNCTIONS if line.startswith(name + ' STEXT ')), None)
            if active:
                require(active not in result, 'duplicate codegen function ' + active)
                result[active] = []
        elif active:
            match = re.match(r'\s*0x[0-9a-f]+\s+\d+\s+\(.*\)\t([^\t]+)(?:\t(.*))?$', line)
            if match and match[1] not in ('PCDATA', 'FUNCDATA'):
                result[active].append(match[1] + ('\t' + match[2] if match[2] else ''))
    require(set(result) == set(FUNCTIONS) and all(result.values()), 'missing/unknown compiler instruction format')
    return result


def validate_manifest(manifest, root):
    require(set(manifest) == {'schema', 'goVersion', 'python', 'sourceParents', 'build', 'sources', 'targets'}, 'schedule manifest keys')
    require(manifest['schema'] == 'sequential-pinned-arithmetic-schedules-v1' and manifest['goVersion'] == 'go1.22.12' and
            manifest['python'] == 'CPython 3.12.x', 'unknown compiler/manifest version')
    same(manifest['build'], {'CGO_ENABLED': '0', 'flags': ['-buildvcs=false', '-trimpath'],
                            'GOAMD64': 'v1', 'GOEXPERIMENT': '', 'GOFLAGS': '', 'GOWORK': 'off'}, '$manifest.build')
    require(manifest['sources'] == source_hashes(root), 'unknown/unreviewed working source; arithmetic pin must be reviewed')
    require(set(manifest['targets']) == set(TARGETS), 'missing/extra target schedules')
    for target, pin in manifest['targets'].items():
        require(type(pin) is dict and set(pin) == {'accountingSchedule', 'instructions', 'buildInputs'} and
                pin['accountingSchedule'] == TARGETS[target], 'unknown accounting schedule ' + target)
        require(type(pin['instructions']) is dict and set(pin['instructions']) == set(FUNCTIONS), 'incomplete instruction pins')
        for entry in pin['instructions'].values():
            require(type(entry) is dict and set(entry) == {'sha256', 'instructionCount'} and
                    type(entry['sha256']) is str and re.fullmatch('[0-9a-f]{64}', entry['sha256']) and
                    type(entry['instructionCount']) is int and entry['instructionCount'] > 0, 'invalid instruction pin')


def verify_instructions(pin, observed, target):
    require(pin == observed, 'unknown/unreviewed compiler instruction schedule ' + target)


def check_codegen(root, go, output):
    manifest = decode(MANIFEST.read_text())
    validate_manifest(manifest, root)
    env = os.environ.copy()
    require(not any(env.get(key) for key in ('GOCOMPILEDEBUG', 'GOEXPERIMENT', 'GOFLAGS', 'GODEBUG', 'GOWASM', 'GOARM64')),
            'unsupported compiler/runtime options')
    require(env.get('GOAMD64', 'v1') == 'v1', 'unsupported GOAMD64 feature level')
    env.update(CGO_ENABLED='0', GOTOOLCHAIN='local', GOPROXY='off', GOWORK='off')
    def run(args, target_env=None):
        result = subprocess.run([go] + args, cwd=root, env=env | (target_env or {}), text=True, capture_output=True)
        require(result.returncode == 0, 'pinned Go command failed: ' + result.stderr[:1000])
        return result
    config = decode(run(['env', '-json', 'GOVERSION', 'GOARCH', 'GOHOSTARCH', 'GOOS', 'GOHOSTOS', 'GOAMD64', 'GOEXPERIMENT', 'GOFLAGS', 'GOWASM']).stdout)
    machine = {'x86_64': 'amd64', 'aarch64': 'arm64'}.get(platform.machine())
    require(config['GOVERSION'] == 'go1.22.12' and config['GOOS'] == config['GOHOSTOS'] == 'linux' and
            config['GOARCH'] == config['GOHOSTARCH'] == machine and machine in ('amd64', 'arm64'), 'unsupported Go compiler/native host')
    require(config['GOAMD64'] in ('', 'v1') and not config['GOEXPERIMENT'] and not config['GOFLAGS'] and not config['GOWASM'],
            'unsupported Go build configuration')
    receipts = {}
    with tempfile.TemporaryDirectory(prefix='sequential-codegen-') as scratch:
        for target in ('linux-' + machine, 'js-wasm'):
            pin = manifest['targets'][target]
            require(set(pin) == {'accountingSchedule', 'instructions', 'buildInputs'} and pin['accountingSchedule'] == TARGETS[target],
                    'unknown accounting schedule ' + target)
            goos, arch = target.split('-')
            dependencies = run(['list', '-buildvcs=false', '-trimpath', '-deps', '-json', './cmd/enginewasm', './cmd/dslwasm', './engine', './report'],
                               {'GOOS': goos, 'GOARCH': arch})
            require(pin['buildInputs'] == dependency_inputs(dependencies.stdout, root),
                    'unknown/unreviewed compile dependency closure ' + target)
            built = run(['build', '-buildvcs=false', '-trimpath', '-gcflags=' + MODULE + 'engine=-S',
                         '-gcflags=' + MODULE + 'report=-S', '-o', str(Path(scratch) / target), './cmd/enginewasm'],
                        {'GOOS': goos, 'GOARCH': arch})
            listings = instruction_lists(built.stderr)
            observed = {name: {'sha256': sha(('\n'.join(lines) + '\n').encode()), 'instructionCount': len(lines)}
                        for name, lines in listings.items()}
            verify_instructions(pin['instructions'], observed, target)
            receipts[target] = {'instructions': observed, 'buildInputCount': len(pin['buildInputs']['files']),
                                'buildInputsSha256': sha(json.dumps(pin['buildInputs'], sort_keys=True, separators=(',', ':')).encode())}
            if output:
                for name, lines in listings.items():
                    filename = name.removeprefix(MODULE).replace('/', '-').replace('(*', '').replace(')', '')
                    (output / (target + '.' + filename + '.instructions.txt')).write_text('\n'.join(lines) + '\n')
    return dict(schema='sequential-codegen-verification-v1', goVersion=config['GOVERSION'],
                nativeTarget='linux-' + machine, python=platform.python_version(),
                manifestSha256=sha(MANIFEST.read_bytes()), sourceParents=manifest['sourceParents'],
                sourceCount=len(manifest['sources']), targets=receipts)


def main():
    supported_python()
    if len(sys.argv) == 3 and sys.argv[1] == 'pair':
        result = verify_pair(decode(sys.stdin.read()), sys.argv[2])
    elif len(sys.argv) in (4, 5) and sys.argv[1] == 'codegen':
        output = Path(sys.argv[4]) if len(sys.argv) == 5 else None
        if output:
            output.mkdir(parents=True, exist_ok=True)
        result = check_codegen(Path(sys.argv[2]).resolve(), sys.argv[3], output)
    else:
        raise ProofError('usage: sequential-accounting-proof.py pair <linux-amd64|linux-arm64> | codegen <repo> <go> [output]')
    print(json.dumps(result, separators=(',', ':'), allow_nan=False))


if __name__ == '__main__':
    try:
        main()
    except (ProofError, KeyError, IndexError, TypeError, ValueError, OSError) as error:
        print('Exact Sequential arithmetic proof FAILED: ' + str(error), file=sys.stderr)
        sys.exit(1)
