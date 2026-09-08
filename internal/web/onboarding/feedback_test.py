import importlib.util
import io
import json
import multiprocessing
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import uuid
import sys
sys.path.insert(0,str(Path(__file__).parent))
import ajent_feedback as feedback
import join as client

def writer(path):
    for _ in range(20): feedback.capture(path,'GET','/v1/search?q=SENTINEL', '0.4.0',3,client.RequestError(503))

class FeedbackTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
        self.directory=Path(self.tmp.name)
        client.ORIGIN='https://ajent.test'
        self.env=patch.dict(os.environ,{'AJENT_CONFIG_DIR':str(self.directory)});self.env.start();self.addCleanup(self.env.stop)
    def test_concurrent_private_bounded_redacted(self):
        jobs=[multiprocessing.Process(target=writer,args=(self.tmp.name,)) for _ in range(6)]
        for job in jobs:job.start()
        for job in jobs:job.join();self.assertEqual(job.exitcode,0)
        value=feedback.local(self.directory,'preview')
        self.assertGreater(len(value['records']),0);self.assertLessEqual(len(value['records']),100)
        self.assertLessEqual(value['counters']['logical_operations'],120)
        raw=(self.directory/'feedback/state.json').read_text();self.assertNotIn('SENTINEL',raw)
        self.assertEqual((self.directory/'feedback/state.json').stat().st_mode & 0o777,0o600)
    def test_expiry_clear_preserves_policy(self):
        feedback.local(self.directory,'set');writer(self.tmp.name)
        with feedback.state(self.directory) as value:value['records'][0]['created']=0
        self.assertEqual(feedback.local(self.directory,'preview')['pending_count'],19)
        self.assertEqual(feedback.local(self.directory,'clear')['pending_count'],0)
        self.assertFalse(feedback.local(self.directory,'show')['policy']['revoked'])
    def test_failopen_and_excluded_feedback(self):
        with patch.object(feedback,'state',side_effect=OSError('unwritable')):
            feedback.capture(self.directory,'GET','/v1/feed','0.4.0',1,client.RequestError(403))
        feedback.capture(self.directory,'POST','/v1/improvements','0.4.0',3,client.RequestError(503))
        self.assertEqual(feedback.local(self.directory,'preview')['pending_count'],0)
    def test_policy_no_network_and_lost_response_exact_payload(self):
        writer(self.tmp.name)
        with patch.object(client,'request') as request:
            out=feedback.help_improve(client,{},{});self.assertEqual(out['submission_state'],'local_draft');request.assert_not_called()
        feedback.local(self.directory,'set')
        calls=[]
        def request(c,method,path,body=None,key=None,**kwargs):
            if method=='GET':return {'capabilities':['improvements']}
            calls.append(json.dumps(body,sort_keys=True))
            if len(calls)==1:raise client.RequestError(0)
            return {'report_id':'receipt','status':'new'}
        with patch.object(client,'request',side_effect=request):
            first=feedback.help_improve(client,{},{});self.assertEqual(first['submission_state'],'queued')
            changed=feedback.help_improve(client,{}, {'evidence_id':first['evidence_id'],'observed':'changed context'})
            self.assertEqual(changed['submission_state'],'queued_original');self.assertEqual(len(calls),1)
            second=feedback.help_improve(client,{}, {'evidence_id':first['evidence_id']})
            self.assertEqual(second['report_id'],'receipt');self.assertEqual(calls[0],calls[1])
        feedback.local(self.directory,'revoke')
        with patch.object(client,'request') as request:
            feedback.help_improve(client,{},{});request.assert_not_called()
    def test_real_request_exhaustion_single_logical_failure(self):
        import urllib.error
        client.ORIGIN='https://ajent.test'
        with patch.object(client.urllib.request,'build_opener') as opener,patch.object(client.time,'sleep'):
            opener.return_value.open.side_effect=urllib.error.URLError('SENTINEL')
            with self.assertRaises(client.RequestError):client.request({'server':client.ORIGIN,'token':'SENTINEL'},'GET','/v1/search?q=SENTINEL')
        records=feedback.local(self.directory,'preview')['records'];self.assertEqual(len(records),1);self.assertEqual(records[0]['evidence']['attempt_count'],3)
        self.assertNotIn('SENTINEL',json.dumps(records))
    def test_recovery_not_candidate(self):
        feedback.capture(self.directory,'GET','/v1/search?q=private','0.4.0',2,result={'items':[]})
        value=feedback.local(self.directory,'preview');self.assertEqual(value['pending_count'],0);self.assertEqual(value['counters']['empty_search_observations'],1);self.assertEqual(value['counters']['successful_after_retry'],1)
    def test_backup_rollback_preserves_sentinels(self):
        config=self.directory/'config.json';config.write_text('{"sentinel":"config"}')
        hook=self.directory/'hook.sh';hook.write_text('sentinel hook');hook.chmod(0o700)
        new=self.directory/'new.py'
        backup=client.backup_install(self.directory,[config,hook,new])
        config.write_text('changed');hook.write_text('changed');new.write_text('new')
        client.rollback_install(self.directory,backup.name)
        self.assertEqual(config.read_text(),'{"sentinel":"config"}');self.assertEqual(hook.read_text(),'sentinel hook');self.assertFalse(new.exists());self.assertEqual(hook.stat().st_mode & 0o777,0o700)
    def test_credential_isolation_and_nonblocking_capture(self):
        import fcntl
        first={'server':'https://ajent.test','token':'one'};second={'server':'https://ajent.test','token':'two'}
        feedback.capture(self.directory,'GET','/v1/feed','0.4.0',1,client.RequestError(503),credentials=first)
        evidence_id=feedback.local(self.directory,'preview')['records'][0]['evidence_id']
        with patch.object(client,'request') as request:
            self.assertEqual(feedback.help_improve(client,second,{})['submission_state'],'empty')
            with self.assertRaises(ValueError):feedback.help_improve(client,second,{'evidence_id':evidence_id})
            request.assert_not_called()
        with feedback.state(self.directory):
            before=__import__('time').monotonic()
            feedback.capture(self.directory,'GET','/v1/feed','0.4.0',1,client.RequestError(503),credentials=first)
            self.assertLess(__import__('time').monotonic()-before,.1)
        self.assertGreater(feedback.CAPTURE_FAILURES,0)
    def test_actual_stdio_capabilities_local_draft_and_lost_response(self):
        import subprocess
        writer(self.tmp.name);feedback.local(self.directory,'set')
        script = """
import sys
sys.path.insert(0,sys.argv[1])
import join as client, ajent_mcp
client.root_credentials=lambda:{}
client.existing_tool_credentials=lambda profile:{}
client.startup_inbox=lambda profile:{'state':'unavailable'}
attempts=[]
def request(c,method,path,body=None,key=None,**kwargs):
    if method=='GET':return {'capabilities':['improvements','improvement_verification']}
    attempts.append(body)
    if len(attempts)==1:raise client.RequestError(0)
    assert attempts[0]==body
    return {'report_id':'fixture','status':'new','submission_state':'replayed'}
client.request=request
ajent_mcp.serve(client,'fixture')
"""
        messages=[{'id':1,'method':'initialize'},{'id':2,'method':'tools/list'}, {'id':3,'method':'tools/call','params':{'name':'ajent_help_improve','arguments':{}}}, {'id':4,'method':'tools/call','params':{'name':'ajent_help_improve','arguments':{}}}]
        result=subprocess.run([sys.executable,'-c',script,str(Path(__file__).parent)],input=''.join(json.dumps(m)+'\n' for m in messages),capture_output=True,text=True,check=True)
        out=[json.loads(line) for line in result.stdout.splitlines()]
        tool=next(t for t in out[1]['result']['tools'] if t['name']=='ajent_help_improve')
        self.assertIn('verify',tool['inputSchema']['properties']['action']['enum'])
        self.assertNotIn('approved',tool['inputSchema']['properties'])
        self.assertIn('queued',out[2]['result']['content'][0]['text'])
        self.assertIn('replayed',out[3]['result']['content'][0]['text'])
    def test_verification_queue_preserves_id_and_rejects_changed_outcome(self):
        feedback.local(self.directory,'set')
        calls=[]
        def request(c,method,path,body=None,key=None,**kwargs):
            if method=='GET':return {'capabilities':['improvements','improvement_verification']}
            calls.append(body)
            if len(calls)==1:raise client.RequestError(0)
            return {'status':'verified','replayed':True}
        args={'action':'verify','report_id':str(uuid.uuid4()),'deployed_revision':'abc123','outcome':'not_reproduced','verification_evidence':'retried original workflow'}
        with patch.object(client,'request',side_effect=request):
            self.assertEqual(feedback.help_improve(client,{},args)['submission_state'],'queued')
            self.assertEqual(feedback.help_improve(client,{},dict(args,outcome='reproduced'))['submission_state'],'queued_original')
            self.assertEqual(len(calls),1)
            self.assertEqual(feedback.help_improve(client,{},args)['status'],'verified')
            self.assertEqual(calls[0],calls[1])
    def test_byte_limit_eviction(self):
        with feedback.state(self.directory) as value:
            value['records']=[{'evidence_id':str(uuid.uuid4()),'created':__import__('time').time(),'payload':'x'*15000} for _ in range(100)]
        raw=(self.directory/'feedback/state.json').read_bytes()
        self.assertLessEqual(len(raw),feedback.LIMIT)
        self.assertGreater(feedback.local(self.directory,'show')['counters']['dropped'],0)
    def test_component_upgrade_single_rollback_includes_prior_client_config(self):
        from types import SimpleNamespace
        import contextlib
        client_path=self.directory/'client.py';client_path.write_text('old client')
        config=self.directory/'config.json';config.write_text('{"sentinel":true}')
        hook=self.directory/'marketplace/plugins/ajent/scripts/session_start.py';hook.parent.mkdir(parents=True);hook.write_text('old hook')
        prior=client.backup_install(self.directory,[client_path,config])
        client_path.write_text('new client');config.write_text('{"sentinel":true,"updated":true}')
        bundle={'ajent_feedback.py':'# module','marketplace/plugins/ajent/scripts/session_start.py':'new hook'}
        with patch.object(client.urllib.request,'build_opener') as opener,patch.dict(os.environ,{'AJENT_INTEGRATIONS':'0'}),patch.dict(sys.modules,{'ajent_integrations':SimpleNamespace(install=lambda *a:None)}):
            opener.return_value.open.return_value=io.StringIO(json.dumps(bundle))
            client.install_components(self.directory,client_path,prior)
        backup=sorted((self.directory/'backups').iterdir())[-1]
        client.rollback_install(self.directory,backup.name)
        self.assertEqual(client_path.read_text(),'old client');self.assertEqual(config.read_text(),'{"sentinel":true}');self.assertEqual(hook.read_text(),'old hook')
        self.assertFalse((self.directory/'ajent_feedback.py').exists())
    def test_feature_requires_attempt_and_missing_capability(self):
        with self.assertRaises(ValueError):feedback.help_improve(client,{}, {'category':'feature','attempted_job':'find'})
        out=feedback.help_improve(client,{}, {'category':'feature','attempted_job':'find','expected':'missing filter'})
        self.assertEqual(out['submission_state'],'local_draft')
    def test_unsupported_server_preserves_draft(self):
        writer(self.tmp.name);feedback.local(self.directory,'set')
        with patch.object(client,'request',return_value={'capabilities':[]}) as request:
            self.assertEqual(feedback.help_improve(client,{}, {})['submission_state'],'unsupported')
            self.assertEqual(request.call_count,1)
if __name__=='__main__':unittest.main()
