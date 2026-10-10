#!/usr/bin/env python3
"""Synthetic tests for the independent exact arithmetic qualification helper.

CPython 3.12.x, standard library only. Tiny hand-constructed ledgers exercise
binding/replay; arithmetic tests use rational identities and binary64 boundary
vectors rather than captured engine output. These are not production goldens.
"""
import copy
from fractions import Fraction
import importlib.util
import json
from pathlib import Path
import random
import struct
import sys
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('sequential_accounting_proof', Path(__file__).with_name('sequential-accounting-proof.py'))
proof = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(proof)
ROOT = Path(__file__).resolve().parents[2]


def value(bits):
    return struct.unpack('>d', bits.to_bytes(8, 'big'))[0]


def refresh_bits(capture):
    result = {}
    integers = {'index', 'tradeIndex', 'entryIndex', 'exitIndex', 'trades', 'maxWinStreak', 'maxLossStreak'}
    def walk(item, path, key=''):
        if type(item) is dict:
            for field, subvalue in item.items():
                walk(subvalue, path + '.' + field, field)
        elif type(item) is list:
            for index, subvalue in enumerate(item):
                walk(subvalue, f'{path}[{index}]')
        elif type(item) in (int, float) and key not in integers:
            result[path] = f'{proof.float_bits(item):016x}'
    for section in ('accounting', 'metrics', 'operands'):
        walk(capture[section], '$.' + section)
    capture['float64bits'] = result


def request_for(closes=(100, 110), fee=1, no_trades=False):
    """Simple unit-size ledger, independently computed with Python's operators."""
    source = (ROOT / 'conformance/run/family-sequential-full-e1-long.strat').read_text()
    bars = [[index * 300000, close, close, close, close, 1] for index, close in enumerate(closes)]
    costs = dict(feePerUnit=fee, fillOn='close', slippage=0, startEquity=100)
    fixture = dict(schema='dsl-conformance-run-fixture-v1', case='synthetic-reference-unit-test',
                   strategyId='dslSequentialFullE1', symbol='SYNTH', timeframe='5m', rangeMethod='zone',
                   higherTimeframe=None, costs=costs, bars=bars)
    fixture_text = json.dumps(fixture)
    realized = 0
    trades, events = [], []
    if not no_trades:
        for index, (entry, exit_price) in enumerate(zip(closes, closes[1:])):
            before = realized
            realized -= fee
            points, credit = exit_price - entry, exit_price - entry - fee
            trade = dict(tradeIndex=index, entryIndex=index, exitIndex=index + 1, entryT=bars[index][0],
                         exitT=bars[index + 1][0], side='long', entry=entry, exit=exit_price, size=1,
                         points=points, entryFee=fee, exitFee=fee, exitCredit=credit, netPnl=credit - fee)
            trades.append(trade)
            events.append(dict(kind='entry', tradeIndex=index, index=index, t=bars[index][0],
                               realizedBefore=before, realizedAfter=realized, amount=fee))
            before = realized
            realized += credit
            events.append(dict(kind='exit', tradeIndex=index, index=index + 1, t=bars[index + 1][0],
                               realizedBefore=before, realizedAfter=realized, amount=credit))
    marks = []
    for index, bar in enumerate(bars):
        available = [event for event in events if event['index'] <= index]
        current = available[-1]['realizedAfter'] if available else 0
        marks.append(dict(index=index, t=bar[0], realized=current, unrealized=0, equity=100 + current))
    nets = [trade['netPnl'] for trade in trades]
    winners = [net for net in nets if net > 0]
    losers = [net for net in nets if net <= 0]
    gross_win, gross_loss = sum(winners), -sum(losers)
    peak, max_dd, max_pct = 100, 0, 0
    for mark in marks:
        peak = max(peak, mark['equity'])
        dd = peak - mark['equity']
        max_dd, max_pct = max(max_dd, dd), max(max_pct, dd / peak * 100)
    ws = ls = mws = mls = 0
    for net in nets:
        if net > 0:
            ws += 1
            ls = 0
        else:
            ls += 1
            ws = 0
        mws, mls = max(mws, ws), max(mls, ls)
    h = dict(startEquity=100, endEquity=100 + realized, net=realized, returnPct=realized / 100 * 100,
             trades=len(trades), winRate=len(winners) / len(nets) * 100 if nets else None,
             profitFactor=(gross_win / gross_loss if gross_loss else (None if gross_win else 0)) if nets else None,
             expectancy=realized / len(nets) if nets else None,
             avgWin=gross_win / len(winners) if winners else None,
             avgLoss=-gross_loss / len(losers) if losers else None,
             worstLoss=min([0] + nets), maxDD=max_dd, maxDDpct=max_pct, maxWinStreak=mws, maxLossStreak=mls,
             avgHoldBars=1 if nets else None)
    nulls = {}
    if not nets:
        nulls.update(winRate='no-trades', profitFactor='no-trades', expectancy='no-trades', avgHoldBars='no-trades')
    elif gross_win and not gross_loss:
        nulls['profitFactor'] = 'no-losses'
    if not winners:
        nulls['avgWin'] = 'no-winning-trades'
    if not losers:
        nulls['avgLoss'] = 'no-nonwinning-trades'
    h.update({key + 'Reason': reason for key, reason in nulls.items()})
    # The existing metric projection normalizes only projected zero values.
    if h['avgLoss'] == 0:
        h['avgLoss'] = 0
    metrics = dict(basis='sequential-broker-close-mtm-v1', headline=h,
                   tradeAccounting=[dict(index=index, entryFee=fee, exitFee=fee, netPnl=trade['netPnl'])
                                    for index, trade in enumerate(trades)],
                   equity=[dict(index=mark['index'], t=mark['t'], equity=mark['equity']) for mark in marks])
    run = dict(schema='dsl-conformance-trades-v1', case=fixture['case'], strategyId=fixture['strategyId'],
               symbol='SYNTH', timeframe='5m', rangeMethod='zone', costs=costs, tradeCount=len(trades),
               trades=[{key: trade[key] for key in ('entryIndex', 'exitIndex', 'entryT', 'exitT', 'side')} for trade in trades])
    capture = dict(schema='sequential-raw-accounting-diagnostic-v1', fixtureSha256=proof.sha(fixture_text.encode()),
                   dslSha256=proof.sha(source.encode()), run=run,
                   accounting=dict(profile='seq.full.public_approx.v1', policy='E1', startEquity=100,
                                   endEquity=100 + realized, finalRealized=realized, trades=trades, events=events, marks=marks),
                   metrics=metrics, operands=dict(normalizedCosts=costs, bars=bars))
    refresh_bits(capture)
    identity = dict(fixtureSha256=capture['fixtureSha256'], dslSha256=capture['dslSha256'],
                    strategyId=fixture['strategyId'], symbol='SYNTH', timeframe='5m', barCount=len(bars),
                    firstBarMs=bars[0][0], lastBarMs=bars[-1][0], costs=costs, profile='seq.full.public_approx.v1', policy='E1')
    companion = dict(schema='strat-sequential-backtest-result-v1', contractVersion=1, identity=identity,
                     capabilities=dict(headline=True, trades=True, equity=True, groupings=False, monthly=False,
                                       rDistribution=False, portfolio=False), run=run, metrics=metrics)
    return dict(nativeRaw=json.dumps(capture), wasmRaw=json.dumps(capture), nativeCompanion=json.dumps(companion),
                wasmCompanion=json.dumps(companion), fixture=fixture_text, source=source)


def mutate(request, change, both=False, bind=True):
    request = copy.deepcopy(request)
    for target in (('native', 'wasm') if both else ('native',)):
        capture = proof.decode(request[target + 'Raw'])
        companion = proof.decode(request[target + 'Companion'])
        change(capture, companion)
        if bind:
            companion['metrics'] = copy.deepcopy(capture['metrics'])
            companion['run'] = copy.deepcopy(capture['run'])
        request[target + 'Raw'] = json.dumps(capture)
        request[target + 'Companion'] = json.dumps(companion)
    return request


class ArithmeticTests(unittest.TestCase):
    def test_finite_roundtrips_and_ties(self):
        rng = random.Random(229)
        samples = [0, proof.SIGN, 1, proof.SIGN | 1, proof.FRAC, 1 << 52, proof.MAX, proof.SIGN | proof.MAX]
        samples += [(rng.getrandbits(1) << 63) | (rng.randrange(2047) << 52) | rng.getrandbits(52) for _ in range(4000)]
        for bits in samples:
            self.assertEqual(proof.rn(proof.rat(bits), bits == proof.SIGN), bits)
        for exponent in range(2047):
            for bits in (exponent << 52, (exponent << 52) + 1, (exponent << 52) + proof.FRAC - 1):
                midpoint = (proof.rat(bits) + proof.rat(bits + 1)) / 2
                expected = bits + (bits & 1)
                self.assertEqual(proof.rn(midpoint), expected)
                self.assertEqual(proof.rn(-midpoint), expected | proof.SIGN)

    def test_subnormal_underflow_and_normal_boundary(self):
        self.assertEqual(proof.rn(Fraction(1, 1 << 1075)), 0)
        self.assertEqual(proof.rn(-Fraction(1, 1 << 1075)), proof.SIGN)
        self.assertEqual(proof.rn(Fraction(3, 1 << 1075)), 2)
        self.assertEqual(proof.rn((proof.rat(proof.FRAC) + proof.rat(1 << 52)) / 2), 1 << 52)
        self.assertEqual(proof.mul(1, proof.integer(1)), 1)
        self.assertEqual(proof.div(1, proof.integer(2)), 0)

    def test_signed_zero_all_operations(self):
        p, n, one = 0, proof.SIGN, proof.integer(1)
        self.assertEqual(proof.add(n, n), n)
        self.assertEqual(proof.add(n, p), p)
        self.assertEqual(proof.sub(n, p), n)
        self.assertEqual(proof.sub(n, n), p)
        self.assertEqual(proof.mul(n, one), n)
        self.assertEqual(proof.mul(n, proof.neg(one)), p)
        self.assertEqual(proof.div(n, one), n)
        self.assertEqual(proof.fma(n, one, n), n)
        self.assertEqual(proof.fma(n, one, p), p)
        self.assertEqual(proof.fma(one, one, proof.neg(one)), p)
        self.assertEqual(proof.float_bits(proof.decode('-0')), n)
        self.assertEqual(proof.normalize(n), p)

    def test_fma_is_single_rounding(self):
        a = proof.rn(Fraction(1) + Fraction(1, 1 << 27))
        b = proof.rn(Fraction(1) - Fraction(1, 1 << 27))
        minus_one = proof.integer(-1)
        self.assertEqual(proof.add(proof.mul(a, b), minus_one), 0)
        self.assertEqual(proof.fma(a, b, minus_one), proof.rn(-Fraction(1, 1 << 54)))
        self.assertEqual(proof.fma(proof.MAX, proof.integer(2), proof.neg(proof.MAX)), proof.MAX)

    def test_overflow_and_nonfinite_refuse(self):
        for action in (lambda: proof.rn(proof.rat(proof.MAX) + Fraction(1 << 970)),
                       lambda: proof.mul(proof.MAX, proof.integer(2)),
                       lambda: proof.div(proof.integer(1), 0),
                       lambda: proof.rat(0x7ff0000000000000),
                       lambda: proof.float_bits(float('nan')), lambda: proof.decode('NaN')):
            with self.assertRaises(proof.ProofError):
                action()
        self.assertEqual(proof.rn(proof.rat(proof.MAX) + Fraction((1 << 970) - 1)), proof.MAX)

    def test_separate_operations_against_binary64(self):
        rng = random.Random(230)
        for _ in range(1000):
            a, b = [(rng.getrandbits(1) << 63) | (rng.randrange(2047) << 52) | rng.getrandbits(52) for _ in range(2)]
            av, bv = value(a), value(b)
            for operation, expected in ((proof.add, av + bv), (proof.sub, av - bv), (proof.mul, av * bv), (proof.div, av / bv)):
                if abs(expected) == float('inf'):
                    with self.assertRaises(proof.ProofError):
                        operation(a, b)
                else:
                    self.assertEqual(operation(a, b), proof.float_bits(expected))


class BindingReplayTests(unittest.TestCase):
    def setUp(self):
        self.request = request_for()

    def refused(self, request):
        with self.assertRaises((proof.ProofError, KeyError, IndexError)):
            proof.verify_pair(request, 'linux-arm64')

    def test_all_supported_schedules_and_empty_ledger(self):
        for target in ('linux-amd64', 'linux-arm64'):
            for request in (self.request, request_for(no_trades=True), request_for((100, 100), fee=0)):
                result = proof.verify_pair(request, target)
                self.assertTrue(result['qualificationPass'])
                self.assertTrue(result['rawCrossTargetExact'])
                self.assertGreater(result['exactArithmeticChecks'], 0)

    def test_independent_drawdown_maxima(self):
        request = request_for((100, 80, 200, 170), fee=0)
        result = proof.verify_pair(request, 'linux-arm64')
        self.assertTrue(result['qualificationPass'])
        headline = proof.decode(request['nativeRaw'])['metrics']['headline']
        self.assertEqual((headline['maxDD'], headline['maxDDpct']), (30, 20))

    def test_missing_extra_wrong_typed_evidence_both_targets(self):
        faults = [
            lambda c, _: c['float64bits'].pop('$.accounting.startEquity'),
            lambda c, _: c['float64bits'].update({'$.extra': '0000000000000000'}),
            lambda c, _: c['accounting'].update(extra=0),
            lambda c, _: c['accounting'].update(startEquity='100'),
            lambda c, _: c['accounting'].update(startEquity=None),
            lambda c, _: c['accounting'].pop('startEquity'),
            lambda c, _: c['accounting'].update(marks={}),
            lambda c, _: c['accounting']['marks'].pop(),
            lambda c, _: c['accounting']['events'].pop(),
            lambda c, _: c['accounting']['trades'].pop(),
            lambda c, _: c['accounting']['marks'][0].update(index=True),
            lambda c, _: c['metrics']['headline'].update(trades=1.0),
            lambda c, _: c['metrics']['headline'].update(expectancy='8'),
            lambda c, _: c['metrics']['headline'].update(profitFactorReason='wrong'),
            lambda c, _: c['operands']['bars'][0].append(0),
            lambda c, _: c['float64bits'].update({'$.accounting.startEquity': 100}),
            lambda c, _: c.update(extra=0),
        ]
        for index, fault in enumerate(faults):
            for both in (False, True):
                with self.subTest(index=index, both=both):
                    self.refused(mutate(self.request, fault, both))

    def test_original_cost_source_identity_binding(self):
        def cost(c, companion):
            c['operands']['normalizedCosts']['slippage'] = 0.3
            c['run']['costs']['slippage'] = 0.3
            companion['identity']['costs']['slippage'] = 0.3
            refresh_bits(c)
        self.refused(mutate(request_for(no_trades=True), cost, both=True))
        def profile(c, companion):
            c['accounting']['policy'] = companion['identity']['policy'] = 'E2'
        self.refused(mutate(self.request, profile, both=True))
        def identity(c, companion):
            c['fixtureSha256'] = companion['identity']['fixtureSha256'] = '0' * 64
        self.refused(mutate(self.request, identity, both=True))
        request = copy.deepcopy(self.request)
        request['source'] += '\n'
        self.refused(request)

    def test_category_sign_order_and_projection_corruption(self):
        faults = [lambda c, _: c['accounting']['trades'][0].update(side='short'),
                  lambda c, _: c['accounting']['events'][0].update(kind='exit'),
                  lambda c, _: c['accounting']['trades'][0].update(tradeIndex=2),
                  lambda c, _: c['metrics']['headline'].update(maxWinStreak=0),
                  lambda c, _: c['accounting']['marks'][0].update(realized=1),
                  lambda c, _: c['accounting']['trades'][0].update(netPnl=-8),
                  lambda c, _: c['metrics']['headline'].update(worstLoss=-0.0),
                  lambda c, _: c['accounting']['marks'][0].update(unrealized=-0.0),
                  lambda c, _: c['accounting']['events'][1].update(realizedAfter=9)]
        for fault in faults:
            def changed(c, companion):
                fault(c, companion)
                refresh_bits(c)
            for both in (False, True):
                self.refused(mutate(self.request, changed, both))

    def test_every_predicted_numeric_field_rejects_one_bit_corruption(self):
        capture = proof.decode(self.request['nativeRaw'])
        replay = proof.Replay(capture, 'linux-arm64').run()
        for path, bits in replay.expected.items():
            def change(c, _):
                parts = __import__('re').findall(r'[^.\[\]]+', path[2:])
                node = c
                for part in parts[:-1]:
                    node = node[int(part)] if type(node) is list else node[part]
                key = int(parts[-1]) if type(node) is list else parts[-1]
                node[key] = value(bits ^ 1)
                refresh_bits(c)
            with self.subTest(path=path):
                self.refused(mutate(self.request, change, both=True))

    def test_unknown_schedule_and_interpreter_fail(self):
        for target in ('darwin-arm64', 'linux-riscv64', 'separate', '', 'linux-amd64-v3'):
            with self.assertRaises(proof.ProofError):
                proof.verify_pair(self.request, target)
        with patch.object(proof.sys, 'version_info', (3, 11)):
            with self.assertRaises(proof.ProofError):
                proof.supported_python()
        with patch.object(proof.platform, 'python_implementation', return_value='PyPy'):
            with self.assertRaises(proof.ProofError):
                proof.supported_python()

    def test_codegen_completeness_and_unknown_instruction_stream(self):
        with self.assertRaises(proof.ProofError):
            proof.instruction_lists('')
        assembly = ''.join(name + ' STEXT size=1\n\t0x0000 00000 (x.go:1)\tRET\n' for name in proof.FUNCTIONS)
        streams = proof.instruction_lists(assembly)
        manifest = proof.decode(proof.MANIFEST.read_text())
        for target in proof.TARGETS:
            observed = {name: {'sha256': proof.sha(('\n'.join(lines) + '\n').encode()), 'instructionCount': len(lines)}
                        for name, lines in streams.items()}
            with self.assertRaises(proof.ProofError):
                proof.verify_instructions(manifest['targets'][target]['instructions'], observed, target)
        with self.assertRaises(proof.ProofError):
            proof.instruction_lists(assembly + assembly)
        with self.assertRaises(proof.ProofError):
            proof.instruction_lists(assembly.replace('\tRET', ' unrecognized', 1))

    def test_exact_explained_target_difference(self):
        # Independent fixed IEEE-754 vector: 0.3 - 0.1*0.3. ARM subtracts
        # the unrounded product; AMD/WASM subtract RN(0.1*0.3).
        request = request_for((100, 101), fee=0.1)
        nominal = value(0x3f9eb851eb851eb8)
        for target, credit_bits in (('native', 0x3fd147ae147ae147), ('wasm', 0x3fd147ae147ae148)):
            capture = proof.decode(request[target + 'Raw'])
            companion = proof.decode(request[target + 'Companion'])
            credit = value(credit_bits)
            net, end = credit - nominal, 100 + (credit - nominal)
            a, m = capture['accounting'], capture['metrics']
            a['trades'][0].update(size=0.3, entryFee=nominal, exitFee=nominal, exitCredit=credit, netPnl=net)
            a['events'][0].update(realizedAfter=-nominal, amount=nominal)
            a['events'][1].update(realizedBefore=-nominal, realizedAfter=net, amount=credit)
            a.update(finalRealized=net, endEquity=end)
            a['marks'][0].update(realized=-nominal, equity=100 - nominal)
            a['marks'][1].update(realized=net, equity=end)
            dd = 100 - (100 - nominal)
            m['headline'].update(endEquity=end, net=net, returnPct=net / 100 * 100,
                                 expectancy=net, avgWin=net, maxDD=dd, maxDDpct=dd / 100 * 100)
            m['tradeAccounting'][0].update(entryFee=nominal, exitFee=nominal, netPnl=net)
            m['equity'][0]['equity'] = 100 - nominal
            m['equity'][1]['equity'] = end
            refresh_bits(capture)
            companion['metrics'] = copy.deepcopy(m)
            request[target + 'Raw'], request[target + 'Companion'] = json.dumps(capture), json.dumps(companion)
        result = proof.verify_pair(request, 'linux-arm64')
        self.assertTrue(result['qualificationPass'])
        self.assertFalse(result['rawCrossTargetExact'])
        self.assertGreaterEqual(len(result['explainedNumericLeaves']), 4)
        with self.assertRaises(proof.ProofError):
            proof.verify_pair(request, 'linux-amd64')

    def test_manifest_source_target_option_corruption(self):
        manifest = proof.decode(proof.MANIFEST.read_text())
        proof.validate_manifest(manifest, ROOT)
        faults = [lambda m: m.update(goVersion='go1.23.0'),
                  lambda m: m['build'].update(GOAMD64='v3'),
                  lambda m: m['build'].update(GOEXPERIMENT='newinliner'),
                  lambda m: m['targets'].pop('js-wasm'),
                  lambda m: m['targets'].update({'linux-riscv64': {}}),
                  lambda m: m['targets']['linux-arm64'].update(accountingSchedule='unreviewed'),
                  lambda m: m['sources'].pop('engine/broker.go'),
                  lambda m: m['sources'].update({'unexpected.s': '0' * 64}),
                  lambda m: m['targets']['js-wasm']['instructions'].pop(proof.FUNCTIONS[0]),
                  lambda m: m['targets']['js-wasm']['instructions'][proof.FUNCTIONS[0]].update(sha256='bad')]
        for fault in faults:
            changed = copy.deepcopy(manifest)
            fault(changed)
            with self.assertRaises(proof.ProofError):
                proof.validate_manifest(changed, ROOT)

    def test_non_go_build_inputs_and_workspace_are_observable(self):
        with tempfile.TemporaryDirectory(prefix='sequential-proof-test-') as directory:
            root = Path(directory)
            (root / 'go.mod').write_text('module ' + proof.MODULE[:-1])
            before = proof.source_hashes(root)
            for name in ('extra.s', 'extra.syso', 'extra.h', 'go.work'):
                (root / name).write_text('synthetic mutation')
                self.assertNotEqual(proof.source_hashes(root), before)
                (root / name).unlink()
            (root / 'embedded.json').write_text('{"synthetic":1}')
            package = dict(ImportPath=proof.MODULE + 'synthetic', Dir=directory,
                           Module=dict(Main=True, Path=proof.MODULE[:-1], Dir=directory),
                           EmbedFiles=['embedded.json'])
            first = proof.dependency_inputs(json.dumps(package), root)
            (root / 'embedded.json').write_text('{"synthetic":2}')
            self.assertNotEqual(proof.dependency_inputs(json.dumps(package), root), first)
            package['Module']['Replace'] = {'Path': 'elsewhere'}
            with self.assertRaises(proof.ProofError):
                proof.dependency_inputs(json.dumps(package), root)

    def test_original_cost_wrong_type_is_not_a_default(self):
        for field, invalid in (('startEquity', False), ('startEquity', None), ('fillOn', False), ('fillOn', None)):
            request = request_for(no_trades=True)
            fixture = proof.decode(request['fixture'])
            fixture['costs'][field] = invalid
            request['fixture'] = json.dumps(fixture)
            digest = proof.sha(request['fixture'].encode())
            def rehash(capture, companion):
                capture['fixtureSha256'] = companion['identity']['fixtureSha256'] = digest
            self.refused(mutate(request, rehash, both=True))

    def test_duplicate_json_keys_fail(self):
        with self.assertRaises(proof.ProofError):
            proof.decode('{"x":1,"x":1}')


if __name__ == '__main__':
    proof.supported_python()
    suite = unittest.defaultTestLoader.loadTestsFromModule(sys.modules[__name__])
    result = unittest.TextTestRunner(stream=sys.stderr, verbosity=1).run(suite)
    print(json.dumps(dict(schema='sequential-proof-tests-v1', passed=result.wasSuccessful(),
                          tests=result.testsRun, failures=len(result.failures), errors=len(result.errors))))
    sys.exit(0 if result.wasSuccessful() else 1)
