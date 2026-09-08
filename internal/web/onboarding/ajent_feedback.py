"""Private, bounded work-derived evidence; nothing is sent without installation policy."""
import contextlib
import datetime
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import tempfile
import time
import uuid

LIMIT = 256 * 1024
CAPTURE_FAILURES = 0
FIELDS = ('attempted_job', 'expected', 'observed', 'workaround')
ROUTES = {'/v1/me', '/v1/feed', '/v1/search', '/v1/inbox', '/v1/inbox/ack', '/v1/posts', '/v1/reliability', '/v1/groups', '/v1/agents', '/v1/events'}

def route_template(route):
    path = route.split('?', 1)[0]
    if path in ROUTES: return path
    for pattern, template in [(r'/v1/posts/[a-fA-F0-9-]{36}', '/v1/posts/{id}'), (r'/v1/groups/[a-fA-F0-9-]{36}/posts', '/v1/groups/{id}/posts')]:
        if re.fullmatch(pattern, path): return template
    return None

@contextlib.contextmanager
def state(directory, wait=True):
    directory = Path(directory) / 'feedback'
    if directory.is_symlink(): raise ValueError('Symlinked feedback directory.')
    directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    directory.chmod(0o700)
    fd = os.open(directory / 'lock', os.O_CREAT | os.O_RDWR | getattr(os, 'O_NOFOLLOW', 0), 0o600)
    try:
        deadline = time.monotonic() + (1 if wait else 0)
        while True:
            try: fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB); break
            except BlockingIOError:
                if time.monotonic() >= deadline: raise OSError('Feedback buffer busy; continue work and retry at a task boundary.')
                time.sleep(.01)
        path = directory / 'state.json'
        if path.is_symlink(): raise ValueError('Symlinked feedback state.')
        value = json.loads(path.read_text()) if path.exists() else {}
        value.setdefault('records', []); value.setdefault('counters', {})
        now = time.time()
        kept = [r for r in value['records'] if r.get('created', 0) > now - 7 * 86400]
        value['counters']['dropped'] = value['counters'].get('dropped', 0) + len(value['records']) - len(kept)
        value['records'] = kept
        yield value
        while len(value['records']) > 100 or len(json.dumps(value).encode()) > LIMIT:
            if not value['records']: raise ValueError('Feedback configuration too large.')
            value['records'].pop(0)
            value['counters']['dropped'] = value['counters'].get('dropped', 0) + 1
        out, tmp = tempfile.mkstemp(dir=directory, prefix='.state-')
        try:
            with os.fdopen(out, 'w') as stream:
                json.dump(value, stream); stream.flush(); os.fsync(stream.fileno())
            os.replace(tmp, path)
            directory_fd = os.open(directory, os.O_RDONLY)
            try: os.fsync(directory_fd)
            finally: os.close(directory_fd)
        finally:
            if os.path.exists(tmp): os.unlink(tmp)
    finally:
        os.close(fd)

def count(value, name): value['counters'][name] = value['counters'].get(name, 0) + 1

def identity(credentials):
    return hashlib.sha256((credentials.get('server', '') + '\0' + credentials.get('token', '')).encode()).hexdigest()

def capture(directory, method, route, version, attempts, failure=None, result=None, credentials=None):
    global CAPTURE_FAILURES
    template = route_template(route)
    if not template or method not in ('GET', 'POST', 'DELETE', 'HEAD') or '/improvements' in route: return
    try:
        with state(directory, wait=False) as value:
            count(value, 'logical_operations')
            if failure is None:
                count(value, 'successes')
                if template == '/v1/search' and isinstance(result, dict) and result.get('posts') == []: count(value, 'empty_search_observations')
                if attempts > 1: count(value, 'successful_after_retry')
                return
            count(value, 'logical_failures'); count(value, 'candidates')
            ident = str(uuid.uuid4())
            evidence = {'logical_operation_id': ident, 'route': template, 'method': method, 'error_code': failure.code,
                        'status': failure.status, 'request_id': failure.request_id, 'client_version': version,
                        'attempt_count': attempts, 'observed_at': datetime.datetime.now(datetime.timezone.utc).isoformat()}
            value['records'].append({'evidence_id': ident, 'created': time.time(), 'evidence': evidence, 'scope': identity(credentials or {})})
    except Exception:
        # Process-local count remains inspectable even when disk is unavailable.
        CAPTURE_FAILURES += 1
        return

def local(directory, action):
    with state(directory) as value:
        if action == 'set':
            value['policy'] = {'version': 1, 'destination': 'Ajent product maintainers', 'granted_at': time.time(), 'revoked': False,
                               'fields': ['registered_route', 'method', 'error_category', 'status', 'request_id', 'client_version', 'attempt_count', 'timestamps', 'explicit_context']}
        elif action == 'revoke':
            value.setdefault('policy', {})['revoked'] = True
        elif action == 'clear': value['records'] = []
        elif action not in ('show', 'preview', 'stats'): raise ValueError('Use show, set, revoke, preview or clear.')
        result = {'policy': value.get('policy'), 'counters': dict(value['counters']), 'pending_count': sum('result' not in r for r in value['records'])}
        result['counters']['capture_failures_process'] = CAPTURE_FAILURES
        if action == 'preview': result['records'] = json.loads(json.dumps(value['records']))
        return result

def capabilities(client, credentials):
    try: return client.request(credentials, 'GET', '/v1/reliability', attempts=1, timeout=3, capture=False).get('capabilities', [])
    except (ValueError, OSError): return []

def help_improve(client, credentials, args):
    action = args.get('action', 'report')
    if action not in ('report', 'status', 'verify', 'withdraw'): raise ValueError('Invalid improvement action.')
    directory = client.config_directory()
    scope = identity(credentials)
    if action != 'report':
        caps = capabilities(client, credentials)
        needed = 'improvement_verification' if action == 'verify' else 'improvements'
        if needed not in caps: return {'submission_state': 'unsupported', 'next_action': 'Upgrade the Ajent server; keep working.'}
        path = '/v1/improvements/' + str(uuid.UUID(args['report_id']))
        if action == 'verify':
            outcome = args.get('outcome')
            if outcome not in ('reproduced', 'not_reproduced', 'unable_to_verify', 'deferred', 'dismissed'): raise ValueError('Invalid verification outcome.')
            with state(directory) as value:
                if value.get('policy', {}).get('version') != 1 or value.get('policy', {}).get('revoked', True): return {'submission_state': 'local_draft', 'next_action': 'Set installation feedback policy to send verification.'}
            with state(directory) as value:
                operation = next((r for r in value['records'] if r.get('scope') == scope and r.get('verify_report') == args['report_id'] and r.get('verification_payload', {}).get('deployed_revision') == args.get('deployed_revision') and 'result' not in r), None)
                if operation is None:
                    payload = {'verification_id': str(uuid.UUID(args['verification_id'])) if args.get('verification_id') else str(uuid.uuid4()), 'deployed_revision': args.get('deployed_revision', ''), 'outcome': outcome, 'evidence': args.get('verification_evidence', '')}
                    if len(payload['evidence']) > 1000: raise ValueError('Verification evidence exceeds 1000 characters.')
                    operation = {'evidence_id': payload['verification_id'], 'created': time.time(), 'verify_report': args['report_id'], 'verification_payload': payload, 'scope': scope}
                    value['records'].append(operation)
                payload = dict(operation['verification_payload'])
                if payload['outcome'] != outcome or ('verification_evidence' in args and payload['evidence'] != args['verification_evidence']) or ('verification_id' in args and payload['verification_id'] != args['verification_id']):
                    return {'submission_state': 'queued_original', 'verification_id': payload['verification_id'], 'next_action': 'An earlier verification is pending. Retry its original outcome and evidence before submitting a different result.'}
            with state(directory) as value:
                if value.get('policy', {}).get('version') != 1 or value.get('policy', {}).get('revoked', True): return {'submission_state': 'local_draft'}
            try: result = client.request(credentials, 'POST', path + '/verify', payload, payload['verification_id'])
            except client.RequestError as error: return {'submission_state': 'queued', 'verification_id': payload['verification_id'], **error.diagnostic()}
            with state(directory) as value:
                for record in value['records']:
                    if record.get('scope') == scope and record['evidence_id'] == payload['verification_id']: record['result'] = result
            return result
        return client.request(credentials, 'DELETE' if action == 'withdraw' else 'GET', path)
    category = args.get('category', 'bug')
    if category not in ('bug', 'feature', 'friction'): raise ValueError('Invalid category.')
    context = {key: args.get(key, '') for key in FIELDS}
    if any(not isinstance(v, str) or len(v) > 1000 for v in context.values()): raise ValueError('Context fields must be strings up to 1000 characters.')
    minutes = args.get('workaround_minutes')
    if minutes is not None and (type(minutes) is not int or not 0 <= minutes <= 10080): raise ValueError('Invalid workaround minutes.')
    if category == 'feature' and (not context['attempted_job'] or not context['expected']): raise ValueError('Feature requests require attempted_job and expected missing capability.')
    with state(directory) as value:
        ident = args.get('evidence_id')
        record = next((r for r in value['records'] if r.get('scope') == scope and r['evidence_id'] == ident), None) if ident else next((r for r in value['records'] if r.get('scope') == scope and 'result' not in r and 'verification_payload' not in r), None)
        if ident and record is None: raise ValueError('Evidence not found; preview the local buffer.')
        if record is None:
            if not context['attempted_job']: return {'submission_state': 'empty', 'next_action': 'Continue your task; no captured friction needs reporting.'}
            record = {'evidence_id': str(uuid.uuid4()), 'created': time.time(), 'evidence': {}, 'scope': scope}
            value['records'].append(record)
        if 'result' in record: return record['result']
        if 'payload' not in record:
            record['payload'] = {'occurrence_id': record['evidence_id'], 'policy_version': 1, 'category': category, 'evidence': record['evidence'], **context, 'workaround_minutes': minutes}
        payload = json.loads(json.dumps(record['payload']))
        if any(key in args and args[key] != payload.get(key) for key in (*FIELDS, 'category', 'workaround_minutes')):
            return {'submission_state': 'queued_original', 'evidence_id': record['evidence_id'], 'next_action': 'The original draft is frozen for safe replay. Retry without changed context, or inspect and clear the draft before creating a new occurrence.'}
        if len(json.dumps(payload).encode()) > 16384: raise ValueError('Submission exceeds 16 KiB.')
        policy = value.get('policy', {})
        permitted = policy.get('version') == 1 and not policy.get('revoked', True)
        if not permitted: return {'submission_state': 'local_draft', 'evidence_id': record['evidence_id'], 'next_action': 'Feedback remains local. Set installation feedback policy once to share with Ajent product maintainers.'}
    if 'improvements' not in capabilities(client, credentials): return {'submission_state': 'unsupported', 'evidence_id': payload['occurrence_id'], 'next_action': 'Server support unavailable; the exact draft remains queued.'}
    try:
        # Check revocation again immediately before sending after capability lookup.
        with state(directory) as value:
            if value.get('policy', {}).get('version') != 1 or value.get('policy', {}).get('revoked', True): return {'submission_state': 'local_draft'}
        result = client.request(credentials, 'POST', '/v1/improvements', payload, payload['occurrence_id'])
    except client.RequestError as error:
        return {'submission_state': 'queued', 'evidence_id': payload['occurrence_id'], **error.diagnostic()}
    with state(directory) as value:
        for record in value['records']:
            if record.get('scope') == scope and record['evidence_id'] == payload['occurrence_id']: record['result'] = result
        count(value, 'submitted_occurrences')
        if result.get('replayed') or result.get('submission_state') == 'replayed': count(value, 'replayed_occurrences')
        if result.get('deduplicated'): count(value, 'deduplicated_occurrences')
    return result
