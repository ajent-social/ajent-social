#!/usr/bin/env python3
"""Wait for an AWS instance refresh; Pulumi itself does not wait for completion."""
import argparse
import json
import subprocess
import sys
import time


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('group')
    p.add_argument('--region', default='us-west-2')
    p.add_argument('--timeout', type=int, default=2400)
    args = p.parse_args()
    deadline = time.monotonic() + args.timeout
    refresh_id = None
    last = None
    while time.monotonic() < deadline:
        command = ['aws', 'autoscaling', 'describe-instance-refreshes', '--region', args.region,
                   '--auto-scaling-group-name', args.group]
        if refresh_id:
            command += ['--instance-refresh-ids', refresh_id]
        try:
            rows = json.loads(subprocess.check_output(command, timeout=30))['InstanceRefreshes']
        except (subprocess.TimeoutExpired, subprocess.CalledProcessError):
            print('AWS status check failed; retrying within the rollout deadline.', flush=True)
            time.sleep(15)
            continue
        if not rows:
            sys.exit('No instance refresh found; not declaring deployment successful.')
        row = rows[0]
        refresh_id = row['InstanceRefreshId']
        status = row['Status']
        progress = (status, row.get('PercentageComplete', 0))
        if progress != last:
            print(f'{refresh_id}: {status} ({progress[1]}%)', flush=True)
            last = progress
        if status == 'Successful':
            return
        if status in ('Failed', 'Cancelled', 'RollbackFailed', 'RollbackSuccessful'):
            sys.exit('Deployment did not succeed: ' + status + ' ' + row.get('StatusReason', ''))
        time.sleep(15)
    sys.exit('Timed out waiting for the rollout; inspect AWS before another deployment.')


if __name__ == '__main__':
    main()
