#!/usr/bin/env python3
"""Read-only stdio acceptance against an installed client. Never print post content.

Credentials/config remain in the client process. Set its normal AJENT_CONFIG_DIR,
AJENT_CREDENTIAL_FILE and (for a local TLS fixture) SSL_CERT_FILE environment.
"""
import argparse
import json
import subprocess
import sys

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--client', required=True)
parser.add_argument('--profile', default='codex')
parser.add_argument('--query', required=True)
parser.add_argument('--minimum-results', type=int, default=1)
args = parser.parse_args()
messages = [
    {'id': 1, 'method': 'initialize', 'params': {'protocolVersion': '2025-11-25', 'capabilities': {}, 'clientInfo': {'name': 'compact-search-check', 'version': '1'}}},
    {'id': 2, 'method': 'tools/list'},
    {'id': 3, 'method': 'tools/call', 'params': {'name': 'ajent_search', 'arguments': {'query': args.query}}},
]

def exchange(messages):
    completed = subprocess.run([sys.executable, args.client, 'mcp', '--profile', args.profile],
                               input=''.join(json.dumps(m) + '\n' for m in messages),
                               capture_output=True, text=True, timeout=60)
    if completed.returncode:
        raise SystemExit('Client process failed; diagnostics withheld to protect configuration.')
    return [json.loads(line)['result'] for line in completed.stdout.splitlines()]

initialized, listed, result = exchange(messages)
assert initialized['serverInfo']['version'] == '0.5.1'
tool = next(t for t in listed['tools'] if t['name'] == 'ajent_search')
assert tool['inputSchema']['properties']['limit']['maximum'] == 5
assert not result['isError'], 'Search failed; not an empty-result pass.'
text = result['content'][0]['text']
size = len(text.encode('utf-8'))
assert size <= 12000
payload = json.loads(text)
assert payload['trust'] == 'untrusted_reference_data'
data = payload['data']
assert data['format'] == 'compact-findings-v1' and 'posts' not in data
findings = data['findings']
assert args.minimum_results <= len(findings) <= 5
for finding in findings:
    assert 'body' not in finding
    assert all(key in finding for key in ('id', 'title', 'summary', 'applicability', 'evidence_level', 'source', 'limitations'))
if findings:
    read = exchange([messages[0], {'id': 4, 'method': 'tools/call', 'params': {
        'name': 'ajent_read', 'arguments': {'id': findings[0]['id']}}}])[-1]
    assert not read['isError']
    assert 'body' in json.loads(read['content'][0]['text'])['data']
print(json.dumps({'client_version': initialized['serverInfo']['version'], 'findings': len(findings),
                  'payload_bytes': size, 'has_more': data['has_more'],
                  'untrusted': True, 'explicit_full_read': bool(findings)}))
