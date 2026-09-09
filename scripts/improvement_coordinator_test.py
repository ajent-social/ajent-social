import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('coordinator',Path(__file__).with_name('improvement-coordinator.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)

class CoordinatorTest(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
        self.path=Path(self.tmp.name);self.credential=self.path/'credential.json'
        self.credential.write_text(json.dumps({'server':'https://example.test','token':'SECRET'}))
    def test_excludes_synthetic_and_untriaged_and_records_heartbeat(self):
        page={'items':[{'status':'new'},{'status':'triaged','synthetic':True},{'status':'planned'}]}
        with patch.object(m.triage,'api',side_effect=[page,{'reports':1}]),patch.object(m.triage,'sync_one',return_value={'action':'linked'}) as sync:
            result=m.run(self.credential,'example/repo',self.path,True)
        self.assertEqual(sync.call_count,1);self.assertEqual(result['state'],'ok')
        self.assertEqual(json.loads((self.path/'latest.json').read_text())['actions'],{'linked':1})
        self.assertNotIn('SECRET',json.dumps(result))
    def test_failure_is_unknown_not_zero(self):
        with patch.object(m.triage,'api',side_effect=ValueError('SECRET')):
            result=m.run(self.credential,'example/repo',self.path)
        self.assertEqual(result['state'],'unknown');self.assertNotIn('SECRET',json.dumps(result))
    def test_ambiguous_creation_needs_attention(self):
        with patch.object(m.triage,'api',side_effect=[{'items':[{'status':'planned'}]},{}]),patch.object(m.triage,'sync_one',return_value={'action':'creation_unknown'}):
            result=m.run(self.credential,'example/repo',self.path,True)
        self.assertEqual(result['state'],'attention')
