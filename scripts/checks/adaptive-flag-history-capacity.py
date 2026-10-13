#!/usr/bin/env python3
"""Invented-only Linux capacity qualification for adaptive-history-v1.

Usage: python3 adaptive-flag-history-capacity.py --native /path/heisentick \
    --output-dir /new/evidence/directory
       python3 adaptive-flag-history-capacity.py --self-test --output-dir /new/tests

Each CLI run is a new process, with no process/state reuse or source slicing.
The fixed supervisor checks wall time, continuously drains bounded stdout, samples
Linux VmRSS/VmHWM, and checks wait4 ru_maxrss after exit. RSS sampling cannot make
an instantaneous kernel-enforced RSS guarantee; a terminal peak above the ceiling
still invalidates a result. Only the owned child PID is killed on a breach. Full
accepted stdout is retained verbatim; failure captures are never accepted output.
No candles from history, network requests, Python economic oracle or study claim.
"""

import argparse
import array
import base64
from collections import Counter
import hashlib
import json
import math
import os
from pathlib import Path
import selectors
import signal
import struct
import subprocess
import sys
import time

PROFILE = 'adaptive-history-v1'
SCHEMA = 'strat-adaptive-volume-flag-unit-history-v1'
RAW_SCHEMA = 'strat-adaptive-volume-flag-history-raw-v1'
ROWS = 70_000
WARMUP = 20_000
DAYS = 2_200
TIMEOUT_SECONDS = 240.0
RSS_BYTES = 5 * 1024 ** 3 // 2
STDOUT_BYTES = 256 * 1024 ** 2
STDERR_BYTES = 1024 ** 2
POLL_SECONDS = 0.01
DAY_MS = 86_400_000
COSTS = ('RAW', 'RAZOR_PROXY_BASIC', 'RAZOR_PROXY_HARSH_AGGREGATE')
CORPUS_SHA256 = 'af392754f48672cfe31d7161454dfc35ee73977e4c38f0a72b21c15be6fb2f97'
ROOT = Path(__file__).resolve().parents[2]


def sha_file(path):
    digest = hashlib.sha256()
    with Path(path).open('rb') as stream:
        for data in iter(lambda: stream.read(1024 * 1024), b''):
            digest.update(data)
    return digest.hexdigest()


def save_json(path, data):
    path = Path(path)
    temporary = path.with_suffix(path.suffix + '.writing')
    with temporary.open('x', encoding='utf-8') as stream:
        json.dump(data, stream, indent=2, allow_nan=False, sort_keys=True)
        stream.write('\n')
    temporary.replace(path)


def strict_json(data):
    def object_pairs(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError('duplicate JSON key: ' + key)
            result[key] = value
        return result

    def nonfinite(value):
        raise ValueError('nonfinite JSON number: ' + value)

    def finite_float(value):
        number = float(value)
        if not math.isfinite(number):
            raise ValueError('nonfinite JSON numeric value')
        return number

    return json.loads(data, object_pairs_hook=object_pairs, parse_constant=nonfinite, parse_float=finite_float)


def proc_rss(pid):
    try:
        text = Path(f'/proc/{pid}/status').read_text()
    except FileNotFoundError:
        return 0
    observed = [int(line.split()[1]) * 1024 for line in text.splitlines()
                if line.startswith(('VmRSS:', 'VmHWM:'))]
    return max(observed, default=0)


def supervise(command, directory, *, timeout=TIMEOUT_SECONDS, rss_limit=RSS_BYTES,
              stdout_limit=STDOUT_BYTES, cancel_after=None, validator=None):
    """Internal test knobs never change the fixed production CLI profile.

    Acceptance requires both complete process success and caller validation. A
    failed stderr/output stream or interruption closes the candidate fail-closed.
    wait4, rather than cumulative RUSAGE_CHILDREN, measures this child only.
    """
    directory.mkdir()
    stdout_path = directory / 'stdout.capture'
    stderr_path = directory / 'stderr.capture'
    measurements = {
        'command': [str(x) for x in command], 'accepted': False,
        'limits': {'seconds': timeout, 'rssBytes': rss_limit, 'stdoutBytes': stdout_limit},
        'rssMeasurement': 'Linux /proc VmRSS/VmHWM sampled every 10ms plus child wait4 ru_maxrss',
        'stdoutBytesObserved': 0, 'stderrBytesObserved': 0, 'peakRssBytes': 0,
    }
    child = None
    selector = selectors.DefaultSelector()
    started = time.monotonic()
    reason = None
    usage = None
    status = None
    interrupted = False
    with stdout_path.open('xb') as stdout, stderr_path.open('xb') as stderr:
        try:
            child = subprocess.Popen([str(x) for x in command], stdin=subprocess.DEVNULL,
                                     stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                     start_new_session=True)
            measurements['pid'] = child.pid
            for stream, role in ((child.stdout, 'stdout'), (child.stderr, 'stderr')):
                os.set_blocking(stream.fileno(), False)
                selector.register(stream, selectors.EVENT_READ, role)
            while status is None or selector.get_map():
                elapsed = time.monotonic() - started
                if status is None:
                    measurements['peakRssBytes'] = max(measurements['peakRssBytes'], proc_rss(child.pid))
                    if reason is None:
                        if elapsed > timeout:
                            reason = 'timeout'
                        elif measurements['peakRssBytes'] > rss_limit:
                            reason = 'rss_limit'
                        elif cancel_after is not None and elapsed >= cancel_after:
                            reason = 'cancelled'
                    if reason is not None:
                        try:
                            os.kill(child.pid, signal.SIGKILL)
                        except ProcessLookupError:
                            pass
                for key, _ in selector.select(POLL_SECONDS):
                    chunk = os.read(key.fd, 64 * 1024)
                    if not chunk:
                        selector.unregister(key.fileobj)
                        key.fileobj.close()
                        continue
                    role = key.data
                    counter = role + 'BytesObserved'
                    previous = measurements[counter]
                    measurements[counter] += len(chunk)
                    limit = stdout_limit if role == 'stdout' else STDERR_BYTES
                    destination = stdout if role == 'stdout' else stderr
                    # Keep at most limit+1, enough to prove the breach without an
                    # unbounded capture. Accepted captures preserve every byte.
                    destination.write(chunk[:max(0, limit + 1 - previous)])
                    if measurements[counter] > limit and reason is None:
                        reason = role + '_limit'
                        if status is None:
                            try:
                                os.kill(child.pid, signal.SIGKILL)
                            except ProcessLookupError:
                                pass
                if status is None:
                    pid, raw_status, result_usage = os.wait4(child.pid, os.WNOHANG)
                    if pid:
                        status, usage = raw_status, result_usage
                        child.returncode = os.waitstatus_to_exitcode(status)
                # Go CLI does not spawn descendants. Inherited descriptors from
                # a misbehaving child cannot prolong the supervisor indefinitely.
                if status is not None and selector.get_map() and elapsed > timeout:
                    reason = reason or 'unclosed_output'
                    break
        except KeyboardInterrupt:
            interrupted = True
            reason = 'cancelled'
        except Exception as error:
            reason = 'supervisor_error'
            measurements['supervisorError'] = type(error).__name__ + ': ' + str(error)
        finally:
            if child is not None and status is None:
                try:
                    os.kill(child.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                _, status, usage = os.wait4(child.pid, 0)
                child.returncode = os.waitstatus_to_exitcode(status)
            for key in list(selector.get_map().values()):
                selector.unregister(key.fileobj)
                key.fileobj.close()
            selector.close()
    measurements['elapsedSeconds'] = time.monotonic() - started
    measurements['exitCode'] = None if child is None else child.returncode
    if usage is not None:
        measurements['wait4PeakRssBytes'] = usage.ru_maxrss * 1024
        measurements['peakRssBytes'] = max(measurements['peakRssBytes'], usage.ru_maxrss * 1024)
        measurements['userCpuSeconds'] = usage.ru_utime
        measurements['systemCpuSeconds'] = usage.ru_stime
    if measurements['peakRssBytes'] > rss_limit:
        reason = reason or 'rss_limit'
    if measurements['elapsedSeconds'] > timeout:
        reason = reason or 'timeout'
    if child is None or child.returncode != 0:
        reason = reason or 'nonzero_exit'
    if reason is None and measurements['stderrBytesObserved']:
        reason = 'unexpected_stderr'
    if reason is None:
        try:
            data = stdout_path.read_bytes()
            require(data.endswith(b'\n'), 'complete newline-terminated document required')
            document = strict_json(data)
            if validator is not None:
                measurements['validation'] = validator(document)
        except Exception as error:
            reason = 'output_validation'
            measurements['validationError'] = type(error).__name__ + ': ' + str(error)
    measurements['stdoutCaptureBytes'] = stdout_path.stat().st_size
    measurements['stdoutSha256'] = sha_file(stdout_path)
    measurements['stderrCaptureBytes'] = stderr_path.stat().st_size
    measurements['stderrSha256'] = sha_file(stderr_path)
    measurements['accepted'] = reason is None
    measurements['failure'] = reason
    output_name = 'accepted.json' if reason is None else 'failed.stdout'
    stdout_path.rename(directory / output_name)
    stderr_path.rename(directory / 'stderr.txt')
    measurements['stdoutFile'] = output_name
    save_json(directory / 'measurement.json', measurements)
    if interrupted:
        raise KeyboardInterrupt
    return measurements


def require(value, message):
    if not value:
        raise ValueError(message)


def corpus_inputs():
    path = ROOT / 'testsupport/testdata/adaptive-flag-runtime-corpus-v1.json'
    require(sha_file(path) == CORPUS_SHA256, 'invented source corpus identity changed')
    return strict_json(path.read_bytes())


def decode_bars(payload):
    data = base64.b64decode(payload, validate=True)
    magic, version, n, columns = struct.unpack_from('<IIII', data)
    require((magic, version, columns) == (0x31425442, 1, 6), 'bad invented BTB1 header')
    require(len(data) == 16 + n * 48, 'bad invented BTB1 length')
    values = struct.unpack_from('<' + 'd' * n * 6, data, 16)
    return [tuple(values[col * n + row] for col in range(6)) for row in range(n)]


def write_bars(path, bars):
    with path.open('xb') as stream:
        stream.write(struct.pack('<IIII', 0x31425442, 1, len(bars), 6))
        for column in range(6):
            values = array.array('d', (row[column] for row in bars))
            if sys.byteorder != 'little':
                values.byteswap()
            values.tofile(stream)


def fixture(directory, corpus, bundle, timeframe, *, n=ROWS, days=None):
    directory.mkdir()
    step = 1_800_000 if timeframe == 'M30' else 3_600_000
    by_id = {case['id']: case for case in corpus['cases']}
    long_case = by_id[f'{bundle}/{timeframe}/full-long']
    short_case = by_id[f'{bundle}/{timeframe}/full-short']
    source = corpus['sources'][long_case['sourceSha256']]['text'].encode()
    require(hashlib.sha256(source).hexdigest() == long_case['sourceSha256'], 'DSL identity mismatch')
    (directory / 'source.strat').write_bytes(source)
    if days is None:
        # Twenty thousand supplied warmup rows, then fifty 1,000-row periods.
        # Every period has a long flat lead-in and an alternating frozen invented
        # long/short shape. Timestamps and all state remain one uninterrupted run.
        bars = [(i * step, 100., 101., 99., 100., 100.) for i in range(n)]
        for block in range(WARMUP, n, 1000):
            selected = long_case if (block // 1000) % 2 == 0 else short_case
            shape = decode_bars(corpus['assets'][selected['btb1Sha256']]['payload'])
            end = min(block + 1000, n)
            if end - block < len(shape):
                continue
            for index in range(block, end - len(shape)):
                bars[index] = (index * step, *shape[0][1:])
            for index, row in enumerate(shape, end - len(shape)):
                bars[index] = (index * step, *row[1:])
        start, end = WARMUP * step, n * step
        warmup = WARMUP
    else:
        bars = [(0., 100., 101., 99., 100., 100.)]
        start, end, warmup = 0, days * DAY_MS, 0
    write_bars(directory / 'bars.btb1', bars)
    raw = {'schema': 'adaptive-flag-runtime-request-v1', 'numericalPolicy': 'BINARY64_ORDERED_V1',
           'symbol': 'XAUUSD', 'timeframe': timeframe,
           'executionWindow': {'tradeFromMs': start, 'tradeToMs': end}}
    save_json(directory / 'request.json', raw)
    return {'directory': directory, 'rows': len(bars), 'warmup': warmup,
            'days': (end - 1) // DAY_MS - start // DAY_MS + 1,
            'timeframe': timeframe, 'bundle': bundle,
            'dslSha256': sha_file(directory / 'source.strat'),
            'btb1Sha256': sha_file(directory / 'bars.btb1'),
            'rawRequestSha256': sha_file(directory / 'request.json'),
            'window': raw['executionWindow']}


def projection_input(path, policy, data_sha):
    data = {'schema': 'adaptive-flag-unit-request-v1', 'scenario': 'UNIT_POINT_VALUE_1',
            'numericalPolicy': 'BINARY64_ORDERED_V1', 'costPolicy': policy,
            'dataSource': {'id': 'INVENTED_NATIVE_HISTORY_CAPACITY_V1', 'sourceSha256': data_sha}}
    save_json(path, data)
    return data


def success_validator(case, projection, projection_path):
    def validate(document):
        require(type(document) is dict and document.get('schema') == SCHEMA, 'distinct history schema missing')
        require(document.get('profile') == PROFILE and document.get('rawEnvelopeSchema') == RAW_SCHEMA, 'profile/raw schema identity missing')
        require('raw' not in document, 'raw report duplicated in final envelope')
        require(document['rawRequestSha256'] == case['rawRequestSha256'], 'raw request byte identity mismatch')
        require(document['rawRequest'] == strict_json((case['directory'] / 'request.json').read_bytes()), 'raw request changed')
        require(document['projectionRequest'] == projection, 'projection request changed')
        require(document['projectionRequestSha256'] == sha_file(projection_path), 'projection byte identity mismatch')
        require(0 < document['rawEnvelopeBytes'] <= STDOUT_BYTES, 'raw compact byte length outside profile')
        raw_sha = document['rawEnvelopeSha256']
        require(type(raw_sha) is str and len(raw_sha) == 64 and all(c in '0123456789abcdef' for c in raw_sha), 'bad compact raw hash')
        projected = document['projection']
        manifest = projected['manifest']
        require(projected['schema'] == 'adaptive-unit-projection-go-v1', 'ordered Go projection schema changed')
        require(manifest['rawEnvelopeSha256'] == raw_sha, 'raw hash domains differ')
        require(manifest['dslSha256'] == case['dslSha256'] and manifest['btb1Sha256'] == case['btb1Sha256'], 'input identities differ')
        require(manifest['warmupRowsExcludedFromAccounting'] == case['warmup'], 'warmup prefix not retained/excluded intact')
        require(manifest['fundingStatus'] == 'unknown_unmodeled' and manifest['fundingCashU'] is None, 'unknown funding lost')
        require(manifest['costPolicy']['name'] == projection['costPolicy'], 'cost policy changed')
        require(all(manifest['executionWindow'][key] == value for key, value in case['window'].items()), 'half-open window changed')
        require(len(projected['marks']) == case['rows'] - case['warmup'], 'accounting row count changed')
        require(len(projected['daily']) == case['days'], 'UTC day labels changed')
        if projected['marks']:
            require(projected['marks'][0]['row_index'] == case['warmup'], 'accounting restarted at zero')
            require(projected['marks'][-1]['row_index'] == case['rows'] - 1, 'terminal row trimmed')
        require('close_marked_max_drawdown_R' in projected and 'closed_trade_max_drawdown_R' in projected, 'distinct drawdowns lost')
        sides = Counter(order['side'] for order in projected['orders'])
        if case['rows'] == ROWS:
            require(sides['long'] > 0 and sides['short'] > 0 and len(projected['closed_trades']) > 0,
                    'capacity fixture failed to exercise both nonempty lifecycle directions')
        return {'rows': case['rows'], 'warmupRows': case['warmup'], 'accountingRows': len(projected['marks']),
                'days': len(projected['daily']), 'ordersBySide': dict(sides),
                'closedTrades': len(projected['closed_trades']), 'terminalState': projected['terminal']['native_state']['status'],
                'buildIdentity': manifest['buildIdentity'], 'rawEnvelopeSha256': raw_sha,
                'rawEnvelopeBytes': document['rawEnvelopeBytes']}
    return validate


def run_refusal(command, directory):
    # A valid closed refusal is preserved as failed.stdout, never accepted.json.
    result = supervise(command, directory)
    require(not result['accepted'] and result['failure'] == 'nonzero_exit', 'expected bounded native refusal')
    document = strict_json((directory / 'failed.stdout').read_bytes())
    require(document.get('schema') == 'adaptive-flag-unit-error-v1', 'refusal is not one complete closed error')
    require(document['error']['code'] == 'resource_limit', 'unexpected refusal code')
    require((directory / 'stderr.txt').read_text() == 'heisentick: resource_limit\n', 'unstable native refusal diagnostic')
    result['expectedRefusalVerified'] = True
    save_json(directory / 'refusal-verification.json', {'expected': True, 'code': 'resource_limit'})
    return result


def self_tests(directory):
    directory.mkdir()
    tests = [
        ('complete', 'print("{}")', {}, None),
        ('partial', 'import sys;sys.stdout.write("{\\\"incomplete\\\":");sys.stdout.flush()', {}, 'output_validation'),
        ('nonzero', 'import sys;print("{}");sys.exit(7)', {}, 'nonzero_exit'),
        ('duplicate', 'print(\'{"a":1,"a":2}\')', {}, 'output_validation'),
        ('nonfinite', 'print(\'{"a":1e999}\')', {}, 'output_validation'),
        ('timeout', 'import time;print("{",flush=True);time.sleep(10)', {'timeout': .15}, 'timeout'),
        ('cancelled', 'import time;print("{",flush=True);time.sleep(10)', {'cancel_after': .05}, 'cancelled'),
        ('stdout', 'import sys,time;sys.stdout.write("x"*65536);sys.stdout.flush();time.sleep(10)', {'stdout_limit': 1024}, 'stdout_limit'),
        ('rss', 'import time;x=bytearray(128*1024*1024);time.sleep(10)', {'rss_limit': 64*1024*1024}, 'rss_limit'),
    ]
    results = []
    for name, code, options, expected in tests:
        path = directory / name
        result = supervise([sys.executable, '-c', code], path, **options)
        require(result['failure'] == expected, f'supervisor self-test {name} failed: {result}')
        require((path / 'accepted.json').exists() == (expected is None), 'failed partial output was accepted')
        require(result['stdoutCaptureBytes'] <= options.get('stdout_limit', STDOUT_BYTES) + 1, 'unbounded failure capture')
        results.append({'name': name, 'failure': expected, 'measurement': str(path / 'measurement.json')})
    # Popen.kill/send_signal internally polls and may reap a child. Force the
    # child to finish before signalling without reaping it, then require our
    # sole wait4 to retain that exact child's resource accounting. All Popen
    # public reap paths are forbidden while this regression runs.
    path = directory / 'exited-before-limit-kill'
    original_kill = os.kill
    original_methods = {name: getattr(subprocess.Popen, name)
                        for name in ('kill', 'poll', 'wait')}
    signalled = []

    def forbidden_reaper(*args, **kwargs):
        raise AssertionError('Popen reaper invoked before sole wait4')

    def signal_exited_child(pid, sig):
        require(sig == signal.SIGKILL, 'unexpected cleanup signal')
        terminal = os.waitid(os.P_PID, pid, os.WEXITED | os.WNOWAIT)
        require(terminal.si_pid == pid, 'waitid observed a different child')
        signalled.append(pid)
        return original_kill(pid, sig)

    try:
        for name in original_methods:
            setattr(subprocess.Popen, name, forbidden_reaper)
        os.kill = signal_exited_child
        result = supervise([sys.executable, '-c', 'print("x"*2048)'], path,
                           stdout_limit=1024)
    finally:
        os.kill = original_kill
        for name, method in original_methods.items():
            setattr(subprocess.Popen, name, method)
    require(result['failure'] == 'stdout_limit' and not result['accepted'],
            'exited-before-limit race was not a closed refusal')
    require(signalled and set(signalled) == {result['pid']}, 'wrong child signalled')
    require(result['exitCode'] == 0 and 'wait4PeakRssBytes' in result,
            'sole wait4 lost naturally exited child resource evidence')
    require(not (path / 'accepted.json').exists() and (path / 'failed.stdout').exists(),
            'race failure output was accepted')
    require(result['stdoutCaptureBytes'] == 1025, 'race failure capture was unbounded')
    results.append({'name': 'exited-before-limit-kill', 'failure': 'stdout_limit',
                    'measurement': str(path / 'measurement.json')})
    save_json(directory / 'self-test-results.json', results)
    return results


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--native', type=Path)
    parser.add_argument('--output-dir', required=True, type=Path)
    parser.add_argument('--self-test', action='store_true')
    args = parser.parse_args()
    require(sys.platform == 'linux' and hasattr(os, 'wait4') and Path('/proc/self/status').exists(),
            'qualification requires Linux /proc and wait4 resource observations')
    require(args.self_test or args.native is not None, '--native is required for capacity qualification')
    output = args.output_dir.resolve()
    output.mkdir(parents=True, exist_ok=False)
    receipt = {'schema': 'adaptive-flag-history-invented-capacity-v1', 'profile': PROFILE,
               'qualification': 'Invented native-only resource/lifecycle qualification; no historical validation or browser/consumer qualification.',
               'limits': {'rowsIncludingWarmup': ROWS, 'days': DAYS, 'seconds': TIMEOUT_SECONDS,
                          'peakRssBytes': RSS_BYTES, 'stdoutBytes': STDOUT_BYTES},
               'harnessSha256': sha_file(__file__), 'corpusSha256': CORPUS_SHA256, 'reports': [], 'complete': False}
    receipt_path = output / 'receipt.json'
    save_json(receipt_path, receipt)
    def cancellation(signum, frame):
        raise KeyboardInterrupt('supervisor cancelled by signal ' + str(signum))

    previous_term = signal.signal(signal.SIGTERM, cancellation)
    try:
        receipt['supervisorTests'] = self_tests(output / 'supervisor-tests')
        if args.self_test:
            receipt['complete'] = True
            save_json(receipt_path, receipt)
            print('Supervisor self-tests passed; no strategy execution.')
            return 0
        native = args.native.resolve(strict=True)
        receipt['nativeSha256'] = sha_file(native)
        receipt['nativePath'] = str(native)
        corpus = corpus_inputs()
        fixtures = output / 'invented-inputs'
        fixtures.mkdir()
        cases = []
        for bundle in ('INITIAL', 'TWEAKED', 'SNAPSHOT_C'):
            for timeframe in ('M30', 'H1'):
                case = fixture(fixtures / f'{bundle}-{timeframe}-70000', corpus, bundle, timeframe)
                cases.append(case)
                for policy in COSTS:
                    projection_path = case['directory'] / f'{policy}.json'
                    projection = projection_input(projection_path, policy, case['btb1Sha256'])
                    command = [native, 'adaptive-flag-history-unit-report', '--profile=' + PROFILE,
                               '--request-file=' + str(case['directory'] / 'request.json'),
                               '--projection-file=' + str(projection_path), '--dsl-file=' + str(case['directory'] / 'source.strat'),
                               '--bars-file=' + str(case['directory'] / 'bars.btb1')]
                    name = f'{bundle}-{timeframe}-{policy}-70000'
                    result = supervise(command, output / name, validator=success_validator(case, projection, projection_path))
                    receipt['reports'].append({'case': name, **result})
                    save_json(receipt_path, receipt)
                    require(result['accepted'], name + ' capacity run refused; failure evidence retained')
                    print(f'{name}: {result["elapsedSeconds"]:.3f}s, {result["peakRssBytes"]} RSS bytes, {result["stdoutCaptureBytes"]} stdout bytes', flush=True)
        for n, days, label, reject in ((70001, None, '70001-rows', True), (1, 2200, '2200-days', False), (1, 2201, '2201-days', True)):
            case = fixture(fixtures / label, corpus, 'INITIAL', 'M30', n=n, days=days)
            projection_path = case['directory'] / 'RAW.json'
            projection = projection_input(projection_path, 'RAW', case['btb1Sha256'])
            command = [native, 'adaptive-flag-history-unit-report', '--profile=' + PROFILE,
                       '--request-file=' + str(case['directory'] / 'request.json'), '--projection-file=' + str(projection_path),
                       '--dsl-file=' + str(case['directory'] / 'source.strat'), '--bars-file=' + str(case['directory'] / 'bars.btb1')]
            result = run_refusal(command, output / label) if reject else supervise(command, output / label, validator=success_validator(case, projection, projection_path))
            receipt['reports'].append({'case': label, **result})
            save_json(receipt_path, receipt)
            require(reject or result['accepted'], label + ' boundary acceptance failed')
        require(sha_file(native) == receipt['nativeSha256'], 'executing binary changed during qualification')
        receipt['complete'] = True
        save_json(receipt_path, receipt)
        print('18 cold 70,000-row cases and row/day boundaries passed. Full outputs and measurements retained.')
        return 0
    except BaseException as error:
        receipt['failure'] = type(error).__name__ + ': ' + str(error)
        save_json(receipt_path, receipt)
        raise
    finally:
        signal.signal(signal.SIGTERM, previous_term)


if __name__ == '__main__':
    sys.exit(main())
