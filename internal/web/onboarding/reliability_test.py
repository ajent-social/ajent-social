import importlib.util
import io
import json
from pathlib import Path
import sys
import unittest
from unittest.mock import patch
import urllib.error


def module(name,file):
    spec=importlib.util.spec_from_file_location(name,Path(__file__).with_name(file));m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
client=module('client','join.py');mcp=module('mcp','ajent_mcp.py')
client.ORIGIN='https://ajent.test'

class ClientReliabilityTests(unittest.TestCase):
    def setUp(self):self.c={'server':client.ORIGIN,'token':'secret-fixture','group_id':'00000000-0000-0000-0000-000000000001'}
    def error(self,code,retry='0'):
        return urllib.error.HTTPError('https://ajent.test',code,'private-error',{'Retry-After':retry},io.BytesIO(b'private-response'))
    def test_retry_idempotent_post(self):
        with patch.object(client.urllib.request,'build_opener') as opener,patch.object(client.time,'sleep'):
            opener.return_value.open.side_effect=[self.error(503),io.StringIO('{"id":"ok"}')]
            self.assertEqual(client.request(self.c,'POST','/v1/posts',{'body':'private'},'same-key'),{'id':'ok'})
            requests=[call.args[0] for call in opener.return_value.open.call_args_list]
            self.assertEqual(len(requests),2);self.assertIs(requests[0],requests[1]);self.assertEqual(requests[0].get_header('Idempotency-key'),'same-key')
    def test_no_retry_unauthorized_or_unsafe_mutation(self):
        for method,path,code in [('GET','/v1/me',401),('POST','/v1/browser-login',503)]:
            with self.subTest(method=method),patch.object(client.urllib.request,'build_opener') as opener:
                opener.return_value.open.side_effect=self.error(code)
                with self.assertRaises(client.RequestError) as caught:client.request(self.c,method,path,{})
                self.assertEqual(opener.return_value.open.call_count,1)
                self.assertNotIn('private',str(caught.exception))
    def test_retry_after_bounded_and_sanitized(self):
        with patch.object(client.urllib.request,'build_opener') as opener:
            opener.return_value.open.side_effect=self.error(429,'3600')
            with self.assertRaises(client.RequestError) as caught:client.request(self.c,'GET','/v1/feed')
            self.assertEqual(opener.return_value.open.call_count,1)
            self.assertEqual(caught.exception.retry_after,3600)
    def test_retry_after_http_date(self):
        with patch.object(client.urllib.request,'build_opener') as opener:
            opener.return_value.open.side_effect=self.error(429,'Wed, 01 Jan 2031 00:00:00 GMT')
            with self.assertRaises(client.RequestError) as caught:client.request(self.c,'GET','/v1/feed')
            self.assertEqual(opener.return_value.open.call_count,1)
            self.assertGreater(caught.exception.retry_after,2)
    def exchange(self,messages):
        with patch.object(sys,'stdin',io.StringIO(''.join(json.dumps(m)+'\n' for m in messages))),patch.object(sys,'stdout',io.StringIO()) as out,patch.object(client,'startup_inbox',return_value={'state':'unavailable'}):
            mcp.serve(client,'test-agent')
            return [json.loads(line) for line in out.getvalue().splitlines()]
    def test_tool_contract_and_startup_failure(self):
        out=self.exchange([{'id':1,'method':'initialize'},{'id':2,'method':'tools/list'}])
        self.assertIn('unavailable',out[0]['result']['instructions'])
        tools={t['name']:t for t in out[1]['result']['tools']}
        for name in ['ajent_inbox','ajent_ack','ajent_retract','ajent_diagnose']:self.assertIn(name,tools)
        self.assertTrue(tools['ajent_retract']['annotations']['destructiveHint'])
        for tool in tools.values():self.assertIn('untrusted',tool['description'])
    def test_login_rejects_remote_origin_and_extra_fields(self):
        with patch.object(client,'root_credentials',return_value=self.c),patch.object(client,'request',return_value={'url':'https://evil.test/connect-login','token':'private'}):
            out=self.exchange([{'id':1,'method':'tools/call','params':{'name':'ajent_login','arguments':{}}}])
            self.assertTrue(out[0]['result']['isError']);self.assertNotIn('evil',json.dumps(out));self.assertNotIn('private',json.dumps(out))
    def test_envelope_and_typed_error(self):
        with patch.object(client,'tool_credentials',return_value=self.c),patch.object(client,'request',return_value={'items':[]}):
            out=self.exchange([{'id':1,'method':'tools/call','params':{'name':'ajent_inbox','arguments':{}}}])
            payload=json.loads(out[0]['result']['content'][0]['text']);self.assertEqual(payload['trust'],'untrusted_reference_data')
        with patch.object(client,'tool_credentials',return_value=self.c),patch.object(client,'request',side_effect=client.RequestError(403)):
            out=self.exchange([{'id':1,'method':'tools/call','params':{'name':'ajent_inbox','arguments':{}}}])
            self.assertIn('permission_denied',json.dumps(out))
    def test_boolean_cursor_rejected(self):
        out=self.exchange([{'id':1,'method':'tools/call','params':{'name':'ajent_ack','arguments':{'through':True}}}])
        self.assertTrue(out[0]['result']['isError'])
if __name__=='__main__':unittest.main()
