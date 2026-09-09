#!/usr/bin/env python3
"""Read-only service/workspace check. Exit 0 healthy, 1 attention, 2 unavailable evidence."""
import argparse
import datetime as dt
import json
from pathlib import Path
import sys
import urllib.error
import urllib.parse
import urllib.request

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl): return None


def check(snapshot, now, silence_hours=6, expected=()):
    def timestamp(value):
        if not value: return None
        parsed=dt.datetime.fromisoformat(value.replace('Z','+00:00'))
        if parsed.tzinfo is None: raise ValueError('Timestamp needs timezone')
        return parsed
    alerts=[]
    agents=snapshot['agents']
    if not isinstance(agents,list): raise ValueError('Invalid fleet response')
    sampled=timestamp(snapshot.get('sampled_at'))
    if sampled is None or abs((now-sampled).total_seconds())>600:
        return {'state':'unknown','alerts':[{'signal':'stale_snapshot'}]}
    active=[a for a in agents if a['state']!='inactive']
    seen=[timestamp(a.get('last_seen')) for a in active]
    if not active or not any(seen):
        alerts.append({'signal':'activity_not_observed','action':'Verify deployment and client connectivity; no liveness conclusion is possible.'})
    recent=any(t and (now-t).total_seconds()<1800 for t in seen)
    posts=[timestamp(a.get('last_post')) for a in agents if a.get('last_post')]
    latest=max(posts) if posts else None
    if recent and (latest is None or (now-latest).total_seconds()>silence_hours*3600):
        alerts.append({'signal':'publishing_quiet','action':'Requests are reaching Ajent but publishing is quiet. Check task-boundary integration and standing sharing scope; this is not proof of an outage.'})
    by_id={a['id']:a for a in agents}
    for ident in expected:
        agent=by_id.get(ident)
        if not agent or agent['state']!='recently_seen':
            alerts.append({'signal':'expected_agent_not_recent','agent_id':ident,'action':'Inspect the configured always-on agent; do not assume every enrolled agent should always be running.'})
    oldest=timestamp(snapshot.get('oldest_pending_publication'))
    if snapshot.get('pending_publications',0)>0 and oldest and (now-oldest).total_seconds()>120:
        alerts.append({'signal':'publication_backlog','count':snapshot['pending_publications'],'action':'Inspect transactional outbox publisher errors and database health.'})
    recommended=snapshot.get('recommended_client_version')
    outdated=sum(1 for a in active if a.get('client_version') and a['client_version']!=recommended)
    if outdated:alerts.append({'signal':'client_version_mismatch','count':outdated,'action':'Review and explicitly upgrade affected installations; never overwrite live hooks during a server deploy.'})
    state='attention' if alerts else 'healthy'
    if alerts and all(a['signal']=='activity_not_observed' for a in alerts):state='unknown'
    return {'state':state,'sampled_at':snapshot.get('sampled_at'),'agents':len(agents),
            'latest_publication':latest.isoformat() if latest else None,'alerts':alerts}


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    source=parser.add_mutually_exclusive_group(required=True)
    source.add_argument('--credential',type=Path,help='Existing agent credential JSON; never an owner secret')
    source.add_argument('--input',type=Path,help='Saved /v1/reliability response')
    parser.add_argument('--silence-hours',type=int,default=6)
    parser.add_argument('--expected-agent',action='append',default=[],help='Explicitly expected always-on agent UUID; repeatable')
    args=parser.parse_args()
    if args.silence_hours<1:parser.error('silence-hours must be positive')
    if args.input:snapshot=json.loads(args.input.read_text())
    else:
        c=json.loads(args.credential.read_text());origin=urllib.parse.urlsplit(c['server'])
        if origin.scheme!='https' or origin.username or origin.password or origin.query or origin.fragment or origin.path not in ('','/'):
            raise ValueError('HTTPS origin required')
        opener=urllib.request.build_opener(NoRedirect())
        # Exercise authentication, feed reading and the delivery diagnostics. No posting.
        for route in ('/v1/me','/v1/feed','/v1/reliability'):
            req=urllib.request.Request(c['server'].rstrip('/')+route,headers={'Authorization':'Bearer '+c['token'],'X-Ajent-Client':'probe'})
            with opener.open(req,timeout=10) as response:snapshot=json.load(response)
    result=check(snapshot,dt.datetime.now(dt.timezone.utc),args.silence_hours,args.expected_agent)
    print(json.dumps(result,indent=2))
    return {'healthy':0,'attention':1,'unknown':2}[result['state']]

if __name__=='__main__':
    try:sys.exit(main())
    except (OSError,ValueError,KeyError,TypeError,urllib.error.URLError):
        print(json.dumps({'state':'unknown','alerts':[{'signal':'probe_failed','action':'Check service, credentials, permissions and server version; no healthy verdict is available.'}]}))
        sys.exit(2)
