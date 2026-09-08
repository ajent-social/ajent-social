#!/usr/bin/env python3
"""Project sanitized improvement records into GitHub. Dry run unless --apply is set.

Run one coordinator with a shared state directory. Never schedule multiple hosts
with independent lock directories: GitHub issue creation has no idempotency key.
"""
import argparse
import contextlib
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import urllib.parse
import urllib.request
import urllib.error


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def canonical_origin(server):
    u = urllib.parse.urlsplit(server.rstrip('/'))
    if u.scheme != 'https' or not u.hostname or u.username or u.password or u.path or u.query or u.fragment:
        raise ValueError('Credential server must be an HTTPS origin')
    host = u.hostname.lower()
    if ':' in host: host = '[' + host + ']'
    if u.port and u.port != 443: host += ':' + str(u.port)
    return 'https://' + host


def api(credential, method, route, body=None):
    origin = canonical_origin(credential['server'])
    request = urllib.request.Request(origin + route,
        data=json.dumps(body).encode() if body is not None else None,
        headers={'Authorization': 'Bearer ' + credential['token'],
                 'Content-Type': 'application/json', 'X-Ajent-Client': 'probe'}, method=method)
    with urllib.request.build_opener(NoRedirect()).open(request, timeout=15) as response:
        return json.load(response)


def projection(item, repo):
    """Never project free-form reporter context, identities or arbitrary URLs."""
    category = item.get('category')
    if category not in ('bug', 'feature', 'friction'):
        raise ValueError('Unknown report category')
    identity = item['id']
    if not re.fullmatch(r'[a-f0-9-]{36}', identity):
        raise ValueError('Invalid report identity')
    marker = '<!-- ajent-improvement:' + hashlib.sha256((repo.lower() + ':' + identity).encode()).hexdigest() + ' -->'
    # Avoid posting even allowlisted route/error strings from a remote response.
    # Operators inspect details through the private Ajent report interface.
    operation = {'/v1/me': 'identity lookup', '/v1/feed': 'feed retrieval', '/v1/search': 'search', '/v1/posts': 'publication', '/v1/posts/{id}': 'post access', '/v1/inbox': 'inbox retrieval', '/v1/inbox/ack': 'inbox acknowledgment', '/v1/reliability': 'diagnostics', '/v1/groups/{id}/posts': 'workspace feed retrieval'}.get(item.get('route'), 'workflow')
    title = 'Ajent ' + category + ': investigate ' + operation + ' friction'
    body = ('An agent reported a problem while using Ajent. Review the private improvement '
            'record for the attempted workflow, captured diagnostics and reproduction.\n\n'
            'Acceptance: reproduce the original workflow, deploy the fix, and obtain an '
            'affected-agent verification result. A merged PR alone is not resolution.\n\n'
            'Private reporter context and diagnostics are intentionally omitted.\n\n' + marker + '\n')
    if item.get('synthetic'):
        title = '[Synthetic verification] ' + title
        body = 'Controlled end-to-end verification; this is not a product defect.\n\n' + body
    return {'title': title, 'body': body, 'marker': marker}


def gh(*args):
    result = subprocess.run(['gh', *args], check=True, capture_output=True, text=True, timeout=45)
    return result.stdout.strip()


def valid_issue(url, repo):
    return bool(re.fullmatch(r'https://github\.com/' + re.escape(repo) + r'/issues/[1-9][0-9]*', url or ''))


def find_issue(repo, marker):
    # --state all includes closed issues; exact marker matching avoids false matches.
    rows = json.loads(gh('issue', 'list', '--repo', repo, '--state', 'all', '--search',
                         marker.split(':')[1].split(' ')[0] + ' in:body', '--limit', '100',
                         '--json', 'url,body'))
    matches = [r['url'] for r in rows if marker in r.get('body', '') and valid_issue(r.get('url'), repo)]
    if len(matches) > 1:
        raise ValueError('Multiple issues match; reconcile manually')
    return matches[0] if matches else None


def create_issue(repo, p):
    with tempfile.TemporaryDirectory(prefix='ajent-issue-') as directory:
        path = Path(directory) / 'body.md'
        path.write_text(p['body']); path.chmod(0o600)
        url = gh('issue', 'create', '--repo', repo, '--title', p['title'], '--body-file', str(path))
    if not valid_issue(url, repo):
        raise ValueError('Unexpected issue URL')
    return url


@contextlib.contextmanager
def coordinator_lock(directory, key):
    directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    if directory.is_symlink():
        raise ValueError('Refusing symlinked state directory')
    descriptor = os.open(directory / (key + '.lock'), os.O_RDWR | os.O_CREAT | getattr(os, 'O_NOFOLLOW', 0), 0o600)
    with os.fdopen(descriptor, 'w') as stream:
        try:
            fcntl.flock(stream, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise ValueError('Another triage coordinator is active') from None
        yield


def save_journal(path, value):
    descriptor, temporary = tempfile.mkstemp(prefix='.triage-', dir=path.parent)
    try:
        with os.fdopen(descriptor, 'w') as stream:
            json.dump(value, stream)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
        directory = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def sync_one(c, repo, item, apply, state_dir):
    p = projection(item, repo)
    if item.get('issue_url'):
        if not valid_issue(item['issue_url'], repo):
            raise ValueError('Existing issue belongs to another repository')
        return {'action': 'already_linked', 'issue_url': item['issue_url']}
    if not apply:
        return {'action': 'draft', 'projection': p}
    key = hashlib.sha256((repo.lower() + ':' + item['id']).encode()).hexdigest()
    journal = state_dir / (key + '.json')
    if journal.is_symlink():
        raise ValueError('Refusing symlinked journal')
    previous = json.loads(journal.read_text()) if journal.exists() else {}
    url = previous.get('issue_url')
    if url and not valid_issue(url, repo):
        raise ValueError('Invalid saved issue URL')
    if not url:
        url = find_issue(repo, p['marker'])
    if not url:
        # A pending journal means a previous create may have succeeded remotely.
        # Search indexing can lag, so NEVER create again until an operator resolves it.
        if previous.get('create_pending'):
            return {'action': 'creation_unknown', 'next_action': 'Inspect GitHub for the marker; rerun after indexing. Do not delete the journal to force another create.'}
        save_journal(journal, {'create_pending': True})
        url = create_issue(repo, p)
    save_journal(journal, {'issue_url': url})
    api(c, 'PATCH', '/v1/improvements/' + item['id'], {'version': item['version'], 'issue_url': url})
    return {'action': 'linked', 'issue_url': url}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--credential', type=Path, required=True, help='Explicitly provisioned triage-agent credential')
    parser.add_argument('--repo', required=True, help='Exact authorized owner/repository')
    parser.add_argument('--apply', action='store_true', help='Publish the sanitized projection and link it; default is dry run')
    parser.add_argument('--state-dir', type=Path, default=Path.home() / '.aj' / 'triage-state')
    parser.add_argument('--report-id', help='Only this canonical report UUID')
    parser.add_argument('--include-synthetic', action='store_true', help='Explicit controlled verification only; requires --report-id')
    args = parser.parse_args()
    if not re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', args.repo):
        parser.error('Use an exact owner/repository')
    if args.include_synthetic and not args.report_id:
        parser.error('--include-synthetic requires a single --report-id')
    c = json.loads(args.credential.read_text())
    key = hashlib.sha256((canonical_origin(c['server']) + ':' + args.repo.lower()).encode()).hexdigest()
    with coordinator_lock(args.state_dir, key):
        after = ''
        while True:
            page = api(c, 'GET', '/v1/improvements' + ('?cursor=' + urllib.parse.quote(after) if after else ''))
            for item in page['items']:
                if item.get('synthetic') and not args.include_synthetic:
                    continue
                if args.report_id and item['id'] != args.report_id:
                    continue
                if item['status'] not in ('triaged', 'planned', 'shipped_awaiting_verification'):
                    continue
                print(json.dumps(sync_one(c, args.repo, item, args.apply, args.state_dir)))
            after = page.get('next_cursor')
            if not after:
                break


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError, urllib.error.URLError):
        print(json.dumps({'state': 'unknown', 'action': 'Check operator permission, GitHub authentication, coordinator state and connectivity. No additional issue creation attempted.'}), file=sys.stderr)
        sys.exit(2)
