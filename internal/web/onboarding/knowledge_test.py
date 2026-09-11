"""Explicit knowledge consent, compatibility and compact MCP boundaries."""
import json
import unittest
import uuid
from unittest.mock import patch
import reliability_test as reliability
client, mcp = reliability.client, reliability.mcp


class KnowledgeTests(unittest.TestCase):
    setUp = reliability.ClientReliabilityTests.setUp
    exchange = reliability.ClientReliabilityTests.exchange

    def call(self, name, args, response):
        with patch.object(client, 'tool_credentials', return_value=self.c), patch.object(client, 'request', side_effect=[{'capabilities': ['global_knowledge_findings']}, response]) as request:
            result = self.exchange([{'id': 1, 'method': 'tools/call', 'params': {'name': name, 'arguments': args}}])[0]['result']
        return result, request

    def test_publication_is_explicit_and_never_changes_web_visibility(self):
        ident = str(uuid.uuid4())
        result, request = self.call('ajent_knowledge_publication', {'id': ident, 'publish': True, 'approved': False}, {})
        self.assertTrue(result['isError']); request.assert_not_called()
        result, request = self.call('ajent_knowledge_publication', {'id': ident, 'publish': True, 'approved': True}, {'state': 'pending'})
        self.assertFalse(result['isError'])
        self.assertEqual(request.call_args.args, (self.c, 'POST', '/v1/posts/' + ident + '/knowledge', {'publish': True}))
        self.assertEqual(json.loads(result['content'][0]['text'])['data']['state'], 'pending')
        result, request = self.call('ajent_knowledge_publication', {'id': ident, 'publish': False, 'approved': False}, {'state': 'cancel_pending'})
        self.assertFalse(result['isError'])
        self.assertEqual(request.call_args.args[-1], {'publish': False})

    def test_old_server_does_not_receive_a_write_or_fallback_post(self):
        with patch.object(client, 'request', return_value={'capabilities': []}) as request:
            with self.assertRaises(client.UnsupportedCapability):
                client.knowledge_action(self.c, {'action': 'publish', 'id': str(uuid.uuid4()), 'approved': True})
        self.assertEqual(request.call_count, 1)
        self.assertEqual(request.call_args.args[1:], ('GET', '/v1/reliability'))

    def test_query_and_ids_validate_before_network(self):
        cases = [{'action': 'search', 'query': ''}, {'action': 'search', 'query': '🧪' * 1025},
                 {'action': 'status', 'id': '../secrets'}, {'action': 'read', 'id': '../secrets'}, {'action': 'unknown'}]
        for args in cases:
            with self.subTest(args=str(args)[:60]), patch.object(client, 'request') as request:
                with self.assertRaises(ValueError): client.knowledge_action(self.c, args)
                request.assert_not_called()

    def test_search_envelope_is_bounded_and_evidence_never_upgraded(self):
        rows = [{'post_id': str(uuid.uuid4()), 'title': '🧪' * 1000, 'excerpt': '🧪\\"\n' * 10000,
                 'evidence_state': 'independently_verified', 'search_state': 'semantic'} for _ in range(20)]
        result, request = self.call('ajent_knowledge_search', {'query': 'related knowledge'}, {'results': rows})
        self.assertFalse(result['isError'])
        text = result['content'][0]['text']; self.assertLessEqual(len(text.encode()), 12000)
        out = json.loads(text)
        self.assertEqual(out['trust'], 'untrusted_reference_data')
        self.assertEqual(len(out['data']['findings']), 5)
        self.assertTrue(out['data']['has_more'])
        self.assertTrue(all(r['evidence_level'] == 'author_reported_unverified' for r in out['data']['findings']))
        self.assertEqual(request.call_args.args[2], '/v1/knowledge/search?q=related+knowledge')

    def test_status_and_empty_search_preserve_honest_state(self):
        ident = str(uuid.uuid4())
        result, request = self.call('ajent_knowledge_status', {'id': ident}, {'state': 'recovery_required'})
        self.assertEqual(request.call_args.args[1:], ('GET', '/v1/posts/' + ident + '/knowledge'))
        self.assertEqual(json.loads(result['content'][0]['text'])['data']['state'], 'recovery_required')
        result, _ = self.call('ajent_knowledge_search', {'query': 'nothing'}, {'results': [], 'truncated': False})
        self.assertEqual(json.loads(result['content'][0]['text'])['data']['findings'], [])

    def test_read_returns_bounded_canonical_fact(self):
        ident = str(uuid.uuid4())
        response = {'post_id': ident, 'fact_id': 'a' * 64, 'fact': 'canonical fact text', 'provenance': 'post:1', 'valid_until': '', 'content_untrusted': True}
        result, request = self.call('ajent_knowledge_read', {'id': ident}, response)
        self.assertFalse(result['isError'])
        self.assertEqual(request.call_args.args[1:], ('GET', '/v1/posts/' + ident + '/knowledge/exact'))
        data = json.loads(result['content'][0]['text'])['data']
        self.assertEqual(data['format'], 'exact-read-v1')
        self.assertEqual(data['post_id'], ident)
        self.assertEqual(data['fact'], 'canonical fact text')
        self.assertEqual(data['evidence_level'], 'author_reported_unverified')

    def test_read_bounds_oversize_fact_text(self):
        ident = str(uuid.uuid4())
        response = {'post_id': ident, 'fact': '🧪' * 20000, 'provenance': 'post:1', 'valid_until': ''}
        result, _ = self.call('ajent_knowledge_read', {'id': ident}, response)
        self.assertFalse(result['isError'])
        self.assertLessEqual(len(result['content'][0]['text'].encode()), 12000)

    def test_old_server_refuses_read_without_network_write(self):
        with patch.object(client, 'request', return_value={'capabilities': []}) as request:
            with self.assertRaises(client.UnsupportedCapability):
                client.knowledge_action(self.c, {'action': 'read', 'id': str(uuid.uuid4())})
        self.assertEqual(request.call_count, 1)
        self.assertEqual(request.call_args.args[1:], ('GET', '/v1/reliability'))


if __name__ == '__main__': unittest.main()
