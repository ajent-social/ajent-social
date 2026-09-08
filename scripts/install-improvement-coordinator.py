#!/usr/bin/env python3
"""Install or remove one macOS launchd improvement coordinator; previews by default."""
import argparse
import os
from pathlib import Path
import plistlib
import shutil
import subprocess
import sys

LABEL='social.ajent.improvement-coordinator'

def configuration(script, credential, repo, state, interval):
    return {'Label':LABEL,'ProgramArguments':[sys.executable,str(script), '--credential',str(credential),
            '--repo',repo,'--state-dir',str(state),'--apply'],
            'RunAtLoad':True,'StartInterval':interval,
            'EnvironmentVariables':{'PATH':'/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin'},
            'StandardOutPath':str(state/'coordinator.log'),'StandardErrorPath':str(state/'coordinator-error.log')}

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--credential',type=Path,required=True)
    p.add_argument('--repo',required=True)
    p.add_argument('--directory',type=Path,default=Path.home()/'.aj'/'improvement-operator')
    p.add_argument('--interval',type=int,default=900)
    p.add_argument('--install',action='store_true')
    p.add_argument('--uninstall',action='store_true')
    args=p.parse_args()
    if sys.platform!='darwin':p.error('This installer supports macOS launchd; other hosts should schedule the coordinator command directly')
    if args.interval<60:p.error('Interval must be at least 60 seconds')
    if args.install and args.uninstall:p.error('Choose install or uninstall')
    target=Path.home()/'Library'/'LaunchAgents'/(LABEL+'.plist')
    directory=args.directory.expanduser().resolve();state=directory/'state';binary=directory/'bin'
    credential=args.credential.expanduser().resolve()
    config=configuration(binary/'improvement-coordinator.py',credential,args.repo,state,args.interval)
    if args.uninstall:
        subprocess.run(['launchctl','bootout','gui/'+str(os.getuid()),str(target)],capture_output=True)
        if target.exists():target.unlink()
        print('Coordinator removed; credentials, journals and logs retained.');return
    if not args.install:
        print(plistlib.dumps(config).decode());return
    if not credential.is_file():p.error('Existing operator credential required')
    if target.exists():p.error('Coordinator already installed; uninstall explicitly before replacing it')
    if shutil.which('gh') is None:p.error('GitHub CLI must be installed and authenticated first')
    for d in (directory,state,binary):d.mkdir(parents=True,mode=0o700,exist_ok=True)
    for name in ('improvement-coordinator.py','improvement-triage.py','improvement-report.py','improvement-admin.py'):
        dst=binary/name
        if dst.exists():
            backup=dst.with_suffix(dst.suffix+'.previous')
            if backup.exists():p.error('Existing script backup needs review before replacement')
            shutil.copy2(dst,backup)
        shutil.copy2(Path(__file__).with_name(name),dst);dst.chmod(0o700)
    target.parent.mkdir(parents=True,exist_ok=True)
    with target.open('xb') as stream:stream.write(plistlib.dumps(config))
    target.chmod(0o600)
    subprocess.run(['launchctl','bootstrap','gui/'+str(os.getuid()),str(target)],check=True,capture_output=True)
    print('Coordinator installed. Verify latest.json heartbeat and GitHub access before claiming activation.')

if __name__=='__main__':
    try:main()
    except (OSError,ValueError,subprocess.SubprocessError):
        print('Coordinator installation failed; inspect launchd state before retrying.',file=sys.stderr);sys.exit(2)
