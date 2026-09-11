#!/usr/bin/env python3
"""Compare private `pulumi preview --json --show-sames` migration receipts."""
import json
import sys


def identities(preview):
    summary = preview.get('changeSummary', {})
    if not summary.get('same') or any(count for operation, count in summary.items() if operation != 'same'):
        raise ValueError('Preview must contain unchanged resources and zero changes/replacements.')
    if any(d.get('severity') == 'error' for d in preview.get('diagnostics', [])):
        raise ValueError('Preview contains error diagnostics.')
    resources = {}
    for step in preview.get('steps', []):
        if step.get('op') != 'same':
            raise ValueError('Preview contains a resource operation other than same.')
        state = step.get('newState', {})
        urn = step.get('urn')
        if not urn or not state.get('type'):
            raise ValueError('Preview resource identity is missing.')
        if urn in resources:
            raise ValueError('Preview contains duplicate resource identities.')
        # Provider references include the versioned default-provider URN and ID.
        resources[urn] = (state['type'], state.get('provider'), bool(state.get('protect')))
    if len(resources) < 2 or len(resources) != summary['same']:
        raise ValueError('Incomplete resource receipt; capture with --show-sames.')
    return resources


def compare(before, after):
    old, new = identities(before), identities(after)
    if old != new:
        raise ValueError('Resource URNs, types, providers or protection changed.')
    return len(old)


def main():
    if len(sys.argv) != 3:
        raise SystemExit('Usage: check-infra-migration.py TYPESCRIPT_PREVIEW_JSON GO_PREVIEW_JSON')
    try:
        with open(sys.argv[1]) as source, open(sys.argv[2]) as target:
            count = compare(json.load(source), json.load(target))
    except (OSError, ValueError, TypeError, KeyError) as error:
        raise SystemExit(str(error)) from error
    print(f'{count} unchanged resources; identical URNs, providers and protection; zero changes/replacements.')


if __name__ == '__main__':
    main()
