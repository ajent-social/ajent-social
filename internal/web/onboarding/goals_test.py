import io
import json
import sys
import unittest
from unittest.mock import patch
import reliability_test as rel
client, mcp = rel.client, rel.mcp

class GoalTools(unittest.TestCase):
    setUp = rel.ClientReliabilityTests.setUp
    exchange = rel.ClientReliabilityTests.exchange
    error = rel.ClientReliabilityTests.error

    def invoke(self, name, args):
        return self.exchange([{'id':1,'method':'tools/call','params':{'name':name,'arguments':args}}])[0]['result']

    def test_update_preserves_version_and_key(self):
        ident='00000000-0000-0000-0000-000000000002'
        args={'id':ident,'version':4,'action':'ask','body':'Choose a deadline','key':'same-key'}
        with patch.object(client,'tool_credentials',return_value=self.c),patch.object(client.feedback_module(),'capabilities',return_value=['goals']),patch.object(client,'request',return_value={'id':ident,'version':5}) as request:
            result=self.invoke('ajent_goal_update',args)
            self.assertFalse(result['isError'])
            self.assertEqual(request.call_args.args,(self.c,'POST','/v1/goals/'+ident+'/events',{'version':4,'action':'ask','body':'Choose a deadline'},'same-key'))

    def test_old_server_blocks_writes_and_forged_human_actions(self):
        with patch.object(client,'tool_credentials',return_value=self.c),patch.object(client.feedback_module(),'capabilities',return_value=[]),patch.object(client,'request') as request:
            result=self.invoke('ajent_goal_track',{'title':'Goal','outcome':'Work','criteria':'Check','authority':'Existing permission','key':'k'})
            self.assertTrue(result['isError']);request.assert_not_called()
        result=self.invoke('ajent_goal_update',{'id':'00000000-0000-0000-0000-000000000002','version':1,'action':'complete','body':'Done','key':'k'})
        self.assertTrue(result['isError'])

    def test_read_large_goal_is_paged_not_omitted(self):
        ident='00000000-0000-0000-0000-000000000002'
        value={'goal':{'id':ident,'outcome':'界'*15000},'events':[]}
        with patch.object(client,'tool_credentials',return_value=self.c),patch.object(client.feedback_module(),'capabilities',return_value=['goals']),patch.object(client,'request',return_value=value):
            result=self.invoke('ajent_goal',{'id':ident})
            payload=json.loads(result['content'][0]['text'])['data']
            self.assertEqual(payload['format'],'json_fragment')
            self.assertGreater(payload['next_offset'],0)
            self.assertLessEqual(len(result['content'][0]['text'].encode()),12000)

    def test_network_retry_preserves_goal_request(self):
        with patch.object(client.urllib.request,'build_opener') as opener,patch.object(client.time,'sleep'):
            opener.return_value.open.side_effect=[self.error(503),io.StringIO('{"version":2}')]
            body={'version':1,'action':'receive','body':'Received'}
            result=client.request(self.c,'POST','/v1/goals/00000000-0000-0000-0000-000000000002/events',body,'stable')
            self.assertEqual(result,{'version':2})
            calls=opener.return_value.open.call_args_list
            self.assertEqual(len(calls),2);self.assertIs(calls[0].args[0],calls[1].args[0])

if __name__=='__main__': unittest.main()
