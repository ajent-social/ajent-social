#!/usr/bin/env python3
"""Report observed improvement outcomes with missing evidence explicit."""
import argparse
import datetime as dt
import json
from pathlib import Path
import sys


def report(snapshot, now, client=None, baseline=None):
    sampled = dt.datetime.fromisoformat(snapshot['sampled_at'].replace('Z','+00:00'))
    if sampled.tzinfo is None or not 0 <= (now-sampled).total_seconds() <= 1800:
        return {'state':'unknown','reason':'stale_coordinator_snapshot'}
    if snapshot.get('state') not in ('ok','attention') or 'metrics' not in snapshot:
        return {'state':'unknown','reason':'incomplete_coordinator_pass'}
    result = {'state':snapshot['state'],'sampled_at':snapshot['sampled_at'],
              'server':snapshot['metrics'],'coordinator_actions':snapshot.get('actions',{}),
              'limits':['Counts are observed reports and occurrences, not independent users or prevented bugs.',
                        'No response or unable-to-verify is not verified resolution.',
                        'Eligible post-fix workflow attempts are not measured; recurrence rates per affected attempt are unavailable.']}
    if client is not None:
        # Explicit bounded projection, not arbitrary state-file upload.
        fields=('logical_operations','logical_failures','successful_after_retry','empty_search_observations','dropped','capture_failures_process','submitted_occurrences','replayed_occurrences','deduplicated_occurrences')
        client=client.get('counters',client)
        result['local_client_counters']={k:client[k] for k in fields if type(client.get(k)) is int and client[k]>=0}
        result['limits'].append('Local counters describe one installation; do not extrapolate to the fleet.')
    if baseline is not None:
        result['baseline']={'sampled_at':baseline.get('sampled_at'),'server':baseline.get('metrics')}
        result['limits'].append('Baseline is displayed for review; window and workload comparability must be established before claiming improvement.')
    return result


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--input',type=Path,required=True,help='Coordinator latest.json snapshot')
    p.add_argument('--client-counters',type=Path)
    p.add_argument('--baseline',type=Path)
    args=p.parse_args()
    result=report(json.loads(args.input.read_text()),dt.datetime.now(dt.timezone.utc),
                  json.loads(args.client_counters.read_text()) if args.client_counters else None,
                  json.loads(args.baseline.read_text()) if args.baseline else None)
    print(json.dumps(result,indent=2));return 2 if result['state']=='unknown' else 0

if __name__=='__main__':
    try:sys.exit(main())
    except (OSError,ValueError,KeyError,TypeError):
        print('{"state":"unknown","reason":"invalid_or_missing_snapshot"}');sys.exit(2)
