import importlib.util
import io
import json
from pathlib import Path
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('mcp',Path(__file__).with_name('ajent_mcp.py'))
mcp=importlib.util.module_from_spec(spec);spec.loader.exec_module(mcp)

class Client:
    def __init__(self,domain='sire.run',fail=False):self.domain=domain;self.posts=[];self.fail=fail
    def tool_credentials(self,profile):return {'autojoin_domain':self.domain,'group_id':'00000000-0000-4000-8000-000000000001'}
    def request(self,c,method,path,body=None,key=None):
        if path=='/v1/posts':
            self.posts.append((body,key))
            if self.fail:self.fail=False;raise OSError('private diagnostic')
        return {'id':'fixture'}

class AutojoinTests(unittest.TestCase):
    def run_mcp(self,client,methods):
        messages=[{'jsonrpc':'2.0','id':i,'method':m,'params':{'name':'ajent_me','arguments':{}}} for i,m in enumerate(methods)]
        out=io.StringIO();err=io.StringIO()
        with patch('sys.stdin',io.StringIO(''.join(json.dumps(m)+'\n' for m in messages))),patch('sys.stdout',out),patch('sys.stderr',err):mcp.serve(client,'codex-project')
        return [json.loads(l) for l in out.getvalue().splitlines()],err.getvalue()
    def test_opt_in_checks_in_once_without_task_or_path_capture(self):
        c=Client();out,err=self.run_mcp(c,['initialize','initialize','tools/call'])
        self.assertEqual(len(c.posts),1);self.assertEqual(c.posts[0][0]['audience'],'private')
        self.assertNotIn(str(Path.home()),json.dumps(c.posts));self.assertEqual(len(out),3)
    def test_no_opt_in_no_publication(self):
        c=Client('');self.run_mcp(c,['initialize','tools/call']);self.assertEqual(c.posts,[])
    def test_retry_retains_idempotency_key_and_initialization_survives(self):
        c=Client(fail=True);out,err=self.run_mcp(c,['initialize','tools/call'])
        self.assertEqual(len(c.posts),2);self.assertEqual(c.posts[0][1],c.posts[1][1])
        self.assertEqual(c.posts[0][0],c.posts[1][0]);self.assertNotIn('private diagnostic',err)
        self.assertIn('protocolVersion',out[0]['result'])

if __name__=='__main__':unittest.main()
