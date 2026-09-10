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
 p.add_argument('--repository',help='Exact owner/repository for control, decisions and worker health')
 p.add_argument('--decisions',action='store_true',help='List durable triage decisions for --repository')
 p.add_argument('--decision-id',help='Decision UUID to review')
 p.add_argument('--review',choices=['accept','change','reject'])
 p.add_argument('--control',action='store_true',help='Read improvement control for --repository')
 p.add_argument('--set-control',action='store_true',help='Update improvement control from flags')
 p.add_argument('--mode',choices=['off','shadow','canary'])
 p.add_argument('--control-owner',choices=['local','cloud','off'])
 p.add_argument('--operator-id');p.add_argument('--canary-org')
 p.add_argument('--allow-synthetic',action='store_true')
 p.add_argument('--worker-health',action='store_true')
 p.add_argument('--triage-metrics',action='store_true')
 args=p.parse_args();c=json.loads(args.credential.read_text())
 if args.worker_health or args.triage_metrics or args.control or args.set_control or args.decisions or args.decision_id:
  if not args.repository:p.error('--repository is required for control and decision commands')
  repo=args.repository
  if args.worker_health:
   value=triage.api(c,'GET','/v1/improvements/worker-health?repository='+repo)
  elif args.triage_metrics:
   value=triage.api(c,'GET','/v1/improvements/triage-metrics?repository='+repo)
  elif args.control:
   value=triage.api(c,'GET','/v1/improvements/control?repository='+repo)
  elif args.set_control:
   if args.version is None:p.error('A current --version is required for --set-control')
   body={'repository':repo,'version':args.version}
   if args.mode: body['mode']=args.mode
   if args.control_owner: body['owner']=args.control_owner
   if args.operator_id: body['operator_id']=args.operator_id
   if args.canary_org: body['canary_org']=args.canary_org
   body['allow_synthetic']=bool(args.allow_synthetic)
   value=triage.api(c,'PUT','/v1/improvements/control',body)
  elif args.decision_id:
   if not args.review or args.version is None:p.error('--review and --version are required for decision review')
   body={'version':args.version,'action':args.review}
   value=triage.api(c,'POST','/v1/improvements/decisions/'+str(uuid.UUID(args.decision_id))+'/review?repository='+repo,body)
  else:
   q='/v1/improvements/decisions?repository='+repo
   if args.cursor: q+='&cursor='+str(uuid.UUID(args.cursor))
   value=triage.api(c,'GET',q)
 elif args.id:
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
