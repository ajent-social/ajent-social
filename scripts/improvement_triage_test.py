import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('triage', Path(__file__).with_name('improvement-triage.py'))
m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)

class ImprovementTriageTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(); self.addCleanup(self.tmp.cleanup)
        self.state = Path(self.tmp.name)
        self.item = {'id':'11111111-1111-4111-8111-111111111111','category':'bug','version':1,'status':'triaged',
                     'attempted_job':'PRIVATE-SENTINEL','route':'/private/SECRET','token':'PRIVATE-SENTINEL'}
        self.url='https://github.com/example/project/issues/42'
    def test_dry_run_has_no_network_and_no_private_content(self):
        with patch.object(m,'api') as api,patch.object(m,'gh') as gh:
            result=m.sync_one({},'example/project',self.item,False,self.state)
        api.assert_not_called();gh.assert_not_called()
        self.assertNotIn('PRIVATE',json.dumps(result));self.assertNotIn('SECRET',json.dumps(result))
    def test_create_then_link_failure_recovers_without_duplicate(self):
        with patch.object(m,'find_issue',return_value=None),patch.object(m,'create_issue',return_value=self.url) as create,patch.object(m,'api',side_effect=ValueError):
            with self.assertRaises(ValueError):m.sync_one({},'example/project',self.item,True,self.state)
        create.assert_called_once()
        with patch.object(m,'create_issue') as create,patch.object(m,'api',return_value={}):
            result=m.sync_one({},'example/project',self.item,True,self.state)
        create.assert_not_called();self.assertEqual(result['issue_url'],self.url)
    def test_ambiguous_create_never_blindly_repeats(self):
        with patch.object(m,'find_issue',return_value=None),patch.object(m,'create_issue',side_effect=ValueError):
            with self.assertRaises(ValueError):m.sync_one({},'example/project',self.item,True,self.state)
        with patch.object(m,'find_issue',return_value=None),patch.object(m,'create_issue') as create:
            result=m.sync_one({},'example/project',self.item,True,self.state)
        self.assertEqual(result['action'],'creation_unknown');create.assert_not_called()
    def test_existing_marker_attaches_without_creation(self):
        with patch.object(m,'find_issue',return_value=self.url),patch.object(m,'create_issue') as create,patch.object(m,'api') as api:
            m.sync_one({},'example/project',self.item,True,self.state)
        create.assert_not_called();self.assertEqual(api.call_args.args[-1]['issue_url'],self.url)
    def test_foreign_repository_link_rejected(self):
        self.item['issue_url']='https://github.com/evil/repo/issues/2'
        with self.assertRaises(ValueError):m.sync_one({},'example/project',self.item,True,self.state)
    def test_only_one_local_coordinator(self):
        with m.coordinator_lock(self.state,'key'):
            with self.assertRaises(ValueError):
                with m.coordinator_lock(self.state,'key'):pass
    def test_projection_marker_stable(self):
        a=m.projection(self.item,'example/project');self.item['version']=8
        self.assertEqual(a['marker'],m.projection(self.item,'example/project')['marker'])

    def test_origin_lock_identity_is_canonical(self):
        self.assertEqual(m.canonical_origin('https://EXAMPLE.test:443/'),m.canonical_origin('https://example.test'))
    def test_origin_rejects_credentials(self):
        with self.assertRaises(ValueError):m.canonical_origin('https://user:secret@example.test')
