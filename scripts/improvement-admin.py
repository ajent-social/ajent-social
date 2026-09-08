#!/usr/bin/env python3
"""Private improvement triage. Uses an explicitly provisioned operator credential."""
import argparse
import importlib.util
import json
from pathlib import Path
import sys
import uuid
spec=importlib.util.spec_from_file_location('triage',Path(__file__).with_name('improvement-triage.py'))
triage=importlib.util.module_from_spec(spec);spec.loader.exec_module(triage)

def main():
 p=argparse.ArgumentParser(description=__doc__)
 p.add_argument('--credential',type=Path,required=True)
 p.add_argument('--id',help='Canonical report UUID; omit to list private reports')
 p.add_argument('--cursor',default='')
 p.add_argument('--version',type=int)
 p.add_argument('--status',choices=['triaged','planned','shipped_awaiting_verification','declined','withdrawn'])
 p.add_argument('--priority',choices=['critical','high','normal','low'])
 p.add_argument('--owner');p.add_argument('--workaround');p.add_argument('--reason');p.add_argument('--issue-url');p.add_argument('--deployed-revision')
 args=p.parse_args();c=json.loads(args.credential.read_text())
 if args.id:
  ident=str(uuid.UUID(args.id))
  if not args.version:p.error('A current --version is required for a triage update')
  body={k:getattr(args,k) for k in ('version','status','priority','owner','workaround','reason','issue_url','deployed_revision') if getattr(args,k) is not None}
  value=triage.api(c,'PATCH','/v1/improvements/'+ident,body)
 else:
  value=triage.api(c,'GET','/v1/improvements'+('?cursor='+str(uuid.UUID(args.cursor)) if args.cursor else ''))
 print(json.dumps(value,indent=2))

if __name__=='__main__':
 try:main()
 except (OSError,ValueError,KeyError,TypeError):
  print('{"state":"unknown","action":"Check credential, operator capability, current version and arguments."}',file=sys.stderr);sys.exit(2)
