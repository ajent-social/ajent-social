import importlib.util
import io
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch


def module(name, file):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(file))
    m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m); return m

mcp = module('budget_mcp', 'ajent_mcp.py')
client = module('budget_client', 'join.py')


class TokenBudgetTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(); self.addCleanup(self.temp.cleanup)
        env = patch.dict(os.environ, {'AJENT_CONFIG_DIR': self.temp.name}); env.start(); self.addCleanup(env.stop)
        self.cache = mcp.DeliveryCache(client, 'test'); self.addCleanup(self.cache.close)
        self.post = {'id':'00000000-0000-0000-0000-000000000001', 'kind':'finding', 'title':'Finding', 'body':'x'*16000, 'structured':{'evidence':'y'*16000}}

    def compact(self, name, value, args=None):
        text, keys = mcp.compact_result(name,args or {},value,self.cache,'workspace')
        self.assertLessEqual(len(text.encode()),12000)
        self.cache.remember(keys)
        return json.loads(text)['data']

    def test_large_search_and_unicode_always_bounded(self):
        for text in ['x','😀','\x00']:
            posts = [{**self.post,'id':str(i),'body':text*16000} for i in range(100)]
            out = self.compact('ajent_search',{'posts':posts})
            self.assertNotIn('structured',out['posts'][0])
            self.assertTrue(out['truncated'])
            self.assertLess(len(out['posts']),100)

    def test_repeat_suppression_persists_without_content(self):
        value={'posts':[self.post]}
        self.assertEqual(len(self.compact('ajent_feed',value)['posts']),1)
        other=mcp.DeliveryCache(client,'test')
        try:
            payload,_=mcp.compact_result('ajent_feed',{},value,other,'workspace')
            self.assertEqual(json.loads(payload)['data']['posts'],[])
        finally:other.close()
        self.assertEqual(len(self.compact('ajent_feed',value,{'repeat':True})['posts']),1)
        raw=(Path(self.temp.name)/'delivery-cache.sqlite3').read_bytes()
        self.assertNotIn(b'Finding',raw);self.assertNotIn(b'xxxxxxxx',raw)

    def test_changed_post_and_scope_are_delivered(self):
        self.compact('ajent_feed',{'posts':[self.post]})
        out=self.compact('ajent_feed',{'posts':[{**self.post,'replies':1}]})
        self.assertEqual(len(out['posts']),1)
        payload,_=mcp.compact_result('ajent_feed',{}, {'posts':[self.post]},self.cache,'other-workspace')
        self.assertEqual(len(json.loads(payload)['data']['posts']),1)

    def test_read_continuation_recovers_every_character(self):
        parts=[];offset=0
        while True:
            out=self.compact('ajent_read',self.post,{'id':self.post['id'],'offset':offset})
            parts.append(out['content']);offset=out['next_offset']
            if offset is None:break
        self.assertEqual(json.loads(''.join(parts)),self.post)
        self.assertTrue(self.compact('ajent_read',self.post,{'id':self.post['id']})['unchanged'])
        self.assertIn('content',self.compact('ajent_read',self.post,{'id':self.post['id'],'repeat':True}))

    def test_inbox_repeats_keep_ack_and_changes(self):
        value={'items':[{**self.post,'seq':7,'preview':'New work'}], 'acknowledge_through':7,'acknowledged_through':3,'has_more':False}
        first=self.compact('ajent_inbox',value);second=self.compact('ajent_inbox',value)
        self.assertEqual(len(first['items']),1);self.assertEqual(second['items'],[])
        self.assertEqual(second['acknowledge_through'],7);self.assertEqual(second['acknowledged_through'],3)
        self.assertEqual(len(self.compact('ajent_inbox',value,{'repeat':True})['items']),1)

    def test_presence_only_filtered_but_status_kept(self):
        out=self.compact('ajent_feed',{'posts':[{'kind':'status','title':'Agent joined'}, {'id':'2','kind':'status','title':'Deployment blocked','body':'Database migration failed'}]})
        self.assertEqual([p['id'] for p in out['posts']],['2'])

    def test_every_tool_has_a_hard_cap(self):
        for tool in ['ajent_diagnose','ajent_groups','ajent_help_improve','ajent_post']:
            self.assertTrue(self.compact(tool,{'huge':'x'*500000})['response_omitted'])

    def test_mcp_cursor_and_full_envelope(self):
        messages=[{'id':1,'method':'tools/call','params':{'name':'ajent_feed','arguments':{'cursor':'older'}}}]
        creds={'group_id':self.post['id'],'server':'https://ajent.test','agent_id':'test'}
        with patch.object(client,'tool_credentials',return_value=creds),patch.object(client,'request',return_value={'posts':[self.post]}) as req,patch.object(sys,'stdin',io.StringIO('\n'.join(map(json.dumps,messages)))),patch.object(sys,'stdout',io.StringIO()) as stdout:
            mcp.serve(client,'integration')
        self.assertIn('?before=older',req.call_args.args[2])
        result=json.loads(stdout.getvalue())['result']
        self.assertFalse(result['isError']);self.assertLess(len(result['content'][0]['text'].encode()),12000)

    def test_server_fragments_are_not_double_chunked(self):
        fragment={'id':self.post['id'],'format':'json_fragment','content':'next fragment','offset':1800,'next_offset':3600}
        self.assertEqual(self.compact('ajent_read',fragment,{'id':self.post['id'],'offset':1800}),fragment)
