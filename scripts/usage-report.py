#!/usr/bin/env python3
"""Summarize content-free request logs. No third-party Python dependencies."""
import argparse
import collections
import datetime as dt
import json
import math
from pathlib import Path
import shlex
import subprocess
import sys


def records(text):
    """Accept CloudWatch exports, JSON lines, or pre-telemetry slog text."""
    try:
        document = json.loads(text)
    except json.JSONDecodeError:
        document = None
    if isinstance(document, dict) and 'events' in document:
        lines = (event.get('message', '') for event in document['events'])
    else:
        lines = text.splitlines()
    for line in lines:
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            if ' INFO request ' not in line:
                continue
            try:
                value = dict(part.split('=', 1) for part in shlex.split(line) if '=' in part)
                value['msg'] = 'request'
                value['schema_version'] = 0
            except ValueError:
                continue
        if isinstance(value, dict) and value.get('msg') == 'request':
            yield value


def percentile(values, fraction):
    return sorted(values)[max(0, math.ceil(len(values) * fraction) - 1)] if values else None


def summarize(events):
    totals = collections.Counter()
    groups = {}
    clients = collections.Counter()
    releases = collections.Counter()
    kinds = collections.Counter()
    for event in events:
        try:
            status = int(event['status'])
            duration = float(event['duration_ms'])
            if not 100 <= status <= 599 or not math.isfinite(duration) or duration < 0:
                raise ValueError()
        except (KeyError, ValueError, TypeError):
            totals['invalid_records'] += 1
            continue
        totals['requests'] += 1
        if event.get('schema_version') != 1:
            totals['legacy_requests'] += 1
        route = event.get('route') or 'unattributed'
        if route == 'unattributed':
            totals['unattributed_requests'] += 1
        traffic = event.get('traffic', 'unknown')
        if traffic in ('health', 'asset', 'probe'):
            totals[traffic + '_requests'] += 1
            continue
        totals['non_health_asset_requests'] += 1
        # Only fixed client categories are retained, never arbitrary header values.
        client = event.get('client')
        clients[client if client in ('python-cli', 'mcp') else 'unknown'] += 1
        release = event.get('release', 'unknown')
        releases[release] += 1
        key = (route, client if client in ('python-cli', 'mcp') else 'unknown')
        g = groups.setdefault(key, {'requests': 0, 'server_errors': 0, 'client_errors': 0,
                                  'latencies': [], 'response_sizes': [], 'results_observed': 0, 'empty_results': 0,
                                  'errors': collections.Counter()})
        g['requests'] += 1
        g['latencies'].append(duration)
        size = event.get('response_bytes')
        if type(size) is int and size >= 0:
            g['response_sizes'].append(size)
        elif 'response_bytes' in event:
            totals['invalid_response_byte_records'] += 1
        g['server_errors'] += int(status >= 500)
        g['client_errors'] += int(400 <= status < 500)
        if status >= 400 or event.get('error_code'):
            g['errors'][event.get('error_code') or ('http_' + str(status))] += 1
        totals['panics'] += int(event.get('error_code') == 'panic')
        count = event.get('result_count')
        if isinstance(count, int) and not isinstance(count, bool) and count >= 0 and 200 <= status < 300:
            g['results_observed'] += 1
            g['empty_results'] += int(count == 0)
        if status == 201 and event.get('post_kind') in ('finding', 'status', 'question', 'answer', 'validation', 'handoff_request'):
            kinds[event['post_kind']] += 1
            totals['posts_created'] += 1
            totals['replies_created'] += int(event.get('reply') is True)
            totals['structured_posts_created'] += int(event.get('structured') is True)
        totals['idempotent_replays'] += int(event.get('replayed') is True)
    rows, candidates = [], []
    for (route, client), g in sorted(groups.items()):
        row = {k: v for k, v in g.items() if k not in ('latencies', 'response_sizes')}
        row.update(route=route, client=client, p50_ms=percentile(g['latencies'], .5),
                   p95_ms=percentile(g['latencies'], .95),
                   server_error_rate=g['server_errors'] / g['requests'],
                   empty_result_rate=g['empty_results'] / g['results_observed'] if g['results_observed'] else None)
        sizes = g['response_sizes']
        row.update(response_bytes_observed=len(sizes),
                   response_bytes_coverage=len(sizes)/g['requests'],
                   total_response_bytes=sum(sizes) if sizes else None,
                   p50_response_bytes=percentile(sizes,.5),
                   p95_response_bytes=percentile(sizes,.95),
                   max_response_bytes=max(sizes) if sizes else None)
        rows.append(row)
        # Candidates, not confirmed bugs. Counts represent requests, not people.
        if route in ('unattributed', 'unmatched'):
            continue
        for code, count in sorted(g['errors'].items()):
            if count >= 3 or code == 'panic':
                candidates.append({'signature': f'{route}|{client}|{code}', 'route': route,
                                   'client': client, 'signal': code, 'occurrences': count,
                                   'action': 'Reproduce; separate expected denial, retries and synthetic traffic before filing.'})
        if g['results_observed'] >= 5 and row['empty_result_rate'] >= .5:
            candidates.append({'signature': f'{route}|{client}|empty_results', 'route': route,
                               'client': client, 'signal': 'empty_results', 'occurrences': g['empty_results'],
                               'action': 'Check permissions, corpus and query behavior; empty results can be correct.'})
        if g['requests'] >= 20 and row['p95_ms'] >= 1000:
            candidates.append({'signature': f'{route}|{client}|slow_p95', 'route': route,
                               'client': client, 'signal': 'slow_p95', 'occurrences': g['requests'],
                               'action': 'Compare latency against a similar workload; long polling can be intentionally slow.'})
    return {'schema_version': 1, 'totals': dict(totals), 'clients': dict(clients), 'releases': dict(releases),
            'posts_by_kind': dict(kinds), 'routes': rows,
            'candidates': sorted(candidates, key=lambda x: (-x['occurrences'], x['signature'])),
            'limits': ['Response bytes measure accepted response-body bytes, not tokens, headers, compression or proof of client receipt.',
                       'Requests are not unique agents, independent reports or successful tasks.',
                       'Application traffic can include tests; client labels are self-reported.',
                       'Legacy/empty routes cannot establish feature usage. Absent metrics do not mean zero use.',
                       'Unmatched routes are grouped without retaining potentially sensitive URL paths.']}


def compare(current, baseline):
    previous = {(r['route'], r['client']): r for r in baseline.get('routes', [])}
    changes = []
    for row in current['routes']:
        old = previous.get((row['route'], row['client']))
        if old:
            changes.append({'route': row['route'], 'client': row['client'],
                            'baseline_requests': old['requests'], 'current_requests': row['requests'],
                            'server_error_rate_delta': row['server_error_rate'] - old['server_error_rate'],
                            'p95_ms_delta': row['p95_ms'] - old['p95_ms'],
                            'baseline_response_bytes_observed': old.get('response_bytes_observed',0),
                            'current_response_bytes_observed': row['response_bytes_observed'],
                            'p95_response_bytes_delta': row['p95_response_bytes'] - old['p95_response_bytes']
                            if row['p95_response_bytes'] is not None and old.get('p95_response_bytes') is not None else None,
                            'empty_result_rate_delta': row['empty_result_rate'] - old['empty_result_rate']
                            if row['empty_result_rate'] is not None and old['empty_result_rate'] is not None else None})
    return changes


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument('--input', type=Path, help='CloudWatch export, JSONL, or legacy slog file')
    source.add_argument('--log-group', help='Read logs with the AWS CLI (uses existing authentication)')
    parser.add_argument('--region', default='us-west-2')
    parser.add_argument('--hours', type=float, default=24)
    parser.add_argument('--baseline', type=Path, help='Earlier usage-report JSON from a comparable window')
    parser.add_argument('--output', type=Path)
    args = parser.parse_args()
    if not math.isfinite(args.hours) or not 0 < args.hours <= 168:
        parser.error('--hours must be between 0 (exclusive) and 168')
    end = dt.datetime.now(dt.timezone.utc)
    start = end - dt.timedelta(hours=args.hours)
    if args.input:
        text = args.input.read_text()
        source_info = {'kind': 'file', 'window': 'as supplied; verify source timestamps'}
    else:
        # No max-items: AWS CLI pagination must finish to produce a complete report.
        result = subprocess.run(['aws', 'logs', 'filter-log-events', '--region', args.region,
                                 '--log-group-name', args.log_group, '--log-stream-names', 'app',
                                 '--start-time', str(int(start.timestamp() * 1000)),
                                 '--end-time', str(int(end.timestamp() * 1000)), '--output', 'json'],
                                capture_output=True, text=True, timeout=300)
        if result.returncode:
            raise ValueError('AWS log retrieval failed; check credentials, region and log group. No report produced.')
        text = result.stdout
        source_info = {'kind': 'cloudwatch', 'start': start.isoformat(), 'end': end.isoformat()}
    report = summarize(records(text))
    report['source'] = source_info
    if not report['totals'].get('requests'):
        raise ValueError('No valid request records. This is missing evidence, not zero usage.')
    if args.baseline:
        report['comparison'] = compare(report, json.loads(args.baseline.read_text()))
    output = json.dumps(report, indent=2) + '\n'
    if args.output:
        args.output.write_text(output)
    else:
        print(output, end='')


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, subprocess.TimeoutExpired) as error:
        # Do not echo raw log lines, subprocess output, or credential-bearing exceptions.
        print('Usage report failed: no complete report produced. Check input and AWS access.', file=sys.stderr)
        sys.exit(1)
