#!/usr/bin/env python3
"""One bounded triage/metrics pass, suitable for an existing scheduler."""
import argparse
import datetime
import importlib.util
import json
from pathlib import Path
import sys

spec = importlib.util.spec_from_file_location('triage', Path(__file__).with_name('improvement-triage.py'))
triage = importlib.util.module_from_spec(spec); spec.loader.exec_module(triage)


def run(credential, repo, directory, apply=False):
    directory.mkdir(parents=True, mode=0o700, exist_ok=True)
    credential_value = json.loads(credential.read_text())
    key = triage.hashlib.sha256((triage.canonical_origin(credential_value['server']) + ':' + repo.lower()).encode()).hexdigest()
    summary = {'sampled_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
               'state': 'running', 'actions': {}, 'apply': apply}
    with triage.coordinator_lock(directory, key):
        try:
            cursor = ''; pages = 0
            while True:
                page = triage.api(credential_value, 'GET', '/v1/improvements' + ('?cursor=' + triage.urllib.parse.quote(cursor) if cursor else ''))
                pages += 1
                for item in page['items']:
                    if item.get('synthetic') or item['status'] not in ('triaged', 'planned', 'shipped_awaiting_verification'):
                        continue
                    result = triage.sync_one(credential_value, repo, item, apply, directory)
                    action = result['action']; summary['actions'][action] = summary['actions'].get(action, 0) + 1
                next_cursor = page.get('next_cursor')
                if not next_cursor:
                    break
                if next_cursor == cursor or pages >= 100:
                    raise ValueError('Pagination incomplete')
                cursor = next_cursor
            summary['metrics'] = triage.api(credential_value, 'GET', '/v1/improvements/metrics')
            summary['state'] = 'attention' if summary['actions'].get('creation_unknown') else 'ok'
        except Exception:
            summary['state'] = 'unknown'
            summary['error'] = 'Coordinator incomplete; inspect permissions, connectivity and pending creation journals.'
        triage.save_journal(directory / 'latest.json', summary)
    return summary


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--credential', type=Path, required=True)
    p.add_argument('--repo', required=True)
    p.add_argument('--state-dir', type=Path, required=True)
    p.add_argument('--apply', action='store_true')
    args = p.parse_args()
    if not triage.re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', args.repo):
        p.error('Use an exact owner/repository')
    result = run(args.credential, args.repo, args.state_dir, args.apply)
    print(json.dumps(result))
    return 0 if result['state'] == 'ok' else 2


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (OSError, ValueError, KeyError, TypeError):
        print('{"state":"unknown","error":"Coordinator configuration unavailable"}')
        sys.exit(2)
